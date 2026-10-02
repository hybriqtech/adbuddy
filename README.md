[Leer en español](README.es.md)

# ADBuddy

Chat from your phone with a coding agent on your PC, and let it manage your Android phone over Wi-Fi adb, with every command ranked by risk, every change journaled and the commands it ranks red (irreversible, dangerous or not understood) confirmed first.

## What it does

- **The agent manages your Android over Wi-Fi adb.** The coding agent you already use on your PC (Claude Code, Codex, Gemini CLI or OpenCode) runs adb commands on your phone: no USB cable, and you do not need to know what adb is.
- **You chat with it from the phone.** A small ADBuddy app on the phone has the chat, shows what the agent does and asks you before the commands it ranks red.
- **Guarded by default.** Every command is ranked green, yellow or red, changes are journaled, with their undo when they have one, and red commands wait for your answer. The app has a Pause and a **Cut access** button.

## How it works

```text
  Phone (Android 11+)                      PC (Windows, macOS or Linux)
  +--------------------+                   +-----------------------------------------+
  | ADBuddy app        |  local channel    | coding agent (in the environment)       |
  |  chat, questions,  | <---------------> |      | runs its "adb" = the shim        |
  |  Pause, Cut access |  (carried by adb) |      v                                  |
  |                    |                   | host: adbuddy serve                     |
  | Wireless debugging | <---------------- |  ranks, journals, asks you,             |
  +--------------------+     Wi-Fi adb     |  then runs the real adb                 |
                                           |  (ADBuddy's own platform-tools)         |
                                           +-----------------------------------------+
```

- **On the PC:** the host (`adbuddy serve`), the coding agent and ADBuddy's own copy of adb. The agent works in a folder of its own, the *environment* (`~/ADBuddy/<phone name>/`).
- **On the phone:** the ADBuddy app and Android's Wireless debugging.
- **The agent does not run the real adb.** When ADBuddy starts the agent, the `adb` it finds first is a *shim*, a stand-in program that forwards each command to the host. The host ranks it, writes it to the journal, asks you when needed and only then runs the real adb.

## Quick start

There is no public release yet, so today you build the host from source. [Install ADBuddy](docs/install.md) has the details and the other systems.

