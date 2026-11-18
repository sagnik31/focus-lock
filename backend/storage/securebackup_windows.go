package storage

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// secureDirSDDL locks the secure backup folder to SYSTEM and Administrators
// (full control, inherited by the files in it) with inheritance from
// ProgramData switched off, so a non-elevated process of the signed-in user
// can neither read, replace nor delete the copy kept there.
const secureDirSDDL = "O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"

// secureBackupDir returns %ProgramData%\FocusLock, created (if needed) and
// locked to SYSTEM + Administrators. It returns "" when the folder can't be
// created or its ACL can't be set: an unprotected folder would be no safer
// than %APPDATA%, so the store then runs without the secure copy.
func secureBackupDir() string {
	base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil || base == "" {
		return ""
	}
	dir := filepath.Join(base, "FocusLock")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	if err := protectDir(dir); err != nil {
		return ""
	}
	return dir
}

// protectDir sets dir's owner to Administrators and replaces its DACL with
// secureDirSDDL (protected: nothing inherited from the parent).
func protectDir(dir string) error {
	sd, err := windows.SecurityDescriptorFromString(secureDirSDDL)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		owner, nil, dacl, nil)
}
