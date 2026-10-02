---
title: FAQ
layout: default
lang: en
---

# FAQ

Short answers. Each one points to the guide that has the detail.

## Do I need a USB cable?

No. ADBuddy uses Android's Wireless debugging, so the phone and the PC only need the same Wi-Fi network (or Tailscale). You turn Wireless debugging on once in Developer options, then pair with a QR code or a pairing code. See [Set up your phone](setup/) and [Install ADBuddy](install.md).

## Does it work with my agent?

ADBuddy supports Claude Code, Codex, Gemini CLI and OpenCode, and recommends Claude Code. It does not install agents: it starts the one you already have, and tells you how to install it if it is missing. With Claude Code the phone's chat shows the agent's answers as chat bubbles; with the others it shows the agent's terminal. See [Use ADBuddy](usage.md#talk-to-the-agent).

## What does the agent see on my phone?

What adb lets it read: the apps installed, settings, files in shared storage, the phone's logs, what is on the screen (the text of its elements, and screenshots) and, through content providers, what the phone's shell may read, such as contacts and call history (it depends on the phone). Reading is green, so it does not ask you. ADBuddy's screen helpers never carry the text of password fields, and, with the app, nothing touches or reads the screen while a protected app is in front (until the app has sent a list, only ADBuddy itself is protected; after that the PC keeps the last list it got). What it reads goes into its conversation, and so to its model provider. See [Safety](safety.md#the-three-levels) and [What ADBuddy does not guarantee](safety.md#what-adbuddy-does-not-guarantee).

## Can it delete my photos?

Deleting is a red command (`rm`), so it waits for your answer, and you can say no. If you allow it, it is done: ADBuddy cannot undo a delete. Moving a photo to a new path is yellow: it runs and is journaled with its undo; moving it into a folder that already exists asks for confirmation. The agent can also delete through an app's screen with taps, and taps are not confirmed. Keep a backup of what matters, and read each red question before you allow it. See [Safety](safety.md#the-three-levels) and [Confirmations](safety.md#confirmations).

## How do I undo something?

Run `adbuddy undo` to undo the most recent change that has an inverse, `adbuddy undo session` for the whole session, or `adbuddy undo <id>` for one entry from `adbuddy journal`. Some things have no undo: red commands you allowed, taps and typing, and app updates. See [Use ADBuddy](usage.md#look-back-and-undo) and [Safety](safety.md#the-journal-and-undo).

## Does it work away from home?

Yes, with a little more work. Tailscale gives the PC and the phone addresses that reach each other on any network: pair with `adbuddy setup --code --connect <ip:port>` and reconnect with `adbuddy connect <ip:port>`. With only mobile data, remote mode (`adbuddy remote on`) opens adb's port 5555 for 12 hours, and that port is open on every network the phone joins. See [Remote mode](usage.md#remote-mode).

## What if I lose my phone or my PC?

If you lose the PC, tap **Cut access** in the app, or turn off both USB debugging and Wireless debugging (turning off only Wireless debugging does not close port 5555). If remote mode was on, restart the phone. Then use **Revoke USB debugging authorizations**, forget the PC under **Paired devices** and remove it from your Tailscale network. If you lose the phone, ADBuddy adds no other way in: lock it and erase it with your phone's own tools. The journal on the PC lists what the agent ran. See [Pause and cut access](usage.md#pause-and-cut-access) and [Uninstall](install.md#uninstall).

## Why doesn't it ask for my PIN?

It has no need to. ADBuddy's helpers do not type into password fields when the app is connected, the agent's instructions forbid asking for a PIN or a password, and Android asks for its own when it must. To allow a red command from the phone, you unlock the phone yourself. See [Safety](safety.md#protected-apps-and-the-screen).

## How much does it cost?

ADBuddy is free and open source, under the Apache-2.0 licence. The agent you use with it may have its own cost (a subscription or an API plan): ADBuddy has no model of its own and needs no API key. See [Install ADBuddy](install.md).

## Is it safe?

It lowers the risk of letting an agent drive your phone: it ranks every command, journals changes with their undo, asks you before the adb commands it knows are irreversible (taps and typing are not asked about) and has a Pause and a Cut access button. It does not make it risk-free. It protects against mistakes and prompt injection, not against an agent determined to find another adb, and the real guarantee is **Cut access**. Read [Safety](safety.md), including the part about what it does not guarantee.

## Does it collect telemetry?

No. ADBuddy has no telemetry, no analytics and no server of its own. During `adbuddy setup` the host downloads Google's platform-tools from `dl.google.com`, and, once there is a release, the phone app from GitHub. Your agent talks to its own provider, which ADBuddy does not control. See [Privacy](safety.md#privacy).

## Is it from Google, Anthropic or OpenAI?

No. ADBuddy is an independent open source project. It is not made, endorsed or supported by Google, Anthropic or OpenAI. Android is a trademark of Google; Claude Code, Codex and Gemini CLI belong to their makers.

## Next

- [Safety](safety.md): what ADBuddy does, and what it does not guarantee.
- [Use ADBuddy](usage.md): the day-to-day guide.
- [Troubleshooting](troubleshooting.md): `adbuddy doctor` and the error codes.
- [Install ADBuddy](install.md): build from source, update and uninstall.
