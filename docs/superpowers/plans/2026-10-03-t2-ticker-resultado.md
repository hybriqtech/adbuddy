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
| `dumpsys --priority HIGH`, `--priority NORMAL` | no en local; **en el CI, el del texto** (ver abajo) | no en local; sí en el CI | 🔴 (antes 🟢, desde la corrección del CI) |
| `dumpsys --skip notification` (y cualquier `--skip`) | no en local; **en el CI, el del texto** | no en local; sí en el CI | 🔴 (antes 🟢, desde la corrección del CI) |
| `-l`, `--help`, `--pid`, `--thread`, `--clients`, `--stability` (sin `--dump`) | no | no | 🟢 |
| `dumpsys statusbar`, `dumpsys activity service com.android.systemui/.SystemUIService` | no | no | 🟢 |
| `cmd notification list` (solo claves) | no | no | 🔴 (`cmd.other`, ya lo era) |
| `cmd notification get '<clave>'` | sí | sí | 🔴 (`cmd.other`, ya lo era) |

Lo verde de `dumpsys` (un servicio nombrado, `-l`, `--help` y las opciones que solo imprimen PID, hilos, clientes o estabilidad) se midió solo en el AOSP API 34 del emulador; en otras versiones de Android o capas de fabricante puede variar, y al cambiar de imagen se vuelve a medir con `TestNotificationReadsAndTheCodes`. Un `--priority` repetido (cuál gana no está medido) es 🔴 siempre.

## Corrección tras el fallo del CI (2026-10-03, rama `fix/dumpsys-other-services`)

**Qué pasó.** En el job `emulator` del PR #54 (emulador del CI, `android-34` con swiftshader, después de los otros tests de la suite), `TestNotificationReadsAndTheCodes` encontró el **código del texto** (`735102`, que el volcado redactado de `notification` nunca enseña) en `dumpsys --priority NORMAL` y en `dumpsys --skip notification` (15 MB), las dos 🟢. Con `--skip notification` el servicio de notificaciones no se vuelca, así que el texto entero de la notificación lo tenía **otro servicio**.

**Lo que se midió en local** (emulador propio `adbuddy-dev34`, la misma imagen; la notificación de `TickerOnEmulatorTest` publicada y viva mientras se mide; cada uno de los 276-277 servicios de `dumpsys -l` volcado por separado y, además, `dumpsys`, `--priority NORMAL|HIGH|CRITICAL` y `--skip notification` enteros, buscando en qué sección `DUMP OF SERVICE` sale cada código):

| Estado | Servicios con un código | Volcados de varios servicios con el código del texto |
|---|---|---|
| Emulador limpio, notificación viva | solo `notification` (ticker, 8 caracteres) | ninguno |
| Con el servicio de accesibilidad de ADBuddy activo | solo `notification` | ninguno |
| Con la persiana abierta | solo `notification` | ninguno |
| Con la pantalla apagada y en la pantalla de bloqueo | solo `notification` | ninguno |
| Sondeo cada 3 s de `--skip notification` antes, durante y después de publicarla (30 rondas) | — | ninguno |
| Tras cancelarla | ninguno | ninguno |
| Después de la suite en el orden del CI (dos veces), y de los tests que van justo antes (`TestTypeWritesUnicodeIntoAField` a `TestTheScreenshotGoesToTheHost`) | solo `notification` | ninguno |

**No se ha reproducido**, y por tanto no se sabe qué servicio lo tenía ni en qué estado. Hipótesis (b) (temporización del test) descartada como explicación de un falso positivo: el test compara la salida real de `adb shell`, sin reintentos ni salidas mezcladas; si el código salió, estaba en el volcado. La hipótesis (a) queda en pie sin el nombre del servicio.

**Decisión.** Como el hueco se ha visto (un 🟢 que enseña el texto entero de una notificación) y no se puede acotar a un servicio, todo `dumpsys` que vuelque **varios servicios** es 🔴, deje fuera lo que deje: sin servicio, con `--skip` o con `--priority` (`dumpsysSeveral`, motivo bilingüe «dumpsys of several services may show the text of notifications, codes included»). No es especial: una regla del usuario puede bajarlo, como el redactado de `notification`. Siguen 🟢 un servicio nombrado (con su lista de argumentos de lectura), `-l`, `--help` y `--pid`/`--thread`/`--clients`/`--stability` sin `--dump`, que no llaman al volcado de ningún servicio. Los servicios sueltos no pasan a lista blanca: ninguno, salvo `notification`, enseñó un código en ningún estado medido, y subir `dumpsys activity`, `window` o `package` (habituales) sin evidencia rompería el contrato de `habitual.jsonl`.

`habitual.jsonl` sube `dumpsys --priority HIGH` y `dumpsys --skip notification`, y añade `dumpsys --priority NORMAL` en 🔴, con el motivo en un comentario.

**El test, para la próxima vez.** `TestNotificationReadsAndTheCodes` registra en qué servicio (`DUMP OF SERVICE …`) y línea sale cada código; para las lecturas de varios servicios exige al menos lo medido (pueden enseñar más) y 🔴 siempre; y vuelca cada servicio de `dumpsys -l` por separado con la notificación viva y otra vez tras cancelarla, exigiendo 🔴 para cualquiera que enseñe un código. Si el CI vuelve a caer en ese estado, el test fallará nombrando el servicio, y ese servicio se sube en el clasificador.

## Cambio en el clasificador

`checkDumpsys` (`host/internal/classify/checks_read.go`, `dumpsysNotifications`) pone en 🔴, sin ser especial, todo `dumpsys` que vuelque el servicio de notificaciones, con un motivo bilingüe que nombra el ticker. Una regla del usuario puede bajarlo, como pide el brief.

Sube de nivel en `testdata/habitual.jsonl`, justificado por la medida: `dumpsys`, `dumpsys --priority CRITICAL` y `dumpsys --skip meminfo`. Si solo hubiera subido `dumpsys notification`, el código seguiría saliendo con `dumpsys --priority CRITICAL` (1 MB, menos de un segundo). `docs/safety.md` lo explica.

No se ha tocado el evaluador de shell ni el oráculo.

## Tests

- `android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/TickerOnEmulatorTest.kt`: publica la notificación y la mantiene hasta que el host envía el broadcast `tech.hybriq.adbuddy.test.TICKER_DONE`; después la cancela y borra su canal. Solo corre con `-e adbuddyTicker 1`. `TestInstrumentedSuite` lo admite como salto (`allowedSkips`, 17 tests).
- `host/internal/emutest/notification_dump_test.go` (`TestNotificationReadsAndTheCodes`): da POST_NOTIFICATIONS con `pm grant`, lanza el instrumentado y lee las 20 órdenes de la tabla `notificationReads`. Comprueba que cada una enseña lo medido (al menos lo medido, en las de varios servicios) y que classify pone en 🔴 las que enseñan un código y todas las de varios servicios; después vuelca cada servicio por separado, con la notificación viva y tras cancelarla.
- `host/internal/classify/screen_test.go` (`TestDumpsysOfTheNotificationsIsRed`, `TestDumpsysOfSeveralServicesIsRed`): niveles de cada forma, sin emulador.
