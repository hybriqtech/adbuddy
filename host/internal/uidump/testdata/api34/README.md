# Real outputs of the API 34 emulator for the guard

Recorded on 2026-10-03 by the guard's tests of `host/internal/emutest` (`guard_test.go`) on the AVD `adbuddy-dev34`
(`system-images;android-34;google_apis;x86_64`), with `ADBUDDY_GUARD_RECORD` set to a folder. T5 of the manual
validations plan (`docs/superpowers/plans/2026-10-03-validaciones-manuales.md`).

Each file is `<state>.<read>.txt`, the read as the guard runs it (`daemon/guard.go`, `daemon/ui.go`):

| read | command |
|---|---|
| focus | `dumpsys window \| grep -E 'mCurrentFocus'` |
| windows | `dumpsys window windows` |
| activities | `dumpsys activity activities` |
| power | `dumpsys power` (only in the states where it differs: `settings` stands for every state with the screen on and unlocked) |
| accessibility | `dumpsys accessibility` |
| uiautomator | `uiautomator dump /dev/tty` (`adb exec-out`) |

The states:

| state | the screen |
|---|---|
| settings | Settings in front, nothing over it |
| adbuddy | ADBuddy (its first-run screen) in front |
| shade | the notification shade over Settings (`cmd statusbar expand-notifications`) |
| quick-settings | the quick settings over Settings (`cmd statusbar expand-settings`) |
| keyboard | Settings' search with the keyboard up |
| volume-dialog | systemui's volume panel over Settings (one volume key) |
| split | ADBuddy and Settings in a split screen (`WMShell splitscreen moveToSideStage`) |
| screen-off | the screen off over Settings, no lock |
| doze | the same, in Doze (`dumpsys deviceidle force-idle`) |
| locked-off, locked | a PIN set, the screen off, then on (the lock screen) |
| headsup | a heads-up of a test notification over Settings |
| headsup-gone | the same notification once its heads-up has gone (still in the shade) |
| bubble-collapsed | the bubble of ADBuddy's conversation over Settings (test notification without the foreground service) |
| bubble-expanded, bubble-expanded-a11y | the bubble open (`ChatBubbleActivity`), with ADBuddy's accessibility service off, then on |
| a11y-off, a11y-on | Settings in front, ADBuddy's accessibility service off, then on and bound |

adb.exe on Windows returns these outputs with CRLF; the repository keeps them with LF (`.gitattributes`), and the
tests read them both ways. Nothing personal is in them (`TestAPI34SamplesHoldNothingPersonal`): the emulator has
no account, and the test notifications carry only "ADBuddy guard test".

To record them again: start the emulator as `host/README.md` says, then
`ADBUDDY_GUARD_RECORD=<folder> go test -count=1 -run 'TestTheGuard|TestTheFallbackWithoutTheApp' ./internal/emutest`,
check the files (`git grep` for serials and addresses), and drop the `power` files of the states with the screen on
and unlocked other than `settings`.