1. **Install the host.** You need [Git](https://git-scm.com/) and [Go](https://go.dev/dl/) 1.27 or newer. On Windows, build `adbuddy.exe` instead and put it in a folder in your `PATH`.

   ```sh
   git clone https://github.com/hybriqtech/adbuddy.git
   cd adbuddy/host
   go build -o adbuddy ./cmd/adbuddy
   mkdir -p ~/.local/bin
   cp adbuddy ~/.local/bin/
   ```

   When there is a public release, a single `curl … | sh` command will install it on Linux and macOS (Windows will use a downloaded archive): see [Install ADBuddy](docs/install.md).

2. **Prepare the phone.** Turn on Developer options and Wireless debugging. [Set up your phone](https://hybriqtech.github.io/adbuddy/setup/) has the steps (screenshots coming; in the repository: [docs/setup/index.md](docs/setup/index.md)).

3. **Run setup.** It downloads Android platform-tools, pairs the phone with a QR code, installs the app and prepares the agent's environment. Today it does not download the app: build the APK as [Install ADBuddy](docs/install.md#build-from-source) says and pass it.

   ```sh
   adbuddy setup --apk <file>
   ```

4. **Start the host** in a terminal and keep it open. By default it starts the agent for you.

   ```sh
   adbuddy serve
   ```

5. **Talk to the agent.** Open the ADBuddy app on the phone and write to it. Or, from a second terminal on the PC:

   ```sh
   adbuddy attach
   ```

   If you prefer to run the agent in a terminal of your own, start the host with `adbuddy serve --no-agent` and run `adbuddy run` in another terminal; you lose the chat on the phone. See [Use ADBuddy](docs/usage.md).

## Supported agents

Claude Code (recommended), Codex, Gemini CLI and OpenCode. ADBuddy does not install agents: it starts the one you already have, and if it is missing, `adbuddy run` prints the command that installs it. With Claude Code the phone shows the agent's answers as chat bubbles; with the others it shows the agent's terminal.

## Safety

[Safety](docs/safety.md) has the full explanation. In short:

- Every `adb` command the agent runs passes through the shim and the host, which ranks it: green (read-only), yellow (reversible or harmless) or red (irreversible, dangerous or not understood). A command the host does not understand is red.
- Green and yellow commands run at once and are journaled; yellow ones with their undo when they have one (`adbuddy journal`, `adbuddy undo`). Taps and typing are yellow and have no undo. A yellow command becomes red when ADBuddy cannot read the state it needs or the change would overwrite something.
- Red commands wait for your answer, on the phone (it must be unlocked to allow) or in the host's terminal. An unanswered question counts as a no (after 2 minutes by default).
- With the app installed, banking, payments, password and 2FA apps are protected while they are in front (a proposal from their names that you confirm or edit in the app). Without the app, only ADBuddy itself is protected.
- ADBuddy has no telemetry and no server of its own. What the agent reads on your phone goes to the agent's own provider.
- **The shim protects against mistakes and prompt injection. It does not protect against an agent determined to find another adb.** The real guarantee is **Cut access** in the app, which turns debugging off from the phone: with it off, the PC cannot reach the phone.

ADBuddy lowers the risk of letting an agent drive your phone; it does not remove it. Use a phone and accounts you can afford to repair, and read each question before you allow it.

## Why ADBuddy

ADBuddy has no model of its own and needs no API keys. As of September 2026, Google's open source ARTEMIS needs its own LLM and API keys; ADBuddy uses the agent and subscription you already have.

## Requirements

- **Phone:** Android 11 or newer, with Wireless debugging.
- **PC:** Windows, macOS or Linux, on the same Wi-Fi network as the phone (or Tailscale).
- **A coding agent** installed on the PC: Claude Code, Codex, Gemini CLI or OpenCode.
- **adb** 37 or newer: `adbuddy setup` downloads Google's platform-tools for you, so you do not install it.

## Commands

These are the commands of `adbuddy --help`; `adbuddy <command> --help` shows the options of each one.

| Command | What it does |
|---|---|
| `adbuddy setup` | Set up ADBuddy: get adb, pair the phone over Wi-Fi and prepare the agent environment |
| `adbuddy serve` | Run the ADBuddy host: live journal and confirmations for the agent's adb commands |
| `adbuddy run` | Start a coding agent in the ADBuddy environment |
| `adbuddy attach` | Connect this terminal to the agent's session in `adbuddy serve` (Ctrl+] to detach) |
| `adbuddy new` | Start a new conversation in `adbuddy serve`, with another agent if given |
| `adbuddy stop` | Stop the agent that `adbuddy serve` runs (serve keeps running) |
| `adbuddy journal` | Show the journal of adb commands and their inverses |
| `adbuddy undo` | Undo journaled changes by running their inverses |
| `adbuddy connect <ip:port>` | Connect to the phone by IP address and port (Tailscale or another network without mDNS) |
| `adbuddy remote` | Open adb's network port 5555 for 12 hours (remote mode), close it, or see its state |
| `adbuddy doctor` | Check that ADBuddy, adb, the phone and the app are ready, and say how to fix what is not |
| `adbuddy env` | Manage the agent environment |
| `adbuddy screen` | List what is on the phone's screen: ids, texts and where to tap |
| `adbuddy tap` | Tap an element of the phone's screen by its text, its id or a point |
| `adbuddy type "<text>"` | Type text in the focused field of the phone (it replaces what the field has) |
| `adbuddy foreground` | Show the app and activity in the phone's foreground |
| `adbuddy notifications` | List the phone's current notifications (protected apps show no text) |
| `adbuddy screenshot` | Save a screenshot of the phone, scaled down |
| `adbuddy send <file>` | Send a file of the environment to the user's phone (for the agent) |
| `adbuddy version` | Print the ADBuddy version |

## Documentation

- [Install ADBuddy](docs/install.md): build from source, update and uninstall.
- [Set up your phone](https://hybriqtech.github.io/adbuddy/setup/): Developer options, Wireless debugging and pairing ([docs/setup/index.md](docs/setup/index.md) in the repository); [in Spanish](docs/setup/es/index.md).
- [Use ADBuddy](docs/usage.md): start the host, talk to the agent, answer questions, undo, remote mode.
- [Safety](docs/safety.md): what ADBuddy protects and what it does not guarantee.
- [FAQ](docs/faq.md): short answers.
- [Troubleshooting](docs/troubleshooting.md): `adbuddy doctor` and the error codes.
- The same guides as a website: https://hybriqtech.github.io/adbuddy/
- For developers: [host/README.md](host/README.md).

<!-- Enable when the GitHub Sponsors and Ko-fi pages exist (docs/pre-public-checklist.md):

## Support the project

ADBuddy is free and open source; if it saves you time, you can support its development.

[GitHub Sponsors](https://github.com/sponsors/hybriqtech) · [Ko-fi](https://ko-fi.com/<handle>)
-->

## License

Apache-2.0. See [LICENSE](LICENSE).

## Status

There is no public release yet: the [Releases page](https://github.com/hybriqtech/adbuddy/releases) is where one will appear. ADBuddy is under active development, and today you build it from source. Homebrew, winget and signed Windows binaries are not available yet.
