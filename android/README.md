# ADBuddy for Android

The phone side of ADBuddy (spec §6): it shows what the coding agent on your PC
does with your phone, asks you before anything irreversible, and lets you
pause the agent or cut its access at any time. Milestone M2: consent,
confirmations, activity panel, Pause and Cut access. Milestone M3 adds the chat
with the agent: its terminal (xterm.js) with quick keys, attachments, downloads
and screenshots, and a conversation notification with direct reply, a bubble
and a "Done" notice.

The app is installed and set up by the host (`adbuddy setup`), over the adb
connection you paired: you do not need to install it by hand.

## Build

Requirements: JDK 17, Android SDK platform 37, and Gradle 9.8 (there is no
Gradle wrapper in the repository yet).

```sh
cd android
gradle :app:testDebugUnitTest     # unit tests (protocol, state, handshake, WebSocket client)
gradle :app:assembleDebug         # app/build/outputs/apk/debug/app-debug.apk
adbuddy setup --apk app/build/outputs/apk/debug/app-debug.apk
```

Instrumented tests (`gradle :app:connectedDebugAndroidTest`) need an emulator:
never run them on your own phone.

## Chat and terminal

- **Terminal:** xterm.js 5.5.0 with its fit and web-links addons (MIT), bundled in
  `app/src/main/assets/term/` (`VENDOR.md` lists versions and SHA-256; a unit test
  checks them). The WebView loads only those files (`TermWebPolicy`), cannot reach
  the network, files or content, and the page talks to the app through three
  methods: keyboard data, a new size and a link to open (always asked first, with
  the domain). OSC 52 (writing to the clipboard) is off. The host rules the size.
- **Files:** up to 200 MiB each, uploaded in 256 KiB frames and resumed from the
  host's offset; downloads go to the app's cache (500 MiB, oldest first) and are
  opened or shared through a `FileProvider` with a temporary read permission.
- **Notification:** a conversation notification with direct reply, screenshot, Pause
  and Cut access. Reply and screenshot never act with the phone locked. The reply is
  a `RemoteInput`: notification listeners and watches you have authorised can fire it.
- **Privacy:** neither the chat nor the terminal text is written to any log, and the
  lock screen shows no message text, no activity and no PC name.

## How it talks to the host

- The host writes a random 256-bit token, the port and the PC's name to the
  app's provider on stdin (`adb shell content write --uri
  content://tech.hybriq.adbuddy.bridge/handshake`). The provider only answers
  the shell user (uid 2000), and the host's classifier blocks the agent from
  using it.
- `adb reverse tcp:<port> tcp:<port>` makes `127.0.0.1:<port>` on the phone
  reach the host on the PC. The app connects to `ws://127.0.0.1:<port>/v1`
  and sends the token in its `hello`.
- Everything the host sends (titles, commands, journal entries) is data shown
  as text, never instructions.
