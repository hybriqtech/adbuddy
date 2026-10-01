# ADBuddy host

The `adbuddy` binary: the host that sits between a coding agent and your Android phone. Every `adb` command the agent runs goes through it, is classified by risk, is written to a journal with its inverse and, when it cannot be undone, waits for your confirmation.

This is milestone M1: ADBuddy downloads its own adb, pairs your phone over Wi-Fi and guards every command. The phone app and the chat arrive in M2-M4.

## Install

- **Linux and macOS:**

  ```sh
  curl -fsSL https://github.com/hybriqtech/adbuddy/releases/latest/download/install.sh | sh
  ```

  It installs the latest release in `~/.local/bin` after checking its SHA-256. `ADBUDDY_INSTALL_DIR` picks another folder, and `ADBUDDY_VERSION=v0.3.0` another release.
- **Windows:** download `adbuddy_<version>_windows_amd64.zip` (or `_arm64`) from the [releases](https://github.com/hybriqtech/adbuddy/releases), check it against `checksums.txt` and put `adbuddy.exe` in a folder in your `PATH`. winget comes later.
- **From source:**

  ```sh
  cd host
  go build -o adbuddy ./cmd/adbuddy      # adbuddy.exe on Windows
  ```

  A build from source pins no signing certificate for the phone app. Install the app with `adbuddy setup --apk <file>`.

Releases: `docs/releasing.md`. Licensed under Apache-2.0 (`LICENSE`), © 2026 hybriqtech.

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

### Mobile data only (remote mode)

Without any Wi-Fi, ADBuddy can reach the phone over Tailscale through `adb tcpip 5555`. The phone must be connected to the host and have Tailscale on. You turn it on, not the agent:

```sh
adbuddy remote on       # asks you to confirm (in the app or in the `serve` terminal), then opens port 5555 for 12 hours
adbuddy remote off      # closes it (`adb usb`)
adbuddy remote status   # on, closing, or off, and when it expires
```

You can also use the switch in the ADBuddy app. From the agent's session these commands are refused (`AGENT_SESSION`). Port 5555 stays open on every network the phone joins until it expires or you turn it off, so use it only when you need it. Only a running `adbuddy serve` closes it when it expires: with serve stopped nothing expires, and the port stays open until you run `adbuddy remote off` (it closes the port directly) or start serve again.

`adbuddy serve` also watches the phone: if it disappears (for example after `adb tcpip` or when you leave home), after two failed checks, 10 s apart, it tries the saved address, the phone on the local network, the phone's Tailscale address and, in remote mode, `<tailscale ip>:5555`. In remote mode screenshots default to JPEG at half scale.

## Risk levels

| Level | Examples | What happens |
|---|---|---|
| 🟢 read-only | `getprop`, `dumpsys`, `pm list`, `settings get`, `ls`, `cat`, `pull` into the agent environment | Runs and is journaled |
| 🟡 reversible or harmless | `settings put`, `pm disable-user`, `pm uninstall --user 0` (system apps), `appops set`, `mv`, `install`, `push`, `input`, `am start` | Runs and is journaled with its inverse (if it has one) |
| 🔴 irreversible, dangerous or unknown | `rm`, `pm clear`, `uninstall`, `reboot`, `settings delete`, anything ADBuddy does not understand | Waits for your confirmation |

Anything that could cut the connection or disable ADBuddy (Wi-Fi, debugging settings, `adb tcpip`/`usb`/`disconnect`/`kill-server`, touching the ADBuddy app) is always 🔴, whatever your own rules say.

`adb shell` lines are parsed like the phone's shell does, and the most dangerous part wins. ADBuddy only reads the shell it lists: plain commands, `;`, `&&`, `||`, pipes, `if`, `for … in`, `while`, `until`, here-docs, `!`, `( )`, `{ }`, `[ … ]`, `$(…)`, `sh -c`, `su -c`, redirections to `/dev/null`, redirections between descriptors (`2>&1`) and variables of the script's own, with lower-case names (reading upper-case ones, such as `$EXTERNAL_STORAGE`, is fine). Anything else (`[[ ]]`, `(( ))`, `case`, functions, `typeset`, `export`, arrays, assigning upper-case variables, builtins such as `trap` or `eval`) is not analysed: it is 🔴 and cannot be allowed for the whole session. A write to a file (`> /sdcard/x`) is an ordinary 🔴 that you can allow for the session.

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

`adbuddy` commands: 0 success, 1 failure, 2 invalid usage, 75 temporary failure, 78 configuration error, 130 interrupted (`INTERRUPTED`: Ctrl+C during `setup` or `connect`, or while `serve` starts). With `--json` they print `{"ok":true,"data":…}` or `{"ok":false,"error":{…}}`.

The `adb` shim returns adb's own exit code, 126 when a command is denied, expires or is blocked (`Denied by user: …`; `Denied: nobody could confirm: …` when nobody could be asked, with `serve --confirm deny` or no terminal; `Blocked by ADBuddy: …`), and 1 with `ADBuddy host not running (run: adbuddy serve)` when the host is not running.

## Security notes

- The shim talks to the host over a local socket (`adbuddy.sock` in the data folder) plus a per-installation secret (`ipc.secret`, readable only by you). On Windows the socket file inherits the permissions of `%LOCALAPPDATA%\ADBuddy\`, which only your user can open.
- If the journal cannot be written, nothing 🟡 or 🔴 runs.
- Every journal record is signed with the installation secret, and `adbuddy undo` only runs inverses that ADBuddy itself wrote, in the shapes it knows; `adbuddy journal` marks records that were not written by ADBuddy.
- ADBuddy protects against mistakes and prompt injection, not against an agent determined to find another adb. The real kill switch is in the phone app: "Cut access" turns debugging off from the phone, without the PC.

## Phone app

`adbuddy setup` installs the ADBuddy app (the latest release, or `--apk <file>`) and asks for your consent in it. Only if you accept, it gives the app the permission for "Cut access", exempts it from battery optimisation and turns on its accessibility service, keeping any other one. From then on `adbuddy serve` connects to the app through `adb reverse` on `127.0.0.1:<chat_port>`: 🔴 confirmations go to the phone (unlocking is required to allow), the app shows the journal live, and you can pause the agent from the phone. When the app is not connected, confirmations happen in the terminal as before. The agent cannot use the app's channel: `content` and `run-as` on `tech.hybriq.adbuddy` always ask first.

## Claude Code chat view (M4)

With Claude Code as the agent, the phone shows its answers as chat bubbles, the
tools it uses, and its permission questions with buttons (Yes / Yes, always / No).
The terminal stays as the fallback. Spec: `docs/superpowers/specs/2026-10-01-adbuddy-m4-claude-code-design.md`.

- **How:** `adbuddy env init` (and `serve`) put six hooks in the environment's
  `.claude/settings.json` (`SessionStart`, `UserPromptSubmit`, `Stop`,
  `Notification`, `PermissionRequest`, `SessionEnd`), each running the
  environment's own `adbuddy hook <event>`, and the host follows Claude Code's
  transcript. Your own hooks in that file are kept.
- **`adbuddy hook` fails open:** if the host is down or anything is wrong it
  says nothing and exits 0, so Claude Code asks on the terminal as usual (an
  exit code 2 would block the agent, so it never uses it). With no phone connected,
  permission questions are not held for the phone either.
- **"Yes, always"** is only offered for a Bash command that is short, on one line,
  with no shell operators, and that cannot touch adb, the shim or the phone. It adds
  the exact command (never a wildcard) to the environment's
  `.claude/settings.local.json`. A permission of Claude Code is not a 🔴
  confirmation: the shim still classifies and journals every adb command.
- **Your own global hooks** (`~/.claude/settings.json`) that answer
  `PermissionRequest` or `PreToolUse` can answer the same request as ADBuddy's:
  `adbuddy doctor` warns about them.
- **If Claude Code changes its transcript format** the chat falls back to the last
  answer of each turn (from the `Stop` hook) and the terminal view, and says so once.
  Tested against Claude Code 2.1.x; recordings in `host/internal/claudecode/testdata`.
- **Real Claude Code test** (spends a few cents, never touches the phone nor your
  `~/.claude` settings): `ADBUDDY_REAL_CLAUDE=1 go test ./internal/cli -run TestRealClaudeCode -v -timeout 600s`.

## Manual check with a real phone

With the phone paired and wireless debugging on:

```sh
adbuddy serve --confirm deny     # deny: every 🔴 is refused without asking
# in another terminal, using the environment's shim:
~/ADBuddy/<phone model>/.adbuddy/bin/adb shell getprop ro.product.model
~/ADBuddy/<phone model>/.adbuddy/bin/adb shell settings get global window_animation_scale
adbuddy journal
```

## Skills

Skills are recipes the agent follows for common phone tasks: `device-profile`, `permissions-audit`, `storage-cleanup`, `debloat`, `battery`, `organize-files`, `backup-to-pc`, `home-screen` and `build-utility-apk`. They live in `internal/workspace/skills/<name>/SKILL.md` and are embedded in the binary. `adbuddy env init` copies them to `.claude/skills/` and `.agents/skills/` of the environment (and writes `.gemini/settings.json` so Gemini CLI reads `AGENTS.md`); run `adbuddy env init --force` to refresh them after an update. A skill that starts with `<!-- adbuddy:user -->` is yours: it is never overwritten. `adbuddy setup` rebuilds the environment with Force, so it rewrites every skill that lacks that mark. `env init` prints the skill paths grouped by folder (with `/` also on Windows); `--json` lists them all.

To add a skill: create its folder, add its name to `requiredSkills` in `internal/workspace/skills_test.go`, and write its examples in checked blocks: ```` ```adb ```` (a command without the `adb` prefix, annotated `# green`, `# yellow` or `# red`), ```` ```adbuddy ```` (a subcommand) and, for commands that run on the PC, ```` ```text ```` (not checked). The guard `internal/cli/skills_commands_test.go` enforces:

- the annotation is the level the real classifier gives (at run time the level can rise through the precheck);
- recipes never clear app data, wipe, reboot, install granting permissions or delete recursively; the one exception is the exact `shell rm -r /sdcard/ADBuddy-papelera` in `storage-cleanup`;
- scripts go in single quotes, and a line that starts with `adb` or `adbuddy` outside a checked block is an error.

## Tests

```sh
go test ./...
```

The tests never touch a phone: they use a fake adb (`internal/testutil/fakeadb`).

### The classifier's oracle

What the phone's shell (mksh) really does is not taken from memory: `internal/classify/testdata/oracle.jsonl` records, for every case of `differential.jsonl` and every generated one, whether a real mksh ran the sentinel `@S@`. `go test` fails if a case is not recorded, or if mksh ran the sentinel and the classifier did not answer 🔴 special. `mksh-builtins.tsv` lists the builtins of that mksh, and each must be admitted with a grammar or never analysed.

To record them again after adding a case, use the Android emulator (SDK packages `emulator` and `system-images;android-37.0;google_apis;x86_64`). It has its own adb server, and the recorder refuses any device that is not an emulator:

```sh
eval "$(scripts/emulator.sh start)"     # from the repository root; about a minute
cd host && go test -timeout 60m -count=1 -run TestRecordOracle ./internal/classify   # about 10 minutes
go test ./internal/classify
../scripts/emulator.sh stop
```

The `emulator` workflow records it again on every pull request that touches the classifier and fails if the result differs from the files in the repository.

The read-only examples of the skills (`# green`, `shell ...`) also run on the emulator with `go test -timeout 20m -count=1 -run TestSkillsReadOnlyOnEmulator ./internal/cli`, with the same variables as the recorder; it fails if the phone says a program or an option does not exist, and it refuses any serial that is not `emulator-*`. The `emulator` workflow runs it too.
