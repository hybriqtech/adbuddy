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
| `-l`, `--help`, `--pid`, `--thread`, `--clients`, `--stability` (sin `--dump`) | no | no | 🟢 (medido de nuevo: `--thread` y `--clients` dejan vacía la sección de cada servicio, con `NAME_NOT_FOUND` por stderr en los de Java; `--stability` imprime solo `Stability: …`; ninguno llama al volcado) |
| `dumpsys ''` (palabra vacía donde va el servicio) | no (`Can't find service: `) | no | 🔴 (antes 🟢; ninguna lectura conocida la usa) |
| `dumpsys statusbar`, `dumpsys activity service com.android.systemui/.SystemUIService` | no | no | 🟢 |
| `cmd notification list` (solo claves) | no | no | 🔴 (`cmd.other`, ya lo era) |
| `cmd notification get '<clave>'` | sí | sí | 🔴 (`cmd.other`, ya lo era) |

Lo verde de `dumpsys` (un servicio nombrado, `-l`, `--help` y las opciones que solo imprimen PID, hilos, clientes o estabilidad) se midió solo en el AOSP API 34 del emulador; en otras versiones de Android o capas de fabricante puede variar, y al cambiar de imagen se vuelve a medir con `TestNotificationReadsAndTheCodes`. Un `--priority` repetido (cuál gana no está medido) es 🔴 siempre.

## Corrección tras el fallo del CI (2026-10-03, rama `fix/dumpsys-other-services`)

**Qué pasó.** En el job `emulator` del PR #54 (emulador del CI, `android-34` con swiftshader, después de los otros tests de la suite), `TestNotificationReadsAndTheCodes` encontró la cadena del **código del texto** (`735102`, que el volcado redactado de `notification` nunca enseña) en `dumpsys --priority NORMAL` y en `dumpsys --skip notification` (15 MB), las dos 🟢. Con `--skip notification` el servicio de notificaciones no se vuelca. Lo que se vio son esas 6 cifras, no el texto de la notificación, y su origen **no está confirmado**: o un servicio que recibió el texto (quizá tarde, en un estado que no se ha reproducido), o una coincidencia por azar de 6 cifras entre los números (milisegundos, contadores) de 15 MB de volcado. El ticker redactado sí conserva 8 caracteres (medido).

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

**No se ha reproducido**, y por tanto no se sabe si algún servicio lo tenía. La temporización del test no explica el hallazgo (el test compara la salida real de `adb shell`, sin reintentos ni salidas mezcladas: las 6 cifras estaban en el volcado), pero buscar 6 cifras como subcadena en 15 MB llenos de números puede dar un falso positivo. Por eso el test usa ahora marcas con letras (ver «El test» abajo).

**Decisión.** Como no se puede descartar el hueco (un 🟢 que enseñe el texto de una notificación) ni acotarlo a un servicio, todo `dumpsys` que vuelque **varios servicios** es 🔴, deje fuera lo que deje: sin servicio, con `--skip` o con `--priority` (`dumpsysSeveral`, motivo bilingüe «dumpsys of several services may show the text of notifications, codes included»). No es especial: una regla del usuario puede bajarlo, como el redactado de `notification`. Siguen 🟢 un servicio nombrado (con su lista de argumentos de lectura), `-l`, `--help` y `--pid`/`--thread`/`--clients`/`--stability` sin `--dump`, que no llaman al volcado de ningún servicio. Los servicios sueltos no pasan a lista blanca: ninguno, salvo `notification`, enseñó un código en ningún estado medido, y subir `dumpsys activity`, `window` o `package` (habituales) sin evidencia rompería el contrato de `habitual.jsonl`.

`habitual.jsonl` sube `dumpsys --priority HIGH` y `dumpsys --skip notification`, y añade `dumpsys --priority NORMAL` en 🔴, con el motivo en un comentario.

