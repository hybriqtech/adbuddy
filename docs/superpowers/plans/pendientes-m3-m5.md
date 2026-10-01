# Pendientes para planes futuros (M2.5b, M3, M5, M6 y v1.1+)

Notas que hay que incorporar cuando se escriban esos planes (desde el 2026-09-29).

## M2.5b · La app en el emulador

Spec: `docs/superpowers/specs/2026-09-30-adbuddy-m2.5-clasificador-lista-blanca-design.md` (§7). M2.5a (clasificador con lista blanca, corpus como datos, oráculo de mksh y arnés del emulador) está hecho.

- **La imagen `android-37.0` google_apis no sirve para los tests de la app:** con el emulador 37.1.11, `surfaceflinger` aborta cada 9-15 s desde el arranque (`Assertion failed: !rcEnc->featureInfo()->hasReadColorBufferDma`, hilo `RegionSampling`) y Android reinicia su sistema en bucle; pasa con `-gpu swiftshader_indirect`, `host` y `guest` y desactivando GLDMA/GLDMA2. El oráculo de M2.5a lo esquiva parando el sistema (`su 0 stop`), pero la app necesita el sistema en marcha. Hay que probar otra imagen (por ejemplo `android-36` google_apis o una `*_atd`), y eso es una instalación que decide el usuario.
- **Tests instrumentados** (`connectedDebugAndroidTest`) y **tests del host contra el emulador** (paquete `host/internal/emutest`, con `ADBUDDY_EMULATOR_SERIAL`, que `scripts/emulator.sh start` ya exporta).
- **Validaciones manuales de M2 que se automatizan:** 1 (`TokenProvider` y el uid del shell), 2 (`content write` por stdin), 3 (servicio en primer plano desde el proveedor), 6 (Cortar acceso y Reactivar) y 10 (`ContentObserver` de `adb_wifi_enabled`). La 7 (extremo a extremo), solo si sale sin tocar el protocolo ni la app.
- **Primera tarea del plan:** comprobar si el emulador sobrevive a `adb_enabled=0` y vuelve al reactivar; de eso depende cómo se automatiza la 6.
- **El job `emulator` de la CI** ya existe: M2.5b le añade los tests de la app y `android/**` a sus rutas.
- `android/local.properties` no se versiona: crearlo con `sdk.dir=C\:/Users/<usuario>/AppData/Local/Android/Sdk`.

### Ampliaciones de la lista blanca, si el uso real las pide

Cada una es un caso nuevo en el evaluador, sus usos en el generador de casos y una regrabación del oráculo:

- `case … esac`;
- `[[ ]]` con operadores de texto y de fichero;
- nombres de variable en mayúsculas que no sean de la shell ni del entorno (hoy solo minúsculas: `PKG=…` pide confirmación);
- `print`, `wait`, `unset` de nombres propios.

## Aplazados de M2.5a (revisión final)

- `checkDumpsys` no mira la primera palabra tras `dumpsys`: `dumpsys --reset` o `dumpsys set` siguen 🟢 (heredado de M2; `checks_read.go`).
- El oráculo cuenta como "no se ejecuta" una centinela que dispara más de 1 s después de acabar el caso. Lo que queda en segundo plano ya es 🔴 especial por construcción, pero el oráculo no lo demuestra.
- El grabador no mata procesos de otros usuarios que sobreviven a un caso (solo darían falsos "se ejecutó") ni borra el fichero de sesión antes de cada caso.
- Si salta el `-timeout` de Go durante la grabación, el sistema Android del emulador queda parado (`su 0 start` para arrancarlo).
- Muchos "no se ejecuta" del oráculo dependen de la imagen (`su` de AOSP frente a Magisk, `busybox`, `script`…): hoy todos son 🔴 especial, pero si se admiten en la lista blanca hay que regrabar en otra imagen.
- `emulator.yml`: el paso de grabar pasaría con "no tests to run" si se renombra `TestRecordOracle`; y los límites (60 min de test, 90 de job) se pueden bajar cuando se vea lo que tarda en CI.
- `xargs -n1` y `xargs -I{}` (opción y valor pegados) piden confirmación como especiales.

## M3 · Chat: daemon con PTY

