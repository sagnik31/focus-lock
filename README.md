# Focus Lock

**Focus Lock** is a high-security application and website blocker designed for Windows. It is engineered for users who require absolute focus and measures to prevent themselves from bypassing self-imposed restrictions.

> [!WARNING]
> **USE AT YOUR OWN RISK.** This application leverages advanced Windows APIs to protect itself from termination. Forcefully killing the process (e.g., via Task Manager or external tools) while a session is active **WILL cause a Blue Screen of Death (BSOD)** and potential data loss.

## Key Features

*   **Unstoppable Sessions**: Once a focus session is initiated, it cannot be cancelled. The application marks itself as a Critical Process (`RtlSetProcessIsCritical`). Any attempt to terminate it triggers a system crash (BSOD).
*   **Website Blocking**: Blocks access to distracting websites system-wide by modifying the Windows Hosts file. Supports smart subdomain blocking (e.g., `www.`, `m.`, `mobile.`) and works across all browsers including Incognito/Private modes.
*   **Category Presets**: Includes one-click blocking for common distraction categories: Social Media, Entertainment, Gaming, and Adult content.
*   **VPN & Evasion Protection**: Automatically detects and blocks common VPN applications and their associated domains to prevent users from bypassing restrictions.
*   **Secure Network Timer**: Utilizes **NTP (Network Time Protocol)** to synchronize with reliable time servers. Manipulating the local system clock does **not** bypass the lock.
*   **Ghost Process**: A background "Ghost" process with an obfuscated, system-like name monitors the lock state and enforces restrictions, ensuring protection even if the main UI is closed.
*   **High-Performance Enforcement**: The enforcement engine uses an O(1) fast-path algorithm to inspect processes every 200ms, ensuring near-instant termination of blocked applications without impacting system performance.
*   **Smart App Discovery**: Automatically catalogues installed programs, filtering out system components to provide a clean, relevant list of applications for blocking.

## Installation

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

## Usage Guide

### 1. Initial Setup
For the protection features to function correctly, **Focus Lock must run with Administrator privileges**.

To facilitate this without frequent UAC prompts:
1.  Navigate to the `scripts` folder.
2.  Right-click `setup_uac_bypass.ps1` and select **Run with PowerShell**.
3.  This creates a shortcut named **"Focus Lock"** on your Desktop. Always use this shortcut to launch the application.

### 2. Blocking Applications
1.  Navigate to the **Apps** tab.
2.  **Add by Name**: Enter the executable name (e.g., `discord.exe`) in the input field and press Enter.
3.  **Select from List**: Click the **Select All** button or choose individual applications from the list.

### 3. Blocking Websites
1.  Navigate to the **Websites** tab.
2.  **Add URL**: Enter the domain (e.g., `facebook.com`) and press Enter.
3.  **Category Toggles**: Use the toggle switches to block entire categories of websites instantly.

### 4. Starting a Session
1.  Select the desired duration using the time slider or the custom keypad.
2.  Click **Start Focus**.
3.  Review the **Confirm Lock** modal, which lists the duration and all blocked items.
4.  Confirm to begin the session. The enforcement mechanism activates immediately.

## Technical Architecture

*   **Frontend**: React + TypeScript + TailwindCSS
*   **Backend**: Go (Wails framework)
*   **Enforcement Mechanisms**:
    *   **Process Termination**: Uses `CreateToolhelp32Snapshot` and `TerminateProcess` with a dual-loop architecture (Fast Check vs. Deep Metadata Inspection).
    *   **Network Blocking**: Modifies `C:\Windows\System32\drivers\etc\hosts` to redirect blocked domains to `0.0.0.0`.
    *   **Critical Status**: Sets the process status to Critical, forcing the OS kernel to panic if the process is killed unexpectedly.

## Disclaimer

This software alters critical system process states. The developers are not responsible for any data loss, system instability, or inconvenience caused by the inability to access blocked applications or websites during an active session. **Focus wisely.**
