# ADBuddy v1 · Diferidos B (guarda con %XX, menores de M5b, IPC robusto y tests de la app) · Plan de implementación

> **Para quien ejecute:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`. Los pasos usan casillas (`- [ ]`). Implementadores y re-revisiones en Sonnet (`adbuddy-implementer`, `adbuddy-rereviewer`); `adbuddy-reviewer-opus` para las tareas marcadas 🔒 (daemon, guarda, modo remoto, IPC) y para la revisión final; `adbuddy-reviewer` para las demás. Nunca Haiku. **Antes de la tarea 1, `adbuddy-reviewer-opus` revisa este plan completo, código incluido** (CLAUDE.md), y el plan se corrige con sus hallazgos. Las tareas de `host/` (1 a 6) y las de `android/` (7 a 11) son árboles disjuntos: pueden ir en paralelo, una de cada árbol a la vez.

**Meta:** cerrar seis diferidos del triage de la v1 (V11, V38, V29, V21, V42, V43): que la guarda no se pueda saltar con un nombre de paquete codificado, que el modo remoto no reinicie adbd sin necesidad ni el vigilante se quede colgado, que el IPC no se deje secuestrar ni leer un secreto a medias ni reservar memoria sin autenticar, y que la política del WebView de la terminal, los reductores, el `Bridge`, el Markdown y el `TerminalFeed` de la app tengan los tests (y los arreglos) que les faltaban.

**Arquitectura:** cambios pequeños y locales, sin piezas nuevas salvo el cerrojo de fichero del IPC (`ipc/lock*.go`, con `golang.org/x/sys`, que ya está en `go.mod`). **El protocolo de cable no cambia** (ni el del IPC ni el del canal con la app), la autenticación del IPC tampoco, `classify/` no se toca y no hay motivos nuevos del clasificador.

**Tecnologías:** Go 1.27 (`host/`), Kotlin + JUnit 4 + MockWebServer (`android/`, solo tests JVM). Nada nuevo en `go.mod` ni en `libs.versions.toml`.

**Fuentes:** triage `C:\Users\Jose\orca\workspaces\adbuddy\skimmer\.superpowers\sdd\2026-10-02-adbuddy-v1-diferidos\triage.md` (filas V11, V38, V29, V21, V42, V43) · ledger de M5b `.superpowers/sdd/2026-10-01-adbuddy-m5b-modo-remoto/progress.md` (rulings del modo remoto) · `docs/superpowers/plans/pendientes-m3-m5.md` (M3: `Bridge` y reductores, `TermWebPolicyTest`, revisión final de M3b; M4: Markdown y herramientas; M5: menores aplazados de M5b) · spec v1 `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md` (§4.1 IPC, §3.3 modo remoto) · spec M5 `2026-10-01-adbuddy-m5-ayudas-remoto-design.md` (§4.3 guarda, §7.2 modo remoto).

**Verificación del plan (réplica acumulada):** el código de este plan salió de una copia de `fix/v1-diferidos-b` en 2fcf8e2 (`git archive`), donde se aplicó **tarea a tarea** y se comprobó cada RED y cada GREEN con los comandos de cada tarea (salidas reales citadas en los pasos). Al final de la réplica: `go build ./... && go vet ./... && gofmt -l .` limpios, `GOOS=linux go vet ./...` y `GOOS=darwin go vet ./...` limpios, `go test -count=1 -timeout 15m ./...` con los 29 paquetes en `ok`, y `gradle :app:testDebugUnitTest --offline` en verde (57 clases). Los ficheros nuevos son byte a byte los del plan; los cambios a ficheros existentes son pares "reemplazar / por" para la herramienta Edit (cada texto "reemplazar" es único en su fichero) o "añadir al final".

**Actualización tras M6c (rama rebasada sobre `origin/main` 0aaa593, con el catálogo i18n de M6c y los docs de M6d):** el plan se revisó contra esta rama. De los ficheros que toca, M6c solo cambió `cli/phone_watch.go` (textos al catálogo; el `step` y `watchTryWait` que edita la Tarea 3 siguen iguales), `cli/serve.go` (el `DEVICE_NOT_FOUND` ya sale del catálogo: la Tarea 3 cambia ahora `catalog/en.toml` y `catalog/es.toml` en vez de `serve.go`) y `cli/remote_test.go` (un import). `daemon/guard.go`, `remote/`, `ipc/`, los tests de `daemon` y todo lo de la app que toca el plan no cambiaron. El plan entero se volvió a aplicar con el script sobre un `git archive` limpio de esta rama: resultado en "Réplica acumulada tarea a tarea".

## Global Constraints

- Rama `fix/v1-diferidos-b`, sobre `origin/main` 0aaa593 (M5c, M6c y M6d integradas). Commits con identidad local hybriqtech (nunca `git config --global`) y trailer **exacto** `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` (también en fixups; comprobarlo con `git log -1 --format=%B`). Nunca `--no-verify`, nunca `--force`, sin `git stash`. Sin push ni PR salvo que lo pida el controlador; a `main` integra el usuario. `git grep` de seriales e IPs antes de cualquier push.
- **Sin IPs RFC 1918** en código, tests ni docs: `100.64.0.5`, `198.51.100.x`, `192.0.2.x`. Sin seriales reales (los tests usan los de sus fixtures).
- **El móvil del usuario no se toca.** Los tests usan el adb falso (`testutil/fakeadb`, `fakeADB` del daemon, `ctlADB`/`env` del modo remoto), `FakeChannel`, MockWebServer y la JVM. Lo que necesita un móvil va a "Validaciones manuales pendientes" y solo se hace con permiso explícito del usuario.
- **Rulings de M5b que no se rompen:** el estado del modo remoto es de tres valores y lo desconocido nunca es "apagado" (un falso apagado oculta un 5555 abierto); `Enable` guarda la caducidad antes de `adb tcpip`; `Closing` sigue siendo "encendido" hasta que `adb usb` cierra; toda pregunta al usuario pasa por la puerta del daemon (`AskUser`); falla cerrado ante lo que no se entiende.
- **La guarda falla cerrado:** lo que no sabe decodificar o no puede leer bloquea; ningún cambio de este plan deja pasar algo que hoy bloquea (los tests existentes de `daemon/` siguen en verde sin tocarlos).
- **IPC:** ni el formato de las tramas, ni `Request`/`Exit`, ni la comprobación del secreto cambian. El secreto sigue siendo de 64 hex, legible solo por el usuario (0600 en Unix; en Windows, la ACL heredada de la carpeta de datos, como hasta ahora).
- `host/internal/classify/` no se toca; `testdata/habitual.jsonl` intacto. Ningún motivo nuevo de `classify`.
- **Cada tarea compila y pasa sus tests por sí sola;** nada usa símbolos de tareas posteriores.
- **Textos nuevos de cara al usuario:** siguen las reglas de M6c (spec `docs/superpowers/specs/2026-10-02-adbuddy-m6c-i18n-design.md` §3 y §4.2, y su receta de añadir cadenas): lo que lee una persona va al catálogo `host/internal/i18n/catalog/{en,es}.toml` con la misma clave en los dos y verbos con índice (`%[1]s`), y lo vigilan `TestCatalogParity`, `TestKeyUsesAnalyzer` y `TestKeysExist`; lo que lee el agente («Blocked by ADBuddy: …» y las razones de la guarda) y las causas técnicas de paquetes internos (el `err.Error()` que va dentro de `error.cause`) se quedan en inglés a propósito. Lista en "Textos nuevos".
- **Go:** `cd host && go build ./... && go vet ./... && gofmt -l . && go test -count=1 -timeout 5m ./<paquete>/...`; la suite completa (`-timeout 15m ./...`) en el último paso de cada tarea de host. Este PC no corre `-race`: la concurrencia se razona en cada tarea y la corre la CI. Las tareas de IPC además: `GOOS=linux go vet ./internal/ipc && GOOS=darwin go vet ./internal/ipc` (el cerrojo tiene una versión por sistema).
- **Kotlin:** `export JAVA_HOME="/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1"; export PATH="$JAVA_HOME/bin:/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin:$PATH"; cd android && gradle :app:testDebugUnitTest --offline` (sin wrapper). Para una clase: `--tests 'tech.hybriq.adbuddy.<paquete>.<Clase>'`. Los fallos se leen en `app/build/test-results/testDebugUnitTest/*.xml` (`grep -h -A3 "<failure"`).
- `docs/superpowers/plans/pendientes-m3-m5.md` y `docs/troubleshooting.md` están en **CRLF** en este árbol (`core.autocrlf=true`, sin atributo `eol`): sus ediciones usan anclas de **una sola línea**. `host/**` es LF (`.gitattributes`).
- Pasar a los revisores límites de tiempo (`go test -timeout 5m`, tope de 30 minutos por revisión): una se colgó más de una hora en M2.
- Windows con Git Bash: sin `sleep` en primer plano en los pasos; los comandos largos con timeout amplio o en segundo plano.

## Rulings de este plan (van al ledger tal cual)

1. **Ruling: la guarda mira cada decodificación %XX de cada palabra (hasta 4 capas) y parte por todo carácter que no puede estar en un nombre de paquete.** — Android decodifica `%XX` en el host de un URI `android-app://` y en los valores de un URI `intent:`, y hoy `am start -d android-app://com%2Eexample%2Ebank` pasa; además la verificación encontró que `?` y `@` tampoco partían (`android-app://com.example.bank?x=1` y `android-app://u@com.example.bank` pasan hoy). Hex en mayúsculas o minúsculas; un `%` mal formado se deja tal cual (Android lo convierte en U+FFFD, que ningún paquete tiene), y como `%` también parte, `com.example.bank%2` bloquea (falla cerrado). Más de 4 capas: bloquea con un texto propio. — Si me equivoco: un `am start` legítimo con un URI raro que nombra una app protegida queda bloqueado (el usuario lo hace a mano); nunca deja pasar algo que hoy bloquea.
2. **Ruling: `Disable` sin caducidad guardada lee el puerto antes de `adb usb`.** — Con el modo apagado, `adb usb` reinicia adbd y corta la conexión del móvil por nada (la CLI sin serve ya no lo hace desde la revisión final de M5b, menor 3). Se lee `service.adb.tcp.port`: si no es 5555 no se hace nada; si es 5555 **o no se puede leer**, `adb usb` como antes (falla cerrado: cierra también un 5555 abierto a mano). No cambia ningún estado: sin caducidad ya era "apagado". — Si me equivoco: una lectura de adb más en cada "off" con el modo apagado.
3. **Ruling: tras un `adb usb` que cerró el puerto y un `SetUntil` que falló, los reintentos de esa misma caducidad solo intentan borrarla.** — Hoy cada reintento de `Expire` (espaciados hasta 2 min, ruling 41 de M5b) vuelve a reiniciar adbd y a cortar la conexión. El controlador recuerda en memoria (`closedFor`, bajo su mutex) la caducidad cuyo puerto ya cerró; cualquier otra caducidad vuelve a pasar por `adb usb`, y `Enable` lo olvida justo antes de `adb tcpip` (revisión Opus del plan: si `tcpip` diera un falso "sin móvil" aunque la orden llegó, `Enable` restaura la caducidad anterior, que podría ser igual a `closedFor`, y el siguiente `Expire` se saltaría `adb usb`: falso apagado con el 5555 abierto). El estado sigue siendo `Closing` (encendido) hasta que la caducidad se borra: nunca un falso apagado. — Si me equivoco: si alguien abre el 5555 fuera del controlador entre el fallo y el reintento, el reintento no lo cierra (igual que hoy no se cierra un 5555 abierto a mano con el modo apagado).
4. **Ruling: cada mirada del vigilante a `adb devices` se corta a `TryWait` (8 s).** — Un `adb devices` colgado dejaba la gorrutina de `watchPhone` parada para siempre dentro de un `step`. Un corte es "adb falla", que ya no cambia nada. — Si me equivoco: con un adb lentísimo (> 8 s) el vigilante no ve el móvil en esa mirada y lo vuelve a intentar en la siguiente.
5. **Ruling: el texto de `DEVICE_NOT_FOUND` con direcciones guardadas dice que no funcionó ninguna, cuáles se probaron y que el error es el último.** — Decía "its saved address did not work (a, b: <error de b>)": en singular y con el error de una sola como si fuera de todas. Desde M6c el texto es la clave `err.serve.device_not_found` del catálogo: se cambia en inglés y en español con los mismos argumentos (`serve.go` no cambia), y va con su línea de `docs/troubleshooting.md` (solo existe en inglés). — Si me equivoco: una frase del catálogo que se retoca en los dos idiomas.
6. **Ruling: el daemon guarda un cerrojo de sistema (`adbuddy.lock`, `flock` en Unix, `LockFileEx` en Windows) mientras sirve.** — `Listen` hacía Stat, Dial y Remove sin exclusión: dos `serve` que arrancan a la vez con un socket muerto podían quitarse el socket vivo el uno al otro. El cerrojo se suelta en `Close` y con la muerte del proceso, sea como sea; el fichero se queda en disco (borrarlo abriría otra carrera). La comprobación por Dial se mantiene para un daemon de una versión anterior, que no toma el cerrojo. — Si me equivoco: en un sistema de ficheros sin cerrojos (una carpeta de datos en red rara) `serve` no arranca y lo dice; `ADBUDDY_DATA_DIR` local lo arregla.
7. **Ruling: el secreto se escribe entero con otro nombre en la misma carpeta y se renombra, con un cerrojo de creación (`ipc.secret.lock`), y `ReadSecret` reintenta un momento lo que no puede leer entero.** — Con `O_EXCL` y luego la escritura, un lector veía un secreto vacío o a medias (la réplica lo reproduce: 2 de 10 ejecuciones de `TestSecretCreatedOnceUnderARace` fallan con el código de hoy, con un creador que devuelve `""`). Renombrar no basta solo (el último en renombrar ganaría y el primero serviría con un secreto que ya no está en el fichero): por eso el cerrojo. En Windows un renombrado en curso hace fallar la apertura con "being used by another process": `ReadSecret` reintenta 20 veces cada 10 ms (variables: los tests las alargan para una CI lenta) todo error que no sea "no existe" (también un secreto corto de un host antiguo que lo escribe en dos pasos). El fichero temporal se crea en la misma carpeta, así que hereda los mismos permisos y la misma ACL que antes. — Si me equivoco: un secreto de verdad corrupto tarda 200 ms más en dar su error.
8. **Ruling: como mucho 32 conexiones esperan a autenticarse a la vez, y `ReadFrame` no reserva más de 64 KiB antes de que lleguen los bytes.** — Cada conexión sin autenticar podía hacer que el servidor reservara 1 MiB durante 5 s, sin límite de conexiones. La 33.ª se cierra al aceptarla (falla cerrado); la plaza se devuelve en cuanto la conexión se autentica, y una rechazada o que se va la conserva hasta que termina (`finish` espera hasta 2 s a que el cliente cierre), así que tampoco los rechazos crecen sin tope. Un proceso del mismo usuario puede agotar el cupo (32 conexiones que no mandan nada, renovadas cada 5 s, el plazo absoluto de lectura antes de autenticar): es una denegación de servicio local, no una frontera de privilegios (ese proceso ya podría matar `serve`), y cada plaza se libera sola a los 5 s. Por encima de 64 KiB el búfer se dobla solo con lo leído, así que la memoria es a lo sumo el doble de lo recibido. Las tramas grandes siguen llegando enteras. — Si me equivoco: con más de 32 clientes legítimos a la vez en la fase previa a autenticar (microsegundos cada uno), alguno recibe un cierre y su orden falla ("lost the connection"); el agente la repite.
9. **Ruling: el WebView de la terminal solo carga un fichero de `/assets/term/` por su nombre llano** (`[A-Za-z0-9_-][A-Za-z0-9._-]*` y sin `..`). — Ningún caso de borde sacaba de la carpeta, pero `index.html?x=1`, `#frag`, `//index.html` o `sub/x` pasaban la política; la carpeta es plana y la página solo enlaza nombres llanos. Una lista blanca no deja forma de salir. — Si me equivoco: un fichero nuevo con otro carácter en el nombre no carga y se ve enseguida.
10. **Ruling: los tests nuevos de `Bridge` y el fuzz de los reductores son de caracterización; su RED es una mutación comprobada.** — El código ya cumple (el fuzz no encontró ninguna excepción con 3 semillas × 5000 mensajes). Quitar `if (webSocket !== socket) return` de `Listener.onMessage` hace fallar `anOldSocketIsIgnoredAfterANewLink`; cambiar el `catch (e: IllegalArgumentException)` de `ChatReducer.snapshot` por otro tipo hace fallar el fuzz (semilla 20261002, paso 33). La reentrada `onResult → request` no tiene mutación realista que la rompa (los monitores de la JVM son reentrantes): queda como test del contrato. — Si me equivoco: tests que no detectan una regresión concreta; los ve la revisión.
11. **Ruling: una imagen dentro de un enlace (`[![alt](imagen)](url)`) es el enlace con el alt como texto.** — Hoy sale un enlace con texto `![alt` hacia la URL de la imagen, y el contrato es que la URL de una imagen nunca es un enlace (`imagesAreOnlyTheirAltTextAndTheUrlIsNeverALink`). Si la URL de fuera no es web (o no hay), queda el alt como texto. Sin recursión ni búsquedas nuevas fuera de `Finder`, y con casos hostiles nuevos en el test de tiempo lineal. — Si me equivoco: un texto que empezaba por `![` dentro de corchetes pierde su enlace (queda el texto).
12. **Ruling: `TerminalFeed` guarda una pila de vistas y solo dibuja la de arriba.** — Con la burbuja sobre una `MainActivity` visible, al plegar la burbuja se mandaba `unwatch` y la terminal de debajo se congelaba hasta un onStop/onStart. Ahora la vista de debajo se redibuja desde la memoria y el flujo sigue (sin `watch` ni `unwatch`); el flujo solo se para con la última vista. — Si me equivoco: una vista que nunca se desengancha seguiría en la pila; todas se desenganchan en `onStopOrDispose` o al morir el renderizador.

## Verificación de los ítems del triage (en esta rama)

| Ítem | ¿Real? | Qué hace este plan |
|---|---|---|
| V11 `%XX` en nombres de paquete | Sí (reproducido). Además `?` y `@` tampoco partían | Tarea 1 |
| V38 `Online(ctx)` sin tope en `step` | Sí (`phone_watch.go:125`; un `adb devices` de 20 s para el `step` 20 s) | Tarea 3 |
| V38 `Disable` sin modo remoto reinicia adbd | Sí (`remote.go:169-183`; con serve, `remote off` o `remote.set` apagado lanzan `adb usb`) | Tarea 2 |
| V38 `adb usb` repetido si `SetUntil` falla | Sí, mitigado por el espaciado (ruling 41 de M5b) | Tarea 2 |
| V38 `REMOTE_UNAVAILABLE` depende del texto de adb | Sí (`remote.NoPhone`), pero falla al lado seguro | Diferido (abajo) |
| V38 `DEVICE_NOT_FOUND` ambiguo | Sí, también tras M6c (`err.serve.device_not_found` en `catalog/{en,es}.toml`, con varias direcciones y un solo error, en los dos idiomas) | Tarea 3 (catálogo) |
| V38 ~900 ms de adb por llamada de la guarda | Sí (`guard.go:230`, `focusCmd` en cada llamada guardada, y `windowsCmd` sin app) | Diferido (abajo) |
| V29 Stat/Dial/Remove sin cerrojo | Sí (`server.go:192-201`; reproducido quitando el socket) | Tarea 4 |
| V29 secreto sin escritura atómica | Sí (reproducido: 2/10 ejecuciones del test de carrera) | Tarea 5 |
| V29 1 MiB antes de autenticar, sin límite | Sí (1 048 728 bytes reservados para 9 recibidos; la conexión 33 se queda) | Tarea 6 |
| V21 casos de borde de `TermWebPolicy` | Sin fuga, pero `?x=1`, `#frag`, `//`, `sub/` pasaban | Tarea 7 |
| V42 tests de socket viejo y reentrada | Faltaban; el código cumple | Tarea 8 |
| V42 ningún reductor lanza | El fuzz no encontró excepciones | Tarea 9 |
| V43 `Markdown.MAX_BLOCKS` admite uno más | Sí (2001) | Tarea 10 |
| V43 imagen dentro de enlace | Fallo real: la URL de la imagen salía como enlace | Tarea 10 |
| V43 pila de sinks de `TerminalFeed` | Sí (lógica pura, test JVM) | Tarea 11 |
| V43 herramientas atascadas en `running` | Sí | Diferido (abajo) |
| V43 `PickMultipleVisualMedia(10)` sin descontar borradores | Sí | Diferido (abajo) |

## Fuera de alcance (diferidos, con motivo)

- **V38 · `REMOTE_UNAVAILABLE` depende del texto de adb** (`remote.NoPhone` busca "no devices/emulators found", "device offline", "no devices", "no phone connected"). Es inherente: adb solo lo dice en texto. Falla al lado seguro: un texto que no reconoce cuenta como "la orden pudo llegar", así que `Enable` deja la caducidad nueva (falso "encendido", nunca falso apagado) y la CLI dice `REMOTE_FAILED` en vez de `REMOTE_UNAVAILABLE`; lo cubren `TestEnableTimeoutKeepsUntil` y `TestEnableTcpipFailsPortUnreadableKeepsUntil`. Cambiarlo (por ejemplo, preguntar `adb get-state` tras el fallo) toca la invariante central de M5b y debe validarse con versiones reales de adb: decisión de diseño, no de este plan.
- **V38 · ~900 ms de consultas a adb por llamada de la guarda.** Real: `systemUIReason` lanza `dumpsys window | grep mCurrentFocus` en cada orden guardada, y sin app además `dumpsys window windows`. Unirlas en una consulta exige muestras reales de `dumpsys window windows` (ruling 62 de M5b: el emulador no sirve; validación 17 de M5b, bloqueante) y tocar el parser de la guarda. Coste: latencia, no seguridad. Para v1.1, tras la validación 17.
- **V43 · herramientas atascadas en `running`.** Real: si el transcript no trae el `tool_result` (herramienta interrumpida), el chip sigue en `running` hasta que salen otras cinco o hasta la sesión siguiente, y con el agente en `waiting` hace visible la tira. La app no tiene con qué arreglarlo bien: no conoce el fin de turno, y `ToolActivity` no trae hora. Una heurística por `messageId` (una herramienta posterior de otro mensaje del asistente) falla cuando el turno siguiente no usa herramientas. Con M6c integrada ya no hay solape con `cli/phone_agent.go`, pero el arreglo sigue siendo una decisión de diseño, no un menor: el hook `Stop` **no se dispara cuando el usuario interrumpe** (Esc), que es justo el caso de la herramienta interrumpida, así que la señal útil sería el `UserPromptSubmit` (o el `SessionStart`) siguiente cerrando las que sigan en `running`; y el protocolo solo tiene `running`, `done` y `error` (¿una interrumpida es `error`, o un estado nuevo, con cambio de protocolo en las dos partes?). Para la spec de v1.1 o un plan propio del adaptador de Claude Code.
- **V43 · `PickMultipleVisualMedia(10)` no descuenta los borradores.** Real, pero es UI: el lanzador se crea en la composición de `ChatRoot` con un máximo fijo y el contrato exige un máximo ≥ 2; descontar exige rehacer el lanzador al cambiar los borradores (Compose, sin test JVM). `Composer.canSend` ya rechaza más de 10, así que el usuario ve que no puede enviar. Para la siguiente ronda de UI.
- **V21 (resto de su línea en pendientes):** el WebView recreado sin límite si el renderizador muere en bucle y `onReceivedError` visibles son UI y no estaban en el encargo.

## Textos nuevos de cara al usuario (según las reglas de M6c)

1. **Al catálogo (en y es), Tarea 3:** `err.serve.device_not_found` (lo usa `serve.go` con `TransientErrorT`). Inglés: `the phone did not show up, and none of its saved addresses worked (tried %[1]s; last error seen: %[2]s)`. Español: `el móvil no apareció, y no funcionó ninguna de sus direcciones guardadas (probadas: %[1]s; último error visto: %[2]s)`. El arreglo (`err.serve.device_not_found.fix`, con `%[3]s`) no cambia. `--json` y la sesión del agente siguen viendo el inglés (`NewErrorT` calcula `Message` en inglés). Y la misma frase inglesa en `docs/troubleshooting.md` ("The Wireless debugging port changed"), que no tiene versión en español ni test que compare su texto con el catálogo (los `TestDocs*` de `cli/docs_test.go` miran vallas, enlaces, órdenes y privacidad).
2. **En inglés a propósito, Tarea 1:** la razón nueva de la guarda (`daemon/guard.go`, `encodedReason`), tras "Blocked by ADBuddy: ": `the app this command opens is hidden under too many layers of %-encoding; ADBuddy does not open an app without knowing which`. Spec de M6c §3: los mensajes de bloqueo de la shim y las razones de la guarda son salida para el agente y no se traducen (M6c no tocó `guard.go`; `protectedReason` y `screenProtectedReason` siguen en inglés).
3. **En inglés a propósito, Tarea 5:** `another ADBuddy is creating <ruta>/ipc.secret: the lock is held` es la causa técnica que devuelve `ipc.EnsureSecret`; `serve.go` ya la mete como argumento de `error.cause` dentro de `SECRET_FAILED` (`NewErrorT("SECRET_FAILED", "error.cause", "err.serve.secret_failed.fix", err.Error(), e.dirs.Data)`), y M6c deja en inglés las causas de paquetes internos (pendientes, "Se quedan en inglés a propósito"). Solo sale si otro proceso tarda más de 5 s en crear el secreto.

La app no gana cadenas: ninguna tarea toca `strings.xml`.

## Mapa de ficheros

**Host (modificado):** `daemon/guard.go`, `daemon/ui_test.go` (T1) · `remote/remote.go`, `remote/remote_test.go`, `cli/remotecmd_test.go` (T2) · `cli/phone_watch.go`, `cli/phone_watch_test.go`, `i18n/catalog/en.toml`, `i18n/catalog/es.toml`, `cli/remote_test.go`, `docs/troubleshooting.md` (T3) · `ipc/server.go`, `ipc/ipc_test.go` (T4, T6) · `ipc/ipc.go` (T5, T6) · `ipc/lock.go` (T4, T5).

**Host (nuevo):** `ipc/lock.go`, `ipc/lock_unix.go`, `ipc/lock_windows.go` (T4).

**App (modificado, rutas bajo `android/app/src/`):** `main/kotlin/tech/hybriq/adbuddy/term/TermWebPolicy.kt`, `test/kotlin/tech/hybriq/adbuddy/term/TermWebPolicyTest.kt` (T7) · `test/kotlin/tech/hybriq/adbuddy/bridge/BridgeTest.kt` (T8) · `main/kotlin/tech/hybriq/adbuddy/chat/Markdown.kt`, `test/kotlin/tech/hybriq/adbuddy/chat/MarkdownTest.kt` (T10) · `main/kotlin/tech/hybriq/adbuddy/term/TerminalFeed.kt`, `test/kotlin/tech/hybriq/adbuddy/term/TerminalFeedTest.kt` (T11).

**App (nuevo):** `test/kotlin/tech/hybriq/adbuddy/bridge/ReducerFuzzTest.kt` (T9).

**Docs:** `docs/superpowers/plans/pendientes-m3-m5.md` (T12).

---

### Task 1: La guarda decodifica %XX y parte por todo lo que no es de un nombre de paquete (V11) 🔒

**Revisor:** `adbuddy-reviewer-opus` (daemon, guarda).

**Files:**
- Modify: `host/internal/daemon/guard.go` (el bucle de `guard` que busca paquetes en `f.Words`, líneas 53-61, y funciones nuevas antes de `runsUnwrapped`)
- Test: `host/internal/daemon/ui_test.go` (dos tests nuevos antes de `TestGuardFailsClosed`)

**Interfaces:**
- Consumes: `d.protected(pkg)`, `protectedReason(pkg)`, el entorno de test `newEnv`/`withApp`/`e.exec` (`daemon_test.go`, `ui_test.go`).
- Produces (privadas del paquete `daemon`): `const encodedReason string`, `const maxDecodings = 4`, `func decodings(w string) (forms []string, ok bool)`, `func percentDecode(s string) string`, `func isHex(c byte) bool`, `func unhex(c byte) byte`, `func notInPackageName(r rune) bool`.

- [ ] **Step 1: Test que falla.** Insertar antes de `func TestGuardFailsClosed(t *testing.T) {` en `host/internal/daemon/ui_test.go`:

**Editar `host/internal/daemon/ui_test.go`:** reemplazar

```go
func TestGuardFailsClosed(t *testing.T) {
```

por

```go
// V11: the name of the app may come %-encoded (Android decodes %XX in a
// URI's host and in an intent URI's values), with hex digits in either case,
// encoded more than once, or after a character that cannot be in a package
// name (?, @): every decoding is looked at, and that character splits it.
func TestGuardBlocksOpeningAProtectedAppEncoded(t *testing.T) {
	e := newEnv(t)
	e.withApp("com.example.bank")
	const named, layers = "is a protected app", "too many layers of %-encoding"
	for _, c := range []struct {
		name string
		argv []string
		want string // "" = it runs
	}{
		{"encoded dots", []string{"shell", "am", "start", "-d", "android-app://com%2Eexample%2Ebank"}, named},
		{"lower-case hex", []string{"shell", "am", "start", "-a", "android.intent.action.VIEW", "-d", "android-app://com%2eexample%2ebank/https/example.com"}, named},
		{"every letter encoded", []string{"shell", "am", "start", "-d", "android-app://%63%6F%6D%2E%65%78%61%6D%70%6C%65%2E%62%61%6E%6B"}, named},
		{"encoded twice", []string{"shell", "am", "start", "-d", "android-app://com%252Eexample%252Ebank"}, named},
		{"encoded slashes", []string{"shell", "am", "start", "-d", "android-app:%2F%2Fcom.example.bank"}, named},
		{"encoded scheme", []string{"shell", "am", "start", "-d", "android-app%3A%2F%2Fcom.example.bank"}, named},
		{"query", []string{"shell", "am", "start", "-d", "android-app://com.example.bank?x=1"}, named},
		{"user info", []string{"shell", "am", "start", "-d", "android-app://u@com.example.bank"}, named},
		{"intent URI", []string{"shell", "am start 'intent:#Intent;package=com%2Eexample%2Ebank;end'"}, named},
		{"ADBuddy", []string{"shell", "am", "start", "-d", "android-app://tech%2Ehybriq%2Eadbuddy"}, named},
		{"a cut % after the name", []string{"shell", "am", "start", "-d", "android-app://com.example.bank%2"}, named},
		{"five layers", []string{"shell", "am", "start", "-d", "android-app://com%252525252Eexample%252525252Ebank"}, layers},
		{"malformed % is not a name", []string{"shell", "am", "start", "-d", "android-app://com%2Eexample%ZZbank"}, ""},
		{"another app", []string{"shell", "am", "start", "-d", "android-app://com%2Eandroid%2Esettings"}, ""},
	} {
		s, exit := e.exec(t, c.argv...)
		if c.want == "" {
			if exit.Code != 0 {
				t.Errorf("%s: blocked: %+v %q", c.name, exit, s.errOut.String())
			}
			continue
		}
		if exit.Code != 126 || !strings.Contains(s.errOut.String(), c.want) {
			t.Errorf("%s: %q: %+v %q", c.name, c.argv, exit, s.errOut.String())
		}
	}
}

func TestGuardFailsClosed(t *testing.T) {
```

- [ ] **Step 2: Ver que falla.**

```bash
cd host && go test -count=1 -timeout 5m ./internal/daemon -run TestGuardBlocksOpeningAProtectedAppEncoded
```

Esperado: FAIL en 12 casos, todos con `{Code:0 …}` (se ejecutaron) salvo "intent URI", que hoy da el `aloneReason` ("must run alone") en vez de "is a protected app". Salida real de la réplica (recortada):

```
--- FAIL: TestGuardBlocksOpeningAProtectedAppEncoded (0.17s)
    ui_test.go:462: encoded dots: ["shell" "am" "start" "-d" "android-app://com%2Eexample%2Ebank"]: {Code:0 ErrorCode: Error:} ""
    ui_test.go:462: query: ["shell" "am" "start" "-d" "android-app://com.example.bank?x=1"]: {Code:0 ErrorCode: Error:} ""
    ui_test.go:462: user info: ["shell" "am" "start" "-d" "android-app://u@com.example.bank"]: {Code:0 ErrorCode: Error:} ""
    ui_test.go:462: intent URI: […]: {Code:126 ErrorCode: Error:} "Blocked by ADBuddy: a command that touches or reads the screen must run alone: …"
    ui_test.go:462: five layers: […]: {Code:0 ErrorCode: Error:} ""
FAIL
```

- [ ] **Step 3: Implementar.**

**Editar `host/internal/daemon/guard.go`:** (cambio 1 de 2) reemplazar

```go
		// am start and monkey may name the app they open, as a component,
		// a package or inside an intent URI.
		for _, w := range f.Words[1:] {
			for _, tok := range strings.FieldsFunc(w, func(r rune) bool { return strings.ContainsRune("/;=:#,{} \t", r) }) {
				if d.protected(tok) {
					return protectedReason(tok)
				}
			}
		}
```

por

```go
		// am start and monkey may name the app they open, as a component,
		// a package or inside an intent URI, and %-encoded: Android decodes
		// %XX in a URI's host and in an intent URI's values. Every decoding
		// is looked at, and anything that cannot be in a package name splits
		// it (/, ;, =, ?, @, %…).
		for _, w := range f.Words[1:] {
			forms, ok := decodings(w)
			if !ok {
				return encodedReason
			}
			for _, form := range forms {
				for _, tok := range strings.FieldsFunc(form, notInPackageName) {
					if d.protected(tok) {
						return protectedReason(tok)
					}
				}
			}
		}
```

**Editar `host/internal/daemon/guard.go`:** (cambio 2 de 2) reemplazar

```go
// runsUnwrapped reports whether the shell text of args starts with the
```

por

```go
// encodedReason is why a word %-encoded more than maxDecodings times blocks:
// the app it opens cannot be told.
const encodedReason = "the app this command opens is hidden under too many layers of %-encoding; ADBuddy does not open an app without knowing which"

// maxDecodings is how many times decodings undoes the %-encoding of a word.
const maxDecodings = 4

// decodings returns w and each of its successive %-decodings (percentDecode),
// up to maxDecodings of them. ok is false when w still changes after that
// many: what it would end up naming is not known.
func decodings(w string) (forms []string, ok bool) {
	forms = []string{w}
	cur := w
	for range maxDecodings {
		next := percentDecode(cur)
		if next == cur {
			return forms, true
		}
		forms = append(forms, next)
		cur = next
	}
	return forms, percentDecode(cur) == cur
}

// percentDecode turns each %XX (two hex digits, in either case) into its
// byte and leaves any other % as it is. Android's Uri.decode turns a
// malformed escape into U+FFFD, which no package name has either.
func percentDecode(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]) {
			b.WriteByte(unhex(s[i+1])<<4 | unhex(s[i+2]))
			i += 2
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isHex(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

// unhex is the value of the hex digit c (isHex(c)).
func unhex(c byte) byte {
	switch {
	case c <= '9':
		return c - '0'
	case c <= 'F':
		return c - 'A' + 10
	}
	return c - 'a' + 10
}

// notInPackageName reports whether r cannot be in an Android package name
// (ASCII letters, digits, _ and .).
func notInPackageName(r rune) bool {
	return !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' || r == '_' || r == '.')
}

// runsUnwrapped reports whether the shell text of args starts with the
```

- [ ] **Step 4: Test unitario de `decodings`.** Insertar tras el test del paso 1:

**Editar `host/internal/daemon/ui_test.go`:** reemplazar

```go
		if exit.Code != 126 || !strings.Contains(s.errOut.String(), c.want) {
			t.Errorf("%s: %q: %+v %q", c.name, c.argv, exit, s.errOut.String())
		}
	}
}

func TestGuardFailsClosed(t *testing.T) {
```

por

```go
		if exit.Code != 126 || !strings.Contains(s.errOut.String(), c.want) {
			t.Errorf("%s: %q: %+v %q", c.name, c.argv, exit, s.errOut.String())
		}
	}
}

func TestPercentDecodings(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
		ok   bool
	}{
		{"plain", []string{"plain"}, true},
		{"a%2Eb", []string{"a%2Eb", "a.b"}, true},
		{"a%2eb", []string{"a%2eb", "a.b"}, true},
		{"a%252Eb", []string{"a%252Eb", "a%2Eb", "a.b"}, true},
		{"a%ZZb%2", []string{"a%ZZb%2"}, true},
		{"%", []string{"%"}, true},
		{"a%25252525252Eb", nil, false},
	} {
		got, ok := decodings(c.in)
		if ok != c.ok || ok && !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: %q %v, want %q %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestGuardFailsClosed(t *testing.T) {
```

(`reflect` ya está importado en `ui_test.go`.)

- [ ] **Step 5: Ver que pasa, y que lo ya cubierto no se rompe.**

```bash
cd host && gofmt -l ./internal/daemon && go vet ./internal/daemon && go test -count=1 -timeout 5m ./internal/daemon
```

Esperado: `gofmt` sin salida, `ok  github.com/hybriqtech/adbuddy/host/internal/daemon` (réplica: 6.6 s). `TestGuardBlocksOpeningAProtectedApp`, `TestGuardWantsScreenCommandsAlone` y el resto siguen en verde sin tocarlos.

- [ ] **Step 6: Suite completa del host.**

```bash
cd host && go build ./... && go vet ./... && gofmt -l . && go test -count=1 -timeout 15m ./...
```

Esperado: todo `ok`.

- [ ] **Step 7: Commit.**

```bash
git add host/internal/daemon/guard.go host/internal/daemon/ui_test.go
git commit -m "fix(host): la guarda decodifica %XX y parte por lo que no es de un nombre de paquete

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Concurrencia:** funciones puras; nada compartido.

---

### Task 2: El modo remoto no reinicia adbd sin necesidad (V38: `Disable` apagado y reintentos tras un `SetUntil` fallido) 🔒

**Revisor:** `adbuddy-reviewer-opus` (modo remoto: invariante de "nunca falso apagado").

**Files:**
- Modify: `host/internal/remote/remote.go` (struct `Controller`, `Disable`, `disable`)
- Test: `host/internal/remote/remote_test.go` (dos tests al final)
- Modify: `host/internal/cli/remotecmd_test.go` (`TestRemoteOnAllowedAfterALaterOffDenies`: el "off" con el modo apagado ya no lanza `adb usb`)

**Interfaces:**
- Consumes: `remote.Store` (`Until`, `SetUntil`), `c.adbCmd`, `Port`.
- Produces: campo privado `Controller.closedFor time.Time`; `Disable` y `disable` con la misma firma (`func (c *Controller) Disable(ctx context.Context) (State, error)`).

- [ ] **Step 1: Tests que fallan.** Añadir al final de `host/internal/remote/remote_test.go`:

```go

// V38: with no expiry saved the host believes the port closed, and `adb usb`
// would restart adbd (and drop the phone's connection) for nothing. The port
// is still looked at: one that reads 5555, or cannot be read, is closed.
func TestDisableWithoutTheModeLooksAtThePortFirst(t *testing.T) {
	for _, c := range []struct {
		name      string
		port      string
		portFails bool
		want      string
	}{
		{"closed", "", false, "adb " + getprop},
		{"another port", "37123", false, "adb " + getprop},
		{"open", "5555", false, "adb " + getprop + "|adb usb|save zero"},
		{"unreadable", "", true, "adb " + getprop + "|adb usb|save zero"},
	} {
		e := newEnv()
		e.port, e.portFails = c.port, c.portFails
		s, err := e.controller().Disable(context.Background())
		if err != nil || s != (State{}) {
			t.Fatalf("%s: state %+v err %v", c.name, s, err)
		}
		if got := strings.Join(e.events, "|"); got != c.want {
			t.Fatalf("%s: events %q, want %q", c.name, got, c.want)
		}
	}
}

// V38: `adb usb` closed the port but the expiry could not be cleared: the
// next attempts only try to clear it, without restarting adbd each time.
// A new expiry (Enable) closes with `adb usb` again.
func TestExpireAfterAFailedSaveDoesNotRunUsbAgain(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(time.Hour)
	e.now = t0.Add(2 * time.Hour)
	e.saveFail = true
	c := e.controller()
	if s, changed, err := c.Expire(context.Background()); err == nil || changed || !s.Enabled || !s.Closing {
		t.Fatalf("first: %+v %v %v", s, changed, err)
	}
	if s, changed, err := c.Expire(context.Background()); err == nil || changed || !s.Enabled || !s.Closing {
		t.Fatalf("second: %+v %v %v", s, changed, err)
	}
	if got := strings.Join(e.events, "|"); got != "adb usb" {
		t.Fatalf("events %q: adb usb ran again", got)
	}
	e.saveFail = false
	if s, changed, err := c.Expire(context.Background()); err != nil || !changed || s != (State{}) {
		t.Fatalf("third: %+v %v %v", s, changed, err)
	}
	if got := strings.Join(e.events, "|"); got != "adb usb|save zero" {
		t.Fatalf("events %q", got)
	}
	// A failed save again, then a new expiry: its port is closed with adb usb.
	e.until, e.saveFail, e.events = t0.Add(time.Hour), true, nil
	if _, _, err := c.Expire(context.Background()); err == nil {
		t.Fatal("the save did not fail")
	}
	e.saveFail = false
	if _, err := c.Enable(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.Add(13 * time.Hour)
	if _, changed, err := c.Expire(context.Background()); err != nil || !changed {
		t.Fatalf("after Enable: %v %v", changed, err)
	}
	if got := strings.Join(e.events, "|"); got != "adb usb|save 12h0m0s|adb tcpip 5555|adb usb|save zero" {
		t.Fatalf("events %q", got)
	}
}

// V38 (plan review): Enable forgets the expiry whose port was closed before
// it runs `adb tcpip`. If tcpip then fails as "no phone", the old expiry is
// put back, but the order may have reached adbd: the next Expire must close
// with `adb usb` again, not only clear the expiry (a false "off" with the
// port open).
func TestEnableForgetsTheExpiryItsPortWasClosedFor(t *testing.T) {
	e := newEnv()
	old := t0.Add(time.Hour)
	e.until = old
	e.now = t0.Add(2 * time.Hour)
	e.saveFail = true
	c := e.controller()
	if _, _, err := c.Expire(context.Background()); err == nil {
		t.Fatal("the save did not fail")
	}
	e.saveFail = false
	e.tcpipFail, e.tcpipErr = true, "error: no devices/emulators found"
	if _, err := c.Enable(context.Background()); err == nil || !NoPhone(err) {
		t.Fatalf("Enable: %v", err)
	}
	if !e.until.Equal(old) {
		t.Fatalf("until %v: the old expiry was not put back", e.until)
	}
	e.events = nil
	if _, changed, err := c.Expire(context.Background()); err != nil || !changed {
		t.Fatalf("Expire: changed %v err %v", changed, err)
	}
	if got := strings.Join(e.events, "|"); got != "adb usb|save zero" {
		t.Fatalf("events %q: the port was not closed again", got)
	}
}
```

- [ ] **Step 2: Ver que fallan.**

```bash
cd host && go test -count=1 -timeout 5m ./internal/remote
```

Esperado (réplica; `TestEnableForgetsTheExpiryItsPortWasClosedFor` pasa ya con el código de hoy, que siempre lanza `adb usb`: vigila el cambio 3 del paso 3, y sin ese cambio falla con `events "save zero": the port was not closed again`):

```
--- FAIL: TestDisableWithoutTheModeLooksAtThePortFirst (0.00s)
    remote_test.go:478: closed: events "adb usb|save zero", want "adb shell getprop service.adb.tcp.port"
--- FAIL: TestExpireAfterAFailedSaveDoesNotRunUsbAgain (0.00s)
    remote_test.go:499: events "adb usb|adb usb": adb usb ran again
FAIL
```

- [ ] **Step 3: Implementar.**

**Editar `host/internal/remote/remote.go`:** (cambio 1 de 3) reemplazar

```go
type Controller struct {
	mu  sync.Mutex // one change at a time: adbd restarts under each
	adb ADB
	st  Store
	now func() time.Time
}
```

por

```go
type Controller struct {
	mu  sync.Mutex // one change at a time: adbd restarts under each
	adb ADB
	st  Store
	now func() time.Time
	// closedFor is the expiry whose port `adb usb` closed while clearing it
	// failed (under mu): the next attempt only clears it, without restarting
	// adbd again. Any other expiry (Enable saved a new one) runs `adb usb`.
	closedFor time.Time
}
```

**Editar `host/internal/remote/remote.go`:** (cambio 2 de 3) reemplazar

```go
// Disable closes the port (`adb usb`) and only then clears the expiry: if
// adb fails the mode stays on (and says so) and is tried again.
func (c *Controller) Disable(ctx context.Context) (State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.disable(ctx)
}

func (c *Controller) disable(ctx context.Context) (State, error) {
	if _, err := c.adbCmd(ctx, "usb"); err != nil {
		return c.State(), err
	}
	if err := c.st.SetUntil(time.Time{}); err != nil {
		return c.State(), err
	}
	return State{}, nil
}
```

por

```go
// Disable closes the port (`adb usb`) and only then clears the expiry: if
// adb fails the mode stays on (and says so) and is tried again. With no
// expiry saved (off as far as the host knows) it first reads the port:
// `adb usb` would restart adbd, and drop the phone's connection, for nothing.
// A port that reads 5555, or cannot be read, is closed all the same.
func (c *Controller) Disable(ctx context.Context) (State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st.Until().IsZero() {
		if port, err := c.adbCmd(ctx, "shell", "getprop service.adb.tcp.port"); err == nil && port != Port {
			return State{}, nil
		}
	}
	return c.disable(ctx)
}

// disable runs `adb usb` and clears the expiry. When clearing failed after
// `adb usb` closed the port, the next call for the same expiry only clears it
// (closedFor): each `adb usb` restarts adbd and the phone's connection.
func (c *Controller) disable(ctx context.Context) (State, error) {
	until := c.st.Until()
	if c.closedFor.IsZero() || !until.Equal(c.closedFor) {
		if _, err := c.adbCmd(ctx, "usb"); err != nil {
			return c.State(), err
		}
	}
	if err := c.st.SetUntil(time.Time{}); err != nil {
		c.closedFor = until
		return c.State(), err
	}
	c.closedFor = time.Time{}
	return State{}, nil
}
```

**Editar `host/internal/remote/remote.go`:** (cambio 3 de 3) reemplazar

```go
	prev := c.st.Until() // zero when off; passed when Closing
	if err := c.st.SetUntil(c.now().Add(Lifetime)); err != nil {
		return c.State(), err
	}
	if _, err := c.adbCmd(ctx, "tcpip", Port); err != nil {
```

por

```go
	prev := c.st.Until() // zero when off; passed when Closing
	if err := c.st.SetUntil(c.now().Add(Lifetime)); err != nil {
		return c.State(), err
	}
	// The port may open now: whatever expiry ends up saved (prev comes back
	// on "no phone", and the order may still have reached adbd), the next
	// disable closes it with `adb usb`.
	c.closedFor = time.Time{}
	if _, err := c.adbCmd(ctx, "tcpip", Port); err != nil {
```

- [ ] **Step 4: Ver que pasan.**

```bash
cd host && gofmt -l ./internal/remote && go vet ./internal/remote && go test -count=1 -timeout 5m ./internal/remote
```

Esperado: `ok` (todos los tests existentes de `remote` siguen en verde: `TestDisableRunsUsbThenClears`, `TestDisableSaveFailsStaysEnabled`, `TestExpireUsbFailsStaysClosing`, `TestEnableWithoutPhoneWhileClosingKeepsTheOldUntil`…).

- [ ] **Step 5: Ajustar el test de la CLI que esperaba `adb usb` con el modo apagado.** Con este cambio, `TestRemoteOnAllowedAfterALaterOffDenies` se cuelga esperando un `usb` que ya no ocurre (réplica: `remotecmd_test.go:602: timed out waiting for the off to run`): el "off" encolado con el modo apagado solo lee el puerto (el `ctlADB` del test contesta `""`).

**Editar `host/internal/cli/remotecmd_test.go`:** reemplazar

```go
	waitUntil(t, "the off to run", func() bool { return h.g.phone.count("usb") == 1 })
	if h.g.phone.count("tcpip 5555") != 0 {
		t.Fatalf("calls %v", h.g.phone.called())
	}
}
```

por

```go
	// The mode was off: the off only looks at the port (closed), without `adb usb`.
	waitUntil(t, "the off to run", func() bool { return h.g.phone.count("shell getprop service.adb.tcp.port") == 1 })
	if h.g.phone.count("tcpip 5555") != 0 || h.g.phone.count("usb") != 0 {
		t.Fatalf("calls %v", h.g.phone.called())
	}
}
```

- [ ] **Step 6: Tests de la CLI del modo remoto y suite completa.**

```bash
cd host && go test -count=1 -timeout 10m ./internal/cli -run 'Remote|Watch|Expire'
cd host && go build ./... && go vet ./... && gofmt -l . && go test -count=1 -timeout 15m ./...
```

Esperado: `ok` (réplica: 32 s el primero).

- [ ] **Step 7: Commit.**

```bash
git add host/internal/remote/remote.go host/internal/remote/remote_test.go host/internal/cli/remotecmd_test.go
git commit -m "fix(host): el modo remoto no reinicia adbd si ya está apagado ni al reintentar tras un fallo al guardar

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Concurrencia:** `closedFor` se lee y escribe en `disable` y se pone a cero en `Enable` antes de `adb tcpip`, siempre con `c.mu` tomado (lo toman `Disable`, `Expire`, `Reconcile` y `Enable`). Que `Enable` lo olvide importa aunque guarde una caducidad nueva: si `tcpip` falla con un texto de "sin móvil" (`NoPhone`), `Enable` restaura la anterior, que puede ser justo `closedFor`, y la orden pudo llegar a adbd; sin ponerlo a cero, el `Expire` siguiente borraría la caducidad sin `adb usb` (lo vigila `TestEnableForgetsTheExpiryItsPortWasClosedFor`).

---

### Task 3: El vigilante no se cuelga en `adb devices` y `DEVICE_NOT_FOUND` dice qué se probó (V38)

**Revisor:** `adbuddy-reviewer` (no toca la verificación de identidad del vigilante ni ningún puerto; solo acota una llamada y cambia un texto).

**Files:**
- Modify: `host/internal/cli/phone_watch.go` (comentario de `watchTryWait`, `step`)
- Test: `host/internal/cli/phone_watch_test.go` (test al final)
- Modify: `host/internal/i18n/catalog/en.toml:340` y `host/internal/i18n/catalog/es.toml:339` (clave `err.serve.device_not_found`, que `serve.go:122` ya usa con `TransientErrorT` desde M6c: `serve.go` **no se toca**)
- Test: `host/internal/cli/remote_test.go` (test nuevo antes de `TestSetupQRHintWhenNoPhoneIsVisible`, en inglés y en español; import de `i18ntest`)
- Modify: `docs/troubleshooting.md:209` (CRLF: ancla de una línea; solo existe en inglés)

**Interfaces:**
- Consumes: `newRig`, `testutil.FakeRule{SleepMs}`, `newActivePhone`, `tsConfig`, `serveOnce`, `shortWaits`, `phoneGone`, `lanAddr` (`phone_watch_test.go`), `tsAddr` (`connect_test.go`); `i18ntest.Lang` (`internal/i18n/i18ntest`); la llamada de M6c `TransientErrorT("DEVICE_NOT_FOUND", "err.serve.device_not_found", "err.serve.device_not_found.fix", strings.Join(addrs, ", "), AsError(k.AddrErr).Message, reconnect)`, que pasa `%[1]s` (direcciones), `%[2]s` (último error) y `%[3]s` (el arreglo).
- Produces: nada nuevo; `watcher.step` acota `Online` con `w.tryWait`; el patrón de `err.serve.device_not_found` cambia en los dos idiomas con los mismos verbos (`%[1]s`, `%[2]s`), así que `TestCatalogParity` y `TestKeyUsesAnalyzer` siguen igual.

- [ ] **Step 1: Tests que fallan.** Añadir al final de `host/internal/cli/phone_watch_test.go`:

```go

// V38: a look at `adb devices` that hangs is cut at TryWait, so the watcher
// does not stop for good inside one step (adb failing says nothing about the
// phone: nothing changes).
func TestWatchPhoneAHungDevicesIsCut(t *testing.T) {
	hang := testutil.FakeRule{Match: "^devices$", SleepMs: 20000}
	r := newRig(t, hang)
	active := newActivePhone(adb.Runner{Path: r.runner.Path, Serial: phoneSerial})
	w := newWatcher(phoneWatch{Active: active, Runner: r.runner, Serial: phoneSerial,
		Cfg: func() config.Config { return tsConfig() }, RemoteOn: func() bool { return false }, Log: &bytes.Buffer{},
		Every: time.Millisecond, Misses: 1, TryWait: 500 * time.Millisecond})
	start := time.Now()
	w.step(context.Background())
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("one step took %v", d)
	}
	if active.Serial() != phoneSerial || w.count != 0 {
		t.Fatalf("serial %q count %d", active.Serial(), w.count)
	}
}
```

Y en `host/internal/cli/remote_test.go` (el import de `i18ntest` y el test):

**Editar `host/internal/cli/remote_test.go`:** (cambio 1 de 2) reemplazar

```go
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
```

por

```go
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/i18n/i18ntest"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
```

**Editar `host/internal/cli/remote_test.go`:** (cambio 2 de 2) reemplazar

```go
// In QR mode, a network where no phone announces wireless debugging gets one
```

por

```go
// V38: with several saved addresses, the error says that none worked, which
// ones were tried, and that the error shown is the last one (not the error
// of every address). For a person it is in their language (M6c catalog).
func TestServeSavedAddressesGoneSaysWhichWereTried(t *testing.T) {
	shortWaits(t)
	cfg := config.Default()
	cfg.Serial, cfg.Address = phoneSerial, lanAddr
	cfg.TailscaleIP, cfg.TailscalePort = "100.64.0.5", 41234
	rules := []testutil.FakeRule{
		{Match: "^connect ", Stdout: "failed to connect: Connection refused\n"},
		{Match: "^devices$", Stdout: phoneGone},
		{Match: "^mdns services$", Stdout: "List of discovered mdns services\n"},
	}
	code, errOut, _, _ := serveOnce(t, rules, cfg)
	want := "none of its saved addresses worked (tried " + lanAddr + ", " + tsAddr + "; last error seen: "
	if code != ExitTransient || !strings.Contains(errOut, "DEVICE_NOT_FOUND") || !strings.Contains(errOut, want) {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	i18ntest.Lang(t, i18n.ES)
	code, errOut, _, _ = serveOnce(t, rules, cfg)
	wantES := "no funcionó ninguna de sus direcciones guardadas (probadas: " + lanAddr + ", " + tsAddr + "; último error visto: "
	if code != ExitTransient || !strings.Contains(errOut, "DEVICE_NOT_FOUND") || !strings.Contains(errOut, wantES) {
		t.Fatalf("Spanish: exit %d stderr %q", code, errOut)
	}
}

// In QR mode, a network where no phone announces wireless debugging gets one
```

- [ ] **Step 2: Ver que fallan.**

```bash
cd host && go test -count=1 -timeout 5m ./internal/cli -run 'TestWatchPhoneAHungDevicesIsCut|TestServeSavedAddressesGoneSaysWhichWereTried'
```

Esperado (réplica): `phone_watch_test.go:431: one step took 20.1829521s`, y `remote_test.go:244: exit 75 stderr "…Error: DEVICE_NOT_FOUND — the phone did not show up, and its saved address did not work (198.51.100.7:40000, 100.64.0.5:41234: adb connect 100.64.0.5:41234: failed to connect: Connection refused)…"`.

- [ ] **Step 3: Implementar.**

**Editar `host/internal/cli/phone_watch.go`:** (cambio 1 de 2) reemplazar

```go
	watchTryWait = 8 * time.Second  // bounds each `adb connect` of a candidate
)
```

por

```go
	watchTryWait = 8 * time.Second  // bounds each look at `adb devices` and each `adb connect` of a candidate
)
```

**Editar `host/internal/cli/phone_watch.go`:** (cambio 2 de 2) reemplazar

```go
// step is one look at the connection.
func (w *watcher) step(ctx context.Context) {
	online, err := w.hostRunner().Online(ctx)
	if err != nil || ctx.Err() != nil {
```

por

```go
// step is one look at the connection. A hung `adb devices` is cut at
// tryWait: otherwise the watcher would stop for good inside one step.
func (w *watcher) step(ctx context.Context) {
	octx, cancel := context.WithTimeout(ctx, w.tryWait)
	online, err := w.hostRunner().Online(octx)
	cancel()
	if err != nil || ctx.Err() != nil {
```

El texto vive en el catálogo de M6c (la receta de M6c: la misma clave en `en.toml` y `es.toml`, con los mismos verbos con índice; `serve.go` no cambia). Los dos ficheros son LF.

**Editar `host/internal/i18n/catalog/en.toml`:** reemplazar

```toml
"err.serve.device_not_found" = "the phone did not show up, and its saved address did not work (%[1]s: %[2]s)"
```

por

```toml
"err.serve.device_not_found" = "the phone did not show up, and none of its saved addresses worked (tried %[1]s; last error seen: %[2]s)"
```

**Editar `host/internal/i18n/catalog/es.toml`:** reemplazar

```toml
"err.serve.device_not_found" = "el móvil no apareció, y su dirección guardada no funcionó (%[1]s: %[2]s)"
```

por

```toml
"err.serve.device_not_found" = "el móvil no apareció, y no funcionó ninguna de sus direcciones guardadas (probadas: %[1]s; último error visto: %[2]s)"
```

**Editar `docs/troubleshooting.md`** (CRLF; ancla de una línea): reemplazar

```
Every time Wireless debugging is turned on again, the phone picks a new port. If the saved address stops working, `adbuddy serve` says `the phone did not show up, and its saved address did not work` and suggests the fix:
```

por

```
Every time Wireless debugging is turned on again, the phone picks a new port. If the saved address stops working, `adbuddy serve` says `the phone did not show up, and none of its saved addresses worked` and suggests the fix:
```

(Comprobar después con `file docs/troubleshooting.md` que sigue "with CRLF line terminators" y con `git diff --stat` que solo cambia una línea.)

- [ ] **Step 4: Ver que pasan.**

```bash
cd host && gofmt -l ./internal/cli && go vet ./internal/cli && go test -count=1 -timeout 5m ./internal/cli -run 'TestWatchPhone|TestServeSavedAddress|TestDocs' && go test -count=1 -timeout 5m ./internal/i18n/...
```

Esperado: `ok` (réplica: 2.2 s; `TestServeSavedAddressGone` sigue en verde: busca `DEVICE_NOT_FOUND` y `adbuddy connect <new ip:port>`, que no cambian).

- [ ] **Step 5: Suite completa.**

```bash
cd host && go build ./... && go vet ./... && gofmt -l . && go test -count=1 -timeout 15m ./...
```

- [ ] **Step 6: Commit.**

```bash
git add host/internal/cli/phone_watch.go host/internal/cli/phone_watch_test.go host/internal/i18n/catalog/en.toml host/internal/i18n/catalog/es.toml host/internal/cli/remote_test.go docs/troubleshooting.md
git commit -m "fix(host): el vigilante acota adb devices y DEVICE_NOT_FOUND dice qué direcciones se probaron

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Concurrencia:** `step` corre en la única gorrutina del vigilante; el contexto nuevo se cancela antes de seguir.

---

### Task 4: Un solo daemon por carpeta de datos, con un cerrojo de sistema (V29) 🔒

**Revisor:** `adbuddy-reviewer-opus` (IPC).

**Files:**
- Create: `host/internal/ipc/lock.go`, `host/internal/ipc/lock_unix.go`, `host/internal/ipc/lock_windows.go`
- Modify: `host/internal/ipc/server.go` (struct `Server`, `Listen`, `Close`)
- Test: `host/internal/ipc/ipc_test.go` (test al final)

**Interfaces:**
- Consumes: `golang.org/x/sys/unix` (`Flock`, `LOCK_EX`, `LOCK_NB`, `EWOULDBLOCK`) y `golang.org/x/sys/windows` (`LockFileEx`, `LOCKFILE_EXCLUSIVE_LOCK`, `LOCKFILE_FAIL_IMMEDIATELY`, `ERROR_LOCK_VIOLATION`, `Overlapped`), ya en `go.mod` (v0.48.0).
- Produces (privadas de `ipc`): `func lockPath(dataDir string) string`, `var errLocked error`, `func tryLock(path string) (*os.File, error)`, `func lockFile(f *os.File) error` (una por sistema); campo `Server.lock *os.File`. La Tarea 5 añade `waitLock` a `lock.go`.

- [ ] **Step 1: Test que falla.** Añadir al final de `host/internal/ipc/ipc_test.go`:

```go

// V29: the daemon holds a lock for as long as it serves, so a second Listen
// in the same directory fails even when the first one's socket is gone: two
// serve that start at once could otherwise both find a dead socket, and the
// second remove the first one's live socket (Stat, Dial and Remove are not
// one step).
func TestSecondServerRefusedEvenWithoutTheSocket(t *testing.T) {
	dir := shortDir(t)
	s := startServer(t, dir, func(ctx context.Context, req Request, c *Conn) Exit { return Exit{} })
	secret, err := ReadSecret(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(SocketPath(dir)); err != nil {
		t.Fatal(err)
	}
	s2, err := Listen(dir, secret, func(ctx context.Context, req Request, c *Conn) Exit { return Exit{} })
	if !errors.Is(err, ErrAlreadyRunning) {
		if s2 != nil {
			s2.Close()
		}
		t.Fatalf("second server: %v", err)
	}
	s.Close()
	// The lock goes with the first server.
	s3, err := Listen(dir, secret, func(ctx context.Context, req Request, c *Conn) Exit { return Exit{} })
	if err != nil {
		t.Fatalf("after close: %v", err)
	}
	s3.Close()
}
```

- [ ] **Step 2: Ver que falla.**

```bash
cd host && go test -count=1 -timeout 5m ./internal/ipc -run TestSecondServerRefusedEvenWithoutTheSocket
```

Esperado (réplica, Windows: quitar el socket de un listener vivo funciona): `ipc_test.go:405: second server: <nil>` (el segundo servidor arrancó: secuestro).

- [ ] **Step 3: Crear el cerrojo.**

**Crear `host/internal/ipc/lock.go`:**

```go
package ipc

import (
	"errors"
	"os"
	"path/filepath"
)

// lockPath is the file a daemon keeps locked for as long as it serves
// (Listen to Close). It stays on disk: removing a lock file opens a race.
func lockPath(dataDir string) string { return filepath.Join(dataDir, "adbuddy.lock") }

// errLocked means someone else holds the lock.
var errLocked = errors.New("the lock is held")

// tryLock opens path (creating it, readable only by the user) and takes an
// exclusive lock on it without waiting: errLocked when another process, or
// another open of the same file, holds it. Closing the file releases the
// lock, and so does the end of the process, however it ends.
func tryLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
```

**Crear `host/internal/ipc/lock_unix.go`:**

```go
//go:build unix

package ipc

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// lockFile takes an exclusive flock on f without waiting. flock belongs to
// the open file, so a second open in the same process is refused too.
func lockFile(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return errLocked
	}
	return err
}
```

**Crear `host/internal/ipc/lock_windows.go`:**

```go
//go:build windows

package ipc

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// lockFile locks the first byte of f, exclusively and without waiting.
// LockFileEx belongs to the handle, so a second open in the same process is
// refused too.
func lockFile(f *os.File) error {
	err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, new(windows.Overlapped))
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return errLocked
	}
	return err
}
```

- [ ] **Step 4: Usarlo en `Listen` y soltarlo en `Close`.**

**Editar `host/internal/ipc/server.go`:** (cambio 1 de 3) reemplazar

```go
type Server struct {
	ln      net.Listener
	secret  string
```

por

```go
type Server struct {
	ln      net.Listener
	lock    *os.File // held until Close (lockPath)
	secret  string
```

**Editar `host/internal/ipc/server.go`:** (cambio 2 de 3) reemplazar

```go
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err == nil {
		if c, err := net.DialTimeout("unix", path, 500*time.Millisecond); err == nil {
			c.Close()
			return nil, ErrAlreadyRunning
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o600); err != nil {
			ln.Close()
			return nil, err
		}
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	return &Server{ln: ln, secret: secret, handler: h, ctx: ctx, cancel: cancel}, nil
}
```

por

```go
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	// One daemon per data directory, for as long as it serves: two serve that
	// start at once could otherwise both find a dead socket and the second
	// remove the first one's live socket (Stat, Dial and Remove are not one
	// step).
	lock, err := tryLock(lockPath(dataDir))
	if errors.Is(err, errLocked) {
		return nil, ErrAlreadyRunning
	}
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*Server, error) {
		lock.Close()
		return nil, err
	}
	if _, err := os.Stat(path); err == nil {
		// A daemon of an older version holds no lock, but it answers.
		if c, err := net.DialTimeout("unix", path, 500*time.Millisecond); err == nil {
			c.Close()
			return fail(ErrAlreadyRunning)
		}
		if err := os.Remove(path); err != nil {
			return fail(err)
		}
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return fail(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o600); err != nil {
			ln.Close()
			return fail(err)
		}
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	return &Server{ln: ln, lock: lock, secret: secret, handler: h, ctx: ctx, cancel: cancel}, nil
}
```

**Editar `host/internal/ipc/server.go`:** (cambio 3 de 3) reemplazar

```go
	s.cancel(ErrClosed)
	err := s.ln.Close()
	s.wg.Wait()
	return err
}
```

por

```go
	s.cancel(ErrClosed)
	err := s.ln.Close()
	s.wg.Wait()
	// Released after the socket is gone: the next daemon finds none alive.
	_ = s.lock.Close()
	return err
}
```

- [ ] **Step 5: Ver que pasa, en los tres sistemas.**

```bash
cd host && gofmt -l ./internal/ipc && go vet ./internal/ipc && GOOS=linux go vet ./internal/ipc && GOOS=darwin go vet ./internal/ipc && go test -count=1 -timeout 5m ./internal/ipc
```

Esperado: sin salida de `gofmt` ni de los `vet`; `ok  …/internal/ipc` (réplica: 0.6 s). `TestSecondServerRefusedAndStaleSocketReplaced` sigue en verde (el socket muerto se sustituye; el segundo `Listen` sigue diciendo `ErrAlreadyRunning`).

- [ ] **Step 6: Suite completa** (la CLI arranca `serve` muchas veces en la misma carpeta: cada `serve` suelta el cerrojo al cerrar, y en Windows la limpieza de `t.TempDir` fallaría si un fichero quedara abierto).

```bash
cd host && go build ./... && go vet ./... && gofmt -l . && go test -count=1 -timeout 15m ./...
```

Esperado: todo `ok`.

- [ ] **Step 7: Commit.**

```bash
git add host/internal/ipc/lock.go host/internal/ipc/lock_unix.go host/internal/ipc/lock_windows.go host/internal/ipc/server.go host/internal/ipc/ipc_test.go
git commit -m "fix(host): un solo daemon por carpeta de datos, con un cerrojo de sistema mientras sirve

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Concurrencia:** el cerrojo es del sistema (entre procesos) y también entre dos aperturas del mismo proceso (`flock` por descripción de fichero abierta, `LockFileEx` por handle), así que el test de un solo proceso es representativo. `Close` puede llamarse dos veces (los tests lo hacen): el segundo `lock.Close()` devuelve un error que se ignora. **Para el revisor:** `flock` en Linux y macOS lo ejerce la CI; aquí solo `GOOS=… go vet`.

---

### Task 5: El secreto se escribe entero y se lee entero (V29) 🔒

**Revisor:** `adbuddy-reviewer-opus` (IPC: secreto de autenticación).

**Files:**
- Modify: `host/internal/ipc/lock.go` (`waitLock`)
- Modify: `host/internal/ipc/ipc.go` (`EnsureSecret`, `ReadSecret`, import `time`)
- Test: `host/internal/ipc/ipc_test.go` (tres tests al final, imports `io/fs` y `path/filepath`)

**Interfaces:**
- Consumes: `tryLock`, `errLocked` (Tarea 4).
- Produces: `func waitLock(path string, wait time.Duration) (*os.File, error)`; privadas `secretLockPath`, `secretWait`, `secretTries`, `secretPause`, `readSecretOnce`, y `var secretWritten func(tmp string)` (gancho de test). `EnsureSecret` y `ReadSecret` conservan su firma.

- [ ] **Step 1: Tests que fallan con el código de hoy.** Imports del test:

**Editar `host/internal/ipc/ipc_test.go`:** (cambio 1 de 2) reemplazar

```go
	"io"
	"os"
	"runtime"
```

por

```go
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
```

Y añadir al final de `host/internal/ipc/ipc_test.go`:

```go

// V29: many creators at once agree on one secret, and a reader never sees a
// corrupt one meanwhile.
func TestSecretCreatedOnceUnderARace(t *testing.T) {
	dir := shortDir(t)
	setTries(t, 500) // a slow CI must not turn a rename in progress into an error
	const n = 16
	got := make(chan string, n)
	stop := make(chan struct{})
	stopped := false
	defer func() { // a Fatalf must not leave the reader running
		if !stopped {
			close(stop)
		}
	}()
	readerErr := make(chan error, 1)
	go func() {
		for {
			select {
			case <-stop:
				readerErr <- nil
				return
			default:
			}
			if _, err := ReadSecret(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
				readerErr <- err
				return
			}
		}
	}()
	for range n {
		go func() {
			s, err := EnsureSecret(dir)
			if err != nil {
				t.Error(err)
			}
			got <- s
		}()
	}
	first := <-got
	for range n - 1 {
		if s := <-got; s != first {
			t.Fatalf("two secrets: %q and %q", first, s)
		}
	}
	close(stop)
	stopped = true
	if err := <-readerErr; err != nil {
		t.Fatalf("a reader saw %v", err)
	}
}

// setTries lengthens (or shortens) ReadSecret's retries for one test.
func setTries(t *testing.T, n int) {
	t.Helper()
	old := secretTries
	secretTries = n
	t.Cleanup(func() { secretTries = old })
}

// V29: a secret that is not whole yet (an older host writes it in two steps)
// is read again for a moment instead of failing as corrupt.
func TestReadSecretWaitsForAPartialOne(t *testing.T) {
	dir := shortDir(t)
	setTries(t, 500) // up to 5 s for a write that comes after 50 ms, on any CI
	full := strings.Repeat("ab", 32)
	if err := os.WriteFile(SecretPath(dir), []byte(full[:10]), 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		done <- os.WriteFile(SecretPath(dir), []byte(full), 0o600)
	}()
	s, err := ReadSecret(dir)
	if werr := <-done; werr != nil {
		t.Fatal(werr)
	}
	if err != nil || s != full {
		t.Fatalf("%q %v", s, err)
	}
	// A corrupt one stays corrupt, after the retries.
	secretTries = 3
	if err := os.WriteFile(SecretPath(dir), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSecret(dir); err == nil || !strings.Contains(err.Error(), "is corrupt") {
		t.Fatalf("corrupt: %v", err)
	}
}
```

- [ ] **Step 2: Ver que fallan.**

```bash
cd host && go test -count=10 -timeout 2m ./internal/ipc -run 'TestSecretCreatedOnceUnderARace|TestReadSecretWaitsForAPartialOne'
```

Esperado: el paquete de test no compila (`undefined: secretTries`, que `setTries` alarga y que hoy no existe): es el fallo esperado. Comportamiento con el código de hoy, comprobado al redactar el plan con las dos pruebas sin `setTries`: `TestReadSecretWaitsForAPartialOne` falla siempre (`"" …ipc.secret is corrupt`) y `TestSecretCreatedOnceUnderARace` en algunas vueltas (réplica, 10 vueltas: 2 fallos, con `two secrets: "" and "595b…"` y `a reader saw …\ipc.secret is corrupt`).

- [ ] **Step 3: `waitLock`.**

**Editar `host/internal/ipc/lock.go`:** (cambio 1 de 2) reemplazar

```go
import (
	"errors"
	"os"
	"path/filepath"
)
```

por

```go
import (
	"errors"
	"os"
	"path/filepath"
	"time"
)
```

**Editar `host/internal/ipc/lock.go`:** (cambio 2 de 2) reemplazar

```go
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
```

por

```go
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// waitLock is tryLock retried every 10 ms until wait has passed.
func waitLock(path string, wait time.Duration) (*os.File, error) {
	deadline := time.Now().Add(wait)
	for {
		f, err := tryLock(path)
		if !errors.Is(err, errLocked) || time.Now().After(deadline) {
			return f, err
		}
		time.Sleep(10 * time.Millisecond)
	}
}
```

- [ ] **Step 4: Escritura con renombrado y lectura con reintentos.**

**Editar `host/internal/ipc/ipc.go`:** (cambio 1 de 3) reemplazar

```go
	"path/filepath"
	"strings"
)
```

por

```go
	"path/filepath"
	"strings"
	"time"
)
```

**Editar `host/internal/ipc/ipc.go`:** (cambio 2 de 3) reemplazar

```go
// EnsureSecret returns the installation secret, creating it (256 random
// bits, readable only by the user) on first use.
func EnsureSecret(dataDir string) (string, error) {
	if s, err := ReadSecret(dataDir); err == nil {
		return s, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", err
	}
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(b[:])
	f, err := os.OpenFile(SecretPath(dataDir), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return ReadSecret(dataDir) // another process won the race
	}
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(secret); err != nil {
		f.Close()
		return "", err
	}
	return secret, f.Close()
}

// ReadSecret reads the installation secret.
func ReadSecret(dataDir string) (string, error) {
	b, err := os.ReadFile(SecretPath(dataDir))
```

por

```go
// secretLockPath is held while the secret is created: one creator at a time.
func secretLockPath(dataDir string) string { return filepath.Join(dataDir, "ipc.secret.lock") }

// secretWait bounds the wait for another process that is creating the secret.
const secretWait = 5 * time.Second

// secretWritten, when not nil (tests), is called with the temporary file once
// the whole secret is in it, before it takes the secret's name.
var secretWritten func(tmp string)

// EnsureSecret returns the installation secret, creating it (256 random
// bits, readable only by the user) on first use. It is written whole under a
// temporary name in the same folder (same permissions and, on Windows, the
// folder's ACL) and then renamed, so a reader sees no secret or all of it;
// creators take turns (secretLockPath) and a later one reads the first one's.
func EnsureSecret(dataDir string) (string, error) {
	if s, err := ReadSecret(dataDir); err == nil {
		return s, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", err
	}
	lock, err := waitLock(secretLockPath(dataDir), secretWait)
	if err != nil {
		return "", fmt.Errorf("another ADBuddy is creating %s: %w", SecretPath(dataDir), err)
	}
	defer lock.Close()
	if s, err := ReadSecret(dataDir); err == nil {
		return s, nil // another process created it meanwhile
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(b[:])
	f, err := os.CreateTemp(dataDir, "ipc.secret.*.tmp")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // nothing left once renamed
	if _, err := f.WriteString(secret); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if secretWritten != nil {
		secretWritten(tmp)
	}
	if err := os.Rename(tmp, SecretPath(dataDir)); err != nil {
		return "", err
	}
	return secret, nil
}

// A secret that cannot be read whole is read again for a moment: on Windows
// a rename in progress refuses to open it, and a host of an older version
// wrote it in two steps (create, then write). Variables so a test can
// lengthen the wait (a slow CI with -race).
var (
	secretTries = 20
	secretPause = 10 * time.Millisecond
)

// ReadSecret reads the installation secret. No secret at all is answered at
// once (fs.ErrNotExist); any other failure is retried for secretTries.
func ReadSecret(dataDir string) (string, error) {
	var err error
	for i := range secretTries {
		if i > 0 {
			time.Sleep(secretPause)
		}
		var s string
		if s, err = readSecretOnce(dataDir); err == nil || errors.Is(err, fs.ErrNotExist) {
			return s, err
		}
	}
	return "", err
}

func readSecretOnce(dataDir string) (string, error) {
	b, err := os.ReadFile(SecretPath(dataDir))
```

(El resto de la antigua `ReadSecret`, el `TrimSpace` y la comprobación de 64 caracteres, queda como cuerpo de `readSecretOnce` sin cambios.)

- [ ] **Step 5: Test del gancho (nadie ve el secreto a medias, y el segundo creador espera al primero).** Añadir al final de `host/internal/ipc/ipc_test.go`:

```go

// V29: the secret is written whole under a temporary name and then renamed,
// so nobody reads a part of it; and a second creator waits for the first and
// takes its secret.
func TestSecretIsNeverSeenHalfWritten(t *testing.T) {
	dir := shortDir(t)
	second := make(chan string, 1)
	calls := 0
	secretWritten = func(tmp string) {
		calls++
		if calls > 1 {
			t.Error("the secret was created twice")
			return
		}
		if _, err := ReadSecret(dir); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("before the rename a reader got %v, want no secret", err)
		}
		if b, err := os.ReadFile(tmp); err != nil || len(b) != 64 {
			t.Errorf("temporary file %q: %d bytes, %v", tmp, len(b), err)
		}
		go func() {
			s, err := EnsureSecret(dir) // waits for this one to finish
			if err != nil {
				t.Error(err)
			}
			second <- s
		}()
		time.Sleep(50 * time.Millisecond) // the second one is waiting by now
	}
	t.Cleanup(func() { secretWritten = nil })
	first, err := EnsureSecret(dir)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("secretWritten called %d times", calls)
	}
	select {
	case s := <-second:
		if s != first {
			t.Fatalf("second creator got %q, want %q", s, first)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the second creator never returned")
	}
	if s, err := ReadSecret(dir); err != nil || s != first {
		t.Fatalf("file %q %v", s, err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "ipc.secret.*.tmp")); len(left) != 0 {
		t.Fatalf("temporary files left: %q", left)
	}
}
```

- [ ] **Step 6: Ver que pasan, también repetidos.**

```bash
cd host && gofmt -l ./internal/ipc && go vet ./internal/ipc && GOOS=linux go vet ./internal/ipc && GOOS=darwin go vet ./internal/ipc && go test -count=1 -timeout 5m ./internal/ipc && go test -count=30 -timeout 5m ./internal/ipc -run 'TestSecret|TestReadSecret'
```

Esperado: `ok` las dos veces (réplica: 1.1 s y 13 s). Sin los reintentos de `ReadSecret`, la carrera falla en Windows con `open …\ipc.secret: The process cannot access the file because it is being used by another process.` (visto en la réplica antes de añadirlos).

- [ ] **Step 7: Suite completa y commit.**

```bash
cd host && go build ./... && go vet ./... && gofmt -l . && go test -count=1 -timeout 15m ./...
git add host/internal/ipc/lock.go host/internal/ipc/ipc.go host/internal/ipc/ipc_test.go
git commit -m "fix(host): el secreto del IPC se crea con renombrado bajo cerrojo y se relee si no está entero

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Concurrencia:** el cerrojo de creación serializa a los creadores (también dentro de un proceso, por la misma razón que en la Tarea 4). `secretWritten` lo escribe el test antes de cualquier gorrutina y lo borra en `Cleanup`, cuando la segunda gorrutina ya terminó; en `TestSecretCreatedOnceUnderARace` vale `nil` y solo se lee. `calls` lo tocan el gancho y el test en la misma gorrutina (el segundo `EnsureSecret` encuentra el secreto y no llama al gancho). La CI con `-race` lo confirma.

---

### Task 6: Límite de conexiones sin autenticar y tramas que no reservan antes de llegar (V29) 🔒

**Revisor:** `adbuddy-reviewer-opus` (IPC).

**Files:**
- Modify: `host/internal/ipc/ipc.go` (`ReadFrame`, constante `frameChunk`, import `slices`)
- Modify: `host/internal/ipc/server.go` (`maxPreauth`, campo `preauth`, `Listen`, `Serve`, `handle`)
- Test: `host/internal/ipc/ipc_test.go` (dos tests al final, imports `encoding/binary` y `net`)

**Interfaces:**
- Consumes: el cerrojo y `Listen` de la Tarea 4 (el `return &Server{…}` que aquí se amplía).
- Produces: privadas `const maxPreauth = 32`, `const frameChunk = 64 << 10`, campo `Server.preauth chan struct{}`. `ReadFrame` conserva su firma y su contrato (errores incluidos: un final antes de tiempo es `io.ErrUnexpectedEOF`).

- [ ] **Step 1: Tests que fallan.** Imports:

**Editar `host/internal/ipc/ipc_test.go`:** reemplazar

```go
	"crypto/rand"
	"errors"
```

por

```go
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
```

y reemplazar

```go
	"io/fs"
	"os"
```

por

```go
	"io/fs"
	"net"
	"os"
```

Añadir al final de `host/internal/ipc/ipc_test.go`:

```go

// sizeReader records the largest buffer it is asked to fill: what ReadFrame
// has reserved by then, measured without looking at the whole heap.
type sizeReader struct {
	r   io.Reader
	max int
}

func (s *sizeReader) Read(p []byte) (int, error) {
	s.max = max(s.max, len(p))
	return s.r.Read(p)
}

// V29: a header that announces a whole frame does not make ReadFrame reserve
// it before the bytes arrive: what it holds grows with what was read.
func TestReadFrameGrowsWithWhatArrives(t *testing.T) {
	hdr := []byte{FrameRequest, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(hdr[1:], MaxFrame)
	r := &sizeReader{r: io.MultiReader(bytes.NewReader(hdr), strings.NewReader("only this"))}
	_, _, err := ReadFrame(r)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err %v", err)
	}
	if r.max > 64<<10 {
		t.Fatalf("asked to fill %d bytes at once, for 9 that arrived", r.max)
	}
	// A whole frame, larger than the first chunk, still comes back whole.
	var buf bytes.Buffer
	want := bytes.Repeat([]byte("0123456789"), 30_000)
	if err := WriteFrame(&buf, FrameStdout, want); err != nil {
		t.Fatal(err)
	}
	typ, p, err := ReadFrame(&buf)
	if err != nil || typ != FrameStdout || !bytes.Equal(p, want) {
		t.Fatalf("%c %d bytes %v", typ, len(p), err)
	}
}

// V29: at most maxPreauth connections wait to authenticate at once; one more
// is closed right away, and the slots free up when those connections go.
func TestUnauthenticatedConnectionsAreBounded(t *testing.T) {
	dir := shortDir(t)
	startServer(t, dir, func(ctx context.Context, req Request, c *Conn) Exit { return Exit{} })
	var idle []net.Conn
	t.Cleanup(func() {
		for _, c := range idle {
			c.Close()
		}
	})
	for range maxPreauth {
		c, err := net.Dial("unix", SocketPath(dir))
		if err != nil {
			t.Fatal(err)
		}
		idle = append(idle, c)
	}
	extra, err := net.Dial("unix", SocketPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer extra.Close()
	_ = extra.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, err = extra.Read(make([]byte, 1))
	var ne net.Error
	if err == nil || errors.As(err, &ne) && ne.Timeout() {
		t.Fatalf("the connection over the limit was kept: %v", err)
	}
	for _, c := range idle {
		c.Close()
	}
	idle = nil
	deadline := time.Now().Add(10 * time.Second)
	for {
		exit, err := Call(context.Background(), dir, Request{Kind: KindPing}, nil, io.Discard, io.Discard, nil)
		if err == nil && exit.Code == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not served again: %+v %v", exit, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	// A refused request keeps its slot only until its client is gone: more
	// refusals in a row than there are slots, and a real request is served.
	bad, err := json.Marshal(Request{Secret: strings.Repeat("0", 64), Kind: KindPing})
	if err != nil {
		t.Fatal(err)
	}
	for i := range maxPreauth + 8 {
		c, err := net.Dial("unix", SocketPath(dir))
		if err != nil {
			t.Fatal(err)
		}
		_ = c.SetDeadline(time.Now().Add(5 * time.Second))
		if err := WriteFrame(c, FrameRequest, bad); err != nil {
			t.Fatalf("refusal %d: %v", i, err)
		}
		typ, p, err := ReadFrame(c)
		c.Close()
		if err != nil || typ != FrameExit || !strings.Contains(string(p), "unauthorized") {
			t.Fatalf("refusal %d: %c %q %v", i, typ, p, err)
		}
	}
	deadline = time.Now().Add(10 * time.Second)
	for {
		exit, err := Call(context.Background(), dir, Request{Kind: KindPing}, nil, io.Discard, io.Discard, nil)
		if err == nil && exit.Code == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not served after the refusals: %+v %v", exit, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
```

- [ ] **Step 2: Ver que fallan.**

```bash
cd host && go test -count=1 -timeout 2m ./internal/ipc -run 'TestReadFrameGrows|TestUnauthenticated'
```

Esperado: el paquete de test no compila (`undefined: maxPreauth`): es el fallo esperado. Comportamiento con el código de hoy, comprobado en la réplica definiendo `maxPreauth` solo en un fichero de test temporal (que no se commitea):

```
--- FAIL: TestReadFrameGrowsWithWhatArrives (0.00s)
    ipc_test.go:557: asked to fill 1048576 bytes at once, for 9 that arrived
--- FAIL: TestUnauthenticatedConnectionsAreBounded (3.01s)
    ipc_test.go:598: the connection over the limit was kept: read unix @->…\adbuddy.sock: i/o timeout
```

- [ ] **Step 3: `ReadFrame` crece con lo que llega.**

**Editar `host/internal/ipc/ipc.go`:** (cambio 1 de 2) reemplazar

```go
	"path/filepath"
	"strings"
	"time"
)
```

por

```go
	"path/filepath"
	"slices"
	"strings"
	"time"
)
```

**Editar `host/internal/ipc/ipc.go`:** (cambio 2 de 2) reemplazar

```go
// ReadFrame reads one frame.
func ReadFrame(r io.Reader) (byte, []byte, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[1:])
	if n > MaxFrame {
		return 0, nil, fmt.Errorf("frame of %d bytes exceeds %d", n, MaxFrame)
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return hdr[0], payload, nil
}
```

por

```go
// frameChunk is the most ReadFrame reserves before the bytes arrive. Past it
// the buffer doubles only with what has been read, so a header that announces
// MaxFrame (from a client that has not authenticated yet) costs nothing until
// the payload really comes.
const frameChunk = 64 << 10

// ReadFrame reads one frame.
func ReadFrame(r io.Reader) (byte, []byte, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := int(binary.BigEndian.Uint32(hdr[1:]))
	if n > MaxFrame {
		return 0, nil, fmt.Errorf("frame of %d bytes exceeds %d", n, MaxFrame)
	}
	payload := make([]byte, min(n, frameChunk))
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	for len(payload) < n {
		start := len(payload)
		more := min(n-start, start)
		payload = slices.Grow(payload, more)[:start+more]
		if _, err := io.ReadFull(r, payload[start:]); err != nil {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF // part of the frame came
			}
			return 0, nil, err
		}
	}
	return hdr[0], payload, nil
}
```

- [ ] **Step 4: Plazas para las conexiones sin autenticar.**

**Editar `host/internal/ipc/server.go`:** (cambio 1 de 4) reemplazar

```go
// Server accepts requests on the daemon socket.
type Server struct {
	ln      net.Listener
	lock    *os.File // held until Close (lockPath)
	secret  string
	handler Handler
	ctx     context.Context
	cancel  context.CancelCauseFunc
	wg      sync.WaitGroup
	mu      sync.Mutex
	closed  bool
}
```

por

```go
// maxPreauth bounds the connections that have not authenticated yet: each
// may keep the server reading a request (up to MaxFrame) for 5 seconds. One
// over it is closed at once.
const maxPreauth = 32

// Server accepts requests on the daemon socket.
type Server struct {
	ln      net.Listener
	lock    *os.File      // held until Close (lockPath)
	preauth chan struct{} // a slot per connection that has not authenticated yet
	secret  string
	handler Handler
	ctx     context.Context
	cancel  context.CancelCauseFunc
	wg      sync.WaitGroup
	mu      sync.Mutex
	closed  bool
}
```

**Editar `host/internal/ipc/server.go`:** (cambio 2 de 4) reemplazar

```go
	return &Server{ln: ln, lock: lock, secret: secret, handler: h, ctx: ctx, cancel: cancel}, nil
```

por

```go
	return &Server{ln: ln, lock: lock, preauth: make(chan struct{}, maxPreauth), secret: secret, handler: h, ctx: ctx, cancel: cancel}, nil
```

**Editar `host/internal/ipc/server.go`:** (cambio 3 de 4) reemplazar

```go
			return err
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			nc.Close()
			return nil
		}
		s.wg.Add(1)
		s.mu.Unlock()
		go func() {
			defer s.wg.Done()
			s.handle(nc)
		}()
	}
}
```

por

```go
			return err
		}
		select {
		case s.preauth <- struct{}{}: // handle gives it back once authenticated or gone
		default:
			nc.Close() // too many connections waiting to authenticate
			continue
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			<-s.preauth
			nc.Close()
			return nil
		}
		s.wg.Add(1)
		s.mu.Unlock()
		go func() {
			defer s.wg.Done()
			s.handle(nc)
		}()
	}
}
```

**Editar `host/internal/ipc/server.go`:** (cambio 4 de 4) reemplazar

```go
func (s *Server) handle(nc net.Conn) {
	defer nc.Close()
	_ = nc.SetReadDeadline(time.Now().Add(5 * time.Second))
	typ, payload, err := ReadFrame(nc)
	if err != nil || typ != FrameRequest {
		return
	}
	var req Request
	if json.Unmarshal(payload, &req) != nil {
		return
	}
	c := &Conn{nc: nc, resize: make(chan [2]int, 4)}
	if subtle.ConstantTimeCompare([]byte(req.Secret), []byte(s.secret)) != 1 {
		writeExit(c, Exit{Code: 1, Error: "unauthorized"})
		finish(nc, nil)
		return
	}
```

por

```go
func (s *Server) handle(nc net.Conn) {
	defer nc.Close()
	release := sync.OnceFunc(func() { <-s.preauth }) // the slot Serve took for it
	defer release()
	_ = nc.SetReadDeadline(time.Now().Add(5 * time.Second))
	typ, payload, err := ReadFrame(nc)
	if err != nil || typ != FrameRequest {
		return
	}
	var req Request
	if json.Unmarshal(payload, &req) != nil {
		return
	}
	c := &Conn{nc: nc, resize: make(chan [2]int, 4)}
	if subtle.ConstantTimeCompare([]byte(req.Secret), []byte(s.secret)) != 1 {
		// Refused: it keeps its slot (the deferred release) while finish
		// waits up to 2 s for the client to close, so refusals stay bounded.
		writeExit(c, Exit{Code: 1, Error: "unauthorized"})
		finish(nc, nil)
		return
	}
	release() // authenticated: it no longer waits
```

- [ ] **Step 5: Ver que pasan.**

```bash
cd host && gofmt -l ./internal/ipc && go vet ./internal/ipc && GOOS=linux go vet ./internal/ipc && GOOS=darwin go vet ./internal/ipc && go test -count=1 -timeout 5m ./internal/ipc && go test -count=5 -timeout 5m ./internal/ipc
```

Esperado: `ok` las dos (réplica: 1.1 s y 4.6 s). `TestStdinTooLargeCancels`, `TestBinaryOutputLargerThanAFrame` y `TestWrongSecretIsRejected` siguen en verde.

- [ ] **Step 6: Suite completa con los tres sistemas y commit.**

```bash
cd host && go build ./... && go vet ./... && gofmt -l . && GOOS=linux go vet ./... && GOOS=darwin go vet ./... && go test -count=1 -timeout 15m ./...
git add host/internal/ipc/ipc.go host/internal/ipc/server.go host/internal/ipc/ipc_test.go
git commit -m "fix(host): el IPC limita las conexiones sin autenticar y no reserva una trama antes de recibirla

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

Esperado: todo `ok` (réplica: 29 paquetes).

**Concurrencia:** la plaza la toma `Serve` (una gorrutina, en orden de aceptación) y la devuelve `handle` exactamente una vez (`sync.OnceFunc`: tras la comprobación del secreto, o al salir si la conexión se va antes). Si `Serve` ve el servidor cerrado devuelve la plaza que acaba de tomar. Por eso el test es determinista: cuando se acepta la conexión 33, las 32 anteriores ya tienen su plaza. **Para el revisor:** la comparación del secreto sigue siendo `subtle.ConstantTimeCompare` y nada cambia antes de autenticar salvo cuánta memoria se reserva.

---

### Task 7: La terminal solo carga ficheros de su carpeta por su nombre llano (V21)

**Revisor:** `adbuddy-reviewer`.

**Files:**
- Modify: `android/app/src/main/kotlin/tech/hybriq/adbuddy/term/TermWebPolicy.kt`
- Test: `android/app/src/test/kotlin/tech/hybriq/adbuddy/term/TermWebPolicyTest.kt`

**Interfaces:**
- Consumes: `TermWebPolicy.PREFIX`, usada por `TerminalView.kt` (`shouldInterceptRequest`, sin cambios).
- Produces: `TermWebPolicy.allowed(url: String): Boolean` con la misma firma; constante privada `NAME`.

- [ ] **Step 1: Tests que fallan.**

**Editar `android/app/src/test/kotlin/tech/hybriq/adbuddy/term/TermWebPolicyTest.kt`:** (cambio 1 de 2) reemplazar

```kotlin
class TermWebPolicyTest {
    @Test fun onlyTheBundledTerminalAssetsLoad() {
```

por

```kotlin
class TermWebPolicyTest {
    private val base = TermWebPolicy.PREFIX

    @Test fun onlyTheBundledTerminalAssetsLoad() {
```

**Editar `android/app/src/test/kotlin/tech/hybriq/adbuddy/term/TermWebPolicyTest.kt`:** (cambio 2 de 2) reemplazar

```kotlin
            "file:///android_asset/term/index.html", "content://x/y", "data:text/html,hi", "javascript:alert(1)", "",
        )) assertFalse(u, TermWebPolicy.allowed(u))
    }
}
```

por

```kotlin
            "file:///android_asset/term/index.html", "content://x/y", "data:text/html,hi", "javascript:alert(1)", "",
        )) assertFalse(u, TermWebPolicy.allowed(u))
    }

    /** Every file the page loads (index.html and what it links), and the source maps a debugger may ask for. */
    @Test fun theFilesOfTheFolderLoadByTheirPlainName() {
        for (name in listOf("index.html", "xterm.js", "xterm.css", "term.js", "term.css", "addon-fit.js", "addon-web-links.js", "xterm.js.map")) {
            assertTrue(name, TermWebPolicy.allowed(base + name))
        }
    }

    /** V21: one file of the folder by its plain name, nothing else: no query, fragment, sub-folder, escape or odd character. */
    @Test fun edgeCasesAreBlocked() {
        for (rest in listOf(
            "", "?x=1", "index.html?x=1", "#frag", "index.html#frag", "a?/../b", "%2F", "index%2Ehtml", "..", "...", ".",
            ".hidden", "/index.html", "sub/index.html", "index.html/", "a\\b", "a b", "índex.html", "index.html\u0000",
        )) assertFalse("rest \"$rest\"", TermWebPolicy.allowed(base + rest))
        for (u in listOf(
            "HTTPS://APPASSETS.ANDROIDPLATFORM.NET/assets/term/index.html",
            "https://appassets.androidplatform.net/ASSETS/TERM/index.html",
            "https://appassets.androidplatform.net//assets/term/index.html",
            "https://appassets.androidplatform.net:443/assets/term/index.html",
            "https://user@appassets.androidplatform.net/assets/term/index.html",
            "https://appassets.androidplatform.net@example.com/assets/term/index.html",
        )) assertFalse(u, TermWebPolicy.allowed(u))
    }
}
```

- [ ] **Step 2: Ver que fallan.**

```bash
export JAVA_HOME="/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1"; export PATH="$JAVA_HOME/bin:/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin:$PATH"; cd android && gradle :app:testDebugUnitTest --offline --tests 'tech.hybriq.adbuddy.term.TermWebPolicyTest'
```

Esperado: `4 tests completed, 1 failed`; en el XML, `java.lang.AssertionError: rest "?x=1"` (el primer caso que hoy pasa la política).

- [ ] **Step 3: Implementar.**

**Editar `android/app/src/main/kotlin/tech/hybriq/adbuddy/term/TermWebPolicy.kt`:** reemplazar

```kotlin
    fun allowed(url: String): Boolean {
        if (!url.startsWith(PREFIX)) return false
        val rest = url.removePrefix(PREFIX)
        // No way out of the folder, however it is written.
        return rest.isNotEmpty() && !rest.contains("..") && !rest.contains('%') && !rest.contains('\\')
    }
}
```

por

```kotlin
    /** A plain file name of the folder (it is flat): ASCII letters, digits, '.', '-' and '_', not starting with '.'. */
    private val NAME = Regex("[A-Za-z0-9_-][A-Za-z0-9._-]*")

    /**
     * One file of the folder by its plain name, and nothing else: no query, fragment, sub-folder, escape (%),
     * backslash or "..". The page and what it links are plain names; an allowlist leaves no way out of the
     * folder however the URL is written.
     */
    fun allowed(url: String): Boolean {
        if (!url.startsWith(PREFIX)) return false
        val rest = url.removePrefix(PREFIX)
        return NAME.matches(rest) && !rest.contains("..")
    }
}
```

- [ ] **Step 4: Ver que pasan, y la suite de la app.**

```bash
export JAVA_HOME="/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1"; export PATH="$JAVA_HOME/bin:/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin:$PATH"; cd android && gradle :app:testDebugUnitTest --offline --tests 'tech.hybriq.adbuddy.term.TermWebPolicyTest' && gradle :app:testDebugUnitTest --offline
```

Esperado: `BUILD SUCCESSFUL` las dos.

- [ ] **Step 5: Commit.**

```bash
git add android/app/src/main/kotlin/tech/hybriq/adbuddy/term/TermWebPolicy.kt android/app/src/test/kotlin/tech/hybriq/adbuddy/term/TermWebPolicyTest.kt
git commit -m "fix(app): la terminal solo carga ficheros de su carpeta por su nombre llano

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: Tests de `Bridge`: socket viejo y reentrada de `onResult` (V42)