- **Del protocolo común (M3.0, spec `2026-10-01-adbuddy-protocolo-m3-m5-design.md`):**
  - el envío que espera hueco y la consulta de presión de la cola en `server.go` (§6.4 y §7);
  - el test de que se descartan las tramas de ids no aceptados (§12);
  - todo manejador de una petición con `rid` responde por todos sus caminos (§3), también en M4 y M5;
  - los manejadores que guardan estado por conexión comprueban `Server.Current(m)` (§10);
  - el área de tramas (subidas) protege su estado con su propio mutex y tolera una trama de la conexión anterior tras un reemplazo: `OnFrame` no dice de qué conexión viene (pasarle la conexión es un cambio de API interna que M3 puede hacer);
  - el host pone `[]` en `ChatMessage.Attachments` al construir un `chat.message` (`host/internal/bridge/protocol_chat.go`): un slice nil sale como `null` y §4.3 promete listas. La app lo tolera (fixture `chat.message@noattachments`);
  - `Server.Send` va a la conexión actual, no a la que se suscribió: un envío atado a la petición (`SendTo(req, …)` o equivalente), como `Reply`. **Hecho en m3-bridge para los flujos** (`Server.SendTo`, la terminal); las descargas van en tramas binarias, y el envío atado que espera hueco sigue pendiente para M3 (spec §6.4);
  - tests del servidor que llegan con el primer área que publique en `attach`: `OnAttach` llega después de `OnConnect(true)`, `Publish`/`Send` desde `OnAttach` no se bloquean, y `filled()` por el camino de `PublishSnapshot`;
  - `Options.OnAttach` (y por tanto `area.attach`) corre con el lock de conexiones del servidor tomado: tiene que ser rápido y sin E/S bloqueante, o bloquea la conexión y desconexión de cualquier app. Decirlo en su comentario cuando M3 rellene `attach`;
  - en `Bridge.kt`, antes del primer oyente: aislar las excepciones de `onResult` y de los oyentes (`try/catch` por llamada); si no, una en `failPending` puede tumbar el bucle de reconexión o el proceso, y una en el lector corta el socket. **Hecho en m3-bridge** (`safely`);
  - en `Bridge.kt`, `onMessage` resuelve `pending[rid]` sin comprobar que la petición sea de ese socket: `pending[rid]?.takeIf { it.socket === webSocket }`. **Hecho en m3-bridge**, sin test: solo se llega con `start()` con otro link antes de que el bucle viejo falle sus peticiones; queda entre los tests que faltan;
  - antes de que M4 y M5 vayan en paralelo: un gancho de reinicio en `Reducer` (por ejemplo `reset(s)`, llamado con un welcome de sesión nueva y con un snapshot ilegible) o la regla de que cada reductor reemplaza su parte en `snapshot()` en vez de mezclarla; si no, los dos tocarían `BridgeState.welcome`. **Hecho en m3-bridge** (`Reducer.reset`, llamado con un welcome de sesión nueva y con un snapshot ilegible);
  - tests de `Bridge` que faltan: `stop()` con peticiones pendientes, `request`/`send`/`sendFrame` entre `onOpen` y el welcome, mensajes y tramas de un socket viejo, y reentrada `onResult → request`. m3-bridge añade los de `stop()` con peticiones pendientes y del envío antes del welcome; quedan los del socket viejo (también el `takeIf` de `pending` por socket) y la reentrada;
  - **ningún reductor lanza una excepción, con ningún payload** (revisión del plan de m3-bridge), con su test: si lo hace dentro de `_state.updateAndGet`, se corta el socket, el host reenvía el mismo evento y la app entra en un bucle de reconexión. Vale para M3, M4 y M5.

- **Lista de variables de entorno permitidas** para lanzar el agente (§5.1). En M1a, `host/internal/agent.Env` solo quita variables (`CLAUDECODE*`, `CLAUDE_CODE_*`, `CLAUDE_EFFORT`, `CLAUDE_PID`, `ANDROID_SERIAL`, `ANDROID_ADB_SERVER_PORT`, `ADBUDDY_SOURCE`). M3 debe pasar a una lista de permitidas: las de sistema, autenticación y proxy del usuario (`PATH`, `HOME`/`USERPROFILE`, `APPDATA`, `ANTHROPIC_*`, `HTTP(S)_PROXY`/`NO_PROXY`, `CLAUDE_CONFIG_DIR`…), verificando la lista en Windows, macOS y Linux.
- **Confianza previa de la carpeta del entorno** en Claude Code (§5.1, S3): averiguar dónde y con qué clave guarda Claude Code la confianza de una carpeta y preconfiar el entorno antes de lanzar el agente en el PTY; si no es posible, detectar el diálogo y elegir "Yes" de forma explícita (un Enter a ciegas elige "No, exit").

