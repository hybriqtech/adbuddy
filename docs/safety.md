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

The levels decide what waits for you, unless you turn on [YOLO mode](#yolo-mode).

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

The redacted notification dump is red too, but your `rules.yaml` can lower it: even without `--noredact`, Android keeps the first 8 characters of each notification's ticker, and a verification code often starts it (`tickerText=G-123456...`). That covers `dumpsys notification` with any argument. Any `dumpsys` of several services is red in the same way, whatever it leaves out (plain `dumpsys`, any `--skip`, any `--priority`): on the CI's emulator, after other tests, `dumpsys --skip notification` and `dumpsys --priority NORMAL`, which leave the notification service out, showed the code of a notification's text. Where it came from is not confirmed (another service that held it, or a 6-digit number turning up by chance in 15 MB of dump), so no dump of several services is trusted. A single other service, `dumpsys -l`, `dumpsys --help` and `--pid`, `--thread`, `--clients` or `--stability` (without `--dump`, they never call a service's dump) stay green; a repeated `--priority` and an empty service name are red. Those green forms were measured only on the emulator's AOSP API 34, with a notification posted and after it was canceled: other Android versions or vendor layers may differ, and they are measured again with `TestNotificationReadsAndTheCodes` (`host/internal/emutest`, which also dumps each service on its own and fails naming any service that shows the notification while `dumpsys` of it is green) whenever the image changes.

So is a command whose commands come through stdin: `adb shell` or `adb exec-out` without a command (an interactive shell, with or without `-t`), `adb exec-in`, `adb abb_exec` and `adb abb` (the same for a system service: `adb install` streams the APK this way), and a shell, `su` or `eval` on the phone that reads its script from stdin. ADBuddy forwards the agent's stdin to adb but does not read it, so it cannot tell you what would run.

Some settings are always red too, because they weaken the phone's security even though one more command undoes them. Changing or deleting them with `settings put`, `settings delete`, `cmd settings` or `content` on the settings provider waits for you:

- **What you type, fill in or say:** the keyboard and the spell checker (`default_input_method`, `enabled_input_methods`, `disabled_system_input_methods`, `selected_spell_checker`), the autofill and passkey services (`autofill_service`, `credential_service`, `credential_service_primary`) and the assistant (`assistant`, `voice_interaction_service`, `voice_recognition_service`), and the accessibility service a shortcut or the accessibility button turns on (`accessibility_shortcut_target_service`, `accessibility_button_targets`). An app chosen there sees your passwords.
- **Notifications and text messages:** `enabled_notification_listeners`, `enabled_notification_assistant`, `enabled_notification_policy_access_packages` and `sms_default_application`.
- **Where the traffic goes:** the proxy (`http_proxy`, `global_http_proxy_host`, `global_http_proxy_port`, `global_http_proxy_exclusion_list`, `global_proxy_pac_url`), private DNS (`private_dns_mode`, `private_dns_specifier`), the captive portal check's addresses (`captive_portal_http_url`, `captive_portal_https_url`, `captive_portal_fallback_url`, `captive_portal_other_fallback_urls`, `captive_portal_server`) and the always-on VPN (`always_on_vpn_app`, `always_on_vpn_lockdown`).
- **Location:** `location_mode` and `location_providers_allowed`.
- **Setup and lock screen:** `device_provisioned`, `user_setup_complete`, `secure_frp_mode`, `lockscreen.disabled`, `lock_screen_lock_after_timeout` and `lock_screen_allow_private_notifications`.

Other settings, such as `stay_on_while_plugged_in` or the animation scales, stay yellow.

The same goes for the commands that do it without `settings`: `ime enable` and `ime set` (and `cmd input_method`), `cmd notification allow_listener`, `allow_assistant` and `allow_dnd`, `cmd role add-role-holder`, `clear-role-holders` and `set-bypassing-role-qualification`, `dpm set-active-admin`, `set-device-owner` and `set-profile-owner`, and `locksettings set-disabled`, `clear`, `set-pin`, `set-password` and `set-pattern`. So does `appops set <app> PROJECT_MEDIA allow` (or `foreground`), which lets an app record the screen without Android asking you; denying it stays yellow. Other app ops stay yellow, `SYSTEM_ALERT_WINDOW` (drawing over other apps) and `MANAGE_EXTERNAL_STORAGE` (all files) included.

