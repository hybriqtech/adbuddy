---
title: Troubleshooting
layout: default
lang: en
---

# Troubleshooting

Find your symptom or your error code, and follow the fix. If you do not know where to start, run `adbuddy doctor` first.

- [First, run `adbuddy doctor`](#first-run-adbuddy-doctor)
- [Error codes](#error-codes)
- [Problems by symptom](#problems-by-symptom)
- [Collect information for an issue](#collect-information-for-an-issue)

Pairing problems ("Wrong password", `protocol fault`, `PAIRING_TIMEOUT`, the QR code that never works) have their own list in [Set up your phone](setup/#troubleshooting). It is not repeated here.

## First, run `adbuddy doctor`

```sh
adbuddy doctor
```

Doctor checks, in this order, what ADBuddy needs. A check that depends on one that failed is skipped.

1. **config**: `config.toml` reads, and a phone is set up.
2. **adb**: ADBuddy's own adb is installed and is version 37 or newer.
3. **adb server**: ADBuddy's private adb server (port 5038 unless you changed it) answers.
4. **serve**: whether `adbuddy serve` is running.
5. **phone**: the phone you set up is connected to that server.
6. **identity**: that phone is the one you set up, not another phone on the network. When ADBuddy cannot tell (it keeps the phone under a name with no serial number), this is a `⚠` that says so.
7. **network port**: adb's network port on the phone (the `tcpip` mode) is closed, or says why it is open.
8. **phone app**: the ADBuddy app is installed and set up (the permission for Cut access, the accessibility service, the battery exemption) and, with serve running, its channel to the host is open.
9. **environment**: the agent's folder has `AGENTS.md` and the `adb` shim, and the journal can be written.
10. **claude hooks**: warns when your Claude Code settings already have `PermissionRequest` or `PreToolUse` hooks, which can answer the same request as ADBuddy.

Doctor only reads. Nothing on the phone changes. Starting ADBuddy's private adb server, as any adb call does, is the only side effect. Every call is bounded, and the whole run answers within 60 seconds.

### How to read it

Each check is one line, with a symbol:

- `✓` ok.
- `⚠` warning: ADBuddy works, with something missing or optional.
- `✗` failed: ADBuddy cannot work until it is fixed.
- `·` skipped: an earlier check failed, so this one was not run.

A problem comes with one of two lines under it:

- `Run:` is a safe command that you, or the agent, can run.
- `Guidance:` is something only you can do, on the phone or in another terminal.

In the agent's session, the commands only you can run (`adbuddy setup`, `adbuddy serve`, `adbuddy connect <ip:port>` and `adbuddy doctor --fix`) show as `Ask the user to run …` instead.

The last line is the verdict: `ready`, `degraded` (only `⚠` lines) or `broken` (at least one `✗`). When it is broken, fix the `✗` lines from the top: the later ones often follow from the first.

This is real output from a PC where ADBuddy was never set up (paths replaced; the last check, `claude hooks`, left out). On your PC the lines will differ with what you have set up:

```text
✗ config: no phone set up yet (<config folder>/config.toml)
    Run: adbuddy setup
✗ adb: ADBuddy's adb is not installed
    Run: adbuddy setup
· adb server: not checked: ADBuddy's adb does not work
⚠ serve: not running: the agent cannot use the phone
    Run: adbuddy serve
    Guidance: Keep it open in its own terminal while the agent works.
· phone: not checked: adb server down
· identity: not checked: adb server down
· network port: not checked: adb server down
· phone app: not checked: adb server down
✗ environment: <home>/ADBuddy/default: missing AGENTS.md, .adbuddy/bin/adb
    Run: adbuddy env init --dir <home>/ADBuddy/default
Verdict: broken — fix the ✗ lines first, from the top.
```

The lines for a connected phone are not shown here: they are described above and in the symptoms below.

### Exit code

| Code | Meaning |
|---|---|
| 0 | `ready` or `degraded` |
| 1 | `broken` |
| 130 | stopped with Ctrl+C |

### `--json`

```sh
adbuddy doctor --json
```

It prints one JSON object on stdout. The report is in `data`: `verdict`, the `checks` (each with `name`, `status` as `ok`, `warn`, `fail` or `skip`, `detail` and, when there is one, `run` and `guidance`) and, with `--fix`, `fixed`. When the verdict is `broken`, `ok` is `false`, `error.code` is `DOCTOR_BROKEN`, the same report is still in `data` and the exit code is 1.

### `--fix`

```sh
adbuddy doctor --fix
```

`--fix` does one thing: it restarts ADBuddy's private adb server, when the `adb server` check fails. It never does it while `adbuddy serve` is running, and it does not do it either when the ADBuddy socket does not answer, because a serve may be using the server. In both cases it says so on the `adb server` line. It then runs the checks again and lists what it fixed with a `↻` line. It changes nothing else, and it is refused when it runs from the agent's session (`AGENT_SESSION`, a check of the environment, not a barrier).

## Error codes

When a command fails, it prints `Error: <CODE> — <message>` and, when there is one, `Fix: <fix>` on stderr. With `--json` the same comes as `{"ok":false,"error":{"code","message","fix","transient"}}`. The exit code says the kind of failure:

| Exit | Meaning |
|---|---|
| 1 | failure: see the code |
| 2 | invalid usage |
| 75 | temporary: trying again may work |
| 78 | configuration |
| 126 | the command was denied or blocked by ADBuddy (the agent's `adb`, the screen helpers) |
| 130 | stopped with Ctrl+C |

<!-- generated by hand from grep 'NewError(' and the other searches the M6d plan lists (ConfigError, TransientError, `Code:`, remoteExit, sendFail, fail) over host/internal; refresh when the CLI changes -->

"Fix" is what the host itself suggests. Where the host suggests nothing, the last column says what usually helps, and the code itself does not promise it.

| Code | Exit | When | What to do |
|---|---|---|---|
| `ADB_FAILED` | 1 | ADBuddy's adb server did not start or answer (`adbuddy setup`, `adbuddy serve`). | Check the adb path, or stop other adb servers and try again. See [The port 5038 is taken](#the-port-5038-is-taken). |
| `ADB_NOT_CONFIGURED` | 78 | ADBuddy's adb is not installed (`serve`, `connect`, `remote`). | Run `adbuddy setup`, or pass `--adb <path to adb 37 or newer>` to `serve` or `connect`. |
| `AGENT_FAILED` | 1 | `adbuddy run` could not run the agent. | The message says why. Run the agent by hand once to see its own error. |
| `AGENT_NOT_FOUND` | 1 | The agent is not installed or not in your `PATH` (`adbuddy run`, `adbuddy new`). | Install it: the message carries the command. See [The agent is not installed](#the-agent-is-not-installed). |
| `AGENT_SESSION` | 1 | You ran a command that only a person may run (`attach`, `connect`, `doctor --fix`, `new`, `remote`, `serve`, `setup`, `stop`) from the agent's session. | Run it yourself in a terminal. |
| `ALREADY_RUNNING` | 1 | `adbuddy setup` or `adbuddy connect <ip:port>` found a serve running, or `adbuddy serve` found another one. | Stop `adbuddy serve` (Ctrl+C in its terminal) and run the command again; or use the terminal where serve already runs. |
| `ALREADY_UNDONE` | 1 | `adbuddy undo <id>` on an entry already undone. | Nothing to do. |
| `ATTACH_FAILED` | 1 | `adbuddy attach` lost serve, or serve answered with an error. | Check that `adbuddy serve` is still running. |
| `CONFIG_INVALID` | 78 | `config.toml` is not valid. | Fix or delete it: the message names the file. |
| `CONFIG_UNWRITABLE` | 78 (1 from `remote off`) | `config.toml` cannot be saved (`setup`, `connect`). From `adbuddy remote off`: port 5555 is closed but `remote_until` could not be cleared. | Check that the configuration folder is writable. From `remote off`: delete the `remote_until` line in `config.toml`. |
| `CONNECT_FAILED` | 75 | `adbuddy connect <ip:port>` could not connect. | Check the IP address and port on the phone's Wireless debugging screen (the port changes every time wireless debugging is turned on) and that the phone and the PC can reach each other. |
| `DAEMON_ERROR` | varies | Serve answered with an error that has no code of its own. The one seen is `unauthorized`: the secret the command read is not the one serve started with. | The host suggests nothing. Stop `adbuddy serve` and start it again. |
| `DAEMON_NOT_RUNNING` | 1 | A command that needs serve found none. `adbuddy journal --session current` also says it. (The agent's `adb` says something else: see [The agent's `adb` says the host is not running](#the-agents-adb-says-the-host-is-not-running).) | Start `adbuddy serve` in another terminal. For the journal, pass `--session <id>` (ids start with `s_`). |
| `DAEMON_UNREACHABLE` | 1 | The ADBuddy socket exists but does not answer. | Stop the other `adbuddy serve`, or delete the socket file the message names if none is running. From `adbuddy run`: restart serve. |
| `DEVICE_NOT_FOUND` | 75 | No phone in state `device` on ADBuddy's adb server; or none of its saved addresses worked; or the phone did not come back after `adb usb`. | See [The PC cannot find the phone](#the-pc-cannot-find-the-phone). |
| `DEVICE_PORT_OPEN` | 1 | adb's network port on the phone is still open after `adb usb` (`adbuddy serve` and `adbuddy setup`). | Restart the phone (that closes a port opened with `adb tcpip`), turn Wireless debugging on again and run the command again. |
| `DEVICE_UNREADABLE` | 1 (75 when the phone did not answer in time) | The phone did not answer a read. | Unlock the phone and check that it is still connected. |
| `DIRS_UNKNOWN` | 78 | ADBuddy cannot work out its configuration and data folders. | Set `ADBUDDY_CONFIG_DIR` and `ADBUDDY_DATA_DIR`. |
| `DOCTOR_BROKEN` | 1 | `adbuddy doctor --json` found a failed check. | Follow the `run` and `guidance` of the failed checks, from the top. |
| `EXECUTABLE_UNKNOWN` | 1 | The system could not tell where the `adbuddy` program is. | The host suggests nothing. Run it by its full path. |
| `INTERNAL` | 1 | A failure the host did not expect. | The host suggests nothing. [Open an issue](#collect-information-for-an-issue) with the message. |
| `INTERRUPTED` | 130 (1 for a remote-mode request) | You pressed Ctrl+C. From `adbuddy remote`, serve can also say that the request was abandoned and the change may still run. | Nothing to do; for `remote`, check `adbuddy remote status`. |
| `JOURNAL_UNREADABLE` | 1 | The journal could not be read (`adbuddy journal`, `adbuddy undo`). | The host suggests nothing. Check that the `journal` folder of the environment exists and is readable. |
| `JOURNAL_UNWRITABLE` | 1 | `adbuddy undo` could not record what it did. | Check that the journal folder is writable. The changes listed as undone did happen. |
| `MULTIPLE_DEVICES` | 1 | More than one phone is connected to ADBuddy's adb server. | Pass `--serial <serial>` to `serve`, or set `serial` in `config.toml`. |
| `NOT_APPLIED` | 1 | `adbuddy undo <id>` on a command that never ran. | Nothing to do. |
| `NOT_CONFIGURED` | 78 | `adbuddy` was run without a terminal or with `--json` before any setup. | Run `adbuddy setup`. |
| `NOT_FOUND` | 1 | `adbuddy undo <id>` found no such entry; or `adbuddy send <file>` found no such file. | Check the id with `adbuddy journal`; or check the file path. |
| `NOT_A_FILE` | 1 | `adbuddy send <file>` was given something that is not a regular file. | Send a file. |
| `NO_INVERSE` | 1 | `adbuddy undo <id>` on an entry with no inverse (or that is itself an undo). | This change has to be reverted by hand. |
| `NO_SESSION` | 1 | `adbuddy attach` and the agent is not running. | Start it with `adbuddy new`. |
| `NOTHING_TO_UNDO` | 1 | No change with an inverse is left. | See `adbuddy journal` for what was done. |
| `OTHER_DEVICE` | 1 | The entry was made on another phone. | Connect that phone and undo it there, or revert it by hand. |
| `OUTSIDE_WORKSPACE` | 1 | `adbuddy send <file>` was given a file outside the agent's environment. | Put the file inside the environment first. |
| `PAIRING_FAILED` | 1 | Pairing with the phone failed. | See [Set up your phone](setup/#troubleshooting): keep the pairing popup open until the terminal says "Paired". Or try again, or use `adbuddy setup --code`. |
| `PAIRING_TIMEOUT` | 75 | The phone did not scan the QR code in time. | Check that the phone and the PC are on the same Wi-Fi and run `adbuddy setup` again, or use `adbuddy setup --code`. |
| `PAUSED` | 75 | `adbuddy undo` while the agent is paused. | Resume from the ADBuddy app, then run `adbuddy undo` again for the rest. |
| `REMOTE_BUSY` | 1 | Another request to turn remote mode on is waiting, or too many are. | Try again in a moment. |
| `REMOTE_DENIED` | 126 | The question to turn remote mode on was not allowed (denied, expired, nobody to ask, or the agent was paused). | Run `adbuddy remote on` again and allow the question, in the ADBuddy app or the serve terminal. |
| `REMOTE_FAILED` | 1 | Opening or closing port 5555 failed. | Check that the phone is connected and run it again; `adbuddy remote status` says what the host believes. |
| `REMOTE_OFF_FAILED` | 1 | `adbuddy remote off` could not close the port. Remote mode stays on in `config.toml`. | Connect the phone (Wireless debugging on, same network or Tailscale) and run `adbuddy remote off` again. Restarting the phone also closes the port. |
| `REMOTE_UNAVAILABLE` | 1 | Remote mode cannot be used now: no phone connected, or no Tailscale on the phone. | Open the ADBuddy app on the phone with Tailscale on, then run it again. |
| `RULES_INVALID` | 78 | Your `rules.yaml` has a rule the host cannot use. | Fix the rule or remove it. |
| `RULES_UNREADABLE` | 78 | Your `rules.yaml` cannot be read. | Check the file the message names. |
| `SECRET_FAILED` | 1 | `adbuddy serve` could not create or read its secret in the data folder. | Check that the data folder is writable. |
| `SEND_FAILED` | 1 | `adbuddy send <file>` could not send the file. | The host suggests nothing. The message says which step failed. |
| `SESSION_FAILED` | 1 | `adbuddy new` or `adbuddy stop` could not reach serve. | The host suggests nothing. Check that `adbuddy serve` is running. |
| `SOCKET_FAILED` | 1 | `adbuddy serve` could not open or keep its local socket. | The host suggests nothing. Stop any other `adbuddy serve` and start it again. |
| `TERMINAL_FAILED` | 1 | `adbuddy attach` could not set up your terminal. | The host suggests nothing. Try another terminal. |
| `TOO_LARGE` | 1 | `adbuddy send <file>`: the file is over 200 MiB. (A caption over 64 KiB is refused earlier, as `USAGE`.) | Send a smaller file. |
| `TOOLS_CHECKSUM_MISMATCH` | 1 | The platform-tools `adbuddy setup` downloaded do not match Google's checksum. | Run `adbuddy setup` again later. If it keeps failing, pass `--adb <path to adb 37 or newer>` and report it at the project's issues. |
| `TOOLS_DOWNLOAD_FAILED` | 75 | `adbuddy setup` could not download platform-tools. | Check the internet connection and run `adbuddy setup` again, or pass `--adb <path to adb 37 or newer>`. |
| `UI_AMBIGUOUS` | 1 | `adbuddy tap` matched more than one element. | Use `--id <id>` or `--xy <x,y>` from `adbuddy screen`. |
| `UI_BLOCKED` | 126 | A screen helper was refused: `Blocked by ADBuddy: …` with the reason (paused, a protected app in front, waiting for your answer, the journal cannot be written), or `stopped` when you or the agent ended it midway. | See [Blocked by ADBuddy while a question waits](#blocked-by-adbuddy-while-a-question-waits). |
| `UI_FAILED` | 1 | A screen helper failed. The message says why, for example that the app answered something unreadable, or that the screenshot arrived incomplete. | For the screenshot, try again. Otherwise the host suggests nothing. |
| `UI_NOT_FOUND` | 1 | No element on the screen matches. | Run `adbuddy screen` to see what is on the screen. |
| `UI_PROTECTED` | 126 | The element belongs to ADBuddy or a protected app, or the app refused the action (for example typing into a password field). | Do this part on the phone yourself. |
| `UI_TIMEOUT` | 75 | The ADBuddy app did not answer in time. The action may or may not have happened. | Run `adbuddy screen` to see whether it happened before trying again. |
| `UI_UNAVAILABLE` | 75 | Typing Unicode (or a text with `%s`) without the app; or ADBuddy's accessibility service is on but the app is not connected; or the phone's accessibility state could not be read; or, for `adbuddy notifications`, notification access is not allowed or not confirmed. | Open the ADBuddy app on the phone. For notifications: Settings, Notification access, allow it and tap the confirm button. See [The accessibility service will not turn on](#the-accessibility-service-will-not-turn-on). |
| `UNCERTAIN_STATE` | 1 | `adbuddy undo` met a change that failed, was running when the host stopped, or whose place on the phone is taken again. | Check the phone: its inverse is not run automatically. |
| `UNDO_DENIED` | 1 | `adbuddy undo` needed to run a 🔴 inverse (such as `uninstall` or `rm -f`) and its question was denied, expired or had nobody to answer it; nothing was undone. | Run `adbuddy undo` again and allow the 🔴 question (in the ADBuddy app or the serve terminal), or revert the change by hand. |
| `UNDO_FAILED` | 1 | Some inverses failed, or undo was stopped midway. | Check the phone and the journal. Failed inverses can be retried with `adbuddy undo <id>`. |
| `UNKNOWN_AGENT` | 1 | `adbuddy new <agent>` with a name ADBuddy does not know. | Use `claude`, `codex`, `gemini` or `opencode`. |
| `UNKNOWN_KIND`, `UNKNOWN_REQUEST` | 1 | Serve does not know the request. This is most likely a different version of `adbuddy` running serve. | The host suggests nothing. Stop serve and start it again with the same build. |
| `UNTRUSTED_ENTRY` | 1 | The journal entry was not written by ADBuddy. | Check the phone and undo it by hand if needed. |
| `USAGE` | 2 | A wrong command, flag or argument. | Run `adbuddy --help`, or `--help` of that command. |
| `WAITING_FOR_ANSWER` | 75 | `adbuddy undo` while a 🔴 question is open. | Answer it, in the ADBuddy app or the serve terminal, and run `adbuddy undo` again. |
| `WORKSPACE_FAILED` | 1 | The agent's environment folder could not be made or refreshed. | See [The environment folder cannot be made](#the-environment-folder-cannot-be-made). |
| `WORKSPACE_MISMATCH` | 1 | `adbuddy run --workspace <folder>` names another folder than the one serve watches. | Run `adbuddy run` without `--workspace`, or stop serve and start it with `adbuddy serve --workspace <folder>`. |
| `WRITE_FAILED` | 1 | `adbuddy screenshot` could not write the file. | The host suggests nothing. Check that the folder is writable, or pick another with `-o`. |
| `WRONG_PHONE` | 1 | The phone at that address is not the one you set up. | Check the IP address and port on the phone you set up. For a phone found on the network: turn off Wireless debugging on your other devices and try again. |

## Problems by symptom

### The PC cannot find the phone

`adbuddy serve` waits up to 45 seconds and then stops with `DEVICE_NOT_FOUND` (exit 75). `adbuddy setup` and `adbuddy connect <ip:port>` can end with the same code. In the doctor, it is the `phone` line.

1. Unlock the phone and check that **Wireless debugging** is on. Android can turn it off when the Wi-Fi changes, unless you tick **Always allow on this network** (see [Set up your phone](setup/)).
2. Check that the phone and the PC are on the same Wi-Fi, and that the network does not isolate its devices (guest networks often do).
3. If the phone was never paired with this PC, run `adbuddy setup`.
4. If the phone is on another network, for example over Tailscale, or the Wi-Fi blocks mDNS, connect by address with the IP address and port on the Wireless debugging screen: `adbuddy connect <ip:port>`. Stop `adbuddy serve` first.
5. Run `adbuddy doctor` and follow the first `✗` line.

On Windows, see also [Windows and the firewall](#windows-and-the-firewall).

### The Wireless debugging port changed

Every time Wireless debugging is turned on again, the phone picks a new port. If the saved address stops working, `adbuddy serve` says `the phone did not show up, and none of its saved addresses worked` and suggests the fix:

1. Stop `adbuddy serve` (Ctrl+C). `adbuddy connect <ip:port>` does not run while serve runs.
2. On the phone, open **Wireless debugging** and read the IP address and port on its main screen.
3. Run `adbuddy connect <ip:port>` with them. It saves the address.
4. Run `adbuddy serve` again.

While serve runs, it also watches the phone. If it disappears, serve may try the saved address, the local network and the phone's Tailscale address, and it only switches to a phone it can verify is the one you set up ([Use ADBuddy](usage.md#remote-mode)).

### "Wrong password" and `protocol fault` when pairing

These are pairing problems. The steps are in [Set up your phone](setup/#troubleshooting): use the current pairing code, and keep the popup open until the terminal says "Paired".

### The phone is locked

ADBuddy cannot read some things from a locked phone. `adbuddy doctor` then shows `⚠` lines such as `could not read the phone's serial number` with the guidance `Unlock the phone and run adbuddy doctor again.` Commands end with `DEVICE_UNREADABLE`. Unlock the phone and try again.

Two more things need the phone unlocked: allowing a 🔴 question from the phone (Deny always works), and the Reply and Send screenshot buttons of the notification.

### `device offline`

`offline` is a state adb gives to a phone it sees but cannot talk to. The words `device offline` come from adb, not from ADBuddy. ADBuddy only uses a phone in state `device`, so you see it as `DEVICE_NOT_FOUND`, or as the `phone` line of `adbuddy doctor` saying the phone is not connected.

1. Unlock the phone.
2. Turn Wireless debugging off and on again.
3. Read the new IP address and port, and run `adbuddy connect <ip:port>` (stop `adbuddy serve` first).
4. Run `adbuddy doctor`.

If it keeps happening, restart the phone.

### `Denied by user` and `Denied: nobody could confirm`

When the agent runs something 🔴, ADBuddy holds the command until you answer. These two messages end it with exit code 126:

- `Denied by user: <title>`: you denied it, or the question expired after 2 minutes (the message then ends with ` (confirmation expired)`). Ask the agent to try again if you want it.
- `Denied: nobody could confirm: <title>`: there was nobody to ask. It happens when the ADBuddy app is not connected and `adbuddy serve` cannot ask in its terminal: you started it with `--confirm deny`, or its input is not a terminal (the default, `auto`, then denies). Connect the app (see [The app does not connect](#the-app-does-not-connect)), or run `adbuddy serve` in an interactive terminal without `--confirm deny`.

The agent is told not to retry a denied command. See [Confirmations](safety.md#confirmations).

### Blocked by ADBuddy while a question waits

The message starts with `Blocked by ADBuddy: waiting for…`. Only one 🔴 question is open at a time. While it waits, ADBuddy refuses the agent's other commands with:

```text
Blocked by ADBuddy: waiting for your answer to "<title>"
```

Before the question is shown, it says `waiting to ask about "<title>"`. Answer the question, on the phone or in the `adbuddy serve` terminal. If nobody answers, it expires after 2 minutes (`confirm_timeout` in `config.toml`) and counts as a no.

Other `Blocked by ADBuddy:` reasons exist, and the reason after the colon tells which:

| Reason | What to do |
|---|---|
| `paused from the phone: nothing runs until you resume it there` | Choose **Resume** in the ADBuddy app. See [Pause and cut access](usage.md#pause-and-cut-access). |
| `… is a protected app; ask the user to do this part` | An app ADBuddy protects is in front. Do that part yourself, or close the app. See [Protected apps and the screen](safety.md#protected-apps-and-the-screen). |
| `the journal cannot be written (…); without a journal no changes are allowed` | Make the `journal` folder of the environment writable and free some disk space. Doctor shows it on the `environment` line. |
| `only the active phone can be used (…)` | The command named another phone (with `-s`, or through `ANDROID_SERIAL`). Run it for the active phone. |
| `the adb option … is not allowed` | ADBuddy only talks to the active phone through its own adb server. Run the command without that option. |

### The agent's `adb` says the host is not running

When the agent runs `adb` and `adbuddy serve` is not running, the shim prints:

```text
ADBuddy host not running (run: adbuddy serve)
```

Nothing reaches the phone. Start `adbuddy serve` in its own terminal and keep it open, then ask the agent to try again. If serve is running and you still see it, the shim and serve use different data folders: check that `ADBUDDY_DATA_DIR` is the same in both, and run `adbuddy doctor`. In the doctor, the `serve` line says whether it is running.

### The app does not connect

The app is optional: without it, 🔴 questions appear in the `adbuddy serve` terminal and there is no chat on the phone. `adbuddy serve` says what happens with the app when it starts and afterwards. The line it prints tells what to do:

| What `adbuddy serve` prints | What to do |
|---|---|
| `The ADBuddy app is not on the phone` | Run `adbuddy setup`. Until there is a release, give it your own build: `adbuddy setup --apk <file>`. See [Build from source](install.md#build-from-source). |
| `The ADBuddy app on the phone (…) is too old for this host` | Install a newer build with `adbuddy setup --apk <file>`: plain `adbuddy setup` does not replace an app that is already installed. |
| `An app with the token connected, but it is not the app ADBuddy checked` | The app on the phone is not the one the host trusts. Read the reason in brackets in the message. If you installed the app by hand, install it again with `adbuddy setup --apk <file>`. |
| `Could not reach the phone app` | Open the ADBuddy app on the phone. The host tries again by itself. |
| `Port … for the phone app's channel is in use` | Change `chat_port` in `config.toml` to a free port between 1024 and 65535, and start `adbuddy serve` again. |

Doctor's `phone app` line adds two cases. When the app is installed but without a permission, the accessibility service or the battery exemption, run `adbuddy setup` and accept on the phone's consent screen. When it says the channel with serve is not open, wait 15 seconds, run `adbuddy doctor` again, and open the ADBuddy app if it stays closed.

### The accessibility service will not turn on

The screen helpers use the app's accessibility service when the app is connected. Without the app, or with the service off, they fall back to adb: `adbuddy screen` reads the screen with `uiautomator dump`, `adbuddy tap` uses `input tap` and `adbuddy type "<text>"` uses `input text`, which only does plain ASCII. So a service that is off does not stop them. `UI_UNAVAILABLE` comes in the cases of its row in the table: Unicode without the app, the service on with the app not connected, or the phone's accessibility state unreadable.

The service still gives better reading of the screen. To turn it on:

1. Run `adbuddy doctor`. If the `phone app` line says `accessibility service` is missing, run `adbuddy setup` and accept on the ADBuddy app's consent screen: with your accept, the PC turns the service on and keeps the accessibility services you already use.
2. If you turn it on by hand in Android's **Accessibility** settings and the entry is greyed out, or a dialog says access was denied, Android is treating the app as a restricted setting. On the phones where this happens, open the app's page in **Settings → Apps** and look in its ⋮ menu for **Allow restricted settings**. The wording and the place vary by phone and Android version, so this is a pointer, not an exact path.

### Windows and the firewall

Windows may ask about the firewall the first time adb runs. Allow adb (Android's debugging tool, which ADBuddy downloads) on **private networks**. If you said no, the phone may not be found: allow adb again in the firewall's list of allowed apps in Windows Security. [Set up your phone](setup/#troubleshooting) has the same advice.

The app and the host talk over a local connection that adb carries, so the firewall matters for adb itself, not for the app's channel as such.

### The agent is not installed

ADBuddy does not install agents. If the agent you chose is not in your `PATH`:

- `adbuddy serve` starts anyway and prints `Could not start <agent>` with the install command. Serve then has no agent session.
- `adbuddy run` ends with `AGENT_NOT_FOUND`, with the same command in its fix.

The commands the host prints are:

| Agent | Command |
|---|---|
| Claude Code | `npm install -g @anthropic-ai/claude-code` |
| Codex CLI | `npm install -g @openai/codex` |
| Gemini CLI | `npm install -g @google/gemini-cli` |
| OpenCode | `npm install -g opencode-ai` |

Install it, open a new terminal so that your `PATH` is refreshed, and start the agent session again: `adbuddy new` with serve running, or `adbuddy run`. To use another agent, `adbuddy setup --agent <name>`.

### Another `adb` in the `PATH`

ADBuddy does not use the `adb` in your `PATH`. It uses its own copy of Android's platform-tools and its own adb server, on port 5038 (adb's own default is 5037), so your own adb server usually does not collide with it. The `adb` the agent runs is ADBuddy's shim: when ADBuddy starts the agent, the shim's folder is first in the agent's `PATH`.

- In your own terminal, `adb` is still yours. It does not go through ADBuddy and is not classified or journaled. See [What ADBuddy does not guarantee](safety.md#what-adbuddy-does-not-guarantee).
- If you point ADBuddy at an adb with `--adb` or `adb_path`, it must be version 37 or newer. Doctor's `adb` line fails on an older one. Run `adbuddy setup` to use the one ADBuddy downloads.

### The port 5038 is taken

ADBuddy's private adb server needs a free port: 5038 by default. If another program uses it, `adbuddy setup` or `adbuddy serve` ends with `ADB_FAILED`, and doctor's `adb server` line fails.

1. Stop other adb tools that use that port. To see what listens on it, on Linux and macOS:

   ```sh
   lsof -i :5038
   ```

   On Windows, in PowerShell:

   ```powershell
   Get-NetTCPConnection -LocalPort 5038
   ```

2. If `adbuddy serve` is not running, `adbuddy doctor --fix` restarts ADBuddy's own server.
3. If something else must keep the port, change `adb_server_port` in `config.toml` (a number from 1 to 65535).

### The environment folder cannot be made

The agent's environment is the folder `~/ADBuddy/<phone name>/`. The code `WORKSPACE_FAILED` means ADBuddy could not create or refresh it. The message has the system's reason, and the fix depends on where it happened:

- `adbuddy setup` and `adbuddy serve`: `Check that <folder> is writable.`
- `adbuddy serve` when it cannot work out the folder's absolute path: `Pass --workspace with an absolute path.`
- A path with characters the agent's hooks cannot carry: `Use another path: it cannot contain backslashes (on Linux and macOS), quotes, $, backticks or line breaks.`
- `adbuddy run`: `Check that <folder> is writable and that no other program (for example another agent using the adb of ADBuddy) has its files open, then try again.` (when it cannot work out the path, `run` gives the code with no fix).
- `adbuddy env init`: the host suggests nothing. Check that the folder is writable and that no program has its files open.

To make the folder again, once the cause is fixed, run `adbuddy env init`, or `adbuddy env init --dir <folder>` for another one.

## Collect information for an issue

When you open an [issue](https://github.com/hybriqtech/adbuddy/issues), give enough to reproduce it, and nothing private.

1. The version and the system:

   ```sh
   adbuddy version
   ```

2. The doctor report:

   ```sh
   adbuddy doctor --json
   ```

3. The error: the line that starts with `Error:` and, if there is one, `Fix:`, and what you ran.
4. What you expected, and what happened.

**Read what you paste, and remove these first:**

- The phone's **serial number**, which doctor prints on the `phone` line and `adbuddy serve` prints in its `ADBuddy is serving <phone> (session s_…)` line.
- **IP addresses and ports**, yours or the phone's.
- **Paths with your user name**: doctor prints the configuration file and the environment folder, and serve prints the `Journal:` path. Replace the part with your name with `<home>`.
- **The journal**: commands and file names can hold personal data, such as app names, file names and what the agent read. Paste only the lines that matter, after you read them.
- The **tokens and secrets**: the `ipc.secret` file in the data folder, and anything that looks like a key.

A security problem does not go in a public issue: see [Reporting a security problem](safety.md#reporting-a-security-problem).

## Next

- [Use ADBuddy](usage.md): the day-to-day guide.
- [Safety](safety.md): confirmations, protected apps, and what ADBuddy does not guarantee.
- [Set up your phone](setup/): Wireless debugging and pairing.
- [Install ADBuddy](install.md): build from source, update and uninstall.
- [FAQ](faq.md): short answers.