- **HECHO en M3b:** la app muestra los `notice` del host (snackbar; `AdbuddyApp.notices`). Antes: **La app muestra los `notice` del host** (revisión final de M2): ignoraba; el de respuesta tardía ("That question was already answered or moved to the PC terminal.") y el de tipo no soportado deberían verse en la pantalla de inicio.
- **Fixtures JSON compartidas del protocolo:** las hace M3.0 en `protocol/fixtures/` (spec `2026-10-01-adbuddy-protocolo-m3-m5-design.md`, §12).
- **HECHO en M3b (T7, skill `frontend-design`):** base de diseño (tema M3 claro/oscuro con color dinámico, borde a borde, jerarquía; `PaletteContrastTest`). Queda la validación manual en el Pixel. Antes: **Diseño de la app** (prueba del usuario en su Pixel 9 Pro XL con Android 17, 2026-10-01: "funciona bien aunque el diseño de la app es feo"). Antes de las pantallas nuevas de M3 (chat, terminal, adjuntos), una base visual común, que se aplica también a las que ya existen (consentimiento, inicio, Actividad, confirmaciones):
  - **Barras del sistema:** Android 15+ dibuja de borde a borde; la pantalla de consentimiento pone el título bajo la barra de estado. Hay que respetar los insets (`Scaffold` o `windowInsetsPadding`) en todas las pantallas.
  - **Modo oscuro:** el título de la pantalla de consentimiento sale en texto oscuro sobre fondo oscuro y el botón "Ahora no" casi no se lee. Los colores salen del tema (`MaterialTheme.colorScheme`), nunca fijos, con esquema claro y oscuro y color dinámico (Material You) en Android 12+.
  - **Jerarquía:** la pantalla de consentimiento es texto suelto con dos botones; necesita estructura (cabecera, lista de lo que se concede con iconos, nota de privacidad aparte y acciones al pie).
  - Usar el skill `frontend-design` al diseñar, y comprobar cada pantalla en claro, oscuro y con letra grande.
- **PTY en Windows: hijos con consola propia** (re-revisión de la ronda 2 de M3a). Tras una salida normal del agente, `Kill` ya no llama a `taskkill`, y cerrar la consola solo termina lo que sigue enganchado a ella. Los hijos con consola propia (node con `windowsHide`, como los servidores MCP) siguen vivos. Lo mismo pasa con el propio agente si `serve` muere sin ejecutar sus `defer` (pánico, `TerminateProcess`, Administrador de tareas); con Ctrl+C, `SIGTERM` o el cierre de la consola sí se para (e2e de M3a, T10). Arreglo: un Job object con `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` (crear el proceso suspendido, asignarlo, `ResumeThread`, cerrar el job en `Kill`). Ajustes opcionales del mismo fichero: cerrar el handle del hilo tras `CreateProcess`, cerrar `cmdOut` antes de `ClosePseudoConsole` si falla `spawn`, `taskkill` con `CREATE_NO_WINDOW` y `o.Path` como `lpApplicationName`.
- **Aplazados de M3a (revisiones de T6-T9):**
  - un `chat.send` reintentado tras `session.new` o fuera de los 100 del historial se vuelve a inyectar: falta un conjunto acotado de ids ya inyectados, independiente del historial;
  - `chatSnapshot` serializa hasta ~6,4 MB con el lock del servidor tomado: guardar el tamaño de cada mensaje en el upsert;
  - en la terminal, si `sendTerm` falla con un error distinto de `ErrNoApp`, se descarta lo acumulado sin pedir un snapshot (y se pierde el `terminal.size`);
  - subidas: ventana entre conexiones mientras se mueve el fichero final (un `attach.begin` del mismo id desde otra conexión), y sin tope de descargas simultáneas (acotadas por el registro);
  - `outbox/` sin cuota y registro de adjuntos que no se vacía: el agente puede llenar el disco con copias de hasta 200 MiB;
  - `adbuddy send`: una FIFO puesta entre el `Lstat` y el `open` bloquea (abrir con `O_NONBLOCK` en Unix);
  - `daemon/pcwrite.go` debe usar `internal/pathguard` (hoy son dos copias) cuando `daemon.go` quede libre de M5a;
  - `-race` no se ha corrido en M3a (sin cgo en el PC de desarrollo): que lo corra la CI de Linux.
  - **`ADBUDDY_SOURCE` lo declara el cliente:** un agente que vacía la variable puede hacer `attach`, `new` o `stop` sobre su propia sesión. La defensa real es la identidad del par del socket IPC (M4 o antes).
  - `adbuddy send`: entre `ReparseBelow` y `Lstat` hay una ventana en la que un componente puede pasar a junction, y la segunda resolución no la ve (`EvalSymlinks` no sigue junctions).
  - **El lector del bridge escribe en el PTY sin tope:** `chat.send` y `terminal.input` escriben en línea desde el lector, así que un agente que no lee su stdin bloquea también `confirm.response` (el 🔴 caduca y se deniega: falla seguro). También bloquean el lector `session.new`/`stop` (hasta 5 s más el kill) y el trabajo de disco de `attach.*`; si pasa de 10 s, falla el ping. Arreglo: un escritor por sesión con cola acotada.
  - **Subidas y enlaces en `inbox/` (residuales de la ola final):** `onUploadFrame` y el `fail` de `endUpload` borran `.partial/<id>` y `<id>.json` sin comprobar `plainDir` (solo nombres de id válidos); entre `inboxPlain` y `moveExclusive` queda una ventana TOCTOU que solo cierra `openat`; la comprobación de `endUpload`/`onAttachEnd` no tiene un test que la ejerza (haría falta un gancho como `afterOpen` de `send.go`); si `inboxPlain` falla tras mover, el fichero queda sin registrar.
  - **HECHO en M3b (T1):** `session.state.activity` lleva la última orden del agente (`phone.go`).
