# ADBuddy M3a · Host del chat · Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Objetivo:** el host de M3 (spec `docs/superpowers/specs/2026-10-01-adbuddy-m3-chat-design.md`, §3-§6 y §8-§11): la sesión del agente en un PTY dentro de `serve`, `adbuddy attach`, la terminal y el chat hacia el móvil, ficheros en los dos sentidos, captura y `adbuddy send`.

**Estado:** las tareas 1-5 ya están en la rama `m3`. Son el prototipo del riesgo (PTY, pantalla, sesión, attach), reorganizado en commits por tarea por decisión del supervisor (ledger `.superpowers/sdd/2026-10-01-adbuddy-m3a/progress.md`):

| Tarea | Commit | Contenido |
|---|---|---|
| 1 | e6c0ae0 | `host/internal/pty`: `pty.Start(Options) (PTY, error)`; ConPTY en Windows y `creack/pty` en Unix; `Resize`, `Wait`, `Kill` del árbol |
| 2 | 9d74280 | `host/internal/term`: `Screen` (emulador VT, cola de 256 KiB con `epoch`/`offset`, `Since`, `Snapshot`, `Text`, `BracketedPaste`) |
| 3 | a7878ce | `host/internal/ipc`: `KindAttach`, `KindSession`, `KindSend`, `FrameResize`, `Conn.Resizes()`, `ipc.Attach` |
| 4 | 20baeea | `host/internal/session`: `Manager` (estado, `--continue` con vuelta atrás, tamaño, `Paste`, `Busy`, `Subscribe`); `agent.TrustFolder` |
| 5 | feb66ae | `session.ServeAttach`; `adbuddy attach`, `new` y `stop`; `serve` lanza al agente (`--no-agent` para no hacerlo); los tests nunca lanzan un agente real |
| 1-2 | 87a7f96, 55c828a, 425dd0d | Ronda de arreglos de la revisión de T1-T2: ConPTY propio sobre `x/sys/windows` (`Read` termina al salir el agente, tras su última salida); `session.pump` acaba también cuando vuelve `Wait`; pantalla sin scrollback; `Close` sin carrera; cola sin copias. `Since` puede devolver hasta 512 KiB (`2*TailSize`) |
| 3-5 | df3ac6d | Ronda de arreglos de la revisión opus: `Start`/`Stop` en serie con generación; el agente no controla su sesión y solo se aceptan nombres de `agent.IsKnown`; aviso en attach cuando el agente sale; modos de terminal en `Screen.Modes()` y `Snapshot()`; `Busy` también con los permisos de Claude Code; `Stop` amable (`PTY.Hangup`, 5 s o 2 s en Windows, después `Kill`) |

Este plan describe las tareas 6-10: requisitos, interfaces y tests. El código lo escribe el implementador con TDD. **Si un requisito no se puede cumplir tal cual, detente y explícalo.** Revisado por `adbuddy-reviewer-opus` antes de la tarea 6 (16 importantes incorporados).

## Restricciones globales

