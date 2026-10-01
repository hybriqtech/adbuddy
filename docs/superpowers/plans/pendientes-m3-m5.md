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

- **La app muestra los `notice` del host** (revisión final de M2): hoy los ignora; el de respuesta tardía ("That question was already answered or moved to the PC terminal.") y el de tipo no soportado deberían verse en la pantalla de inicio.
- **Fixtures JSON compartidas del protocolo:** las hace M3.0 en `protocol/fixtures/` (spec `2026-10-01-adbuddy-protocolo-m3-m5-design.md`, §12).
- **Diseño de la app** (prueba del usuario en su Pixel 9 Pro XL con Android 17, 2026-10-01: "funciona bien aunque el diseño de la app es feo"). Antes de las pantallas nuevas de M3 (chat, terminal, adjuntos), una base visual común, que se aplica también a las que ya existen (consentimiento, inicio, Actividad, confirmaciones):
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
  - **`session.state.activity` no se rellena** (spec M3 §3.5): M3b lo necesita para la notificación de "Trabajando".
  - **`Busy` solo reconoce los diálogos de Claude Code en inglés:** un Enter del chat puede aprobar un permiso de Codex o Gemini (adb sigue pasando por el shim). Para M4, con los permisos con botones.
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

- **ADBuddy es app protegida frente a sus propias ayudas** (revisión de M2): las acciones de accesibilidad (tocar, leer la pantalla) nunca actúan sobre `tech.hybriq.adbuddy`, para que el agente no pueda pulsar Permitir en su diálogo.
- **Si aparece otra vía de inyección de toques fuera del shim**, pedir autenticación (BiometricPrompt o credencial) al pulsar Permitir en la app. En M2 bastan el bloqueo de órdenes mientras hay una pregunta 🔴 pendiente y que los trabajos en segundo plano sean 🔴 especiales.
- **`TokenProvider` y `BridgeService` no sobrescriben `dump()`**: `dumpsys activity provider|service` es 🟢 para el agente.

## M6 · Publicación

- **README, ventaja de coste frente a ARTEMIS:** ADBuddy usa el agente de código que el usuario ya tiene y funciona con su suscripción (por ejemplo, un plan de Claude Code), sin claves de API. ARTEMIS necesita su propio LLM y sus claves de API, porque el agente le delega tareas enteras. Decirlo en una línea al comparar, y en la portada entre las ventajas.

- **`ADBUDDY_RELEASE_URL`** cambia a la vez el origen del APK y el de su checksum: es un gancho de test que vive en producción. Documentarlo o limitarlo junto con el certificado de firma.
- **Fijar en el host el SHA-256 del certificado de firma del APK** (revisión de la Tarea 4 de M2): el `.sha256` de la release solo protege la integridad, no una release comprometida en la primera instalación.

## Más adelante (v1.1+)

- **Reescribir la orden antes de ejecutarla** (spec de M2.5, §2.3): el host regeneraría el script desde el árbol, con cada palabra literal entre comillas simples, y ejecutaría esa forma canónica en vez del texto del agente. Cierra del todo la diferencia entre el parser de Go y mksh, que hoy solo vigilan los tests diferenciales. Cambia lo que se ejecuta (el diario tendría dos textos) y toca el shim y el daemon.

- **Proxy del protocolo del servidor adb.** Herramientas como ARTEMIS (`adbutils`, `uiautomator2`) hablan con el servidor adb por su protocolo, sin el binario `adb`, así que se saltan el shim. Un proxy que atienda ese protocolo y clasifique las peticiones `host:transport` / `shell:` las pondría bajo el clasificador, el diario y las confirmaciones. Es un trabajo grande: evaluarlo cuando haya demanda.

- **Protocolo v2: el host se autentica ante la app** (revisión de la Tarea 9 de M2): hoy una app que ocupe el puerto mientras no hay túnel puede mostrar preguntas falsas (sus respuestas no llegan al host). Un HMAC del token sobre un nonce de la app dentro del `welcome` lo cerraría.