**Reading text messages is always red.** Login codes arrive by SMS, so `content query` or `content read` on the message providers waits for you: `content://sms`, `content://mms`, `content://mms-sms`, `content://sms-changes` and `content://telephony`, with any user, port, letter case or `%xx` escape in the address, plus any other provider whose name has a part that contains `sms` or starts with `mms`. Other content providers (contacts, call history, media) keep their level.

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

With [YOLO mode](#yolo-mode) on, red commands run without asking.

A red command waits, and so does the agent. The question says what the command does in plain words, taken from the rule's template, with the command's arguments filled in, **not written by the agent**, and shows the raw command.

- **App connected:** the question is a notification on the phone with **Allow** and **Deny**, and **Open**. The app also shows it as a dialog when it is open (or when you tap the notification), which for most commands adds **Allow this same command during the session**: it is never offered on the notification itself, which shows only the first lines. **Allow this same command during the session** covers only the same rule with the same arguments from the same folder, and is never offered for the always-red commands above, nor when the red comes from the state of the phone or a write on the PC, nor for a command whose effect may come from its stdin: the same command with other input could do anything, so it is asked every time. That is decided by an allowlist: a command keeps **Allow this same command during the session** only if every program on it is known not to read stdin (`rm`, `settings`, `reboot`, `am`, `pm` other than its installs…) or to read it only as data it filters to its output (`cat`, `grep`, `head`, `tail`, `wc`, `sort`, `cut`, `tr`) without writing it to a file. An interactive shell, `exec-in`, `abb_exec`, `pm install` and `install-write`, `awk`, `sed`, a shell, `xargs`, `read`, a `$(…)`, `dd`, `tee`, a `-` or `/dev/stdin` operand, or any program ADBuddy does not know, is asked every time. The notification puts the ⚠ warning first and shows the first lines of the detail and of the command; for an always-red command, or a command longer than the notification shows, it offers only **Deny** and **Open**, and you allow it from the dialog, which shows everything and enables **Allow** once you have scrolled to the end. Spaces other than the ordinary one, invisible fillers and combining marks are shown escaped (`\u00a0`), and long runs of blanks, of one kind or mixed, are compacted (`␠×N`, or `×N(␠×a \u00a0×b)` for a mix), so padding cannot hide the end of the command. A command longer than the dialog shows (20 000 characters) can only be denied: nobody can have read its end. ADBuddy on the PC holds these limits whatever the app offers: no **Allow this same command during the session** for a command longer than the notification shows (300 characters), and an **Allow** of a command longer than the dialog shows counts as **Deny**. **To allow, the phone must be unlocked**; Deny always works. On Android 12 and later, if you tap **Allow** on the notification while the phone is locked, you type the PIN and the button completes by itself, with no second tap (**Reply** and **Send screenshot** also complete after the PIN): ADBuddy waits up to 2 seconds for the phone to report itself unlocked, and only then decides again. On Android 11 you unlock first and tap again. An **Allow** that arrives while the phone is locked and before its button has been shown for 600 ms is dropped at once. **Reply** and **Send screenshot** have no such arming: they always wait up to 2 seconds for the unlock. Accepted residual risk, which applies equally to **Reply** and **Send screenshot**: a phone maker's system UI that ignores the authentication requirement, together with you unlocking the phone by another way within those 2 seconds of a tap that came after the button was armed. **Allow** and **Allow this same command during the session** wait 600 ms from when the question appears, in the dialog or in the notification, and wait again when the screen turns on, and in the dialog also when the app comes back to the front or the question can be answered again (after a lost connection): a stray tap or a burst of taps as it pops up allows nothing. **Deny** never waits.
- **No app connected:** the question is in the `adbuddy serve` terminal. You type a two-digit code to allow it; anything else denies it.
- **Nobody to ask** (no app connected and no terminal: `adbuddy serve --confirm deny`, or `auto` without an interactive terminal): the command is refused with `Denied: nobody could confirm: <title>`. `--confirm` only chooses this fallback: while the app is connected, the question is always asked on the phone, even with `--confirm deny`. A question waiting on the phone when the app disconnects moves to the fallback, and is refused the same way with `deny`.

A question expires after 2 minutes (`confirm_timeout` in `config.toml`) and counts as a no. A refused or expired command ends with exit code 126 and `Denied by user: <title>` (with ` (confirmation expired)` when it expired). While a red question is open, other commands are refused with `Blocked by ADBuddy: waiting for your answer to "<title>"`.

The instructions in the environment tell the agent not to retry a denied command or look for another way, and to ask you instead.

## Protected apps and the screen

The agent can read and act on the screen with `adbuddy screen`, `adbuddy tap`, `adbuddy type "<text>"`, `adbuddy foreground`, `adbuddy screenshot` and `adbuddy notifications`, and with raw `input`, `am start` and `screencap`. Some apps are **protected**: while one is in front, nothing touches or reads the screen, and the agent gets `Blocked by ADBuddy: … is a protected app` (or `something on the screen is protected` when the app says so: also for about 8 seconds after any notification arrives, as it may show a code, and while a window ADBuddy cannot read is on screen; the agent waits and tries once more). ADBuddy refuses an `am start` or `monkey` that names a protected app. A link or a tap on an app's icon can still open one; once it is in front, nothing touches or reads the screen. ADBuddy itself is always protected. While it is in front, the agent can only leave it, so that it can go on after you write to it from the phone: by pressing Home alone, or by opening another app by its package name with `monkey` when the ADBuddy app reports the screen. It never taps or types into ADBuddy's screen. Reading it has two known limits: an app whose first screen is a dialog or see-through leaves ADBuddy visible behind it, and the Recents view shows a thumbnail of ADBuddy and of the protected apps that do not hide theirs, so a screenshot can show them. When ADBuddy cannot read some or all of a screen, `adbuddy screen` says so (`unreadable` in `--json`) and does not claim why: it can be the app hiding its content, or the screen still changing; `adbuddy screenshot` may still show it.

While the agent reads the screen (`adbuddy screen`, `adbuddy screenshot`, and raw `screencap`, `screenrecord`, `uiautomator dump` or `dumpsys` that dumps the view), the app shows its frame and "Agent using the screen · Pause" pill first. If the accessibility service is on but the app cannot draw it, the read is blocked (`Blocked by ADBuddy: …`); with the service off or no app connected there is nothing to show and the read goes ahead. `screenrecord` shows it only when it starts. `adbuddy screenshot` takes the frame and the pill off the screen for a moment while it captures, so they do not cover the image, and puts them back right after; the notice is up for at least 400 ms before that. If it cannot take them off, the picture still comes out and says so: `noticeInImage` in `--json`, or a note on stderr.

- **Which apps.** The ADBuddy app proposes the list **by name**: apps in the launcher whose package or label looks like banking, payments, wallets, crypto, passwords or 2FA, plus the apps that receive your login codes (Messages and the phone's default SMS app, Gmail, Outlook, Samsung Email, Proton Mail, Yahoo Mail, Telegram, WhatsApp and Google Play services, those installed), which are protected from the first launch. It is a guess. It can miss an app, and it can flag one that is harmless. You confirm or edit the list in the app (Settings, Protected apps; the first-run step too); once you confirm it, ADBuddy never changes it by itself. Until the app has ever sent a list (or if its saved copy cannot be read), **only ADBuddy itself is protected**; after that the PC keeps the last list it got (also across restarts) until the app sends a new one. Changes you save in the app reach the PC at once when the app is connected; if it is not, the PC keeps the previous list until it connects, so an app you have just protected is not protected on the PC until then. If the PC could not write its copy of the list (`protected.json`), after a restart the previous list comes back until the app sends its own.
- **The guard fails closed.** If ADBuddy cannot tell which app is in front, it does not touch or read the screen. It also blocks while the notification shade is open, because a 2FA code can be on it. A command that touches or reads the screen must run alone, one key at a time, so it cannot open an app and then act in it after the guard looked.
- **Password fields.** The screen dump never carries a password field's text. With the app connected, `adbuddy type "<text>"` refuses password fields. Without the app, and with a raw `input text`, ADBuddy cannot tell which field has the focus. ADBuddy never asks for your PIN or password, and the agent's instructions forbid it too.

While the agent uses the screen, the phone shows a frame and a pill, **Agent using the screen · Pause**.

**Notifications.** `adbuddy notifications` reads them through a listener you allow in the system's own screen and then confirm in ADBuddy; the agent cannot press that confirm button (ADBuddy is protected), and revoking the access in the system takes your confirmation away. So does every new start of the listener: after a reboot, an update of ADBuddy or the system killing it, you confirm again in ADBuddy before the agent can read. The title and text of protected apps, of ADBuddy, of other users or work profiles, and of notifications marked secret never leave the phone. There is no adb fallback. Limit: a notification forwarder you install (a smartwatch companion, KDE Connect) can pass the content of a protected app on under its own name.

## YOLO mode

YOLO mode lets the agent work on your phone without asking you. **With YOLO on, a destructive command (a factory reset, an uninstall, deleting files) runs without asking, and nothing waits for your answer.** Use it only when you are watching, or when you accept that risk.

- **Only you turn it on, and only from the app.** The agent cannot: nothing in the agent's path (the shim, the other commands of `adbuddy`) has an "on". The command line has only `adbuddy yolo off` and `adbuddy yolo status`, `adbuddy yolo on` does not exist (the command line answers with a usage error and sends nothing), and a request that reaches the host over IPC with any verb other than `off` or `status` is refused with `YOLO_ONLY_FROM_THE_PHONE`. In the app the notice lists what YOLO skips, you must choose how long it lasts (15 minutes, 1 hour or until `adbuddy serve` ends), and its button stays disabled for a moment so a stray tap cannot confirm it. The last step is to prove it is you:
  - **Strong fingerprint or face.** If the phone has them, the app asks for one (no PIN fallback). A tap cannot replace the fingerprint or the face itself (see the Known limits below for the face "confirm" button).
  - **The screen lock.** If the phone has no strong biometrics, the app asks for the PIN, pattern or password. **Known risk:** with YOLO already on, a script left running by the agent could watch you type a PIN (`getevent`, `screenrecord`) and use it to turn YOLO on again after it expires. Strong biometrics avoid this; if you do not have them, you accept it.
  - **No secure screen lock.** If the phone has none, the app shows a specific warning and, after its own short wait, turns YOLO on. You accept that a tap injected by the agent could finish the activation.
- **What it skips:** every confirmation (red commands, the confirmation of the inverse of an `adbuddy undo`, and the question of another command that is still waiting for your answer), adb options that would reach another adb server (the command runs through ADBuddy's own), a command that names another phone (it runs on the active phone, the only one the host reaches), a failure of the host to show the on-screen notice (the app's own notice still blocks if the app cannot draw it, for example without the permission to display over other apps), the journal if it cannot be written (the command runs without a record and cannot be undone), the protected-apps list you set, and what the host itself sees on the screen (the notification shade, a pop-up window), which would otherwise block it, and the permission questions of Claude Code (they are answered `allow`, never `allow_always`).
- **What it does not skip:**
  - **Pause.** Pausing the agent from the phone stops it and also turns YOLO off.
  - **The ADBuddy app itself.** While ADBuddy is in front, or half of a split screen, nothing touches or reads the screen, so the agent cannot reach the YOLO switch or Pause. The rules that stop `am start` and `monkey` from opening around it stay. A command that touches or reads the screen still runs alone, and "I cannot tell what is in front" still blocks.
  - **What the app marks as protected.** This is the other half of the screen check: the host lifts what it detects itself, but not what the app reports (ADBuddy in a split screen, a window the phone does not let ADBuddy read), which stays blocked. While YOLO is on and the app is connected, the app stops marking your protected apps, so they are **not protected** for that time; ADBuddy and a window the phone does not let ADBuddy read stay blocked.
  - **The app's own channel.** A command that reaches the link between the app and the host (`content` on ADBuddy's provider, `run-as` on ADBuddy, `am start` or `am broadcast` naming ADBuddy or with a dynamic argument) is neither asked nor run: it is blocked with the message `this touches the ADBuddy app's own channel or private files`. The same goes for any other 🔴 whose literal text names the channel (the authority `tech.hybriq.adbuddy.bridge`, the provider class, or the package together with `content` or `run-as`), even inside a pipe into `sh`, an `eval` or an assignment before `app_process`. Otherwise the agent could rewrite the link towards a server of its own, and the phone's Pause and Off would stop reaching the host. Opaque scripts are not seen (see the pending decision below).
  - **Your question about remote mode.** The question that `adbuddy remote on` asks is yours: while it waits, YOLO does not open the other doors, or the agent could press Allow on the phone.
- **The agent is told.** Each command that ran only because of YOLO prints a line `⚠ YOLO: <what was skipped>` on the agent's stderr, once per command and reason.
- **Turning it off:** the Off button of the band in the app, the Off button of its notification, `adbuddy yolo off` on the PC, the end of its time, Pause, or stopping `adbuddy serve`. YOLO does not survive a restart of `serve`.
- **The journal.** Every entry whose command started while YOLO was on carries a YOLO mark (`yolo: true` in `adbuddy journal --json`).
- **Known limits:**
  - The time is measured with the PC's wall clock: someone who can set the PC's clock back (an administrator, for example) can make a running YOLO last longer, but that person already controls the host.
  - Any result other than success when the app checks for strong biometrics (hardware unavailable, a security update required, a lockout) counts as "no strong biometrics", so the app asks for the screen lock instead: that is the PIN case above.
  - With a strong face unlock and the system's "confirm" button on, that button belongs to the system, not to ADBuddy: a script could press it with `input tap` if the face is recognised while you do nothing. The app cannot prevent this.
  - Under YOLO, `am force-stop`, `pm disable-user` and `pm clear` of the ADBuddy app do run. ADBuddy's rules protect its screen, not its process. If the agent did that you would lose the app you pause from, but `adbuddy yolo off` and the end of the time still work.
  - Turning YOLO on withdraws the red questions that were waiting: the command ends with "run it again" and does not run by itself. While YOLO is on, "allow during the session" is answered as a plain allow and creates no session permission; when YOLO ends, the session permissions created since it began are deleted (the ones from before stay). The permission cards of Claude Code that are waiting when YOLO begins are answered `allow` (by YOLO), and `allow_always` is lowered to `allow`.
  - Under YOLO, `pm uninstall` of ADBuddy and the installer or the ADBuddy page in the system's Settings are not protected: the agent could uninstall the app or take its permissions away. It cannot turn YOLO on with that, and adb already allows it without YOLO. You would need to install the app again.
  - **Pending decision.** A process or an accessibility service that the agent leaves alive under YOLO (`nohup … &`, an install, `settings put secure enabled_accessibility_services`) survives turning YOLO off, the end of its time and Pause, and, because it does not go through the guard, it could inject taps on a later Allow. This is pending a decision (options: ask for authentication on every Allow if there was YOLO in the session, stop lifting those special commands, or accept it). Until then: when YOLO ends, check the processes and the accessibility services of the phone, or restart it. The same decision covers a second limit: under YOLO the commands that name the app's channel are neither run nor asked (literal authority or package, `content`, `run-as`, dynamic `am`), but the opaque scripts are not seen (a script on the phone, `eval "$(cat …)"`, executables without the literal string). An agent under YOLO can rewrite the app's link from an opaque script, and then the phone's Pause and Off would stop reaching the host (the mode still ends by expiry or with `adbuddy yolo off` on the PC).

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

Everything else stays between your devices: the phone and the PC talk over Wireless debugging (over Tailscale's network if you use it for remote access), and the app and the host over the local channel. ADBuddy's own code uses the `INTERNET` permission only for that channel, and the app's network configuration refuses plain-text traffic except to `127.0.0.1`. The terminal in the app is shown by Android's system WebView, which runs inside the app. ADBuddy's own page for it loads only the files bundled in the app and blocks every other request, and ADBuddy sends nothing through it. The WebView is a component of Android that ADBuddy does not control: it may contact Google's servers on its own (we measured it doing so when it first started, to fetch its configuration, and we do not promise it never does at other times). That traffic comes from the system component, not from ADBuddy's code, but Android counts it under ADBuddy's data use because the WebView runs inside the app.

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

Please report it **privately**, not in a public issue. Open a [private security advisory](https://github.com/hybriqtech/adbuddy/security/advisories/new) on the repository; only the maintainers see it. The [security policy](https://github.com/hybriqtech/adbuddy/blob/main/SECURITY.md) says what to include and which versions are supported.

Please do not post working exploits, your phone's serial number or your IP address in public.

## Next

- [Use ADBuddy](usage.md): confirmations, the journal, pause and remote mode in practice.
- [FAQ](faq.md): short answers.
- [Troubleshooting](troubleshooting.md): when something fails.
- [Install ADBuddy](install.md): build from source and set up.