**Revisor:** `adbuddy-reviewer`.

**Files:**
- Test: `android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeTest.kt` (imports, `Host.holdClose`, dos tests antes de `nothingIsSentBeforeTheWelcome`)

**Interfaces:**
- Consumes: `Bridge(scope, appVersion, Timing, onMessage)` (constructor `internal`), `bridge.start/stop/request/listen/state`, y los helpers del test (`hosts`, `link`, `connect`, `fast`, `token`, `tokenB`).
- Produces: `Host.holdClose` (el host no contesta al cierre de la app).

- [ ] **Step 1: Escribir los tests.**

**Editar `android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeTest.kt`:** (cambio 1 de 5) reemplazar

```kotlin
import java.util.concurrent.CountDownLatch
```

por

```kotlin
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
```

(cambio 2 de 5) reemplazar

```kotlin
import kotlinx.coroutines.SupervisorJob
```

por

```kotlin
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.asCoroutineDispatcher
```

(cambio 3 de 5) reemplazar

```kotlin
        @Volatile var ws: WebSocket? = null
```

por

```kotlin
        @Volatile var ws: WebSocket? = null

        /** Does not answer the app's close: the socket stays readable on the app's side and the host can still write. */
        @Volatile var holdClose = false
```

(cambio 4 de 5) reemplazar

