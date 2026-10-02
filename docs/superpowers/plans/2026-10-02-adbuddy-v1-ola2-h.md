# v1 · ola 2 H: diario, sesión y doctor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cerrar tres diferidos de seguridad que la ola 1 dejó: el diario que sigue enlaces y se cuelga con una FIFO (journal), que el agente pueda teclear en su propia terminal declarándose "no agente" (V07) y que `doctor` desde la sesión del agente use el adb real aunque haya pausa (V20).

**Architecture:** Tres tareas en serie. T1 endurece `journal` con el mismo criterio que la ola 1 (`os.Root` para la carpeta, apertura sin bloqueo y solo ficheros regulares, lecturas acotadas) sacando el helper de lectura a un paquete compartido. T2 hace que `doctor` respete la pausa cuando corre en la sesión del agente. T3 identifica en el SERVIDOR (por el PID del otro extremo del socket) si una petición viene del árbol de procesos del agente, en vez de fiarse de `ADBUDDY_SOURCE`, para `attach`, `session new/stop` y `remote`.

**Tech Stack:** Go 1.27 (host), `golang.org/x/sys` (ya en `go.mod`).

**Spec:** `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md` (§4.4 diario/undo, §4.5 confirmaciones, §5 IPC); triage `.superpowers/sdd/2026-10-02-v1-diferidos/triage.md` (filas V07 y V20, y el diferido "Diario" de `docs/superpowers/plans/pendientes-m3-m5.md`, sección "v1 · Diferidos tras la ola 1").

## Global Constraints