- **Aplazados de M3b (revisiones de T4-T10):**
  - la etiqueta "red local o Tailscale" de la cabecera (M5b, con `RemoteState`); el límite de descarga automática en remoto ya usa `state.remote.enabled`;
  - las preguntas 🔴 como tarjetas en el chat en vez del diálogo de M2 (**M4 lo descartó con un ruling**, spec §7.5: son otra clase de pregunta, del shim, y mezclarlas con las del agente invita a contestar una por otra; siguen en `ConfirmDialog`; sigue como decisión de producto para un hito posterior);
  - **sin `attach.cancel` en el protocolo:** una subida quitada a medias sigue contando en el host (uno de los 4 huecos) hasta reconectar o una hora de inactividad; quitar y volver a añadir varios adjuntos grandes seguidos puede dar `busy`;
  - la descarga automática vuelve a arrancar lo que la persona canceló (`downloads[a.id] == null`);
  - las descargas escriben en el hilo lector de OkHttp (256 KiB por trama): si hay lag en los pings con almacenamiento lento, pasarlas a un escritor aparte;
  - `TermWebPolicyTest`: casos de borde (resto vacío, `?x=1`, `#frag`, `a?/../b`, `%2F`); el WebView se recrea sin límite si el renderizador muere en bucle (limitar a 3 y mostrar un error); `onReceivedError`/`onReceivedHttpError` visibles;
  - la salida en vivo de la terminal del móvil contesta las consultas (DA, DSR) como una terminal más: con un PC enganchado el agente las recibe duplicadas (contenido fijo, sin inyección); si molesta, handlers de parser en `term.js`;
  - `Notify.doneLine` puede tomar la respuesta anterior si la última llega después de `session.state: waiting`;
  - `ACTION_RESUME` de la notificación no exige desbloqueo (como en M2): decidir en M5;
  - `TerminalModel`: el tamaño de la terminal lo manda el host y la vista hace scroll horizontal si no cabe;
  - **Revisión final de M3b (menores, DEFER):** la burbuja sobre una `MainActivity` visible comparte un solo sink en `TerminalFeed` (al plegarla se manda `unwatch` y la terminal principal se congela hasta un onStop/onStart: pila de sinks); `PickMultipleVisualMedia(10)` no descuenta los borradores que ya hay; `trim()` puede borrar un fichero que sigue en `DownloadState.Done` (al pulsar, volver a `get`); `Downloads.get` hace E/S en el hilo principal; en `phone.go`, `state()` y `Publish` no van juntos bajo el lock (patrón anterior a M3b; la siguiente publicación lo corrige); "Trabajando: <orden>" no caduca (M4: **resuelto en parte para Claude Code**, donde el estado lo fijan los hooks, `UserPromptSubmit`/`Stop`, spec §6.1; sin cambios para Codex y Gemini; sin ver en un móvil);
  - M3b se probó con tests JVM y `lintDebug`; nada de Compose, WebView ni notificaciones se ha visto en un móvil (ver "Validaciones manuales" del plan de M3b).
  - **`Busy` solo reconoce los diálogos de Claude Code en inglés:** un Enter del chat puede aprobar un permiso de Codex o Gemini (adb sigue pasando por el shim). **Resuelto para Claude Code en M4** (spec §6.2): con el adaptador, `chat.send` responde `busy` si hay permisos pendientes o atención, aunque la pantalla no se reconozca; sigue pendiente para Codex y Gemini (v1.1), que no tienen adaptador.
- **`adbuddy doctor` desde la sesión del agente** usa el adb real fuera del shim (solo lecturas fijas, sin argumentos del agente) aunque haya pausa o una pregunta pendiente, y no lo apunta en el diario. Decidir si se registra o se bloquea en pausa.

## M5 · Ayudas al agente

- `UIRequest.Args` no lleva `omitempty`: un `Args` nil sale como `"args":null`. El host manda siempre `{}` en las operaciones sin argumentos (o la app trata `JsonNull` como `{}`).
- **`adbuddy screen` con el servicio de accesibilidad vinculado y la app desconectada:** devolver un error claro en lugar de usar `uiautomator dump`, que desconectaría el servicio (§6). Mensaje propuesto: `ADBuddy's accessibility service is running but the app is not connected: open the ADBuddy app and try again`.
- **Revisar ARTEMIS de Google antes de escribir las ayudas** (`github.com/google/artemis`, Apache-2.0; revisado en el commit 351ca84 del 2026-09-28). Su app auxiliar (`packages/artemis-accessibility-helper`) resuelve lo mismo que `AgentAccessibilityService` y se puede reutilizar con atribución. Ideas concretas:
  - `HierarchyDumper`: sin `waitForIdle` (no se cuelga), todas las ventanas (app, diálogos, System UI, teclado, pantalla partida), nodos invisibles fuera y límites recortados a la pantalla, a la ventana y a los ancestros con scroll (paridad con UIAutomator), campos semánticos (`error`, `is_heading`, `editable`, `pane_title`, `hint`), captura de pantalla y árbol en el mismo instante (API 30+), IPC por lotes en API 33+ y saneado estricto de XML.
  - `GestureController`: toques, pulsación larga, deslizar y escribir con `dispatchGesture`.
  - Localizar "primero dinámico, coordenadas de respaldo": id, texto o descripción antes que posición.
  - Detectar quién tiene UiAutomation (uiautomator2, Appium) antes de leer la pantalla, porque desactiva los demás servicios de accesibilidad (lo vimos en S2 con `uiautomator dump`).
  - Versión mínima del protocolo entre host y app (`MIN_PROTOCOL_VERSION`), para detectar una app desfasada.
  - Receptor del token protegido con `android:permission="android.permission.WRITE_SECURE_SETTINGS"`: solo el usuario shell o el sistema pueden enviarlo. Útil como alternativa al proveedor, pero **sin** pasar el token en la línea de comandos como hace ARTEMIS (`am broadcast --es token …`): lo mantenemos por stdin (plan de M2).