```kotlin
        override fun onClosing(webSocket: WebSocket, code: Int, reason: String) { webSocket.close(1000, null) }
```

por

```kotlin
        override fun onClosing(webSocket: WebSocket, code: Int, reason: String) { if (!holdClose) webSocket.close(1000, null) }
```

(cambio 5 de 5) reemplazar

```kotlin
    /** Before the welcome nothing is sent: the host reads the hello first. */
```

por

```kotlin
    /**
     * V42: what an old socket still delivers after a new link replaced it changes nothing: not the state, not a
     * listener, not the request that waits on it, which fails with null when its connection ends. The old loop is
     * held on a one-thread dispatcher, and the host does not answer the app's close: the old socket stays open
     * while the host writes on it.
     */
    @Test fun anOldSocketIsIgnoredAfterANewLink() {
        val single = Executors.newSingleThreadExecutor()
        val held = CoroutineScope(SupervisorJob() + single.asCoroutineDispatcher())
        val gate = CountDownLatch(1)
        try {
            val bridge = Bridge(held, "0.3.0", fast) { _, _ -> }
            val errors = LinkedBlockingQueue<Message>()
            bridge.listen(Protocol.ERROR) { errors.add(it) }
            hosts[0].holdClose = true
            bridge.start(link())
            connect(bridge, hosts[0])
            val result = CompletableFuture<Message?>()
            val rid = bridge.request(Protocol.CAPTURE_REQUEST, JsonObject(emptyMap())) { result.complete(it) }
            assertEquals(rid, hosts[0].next().rid)

            single.execute { gate.await() } // the old loop cannot end its connection until the gate opens
            bridge.start(link(tokenB))
            hosts[0].send("""{"v":1,"seq":7,"type":"session.state","ts":"x","payload":{"state":"working"}}""")
            hosts[0].send("""{"v":1,"rid":"$rid","type":"error","ts":"x","payload":{"code":"failed","text":"x"}}""")
            Thread.sleep(300)
            assertEquals("the old socket did not change the state", "idle", bridge.state.value.sessionState.state)
            assertEquals(0L, bridge.state.value.lastSeq)
            assertEquals("no listener heard the old socket", null, errors.poll(100, TimeUnit.MILLISECONDS))
            assertFalse("the old socket did not answer the request", result.isDone)

            gate.countDown()
            assertEquals("its connection ended: the request fails", null, result.get(5, TimeUnit.SECONDS))
            assertEquals(tokenB, connect(bridge, hosts[1]).token)
            bridge.stop()
        } finally {
            gate.countDown()
            hosts[0].ws?.close(1000, null) // it read the app's close and waits to send its own: MockWebServer would wait for it
            held.cancel()
            single.shutdownNow()
        }
    }

    /**
     * V42: onResult runs outside the lock, so it may send another request at once: from an answer (it goes out on
     * the same socket) and from a connection that ended (nothing to send it on: null, and the loop goes on).
     */
    @Test fun onResultMaySendAnotherRequest() {
        val bridge = Bridge(scope, "0.3.0", fast) { _, _ -> }
        bridge.start(link())
        connect(bridge, hosts[0])
        val inner = CompletableFuture<String?>()
        val first = bridge.request(Protocol.CAPTURE_REQUEST, JsonObject(emptyMap())) {
            inner.complete(bridge.request(Protocol.CAPTURE_REQUEST, JsonObject(emptyMap())) { })
        }
        assertEquals(first, hosts[0].next().rid)
        hosts[0].send("""{"v":1,"rid":"$first","type":"error","ts":"x","payload":{"code":"failed","text":"x"}}""")
        val second = inner.get(5, TimeUnit.SECONDS)
        assertNotNull("the request sent from onResult went out", second)
        assertEquals(second, hosts[0].next().rid)

        val fromFailure = CompletableFuture<String?>()
        assertNotNull(bridge.request(Protocol.CAPTURE_REQUEST, JsonObject(emptyMap())) { m ->
            if (m == null) fromFailure.complete(bridge.request(Protocol.CAPTURE_REQUEST, JsonObject(emptyMap())) { })
        })
        hosts[0].next()
        hosts[0].ws!!.close(1000, "restart")
        assertEquals("no socket to send it on", null, fromFailure.get(5, TimeUnit.SECONDS))
        assertEquals("the app connects again", Protocol.HELLO, hosts[1].next().type)
        bridge.stop()
    }

    /** Before the welcome nothing is sent: the host reads the hello first. */
```

