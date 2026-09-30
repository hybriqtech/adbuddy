# ADBuddy host

The `adbuddy` binary: the host that sits between a coding agent and your Android phone. Every `adb` command the agent runs goes through it, is classified by risk, is written to a journal with its inverse and, when it cannot be undone, waits for your confirmation.

This is milestone M1: ADBuddy downloads its own adb, pairs your phone over Wi-Fi and guards every command. The phone app and the chat arrive in M2-M4.

## Build

```sh
cd host
go build -o adbuddy ./cmd/adbuddy      # adbuddy.exe on Windows
```

## Use

0. Set it up once: it downloads Android platform-tools, pairs the phone over Wi-Fi with a QR code, reads its profile and prepares the agent environment.

   ```sh
   adbuddy setup            # or: adbuddy setup --code (IP:port and 6-digit code)
   ```

   Step-by-step guide with pictures: https://hybriqtech.github.io/adbuddy/setup/

   Running plain `adbuddy` before setup starts the setup when it runs in a terminal; otherwise it fails with `NOT_CONFIGURED`.

   `adbuddy setup --adb <path>` uses an adb you already have (37 or newer) instead of downloading one. Stop `adbuddy serve` before running setup again: while it runs, setup refuses with `ALREADY_RUNNING`.

1. In one terminal, start the host and keep it open:

   ```sh
   adbuddy serve
   ```

   It waits for the phone (up to 45 s), refreshes the agent environment in `~/ADBuddy/<phone model>/` and shows every command live. When the agent runs something irreversible it asks here and shows a two-digit code: type that code to allow it; anything else denies it. Unanswered questions expire after 2 minutes and count as a no.

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

## Remote (Tailscale or another network)

ADBuddy finds the phone by mDNS, which only works on the same network. Over Tailscale (or a Wi-Fi that blocks mDNS) connect by IP address and port instead:

1. Pair with the code: `adbuddy setup --code`. On that screen (Wireless debugging → Pair device with pairing code) Android shows the phone's Tailscale IP when Tailscale is on; type that IP and port and the 6-digit code.
2. Give the connection address, the IP:port on the main Wireless debugging screen: either with `adbuddy setup --code --connect 100.64.0.5:41234`, or type it when setup asks "Connection address" (only at a terminal). Without either, setup fails with `DEVICE_NOT_FOUND` as before.
3. Later, when the address changes, connect again with `adbuddy connect <ip:port>`. It checks that the phone at that address is the one you set up (`WRONG_PHONE` otherwise, and nothing is saved) and saves the address in `config.toml`. `adbuddy serve` and `adbuddy setup` try that address first (about 15 seconds at most, checking it is your phone) and then wait for the phone on the local network as usual.

The connection port changes each time wireless debugging is turned on again: `serve` then fails with `DEVICE_NOT_FOUND` and asks you to run `adbuddy connect <new ip:port>`.

Pairing with the QR code needs the same network (it uses mDNS). If `setup` sees no phone with wireless debugging on the network after a few seconds, it says so and suggests `adbuddy setup --code`.

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
adb_path = ""                # empty: ADBuddy's own platform-tools (installed by setup)
adb_server_port = 5038       # ADBuddy's own adb server
serial = ""                  # only needed with several phones
address = ""                 # set by setup and `adbuddy connect`: the last IP:port that worked (e.g. over Tailscale)
workspace = ""               # set by setup: ~/ADBuddy/<phone model>; `run` and `journal` follow the running `serve`
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

`adbuddy` commands: 0 success, 1 failure, 2 invalid usage, 75 temporary failure, 78 configuration error, 130 interrupted (`INTERRUPTED`: Ctrl+C during `setup` or while `serve` starts). With `--json` they print `{"ok":true,"data":…}` or `{"ok":false,"error":{…}}`.

The `adb` shim returns adb's own exit code, 126 when a command is denied, expires or is blocked (`Denied by user: …`; `Denied: nobody could confirm: …` when nobody could be asked, with `serve --confirm deny` or no terminal; `Blocked by ADBuddy: …`), and 1 with `ADBuddy host not running (run: adbuddy serve)` when the host is not running.

## Security notes

- The shim talks to the host over a local socket (`adbuddy.sock` in the data folder) plus a per-installation secret (`ipc.secret`, readable only by you). On Windows the socket file inherits the permissions of `%LOCALAPPDATA%\ADBuddy\`, which only your user can open.
- If the journal cannot be written, nothing 🟡 or 🔴 runs.
- Every journal record is signed with the installation secret, and `adbuddy undo` only runs inverses that ADBuddy itself wrote, in the shapes it knows; `adbuddy journal` marks records that were not written by ADBuddy.
- ADBuddy protects against mistakes and prompt injection, not against an agent determined to find another adb. The real kill switch arrives with the app ("Cut access", M2).

## Manual check with a real phone

With the phone paired and wireless debugging on:

```sh
adbuddy serve --confirm deny     # deny: every 🔴 is refused without asking
# in another terminal, using the environment's shim:
~/ADBuddy/<phone model>/.adbuddy/bin/adb shell getprop ro.product.model
~/ADBuddy/<phone model>/.adbuddy/bin/adb shell settings get global window_animation_scale
adbuddy journal
```

## Tests

```sh
go test ./...
```

The tests never touch a phone: they use a fake adb (`internal/testutil/fakeadb`).