- **Revisión del plan de M5a (opus), para M5c:**
  - **I7: DECIDIDO en M5b** (rulings 7 y 8 de `docs/superpowers/plans/2026-10-01-adbuddy-m5b-modo-remoto.md`): la persiana y los ajustes rápidos desplegados cuentan como "protegido" (guarda con `dumpsys window`, y heads-up con la app o con `dumpsys window windows`); `screenrecord` y `bugreport` son 🔴 especiales; `dumpsys activity top` y `dumpsys` sin servicio pasan por la guarda como lectura de pantalla; `dumpsys notification` con `--noredact`/`--reveal` (o cualquier `dumpsys` con ellos) es 🔴 especial `notifications.reveal`. Lo que queda:
    - **Límite honesto de v1 / v1.1:** `screenrecord` ya iniciado no se corta a media grabación (solo arranca con confirmación). `dumpsys window` sigue 🟢 (solo títulos de ventana). Si en la validación 17 el respaldo por adb bloquea de más o de menos, afinar `fixedSystemUI` (v1.1).
    - **Mensajes y Gmail** (reciben códigos) en la pantalla de M5c, además de la heurística de M5a.
    - **Pantalla de bloqueo:** la guarda bloquea todo salvo Back/Home y la tecla de despertar; documentarlo y comprobarlo (validación 14). Persianas de otros fabricantes (MIUI/HyperOS, etc.): validación 14.
    - **Texto para el dueño de la plantilla `AGENTS.md`** (e5 / skimmer-32), a pasar tal cual: "`adbuddy remote on|off|status` lo usa el usuario, no el agente; desde la sesión del agente se rechaza (`AGENT_SESSION`)." Y los modismos que la guarda bloquea siempre: `dumpsys activity top | grep …` y `dumpsys … | grep …` dan "must run alone"; usa los helpers de `adbuddy` (`screen`, `foreground`…) o la orden sola. `--noredact`/`--reveal` en cualquier `dumpsys` son 🔴 especiales y `bugreport` también.
    - **`bugreportz` y `dumpstate`** en el shell del móvil son 🔴 por desconocidos pero no especiales (T10b M6): valorar si deben ser especiales (M6).
    - **Texto de la guarda y de la CLI "is a protected app"** cuando la causa es una ventana activa y no un paquete: hacerlo genérico (T10c M2).
    - **Menores aplazados de M5b a recoger:**
      - T3: la lectura del puerto está duplicada con `device.TCPPort`; `adb usb` se repite en cada tic si `SetUntil` falla (T5 lo espacia, máximo 2 min); `Disable` sin modo remoto reinicia adbd.
      - T4: `readTLSPort` usa `context.Background` (pasar el ctx del enlace); `warned` sin tope.
      - T5: un `attach` puede publicar tarde un estado viejo (lado seguro); `TestWatchRemoteResetsAfterASuccess` usa `Sleep(300ms)` (cambiar por `waitUntil` si falla en CI con `-race`); falta test de parada de `serve` con peticiones de `remote.set` en cola.
      - T7: sin test de la rama `disconnect` del vigilante con identidad ilegible; `Online(ctx)` sin tope en `step`; texto `DEVICE_NOT_FOUND` ambiguo.
      - T8: ventana de µs si la pausa llega entre `Paused()` y `askCancel` (el chequeo posterior deniega igual); tests de pausa/una pregunta con esperas de 5 s (vigilar en CI con `-race`); `REMOTE_UNAVAILABLE` depende del texto de adb.
      - T9: `TestServeWiresRemoteActive` busca texto en `serve.go`: extraer `daemonOptions(...)` y probar el comportamiento; no cubre Closing.
      - T10: consultas por adb ~900 ms por llamada de la guarda (una sola consulta bastaría).
      - T11/T13: extraer `remoteMode` de `ChatRoot` a función pura con test; `HH:mm` fuerza 24 h; la tarjeta no está en `ActivityScreen`; `UnknownMark` nunca se limpia.
      - T14: `Overlay.show` no restaura el flag de interrupción; el cableado `Overlay`/`WindowManager` no tiene test JVM (validación 19); carrera mínima en la pausa entre `lateBlock` y `acting`.
    - **Descartados por ruido** (en el ledger, no hace falta recogerlos): bordes de CGNAT y puerto 0 en tests de config (T2), `testutil.ReadLog` con línea a medias (T6/T7), `setVar(knownAddrWait)` inútil (T7), `@Volatile` preventivo (T14), JSON de `enabled` sin distinguir mayúsculas (T5), comentario de `daemon.Options.Serial` (T6, tocado ya en T7).
    - **Spec M5 §7.2:** `cfg.Address` es el primer candidato de la reconexión (corregido en la spec).
    - **Validaciones manuales de M5b que dependen de decisiones futuras:** 14 (otros fabricantes: si hace falta una lista de títulos por fabricante) y 17 (si la ventana de 8 s del heads-up debe seguir el ajuste "tiempo para actuar" de Accesibilidad, y si las actualizaciones de notificación no emiten evento). El resto vive en el plan.
  - La heurística de M5a no protege Mensajes ni Gmail, que reciben códigos: proponerlas en la pantalla de M5c.
