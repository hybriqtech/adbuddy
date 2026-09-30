# ADBuddy host

The `adbuddy` binary: the host that sits between a coding agent and your Android phone. Every `adb` command the agent runs goes through it, is classified by risk, is written to a journal with its inverse and, when it cannot be undone, waits for your confirmation.

This is milestone M1a: the safety core. It works with a phone that is already paired over wireless debugging and an adb you point it to. Setup, pairing and ADBuddy's own adb arrive in M1b; the phone app and the chat in M2-M4.

## Build

```sh
cd host
go build -o adbuddy ./cmd/adbuddy      # adbuddy.exe on Windows
```

## Use

1. In one terminal, start the host and keep it open:

   ```sh
   adbuddy serve --adb /path/to/platform-tools/adb
   ```

   It waits for the phone (up to 45 s), prepares the agent environment in `~/ADBuddy/default/` and shows every command live. When the agent runs something irreversible it asks here and shows a two-digit code: type that code to allow it; anything else denies it. Unanswered questions expire after 2 minutes and count as a no.

2. In another terminal, start the agent:

   ```sh
   adbuddy run            # Claude Code by default
   adbuddy run codex
   ```

   The agent runs in the environment folder, with ADBuddy's `adb` first in its `PATH` and the phone already selected.

3. Look back and undo:

   ```sh
   adbuddy journal                    # what was done, with the inverse of each change
   adbuddy journal --session current  # only the current session
   adbuddy undo                       # undo the last change
   adbuddy undo session               # undo the whole session
   adbuddy undo j_01J…                # undo one entry
   ```

`adbuddy env init [--force]` creates or refreshes the environment without starting anything.

## Risk levels

| Level | Examples | What happens |
|---|---|---|
| 🟢 read-only | `getprop`, `dumpsys`, `pm list`, `settings get`, `ls`, `cat`, `pull` into the agent environment | Runs and is journaled |
| 🟡 reversible or harmless | `settings put`, `pm disable-user`, `pm uninstall --user 0` (system apps), `appops set`, `mv`, `install`, `push`, `input`, `am start` | Runs and is journaled with its inverse (if it has one) |
| 🔴 irreversible, dangerous or unknown | `rm`, `pm clear`, `uninstall`, `reboot`, `settings delete`, anything ADBuddy does not understand | Waits for your confirmation |

Anything that could cut the connection or disable ADBuddy (Wi-Fi, debugging settings, `adb tcpip`/`usb`/`disconnect`/`kill-server`, touching the ADBuddy app) is always 🔴, whatever your own rules say.

`adb shell` lines are parsed like the phone's shell does: `;`, `&&`, pipes, `$(…)`, `sh -c`, `su -c`, redirections and variables are all looked at, and the most dangerous part wins.

## Configuration

`config.toml` lives in `%APPDATA%\ADBuddy\` (Windows), `~/Library/Application Support/ADBuddy/` (macOS) or `~/.config/adbuddy/` (Linux):

```toml
adb_path = "C:\\Android\\platform-tools\\adb.exe"
adb_server_port = 5038       # ADBuddy's own adb server
serial = ""                  # only needed with several phones
workspace = ""               # default: ~/ADBuddy/default; `run` and `journal` follow the running `serve`
confirm_timeout = "2m"
language = "es"              # descriptions in the journal: en or es
agent = "claude"             # default for `adbuddy run`
```

Your own rules go in `rules.yaml` next to it and win over the built-in ones (except the fixed protections):

```yaml
rules:
  - {id: my.tmp-cleanup, cmd: rm, level: yellow, title: {en: "Delete temporary files: {args}", es: "Borrar temporales: {args}"}}
```

## Exit codes

`adbuddy` commands: 0 success, 1 failure, 2 invalid usage, 75 temporary failure, 78 configuration error, 130 interrupted (`INTERRUPTED`: Ctrl+C while `serve` waits for the phone). With `--json` they print `{"ok":true,"data":…}` or `{"ok":false,"error":{…}}`.

The `adb` shim returns adb's own exit code, 126 when a command is denied, expires or is blocked (`Denied by user: …`, `Blocked by ADBuddy: …`), and 1 with `ADBuddy host not running (run: adbuddy serve)` when the host is not running.

## Security notes

- The shim talks to the host over a local socket (`adbuddy.sock` in the data folder) plus a per-installation secret (`ipc.secret`, readable only by you). On Windows the socket file inherits the permissions of `%LOCALAPPDATA%\ADBuddy\`, which only your user can open.
- If the journal cannot be written, nothing 🟡 or 🔴 runs.
- Every journal record is signed with the installation secret, and `adbuddy undo` only runs inverses that ADBuddy itself wrote, in the shapes it knows; `adbuddy journal` marks records that were not written by ADBuddy.
- ADBuddy protects against mistakes and prompt injection, not against an agent determined to find another adb. The real kill switch arrives with the app ("Cut access", M2).

## Manual check with a real phone

With the phone paired and wireless debugging on:

```sh
adbuddy serve --adb <path to adb> --confirm deny     # deny: every 🔴 is refused without asking
# in another terminal, using the environment's shim:
~/ADBuddy/default/.adbuddy/bin/adb shell getprop ro.product.model
~/ADBuddy/default/.adbuddy/bin/adb shell settings get global window_animation_scale
adbuddy journal
```

## Tests

```sh
go test ./...
```

The tests never touch a phone: they use a fake adb (`internal/testutil/fakeadb`).
