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
| 3-5 | df3ac6d | Ronda de arreglos de la revisión opus: `Start`/`Stop` en serie con generación; el agente no controla su sesión y solo se aceptan nombres de `agent.IsKnown`; aviso en attach cuando el agente sale; modos de terminal en `Screen.Modes()` y `Snapshot()`; `Busy` también con los permisos de Claude Code; `Stop` amable (`PTY.Hangup`, 5 s o 2 s en Windows, después `Kill`) |

Este plan describe las tareas 6-10: requisitos, interfaces y tests. El código lo escribe el implementador con TDD. **Si un requisito no se puede cumplir tal cual, detente y explícalo.**

## Restricciones globales

- **Rama `m3`.** Integración en `main` solo con el ok del supervisor (adbuddy-e5); nunca `push --force`.
- **Go:** `go` y `gofmt` en el PATH. Los tests, siempre con `-timeout` y `-count=1`, **por paquete** (`go test ./internal/cli/ ./internal/bridge/`…); la suite completa solo al final de la tarea.
- **Antes de cada commit:** `gofmt -l` vacío, y `go vet ./...` con `GOOS=windows`, `linux` y `darwin` (el PTY tiene código por sistema).
- **Commits:** identidad local (hybriqtech), trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`, `git add` con rutas explícitas.
- **Protocolo:** no cambies tipos, campos ni fixtures (`protocol*.go`, `Protocol*.kt` y `protocol/fixtures`). Úsalo como está (spec del protocolo §3-§9).
- **Nunca un agente real en los tests:** en `internal/cli`, `TestMain` sustituye `lookAgent`; el e2e usa `--no-agent` o un agente falso compilado con `testutil.Build`.
- **Ficheros compartidos con otras sesiones:** `host/internal/workspace/templates/AGENTS.md`, `root.go` y `serve.go`. Añade líneas propias, no edites las de otros.
- **El móvil del usuario no se toca.** **Sin subagentes** dentro de una tarea.

## Orden y revisores

| Tarea | Ficheros | Depende de | Revisor |
|---|---|---|---|
| 6 · Sesión y chat en el móvil | `host/internal/cli/phone_chat.go`, `phone_chat_test.go`, y `phone.go` solo si hace falta | 1-5 | `adbuddy-reviewer-opus` (entrada del móvil al PTY) |
| 7 · Terminal hacia el móvil | `host/internal/cli/phone_term.go` (nuevo) y su test; `bridge/server.go` (`Backlog`) | 6 | `adbuddy-reviewer` |
| 8 · Ficheros y captura | `host/internal/cli/phone_files.go` (nuevo) y su test; `bridge/server.go` (`SendFrameTo`) | 6 | `adbuddy-reviewer-opus` (canal y disco) |
| 9 · `adbuddy send` | `host/internal/cli/send.go` (nuevo), `serve_session.go`, `root.go` (una línea), `AGENTS.md` (una línea) | 8 | `adbuddy-reviewer` |
| 10 · Extremo a extremo | `host/internal/testutil/fakeagent` (nuevo), `host/cmd/adbuddy/chat_e2e_test.go` (nuevo) | 6-9 | `adbuddy-reviewer` |

Las tareas son secuenciales: todas tocan `internal/cli`.

---

### Tarea 6: Sesión y chat en el móvil

**Contexto:** `phoneLink` ya tiene `manager *session.Manager` (`useSession`) y publica `session.state` con `sessionInfo`. `chatArea()` (en `phone_chat.go`) devuelve hoy `area{}`. Las áreas se describen en `phone_areas.go`.

**Ojo con la API de la sesión (ronda df3ac6d):**
- `Subscribe` entrega también dos `Output` sin datos por proceso: uno al arrancar (época nueva) y otro con `End` al acabar.
- El nombre que llega en `session.new {agent}` se valida con `agent.IsKnown`.

**Requisitos** (spec M3 §5.1 y §5.3; spec del protocolo §5.2 y §9):
1. **`chatArea()`:** registra los manejadores de `chat.send`, `session.new`, `session.stop` y `agent.interrupt`, y aporta su parte del `snapshot` (`chat`).
2. **`chat.send` (lleva `rid`):**
   - Responde con `Reply`/`error` si el payload no es válido (`invalid`). Lo es si cumple todo esto:
     - `id` cumple `bridge.ValidRID`;
     - `text` tiene como mucho 64 KiB;
     - lleva como mucho 10 adjuntos; si son más, `too_large`.
   - Si el `id` ya está en el historial, vuelve a confirmar el mensaje existente con `PublishRID` y no lo inyecta.
   - Sin agente: `error` `no_agent`. Si `manager.Busy()` dice algo: `error` `busy` con ese texto.
   - Cada adjunto debe existir en el registro de adjuntos (tarea 8). Hasta entonces, cualquier id da `not_found`. Expón un `attachments` (mapa protegido por mutex con id → `bridge.Attachment` y la ruta relativa en `inbox/` o `outbox/`) que la tarea 8 rellenará.
   - Texto inyectado: `text`, y si hay adjuntos, `"\n\nAdjuntos:\n- inbox/<nombre>"` por cada uno, con `manager.Paste`.
   - Después: guarda y publica `chat.message {id, role: user, text, attachments, conversation}` con `PublishRID` y el `rid` del `chat.send`.
3. **Historial:** los últimos 100 `ChatMessage` de la conversación, en memoria y protegidos por el mutex del área.
   - Un `chat.message` con un `id` existente lo sustituye (upsert).
   - `chatSnapshot` copia el historial con `Attachments` nunca `nil` y sin `thumb` (spec M3 §4.3; pendiente de M3.0).
   - El historial se vacía cuando cambia `Conversation` (se lanza otro proceso del agente).
4. **`session.new {agent?}` (`rid`):** si trae `agent` y no cumple `agent.IsKnown`, `error` `invalid`. Si no, `manager.Start(agent o el actual o el configurado, false)`. Si va bien, `PublishRID(session.state)` con el estado nuevo; si no, `error` `failed` con el texto de `agent.InstallHint`. **`session.stop` (`rid`):** `manager.Stop()` y `PublishRID(session.state)`.
5. **`agent.interrupt` (sin `rid`):** `manager.Input([]byte{0x1b})`. Sin agente no hace nada.
6. **Todo manejador con `rid` responde por todos sus caminos** (spec del protocolo §3).

**Tests** (`phone_chat_test.go`, con el `linkedApp` de `phone_areas_test.go` y una sesión con agente falso):
- usa un `session.Manager` con `Start` sustituido por un PTY en memoria, como `fakePTY` en `internal/session/session_test.go` (cópialo o muévelo a `internal/testutil` si lo compartes);
- `chat.send` llega al PTY como pegado y vuelve como `chat.message` con el `rid`; un `id` repetido no se inyecta dos veces;
- `invalid` (rid o id malos, texto de más de 64 KiB), `too_large` (11 adjuntos), `not_found` (adjunto desconocido), `no_agent` y `busy`;
- `session.new` y `session.stop` responden con un `session.state` con su `rid`; `agent.interrupt` escribe ESC;
- el `snapshot` lleva el historial (como mucho 100) sin `thumb`.

- [ ] Paso 1: tests que fallan. - [ ] Paso 2: verlos fallar. - [ ] Paso 3: implementar. - [ ] Paso 4: `go test ./internal/cli/` y la suite completa. - [ ] Paso 5: commit `feat(host): chat y sesión desde el móvil`.

---

### Tarea 7: Terminal hacia el móvil

**Requisitos** (spec M3 §4.1 y §4.3; spec del protocolo §7):
1. **`bridge.Server.Backlog() int`:** cuántos mensajes esperan en la cola de la app conectada (0 sin app). Test en `send_test.go`.
2. **`terminal.watch {epoch, offset}`:** suscribe la conexión que lo pidió y guarda el `Message` para `SendTo`.
   - Si `screen.Since(epoch, offset)` da bytes, se mandan como `terminal.output` desde ese `offset`.
   - Si no, se manda `terminal.snapshot` (`screen.Snapshot()`).
   - Sin agente, nada.
3. **Salida en vivo:** una sola suscripción (`manager.Subscribe`) para el área. Los `Output` sin datos (inicio de proceso) y los de `End` no generan `terminal.output`. El de inicio provoca el `snapshot` del proceso nuevo. `terminal.snapshot` usa `Screen.Snapshot()`, que ya incluye los modos. La salida se acumula y se manda cada 50 ms en trozos de como mucho 64 KiB.
   - Si `Backlog()` pasa de 128, se marca la pantalla como pendiente y se descarta lo acumulado. Cuando baja, se manda un `terminal.snapshot`.
   - Un cambio de `epoch` manda el `snapshot` del proceso nuevo.
   - Si cambia el tamaño de la pantalla, se manda `terminal.size {epoch, offset, cols, rows}`.
4. **`terminal.unwatch`**, una conexión nueva (`area.attach`) o que la app se vaya (`area.gone`) terminan la suscripción. Usa `Server.Current(m)` para no aceptar un `watch` de una conexión reemplazada.
5. **`terminal.input {data}`:** como mucho 64 KiB, o se descarta; va a `manager.Input`. **`terminal.resize {cols, rows}`:** 1..1000, o se descarta; va a `manager.ResizePhone`.
6. **La terminal nunca provoca que se expulse a la app:** `SendTo` solo si `Backlog()` está por debajo del umbral.

**Tests:**
- watch desde un offset en la cola (llegan solo los bytes nuevos) y desde uno viejo (llega un snapshot);
- salida en vivo agrupada;
- cambio de epoch y de tamaño;
- unwatch, y conexión nueva sin watch: no recibe nada;
- input y resize con sus límites;
- presión: con `Backlog` alto, nada de output y luego un snapshot (inyecta `Backlog` con una variable de test o un umbral configurable).

- [ ] Pasos 1-5 como en la tarea 6. Commit `feat(host): la terminal del agente en el móvil`.

---

### Tarea 8: Ficheros y captura

**Requisitos** (spec M3 §6; spec del protocolo §6):
1. **`bridge.Server.SendFrameTo(ctx, req Message, frame []byte) error`:** encola una trama binaria para la conexión de `req` (como `SendTo`).
   - Si `Backlog()` pasa de la mitad de la cola, **espera** a que haya hueco (o a `ctx`) en vez de expulsar a la app.
   - Las tramas y los mensajes salen en orden por el mismo escritor. La cola de `client` pasa a llevar elementos que son mensaje o trama.
   - Es concurrencia del canal: test de orden (tramas y luego `file.end` por `Reply` llegan en ese orden) y de espera con la cola llena.
2. **Subidas:**
   - **`attach.begin {id, name, mime, size, sha256}`:**
     - `id` con `bridge.ParseFrameID`; tamaño de hasta 200 MiB; como mucho 4 subidas en curso por conexión (`busy`); `inbox/` con menos de 2 GiB (`too_large`).
     - Responde `attach.ready {id, offset}`, donde `offset` es lo que ya hay en `inbox/.partial/<id>` si el `sha256` coincide con el de la subida anterior (guárdalo en `inbox/.partial/<id>.json`), o 0.
     - Si el `id` es de un adjunto terminado con otro `sha256`: `invalid`.
   - **Tramas (`area.frame`):** con un mutex propio. Solo para subidas aceptadas en esta conexión. El `offset` debe ser justo lo que hay escrito, o la trama se descarta. Si pasa del tamaño, se descarta la subida y se responde `error` `invalid`.
   - **`attach.end {id}`:** comprueba el tamaño y el SHA-256 (si no, `checksum`). Mueve el fichero a `inbox/<YYYYMMDD-HHMMSS>-<nombre saneado>`, lo registra en `attachments` (tarea 6) y responde `attach.done {attachment}`.
   - **Saneado del nombre:** el nombre base, sin separadores ni caracteres de control, sin nombres reservados de Windows, como mucho 100 caracteres, con `-2`, `-3`… si ya existe.
   - Las subidas sin avanzar durante una hora se borran (al abrir una subida nueva basta).
3. **Descargas:**
   - **`file.get {id, offset}`:** solo ids del registro.
   - Manda tramas de 256 KiB desde `offset` con `SendFrameTo` y, al final, `file.end {id, size}` con `Reply`.
   - **`file.cancel {id}`:** para la descarga en curso. Cada descarga corre en su gorrutina, ligada a la conexión: termina si la app se va.
4. **Captura (`capture.request`, con `rid`):**
   - Ejecuta `exec-out screencap -p` con el adb real (el `phoneapp.ADB` de `startPhoneLink`), con un tiempo límite de 10 s.
   - Guarda `inbox/<fecha>-captura.png` y responde `capture.ready {attachment}` con `thumb`: JPEG de como mucho 16 KiB, con la imagen reducida a 256 px de ancho con la biblioteca estándar.
   - Si falla: `error` `failed` con el motivo.
5. **`inbox/` y `outbox/`** cuelgan del entorno (`ws`), y se crean al usarse.

**Tests:**
- subida completa con hash; reanudación con offset; trama fuera de orden;
- `checksum`, `too_large` y `busy`; nombre saneado (incluidos `..\\x`, `CON` y nombres repetidos);
- descarga completa y desde un offset; `file.cancel`; descarga de un id desconocido;
- captura con el adb falso (`testutil/fakeadb` con una regla que devuelva un PNG pequeño);
- `SendFrameTo` en `bridge`.

- [ ] Pasos 1-5. Commit `feat(host): adjuntos, descargas y captura desde el móvil`.

---

### Tarea 9: `adbuddy send`

**Requisitos** (spec M3 §5.2):
1. **`adbuddy send <fichero> [--caption "…"]`:** IPC `KindSend` con `Argv = [ruta absoluta, caption]`. Lo usa el agente: no se rechaza `ADBUDDY_SOURCE=agent`.
2. **En `serve` (`handleSession` o un manejador propio):**
   - El fichero tiene que estar dentro del entorno (`ws`), comparando rutas tras `EvalSymlinks`. Ni directorios ni más de 200 MiB.
   - Lo copia a `outbox/<YYYYMMDD-HHMMSS>-<nombre saneado>`, calcula SHA-256 y tamaño, y lo registra en `attachments`. En imágenes, `thumb` como la captura.
   - Publica `chat.message {role: assistant, text: caption, attachments: [a]}` sin `rid`.
   - Mensajes de error claros para el agente, en inglés: fuera del entorno, demasiado grande, no existe.
3. **`root.go`:** `newSendCmd(app)` en la línea de `newAttachCmd`.
4. **`host/internal/workspace/templates/AGENTS.md`**, sección Commands: una línea para `adbuddy send <file> [--caption "…"]` (enviar al móvil un resultado). Avisa al supervisor: es compartido.

**Tests:** dentro y fuera del entorno (enlace simbólico que sale, solo en Unix), demasiado grande, directorio, y que el `chat.message` llega a la app falsa con el adjunto descargable (`file.get`).

- [ ] Pasos 1-5. Commit `feat(host): adbuddy send, ficheros del agente al móvil`.

---

### Tarea 10: Extremo a extremo

**Requisitos:**
1. **`host/internal/testutil/fakeagent`:** un programa de test que activa bracketed paste, hace eco de cada línea o pegado (`got:<texto>`), y con `send <fichero>` ejecuta `adbuddy send`. Se compila con `testutil.Build`.
2. **`host/cmd/adbuddy/chat_e2e_test.go`:** `serve` real con `agent = "codex"` en `config.toml` y el `PATH` del proceso apuntando a una carpeta donde `fakeagent` se llama `codex` (`codex.exe` en Windows). Sin `--no-agent`. Comprueba:
   - la app falsa (la de `phone_e2e_test.go`) manda `chat.send` y recibe el `chat.message`;
   - con `terminal.watch` ve `got:<texto>`;
   - `adbuddy attach` por IPC (`ipc.Attach`) ve la misma pantalla;
   - `adbuddy stop` y `adbuddy new` cambian `session.state`.
3. Nunca un agente real: si `codex` no es el falso, el test falla antes de lanzar `serve`.

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