- **Rama `m3`.** Integración en `main` solo con el ok del supervisor (adbuddy-e5); nunca `push --force`.
- **Go:** `go` y `gofmt` en el PATH. Los tests, siempre con `-timeout` y `-count=1`, **por paquete** (`go test ./internal/cli/ ./internal/bridge/`…); la suite completa solo al final de la tarea.
- **Antes de cada commit:** `gofmt -l` vacío, y `go vet ./...` con `GOOS=windows`, `linux` y `darwin` (el PTY tiene código por sistema).
- **Commits:** identidad local (hybriqtech), trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`, `git add` con rutas explícitas.
- **Protocolo:** no cambies tipos, campos ni fixtures (`protocol*.go`, `Protocol*.kt` y `protocol/fixtures`). Úsalo como está (spec del protocolo §3-§9). Los códigos de error son los del protocolo §9.
- **Nunca un agente real en los tests:** en `internal/cli`, `TestMain` sustituye `lookAgent`; el e2e de la tarea 10 usa un agente falso y un `PATH` que solo lo contiene a él.
- **Privacidad (spec M3 §10):** ni el texto del chat ni el de la terminal se escriben en ningún log (stderr de `serve`, diario, mensajes de error).
- **Ficheros que no se tocan** (los cambian otras sesiones): `host/internal/daemon/daemon.go`, `host/internal/ipc/ipc.go` (M5a) y, en `host/cmd/adbuddy/e2e_test.go`, `startWith`/`startWithData` (M6a).
- **Ficheros compartidos:** `host/internal/workspace/templates/AGENTS.md`, `root.go`, `serve.go` y `phone.go`. Solo líneas propias añadidas; no edites las de otros. **No añadas áreas a `setAreas` (`phone.go:147`, la comparte M5):** `chatArea()` reúne los manejadores de `phone_chat.go`, `phone_term.go` y `phone_files.go`.
- **Locks:** nunca llames al `Manager` ni al `bridge.Server` (ni esperes en un canal) con el mutex de un área tomado. `pump` llama a las retrollamadas de `Subscribe` y puede quedarse esperando ese mutex mientras otro manejador espera al PTY.
- **El móvil del usuario no se toca.** **Sin subagentes** dentro de una tarea.

## Orden y revisores

| Tarea | Ficheros | Depende de | Revisor |
|---|---|---|---|
| 6 · Sesión y chat en el móvil | `host/internal/cli/phone_chat.go`, `phone_chat_test.go`, `phone_areas_test.go` (un test), `phone.go` solo si hace falta | 1-5 | `adbuddy-reviewer-opus` (entrada del móvil al PTY) |
| 7 · Terminal hacia el móvil | `host/internal/cli/phone_term.go` (nuevo) y su test; `phone_chat.go` y `phone.go` (enganche); `bridge/server.go` (`Backlog`) y `send_test.go` | 6 | `adbuddy-reviewer` |
| 8 · Ficheros y captura | `host/internal/cli/phone_files.go` (nuevo) y su test; `phone_chat.go` y `phone.go` (enganche, `useWorkspace`, adb); `serve.go` (una línea); `bridge/server.go` (`SendFrameTo`) y su test | 6 | `adbuddy-reviewer-opus` (canal y disco) |
| 9 · `adbuddy send` | `host/internal/cli/send.go` (nuevo) y su test, `serve.go` (una línea de enrutado), `root.go` (una línea), `AGENTS.md` (una línea) | 8 | `adbuddy-reviewer-opus` (contención en el entorno) |
| 10 · Extremo a extremo | `host/internal/testutil/fakeagent` (nuevo), `host/cmd/adbuddy/chat_e2e_test.go` (nuevo) | 6-9 | `adbuddy-reviewer` |

Las tareas son secuenciales: todas tocan `internal/cli`.

---

### Tarea 6: Sesión y chat en el móvil

**Contexto:** `phoneLink` ya tiene `manager *session.Manager` (`useSession`) y publica `session.state` con `sessionInfo`. `chatArea()` (en `phone_chat.go`) devuelve hoy `area{}`. Las áreas se describen en `phone_areas.go`.

**Ojo con la API de la sesión (ronda df3ac6d):**
- `Subscribe` entrega también dos `Output` sin datos por proceso: uno al arrancar (época nueva) y otro con `End` al acabar.
- El nombre que llega en `session.new {agent}` se valida con `agent.IsKnown`.
- `chatArea()` no puede fallar con un `phoneLink{}` vacío (`TestAreasDoNotShareTypes`).

**Requisitos** (spec M3 §5.1 y §5.3; spec del protocolo §3, §4.3, §5.2 y §9):
1. **`chatArea()`:** registra los manejadores de `chat.send`, `session.new`, `session.stop` y `agent.interrupt`, y aporta su parte del `snapshot` (`chat`).
2. **`chat.send` (lleva `rid`; un `rid` malo ya lo descarta el servidor, `server.go:427`):**
   - `id` que no cumple `bridge.ValidRID`: `error` `invalid`.
   - `text` de más de 64 KiB o más de 10 adjuntos: `error` `too_large`.
   - Si el `id` ya está en el historial, vuelve a confirmar el mensaje existente con `PublishRID` y no lo inyecta.
   - **Reintentos sin duplicados:** reserva el `id` en un conjunto de ids en curso, bajo el mutex del área, antes de `Paste`. Si ya está reservado, no se inyecta y responde `error` `busy` (el reintento siguiente recibe la confirmación del existente). Libera la reserva si falla. El mutex nunca está tomado durante `Paste`.
   - Sin agente (`ErrNoAgent`): `error` `no_agent`. Si `manager.Busy()` dice algo: `error` `busy` con ese texto. Otro error de `Paste`: `error` `failed`.
   - Cada adjunto debe existir en el registro de adjuntos (tarea 8). Hasta entonces, cualquier id da `not_found`. Expón un `attachments` (mapa protegido por mutex con id → `bridge.Attachment` y la ruta relativa en `inbox/` o `outbox/`) que la tarea 8 rellenará.
   - Texto inyectado: `text`, y si hay adjuntos, `"\n\nAdjuntos:\n- <ruta del registro>"` por cada uno (`inbox/…` u `outbox/…`, siempre con `/`), con `manager.Paste`.
   - Después: guarda y publica `chat.message {id, role: user, text, attachments, conversation}` con `PublishRID` y el `rid` del `chat.send`.
3. **Historial:** los últimos 100 `ChatMessage` de la conversación, en memoria y protegidos por el mutex del área.
   - Un `chat.message` con un `id` existente lo sustituye (upsert).
   - `chatSnapshot` copia el historial con `Attachments` nunca `nil` y sin `thumb` (spec M3 §4.3; pendiente de M3.0).
   - **`chatSnapshot` recorta los mensajes más antiguos hasta que el JSON de `chat` quede por debajo de 512 KiB** (protocolo §4.3 y §9: el `snapshot` lleva también los 🔴 pendientes y la app no acepta más de 1 MiB).
   - El historial se vacía cuando cambia `Conversation` (se lanza otro proceso del agente).
   - Un método `addMessage(ChatMessage)` (guardar y publicar sin `rid`) para la tarea 9.
4. **`session.new {agent?}` (`rid`):** si trae `agent` y no cumple `agent.IsKnown`, `error` `invalid`. Si no, `manager.Start(agent o el actual o el configurado, false)`. Si va bien, `PublishRID(session.state)` con el estado nuevo; si no, `error` `failed` con el texto de `agent.InstallHint`. **`session.stop` (`rid`):** `manager.Stop()` y `PublishRID(session.state)`.
5. **`agent.interrupt` (sin `rid`):** `manager.Input([]byte{0x1b})`. Sin agente no hace nada.
6. **Todo manejador con `rid` responde por todos sus caminos** (spec del protocolo §3).

**Tests** (`phone_chat_test.go`, con el `linkedApp` de `phone_areas_test.go` y una sesión con agente falso):
- usa un `session.Manager` con `Start` **y `Look`** sustituidos por un PTY en memoria, como `fakePTY` en `internal/session/session_test.go` (cópialo o muévelo a `internal/testutil` si lo compartes);
- `chat.send` llega al PTY como pegado y vuelve como `chat.message` con el `rid`; un `id` repetido no se inyecta dos veces; **dos `chat.send` con el mismo `id` a la vez (dos conexiones o dos gorrutinas) dan una sola inyección**;
- `invalid` (id malo), `too_large` (texto de más de 64 KiB y 11 adjuntos), `not_found` (adjunto desconocido), `no_agent` y `busy`; un `rid` malo no inyecta nada ni recibe respuesta;
- `session.new` y `session.stop` responden con un `session.state` con su `rid`; `agent.interrupt` escribe ESC;
- el `snapshot` lleva el historial (como mucho 100) sin `thumb`, y con mensajes grandes se queda por debajo de 512 KiB;
- **`TestUnhandledTypesAreAnswered` (`phone_areas_test.go:51`)** usa `chat.send` como tipo no manejado: cámbialo por un tipo inventado que lleve `rid`.

- [ ] Paso 1: tests que fallan. - [ ] Paso 2: verlos fallar. - [ ] Paso 3: implementar. - [ ] Paso 4: `go test ./internal/cli/` y la suite completa. - [ ] Paso 5: commit `feat(host): chat y sesión desde el móvil`.

---

### Tarea 7: Terminal hacia el móvil

**Enganche:** los manejadores viven en `phone_term.go` y los registra `chatArea()`. La suscripción a la sesión se crea en `useSession` (`phone.go`), que es cuando llega el `Manager`.

**Requisitos** (spec M3 §4.1 y §4.3; spec del protocolo §7):
1. **`bridge.Server.Backlog() int`:** cuántos mensajes esperan en la cola de la app conectada (0 sin app). Test en `send_test.go`.
2. **`terminal.watch {epoch, offset}`:** suscribe la conexión que lo pidió (comprobando `Server.Current(m)`) y guarda el `Message` para `SendTo`.
   - Si `screen.Since(epoch, offset)` da bytes, se mandan como `terminal.output` desde ese `offset`, en trozos de como mucho 64 KiB (`Since` puede devolver hasta 512 KiB, más que una trama del bridge).
   - Si no, se manda `terminal.snapshot` (`screen.Snapshot()`).
   - Sin agente, nada.
3. **Salida en vivo:** una sola suscripción (`manager.Subscribe`) para el área.
   - **Sin huecos ni duplicados:** lleva `next` (el offset esperado) por suscripción, como `session.ServeAttach` (`attach.go:74`). Los bytes por debajo de `next` se descartan (ya llegaron con `Since`/`Snapshot`). Un hueco (llega un offset mayor que `next`) provoca un `snapshot`.
   - Los `Output` sin datos (inicio de proceso) y los de `End` no generan `terminal.output`. El de inicio solo marca el `snapshot` del proceso nuevo como pendiente. `terminal.snapshot` usa `Screen.Snapshot()`, que ya incluye los modos.
   - La salida se acumula y se manda cada 50 ms en trozos de como mucho 64 KiB. El tick corre solo mientras haya suscripción.
   - Si `Backlog()` pasa de 128, se marca la pantalla como pendiente y se descarta lo acumulado. Cuando baja, se manda un `terminal.snapshot`.
   - Un cambio de `epoch` manda el `snapshot` del proceso nuevo.
   - **Tamaño:** el `Manager` no avisa de los cambios. En cada tick se compara `screen.Size()`; si cambió, primero se vacía lo acumulado y después se manda `terminal.size {epoch, offset: Offset(), cols, rows}`.
   - La retrollamada de `Subscribe` solo copia bajo el mutex; los envíos van fuera (ver Locks).
4. **`terminal.unwatch`**, una conexión nueva (`area.attach`) o que la app se vaya (`area.gone`) terminan la suscripción. Usa `Server.Current(m)` para no aceptar un `watch` de una conexión reemplazada.
5. **`terminal.input {data}`:** como mucho 64 KiB, o se descarta; va a `manager.Input`. **`terminal.resize {cols, rows}`:** 1..1000 por lado, o se descarta (protocolo); el host lo acota a 200×100 antes de `manager.ResizePhone`, para que un `terminal.snapshot` quepa en un mensaje. Test: un `snapshot` de una pantalla de 200×100 llena de estilos queda por debajo de 1 MiB; si no, detente y explícalo.
6. **La terminal nunca provoca que se expulse a la app:** `SendTo` solo si `Backlog()` está por debajo del umbral.

**Tests:**
- watch desde un offset en la cola (llegan solo los bytes nuevos) y desde uno viejo (llega un snapshot);
- salida en vivo agrupada; salida que llega entre el `watch` y la suscripción: sin duplicados ni huecos;
- cambio de epoch y de tamaño (el `terminal.size` llega después de la salida anterior);
- unwatch, y conexión nueva sin watch: no recibe nada;
- input y resize con sus límites, y el acotado a 200×100;
- presión: con `Backlog` alto, nada de output y luego un snapshot (inyecta `Backlog` con una variable de test o un umbral configurable).

- [ ] Pasos 1-5 como en la tarea 6. Commit `feat(host): la terminal del agente en el móvil`.

---

### Tarea 8: Ficheros y captura

**Enganche:** `phoneLink` no guarda hoy ni el entorno ni el adb. Añade `useWorkspace(ws)` (y lo que haga falta para el `phoneapp.ADB` de `startPhoneLink`) en `phone.go`, con una línea nueva `link.useWorkspace(ws)` en `serve.go`. Los manejadores viven en `phone_files.go` y los registra `chatArea()`.

**Requisitos** (spec M3 §6; spec del protocolo §6):
1. **`bridge.Server.SendFrameTo(ctx, req Message, frame []byte) error`:** encola una trama binaria para la conexión de `req` (como `SendTo`).
   - **Contrapresión:** las tramas esperan mientras haya más de 4 elementos en cola (~1 MiB), para no adelantarse a los eventos (un `confirm.request` no puede quedar detrás de 32 MiB en modo remoto). Nunca se usa el camino que expulsa a la app. El escritor avisa por un canal cuando hay hueco; la espera termina también con `ctx` o si la conexión se va. **Nunca se espera con `s.mu` ni con el mutex de ficheros tomados.**
   - Las tramas y los mensajes salen en orden por el mismo escritor. La cola de `client` pasa a llevar elementos que son mensaje o trama.
   - Es concurrencia del canal: test de orden (tramas y luego `file.end` por `Reply` llegan en ese orden) y de espera con la cola llena (y que acaba con `ctx` y con la desconexión).
2. **Subidas:**
   - **`attach.begin {id, name, mime, size, sha256}`** (comprobando `Server.Current(m)`):
     - `id` con `bridge.ParseFrameID`; `sha256` de 64 caracteres hex; `size` entre 0 y 200 MiB; `mime` de como mucho 255 caracteres; si no, `invalid`.
     - Como mucho 4 subidas en curso por conexión (`busy`).
     - Cuota: `inbox/` (contando `.partial` y los tamaños declarados de las subidas en curso) por debajo de 2 GiB, o `too_large` con un texto que pida liberar espacio (spec §6.1).
     - Responde `attach.ready {id, offset}`, donde `offset` es lo que ya hay en `inbox/.partial/<id>` si el `sha256` coincide con el de la subida anterior (guardado en `inbox/.partial/<id>.json`), o 0. El `.partial` se mira con `Lstat` (nunca se sigue un enlace); si es mayor que `size`, se empieza en 0.
     - Si el `id` es de un adjunto terminado con el mismo `sha256`: `attach.ready {offset: size}` y, con el `attach.end`, `attach.done` con el adjunto existente (reintento tras perder la respuesta). Con otro `sha256`: `invalid`.
   - **Tramas (`area.frame`):** con un mutex propio. Solo para subidas aceptadas en esta conexión; las de un id no aceptado o de la conexión anterior se descartan. El `offset` debe ser justo lo que hay escrito, o la trama se descarta. Si pasa del tamaño, se descarta la subida y se marca: el `error` `invalid` se responde en el `attach.end` (las tramas no llevan `rid`). El SHA-256 se calcula de forma incremental con las tramas (en una reanudación, se recalcula una vez lo que ya había).
   - **`attach.end {id}`:** comprueba el tamaño y el SHA-256 (si no, `checksum`). Mueve el fichero a `inbox/<YYYYMMDD-HHMMSS>-<nombre saneado>` **creando el nombre final de forma exclusiva** (`O_EXCL` o `os.Link` + borrar; `os.Rename` sobrescribe en Unix), lo registra en `attachments` (tarea 6) y responde `attach.done {attachment}`.
   - **Saneado del nombre** (función compartida con la tarea 9): el nombre base, sin separadores, sin caracteres de control ni `<>:"|?*` (en todos los sistemas: `a:b` es un flujo alternativo en Windows), sin puntos ni espacios al final, sin nombres reservados de Windows (`CON`, `NUL`, `COM1`…, también con extensión), recortado a 100 caracteres sin romper el UTF-8; vacío, `.` o `..` pasan a `fichero`; con `-2`, `-3`… si ya existe.
   - Las subidas sin avanzar durante una hora se borran (al abrir una subida nueva basta).