- [ ] **Step 2: RED por mutación (ruling 10).** Quitar temporalmente, en `android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Bridge.kt`, la línea `if (webSocket !== socket) return` de `Listener.onMessage(webSocket, text)` (la que va justo después de `val (next, answered) = synchronized(lock) {`), y correr:

```bash
export JAVA_HOME="/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1"; export PATH="$JAVA_HOME/bin:/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin:$PATH"; cd android && gradle :app:testDebugUnitTest --offline --tests 'tech.hybriq.adbuddy.bridge.BridgeTest.anOldSocketIsIgnoredAfterANewLink'
```

Esperado (réplica): `1 test completed, 1 failed`, `org.junit.ComparisonFailure: the old socket did not change the state expected:<[idle]> but was:<[working]>`. **Restaurar la línea** (`git checkout -- android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/Bridge.kt`) y comprobar con `git status` que `Bridge.kt` no queda modificado.

- [ ] **Step 3: Ver que pasan.**

```bash
export JAVA_HOME="/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1"; export PATH="$JAVA_HOME/bin:/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin:$PATH"; cd android && gradle :app:testDebugUnitTest --offline --tests 'tech.hybriq.adbuddy.bridge.BridgeTest'
```

Esperado: `BUILD SUCCESSFUL` (réplica: 26 tests; `anOldSocketIsIgnoredAfterANewLink` 0.43 s, `onResultMaySendAnotherRequest` 0.13 s). Si el `tearDown` dice `Gave up waiting for queue to shut down`, falta el `hosts[0].ws?.close(1000, null)` del `finally` (el host leyó el cierre de la app y espera para mandar el suyo); `cancel()` no sirve en el lado del servidor (NPE en `RealWebSocket.cancel`).

