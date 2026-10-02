# ADBuddy for Android

The phone side of ADBuddy (spec §6): it shows what the coding agent on your PC
does with your phone, asks you before anything irreversible, and lets you
pause the agent or cut its access at any time. Milestone M2: consent,
confirmations, activity panel, Pause and Cut access. Milestone M3 adds the chat
with the agent: its terminal (xterm.js) with quick keys, attachments, downloads
and screenshots, and a conversation notification with direct reply, a bubble
and a "Done" notice. Milestone M4 adds the Claude Code adapter: the agent's
answers as readable bubbles, a strip with the tool it is using, and permission
cards with buttons.

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

Instrumented tests run only on an emulator, never on your own phone, through
`am instrument` on the emulator's own adb server (`TestInstrumentedSuite` in
`host/internal/emutest`).

Use the android-34 image (install it with `sdkmanager "system-images;android-34;google_apis;x86_64"`): android-37 does not work for this, its surfaceflinger loops. `stop` needs the same `ADBUDDY_EMU_*` variables as `start`. From the repository root:

```sh
(cd android && gradle :app:assembleDebug :app:assembleDebugAndroidTest)
export ADBUDDY_EMU_IMAGE='system-images;android-34;google_apis;x86_64' ADBUDDY_EMU_AVD=adbuddy-dev34 ADBUDDY_EMU_STABLE_SECS=120
eval "$(scripts/emulator.sh start)"   # the emulator, with an adb server of its own
cd host && go test -count=1 -timeout 15m -run TestInstrumentedSuite ./internal/emutest
cd .. && scripts/emulator.sh stop
```

Not `connected*` tasks: AGP's test engine (UTP) runs `adb` without the server
port of the environment (on Windows Gradle gives its workers an empty
environment), so it reaches adb server 5037, where your phone may be. The
guard in `app/build.gradle.kts` refuses every `connected*` task.

They check that the provider answers only the shell user (`TokenProviderTest`),
that the app hears changes of `adb_wifi_enabled` (`AdbWifiObserverTest`) and the
strings in both languages (`LocalesTest`). The `emulator` workflow is meant to run them (first run pending).

### Screen tests

`:app:testDebugUnitTest` also draws the screens in the JVM, with Robolectric (SDK 34, which runs on Java 17), Compose's
`ui-test` and Roborazzi: `src/test/kotlin/.../screens` (the first-run steps, Settings, the protected apps and the
notification access in light, dark, text at 200 % and text at 200 % in Spanish) and `.../flows` (the first-run steps in `MainActivity`, the
protected apps list, and the chat bubble reaching Settings). Each screen is checked in its semantics (the texts are
there, whole, on screen and apart) and against a golden image in `src/test/screenshots/`.

- `-Padbuddy.screens=record` records the images. Record on Windows, only the class you changed
  (`--tests '*SettingsScreensTest'`), and look at every image that changed before you commit it. Recording is
  deterministic: images that did not change stay the same in git.
- On Windows the default is `verify`: a pixel changed beyond Roborazzi's colour tolerance (0.7 %) fails, and
  `build/outputs/roborazzi/*_compare.png` shows where.
- Elsewhere the default is `compare`: the differences are written there too, but do not fail (the images have not been
  shown to come out the same on Linux or macOS). CI warns about it in the run's summary and uploads that folder as
  `screen-differences`.
- A UI fault the tests see but that is not fixed yet goes in `KNOWN_UI_ISSUES` (`screens/ScreenTestSupport.kt`): it
  is let through while it lasts and fails once fixed, so it is never recorded as if it were right.

Robolectric's Android jar is a Gradle dependency (`robolectricAndroid` in `app/build.gradle.kts`), so `--offline` works
once the cache has it; the tests never download anything.

## Protected apps, notification access and settings

- **Protected apps** (Settings, and the first-run step after the consent): the agent never reads or
  touches them. The list starts as a proposal made by name (banking, payments, passwords, 2FA, and the
  apps that receive your login codes, such as Messages, Gmail, Outlook, Telegram or WhatsApp (those installed), which are protected from the first launch,
  before you confirm anything); you confirm or edit it, and from then on ADBuddy never changes it by itself. ADBuddy itself is always protected. Saving sends the list to the PC.