- **Cada asunto se reproduce antes de arreglarse** en el `origin/main` actual: se escribe un test que falla sin el arreglo (RED con comando y salida); si ya no existe, se dice en el informe y no se inventa trabajo.
- **Nada que necesite móvil, emulador, claves o red.** Los tests usan el adb falso. En este PC Windows no corre `go test -race` (solo la CI): leer los globales de test antes de lanzar goroutines y no mutar globales con goroutines vivas. Los tests `//go:build unix` no corren aquí: que compilen (`GOOS=linux go vet ./...`, `GOOS=darwin go vet ./...`).
- **Textos de cara al usuario (M6c).** Hay catálogo en/es (`host/internal/i18n/catalog/{en,es}.toml`, motivos del clasificador en `classify/reasons_es.go`) y controles (`TestKeysExist`, `TestReasonsBilingual`, `cli/textlint_test.go`: un contador de literales que solo puede bajar). En `internal/cli`: errores con `fmt.Errorf` o con claves del catálogo, ningún literal nuevo de `NewError/ConfigError/TransientError/UsageError/Fprint*/say/warn/Short/Long/Detail/Guidance`; reutilizar mensajes existentes; todo texto nuevo, en/es en el catálogo. Los `Exit.Error` de IPC se quedan en inglés (como deja M6c).
- **Archivos de otras sesiones (no tocar, o tocar lo mínimo y decirlo):** tern (`serve.go`, `setup.go`, `run.go`, `phone.go`, `helpers.go`, `guard.go`, `session/session.go`, `phone_chat.go`, `phone_term.go`, `rule.go`, `phone_agent.go`) y nautilus (`pcwrite.go`, `send.go`, `phone_files.go`, el `ipc.Lock` de `serve`). Antes de empezar una tarea, `git fetch` y comprobar `git diff origin/main --stat` de los archivos que toca; si `main` ya los cambió, trabajar sobre el estado actual.
- **Seguridad:** un arreglo no puede abrir un hueco que deje ejecutar 🔴 sin confirmación, escribir fuera del entorno, abrir el socket IPC a otros usuarios o saltarse el shim; ante la duda, falla cerrado y se dice en el informe.
- **Proceso del proyecto (`CLAUDE.md`):** cada tarea compila y pasa sus tests sola; commits con identidad local `hybriqtech` y el trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`; nunca `--no-verify`; `git add` con rutas concretas; nunca modelos Haiku; sin IPs privadas ni rutas personales en lo que se sube; ficheros de `host/` en LF; no tocar `docs/superpowers/plans/pendientes-m3-m5.md` dentro de las tareas (se reconcilia al final).
- Comandos desde `host/`: `go test ./... -count=1 -timeout 10m`, `gofmt -l .` (vacío), `go vet ./...`.

## Reparto de revisiones

**Opus** (journal, ipc, daemon, confirmaciones): T1, T2, T3. Implementadores: T1 y T2 Sonnet; T3 opus (criterio y plataformas). Revisión final de la rama: opus. Las tareas van en serie, un subagente a la vez.

## Fuera de esta ola

V08, V18, V31, V34-V36, V40 (tern); V09, V19, V22 y el `ipc.Lock` de serve (nautilus); V27 (decisión de producto); `journal` firma con `ipc.secret` (límite del modelo, ya documentado); que `source` del diario salga del PID del par en vez de `ADBUDDY_SOURCE` (se apunta en T3 como mejora posterior, no se hace).

---

### Task 1: El diario no sigue enlaces ni se cuelga con una FIFO (journal)

**Files:**
- Create: `host/internal/safeio/` (paquete compartido: `OpenRegular`, `ReadRegular`, con `*_unix.go` / `*_other.go`).
- Modify: `host/internal/journal/journal.go` (`openAppend`, `Load`, backups `BackupDir`), `host/internal/workspace/readregular*.go` (pasa a usar `safeio`), `host/internal/precheck/` y `host/internal/daemon/` solo si escriben/leen `journal/backups/<id>/` por ruta (comprobarlo con `grep`).
- Test: `host/internal/journal/*_test.go`, `host/internal/safeio/*_test.go`, y el de workspace que ya cubre `openRegular` (que siga pasando).

**Interfaces:**
- Produces: `safeio.OpenRegular(path string) (*os.File, os.FileInfo, error)` (Lstat previo, rechaza lo que no sea fichero regular, abre con `O_NONBLOCK` en Unix, `f.Stat()` + `os.SameFile`), `safeio.ReadRegular(path string, max int64) ([]byte, error)` (con `io.LimitReader`, error si excede). El de `workspace` ya existe (T5a de la ola 1): se MUEVE, no se duplica; `workspace` llama a `safeio`.
- Consumes: nada de otras tareas.

**Qué hacer (el diseño está decidido; el implementador comprueba cada punto contra el código):**
- **`Load` (`journal.go:~197`)** abre cada `*.jsonl` con `os.Open` por ruta: una FIFO plantada en `journal/2026-10.jsonl` cuelga `adbuddy journal`, `undo` y `doctor`; un enlace se sigue. Debe abrir con `safeio.OpenRegular` (un fichero que no es regular o es un enlace se SALTA y se cuenta como `skipped`, sin colgarse, y se avisa por el camino que ya usa `skipped`; nunca se lee). Mantener el tope de línea de 16 MiB.
- **`openAppend` (`journal.go:~137-140`)** hace `MkdirAll` + `OpenFile(O_APPEND|O_CREATE)` siguiendo enlaces durante toda la sesión: si `journal` o el `.jsonl` del mes pasan a ser enlaces/junctions después de `Init`, el diario se escribe fuera del entorno. Abrir la carpeta con `os.OpenRoot(s.dir)` (rechaza junctions en Windows y enlaces que salen) y el fichero con `root.OpenFile(name, O_RDWR|O_APPEND|O_CREATE|nonblock, 0o600)`; comprobar con `f.Stat()` que es regular. Si la propia carpeta `journal` es un enlace legítimo del usuario (movida a otro disco): decidir como en T9a de la ola 1 (la carpeta del entorno lo rechaza `ensureRealDirs`; el diario hereda esa decisión: se rechaza y el error se explica) y dejarlo escrito en el informe. Falla cerrado: sin diario no se ejecuta nada (ya es así: `Writable`).
- **Backups (`BackupDir`)**: mirar con `grep` quién escribe y lee `journal/backups/<id>/` (precheck/daemon/undo) y aplicarles el mismo criterio si es un cambio pequeño; si no, apuntarlo como residuo.
- **`safeio`**: mover el helper de T5a; los tests de workspace (FIFO `//go:build unix`, `/dev/zero`, enlaces, 1 MiB) pasan sin cambios o se mueven con él.
- **Tests (RED primero):** Unix: `syscall.Mkfifo` en `journal/2026-10.jsonl` ⇒ `Load` vuelve (con tope de tiempo) y la cuenta como saltada, `undo`/`doctor` no se cuelgan; Windows y Unix: junction/enlace en `journal/` o en el `.jsonl` del mes (con `mklink /J` o `os.Symlink`, se saltan si no se puede crear) ⇒ `Append` falla y NO escribe fuera; fichero normal sigue funcionando; limpieza de junctions con `os.Remove(enlace)`, nunca `RemoveAll` que siga el enlace.
- **Informe:** una fila por punto (arreglado / ya no vigente / aplazado con motivo), tests, comandos y salidas, textos nuevos (si algún error llega al usuario, su clave en el catálogo).

- [ ] Reproducir y escribir los tests RED.
- [ ] Crear `safeio`, mover el helper de workspace, adaptar `journal`.
- [ ] Comprobar: `go test ./internal/journal/... ./internal/safeio/... ./internal/workspace/... ./internal/daemon/... -count=1 -timeout 10m`, luego `go test ./... -count=1 -timeout 10m`, `gofmt -l .`, `go vet ./...`, `GOOS=linux go vet ./...`, `GOOS=darwin go vet ./...`.
- [ ] Commit (uno por asunto: `safeio`, `Load`, `openAppend`).

### Task 2: `doctor` desde la sesión del agente respeta la pausa (V20)

**Files:**
- Modify: `host/internal/cli/doctor.go` (la rama `agent := app.Getenv("ADBUDDY_SOURCE") == "agent"`, ~162, y las lecturas con el adb real).
- Test: `host/internal/cli/doctor*_test.go`.

**Interfaces:**
- Consumes: el estado de pausa que el daemon ya da en `KindPing` (comprobar cómo lo expone `ipc.Call` con `KindPing`; si no incluye la pausa, el cambio mínimo es añadir el campo a la respuesta de ping, sin cambiar el protocolo de otras peticiones; decirlo).
- Produces: nada que use T3.

**Qué hacer:**
- Hoy `doctor` desde la sesión del agente corre sus lecturas fijas con el adb REAL (fuera del shim) aunque haya pausa o pregunta pendiente, y no deja rastro. Decisión (ruling del supervisor: "registrar en el diario o bloquear en pausa"): **bloquear en pausa**: si `doctor` corre con `ADBUDDY_SOURCE=agent` y el daemon dice que está en pausa (o no se puede saber con el host arriba), no ejecuta las lecturas de adb y lo dice con un mensaje que YA exista en el catálogo (el de la pausa, buscar `paused` en `catalog/en.toml`); sin pausa, sigue como hoy. Registrar en el diario queda fuera (las lecturas son fijas y de solo lectura): se anota en el informe y en `pendientes`.
- Si `doctor` no puede hablar con el host, el comportamiento actual no cambia (ya avisa de que el host no corre).
- Tests (RED): con el host falso en pausa y `ADBUDDY_SOURCE=agent`, `doctor` no invoca el adb falso (contar llamadas); sin pausa invoca; desde una terminal humana (sin `ADBUDDY_SOURCE`) no cambia nada.

- [ ] Reproducir con el adb falso y escribir el test RED.
- [ ] Arreglar (cambio mínimo; sin literales nuevos en `internal/cli`).
- [ ] Comprobar: `go test ./internal/cli/... -count=1 -timeout 10m`, `go test ./... -count=1 -timeout 10m`, `gofmt -l .`, `go vet ./...`.
- [ ] Commit.

### Task 3: El servidor sabe si la petición viene del agente (V07)

**Files:**
- Create: `host/internal/ipc/peer_*.go` (PID del otro extremo del socket unix: Linux `SO_PEERCRED`, macOS `LOCAL_PEERPID`, Windows `SIO_AF_UNIX_GETPEERPID`), y el modo de pertenencia al árbol del agente: Windows por el Job object de `pty` (V25: `IsProcessInJob`), Unix recorriendo la cadena de padres (`/proc/<pid>/stat` en Linux, `unix.SysctlKinfoProc` en macOS).
- Modify: `host/internal/ipc/server.go` (adjuntar `Peer` a la petición/conexión), `host/internal/cli/serve_session.go` (`handleSessionIn`, ~119), `host/internal/cli/remotecmd.go` (~280), `host/internal/pty/` (exponer "este PID está en el árbol" del hijo), y lo mínimo de `host/internal/session/` (un método que dé el proceso raíz del agente; tern toca `session.go`: cambio mínimo y avisado).
- Test: `host/internal/ipc/*_test.go`, `host/internal/cli/serve_session_test.go`, `host/internal/pty/*_test.go`.

**Interfaces:**
- Produces: `ipc.Conn.PeerPID() (pid int, ok bool)`; `session.Manager.InAgentTree(pid int) (inTree, known bool)`; el criterio de `handleSessionIn` y del manejador de `remote`: **si el servidor sabe que el par está en el árbol del agente → `AGENT_SESSION`; si lo sabe y NO está → permitido aunque `ADBUDDY_SOURCE` diga "agent" (la declaración ya no manda hacia arriba) o no; si no puede saberlo (plataforma sin PID del par, PID que ya no existe, error) → vuelve al comportamiento actual (la declaración del cliente) y se anota el residuo.**

**Qué hacer (diseño decidido; el implementador lo comprueba contra el código y las plataformas):**
- **Problema (V07):** `handleSessionIn` (`serve_session.go:119`) y la rama de `remote` (`remotecmd.go:280`) solo miran `req.Env["ADBUDDY_SOURCE"] == "agent"`, que declara el cliente: un agente que vacía la variable llega a `attach` (teclear en su propia terminal = contestar sus propios permisos), `session new` y `session stop`. Alcance real: solo lo que llega por IPC (`attach`, `session`, `remote`); `doctor --fix`/`connect` no pasan por el servidor (corren en el proceso del agente con su adb; el rodeo con el adb real es un no-garantizado documentado) y quedan fuera.
- **Mecanismo:** el servidor mira el PID del otro extremo del socket (el agente corre como el mismo usuario, así que las credenciales de usuario no sirven; sí el árbol de procesos). El árbol del agente se conoce porque ADBuddy lo lanzó: en Windows el Job object de V25 contiene a todos sus descendientes (mejor que la cadena de padres, que se rompe al morir el padre); en Unix se recorre la cadena de padres del PID del par hasta el PID raíz del agente (limitación: un proceso que se desvincula con `setsid`/doble fork de su padre escapa; se documenta como residuo, y la cadena de padres con huérfanos reparentados a init no se detecta).
- **Falla cerrado cuando se sabe, abierto-al-comportamiento-actual cuando no se puede saber**: no se rompe `attach` en una plataforma sin PID del par (Windows sin `SIO_AF_UNIX_GETPEERPID`: comprobar el nivel mínimo de Windows y si `golang.org/x/sys/windows` lo ofrece; si no, `ok=false` y se usa la declaración).
- **Tests (RED):** conexión real por el socket unix desde un proceso hijo del agente falso (proceso lanzado dentro del árbol del PTY falso/Job) con `ADBUDDY_SOURCE` vacío ⇒ `AGENT_SESSION` (Windows y Unix); desde un proceso fuera del árbol con `ADBUDDY_SOURCE=agent` declarado ⇒ se decide por el árbol (comportamiento documentado: permitido) — decidir con el supervisor si "declarado agente pero fuera del árbol" debe seguir bloqueando por prudencia: **ruling del plan: sigue bloqueando** (la declaración solo puede AÑADIR bloqueo, nunca quitarlo; se evita así una regresión del comportamiento actual) — y el test lo fija; PID desconocido ⇒ solo manda la declaración. Limpiar procesos de prueba siempre (defer + kill del árbol); sin Sleeps frágiles; globales leídos antes de lanzar goroutines (`-race` solo en CI).
- **No hacer:** que `source` del diario salga del PID del par (mejora posterior), ni tocar el protocolo (`Request`) más allá de lo mínimo para llevar el PID al manejador (campo del lado del servidor, no del cliente: el cliente NO lo declara).

- [ ] Reproducir (el agente falso conecta con `ADBUDDY_SOURCE` vacío y llega a `attach`) y escribir los tests RED.
- [ ] Implementar `PeerPID` por plataforma, el árbol del agente (`pty`/`session`) y el criterio en los dos manejadores.
- [ ] Comprobar: `go test ./internal/ipc/... ./internal/pty/... ./internal/session/... ./internal/cli/... -count=1 -timeout 10m`, `go test ./... -count=1 -timeout 10m`, `gofmt -l .`, `go vet ./...`, `GOOS=linux go vet ./...`, `GOOS=darwin go vet ./...`.
- [ ] Informe: tabla por plataforma (qué se comprobó aquí, qué solo compila), residuos (setsid/doble fork en Unix, plataformas sin PID del par), textos nuevos (probablemente ninguno: `AGENT_SESSION` ya existe).
- [ ] Commit (uno por asunto: `PeerPID`, árbol del agente, criterio).

---

## Pasos de cada tarea (se repiten; no se copian en cada una)

- [ ] **Reproducir** y escribir el test que falla sin el arreglo (RED con comando y salida).
- [ ] **Arreglar** con el cambio mínimo; sin refactors fuera del asunto.
- [ ] **Comprobar:** los tests del paquete y `go test ./... -count=1 -timeout 10m`, `gofmt -l .` (vacío), `go vet ./...` (más `GOOS=linux`/`darwin` si hay código de plataforma).
- [ ] **Informe** en el workspace (`.superpowers/sdd/2026-10-02-adbuddy-v1-ola2-h/task-N-report.md`): una fila por asunto, test que lo guarda, comandos y salidas, textos nuevos, residuos.
- [ ] **Commit** con la identidad hybriqtech y el trailer exacto.

## Validaciones manuales pendientes

Nada de esto se prueba sin PC real o permiso: `SIO_AF_UNIX_GETPEERPID` en una versión concreta de Windows 10/11 (si el PID del par no está disponible, T3 vuelve al comportamiento actual); un agente real que intente `attach` tras vaciar `ADBUDDY_SOURCE`; `journal` movido por el usuario a otro disco con una junction.