- [ ] **Step 4: Commit.**

```bash
git add android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeTest.kt
git commit -m "test(app): Bridge ignora un socket viejo y onResult puede volver a pedir

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Concurrencia:** el bucle viejo se retiene en un ejecutor de un hilo ocupado por `gate.await()` (FIFO: la reanudación por cancelación queda detrás); OkHttp entrega los mensajes en sus propios hilos. El `finally` abre la puerta aunque falle una aserción, para que ningún hilo quede colgado.

---

### Task 9: Ningún reductor lanza con ningún payload: fuzz determinista (V42)

**Revisor:** `adbuddy-reviewer`.

**Files:**
- Create: `android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/ReducerFuzzTest.kt`

**Interfaces:**
- Consumes: `BridgeState.REDUCERS`, `BridgeState.apply/disconnected`, `Reducer.event/snapshot/reset`, `Reset`, `Protocol.*`, `Protocol.json`, `Protocol.payload<Snapshot>`, `Message`.
- Produces: nada que use otra tarea.

- [ ] **Step 1: Crear el test.**

**Crear `android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/ReducerFuzzTest.kt`:**

```kotlin
package tech.hybriq.adbuddy.bridge

import kotlin.random.Random
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.fail
import org.junit.Test

/**
 * V42: no reducer throws, whatever the payload (protocol spec §10). A reducer that throws on OkHttp's reader
 * thread cuts the socket, and the snapshot that comes again with the next connection would throw again: a
 * reconnection loop. Deterministic: fixed seeds, so a failure repeats; the message says which one.
 */