- **ADBuddy es app protegida frente a sus propias ayudas** (revisión de M2): las acciones de accesibilidad (tocar, leer la pantalla) nunca actúan sobre `tech.hybriq.adbuddy`, para que el agente no pueda pulsar Permitir en su diálogo.
- **Si aparece otra vía de inyección de toques fuera del shim**, pedir autenticación (BiometricPrompt o credencial) al pulsar Permitir en la app. En M2 bastan el bloqueo de órdenes mientras hay una pregunta 🔴 pendiente y que los trabajos en segundo plano sean 🔴 especiales.
- **`TokenProvider` y `BridgeService` no sobrescriben `dump()`**: `dumpsys activity provider|service` es 🟢 para el agente.

## M6 · Publicación

- **README, ventaja de coste frente a ARTEMIS:** ADBuddy usa el agente de código que el usuario ya tiene y funciona con su suscripción (por ejemplo, un plan de Claude Code), sin claves de API. ARTEMIS necesita su propio LLM y sus claves de API, porque el agente le delega tareas enteras. Decirlo en una línea al comparar, y en la portada entre las ventajas.

- **Hecho en M6a** (`docs/superpowers/plans/2026-10-01-adbuddy-m6a-releases.md`):
  - el host fija el SHA-256 del certificado de firma y lo comprueba en el APK descargado y en la app instalada, antes de cada token y de cada concesión (`phoneapp.Verified`);
  - `ADBUDDY_RELEASE_URL` queda limitado a `https://` o loopback y a `MinAppVersionCode` (spec de M6a §5-§6).

  Falta que el usuario haga lo de `docs/releasing.md`, "User actions".
- **M6c, tras la primera release pública:** tap de Homebrew (`hybriqtech/homebrew-tap` y un token con permiso de escritura), manifiesto de winget (`hybriqtech.adbuddy`, PR a `microsoft/winget-pkgs`), IzzyOnDroid, firma de binarios de Windows con SignPath Foundation y procedencia firmada (`actions/attest-build-provenance`). IzzyOnDroid no acepta el bloque de dependencias cifrado de AGP (`0x504b4453`): antes de pedirlo, `dependenciesInfo { includeInApk = false }` en `android/app/build.gradle.kts`, coordinado con quien lleve `android/` entonces.
- **Rotación de la clave de firma:** no está soportada (spec de M6a §11). Si hace falta, el host tiene que leer el linaje de v3 antes de que salga un APK con la clave nueva.
- **`adbuddy doctor`** no comprueba aún el certificado de la app instalada. Podría decir "app de confianza: release / desarrollo / no" con `phoneapp.Verifier`.
- **Aplazados de las revisiones de M6a:**
  - neutralizar la app por UID o PID (`appops set <uid>`, `kill`, `am kill-all`, `cmd netpolicy`) sale 🔴 no especial: valorar si debe ser especial;
  - validar en el emulador que `appops reset com.no.existe` resetea los modos por UID;
  - endurecimientos opcionales de `setup`: vaciar la caché de `Same` tras un fallo definitivo de `Verify`; exigir un fichero regular en `--apk` (un FIFO sin escritor bloquea).

- **`adbuddy remote on` y `ADBUDDY_SOURCE`** (M5b, Tarea 8): igual que la nota de `ADBUDDY_SOURCE` de más arriba, la variable la declara el cliente, así que un agente que la borre llega al socket. Hoy la defensa real de `on` es la pregunta al usuario (app o terminal de `serve`, sin `Key`, como `Special` y una por petición). Las credenciales del par del socket serían solo una defensa PARCIAL: un proceso del mismo usuario puede leer `ipc.secret`, y la ascendencia de procesos se escapa con `setsid`, `nohup` o `schtasks`.

