# Focus Lock 🔒

**Focus Lock** is a high-security, "un-stoppable" application blocker designed for Windows. It is built for users who need absolute focus and want to prevent themselves from forcefully exiting their self-imposed restrictions.

> [!WARNING]
> **USE AT YOUR OWN RISK.** This application uses advanced Windows APIs to protect itself from termination. Forcefully killing the process (e.g., via Task Manager or external tools) while a session is active **WILL cause a Blue Screen of Death (BSOD)** and potential data loss.

## ✨ Key Features

*   **🛡️ Unstoppable Sessions**: Once a focus session starts, it cannot be cancelled. The application marks itself as a Critical Process (`RtlSetProcessIsCritical`). Termination attempts trigger a system crash (BSOD).
*   **👻 Ghost Process**: A background "Ghost" process (with an obfuscated, system-like name) monitors the lock state and enforces blocking, ensuring protection even if the main UI is closed.
*   **⏱️ Secure Timer**: Uses **monotonic time** to track session duration. Changing the system clock (e.g., setting the date forward) will **NOT** bypass the lock.
*   **🚫 Smart Application Blocking**:
    *   Blocks applications by **filename** (e.g., `WhatsApp.exe`) and **internal metadata** (Product Name/Description).
    *   Renaming a blocked executable (e.g., renaming `game.exe` to `notepad.exe`) will not bypass the block.
*   **⚡ UAC Bypass**: Includes a setup script to create a secure shortcut, allowing you to launch the application without checking "Run as Administrator" every time.

## 🛠️ Installation

### Prerequisites
*   **Go** (v1.21+)
*   **Node.js** (v18+)
*   **Wails CLI**: Install via `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

### Build Steps
1.  Clone the repository:
    ```bash
    git clone https://github.com/yourusername/focus-lock.git
    cd focus-lock
    ```
2.  Build the application:
    ```bash
    wails build
    ```
    The binary will be generated in `build/bin/focus-lock.exe`.

## 🚀 Usage Guide

### 1. Initial Setup (Important!)
For the protection features to work, **Focus Lock must run with Administrator privileges**.

To avoid the UAC prompt every time:
1.  Navigate to the `scripts` folder.
2.  Right-click `setup_uac_bypass.ps1` and select **Run with PowerShell**.
3.  This creates a shortcut named **"Focus Lock"** on your Desktop. Always use this shortcut to launch the app.

### 2. Blocking Applications
1.  Launch Focus Lock.
2.  **Add by Name**: Type the executable name (e.g., `discord.exe`) in the input bar and press Enter.
3.  **Select from List**: Click the **Menu (☰)** button to view a list of installed applications. Check the boxes for apps you want to block and click "Save".

### 3. Starting a Session
1.  Enter the desired duration (Hours, Minutes, Seconds) using the keypad.
2.  Press **OK**.
3.  **The session will begin immediately.** The main window will show the countdown.
    *   *Note: You can close the main window; the "Ghost" process will continue running in the background.*

## 🏗️ Technical Details

*   **Frontend**: React + TypeScript + TailwindCSS
*   **Backend**: Go (Wails framework)
*   **Security Mechanisms**:
    *   **ACL Modification**: Modifies the Discretionary Access Control List (DACL) to deny `PROCESS_TERMINATE` rights to "Everyone".
    *   **Critical Process**: Marks the process as critical to the OS kernel.
    *   **Obfuscation**: Copies the executable to a hidden user directory with a random system-sounding name (e.g., `HostServiceManager.exe`) to evade detection.

## ⚠️ Disclaimer

This software alters critical system process states. The developers are not responsible for any data loss, system instability, or frustration caused by your inability to play games or check social media. **Focus wisely.**