class ReducerFuzzTest {
    private val seeds = listOf(20261002L, 1L, 42L)

    /** A valid payload of each type the reducers read, which the generator then breaks a little or a lot. */
    private val templates: Map<String, String> = mapOf(
        Protocol.CHAT_MESSAGE to """{"id":"m1","role":"assistant","text":"hi","attachments":[{"id":"00112233445566778899aabbccddeeff","name":"a.png","mime":"image/png","size":3,"sha256":"x"}],"conversation":"c1"}""",
        Protocol.TOOL_ACTIVITY to """{"id":"t1","messageId":"a_m1","summary":"Run ls","status":"running"}""",
        Protocol.PERMISSION_REQUEST to """{"id":"p1","tool":"Bash","summary":"ls","detail":"ls -la","expiresAt":"2026-10-02T00:00:00Z","allowAlways":true}""",
        Protocol.PERMISSION_RESOLVED to """{"id":"p1","decision":"allow","by":"phone"}""",
        Protocol.REMOTE_STATE to """{"enabled":true,"address":"100.64.0.5:5555","expiresAt":"2026-10-02T12:00:00Z"}""",
        Protocol.SESSION_STATE to """{"agent":"claude","state":"working","activity":"x","paused":false,"conversation":"c1","view":"chat","pcAttached":false}""",
        Protocol.CONFIRM_REQUEST to """{"id":"j_1","level":"red","title":"Restart","rawCommand":"adb shell reboot","expiresAt":"2026-10-02T00:00:00Z","special":true}""",
        Protocol.JOURNAL_ENTRY to """{"id":"j_1","status":"ok","level":"red","argv":["shell","reboot"],"exitCode":0,"inverse":{"argv":["x"]},"backups":[]}""",
        Protocol.WELCOME to """{"session":"s_1","fromSeq":1,"protocol":1}""",
    )

    private val keys = listOf(
        "id", "role", "text", "attachments", "conversation", "messageId", "summary", "status", "tool", "detail", "expiresAt",
        "allowAlways", "decision", "by", "enabled", "address", "state", "agent", "paused", "pending", "journal", "chat",
        "permissions", "remote", "title", "rawCommand", "level", "argv", "exitCode", "inverse", "session", "fromSeq", "protocol",
    )

    private class Gen(seed: Long, val names: List<String>) {
        val rnd = Random(seed)

        fun primitive(): JsonElement = when (rnd.nextInt(12)) {
            0 -> JsonNull
            1 -> JsonPrimitive(rnd.nextBoolean())
            2 -> JsonPrimitive(rnd.nextInt())
            3 -> JsonPrimitive(rnd.nextLong())
            4 -> JsonPrimitive(rnd.nextDouble() * 1e300)
            5 -> JsonPrimitive(listOf(Double.NaN, Double.POSITIVE_INFINITY, -0.0, 3.5).random(rnd))
            6 -> JsonPrimitive(java.math.BigDecimal("1e400"))
            7 -> JsonPrimitive(listOf(Long.MAX_VALUE, Long.MIN_VALUE, 0L, -1L).random(rnd))
            8 -> JsonPrimitive("a".repeat(rnd.nextInt(0, 3000)))
            else -> JsonPrimitive(listOf("", "m1", "m2", "t1", "p1", "j_1", "running", "done", "error", "true", "1e400", "-1", "\u0000", "\uD800").random(rnd))
        }

