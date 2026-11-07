; Focus Lock - Inno Setup installer
;
; Builds a single Setup.exe that installs the app AND performs the one-time
; elevated enforcement setup (Defender exclusion, staged enforcer, scheduled
; task, HKLM tamper-protection secret) in one pass - removing the manual
; "run an admin PowerShell script" onboarding step.
;
; Build it with scripts\build_installer.ps1 (which runs `wails build` first,
; then compiles this with the Inno Setup compiler, ISCC.exe). Paths below are
; relative to this .iss file (installer\).
;
; NOTE: the installer runs elevated, so the enforcement setup gets admin rights
; at install time. Strong mode still assumes the signed-in user is a local
; administrator (see scripts\install_setup.ps1); on a standard-user account the
; app installs and runs but stays in reduced mode.

#define AppName "Focus Lock"
#define AppVersion "1.0.0"
#define AppPublisher "Sagnik Chakraborty"
#define AppExe "focus-lock.exe"

[Setup]
AppId={{B6E3F7A2-9C4D-4E1B-8A3F-0F2C7D5E9A10}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
UninstallDisplayIcon={app}\{#AppExe}
OutputDir=Output
OutputBaseFilename=FocusLock-Setup-{#AppVersion}
SetupIconFile=..\build\windows\icon.ico
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
; The bundled setup makes system-wide changes (scheduled task, HKLM secret),
; so Setup must run elevated.
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64
ArchitecturesAllowed=x64

[Files]
Source: "..\build\bin\{#AppExe}"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\build\windows\icon.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\scripts\install_setup.ps1"; DestDir: "{app}\scripts"; Flags: ignoreversion

[Icons]
Name: "{group}\{#AppName}"; Filename: "{app}\{#AppExe}"; IconFilename: "{app}\icon.ico"
Name: "{group}\Uninstall {#AppName}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExe}"; IconFilename: "{app}\icon.ico"; Tasks: desktopicon

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; GroupDescription: "Additional shortcuts:"

[Run]
; One-time elevated enforcement setup. {userappdata} is the signed-in user's
; Roaming AppData (where the enforcer and config live), resolved for the
; invoking user even though Setup itself is elevated.
Filename: "powershell.exe"; \
    Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\scripts\install_setup.ps1"" -ExePath ""{app}\{#AppExe}"" -AppData ""{userappdata}"""; \
    StatusMsg: "Setting up enforcement (this is the one-time admin step)..."; \
    Flags: runhidden waituntilterminated
; Offer to launch the app after install.
Filename: "{app}\{#AppExe}"; Description: "Launch {#AppName}"; Flags: nowait postinstall skipifsilent

[UninstallRun]
; Tear down enforcement (scheduled task, HKLM secret, Defender exclusion,
; %APPDATA%\FocusLock) before the files are removed.
Filename: "powershell.exe"; \
    Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\scripts\install_setup.ps1"" -Uninstall -AppData ""{userappdata}"""; \
    RunOnceId: "FocusLockTeardown"; \
    Flags: runhidden waituntilterminated