3. **Descargas:**
   - **`file.get {id, offset}`** (comprobando `Server.Current(m)`): solo ids del registro (si no, `not_found`); `offset` mayor que el tamaño, `invalid`; una descarga a la vez por id (la nueva sustituye a la anterior).
   - Manda tramas de 256 KiB desde `offset` con `SendFrameTo`, hasta el tamaño registrado, y al final `file.end {id, size}` con `Reply`.
   - **`file.cancel {id}`:** para la descarga en curso. Cada descarga corre en su gorrutina, ligada a la conexión: termina si la app se va.
4. **Captura (`capture.request`, con `rid`):**
   - **Corre en su gorrutina** (el lector de la conexión no se bloquea: el pong del ping tiene 10 s, `server.go:454`), una a la vez: si hay otra en curso, `busy`.
   - Ejecuta `exec-out screencap -p` con el adb real (el `phoneapp.ADB` de `startPhoneLink`), con un tiempo límite de 10 s.
   - Guarda `inbox/<fecha>-captura.png` y responde `capture.ready {attachment}` con `thumb`.
   - Si falla: `error` `failed` con el motivo.
5. **Miniaturas** (función compartida con la tarea 9): JPEG de como mucho 16 KiB, con la imagen reducida a 256 px de ancho con la biblioteca estándar. Solo PNG, JPEG y GIF; **primero `image.DecodeConfig`, y nada de miniatura por encima de 40 Mpx** (un PNG pequeño puede declarar dimensiones enormes).
6. **`inbox/` y `outbox/`** cuelgan del entorno (`ws`), y se crean al usarse.