        fun element(depth: Int): JsonElement = if (depth <= 0) primitive() else when (rnd.nextInt(4)) {
            0 -> primitive()
            1 -> JsonArray(List(rnd.nextInt(4)) { element(depth - 1) })
            else -> JsonObject((0 until rnd.nextInt(6)).associate { names.random(rnd) to element(depth - 1) })
        }

        /** The template with fields replaced, dropped or added, or nested values broken further down. */
        fun mutate(el: JsonElement, depth: Int = 3): JsonElement = when {
            rnd.nextInt(10) == 0 -> element(2)
            el is JsonObject -> JsonObject(buildMap {
                for ((k, v) in el) {
                    when (rnd.nextInt(10)) {
                        0 -> Unit // dropped
                        1, 2 -> put(k, element(2))
                        else -> put(k, if (depth > 0) mutate(v, depth - 1) else v)
                    }
                }
                if (rnd.nextInt(5) == 0) put(names.random(rnd), element(2)) // not `keys`: inside buildMap that is the map's own
            })
            el is JsonArray -> JsonArray(el.map { if (depth > 0) mutate(it, depth - 1) else it } + List(rnd.nextInt(2)) { element(2) })
            else -> el
        }
    }

    private fun template(type: String): JsonElement = Protocol.json.parseToJsonElement(templates.getValue(type))

    /** A snapshot: the session state, pending (mostly valid, so each reducer's own part is reached), and those parts. */
    private fun Gen.snapshot(): JsonElement {
        val parts = mutableMapOf(
            "state" to mutate(template(Protocol.SESSION_STATE)),
            "pending" to if (rnd.nextInt(4) == 0) element(2) else JsonArray(List(rnd.nextInt(3)) { template(Protocol.CONFIRM_REQUEST) }),
            "journal" to JsonArray(List(rnd.nextInt(4)) { mutate(template(Protocol.JOURNAL_ENTRY)) }),
            "chat" to JsonArray(List(rnd.nextInt(4)) { mutate(template(Protocol.CHAT_MESSAGE)) }),
            "permissions" to JsonArray(List(rnd.nextInt(4)) { mutate(template(Protocol.PERMISSION_REQUEST)) }),
            "remote" to mutate(template(Protocol.REMOTE_STATE)),
        )
        if (rnd.nextInt(5) == 0) parts[names.random(rnd)] = element(3)
        return if (rnd.nextInt(10) == 0) element(3) else JsonObject(parts)
    }

    private fun Gen.message(): Message {
        val types = BridgeState.REDUCERS.flatMap { it.types } + listOf(Protocol.SNAPSHOT, Protocol.WELCOME, Protocol.ERROR, "x.unknown")
        val type = types.random(rnd)
        val payload = when {
            type == Protocol.SNAPSHOT -> snapshot()
            type in templates -> mutate(template(type))
            else -> element(3)
        }
        val seq = listOf(0L, 1L, 2L, rnd.nextLong(0, 50), Long.MAX_VALUE, -1L).random(rnd)
        return Message(v = 1, seq = seq, rid = if (rnd.nextBoolean()) null else "r1", type = type, ts = "x", payload = payload)
    }

    private fun check(seed: Long, i: Int, what: String, m: Message?, f: () -> Unit) {
        try {
            f()
        } catch (e: Throwable) {
            fail("seed $seed, step $i, $what threw $e with ${m?.type} ${m?.payload.toString().take(500)}")
        }
    }

    @Test fun noReducerThrowsWithAnyPayload() {
        val welcome = Message(v = 1, type = Protocol.WELCOME, ts = "x", payload = template(Protocol.WELCOME))
        for (seed in seeds) {
            val g = Gen(seed, keys)
            var s = BridgeState().apply(welcome)
            for (i in 0 until 5_000) {
                val m = g.message()
                // Through the state, as the channel does.
                check(seed, i, "BridgeState.apply", m) { s = s.apply(m) }
                // And each reducer directly: an event of its own types, any readable snapshot, every reset.
                for (r in BridgeState.REDUCERS) {
                    val type = r.types.random(g.rnd)
                    val own = m.copy(type = type, payload = if (type in templates) g.mutate(template(type)) else g.element(3))
                    check(seed, i, "${r::class.simpleName}.event", own) { r.event(s, own) }
                    val snap = Protocol.payload<Snapshot>(Message(v = 1, type = Protocol.SNAPSHOT, payload = g.snapshot()))
                    if (snap != null) check(seed, i, "${r::class.simpleName}.snapshot", m) { r.snapshot(s, snap) }
                    for (why in Reset.entries) check(seed, i, "${r::class.simpleName}.reset($why)", null) { r.reset(s, why) }
                }
                check(seed, i, "disconnected", null) { s = s.disconnected() }
                if (i % 50 == 0) check(seed, i, "welcome", welcome) { s = s.apply(welcome) } // out of INCOMPATIBLE now and then
            }
        }
    }
}
```

- [ ] **Step 2: Ver que pasa.**

```bash
export JAVA_HOME="/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1"; export PATH="$JAVA_HOME/bin:/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin:$PATH"; cd android && gradle :app:testDebugUnitTest --offline --tests 'tech.hybriq.adbuddy.bridge.ReducerFuzzTest'
```

Esperado: `BUILD SUCCESSFUL` (réplica: 1.7 s). Si algún día falla, el mensaje dice semilla, paso, qué lanzó y el payload: se arregla el reductor (atrapar lo que se escapa como `null`/estado anterior, nunca dejar que salga), con un caso fijo en `ChatReducerTest` o `BridgeStateTest`.

- [ ] **Step 3: RED por mutación (ruling 10).** En `android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ChatReducer.kt`, en `snapshot`, cambiar temporalmente el primer `} catch (e: IllegalArgumentException) {` (el del `decodeFromJsonElement(ChatMessage.serializer(), it)`) por `} catch (e: IllegalStateException) {` y correr el comando del paso 2. Esperado (réplica): `seed 20261002, step 33, ChatReducer.snapshot threw kotlinx.serialization.json.JsonDecodingException: Expected JsonPrimitive, but had JsonArray as the serialized body of string at path: $.role …`. Restaurar con `git checkout -- android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/ChatReducer.kt` y comprobar con `git status`.

- [ ] **Step 4: Suite de la app y commit.**

```bash
export JAVA_HOME="/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1"; export PATH="$JAVA_HOME/bin:/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin:$PATH"; cd android && gradle :app:testDebugUnitTest --offline
git add android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/ReducerFuzzTest.kt
git commit -m "test(app): fuzz determinista de los reductores con cualquier payload

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Nota para el implementador:** dentro de `buildMap { }`, `keys` es la propiedad del mapa que se construye (vacío al empezar), no la lista de la clase: por eso `Gen` la recibe como `names` (en la réplica, usar `keys` ahí dio `NoSuchElementException: Collection is empty`).

---

### Task 10: Markdown: nunca más de `MAX_BLOCKS` y una imagen dentro de un enlace es el enlace (V43)

**Revisor:** `adbuddy-reviewer`.

**Files:**
- Modify: `android/app/src/main/kotlin/tech/hybriq/adbuddy/chat/Markdown.kt` (final de `parse`, rama `'['` de `inlines`, función nueva `linkAfter` tras `link`)
- Test: `android/app/src/test/kotlin/tech/hybriq/adbuddy/chat/MarkdownTest.kt`

**Interfaces:**
- Consumes: `Markdown.parse`, `Markdown.inlines` (`internal`), `Links.domain`, la clase privada `Finder` y `link`.
- Produces: privada `linkAfter(s: String, close: Int, find: Finder): Link?`. `parse` e `inlines` conservan firma.

- [ ] **Step 1: Tests que fallan.**

**Editar `android/app/src/test/kotlin/tech/hybriq/adbuddy/chat/MarkdownTest.kt`:** (cambio 1 de 2) reemplazar

```kotlin
            "images" to "![a](".repeat(n / 5),
```

por

```kotlin
            "images" to "![a](".repeat(n / 5),
            "linked images" to "[![a](b)](".repeat(n / 10),
            "linked images with no link" to "[![a](b)".repeat(n / 9),
```

(cambio 2 de 2) reemplazar

```kotlin
        assertEquals(" [z](", (flat.last() as Inline.Text).text)
    }
```

por

```kotlin
        assertEquals(" [z](", (flat.last() as Inline.Text).text)
    }

    /** V43: never more than MAX_BLOCKS, also when one line flushes a paragraph and adds its own block. */
    @Test fun neverMoreThanMaxBlocks() {
        val headings = (1 until Markdown.MAX_BLOCKS).joinToString("\n") { "# h$it" } // one block short
        val b = Markdown.parse("$headings\nthe last paragraph\n# one too many")
        assertEquals(Markdown.MAX_BLOCKS, b.size)
        assertEquals(p(Inline.Text("the last paragraph")), b.last())
        val fence = Markdown.parse("$headings\nthe last paragraph\n```\ncode")
        assertEquals(Markdown.MAX_BLOCKS, fence.size)
    }

    /**
     * V43: an image inside a link (a badge) is the link, with the image's alt text as its text; the image's own
     * URL is never a link, as with an image alone.
     */
    @Test fun aLinkedImageIsTheLinkWithItsAltText() {
        val flat = (Markdown.parse("[![build](https://img.example/badge.svg)](https://example.com/repo)").single() as Block.Paragraph).inlines
        assertEquals(listOf<Inline>(Inline.Link("build", "https://example.com/repo")), flat)
        for (s in listOf(
            "[![a](https://img.example/x.png)](javascript:alert(1))", // not a web link: only text
            "[![a](https://img.example/x.png) and more", // no link around it
            "[![](https://img.example/x.png)](ftp://example.com/x)",
        )) {
            val inl = (Markdown.parse(s).single() as Block.Paragraph).inlines
            assertTrue("$s: $inl", inl.none { it is Inline.Link })
            assertFalse("$s: $inl", inl.joinToString("") { (it as Inline.Text).text }.contains("img.example"))
        }
        val empty = (Markdown.parse("[![](https://img.example/x.png)](https://example.com/)").single() as Block.Paragraph).inlines
        assertEquals("no alt: the link shows its URL", listOf<Inline>(Inline.Link("https://example.com/", "https://example.com/")), empty)
    }
```

(La clase `MarkdownTest` ya tiene `private fun p(vararg i: Inline) = Block.Paragraph(i.toList())` y los imports de `assertEquals`, `assertTrue` y `assertFalse`.)

- [ ] **Step 2: Ver que fallan.**

```bash
export JAVA_HOME="/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1"; export PATH="$JAVA_HOME/bin:/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin:$PATH"; cd android && gradle :app:testDebugUnitTest --offline --tests 'tech.hybriq.adbuddy.chat.MarkdownTest'
```

Esperado (réplica): `14 tests completed, 2 failed`: `expected:<2000> but was:<2001>` y `expected:<[Link(text=build, url=https://example.com/repo)]> but was:<[Link(text=![build, url=https://img.example/badge.svg), Text(text=](https://example.com/repo))]>`.

- [ ] **Step 3: Implementar.**

**Editar `android/app/src/main/kotlin/tech/hybriq/adbuddy/chat/Markdown.kt`:** (cambio 1 de 3) reemplazar

```kotlin
        flush()
        return out
    }

    /** The inline spans of a text:
```

por

```kotlin
        flush()
        // A line can flush a paragraph and add its own block in one go: one more than the loop allows.
        return if (out.size > MAX_BLOCKS) out.subList(0, MAX_BLOCKS).toList() else out
    }

    /** The inline spans of a text:
```

(cambio 2 de 3) reemplazar

```kotlin
                c == '[' -> {
                    val l = link(s, i, find)
                    if (l == null) { text.append(c); i++ } else {
                        if (Links.domain(l.url) != null) { flush(); out.add(Inline.Link(l.text.ifEmpty { l.url }, l.url)) } else text.append(l.text)
                        i = l.end
                    }
                }
```

por

```kotlin
                c == '[' -> {
                    val l = link(s, i, find)
                    when {
                        l == null -> { text.append(c); i++ }
                        // [![alt](image)](url), a linked image (a badge): the link, with the alt as its text. What
                        // l read is the image (its ']' is the first one); its own URL is never a link.
                        l.text.startsWith("![") -> {
                            val alt = l.text.substring(2)
                            val outer = linkAfter(s, l.end, find)
                            if (outer != null && Links.domain(outer.url) != null) { flush(); out.add(Inline.Link(alt.ifEmpty { outer.url }, outer.url)) } else text.append(alt)
                            i = outer?.end ?: l.end
                        }
                        else -> {
                            if (Links.domain(l.url) != null) { flush(); out.add(Inline.Link(l.text.ifEmpty { l.url }, l.url)) } else text.append(l.text)
                            i = l.end
                        }
                    }
                }
```

(cambio 3 de 3) reemplazar

```kotlin
        return Link(s.substring(open + 1, close), s.substring(start, end), end + 1)
    }
```

por

```kotlin
        return Link(s.substring(open + 1, close), s.substring(start, end), end + 1)
    }

    /** The "](url)" right at [close], which ends a linked image's outer link, or null. Its text is empty. */
    private fun linkAfter(s: String, close: Int, find: Finder): Link? {
        if (close + 1 >= s.length || s[close] != ']' || s[close + 1] != '(') return null
        val end = find.url(close)
        if (end < 0) return null
        return Link("", s.substring(close + 2, end), end + 1)
    }
```

- [ ] **Step 4: Ver que pasan (también el tiempo lineal).**

```bash
export JAVA_HOME="/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1"; export PATH="$JAVA_HOME/bin:/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin:$PATH"; cd android && gradle :app:testDebugUnitTest --offline --tests 'tech.hybriq.adbuddy.chat.*'
```

Esperado: `BUILD SUCCESSFUL` (réplica: `hostileBracketsTakeLinearTime` 0.23 s con los casos nuevos; `imagesAreOnlyTheirAltTextAndTheUrlIsNeverALink` y `theMemoryOfAFailedSearchDoesNotChangeWhatIsALink` siguen en verde).

- [ ] **Step 5: Suite de la app y commit.**

```bash
export JAVA_HOME="/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1"; export PATH="$JAVA_HOME/bin:/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin:$PATH"; cd android && gradle :app:testDebugUnitTest --offline
git add android/app/src/main/kotlin/tech/hybriq/adbuddy/chat/Markdown.kt android/app/src/test/kotlin/tech/hybriq/adbuddy/chat/MarkdownTest.kt
git commit -m "fix(app): el Markdown no pasa de MAX_BLOCKS y una imagen enlazada es el enlace con su alt

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Para el revisor:** `linkAfter` usa `Finder.url`, que guarda una sola respuesta por `close`: una llamada con otro `close` la sustituye, y su exploración se para en el primer carácter que no puede ir en una URL (`(` incluido), así que dos exploraciones fallidas no se solapan más que hasta el `(` de la imagen siguiente; los dos casos hostiles nuevos lo vigilan.

---

### Task 11: `TerminalFeed` con pila de vistas: plegar la burbuja no congela la terminal (V43)

**Revisor:** `adbuddy-reviewer`.

**Files:**
- Modify: `android/app/src/main/kotlin/tech/hybriq/adbuddy/term/TerminalFeed.kt` (`sink` pasa a ser la cima de `sinks`; `attach`, `detach`)
- Test: `android/app/src/test/kotlin/tech/hybriq/adbuddy/term/TerminalFeedTest.kt` (dos tests al final)

**Interfaces:**
- Consumes: `TermSink`, `TerminalModel.replay()/watch()`, `FakeChannel` (`emit`, `sent`).
- Produces: `attach(sink: TermSink)` y `detach(sink: TermSink)` con la misma firma; privados `sinks: ArrayList<TermSink>` y la propiedad `sink` (cima de la pila).

- [ ] **Step 1: Tests.**

**Editar `android/app/src/test/kotlin/tech/hybriq/adbuddy/term/TerminalFeedTest.kt`:** reemplazar

```kotlin
        assertEquals(listOf("Reset", "Resize"), got.map { it::class.simpleName })
    }
}
```

por

```kotlin
        assertEquals(listOf("Reset", "Resize"), got.map { it::class.simpleName })
    }

    /**
     * V43: a bubble over a visible activity is a second view on top. When it goes, the view under it draws again
     * (from memory, without asking the host) and the stream goes on: no unwatch, so the terminal does not freeze.
     */
    @Test fun whenTheBubbleGoesTheViewUnderItDrawsAgain() {
        val under = mutableListOf<TermEffect>()
        val activity = TermSink { under += it }
        val bubble = TermSink { got += it }
        feed.attach(activity)
        snapshot(5)
        feed.attach(bubble)
        ch.emit(Protocol.TERMINAL_OUTPUT, """{"epoch":"e1","offset":5,"data":"${b64("ab")}"}""")
        assertEquals("only the view on top draws", listOf("Reset"), under.map { it::class.simpleName })
        under.clear()
        feed.detach(bubble)
        assertEquals("the stream goes on", 0, types().count { it == Protocol.TERMINAL_UNWATCH })
        assertEquals("the view under it is redrawn from memory", "Reset", under.first()::class.simpleName)
        under.clear()
        ch.emit(Protocol.TERMINAL_OUTPUT, """{"epoch":"e1","offset":7,"data":"${b64("c")}"}""")
        assertEquals(listOf("Write"), under.map { it::class.simpleName })
        assertEquals("the bubble no longer draws", 2, got.size)
    }

    /** V43: the stream stops only when the last view goes, in whatever order they go. */
    @Test fun theLastViewToGoUnwatches() {
        val a = TermSink { }
        val b = TermSink { }
        feed.attach(a)
        feed.attach(b)
        feed.detach(b)
        feed.detach(a)
        assertEquals(1, types().count { it == Protocol.TERMINAL_UNWATCH })
        feed.attach(a)
        feed.attach(a) // the same view twice is one view
        feed.detach(a)
        assertEquals(2, types().count { it == Protocol.TERMINAL_UNWATCH })
    }
}
```

- [ ] **Step 2: Ver que falla.**

```bash
export JAVA_HOME="/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1"; export PATH="$JAVA_HOME/bin:/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin:$PATH"; cd android && gradle :app:testDebugUnitTest --offline --tests 'tech.hybriq.adbuddy.term.TerminalFeedTest'
```

Esperado (réplica): `15 tests completed, 1 failed`, `the stream goes on expected:<0> but was:<1>`. `theLastViewToGoUnwatches` pasa ya con el código de hoy: vigila que la pila no rompa lo que funcionaba.

- [ ] **Step 3: Implementar.**

**Editar `android/app/src/main/kotlin/tech/hybriq/adbuddy/term/TerminalFeed.kt`:** (cambio 1 de 3) reemplazar

```kotlin
    private val model = TerminalModel()
    private var sink: TermSink? = null
