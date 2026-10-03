# T2 · El ticker en `dumpsys notification` (M5b-18, M5c-35): resultado

Fecha: 2026-10-03. Rama `test/ticker-dumpsys`. Este resultado va a las filas M5b-18 y M5c-35 de `2026-10-03-validaciones-manuales.md`, cuya tabla está en la rama `docs/validaciones-manuales`.

## Resultado: hay fuga

El volcado **redactado** de `dumpsys notification` conserva los 8 primeros caracteres de un `tickerText` de más de 16. Con el ticker `G-482913 is your ADBuddy test code`, sale `tickerText=G-482913...`: el código entero. El texto de la notificación sí sale redactado (`android.text=String [length=38]`). `--reveal` equivale a `--noredact`: los dos sacan el ticker y el texto enteros.

Medido en el emulador propio (`adbuddy-dev34`, `system-images;android-34;google_apis;x86_64`). La notificación es propia de la app; la publica el instrumentado `TickerOnEmulatorTest` porque `cmd notification post` no rellena el ticker (T1).

| Orden | Código del ticker | Código del texto | Nivel |
|---|---|---|---|
| `dumpsys notification` (también `-t 10`, `--pkg <pkg>`, `-- notification`, `--dump notification`) | sí (8 caracteres) | no | 🔴 (antes 🟢) |
| `dumpsys notification --noredact` / `--reveal` | sí | sí | 🔴 especial (ya lo era) |
| `dumpsys notification --proto`, `--stats` | no | no | 🔴 (cualquier argumento) |
| `dumpsys --pid notification` | no (solo el PID) | no | 🟢 |
| `dumpsys`, `dumpsys --skip meminfo`, `dumpsys --dump --pid` | sí | no | 🔴 (antes 🟢) |
| `dumpsys --priority CRITICAL` | sí | no | 🔴 (antes 🟢) |
| `dumpsys --priority HIGH`, `--priority NORMAL` | no | no | 🟢 |
| `dumpsys --skip notification`, `-l`, `--help`, `--pid`, `--thread`, `--clients`, `--stability` | no | no | 🟢 |
| `dumpsys statusbar`, `dumpsys activity service com.android.systemui/.SystemUIService` | no | no | 🟢 |
| `cmd notification list` (solo claves) | no | no | 🔴 (`cmd.other`, ya lo era) |
| `cmd notification get '<clave>'` | sí | sí | 🔴 (`cmd.other`, ya lo era) |

## Cambio en el clasificador

`checkDumpsys` (`host/internal/classify/checks_read.go`, `dumpsysNotifications`) pone en 🔴, sin ser especial, todo `dumpsys` que vuelque el servicio de notificaciones, con un motivo bilingüe que nombra el ticker. Una regla del usuario puede bajarlo, como pide el brief.

Sube de nivel en `testdata/habitual.jsonl`, justificado por la medida: `dumpsys`, `dumpsys --priority CRITICAL` y `dumpsys --skip meminfo`. Si solo hubiera subido `dumpsys notification`, el código seguiría saliendo con `dumpsys --priority CRITICAL` (1 MB, menos de un segundo). `docs/safety.md` lo explica.

No se ha tocado el evaluador de shell ni el oráculo.

## Tests

- `android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/TickerOnEmulatorTest.kt`: publica la notificación y la mantiene hasta que el host envía el broadcast `tech.hybriq.adbuddy.test.TICKER_DONE`; después la cancela y borra su canal. Solo corre con `-e adbuddyTicker 1`. `TestInstrumentedSuite` lo admite como salto (`allowedSkips`, 17 tests).
- `host/internal/emutest/notification_dump_test.go` (`TestNotificationReadsAndTheCodes`): da POST_NOTIFICATIONS con `pm grant`, lanza el instrumentado y lee las 20 órdenes de la tabla `notificationReads`. Comprueba que cada una enseña lo medido y que classify pone en 🔴 las que enseñan un código.
- `host/internal/classify/screen_test.go` (`TestDumpsysOfTheNotificationsIsRed`): niveles de cada forma, sin emulador.
