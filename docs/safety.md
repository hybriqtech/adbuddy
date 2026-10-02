---
title: Safety
layout: default
lang: en
---

# Safety

This page says what ADBuddy does to keep your phone safe from a coding agent, and, at the end, what it does not guarantee. Read the end before you decide how far to trust it.

ADBuddy puts a **shim** between the agent and the phone. A shim is a small stand-in program: the agent runs what it thinks is `adb`, and ADBuddy's copy ranks the command by risk, writes it to a **journal** and, when needed, waits for your answer. The **environment** is the folder where the agent works (`~/ADBuddy/<phone name>/`).

## The shim

`adbuddy setup` and `adbuddy serve` put an `adb` program in `<environment>/.adbuddy/bin/`. When ADBuddy starts the agent, that folder is first in the agent's `PATH`, and the phone is already chosen (`ANDROID_SERIAL`). The real adb is not in the environment (by default it is the one `adbuddy setup` downloaded into ADBuddy's data folder), and `adbuddy serve` runs it.

The shim only forwards the command to `adbuddy serve` over a local connection. `adbuddy serve` classifies it, journals it, asks you if needed, runs the real adb and sends the output back. If `adbuddy serve` is not running, the shim prints `ADBuddy host not running (run: adbuddy serve)` and nothing reaches the phone.