- **Notification access** (optional): `adbuddy notifications` reads what the phone's notifications say,
  through a read-only listener (`listener/`) that you allow in the system's own screen. The title and text
  of protected apps and of ADBuddy never leave the phone, and the agent cannot dismiss or tap a notification.
  The screen shows allowed, allowed but not confirmed, not allowed or unknown, and never one for another.
  Allowing it in the system is not enough: you also tap a confirm button in ADBuddy, which nothing else
  presses for you (and the agent cannot, since ADBuddy is protected); revoking the access in the system
  takes that confirmation away.
- **Settings** gathers both, plus the remote-mode switch.

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

## Claude Code (M4)

When the agent is Claude Code, the host reads its transcript and hooks and sends
the app the answers, the tool activity and the permission requests (spec
`docs/superpowers/specs/2026-10-01-adbuddy-m4-claude-code-design.md`). Other
agents keep the terminal view.

- **Safe markdown:** answers are drawn from a small parser of our own
  (`chat/Markdown.kt`): paragraphs, headings, lists, quotes, bold, italic, code and
  code blocks. No HTML (it shows as text) and no images (only their alt text; the
  URL is never loaded). Only `http(s)` links, shown with their real domain and
  asked before they open; a link with a user-info part is not a link, and its text
  has no bidi or invisible characters. The input and the number of blocks are
  bounded.
- **Tool strip:** the tools the agent is running (`tool.activity`: running, done,
  error) appear as chips in a single strip, the last item of the chat, and only
  while the agent is working. Only the short summary is shown, never the output of
  the tool.
- **Permission cards:** above the input, one per pending request, labelled "Claude
  Code asks" so they cannot be taken for ADBuddy's own questions. The card shows the
  tool, the summary and the whole detail, which scrolls. **Yes** and **Yes, always**
  (only if the host offers it) stay disabled until the detail has been scrolled to
  the end and for a short guard after the card appears. **No** skips only the wait
  for the scroll: like the others it needs the link connected, is not offered on a
  card that was just promoted and is not offered while the card is answering. The guard fails closed (a card that was never observed does not answer) and is
  re-armed when the link reconnects. A card answers once (it shows "answering" until
  the host says `permission.resolved`) and only the host removes it. A command the
  card cannot show whole is left to the terminal by the host.
- **Notice:** with a request pending and the app in the background, a generic
  notice ("Claude Code needs an answer") opens the chat. It has no command, no
  text and no buttons except Cut access, and it is silent (same channel as the
  conversation). It is not shown while the link is paused or disconnected. The
  "Done" notice shows the first words of the agent's last message, cleaned of
  separators, invisible characters and markdown delimiters.
- **ADBuddy's 🔴 questions** still use `ConfirmDialog` (M2), unchanged: a Yes on a
  Claude Code card never answers one of them, and neither does a Yes on one of them
  answer a card.

M4 unit tests (JVM, no device):

```sh
gradle :app:testDebugUnitTest --tests '*MarkdownTest' --tests '*LinksTest' \
  --tests '*ChatReducerTest' --tests '*PermissionUiTest' --tests '*ToolStripTest' \
  --tests '*ChatControllerTest' --tests '*ConversationViewTest'
```

The composables (`MarkdownText`, `PermissionCard`, `ToolStripView`) have no tests:
they are validated by hand on a device (see "M4" in
`docs/superpowers/plans/pendientes-m3-m5.md`).

## Languages

The app is in English and Spanish and follows the language of the phone (on Android 13 and later you can also set it for ADBuddy alone in the system settings, Apps, ADBuddy, Language). Every string is in `res/values/strings.xml` and `res/values-es/strings.xml`, in the same commit, with positional markers (`%1$s`) and `<plurals>` where a number is shown; no composable, notification or `contentDescription` has a literal text.

`./gradlew :app:testDebugUnitTest` checks it: `StringsParityTest` (same keys, same markers and plurals in both files, escaping, a string for every `ErrorCode`) and `NoHardcodedTextTest` (a literal text in Kotlin fails; the few that are on purpose are in `src/test/resources/i18n-exceptions.txt`, with their reason). `lintDebug` fails on a missing translation. `LocalesTest` (instrumented, on the emulator: see above) checks the language itself.

An error of the host is shown by its `code` (`ErrorText`), not by its English `text`.

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
