# Focus Lock

Focus Lock is a Windows app and website blocker for people who want to commit to a
distraction-free session and then not be able to talk themselves out of it. You choose what
to block and for how long. Once a session starts, the block stays in force until the timer
runs out, even if you close the app, kill its process or change the system clock.

Built with Go and React on [Wails](https://wails.io/).

> **Note:** Focus Lock is deliberately hard to bypass while a session is active. It closes
> blocked applications and edits the system `hosts` file. Use it on your own machine, and
> understand what it does before starting a long session.

## Why

Most blockers are easy to turn off at the moment of temptation, which is exactly when they
are supposed to work. In Focus Lock you decide to focus once, up front, and can't casually
undo it while the session runs. You can still add to a block mid-session, but you can't
weaken it.

## Features

- **Timed sessions**: a preset, an exact duration, or "focus until" a date and time.
- **Schedules**: recurring windows (including overnight ones) that block automatically.
- **App blocking**: blocked programs are closed if they start during a session.
- **Website blocking**: system-wide through the `hosts` file, and through Chrome policy
  so Secure DNS and cached pages don't get around it.
- **Presets and groups**: one-click category lists, your own named groups and recently
  used sites.
- **Hard to bypass**: a background enforcer survives closing the window and restarts,
  session time is checked against network time, and the settings are signed so editing
  them doesn't help.
- **VPN blocking** (optional): closes common VPN apps and blocks their sites, so a VPN
  can't route around the block.
- **Emergency unlocks**: two one-minute pauses per session.
- **Import**: load block lists, groups and schedules from a settings file.
- **Insights**: streaks, total focus time, most-blocked apps and session history.
- **Automatic updates**: weekly releases, installed from inside the app between sessions.

## Install

Download **`FocusLock-Setup.exe`** from
[Releases → latest](https://github.com/sagnik31/focus-lock/releases/latest), run it and
approve the one UAC prompt. After that the app updates itself.

Updates, uninstalling and the standard-user caveat are covered in
[docs/install.md](docs/install.md).

## Usage

1. **Apps** tab: add executables (for example `discord.exe`) or pick from installed apps.
2. **Websites** tab: add domains (for example `facebook.com`) or turn on a category.
3. Set a duration and click **Start focus**.
4. For recurring blocks, switch the timer panel to **Scheduled** and create a schedule.

## Build from source

You need Go 1.24+, Node.js 22.13+ and the
[Wails CLI](https://wails.io/docs/gettingstarted/installation).

```bash
git clone https://github.com/sagnik31/focus-lock.git
cd focus-lock
wails build        # build/bin/focus-lock.exe
```

Building the installer and running enforcement setup by hand are in
[docs/install.md](docs/install.md#build-from-source).

## Documentation

- [Architecture](docs/architecture.md): processes, the enforcement loop, config integrity,
  the operation policy, trusted time and a code map.
- [Architecture decision records](docs/adr/README.md): why it's built this way and what
  else was considered.
- [Install, update and build](docs/install.md)
- [Installer test checklist](installer/TESTING.md)
- [CLAUDE.md](CLAUDE.md): conventions and checks for contributors and coding agents.

## Disclaimer

Focus Lock terminates other processes and modifies system files on the machine it runs on.
It is provided as-is, without warranty. The author is not responsible for data loss, lost
work, or being unable to reach a blocked application or site during an active session. Set
your sessions deliberately.