**El test, para la próxima vez** (revisión opus del arreglo). Las marcas ya no son de 6 cifras: el ticker es `ADBZQ-482913 is your ADBuddy test code` y el host busca `ADBZQ-48` (los 8 caracteres que conserva el volcado redactado); el texto lleva `ZXQ-735102-KWV`, que se busca entero. Ninguna puede salir por azar en un volcado. `TestNotificationReadsAndTheCodes`:
- en cada lectura de varios servicios (`--priority CRITICAL`, `--priority HIGH` y `--skip notification`, el único de 15 MB que queda) exige al menos lo medido y 🔴 siempre; y, por cada sección `DUMP OF SERVICE` que tenga una marca, **falla (`Fatalf`) nombrando la sección y la forma** si `dumpsys <servicio>` o `dumpsys <servicio> <argumentos que dumpsys le pasó>` (`-a`, `--dump-priority X`, `--dump-priority NORMAL -a`) no es 🔴;
- vuelca cada servicio de `dumpsys -l` por separado de tres maneras (solo, con `-a` y con `--dump-priority NORMAL -a`, lo que reciben de `dumpsys` a secas o con `--skip` y de `--priority NORMAL`), con la notificación viva y tras cancelarla, y falla nombrando cada forma que enseñe una marca y no sea 🔴.

Si el CI vuelve a caer, el mensaje dirá qué servicio y qué forma 🟢 la enseñan; ese servicio se sube en `checkDumpsys`. Si no cae, el hallazgo del CI fue con toda probabilidad la coincidencia de 6 cifras.

Comprobado que falla así: con `dumpsys notification` bajado a 🟢 a propósito (solo para la prueba), el test cae con «dumpsys --priority CRITICAL shows a notification's mark in the dump of the service notification, and classify does not make it 🔴: dumpsys notification (green)…».

**Tiempo.** Tiene su propio contexto de 15 min (`newEmuTimeout`); el instrumentado retiene la notificación como mucho 5 min (`holdMs`) y la cancela en su `finally`; `release()` envía `TICKER_DONE` hasta 3 veces con 15 s de límite cada una. En el emulador propio (`adbuddy-dev34`) el test entero tarda **31 s** (notificación publicada a los 5 s, lecturas a los 12 s, cada servicio con la notificación viva a los 21 s y tras cancelarla a los 31 s; cada pasada de los 277 servicios de tres maneras vuelca unos 34 MB en ~10 s).

## Cambio en el clasificador

`checkDumpsys` (`host/internal/classify/checks_read.go`, `dumpsysNotifications`) pone en 🔴, sin ser especial, todo `dumpsys` que vuelque el servicio de notificaciones, con un motivo bilingüe que nombra el ticker. Una regla del usuario puede bajarlo, como pide el brief.

Sube de nivel en `testdata/habitual.jsonl`, justificado por la medida: `dumpsys`, `dumpsys --priority CRITICAL` y `dumpsys --skip meminfo`. Si solo hubiera subido `dumpsys notification`, el código seguiría saliendo con `dumpsys --priority CRITICAL` (1 MB, menos de un segundo). `docs/safety.md` lo explica.

No se ha tocado el evaluador de shell ni el oráculo.

## Tests

- `android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/TickerOnEmulatorTest.kt`: publica la notificación (ticker `ADBZQ-482913 is your ADBuddy test code`, texto `Your other ADBuddy test code is ZXQ-735102-KWV`; la medida original de arriba usó `G-482913` y `735102`) y la mantiene hasta que el host envía el broadcast `tech.hybriq.adbuddy.test.TICKER_DONE` (o pasa `holdMs`); después la cancela y borra su canal. Solo corre con `-e adbuddyTicker 1`. `TestInstrumentedSuite` lo admite como salto (`allowedSkips`, 17 tests).
- `host/internal/emutest/notification_dump_test.go` (`TestNotificationReadsAndTheCodes`): da POST_NOTIFICATIONS con `pm grant`, lanza el instrumentado y lee las 20 órdenes de la tabla `notificationReads`. Comprueba que cada una enseña lo medido (al menos lo medido, en las de varios servicios) y que classify pone en 🔴 las que enseñan una marca y todas las de varios servicios, y las formas sueltas de cada servicio que tenga una marca en ellas; después vuelca cada servicio por separado de tres maneras, con la notificación viva y tras cancelarla. `TestSectionsWithAMark` prueba sin emulador cómo saca la sección culpable.
- `host/internal/classify/screen_test.go` (`TestDumpsysOfTheNotificationsIsRed`, `TestDumpsysOfSeveralServicesIsRed`, que incluye `dumpsys ''`): niveles de cada forma, sin emulador.