**Tests:**
- subida completa con hash; reanudación con offset; reintento de una subida terminada (mismo y otro `sha256`); trama fuera de orden, de un id no aceptado y de la conexión anterior; trama que pasa del tamaño (`invalid` en el `attach.end`);
- `checksum`, `too_large` (tamaño y cuota) y `busy`; `invalid` (sha256, size, mime);
- nombre saneado: `..\\x`, `../x`, `CON`, `con.txt`, `a:b`, `x. `, vacío, 300 caracteres con multibyte y nombres repetidos; dos subidas con el mismo nombre en el mismo segundo no se pisan;
- `.partial` que es un enlace simbólico (solo en Unix): no se sigue;
- descarga completa y desde un offset; `offset` de más; `file.cancel`; descarga de un id desconocido;
- captura con un `phoneapp.ADB` en memoria en `phone_files_test.go` (como `appPhone`; `fakeadb` no puede devolver bytes binarios), también con una captura en curso (`busy`);
- miniatura de un PNG que declara 100 000×100 000: no se decodifica;
- `SendFrameTo` en `bridge`.

- [ ] Pasos 1-5. Commit `feat(host): adjuntos, descargas y captura desde el móvil`.

---

### Tarea 9: `adbuddy send`

**Requisitos** (spec M3 §5.2):
1. **`adbuddy send <fichero> [--caption "…"]`:** IPC `KindSend` con `Argv = [ruta absoluta, caption]`. Lo usa el agente: no se rechaza `ADBUDDY_SOURCE=agent`. Caption de como mucho 64 KiB, o error claro antes de llamar.
2. **Manejador propio en `send.go`** (`handleSend`), **no en `handleSession`**, que rechaza al agente (`serve_session.go:112`). Enrutado con una línea nueva en `serve.go:196`, antes de la de `KindAttach`/`KindSession`: `if req.Kind == ipc.KindSend { return link.handleSend(ctx, ws, req, c) }`. Nada en `daemon.go`.
   - `len(Argv) == 2` y caption de como mucho 64 KiB, o error.
   - **Contención en el entorno (`ws`):** la misma lógica que `daemon/pcwrite.go` (`resolveLinks`, `within`, que en Windows compara sin mayúsculas, y `reparseBelow`, porque `EvalSymlinks` no sigue las junctions de Windows). Cópiala o sácala a un paquete propio **sin tocar `daemon.go`**. Después, abre el fichero y comprueba con `os.SameFile` que el abierto es el de la ruta resuelta.
   - Solo ficheros regulares (ni directorios, ni FIFOs, que bloquean `open` y con él el cierre de `serve`), de como mucho 200 MiB.
   - Lo copia con `io.LimitReader` (200 MiB + 1) a `outbox/<YYYYMMDD-HHMMSS>-<nombre saneado>` (saneado de la tarea 8, creación exclusiva), calcula SHA-256 y tamaño, y lo registra en `attachments`. En imágenes, `thumb` como la captura (miniaturas de la tarea 8, con su límite de píxeles).
   - Guarda en el historial y publica `chat.message {role: assistant, text: caption, attachments: [a]}` sin `rid` (`addMessage` de la tarea 6).
   - **Sin app conectada:** copia y registra igual (llegará con el `snapshot`), sin pánico, y lo dice en la respuesta.
   - Mensajes de error claros para el agente, en inglés: fuera del entorno, demasiado grande, no es un fichero, no existe.
