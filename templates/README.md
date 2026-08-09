# Scheduling Templates

This directory contains templates for running `git-manager sync` on a schedule.

## systemd (Linux)

Per-user scheduling using systemd timers.

### Manual Setup

1. Determine your `git-manager` binary path. If built locally:
   ```bash
   go build -o $GOPATH/bin/git-manager ./cmd/git-manager
   binary_path=$GOPATH/bin/git-manager
   ```

2. Determine your config file path:
   ```bash
   config_path=$HOME/.config/git-manager/config.toml
   ```
   (Adjust to wherever you keep your `git-manager` config.)

3. Edit `templates/systemd/git-manager.service` and replace:
   - `__GIT_MANAGER_BINARY__` with the full path to your binary
   - `__CONFIG_PATH__` with the full path to your config file

4. Edit `templates/systemd/git-manager.timer` if you want a different schedule:
   - Change `OnCalendar=*-*-* 02:00:00` to your preferred time
   - See `man 7 systemd.time` for format details

5. Copy the edited files to systemd user directory:
   ```bash
   mkdir -p ~/.config/systemd/user/
   cp templates/systemd/git-manager.service ~/.config/systemd/user/
   cp templates/systemd/git-manager.timer ~/.config/systemd/user/
   ```

6. Enable and start the timer:
   ```bash
   systemctl --user daemon-reload
   systemctl --user enable git-manager.timer
   systemctl --user start git-manager.timer
   ```

7. Verify it's running:
   ```bash
   systemctl --user status git-manager.timer
   systemctl --user list-timers --all
   ```

### Logs

View recent runs:
```bash
journalctl --user -u git-manager.service -n 20 -f
```

## launchd (macOS)

Per-user scheduling using LaunchAgent.

### Manual Setup

1. Determine your `git-manager` binary path. If built locally:
   ```bash
   go build -o $HOME/go/bin/git-manager ./cmd/git-manager
   binary_path=$HOME/go/bin/git-manager
   ```

2. Determine your config file path:
   ```bash
   config_path=$HOME/Library/Application\ Support/git-manager/config.toml
   ```
   (Adjust to wherever you keep your `git-manager` config.)

3. Choose a log directory:
   ```bash
   log_path=$HOME/Library/Logs/git-manager
   mkdir -p "$log_path"
   ```

4. Edit `templates/launchd/com.example.git-manager.plist` and replace:
   - `__GIT_MANAGER_BINARY__` with the full path to your binary
   - `__CONFIG_PATH__` with the full path to your config file
   - `__LOG_PATH__` with the full path to your log directory
   - `__USERNAME__` with your actual username (e.g., `octocat`)
   - `com.example.git-manager` → use a reverse-domain label if desired, but keep the `.plist` filename matching the Label

5. To change the schedule, edit `StartCalendarInterval`:
   ```xml
   <key>StartCalendarInterval</key>
   <dict>
       <key>Hour</key>
       <integer>2</integer>
       <key>Minute</key>
       <integer>0</integer>
   </dict>
   ```
   This schedules the sync for 2:00 AM daily. See `man launchd.plist` for other options.

6. Copy the edited plist to the LaunchAgent directory:
   ```bash
   cp templates/launchd/com.example.git-manager.plist \
      ~/Library/LaunchAgents/com.example.git-manager.plist
   ```

7. Load the agent:
   ```bash
   launchctl load ~/Library/LaunchAgents/com.example.git-manager.plist
   ```

8. Verify it's loaded:
   ```bash
   launchctl list | grep git-manager
   ```

### Logs

View recent runs:
```bash
tail -f ~/Library/Logs/git-manager/git-manager.log
tail -f ~/Library/Logs/git-manager/git-manager-error.log
```

To manually trigger a sync run (for testing):
```bash
launchctl start com.example.git-manager
```

## Notes

- **No root required.** Both systemd and launchd configurations are per-user only — no system-wide installation.
- **Credentials.** These templates rely on your existing git/SSH/GPG setup (managed by the user's own credential helpers, SSH agent, etc.) — the templates themselves never store or require any credentials.
- **Config path.** Update the templates with your actual config file path before copying to the system directories.
- **Binary path.** If you install `git-manager` via a package manager or to a standard location (e.g., `/usr/local/bin`), use that full path in the template.

## Issue #14 (Future)

The `git-manager install` and `uninstall` subcommands (issue #14, not yet implemented) will automate these manual steps, substituting placeholders and copying files to the right locations.
