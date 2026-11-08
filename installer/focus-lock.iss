; Focus Lock - Inno Setup installer
;
; Builds a single Setup.exe that installs the app AND performs the one-time
; elevated enforcement setup (Defender exclusions, scheduled task that runs the
; installed exe with --enforce, HKLM tamper-protection secret) in one pass -
; removing the manual "run an admin PowerShell script" onboarding step.
;
; Build it with scripts\build_installer.ps1 (which runs `wails build` first,
; then compiles this with the Inno Setup compiler, ISCC.exe). Paths below are
; relative to this .iss file (installer\).
;
; NOTE: the installer runs elevated, so the enforcement setup gets admin rights
; at install time. Strong mode still assumes the signed-in user is a local
; administrator who approves the UAC prompt themselves (see
; scripts\install_setup.ps1): {userappdata} and the task's user SID come from
; the elevated account. On a standard-user account (or over-the-shoulder admin
; credentials) the app installs and runs but stays in reduced mode.

#define AppName "Focus Lock"
#define AppVersion "1.0.0"
#define AppPublisher "Sagnik Chakraborty"
#define AppExe "focus-lock.exe"
#define EnforcerTask "FocusLock Enforcer"

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
; {userappdata} is used deliberately (the enforcer's config lives in the
; signed-in user's Roaming AppData); see the NOTE above.
UsedUserAreasWarning=no
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
; The one-time elevated enforcement setup runs from [Code] (CurStepChanged), not
; here, so a failure is reported instead of silently ignored.
; Offer to launch the app after install - non-elevated, as the signed-in user,
; so the UI runs at medium integrity as in normal use.
Filename: "{app}\{#AppExe}"; Description: "Launch {#AppName}"; Flags: nowait postinstall skipifsilent runasoriginaluser

[UninstallRun]
; Tear down enforcement (tasks, running enforcer/app, hosts-file block, HKLM +
; HKCU state, both Defender exclusions, %APPDATA%\FocusLock) before the files
; are removed. -ExePath lets it remove the install-folder exclusion.
Filename: "{sys}\WindowsPowerShell\v1.0\powershell.exe"; \
    Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\scripts\install_setup.ps1"" -Uninstall -ExePath ""{app}\{#AppExe}"" -AppData ""{userappdata}"" -LogPath ""{%TEMP}\FocusLock-uninstall.log"""; \
    RunOnceId: "FocusLockTeardown"; \
    Flags: runhidden waituntilterminated

[Code]
var
  EnforcerTaskDisabled: Boolean;
  EnforcementReady: Boolean;

function RunHidden(const FileName, Params: String): Integer;
var
  RC: Integer;
begin
  if Exec(FileName, Params, '', SW_HIDE, ewWaitUntilTerminated, RC) then
    Result := RC
  else
    Result := -1;
end;

// Upgrade: the running enforcer (and the app) hold {app}\{#AppExe} open, and the
// task's 1-minute repetition would relaunch a killed enforcer mid-copy. Disable
// the task, end it and stop every focus-lock.exe so the file can be replaced.
// (This also stops an enforcer running from an older dev/staged location.)
// install_setup.ps1 re-registers the task enabled; DeinitializeSetup re-enables
// it if Setup is cancelled or the setup step fails.
function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  RC: Integer;
begin
  EnforcerTaskDisabled := RunHidden(ExpandConstant('{sys}\schtasks.exe'), '/change /tn "{#EnforcerTask}" /disable') = 0;
  RC := RunHidden(ExpandConstant('{sys}\schtasks.exe'), '/end /tn "{#EnforcerTask}"');
  RC := RunHidden(ExpandConstant('{sys}\taskkill.exe'), '/f /im {#AppExe}');
  RC := RunHidden(ExpandConstant('{sys}\taskkill.exe'), '/f /im FocusLockGhost.exe');
  Sleep(500);
  Result := '';
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  RC: Integer;
  LogPath, Params: String;
begin
  if CurStep <> ssPostInstall then
    Exit;
  WizardForm.StatusLabel.Caption := 'Setting up enforcement (this is the one-time admin step)...';
  LogPath := ExpandConstant('{%TEMP}\FocusLock-setup.log');
  Params := '-NoProfile -ExecutionPolicy Bypass -File "' + ExpandConstant('{app}\scripts\install_setup.ps1') + '"' +
    ' -ExePath "' + ExpandConstant('{app}\{#AppExe}') + '"' +
    ' -AppData "' + ExpandConstant('{userappdata}') + '"' +
    ' -LogPath "' + LogPath + '"';
  RC := RunHidden(ExpandConstant('{sys}\WindowsPowerShell\v1.0\powershell.exe'), Params);
  if RC = 0 then
    EnforcementReady := True
  else
    SuppressibleMsgBox('Focus Lock was installed, but the enforcement setup failed (code ' + IntToStr(RC) + ').' + #13#10#13#10 +
      'The app will run in reduced mode. Details are in:' + #13#10 + LogPath, mbError, MB_OK, IDOK);
end;

procedure DeinitializeSetup();
var
  RC: Integer;
begin
  if EnforcerTaskDisabled and not EnforcementReady then
    RC := RunHidden(ExpandConstant('{sys}\schtasks.exe'), '/change /tn "{#EnforcerTask}" /enable');
end;