## Más adelante (v1.1+)

- **Reescribir la orden antes de ejecutarla** (spec de M2.5, §2.3): el host regeneraría el script desde el árbol, con cada palabra literal entre comillas simples, y ejecutaría esa forma canónica en vez del texto del agente. Cierra del todo la diferencia entre el parser de Go y mksh, que hoy solo vigilan los tests diferenciales. Cambia lo que se ejecuta (el diario tendría dos textos) y toca el shim y el daemon.

- **Proxy del protocolo del servidor adb.** Herramientas como ARTEMIS (`adbutils`, `uiautomator2`) hablan con el servidor adb por su protocolo, sin el binario `adb`, así que se saltan el shim. Un proxy que atienda ese protocolo y clasifique las peticiones `host:transport` / `shell:` las pondría bajo el clasificador, el diario y las confirmaciones. Es un trabajo grande: evaluarlo cuando haya demanda.

- **Protocolo v2: el host se autentica ante la app** (revisión de la Tarea 9 de M2): hoy una app que ocupe el puerto mientras no hay túnel puede mostrar preguntas falsas (sus respuestas no llegan al host). Un HMAC del token sobre un nonce de la app dentro del `welcome` lo cerraría.

## M4 · Adaptador de Claude Code

Spec: `docs/superpowers/specs/2026-10-01-adbuddy-m4-claude-code-design.md`. Plan (registro): `docs/superpowers/plans/2026-10-01-adbuddy-m4-claude-code.md`. Las revisiones y rulings viven en el ledger local de la sesión (no versionado); lo esencial está copiado en el plan (registro) y aquí.

### Validaciones manuales pendientes (nadie las ha corrido en un móvil)

- **Las de la spec §11:** `Notification` y sus tipos; permisos de Edit/Write/WebFetch/MCP; deny con `message`; `/clear`, `/compact`, `--resume`; prompts con `/` y `!`; cómo se ve todo en el móvil real.
- **Qué hace Claude Code con el hook si se responde en la terminal (la duda principal, §5.2):** hoy el daemon se entera de que un permiso se contestó en la terminal por el cierre del hook o por el fin de la herramienta en el transcript. Hay que comprobar con Claude Code real que una de las dos señales llega siempre, y cuánto tarda, para que la tarjeta del móvil no quede viva con un permiso ya contestado.
- **Detalle largo de la tarjeta, en el móvil:** con letra grande, giro de pantalla y teclado abierto, Sí y Sí, siempre siguen inactivos hasta desplazar el detalle al final, el detalle se desplaza dentro de la tarjeta sin cortarse y No está disponible. Los composables (`MarkdownText`, `PermissionCard`, `ToolStripView`) no tienen tests: es la única comprobación que tienen.
- **Aviso de permiso silencioso:** con la app en segundo plano y un permiso pendiente, el aviso genérico ("Claude Code necesita una respuesta") sale sin sonido ni cabecera emergente (canal `conversation`, importancia LOW). Comprobar cómo se ve con el móvil bloqueado (sin comando ni texto, solo Cortar acceso) y con la app en primer plano.

### Decisión de producto pendiente

- **Canal de importancia alta para el aviso "Claude Code necesita una respuesta":** hoy es silencioso (canal `conversation`, LOW; `Ruling` de la tarea E). Un permiso pendiente con la app en segundo plano puede pasar desapercibido hasta que se mire el móvil; caduca a los 600 s y vuelve a la pregunta de la terminal. Falta un canal propio de importancia alta con versión pública genérica (spec M4 §7.4). Decisión de M3/M5, no de M4.

### Menores diferidos de las revisiones (no ejecutan nada sin confirmación)

