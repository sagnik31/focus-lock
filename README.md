# Focus Lock

**Focus Lock** is a high-security application and website blocker for Windows. It enforces self-imposed restrictions that cannot be bypassed during active sessions.

> [!WARNING]
> **USE AT YOUR OWN RISK.** This application marks itself as a Critical Process. Forcefully terminating it during an active session **will cause a Blue Screen of Death (BSOD)** and potential data loss.

## Key Features

- **Unstoppable Sessions**: Once initiated, sessions cannot be cancelled. The process is protected via `RtlSetProcessIsCritical`.
- **Scheduled Blocking**: Configure recurring schedules (e.g., weekdays 9:00-17:00) that automatically enforce blocking without manual intervention.
- **Website Blocking**: Modifies the Windows Hosts file to block domains system-wide. Works across all browsers including Incognito/Private modes.
- **Category Presets**: One-click blocking for Social Media, Entertainment, Gaming, and Adult content.
- **VPN Protection**: Detects and blocks common VPN applications and domains.
- **NTP Time Sync**: Uses Network Time Protocol to prevent bypass via system clock manipulation.
- **Ghost Process**: A background process with an obfuscated name enforces restrictions even when the UI is closed.
- **Persistent Enforcement**: The Ghost process and scheduled task persist across sessions and reboots.

## Installation

### Prerequisites
- **Go** (v1.21+)
- **Node.js** (v18+)
- **Wails CLI**: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

### Build Steps
```bash
git clone https://github.com/yourusername/focus-lock.git
cd focus-lock
wails build
```
The binary is generated at `build/bin/focus-lock.exe`.

## Setup

### One-Time Admin Configuration

For website blocking and persistent enforcement, run this **once** in an elevated PowerShell:

```powershell
schtasks /create /tn "FocusLockGhost" /tr "`"$env:APPDATA\FocusLock\Bin\FocusLockGhost.exe`" --enforce" /sc ONLOGON /rl HIGHEST /f
```

This creates a scheduled task that runs the Ghost process with admin privileges. After this setup:
- Website blocking works without UAC prompts
- Enforcement persists when the UI is closed
- All future sessions work automatically

### Windows Defender Exclusion (if needed)

If Windows Defender blocks the Ghost executable, add an exclusion:

```powershell
Add-MpPreference -ExclusionPath "$env:APPDATA\FocusLock"
```

### UAC Bypass Shortcut (Optional)

To launch the UI without UAC prompts:
1. Navigate to the `scripts` folder.
2. Right-click `setup_uac_bypass.ps1` and select **Run with PowerShell**.
3. Use the created **"Focus Lock"** Desktop shortcut.

## Usage

### Blocking Applications
1. Navigate to the **Apps** tab.
2. Enter an executable name (e.g., `discord.exe`) or select from the list.

### Blocking Websites
1. Navigate to the **Websites** tab.
2. Enter a domain (e.g., `facebook.com`) or use category toggles.

### Manual Sessions
1. Set the duration using the time selector.
2. Click **Start Focus** and confirm.

### Scheduled Sessions
1. Navigate to the **Schedules** tab.
2. Create a schedule with days, start time, and end time.
3. Enable the schedule. Blocking activates automatically during the configured window.

## Technical Architecture

- **Frontend**: React + TypeScript + TailwindCSS
- **Backend**: Go (Wails framework)
- **Enforcement**:
  - **Process Termination**: `CreateToolhelp32Snapshot` + `TerminateProcess` with dual-loop architecture
  - **Network Blocking**: Modifies `C:\Windows\System32\drivers\etc\hosts`
  - **Critical Process**: Kernel panic on unexpected termination

## Disclaimer

This software alters critical system process states. The developers are not responsible for data loss, system instability, or inability to access blocked resources during active sessions. **Focus wisely.**