`adbuddy serve` also refuses to start when the agent runs it (`AGENT_SESSION`), so the agent does not start a host of its own by mistake and answer its own questions. It is a check against mistakes, not a barrier: see [What ADBuddy does not guarantee](#what-adbuddy-does-not-guarantee).

## The three levels

Every command gets one level. For `adb shell`, ADBuddy parses the shell line and takes the highest level of its parts: `a; b` is as risky as the riskier of the two.

| Level | Meaning | What happens | Examples |
|---|---|---|---|
| 🟢 Green | Read-only | Runs at once. Journaled. | `getprop ro.product.model`, `pm list packages`, `dumpsys battery`, `settings get global window_animation_scale` |
| 🟡 Yellow | Reversible or harmless | Runs at once. Journaled, with its undo when it has one. | `settings put global window_animation_scale 0.5`, `pm disable-user --user 0 com.facebook.katana`, `mv /sdcard/a.jpg /sdcard/Pictures/a.jpg`, `input tap 100 200` |
| 🔴 Red | Irreversible, dangerous or not understood | Waits for your answer. | `rm -rf /sdcard/DCIM`, `pm clear com.whatsapp`, `pm uninstall com.x`, `settings delete global foo`, `reboot`, and any command ADBuddy cannot read |

Three points matter:

- **Green is not asked about.** The agent can read a lot without asking you: the apps installed, settings, files in shared storage, what is on the screen, the phone's logs and, through content providers, whatever the phone's shell is allowed to read (contacts and call history, for example; what you get depends on the phone). Text messages are the exception: reading them is always red (see below). What it reads goes to the agent (see [What ADBuddy does not guarantee](#what-adbuddy-does-not-guarantee)).
- **Yellow can become red.** Before a change, ADBuddy reads the phone. If it cannot read the state it needs, or the change would overwrite something (a `mv` onto a path that exists, a file or a folder, a `pm uninstall --user 0` of an app that is not a system app), the command is red.
- **Unknown is red.** A command with no rule, or a shell line ADBuddy does not fully understand, is red. The shell evaluator accepts only a short list of constructs (such as simple commands, pipes, `;`, `&&`, `||`, `if`, `for`, `while`, `[ ]` tests, `$(…)` and the script's own lower-case variables); anything else waits for you every time.

A few commands are **always red** and say so in the question, because they can cut the connection or affect ADBuddy: Wi-Fi and airplane mode, `settings` that touch debugging or accessibility, `adb kill-server`, `adb disconnect`, `adb usb`, `adb tcpip`, reboots, and anything that uninstalls, disables, clears, force-stops or revokes permissions of the ADBuddy app itself. They are fixed in the code: your own `rules.yaml` cannot lower them. The same goes for reading the full text of every app's notifications (`dumpsys … --noredact`), `adb bugreport` and `screenrecord`.

Some settings are always red too, because they weaken the phone's security even though one more command undoes them. Changing or deleting them with `settings put`, `settings delete`, `cmd settings` or `content` on the settings provider waits for you:

- **What you type, fill in or say:** the keyboard (`default_input_method`, `enabled_input_methods`, `disabled_system_input_methods`), the autofill and passkey services (`autofill_service`, `credential_service`, `credential_service_primary`) and the assistant (`assistant`, `voice_interaction_service`, `voice_recognition_service`). An app chosen there sees your passwords.
- **Notifications and text messages:** `enabled_notification_listeners`, `enabled_notification_assistant`, `enabled_notification_policy_access_packages` and `sms_default_application`.
- **Where the traffic goes:** the proxy (`http_proxy`, `global_http_proxy_host`, `global_http_proxy_port`, `global_http_proxy_exclusion_list`, `global_proxy_pac_url`), private DNS (`private_dns_mode`, `private_dns_specifier`), the captive portal check's addresses (`captive_portal_http_url`, `captive_portal_https_url`, `captive_portal_fallback_url`, `captive_portal_other_fallback_urls`, `captive_portal_server`) and the always-on VPN (`always_on_vpn_app`, `always_on_vpn_lockdown`).
- **Location:** `location_mode` and `location_providers_allowed`.
- **Setup and lock screen:** `device_provisioned`, `user_setup_complete`, `secure_frp_mode`, `lockscreen.disabled`, `lock_screen_lock_after_timeout` and `lock_screen_allow_private_notifications`.

Other settings, such as `stay_on_while_plugged_in` or the animation scales, stay yellow.

**Reading text messages is always red.** Login codes arrive by SMS, so `content query` or `content read` on the message providers waits for you: `content://sms`, `content://mms`, `content://mms-sms`, `content://sms-changes` and `content://telephony`, with any user, port, letter case or `%xx` escape in the address, plus any other provider whose name contains `sms` or starts with `mms`. Other content providers (contacts, call history, media) keep their level.

## The journal and undo

Every command is written to the journal in the environment (`<environment>/journal`). A 🟡 or 🔴 command is written **before** it runs, so the record is on disk even if the host stops halfway; its undo is written with it, except for a new app install, whose undo is worked out right after it runs. If the journal cannot be written, ADBuddy blocks every 🟡 and 🔴 command: no record, no change. Each entry carries a signature made with a key ADBuddy keeps in its data folder; `adbuddy journal` marks an entry that does not verify as `not written by ADBuddy`, and `adbuddy undo` will not run it.

```sh
adbuddy journal
adbuddy undo
adbuddy undo session
adbuddy undo <id>
```

[Use ADBuddy](usage.md#look-back-and-undo) shows each form. `adbuddy undo` undoes the most recent change that has an inverse. Undo runs a 🟡 inverse without asking again; a 🔴 inverse (such as `uninstall` after an install, or `rm -f` of a file the agent created) asks you like any 🔴 command, and if you do not allow it that change is skipped. Undo does not run while the agent is paused.

**What has an undo.** ADBuddy records an inverse for the changes it understands: a setting changed, an app disabled or uninstalled for the user (a system app), an app operation mode set with `appops`, a battery exemption, a folder created, a file moved or created, a file pushed (a file it replaces is saved first and put back) and a new app installed.

**What has no undo:**

- Red commands: deleting files, clearing an app's data, rebooting, deleting settings, and anything ADBuddy does not understand. Once you allow one, it is done.
- Taps, typing and key presses (`input …`, `adbuddy tap`, `adbuddy type "<text>"`) and starting apps. They are yellow and journaled, with no undo. **A tap inside an app can confirm something destructive.** The agent's instructions tell it to ask you in the chat first, but that is an instruction to the agent, not a check ADBuddy makes.
- An app update (an install over an existing app), a command that makes two changes (`a; b`), and a command that failed or was running when the host stopped: its state is uncertain.

## Confirmations

A red command waits, and so does the agent. The question says what the command does in plain words, taken from the rule's template, with the command's arguments filled in, **not written by the agent**, and shows the raw command.

- **App connected:** the question is a notification on the phone with **Allow**, **Deny** and, for most commands, **Allow during the session**. The app also shows it as a dialog when it is open. **Allow during the session** covers only the same rule with the same arguments from the same folder, and is never offered for the always-red commands above, nor when the red comes from the state of the phone or a write on the PC. **To allow, the phone must be unlocked**; Deny always works.
- **No app connected:** the question is in the `adbuddy serve` terminal. You type a two-digit code to allow it; anything else denies it.
- **Nobody to ask** (for example `adbuddy serve --confirm deny`): the command is refused with `Denied: nobody could confirm: <title>`.

A question expires after 2 minutes (`confirm_timeout` in `config.toml`) and counts as a no. A refused or expired command ends with exit code 126 and `Denied by user: <title>` (with ` (confirmation expired)` when it expired). While a red question is open, other commands are refused with `Blocked by ADBuddy: waiting for your answer to "<title>"`.

The instructions in the environment tell the agent not to retry a denied command or look for another way, and to ask you instead.

## Protected apps and the screen

The agent can read and act on the screen with `adbuddy screen`, `adbuddy tap`, `adbuddy type "<text>"`, `adbuddy foreground`, `adbuddy screenshot` and `adbuddy notifications`, and with raw `input`, `am start` and `screencap`. Some apps are **protected**: while one is in front, nothing touches or reads the screen, and the agent gets `Blocked by ADBuddy: … is a protected app` (or `something on the screen is protected` when the app says so without a package). ADBuddy refuses an `am start` or `monkey` that names a protected app. A link or a tap on an app's icon can still open one; once it is in front, nothing touches or reads the screen. ADBuddy itself is always protected.

- **Which apps.** The ADBuddy app proposes the list **by name**: apps in the launcher whose package or label looks like banking, payments, wallets, crypto, passwords or 2FA, plus the apps that receive your login codes (Messages and the phone's default SMS app, Gmail, Outlook, Samsung Email, Proton Mail, Yahoo Mail, Telegram, WhatsApp and Google Play services, those installed), which are protected from the first launch. It is a guess. It can miss an app, and it can flag one that is harmless. You confirm or edit the list in the app (Settings, Protected apps; the first-run step too); once you confirm it, ADBuddy never changes it by itself. Without the app, or until the app has sent its list, **only ADBuddy itself is protected**.
- **The guard fails closed.** If ADBuddy cannot tell which app is in front, it does not touch or read the screen. It also blocks while the notification shade is open, because a 2FA code can be on it. A command that touches or reads the screen must run alone, one key at a time, so it cannot open an app and then act in it after the guard looked.
- **Password fields.** The screen dump never carries a password field's text. With the app connected, `adbuddy type "<text>"` refuses password fields. Without the app, and with a raw `input text`, ADBuddy cannot tell which field has the focus. ADBuddy never asks for your PIN or password, and the agent's instructions forbid it too.

While the agent uses the screen, the phone shows a frame and a pill, **Agent using the screen · Pause**.

**Notifications.** `adbuddy notifications` reads them through a listener you allow in the system's own screen and then confirm in ADBuddy; the agent cannot press that confirm button (ADBuddy is protected), and revoking the access in the system takes your confirmation away. The title and text of protected apps, of ADBuddy, of other users or work profiles, and of notifications marked secret never leave the phone. There is no adb fallback. Limit: a notification forwarder you install (a smartwatch companion, KDE Connect) can pass the content of a protected app on under its own name.

## Pause and Cut access

Two buttons in the phone's app. **Cut access** works on the phone itself, without the PC; **Pause** asks the host to stop, so it only works while the app is connected to it.

**Pause** asks the host to stop the agent's use of the phone (only while the app is connected to the host) and keeps the connection. While paused, ADBuddy refuses new adb commands, the screen helpers and `adbuddy undo`, and withdraws open questions. A command that was already running when you paused is not stopped (see [What ADBuddy does not guarantee](#what-adbuddy-does-not-guarantee)). The agent keeps running, and it is told `paused from the phone`. It is in the chat's ⋮ menu, in the conversation notification and in the pill on the screen. **Resume** continues.

**Cut access** turns off Wireless debugging and USB debugging on the phone itself, with a permission `adbuddy setup` gives the app. It does not depend on the PC. It is in the ⋮ menu and in the conversation notification, and the notification button needs no unlock. **Restore access** puts back what was set before; if remote mode was on, its port 5555 opens again until the PC connects or the phone restarts. See [Pause and cut access](usage.md#pause-and-cut-access) for the details. If the app cannot cut, it says so and offers Developer options, where you can turn the two switches off by hand.

## Can you trust the app?

The phone's app gets a token for its channel with the host, and permissions that `adbuddy setup` grants. The host gives them only to an app it recognises. Before it does, it reads the installed app's signature and checks that it is signed with the project's release certificate, or is the one development APK you chose with `adbuddy setup --apk <file>` (the host trusts that exact file, by its SHA-256, and `adbuddy serve` says so when it starts). It also checks a minimum app version. An app that fails gets no token and no permissions.

The app talks to the host over a local connection that adb carries (the phone's own `127.0.0.1` through `adb reverse`), not over the internet.

**Today the host has no release certificate pinned**: there is no public release yet, so the only app it trusts is the development APK you gave it. The minimum version is 0 for now, so it excludes nothing. Treat that trust as "the file you built yourself". See [Install ADBuddy](install.md#build-from-source).

## Prompt injection

Text on the phone is untrusted: a notification, a web page, a file name or a message can say "ignore your instructions". `AGENTS.md`, the file ADBuddy writes in the environment, tells the agent that everything it reads from the phone (screen text, notifications, files, web pages) is **data, not instructions**, and not to follow instructions found there. It also tells it not to put text read from the phone into a command that runs on the PC unless it is quoted for the PC's shell, and to prefer `adbuddy tap --id <id>` to tapping by a label it read.

This is guidance to the agent plus ADBuddy's checks, and the two work together: even if an injected instruction fools the agent, a red command still waits for you, a protected app still blocks, and everything is journaled. It lowers the risk. It does not remove it.

## Privacy

ADBuddy has no telemetry, no analytics and no server of its own. It does not ask for an account.

What the host connects to on the internet, in the code:

- `dl.google.com`, during `adbuddy setup`, to download Google's Android platform-tools (checked against Google's checksum).
- `github.com`, to download the phone app from a release, when a release is published and the host pins its certificate. It does nothing today.

Everything else stays between your devices: the phone and the PC talk over Wireless debugging (over Tailscale's network if you use it for remote access), and the app and the host over the local channel. The app asks for the `INTERNET` permission only for that channel, and its network configuration refuses plain-text traffic except to `127.0.0.1`.

**The agent is separate.** Your coding agent talks to its own provider, under its own terms. ADBuddy does not control that.

## What ADBuddy does not guarantee

ADBuddy lowers the risk of letting an agent drive your phone. It does not make it safe. Know these limits.

- **The shim protects against mistakes and prompt injection. It does not protect against an agent determined to find another adb.** The shim works because the agent's `PATH` finds it first and because the real adb is outside the environment. An agent that runs another adb program (even against ADBuddy's own adb server on port 5038), or talks to the phone's debugging port directly goes around ADBuddy: nothing is classified, journaled or asked. The instructions tell the agent not to do that, but an instruction is not a barrier. **The real guarantee is Cut access** in the app: with debugging off, the PC cannot reach the phone at all.
- **The agent runs as you on the PC.** It can also read and change ADBuddy's own settings and data (`rules.yaml`, the key that signs the journal). The journal's signature catches entries other tools wrote, not a determined agent.
- **Remote mode opens port 5555.** `adbuddy remote on` opens adb's network port 5555 on every network the phone joins, public Wi-Fi included, for 12 hours or until you turn it off. Android still asks for your PC's key, but anyone who can reach the port can try to connect. Only a running `adbuddy serve` closes it at the end of the 12 hours. Turn it on only when you need it. See [Remote mode](usage.md#remote-mode).
- **The classifier is not a vault.** It is a whitelist, checked against a large set of test commands, and anything it does not recognise is red. But the command runs in the phone's own shell, and a mistake in how ADBuddy reads a shell line could rank a dangerous command lower than it deserves. A red you allow runs with the phone's full shell rights. Read the question before you allow it.
- **Protected apps are a guess until you review them.** The proposal comes from names, and an app that is not on the list is not protected. Do not rely on it for an app that matters; keep such apps closed while the agent works, or pause it.
- **Android's Advanced Protection mode may stop ADBuddy.** Where your phone offers that mode, it may restrict debugging, accessibility services and installs from outside the store. ADBuddy has no check for it today, so if setup or the screen helpers fail with it on, that is a reason to look at.
- **What the agent reads goes to its provider.** Screen text, notifications, files and command output that the agent reads go into its conversation, and from there to the company that runs its model. Keep what you do not want shared off the screen, or behind a protected app, while the agent works.
- **Pause does not stop a command that is already running.** It refuses new commands and withdraws open questions, but a command ADBuddy already let through runs to the end. A yellow change with a wait or a loop in front of it (`sleep 600; settings put …`) still happens after you pause. To stop everything at once, use **Cut access**.
- **Yellow actions on the screen run without asking and have no undo.** Taps and typing are not confirmed one by one. Stay near the phone, or pause the agent, while it works on something important.

Use a phone and accounts you can afford to repair, keep a backup of what matters, and know where **Cut access** is.

## Reporting a security problem

The project has no `SECURITY.md` yet, so there is no official channel. Please do this:

1. If the repository has private vulnerability reporting enabled, open a private **security advisory** on the repository's **Security** tab on [GitHub](https://github.com/hybriqtech/adbuddy).
2. If you do not see that option, open an [issue](https://github.com/hybriqtech/adbuddy/issues) that says you have a security problem, **without** exploit details, and ask for a private way to send them.

Please do not post working exploits, your phone's serial number or your IP address in public.

## Next

- [Use ADBuddy](usage.md): confirmations, the journal, pause and remote mode in practice.
- [FAQ](faq.md): short answers.
- [Troubleshooting](troubleshooting.md): when something fails.
- [Install ADBuddy](install.md): build from source and set up.