3. **`root.go`:** una línea nueva `root.AddCommand(newSendCmd(app))` (no edites la de `newAttachCmd`).
4. **`host/internal/workspace/templates/AGENTS.md`**, sección Commands: una línea para `adbuddy send <file> [--caption "…"]` (enviar al móvil un resultado). Avisa al supervisor: es compartido.

**Tests:** dentro y fuera del entorno; enlace simbólico que sale (solo en Unix); **junction que sale (`mklink /J`, solo en Windows, sin permisos de administrador)**; mayúsculas distintas en la ruta del entorno (Windows); demasiado grande; directorio; FIFO (solo en Unix); `Argv` de otra longitud; caption de más; sin app; y que el `chat.message` llega a la app falsa con el adjunto descargable (`file.get`) y queda en el `snapshot`.

- [ ] Pasos 1-5. Commit `feat(host): adbuddy send, ficheros del agente al móvil`.

---

### Tarea 10: Extremo a extremo

**Requisitos:**
1. **`host/internal/testutil/fakeagent`:** un programa de test que se pone en modo crudo (`x/term.MakeRaw` sobre su stdin; en Windows activa la entrada VT), activa bracketed paste y hace eco de cada línea o pegado (`got:<texto>`): lee lo que llega entre `ESC[200~` y `ESC[201~` hasta el `\r`. Con `send <fichero>` ejecuta `adbuddy send`. Se compila con `testutil.Build`.
2. **`host/cmd/adbuddy/chat_e2e_test.go`:** con un arranque propio en ese fichero (sin tocar `startWith`/`startWithData`, que fuerzan `--no-agent` y son de M6a), `serve` real con `agent = "codex"` en `config.toml`, sin `--no-agent`.
   - **`PATH` del proceso:** solo la carpeta donde `fakeagent` se llama `codex` (`codex.exe` en Windows). En Windows, quita `Path` del entorno sin distinguir mayúsculas.
   - **Antes de arrancar `serve`:** para cada nombre de `agent.Known`, `LookPath` con ese `PATH` da el falso o no encuentra nada; si no, el test falla (si no, `session.new {agent:"claude"}` lanzaría el real).
   - Un ayudante con `rid` en el propio fichero (`phone.send` no lo lleva).
   - Comprueba:
     - la app falsa (la de `phone_e2e_test.go`) manda `chat.send` y recibe el `chat.message`;
     - con `terminal.watch` ve `got:<texto>`;
     - `adbuddy attach` por IPC (`ipc.Attach`) ve la misma pantalla;
     - el agente falso hace `adbuddy send` y la app recibe el `chat.message` y descarga el fichero con `file.get`;
     - `adbuddy stop` y `adbuddy new` cambian `session.state`.

- [ ] Pasos 1-5. Commit `test(host): chat de extremo a extremo con un agente falso`.

---

## Validaciones manuales pendientes (al final de M3, con M3b)

En el Pixel del usuario:
- `adbuddy serve` lanza Claude Code;
- `adbuddy attach` y Ctrl+]; dos terminales a la vez;
- chat desde el móvil;
- vista de terminal con teclas rápidas;
- foto y fichero de ida y vuelta, y captura;
- `adbuddy send` desde Claude Code;
- reconexión.

En Linux y macOS (la CI corre los tests de `internal/pty`; esto es lo que no cubre):
- `/exit` del agente llega a `stopped` aunque un nieto con `setsid` mantenga abierto el esclavo (si cerrar el maestro no despierta `Read`, `pump` no acaba y `Stop` espera sin límite).