De la revisión opus de seguridad del host y de su re-revisión:
- **M1, TOCTOU del transcript:** `tail.go` reabre la ruta en cada sondeo y la validación (`ValidTranscript`) se hizo una vez; el `Lstat` de la ronda B solo mira el último componente, así que un directorio intermedio cambiado por un enlace sigue colándose. Arreglo: abrir con el descriptor ya validado o repetir `EvalSymlinks` + `Inside`. El test del enlace se salta en Windows (corre en la CI de Linux).
- **M7, retirada de tarjeta por herramienta + resumen:** `resolvePermsOf` retira una tarjeta cuando acaba cualquier herramienta con el mismo nombre y resumen (`description`). Es seguro (devuelve la decisión a la terminal) pero puede confundir. Cerrarlo con la validación de §5.2 (id de herramienta).
- **M-a, carrera entre `chat.send` y `PermissionRequest`:** el freno de `agentBusy` solo actúa cuando la tarjeta ya existe; un `chat.send` admitido justo antes de que llegue el `PermissionRequest` puede teclear sobre el diálogo. Ventana de milisegundos, sin arreglo barato (el host no sabe que Claude Code va a preguntar).
- **M-c, NotebookEdit:** el resumen y el detalle usan `notebook_path` (`rule.go`) y la guarda de la tarjeta (`CardSafe`) mira otros campos de ruta; comprobar con una grabación real de `NotebookEdit` que `notebook_path` es el único y que `CardSafe` ve lo que la persona aprueba.
- **N3, resto:** `shownRune` ya rechaza los selectores de variación (Mn: U+FE0F, U+E0100…), las etiquetas U+E0000 (Cf), control, Z, Mn, Me y una lista fija de rellenos (U+3164, U+115F, U+1160, U+FFA0, U+2800, U+034F). Lo que queda sin rechazar: caracteres de uso privado (Co, p. ej. U+E000), puntos de código sin asignar (Cn) y símbolos sin glifo como U+FFFC (So).
- **`android` como subcadena y sangrías de más de 8 espacios:** el rechazo de "Sí, siempre" por la subcadena `android` y el de la tarjeta por blancos largos llevan a la terminal comandos legítimos (lado seguro, a costa de comodidad). Además `android_serial` es redundante con `android`.
- **Límite léxico de `AlwaysRule`:** ningún filtro léxico cubre `make install`, scripts propios o alias que llamen a `adb` por dentro (spec §5.3); el "Sí, siempre" se ofrece para ellos. La lista blanca de primeras palabras es un cambio de producto mayor.
- **`isCompactSummary`** no se trata (el resumen de `/compact` puede salir como mensaje de usuario); validar con un `/compact` real. **`<user-prompt`** está en `ignoredPrompts` aunque la spec §4.2 no lo lista (inocuo, ignora prompts internos): ponerlo en la spec o quitarlo.
- **Me-1, qué enseña la tarjeta (revisión final opus):** la de Edit, MultiEdit, Write y NotebookEdit enseña la ruta, no el contenido que se escribe (decisión de la spec, fijada en `claudecode_test.go`), y la de Bash no enseña `dangerouslyDisableSandbox` ni `run_in_background`. Un "Sí" aprueba la entrada entera. Opciones: un aviso en la tarjeta cuando esos campos vengan activados, o dejar esas herramientas a la terminal. (El caso de otras herramientas, como MCP con `url`, ya está cerrado: enseñan su entrada entera o van a la terminal.)
- **Me-5, desvíos de la spec por el lado seguro (revisión final opus):** un `tool_result` no limpia la atención (§6.3); `agent_needs_input` no pasa a `waiting` (§3.1); y la atención caduca a los 120 s incluso con un permiso dejado a la terminal por una tarjeta no apta, y desde entonces solo frena `mgr.Busy()`, que reconoce únicamente textos en inglés. Decidir si se ajusta la spec o el código.
- **Sin test del adaptador que relea el transcript con otro `Tailer`** (el id estable de los prompts de la terminal solo se prueba en el parser), y una aserción débil en `TestParserCapsAMessageBeforeJoining`.
- **`wasSent` expira a los 2 min:** un prompt del móvil que Claude Code tarda más en escribir en el transcript sale repetido como mensaje de la terminal.
- **Ids y versión del transcript:** acotados y validados en la tarea F (ids `^[A-Za-z0-9._-]{1,64}$`, versión `^[0-9A-Za-z._+-]{1,32}$`, spec §4.2/§4.4). Una versión no válida cuenta como no soportada (degrada a la terminal con un texto fijo) y no borra una válida ya vista; los mensajes assistant del transcript llevan el prefijo `a_`. Un mensaje assistant sin `message.id` ni uuid válidos no se muestra: se cuenta (`Parser.Dropped`) y el adaptador lo deja en el log, pero la persona no ve nada en el móvil.

De las revisiones de la app (Sonnet y opus):
- **Color de la etiqueta "Claude Code pregunta":** `tertiary` sobre `tertiaryContainer`, contraste por comprobar (como `PaletteContrastTest`).
- **Quitar los caracteres Cf del texto de enlace** rompe las secuencias con ZWJ/ZWNJ (emoji compuestos, escrituras que los usan): solo afecta al aspecto.
- **`LastMessage.text` de la notificación persistente** (privada) no se sanea de bidi ni saltos (anterior a M4); el titular de "Listo" sí.
- **Markdown:** `MAX_BLOCKS` admite un bloque más de la cuenta y una imagen anidada dentro de un enlace no está cubierta por un test propio.
- **Herramientas atascadas en `running`:** si el transcript no trae el `tool_result` (herramienta interrumpida), el chip se queda en `running` hasta la sesión siguiente. Un tope de tiempo o un `Stop` que las cierre.
- **`Notification` y `Stop` sin transcript:** con el transcript degradado, solo `Stop` trae el último mensaje y `Notification` no se muestra; falta decidir qué ve la persona en ese caso.
- **Conflación de `connected` en `ChatController`** (ronda C): una tarjeta puede quedar en "contestando" hasta `permission.resolved` o los 600 s del host en el caso raro de una reconexión rápida.
