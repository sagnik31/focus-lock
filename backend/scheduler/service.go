package scheduler

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"unicode/utf16"
)

// taskXMLTemplate defines the Ghost task. It is registered from XML rather than
// plain schtasks flags because schtasks cannot express two things we rely on:
//
//  1. Power conditions. The schtasks defaults would end enforcement: "Stop if
//     the computer switches to battery power" terminates the Ghost when AC power
//     is lost, and the implicit 72h execution limit does the same after 3 days.
//
//  2. A repetition trigger. The logon trigger repeats every minute forever, so
//     Task Scheduler re-runs the action every minute. MultipleInstancesPolicy
//     IgnoreNew means that while the Ghost is alive the repeat is a no-op, but if
//     the Ghost is killed the next repeat relaunches it within ~1 minute. This is
//     what makes killing the Ghost pointless rather than a bypass: enforcement
//     comes back on its own, with the lock state intact (persisted to config +
//     registry). RestartOnFailure covers a crash/non-zero exit on top of that.
const taskXMLTemplate = `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <Repetition>
        <Interval>PT1M</Interval>
        <StopAtDurationEnd>false</StopAtDurationEnd>
      </Repetition>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>%s</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <RestartOnFailure>
      <Interval>PT1M</Interval>
      <Count>3</Count>
    </RestartOnFailure>
    <Enabled>true</Enabled>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>"%s"</Command>
      <Arguments>--enforce</Arguments>
    </Exec>
  </Actions>
</Task>
`

// EnablePersistence registers the enforcement task securely.
// Runs <exePath> --enforce at logon with highest privileges (Admin), on AC or
// battery, with no time limit. Overwrites any existing task of the same name.
func EnablePersistence(exePath, taskName string) error {
	u, err := user.Current()
	if err != nil {
		return fmt.Errorf("resolve current user: %w", err)
	}

	f, err := os.CreateTemp("", "focuslock-task-*.xml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())

	_, err = f.Write(encodeUTF16LE(fmt.Sprintf(taskXMLTemplate, xmlEscape(u.Uid), xmlEscape(exePath))))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}

	// Capture stderr so a failure reports schtasks's actual message (e.g.
	// "ERROR: Access is denied." when the UI isn't elevated) instead of a bare
	// "exit status 1". Registering a HighestAvailable task needs an elevated
	// process; see scripts/setup_persistence.ps1 for the one-time admin setup.
	cmd := exec.Command("schtasks", "/create", "/tn", taskName, "/xml", f.Name(), "/f")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if runErr := cmd.Run(); runErr != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%s (%w)", msg, runErr)
		}
		return runErr
	}
	return nil
}

// PersistenceCurrent reports whether a scheduled task named taskName already
// exists with the definition we need: it runs the given enforcer exe, is
// battery-safe (does not stop when the machine switches to battery), and
// carries the 1-minute repetition that relaunches a killed enforcer.
//
// It returns false when the task is missing, outdated (e.g. an older build's
// task that stops on battery and has no repetition), or unreadable, so the
// caller re-registers it. When it returns true the caller can skip
// registration: this lets a non-elevated UI avoid a doomed "/create" that would
// fail with "Access is denied" on every launch and session — nagging the user —
// when a correct task (installed once via scripts/setup_persistence.ps1 or an
// elevated run) is already in place.
func PersistenceCurrent(exePath, taskName string) bool {
	if taskName == "" {
		return false
	}
	// schtasks returns non-zero (and Output errors) when the task is missing.
	out, err := exec.Command("schtasks", "/query", "/tn", taskName, "/xml").Output()
	if err != nil {
		return false
	}
	x := strings.ToLower(string(out))
	// Pre-fix tasks stop on battery and have no repetition trigger.
	if !strings.Contains(x, "<stopifgoingonbatteries>false</stopifgoingonbatteries>") {
		return false
	}
	if !strings.Contains(x, "<repetition>") {
		return false
	}
	// The action must point at the current enforcer exe, so a task left over
	// from a different install path is refreshed rather than trusted.
	return strings.Contains(x, strings.ToLower(exePath))
}

// DisablePersistence removes the task.
func DisablePersistence(taskName string) error {
	if taskName == "" {
		return nil
	}
	return exec.Command("schtasks", "/delete", "/tn", taskName, "/f").Run()
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// encodeUTF16LE returns s as UTF-16LE with a BOM, matching the encoding the
// task XML declares.
func encodeUTF16LE(s string) []byte {
	units := utf16.Encode([]rune(s))
	buf := make([]byte, 2+2*len(units))
	buf[0], buf[1] = 0xFF, 0xFE
	for i, u := range units {
		binary.LittleEndian.PutUint16(buf[2+2*i:], u)
	}
	return buf
}
