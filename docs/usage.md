---
title: Use ADBuddy
layout: default
lang: en
---

# Use ADBuddy

This page is for the day after [setup](install.md): start the host, talk to your agent, answer its questions, look back at what it did and undo it.

ADBuddy puts a **shim** between the agent and the phone. A shim is a small stand-in program: the agent runs what it thinks is `adb`, and ADBuddy's copy ranks the command by risk, writes it to a **journal** and, when it ranks the command red, waits for your answer. The **environment** is the folder where the agent works (`~/ADBuddy/<phone name>/`).

Every command gets one of three levels: green (read-only, runs at once), yellow (reversible or harmless; journaled, with its undo when it has one) and red (irreversible, dangerous or not understood: it waits for you). [Safety](safety.md) explains each one.

## Start the host

1. Put the phone on the same Wi-Fi as the PC, with Wireless debugging on.
2. Open a terminal and run:

   ```sh
   adbuddy serve
   ```

3. Keep that terminal open. It waits for the phone (up to 45 seconds), refreshes the environment, prints every command the agent runs and asks you about the red ones. Ctrl+C stops it.

You will see lines like these, and more:

```text
Waiting for the phone on ADBuddy's adb server (port 5038)…
ADBuddy is serving <phone> (session s_…).
Journal: <environment>/journal
Press Ctrl+C to stop.
Agent: claude, in this host. Connect a terminal with `adbuddy attach` (Ctrl+] to detach), or chat from the phone.
```

**Why a terminal for the host?** `adbuddy serve` is the guard: it classifies, journals and asks. It has to run outside the agent, so the agent does not start its own host and answer its own questions. It refuses to start when the agent runs it (`AGENT_SESSION`), a check against mistakes rather than a barrier. That is why you keep one terminal for the host and talk to the agent from another place: the phone, or a second terminal.

If the phone is not found, serve stops with `DEVICE_NOT_FOUND`: see [Troubleshooting](troubleshooting.md).

## Talk to the agent

By default `adbuddy serve` starts the agent for you, inside the host: Claude Code unless you chose another one in setup. Claude Code continues its last conversation. ADBuddy marks the environment folder as trusted in Claude Code's own settings, so Claude Code does not ask. Reach the agent in one of three ways.

### From the phone

