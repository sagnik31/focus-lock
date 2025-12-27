# Distribution Guide

## 1. Prepare the Release Build
To distribute the app, you need to build a condensed, production-ready binary.

1.  Open your terminal in the project root.
2.  Run the build command:
    ```bash
    wails build -platform windows/amd64 -clean
    ```
    *   `-clean`: Removes previous build artifacts.
    *   `windows/amd64`: Targets standard 64-bit Windows.

3.  The output will be in `build/bin/focus-lock.exe`.

## 2. Handling Runtime Dependencies
Since your users don't have Go or Node, you must ensure the app is standalone.

*   **Go/Node**: These are compiled into the binary. No action needed.
*   **WebView2**: Wails apps use the Edge WebView2 runtime.
    *   **Windows 11**: Pre-installed.
    *   **Windows 10**: Usually pre-installed, but might be missing on older versions.
    *   **Solution**: The Wails-generated installer (if you use `wails build -nsis`) checks for this.

## 3. Bundling External Assets
The app requires the **UAC Bypass Script** to function conveniently. Since this script is **not embedded** in the binary, you must distribute it.

### Option A: Simple Zip File (Easiest)
Create a folder named `FocusLock_v1.0` and add:
1.  `focus-lock.exe` (from `build/bin`)
2.  `setup_uac_bypass.ps1` (from `scripts/`)
3.  **README.txt**: A text file explaining "Run setup_uac_bypass.ps1 as Admin first".

Zip this folder and send it to your testers.

### Option B: Professional Installer (Recommended for End Users)
We have configured the NSIS installer to **automatically** set up the UAC bypass. The user just needs to run the installer.

1.  **Generate Installer**:
    Run:
    ```bash
    wails build -nsis
    ```
2.  **Output**:
    The installer will be in `build/bin/focus-lock-amd64-installer.exe`.

3.  **What it does**:
    *   Installs the app to `Program Files`.
    *   Silently runs a script to create the "FocusLockLauncher" scheduled task.
    *   Creates Desktop and Start Menu shortcuts that use `schtasks` to launch the app **without UAC prompts**.

4.  **Distribution**:
    Just send the `.exe` installer file to your users. They simply install and run!

## 4. Code Signing (Crucial for "Warning Free" Experience)
If you send this to others, Windows SmartScreen will block it saying "Unknown Publisher".
*   **For Testing**: Tell users to click **"More Info" -> "Run Anyway"**.
*   **For Release**: You must buy a code signing certificate (EV Cert) and sign the `.exe`. This is expensive (~$300/year). for a hobby project, just warn your users.

## Checklist for Testers
When distributing to testers, give them these instructions:
1.  Extract the Zip / Run the Installer.
2.  **Right-click -> Run as Administrator** (First time is mandatory).
3.  If using the Zip method, run the `setup_uac_bypass.ps1` script as Admin.
4.  Accept any "Unknown Publisher" warnings (SmartScreen).