```

por

```kotlin
    private val model = TerminalModel()

    /**
     * The views attached, the newest last; only that one draws. A bubble over the activity attaches on top of the
     * activity's view, and when it goes the view under it draws again. Guarded by lock.
     */
    private val sinks = ArrayList<TermSink>()
    private val sink: TermSink? get() = sinks.lastOrNull()
```

(cambio 2 de 3) reemplazar

```kotlin
    /** The view appeared: redraw from memory and watch. */
    fun attach(sink: TermSink) {
        val watch = synchronized(lock) {
            this.sink = sink
            resyncing = false
```

por

```kotlin
    /** The view appeared: it goes on top, is redrawn from memory, and watches. */
    fun attach(sink: TermSink) {
        val watch = synchronized(lock) {
            sinks.removeAll { it === sink }
            sinks.add(sink)
            resyncing = false
```

(cambio 3 de 3) reemplazar

```kotlin
    /**
     * The view left: stop the stream. Only the view that is attached can: with a bubble over the activity the new
     * view attaches before the old one detaches, and the old one must not take the new one's stream away.
     */
    fun detach(sink: TermSink) {
        val was = synchronized(lock) { if (this.sink === sink) { this.sink = null; true } else false }
        if (was) channel.send(Protocol.TERMINAL_UNWATCH, Protocol.json.encodeToJsonElement(Empty.serializer(), Empty))
    }
```

por

```kotlin
    /**
     * The view left. The stream stops only with the last view: with a bubble over the activity the new view
     * attaches before the old one detaches, and the old one must not take the new one's stream away. When the view
     * on top goes, the one under it draws again from memory; the stream goes on, so no watch is needed.
     */
    fun detach(sink: TermSink) {
        val unwatch = synchronized(lock) {
            val wasOnTop = this.sink === sink
            if (!sinks.removeAll { it === sink }) return
            val under = this.sink
            if (under != null && wasOnTop) model.replay().forEach { under.apply(it) }
            under == null
        }
        if (unwatch) channel.send(Protocol.TERMINAL_UNWATCH, Protocol.json.encodeToJsonElement(Empty.serializer(), Empty))
    }
```

(`feed()` y `onConnectedChanged()` leen `sink` y ya están bajo `lock`: no cambian.)

- [ ] **Step 4: Ver que pasan.**

```bash
export JAVA_HOME="/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1"; export PATH="$JAVA_HOME/bin:/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin:$PATH"; cd android && gradle :app:testDebugUnitTest --offline --tests 'tech.hybriq.adbuddy.term.*'
```

Esperado: `BUILD SUCCESSFUL` (`anOldViewCannotDetachTheNewOne` y `detachUnwatchesAndStopsTheSink` siguen en verde).

- [ ] **Step 5: Suite de la app y commit.**

```bash
export JAVA_HOME="/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1"; export PATH="$JAVA_HOME/bin:/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin:$PATH"; cd android && gradle :app:testDebugUnitTest --offline
git add android/app/src/main/kotlin/tech/hybriq/adbuddy/term/TerminalFeed.kt android/app/src/test/kotlin/tech/hybriq/adbuddy/term/TerminalFeedTest.kt
git commit -m "fix(app): TerminalFeed guarda una pila de vistas y plegar la burbuja no congela la terminal

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Concurrencia:** `sinks` solo se toca bajo `lock` (`attach`, `detach`, `feed`, `onConnectedChanged`); el `return` dentro de `synchronized` es local a `detach` (función `inline`) y suelta el monitor. El redibujado ocurre bajo el mismo `lock` que `feed`, así que ningún `Write` adelanta a su `Reset`.

---

### Task 12: Pendientes y verificación final

**Revisor:** el controlador; después, `adbuddy-reviewer-opus` hace la revisión final de la rama.

**Files:**
- Modify: `docs/superpowers/plans/pendientes-m3-m5.md` (CRLF: ancla de una sola línea)

- [ ] **Step 1: Apuntar lo hecho y lo diferido.** Añadir al final del fichero (hoy termina con la sección `## M6c · Idiomas (en/es)`, cuya última línea empieza por `- **\`AGENTS.md\` (M6b) y \`adbuddy doctor\`:**`), con CRLF como el resto:

```markdown

## v1 · Diferidos B (plan `docs/superpowers/plans/2026-10-02-adbuddy-v1-diferidos-b.md`)

- **HECHO:** la guarda decodifica `%XX` (hasta 4 capas, hex en mayúsculas o minúsculas) y parte por todo lo que no es de un nombre de paquete, también `?` y `@` (V11); `Disable` con el modo apagado lee el puerto antes de `adb usb`, y tras un `SetUntil` fallido los reintentos no repiten `adb usb` (V38); el vigilante acota `adb devices` a `TryWait` y `DEVICE_NOT_FOUND` dice qué direcciones se probaron (V38); un cerrojo de sistema por carpeta de datos (`adbuddy.lock`), el secreto con renombrado bajo cerrojo (`ipc.secret.lock`) y relectura, como mucho 32 conexiones sin autenticar y `ReadFrame` que crece con lo que llega (V29); `TermWebPolicy` solo admite nombres llanos de la carpeta (V21); tests de `Bridge` de socket viejo y de reentrada `onResult → request`, y fuzz determinista de los reductores (V42); `Markdown.MAX_BLOCKS` exacto y la imagen enlazada como enlace con su alt, `TerminalFeed` con pila de vistas (V43). Con esto quedan hechos, de M3, los tests de `Bridge` que faltaban, "ningún reductor lanza" y los casos de borde de `TermWebPolicyTest`; de M3b, la pila de sinks; de M4, lo de Markdown; de M5b, `Online(ctx)`, `Disable` sin modo, `adb usb` repetido y `DEVICE_NOT_FOUND`.
- **Diferido:** `REMOTE_UNAVAILABLE` depende del texto de adb (falla al lado seguro; cambiarlo toca la invariante de M5b), las consultas de ~900 ms de la guarda (tras la validación 17 de M5b), las herramientas atascadas en `running` (decisión de diseño: `Stop` no salta al interrumpir, y falta el estado de una interrumpida en el protocolo) y `PickMultipleVisualMedia(10)` sin descontar borradores (UI). Motivos en "Fuera de alcance" del plan.
```

Comprobar con `file docs/superpowers/plans/pendientes-m3-m5.md` que sigue en CRLF y con `git diff` que solo se añaden líneas al final.

- [ ] **Step 2: Verificación final de los dos árboles.**

```bash
cd host && go build ./... && go vet ./... && gofmt -l . && GOOS=linux go vet ./... && GOOS=darwin go vet ./... && go test -count=1 -timeout 15m ./...
export JAVA_HOME="/c/Users/Jose/AppData/Local/Android/jdk-17.0.20.1+1"; export PATH="$JAVA_HOME/bin:/c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin:$PATH"; cd android && gradle :app:testDebugUnitTest :app:assembleDebug :app:lintDebug --offline
git grep -nE '\b(10\.[0-9]+\.[0-9]+\.[0-9]+|192\.168\.[0-9]+\.[0-9]+|172\.(1[6-9]|2[0-9]|3[01])\.[0-9]+\.[0-9]+)\b' -- host android docs/superpowers/plans/2026-10-02-adbuddy-v1-diferidos-b.md
git log --format='%b' origin/main..HEAD | grep -c 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>'
git rev-list --count origin/main..HEAD
git log --format='%an <%ae>' origin/main..HEAD | sort -u
```

Esperado: Go y Gradle en verde; el `git grep` de IPs privadas sin salida nueva respecto a `origin/main`; el número de trailers igual al de commits de la rama (las dos cuentas coinciden) y un solo autor, la identidad local hybriqtech.

- [ ] **Step 3: Commit.**

```bash
git add docs/superpowers/plans/pendientes-m3-m5.md
git commit -m "docs: pendientes al día tras los diferidos B de la v1

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

## Validaciones manuales pendientes (v1 diferidos B)

Solo con permiso explícito del usuario (Pixel 9 Pro XL, Android 17) o en el emulador; ninguna es bloqueante para integrar.

36. **Android decodifica `%XX` en `android-app://`** (informativa, emulador): `adb shell am start -d android-app://com%2Eandroid%2Esettings` abre Ajustes. Si no lo abre, la Tarea 1 bloquea de más (falla cerrado) y no hace falta tocar nada; si lo abre, confirma el hueco que cierra.
37. **El "off" con el modo remoto apagado no corta la conexión** (móvil): con `serve` corriendo y el modo apagado, pulsar "apagar" en la tarjeta del modo remoto (o `adbuddy remote off`): la conexión del móvil no se reinicia (el diario de `serve` no muestra la reconexión). Con el 5555 abierto a mano (`adb tcpip 5555` desde la terminal del usuario) y el modo apagado en ADBuddy, `adbuddy remote off` lo cierra (`getprop service.adb.tcp.port` vacío después).
38. **La burbuja sobre la terminal** (móvil): con la terminal de `MainActivity` a la vista, abrir la burbuja de ADBuddy en la vista de terminal, plegarla, y comprobar que la terminal de debajo sigue recibiendo salida sin salir y volver a entrar.
39. **Un badge en el chat** (móvil, opcional): un mensaje del agente con `[![build](https://img.shields.io/badge/x-y-green)](https://example.com)` se ve como el enlace "build" hacia `example.com` (con el dominio antes de abrir), sin cargar la imagen.
40. **Dos `serve` a la vez en la misma carpeta de datos** (PC, sin móvil): lanzar `adbuddy serve` dos veces casi a la vez; el segundo sale con el mensaje de "ya está en marcha" y el primero sigue sirviendo (`adbuddy status`). En macOS o Linux lo cubre además la CI (`flock`).

## Autorrevisión del plan

### Cobertura frente al encargo

- V11 → Tarea 1 (con doble codificación, `%` mal formado, hex en mayúsculas y minúsculas, cinco capas, y los tests de la guarda que ya había siguen en verde).
- V38 → Tareas 2 y 3 (cuatro ítems reales arreglados) y "Fuera de alcance" (dos diferidos con motivo). Rulings de M5b intactos: ningún camino nuevo pone a cero una caducidad sin `adb usb` previo de esa misma caducidad, ni pregunta nada al usuario.
- V29 → Tareas 4, 5 y 6 (cerrojo, escritura atómica con lectura que reintenta, límite de conexiones sin autenticar y reserva progresiva). Sin cambios de protocolo ni de autenticación.
- V21 → Tarea 7 (los casos de borde pedidos y más; la política se endurece a lista blanca).
- V42 → Tareas 8 y 9 (socket viejo, reentrada, fuzz de `ChatReducer`, `ConfirmReducer` y `RemoteReducer` a través de `BridgeState.REDUCERS` y directamente).
- V43 → Tareas 10 y 11, y dos diferidos con motivo (herramientas atascadas, `PickMultipleVisualMedia`).

### Placeholders y tipos

Sin "TBD" ni pasos sin código. Nombres cruzados comprobados: `tryLock`/`errLocked`/`lockPath` (T4) los usa T5 (`waitLock`) y T6 amplía el `return &Server{ln: ln, lock: lock, …}` que deja T4; `maxPreauth` y `frameChunk` solo en T6; `secretWritten` solo en T5; `encodedReason`/`decodings` solo en T1; `closedFor` solo en T2. Los textos "reemplazar" de T6 sobre `server.go` y `ipc.go` son los que dejan T4 y T5 (la réplica los aplicó en ese orden).

### Réplica acumulada tarea a tarea

Hecha sobre `git archive` de 2fcf8e2 en una carpeta temporal, con un commit por tarea. Host: T1, T2, T3 (suite completa `ok`), después T4, T5, T6 (suite completa, `vet` de los tres sistemas y `gofmt` limpios: 29 paquetes `ok`). App: T7, T8, T9, T10, T11 (suite completa de la app en verde, 57 clases de test). Cada RED de los pasos es la salida real de esa réplica; los RED por mutación (T8, T9) se comprobaron y se deshicieron. El paso de pendientes (T12) no se replicó (es texto).

Después, **un script volvió a aplicar este documento** (bloques `Crear`, pares `reemplazar`/`por` y "añadir al final", en orden de tareas) sobre otro `git archive` limpio de 2fcf8e2: cada texto "reemplazar" apareció exactamente una vez, y el árbol resultante pasó `go build`, `go vet` (Windows, Linux y macOS), `gofmt -l`, `go test -count=1 -timeout 15m ./...` y `gradle :app:testDebugUnitTest --offline`, todo en verde. Aplicado hasta cada tarea de host (1 a 5), `go vet ./...` compila y el paquete tocado pasa (en esa comprobación parcial solo se extrajeron `host/` y `docs/`, así que `TestDocsAreGuarded` falló por los ficheros del repo que faltaban, no por el plan). **Tras el rebase sobre 0aaa593 (M6c y M6d):** el mismo script aplicó el plan actualizado sobre un `git archive` limpio de la rama rebasada: 57 operaciones, cada texto "reemplazar" encontrado una vez (la única que había dejado de valer era la edición de `serve.go`, sustituida por las dos del catálogo), y el árbol pasó `go build ./...`, `go vet ./...` (también `GOOS=linux` y `GOOS=darwin`), `gofmt -l .` vacío, `go test -count=1 -timeout 15m ./...` con los 31 paquetes en `ok` (incluidos `i18n` y `i18n/i18ntest`: paridad del catálogo, claves usadas y existentes) y `gradle :app:testDebugUnitTest --offline` en verde (61 clases de test). El RED de `TestServeSavedAddressesGoneSaysWhichWereTried` sobre esa rama, con el catálogo sin tocar, es el mismo texto de antes (`Error: DEVICE_NOT_FOUND — the phone did not show up, and its saved address did not work (198.51.100.7:40000, 100.64.0.5:41234: adb connect 100.64.0.5:41234: failed to connect: Connection refused)`).

**Tras la revisión Opus del plan** (1 Important y 7 menores, todos incorporados: `Enable` olvida `closedFor` antes de `adb tcpip`, la plaza de una conexión rechazada se devuelve al terminar, el riesgo de denegación local explícito, `secretTries` variable y alargada en los tests, `stop` cerrado en un `defer`, la medida de `ReadFrame` por el búfer pedido y no por el heap, "last error seen", `Reset.entries` y el esquema codificado en la guarda): el script aplicó las 58 operaciones sobre otro `git archive` limpio de la rama, y salió verde igual (31 paquetes en `ok`, `go test -count=10 ./internal/ipc` en verde, `gradle :app:testDebugUnitTest --offline` con 61 clases). Mutaciones comprobadas: sin el `c.closedFor = time.Time{}` de `Enable`, `TestEnableForgetsTheExpiryItsPortWasClosedFor` falla (`events "save zero": the port was not closed again`); con el `ReadFrame` de hoy, `TestReadFrameGrowsWithWhatArrives` falla (`asked to fill 1048576 bytes at once, for 9 that arrived`).

Diferencias con la primera réplica: el orden de los tests en `ipc_test.go`, el CRLF de `docs/troubleshooting.md` (que el plan conserva) y un caso de `TermWebPolicyTest` (la primera réplica tenía `"a\b"`, que en Kotlin es un retroceso; el plan dice `"a\\b"`, la barra invertida que se quería probar, y pasa).

### Riesgos abiertos para el revisor Opus

1. **T1:** el nuevo separador (todo lo que no es `[A-Za-z0-9_.]`) es más ancho que el anterior (`/;=:#,{} \t`): solo puede añadir bloqueos (un paquete no lleva otros caracteres), pero conviene mirar si algún `am start` habitual de `habitual.jsonl` nombra por accidente una app protegida tras partir por `-`: la guarda solo mira palabras de órdenes que actúan (`input`, `am start`, `monkey`, `cmd input`) y solo bloquea si un trozo es exactamente un paquete protegido.
2. **T2:** `closedFor` vive en memoria: si `serve` se reinicia entre el fallo de `SetUntil` y el reintento, `Reconcile` lee el puerto (cerrado) y borra la caducidad sin `adb usb`, que es lo que ya hacía.
3. **T4:** en Windows, `LockFileEx` sobre un handle síncrono con `Overlapped` a cero: probado aquí. Un daemon de una versión anterior no toma el cerrojo; lo sigue detectando el Dial.
4. **T5:** el reintento de `ReadSecret` alarga 200 ms el error de un secreto corrupto (también en el shim de cada orden del agente, solo en ese caso). El fichero `ipc.secret.lock` queda en la carpeta de datos.
5. **T6:** `maxPreauth = 32` y el cierre de la conexión 33 sin respuesta (falla cerrado): un cliente legítimo en una ráfaga muy grande vería "lost the connection to the ADBuddy host". Un proceso del mismo usuario puede agotar el cupo a propósito, renovando 32 conexiones mudas cada 5 s (el plazo absoluto antes de autenticar): denegación de servicio local, no una frontera de privilegios (el mismo usuario puede matar `serve`). Una conexión rechazada conserva su plaza durante la espera de hasta 2 s de `finish`, para que los rechazos tampoco lancen gorrutinas sin tope.
6. **T8:** `anOldSocketIsIgnoredAfterANewLink` depende de que OkHttp siga entregando mensajes tras el cierre iniciado por la app hasta recibir el del host (`holdClose`): es el comportamiento documentado de `RealWebSocket`, comprobado con MockWebServer 5.

### Decisiones y dudas para el controlador

- **Textos:** con M6c integrada, `DEVICE_NOT_FOUND` va al catálogo en los dos idiomas dentro de la Tarea 3; la razón de la guarda y la causa del cerrojo del secreto se quedan en inglés por la spec de M6c (§3: salida para el agente y causas técnicas). Nada que pedir a tern-51 salvo, si quiere, revisar el español de `err.serve.device_not_found`.
- **`docs/troubleshooting.md`** pertenece a la documentación de M6d (skimmer): el cambio es una frase citada; avisar al dueño de los docs.
- **Herramientas atascadas en `running`:** ya no hay solape con M6c, pero sigue diferido por diseño (ver "Fuera de alcance"): `Stop` no salta al interrumpir y el estado de una herramienta interrumpida no existe en el protocolo.