Open the ADBuddy app. It shows the chat. See [The app on the phone](#the-app-on-the-phone).

### From a PC terminal

In a second terminal:

```sh
adbuddy attach
```

You see the agent's screen and type into it. Press Ctrl+] to detach: the agent keeps running. While you are attached, this terminal's size is the agent's. `adbuddy attach` needs an interactive terminal.

### In a terminal of your own

If you want the agent to run in a terminal you control, and you do not need the chat on the phone, start the host without the agent. Then start the agent in another terminal:

```sh
adbuddy serve --no-agent
```

```sh
adbuddy run
adbuddy run codex
adbuddy run codex -- --model o4
```

`adbuddy run` starts the agent in the environment, with ADBuddy's `adb` first in its `PATH` and the phone already chosen. It uses the agent from setup unless you name another: `claude`, `codex`, `gemini` or `opencode`. Put the agent's own options after `--`. The exit code is the agent's. `adbuddy run` needs a running `adbuddy serve`.

If the agent is not installed, the command prints how to install it (`AGENT_NOT_FOUND` for `run`). ADBuddy never installs agents.

### New conversation, stop

From a terminal, while serve runs:

```sh
adbuddy new
adbuddy new codex
adbuddy stop
```

`adbuddy new` starts a fresh conversation, with another agent if you name one. `adbuddy stop` stops the agent; serve keeps running. The phone has **New conversation** in its menu. These commands are refused (`AGENT_SESSION`) when the agent's session runs them; that is a check against mistakes, not a barrier.

## Answer red questions

When the agent runs a command ADBuddy ranks red, it holds the command until you answer. The agent waits.

- **App connected:** the question appears on the phone as a notification with **Allow**, **Deny** and, when the host offers it, **Allow during the session**. The app shows the same question as a dialog when it is open. The phone must be unlocked to allow; Deny always works.
- **No app connected:** the question appears in the `adbuddy serve` terminal, similar to this (a `Why:` line follows when there is a reason):

  ```text
  🔴 Confirmation needed (expires in 2m0s)
     <what the agent wants to do>
     <the command>
  To allow it type 73 and press Enter; anything else denies it.
  Code: 
  ```

  Type the two-digit code to allow it. Anything else denies it.

Unanswered questions expire after 2 minutes (`confirm_timeout` in `config.toml`) and count as a no. The agent gets `Denied by user: …` and exit code 126, and is told not to retry.

`adbuddy serve --confirm deny` refuses every red command without asking. Use it when nobody is there to answer.

## The app on the phone

What the app does today:

- **Chat.** You type a message and the agent answers. With Claude Code you see its answers as chat bubbles, a line with the tool it is using, and its own permission questions as cards with **Yes**, **Yes, always** and **No**. A Claude Code permission is not an ADBuddy confirmation: ADBuddy still ranks and journals every adb command. With other agents the app shows the agent's terminal instead.
- **Terminal.** The arrow in the header shows or hides the agent's terminal.
- **Attach** (the plus button): a photo, a file or a screenshot of the phone. Files can be up to 200 MiB.
- **Files from the agent.** When the agent sends you a file, it appears in the chat: tap it to download. Images download by themselves, up to a size limit.
- **Notifications.** A conversation notification with the last message, **Reply**, **Send screenshot**, **Pause** or **Resume**, and **Cut access**. Reply and the screenshot do nothing while the phone is locked. A **Done** notification tells you when the agent finishes. The lock screen shows no message text.
- **Activity.** The ⋮ menu has **Activity**: every adb command of the agent with its level, its state and, when it has one, its undo. Filter by level at the top.
- **Remote mode.** A card above the chat. See [Remote mode](#remote-mode).

The app is optional. Without it you still have the host: confirmations in the terminal, `adbuddy journal`, `adbuddy undo`.

## Look back and undo

Every command is written to the journal in the environment (`<environment>/journal`).

```sh
adbuddy journal
adbuddy journal --session current
adbuddy journal --limit 0 --json
```

Each entry is one line: the time, the level, the state, what it did, its id (`j_…`) and the command. A change that can be undone shows its inverse below it:

```text
14:02:31 🟡 ok       Change setting screen_off_timeout to 30000  [j_01J…] adb shell settings put system screen_off_timeout 30000
    undo: adb shell settings put system screen_off_timeout 60000
```

`--session current` shows only the session serve is running now. `--limit` shows the last N entries (50 by default, `0` for all). Entries that ADBuddy did not write are marked `not written by ADBuddy`.

To undo:

```sh
adbuddy undo
adbuddy undo session
adbuddy undo <id>
```

- `adbuddy undo` undoes the most recent change that has an inverse. Newer changes with no inverse, or in an uncertain state, are listed as `skipped`.
- `adbuddy undo session` undoes every change of the current session, newest first.
- `adbuddy undo <id>` undoes one entry. Take the id from `adbuddy journal`.

Undo runs the inverse the journal recorded and does not ask again. It prints one line per entry: `undone`, `failed` or `skipped`. It needs `adbuddy serve` running, and it does not run while the agent is paused.

**What has no undo.** ADBuddy records an inverse for the changes it understands: a setting changed, an app disabled or uninstalled for the user, an app operation mode set with `appops`, a battery exemption, a file or folder moved or created, a file pushed (a replaced file is restored from a backup) and a new app installed. It has none for:

- red commands: deleting files, clearing an app's data, rebooting, deleting settings and anything ADBuddy does not understand;
- an app update (an install over an existing app);
- one command that makes two changes (`a; b`);
- a command that failed, or that was running when the host stopped: the state is uncertain, so undo skips it and tells you to check the phone.

These are reverted by hand. Asking `adbuddy undo <id>` for such an entry fails with `NO_INVERSE`; `adbuddy undo` and `adbuddy undo session` list them as `skipped` (an uncertain one, as `UNCERTAIN_STATE`).

## Screen helpers

The agent reads and acts on the phone's screen with these. They go through the host, so `adbuddy serve` must be running. You can run them too:

```sh
adbuddy screen
adbuddy tap "Network & internet"
adbuddy tap --xy 540,1200
adbuddy type "Wi-Fi"
adbuddy foreground
adbuddy screenshot
```

- `adbuddy screen` lists what is on the screen, one element per line, with its id and position. Run it before tapping.
- `adbuddy tap` taps an element by its text, by its id (`--id`) or at a point (`--xy`).
- `adbuddy type "<text>"` types into the field that has the focus. It replaces the field's text. Without the app, only plain ASCII works.
- `adbuddy foreground` shows the app in front.
- `adbuddy screenshot` saves a picture, scaled to half by default, in `screenshots/` of the current folder. `-o` picks the file and `--scale` the size.

Each one is journaled like an adb command. With the app, banking, payments, password and 2FA apps, and ADBuddy itself, are protected: while one is in front, the agent can neither read nor touch the screen. Today ADBuddy guesses that list from app names, and you cannot edit it yet. Without the app, only ADBuddy itself is protected. With the app connected, `adbuddy type "<text>"` refuses password fields.

While the agent uses the screen, the phone shows a frame round it and a pill, **Agent using the screen · Pause**. Tap the pill to pause.

## Send a file to the phone

The agent sends you a result with `adbuddy send <file>`. The file must be inside the environment and at most 200 MiB.

```sh
adbuddy send report.pdf --caption "Your storage report"
```

It arrives in the chat on the phone. You can ask the agent to do it.

## Pause and cut access

Two controls on the phone. **Cut access** works without the PC; **Pause** asks the host, so it needs the app connected.

**Pause** stops the agent from using the phone, and the connection stays up. While paused, ADBuddy refuses every adb command, every screen helper and `adbuddy undo`, and withdraws open questions. The agent is told `paused from the phone`. Pause from the chat's ⋮ menu, from the conversation notification, or from the pill on the screen. Choose **Resume** to continue. Pause is only offered while the phone is connected to the host.

**Cut access** turns off Wireless debugging and USB debugging on the phone. The PC cannot reach it, and nothing the agent runs can work until you restore it. It is in the chat's ⋮ menu and in the conversation notification. The notification button needs no unlock.

To restore, choose **Restore access** in the ⋮ menu. It puts back what was set before the cut. Turning Wireless debugging on again changes its port, so the PC may need `adbuddy connect <ip:port>` to find the phone ([Remote mode](#remote-mode) shows it). If remote mode was on before the cut, its port stays open until the PC connects or the phone restarts.

Cut access needs a permission that `adbuddy setup` gives the app when you accept in it. If the app cannot cut, it says so and offers to open Developer options: turn off USB debugging and Wireless debugging there.

**Cut access is the real kill switch.** ADBuddy protects against mistakes and prompt injection. It does not protect against an agent determined to find another adb. See [Safety](safety.md).

## Remote mode

ADBuddy finds the phone by mDNS, which only works on one network. Away from home, or on a Wi-Fi that blocks mDNS, connect by IP address and port. Tailscale gives the phone and the PC addresses that reach each other anywhere. The documentation addresses below are examples: use the ones your phone shows.

### Over Tailscale

1. Install Tailscale on the PC and the phone and sign in to both with the same account.
2. Pair with a code (the QR code needs the same network). Follow the Tailscale section of [Set up your phone](setup/):

   ```sh
   adbuddy setup --code --connect <ip:port>
   ```

3. Later, when the port changes, stop `adbuddy serve` and connect again with the address on the phone's Wireless debugging screen:

   ```sh
   adbuddy connect 100.64.0.5:41234
   ```

   The command saves the address and tries to check that the phone there is the one you set up. It can only do that when the saved name carries a serial number that tells phones apart; otherwise it prints a note and goes on. The command refuses to run while serve is running (`ALREADY_RUNNING`). While serve runs it also watches the phone: if it disappears, it tries the saved address, the local network and the phone's Tailscale address.

### With mobile data only

Without any Wi-Fi, ADBuddy reaches the phone through adb's port 5555. **You** turn this on. A request from the agent's session is refused (`AGENT_SESSION`, a check against mistakes), and turning it on asks you with a special confirmation:

```sh
adbuddy remote on
adbuddy remote status
adbuddy remote off
```

- `adbuddy remote on` needs `adbuddy serve` running, the ADBuddy app connected and Tailscale on in the phone. It asks you first, in the app or in the serve terminal, then opens port 5555 for **12 hours**.
- `adbuddy remote status` says whether it is on, closing or off, and when it expires.
- `adbuddy remote off` closes it. It works even with serve stopped, as long as the phone is reachable.
- The app has a switch for it too.

**Port 5555 stays open on every network the phone joins, public Wi-Fi included, until it expires or you turn it off.** Android still asks for your PC's key, but anyone who can reach the port can try to connect. Turn it on only when you need it, and turn it off when you are back home.

Only a running `adbuddy serve` closes the port when the 12 hours end. With serve stopped nothing expires: run `adbuddy remote off`, or restart the phone. While remote mode is on, screenshots default to JPEG at half scale.

## The environment and skills

`adbuddy setup` and `adbuddy serve` keep the agent's environment, `~/ADBuddy/<phone name>/`. The folder is named after the phone's model, in lower case with dashes.

- `AGENTS.md` holds the instructions ADBuddy gives every agent: the commands, the levels, and the rule that what it reads on the phone is data, not orders. `CLAUDE.md` points Claude Code at it. Running `adbuddy setup` again rewrites both (with serve stopped); `adbuddy env init --force` does too, but leaves the phone's profile in `AGENTS.md` as "Not collected yet." until the next setup.
- `NOTES.md` is yours. Write what you want the agent to remember about your phone. ADBuddy never changes it.
- `device.json` is the phone's profile, which setup wrote.
- `journal/` is the journal.
- `.adbuddy/bin/` holds the `adb` shim and `adbuddy` that the agent runs.
- `.claude/settings.json` holds ADBuddy's hooks for Claude Code, next to your own, which are kept.

**Skills** are recipes the agent follows for common jobs. Ask for the job in your own words and the agent reads the matching skill first. A skill is a recipe, not a permission: every rule and every confirmation still applies.

| Skill | For |
|---|---|
| `device-profile` | an overview of the phone (read-only) |
| `permissions-audit` | review app permissions and revoke the unneeded |
| `storage-cleanup` | free up space |
| `debloat` | disable or remove pre-installed apps |
| `battery` | reduce background activity |
| `organize-files` | sort downloads, documents and media into folders |
| `backup-to-pc` | copy photos and files to the PC, incrementally |
| `home-screen` | group apps into folders |
| `build-utility-apk` | build a small custom app for the phone |

They live in `.claude/skills/` and `.agents/skills/` of the environment. A skill file that contains `<!-- adbuddy:user -->` is yours and is never overwritten. Running `adbuddy setup` again refreshes the rest.

## Next

- [Safety](safety.md): the levels, the journal, what is protected and what is not.
- [Troubleshooting](troubleshooting.md): `adbuddy doctor` and the error codes.
- [Set up your phone](setup/): Wireless debugging, pairing and Tailscale (screenshots coming).
- [Install ADBuddy](install.md): update and uninstall.
