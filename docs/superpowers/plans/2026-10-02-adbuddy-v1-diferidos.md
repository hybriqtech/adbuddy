# v1 · cierre de diferidos de código (ola 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cerrar, antes de la v1, los asuntos de código aplazados por las revisiones de M1-M6d que no necesitan móvil, claves ni emulador y no solapan con M6c (tern: cadenas e i18n) ni con M5c (nautilus: notificaciones y pantallas de la app). Esta es la **ola 1**; la ola 2 (clasificador, undo/journal, tarjetas de permiso, lector del bridge) va después de integrar M6c, porque cualquier motivo o mensaje nuevo choca con su catálogo.

**Architecture:** Siete tareas independientes, cada una con su test que falla primero, su commit y su revisión. Se parte del triage `.superpowers/sdd/2026-10-02-v1-diferidos/triage.md` (local; las filas de cada tarea se copian abajo), con cada fila comprobada contra el código de `main` en el momento de hacerla.

**Tech Stack:** Go 1.27 (host), Kotlin (solo T7, tests JVM), shell (T2).

## Global Constraints

- **Cada asunto se reproduce antes de arreglarse.** El implementador comprueba que el fallo sigue vigente en el `main` actual (el triage pudo quedarse atrás) y escribe un test que falla sin el arreglo; si el asunto ya no existe, lo dice en el informe y no inventa trabajo.
- **Nada que necesite móvil, emulador o claves; nada de red.** Los tests usan el adb falso. En este PC Windows no corre `go test -race` (solo la CI): leer los globales de test antes de lanzar goroutines.
- **No tocar cadenas de cara al usuario** (mensajes, motivos de `classify`, `strings.xml`) salvo que el asunto sea justo ese texto; M6c los está moviendo a un catálogo. No tocar `android/**/ui/**`, notificaciones ni `ProtectedApps.kt`/`UiExecutor.kt`/`UiHelpers.kt` (M5c).
- **Seguridad:** un arreglo no puede abrir un hueco que dejara ejecutar 🔴 sin confirmación, escribir fuera del entorno o saltarse el shim; ante la duda, falla cerrado y se dice en el informe.
- **Proceso del proyecto (`CLAUDE.md`):** cada tarea compila y pasa sus tests sola; commits con identidad local `hybriqtech` y el trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`; nunca `--no-verify`; `git add` con rutas concretas; nunca modelos Haiku; sin IPs privadas reales en lo que se sube (usar `192.0.2.x`/`100.64.0.x` o construirlas con una concatenación en los tests); ficheros de `host/` en LF.
- Comandos desde `host/`: `go test ./... -count=1 -timeout 10m`, `gofmt -l .` (vacío), `go vet ./...`.

## Reparto de revisiones

| Tarea | Revisor |
|---|---|
| T1 (V24: validación de ruta) y T4 (IPC) | `adbuddy-reviewer-opus` |
| T2, T3, T5, T6, T7 | `adbuddy-reviewer` (Sonnet) |
| Revisión final de la rama | `adbuddy-reviewer-opus` |

Revisión opus del plan antes de ejecutarlo (obligatoria, `CLAUDE.md`). Las tareas van en serie (un subagente a la vez).

## Fuera de la ola 1 (se dice al supervisor)

V27 (lista de variables de entorno permitidas para el agente: cambio de comportamiento con riesgo de romper agentes reales —`AWS_*`, `NODE_*`, claves de otros proveedores— sin poder probarlos aquí; decisión de producto), V01, V07 (undo/journal e IPC de sesión: opus, tras M6c), V02, V03, V06 (clasificador), V04, V05, V18, V40 (tarjetas y atención), V08 (lector del bridge), V09, V19, V20, V22 (contención/envío/doctor), V11, V31, V34, V35, V36, V38 (solapan con M5c/M6c o son solo texto), y V30 (fijar acciones de CI por SHA: hace falta red para conocer los SHA; no se hace a ciegas).

---

### Task 1: Transcript de Claude Code y PTY en Windows (asuntos V24 V25)

**Asuntos (filas del triage: id · asunto · ficheros · origen · seguridad · tamaño · banderas):**

- V24 · `ValidTranscript`: con Go >= 1.23 `EvalSymlinks` no resuelve junctions de Windows; en la rama "no existe" se aceptaría `projects\J\later.jsonl` fuera de `projects/` (`paths.go:63-70`). Además TOCTOU: `tail.go:58-73` reabre por ruta y `Lstat` solo mira el último componente. Arreglo: rechazar reparse points o abrir con el descriptor validado. Se puede probar en este PC (Windows) · `claudecode/paths.go`, `tail.go` · M4 CI (0a22518), M4 M1 · BAJO: lee un `.jsonl` fuera de `projects/` y lo manda al móvil del propio usuario · M · no · no · no · no · no
- V25 · PTY en Windows: hijos con consola propia (y el agente, si `serve` muere sin `defer`) sobreviven; arreglo: Job object `KILL_ON_JOB_CLOSE` (proceso suspendido, asignar, `ResumeThread`) más ajustes opcionales (`pty_windows.go:130,241-247`). Probable en este PC · `pty/pty_windows.go` · M3a re-revisión R2 · BAJO: procesos huérfanos tras morir serve (el shim falla cerrado sin host) · M · no · no · no · no · no

---

### Task 2: Endurecimiento de setup, workspace e instalación (asuntos V14 V32 V26 V15 V16 V17)

**Asuntos (filas del triage: id · asunto · ficheros · origen · seguridad · tamaño · banderas):**

- V14 · `loadSettings` lee `.claude/settings.json` y `.gemini/settings.json` con `os.ReadFile` (`workspace.go:315`): un FIFO o `/dev/zero` que plante el agente cuelga `Init` (Lstat, fichero regular y lectura con límite) · `workspace/workspace.go` · M6b · BAJO: cuelga `env init` · S · no · no · no · no · no
- V32 · `mergeGeminiSettings` sin `context.fileName` escribe `["AGENTS.md"]` (`workspace.go:372-391`) y Gemini deja de cargar `GEMINI.md`: escribir `["GEMINI.md","AGENTS.md"]` · `workspace/workspace.go` · M6b · NINGUNO: funcional (Gemini pierde contexto) · S · no · no · no · no · no
- V26 · `ensureRealDirs` no es todo-o-nada (Init ya escribió AGENTS.md y skills antes de fallar) y hay TOCTOU entre comprobar y escribir (`skills.go:87-106`, `workspace.go:97`); endurecer con `os.Root` (go.mod 1.27) · `workspace/{skills,workspace}.go` · M6b T3 · BAJO: un enlace cambiado a mitad de `env init` escribe fuera del entorno · M · no · no · no · no · no
- V15 · `Same` conserva `ok` tras un fallo definitivo de `Verify` (`phoneapp/trust.go:206-232,236-250`): vaciar la caché (`vr.ok.Store(nil)`) · `phoneapp/trust.go` · M6a (re-revisión opus) · BAJO: `still` ya compara ruta y stat; endurecimiento · S · no · no · no · no · no
- V16 · `--apk` acepta cualquier ruta: un FIFO sin escritor bloquea `os.Open` (`cli/appsetup.go:77`); exigir fichero regular tras `Stat` · `cli/appsetup.go` · M6a (re-revisión opus) · BAJO: cuelga `setup` por error del usuario · S · no · no · no · no · no
- V17 · `install.sh`: `wget --https-only` no garantiza que una redirección no baje a http y BusyBox wget no lo soporta (`scripts/install.sh:34-36`); exigir curl o corregir la frase de la spec §8 · `scripts/install.sh`, `scripts/install_test.sh` · M6a revisión final (M-4) · BAJO: degradación de TLS en la instalación · S · no · no · no · no · no

---

### Task 3: Guarda de pantalla del host: tres arreglos pequeños (asuntos V10 V12 V13)

**Asuntos (filas del triage: id · asunto · ficheros · origen · seguridad · tamaño · banderas):**

- V10 · `uidump.Find` toma los bounds del ancestro clicable sin comprobar que no estén vacíos (`uidump.go:226-232,245`): tap en (0,0). Corta en `MaxDepth` sin señal · `uidump/uidump.go` · M5a T1 · BAJO: toque en la esquina superior izquierda · S · no · no · no · no · no
- V12 · `uiLink.Call` no mira `ctx` antes de `Send` (`uilink.go:133`: un click con ctx cancelado sale y el diario dice "stopped"); `OK:true` sin `result` da `(nil,nil)` (`:142-148`) · `cli/uilink.go` · M5a T3 (M1, M4) · BAJO: diario que no refleja lo ocurrido · S · no · no · no · no · no
- V13 · `protected.json` con `{}` o `null` cuenta como lista conocida (`uilink.go:78-89`): se pierde el aviso de "sin lista" (§4.3.5) · `cli/uilink.go` · M5a T3 (M3) · BAJO: guarda sin lista y sin aviso (la app propone lista desde el primer arranque) · S · no · no · no · no · no

---

### Task 4: IPC robusto (asuntos V29)

**Asuntos (filas del triage: id · asunto · ficheros · origen · seguridad · tamaño · banderas):**

- V29 · Robustez del IPC (M1-M2; solo `Listen` re-verificado): Stat/Dial/Remove sin lock (`ipc/server.go:195-201`: secuestro entre dos `serve`), secreto sin escritura atómica (`ipc.go:145-157`: un lector ve un secreto parcial), 1 MiB reservado antes de autenticar sin límite de conexiones (no re-verificado) · `ipc/server.go`, `ipc/ipc.go` · M1a T10 · BAJO: denegación local entre procesos del mismo usuario · M · no · no · sí (ipc) · no · no

---

### Task 5: Chat y ficheros del host (asuntos V23 V28 V41)

**Asuntos (filas del triage: id · asunto · ficheros · origen · seguridad · tamaño · banderas):**

- V23 · Un `chat.send` reintentado tras `session.new` o fuera de los 100 del historial se vuelve a inyectar (`phone_chat.go:225-237`: solo `inflight` e historial): falta un conjunto acotado de ids ya inyectados · `cli/phone_chat.go` · M3a T6 · BAJO: prompt duplicado al agente · M · no · no · no · no · no
- V28 · Contención de `inbox/` (residuales): `os.Remove` de `.partial/<id>` sin `plainDir` (`phone_files.go:454-455,534-535`), TOCTOU `inboxPlain` a `moveExclusive` (`:541-565`), `endUpload` sin test que ejerza la comprobación, fichero sin registrar si `inboxPlain` falla tras mover, sin tope de descargas simultáneas · `cli/phone_files.go` · M3a revisión final (parked) · BAJO: ventana mínima que solo provoca el agente · M · no · no · no · no · no
- V41 · `chatSnapshot` serializa hasta ~6,4 MB con el lock tomado (`phone_chat.go:105-130`): guardar el tamaño por mensaje; `flushTerm` descarta lo acumulado si `sendTerm` falla con error distinto de `ErrNoApp` · `cli/phone_chat.go`, `phone_term.go` · M3a T6/T7 · NINGUNO: rendimiento · M · no · no · no · no · no

---

### Task 6: Tests y flakes del host (asuntos V37 V33 V39)

**Asuntos (filas del triage: id · asunto · ficheros · origen · seguridad · tamaño · banderas):**

- V37 · Tests y flakes del host: `TestRemoteOnClientGoneWhileAsking` (`remotecmd_test.go:657`, falló una vez en Windows), `TestServeWiresRemoteActive` busca texto en `serve.go` (`serve_test.go:366-378`), `Sleep(300ms)` en `TestWatchRemoteResetsAfterASuccess`, esperas de 5 s · `cli/*_test.go` · M5b T5/T8/T9, M6b · NINGUNO: fiabilidad de la CI · S · no · no · no · no (tests) · no
- V33 · Guarda de ejemplos de las skills (test): bloque ```adb tras valla de 4 backticks o `adb{.x}`; `forbiddenRecipe` sin `rm -r` en `$(…)` o tras `;`, `find -delete`, `pm uninstall` sin `--user`, `MASTER_CLEAR`/`FACTORY_RESET` · `cli/skills_commands_test.go` · M6b · NINGUNO: calidad de las skills (el clasificador ya da 🔴) · S · no · no · no · no · no
- V39 · Menores de M3.0, M3a y m3-bridge (tests y comentarios): test de tramas de ids no aceptados, tests de `OnAttach` (orden tras `OnConnect`, `Publish` desde `OnAttach`, `filled()`); `unsupported` convierte el tipo entero a `[]rune` antes de cortar (~4 MiB transitorios); tope de 16 notices por conexión; `rid:""` · `bridge/server.go`, `bridge/*_test.go` · M3.0, m3-bridge · NINGUNO · M · no · no · no · confirm.go (poco) · no

---

### Task 7: Tests JVM de la app (sin UI) (asuntos V21 V42 V43)

**Asuntos (filas del triage: id · asunto · ficheros · origen · seguridad · tamaño · banderas):**

- V21 · `TermWebPolicyTest` sin casos de borde (resto vacío, `?x=1`, `#frag`, `a?/../b`, `%2F`): es la política de qué carga el WebView · `android/.../term/TermWebPolicy.kt` y su test · M3b T4 · BAJO: política de carga del WebView con poca cobertura · S · no · no · no · no · no
- V42 · App, tests JVM: `Bridge.kt` sin tests de socket viejo y de reentrada `onResult` a `request` (`takeIf` por socket ya está); ningún reductor lanza con ningún payload (fuzz de `ChatReducer`, `Reducer`, `RemoteReducer`) · `android/.../bridge/{Bridge,Reducer,ChatReducer}.kt` y tests · M3 (pendientes), m3-bridge · NINGUNO o BAJO: un reductor que lanza provoca bucle de reconexión · M · no · no · no · no · no
- V43 · App, lógica de chat y terminal sin UI: herramientas atascadas en `running` (`chat/ToolStrip.kt`), `Markdown.MAX_BLOCKS` admite uno más y falta test de imagen dentro de enlace, `PickMultipleVisualMedia(10)` sin descontar borradores, pila de sinks de `TerminalFeed` (burbuja sobre `MainActivity` congela la terminal) · `chat/{ToolStrip,Markdown}.kt`, `term/TerminalFeed.kt` · M3b, M4 · NINGUNO · M · no (validar a mano en móvil, opcional) · no · no · no · no


---

## Pasos de cada tarea (se repiten; no se copian en cada una)

- [ ] **Reproducir:** para cada asunto, comprobar en el `main` actual que sigue vigente (leer el código citado) y escribir el test que falla sin el arreglo (RED con comando y salida). Si ya no existe, apuntarlo en el informe y seguir con los demás.
- [ ] **Arreglar** con el cambio mínimo; sin refactors fuera del asunto; sin tocar cadenas de cara al usuario salvo que el asunto lo pida.
- [ ] **Comprobar:** los tests del paquete y `go test ./... -count=1 -timeout 10m`, `gofmt -l .` (vacío), `go vet ./...`. En T1 (Windows) los tests de junction y de Job object se ejecutan en este PC.
- [ ] **Informe:** una fila por asunto (arreglado / ya no vigente / aplazado con motivo), con el test que lo guarda y los comandos y salidas.
- [ ] **Commit** (uno por asunto o uno por tarea si son pequeños; asunto y causa raíz en el cuerpo), con la identidad hybriqtech y el trailer exacto.

## Notas por tarea

- **T1.** V24: `paths.go` debe rechazar un componente que sea un punto de reanálisis (junction o enlace) bajo `projects/` en Windows aunque `EvalSymlinks` no lo resuelva, y `tail.go` no debe reabrir por una ruta que pudo cambiar (abrir y comparar el descriptor abierto con el `Lstat` de la ruta validada, o rechazar reparse points al abrir); el test crea una junction real con `mklink /J` (se salta si no se puede). Falla cerrado. V25: Job object con `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` (`golang.org/x/sys/windows` ya está en `go.mod`): crear el proceso suspendido, asignarlo al job y reanudar; test (solo Windows, `//go:build windows`) de que cerrar el job mata al hijo y a sus nietos con consola propia; sin cambiar el comportamiento en Linux y macOS.
- **T2.** V14: `Lstat`, exigir fichero regular y leer con límite (1 MiB) en `loadSettings`; un FIFO o un enlace es un error que dice qué hacer (test en Unix con `syscall.Mkfifo`, `//go:build unix`). V32: sin `context.fileName`, `["GEMINI.md","AGENTS.md"]` (y los casos existentes de `mergeGeminiSettings` siguen). V26: una pasada previa de `ensureRealDirs` sobre todas las rutas antes de escribir ningún fichero (todo o nada); `os.Root` solo si es un cambio pequeño y claro; si no, se documenta el TOCTOU residual. V15: vaciar la caché (`vr.ok.Store(nil)`) tras un fallo definitivo de `Verify`. V16: `--apk` exige fichero regular tras `Stat` (error claro; nunca abrir un FIFO). V17: `scripts/install.sh`: exigir `curl` o decir la verdad sobre `wget --https-only`; `scripts/install_test.sh` sigue pasando (no hay `shellcheck` aquí: cuidar la sintaxis POSIX).
- **T3.** V10: bounds vacíos del ancestro clicable ⇒ no hay punto de toque (error, nunca (0,0)); V12: `uiLink.Call` comprueba `ctx` antes de `Send` y trata `OK:true` sin `result` como error o resultado vacío explícito; V13: `protected.json` con `{}` o `null` no cuenta como lista conocida.
- **T4.** V29 (revisión opus): lock alrededor de Stat/Dial/Remove en `Listen` (secuestro entre dos `serve`), escritura atómica del secreto (fichero temporal y `rename`, permisos 0600 desde la creación), y comprobar (leyendo el código) si de verdad se reservan 1 MiB antes de autenticar sin límite de conexiones; si es así, un tope razonable. Cada cambio con su test y sin abrir el socket a otros usuarios.
- **T5.** V23: conjunto acotado de ids de `chat.send` ya inyectados (un reintento tras `session.new` o fuera del historial no vuelve a teclear el prompt); V28: `plainDir` en el `Remove` de `.partial/<id>`, cerrar la ventana `inboxPlain`→`moveExclusive` donde sea barato, test de `endUpload`, y tope de descargas simultáneas; V41: guardar el tamaño por mensaje para no serializar con el lock tomado, y `flushTerm` no descarta lo acumulado ante un error que no sea `ErrNoApp`.
- **T6.** V37: arreglar o estabilizar (esperas por condición, no `Sleep`) los tests citados sin quitar cobertura; V33: la guarda de ejemplos de las skills detecta un bloque ```adb tras una valla de 4 backticks o con info string `adb{.x}` y las recetas prohibidas que le faltan (`rm -r` dentro de `$(…)` o tras `;`, `find -delete`, `pm uninstall` sin `--user`, `MASTER_CLEAR`, `FACTORY_RESET`), con casos negativos; V39: los tests de `OnAttach` y de tramas de ids no aceptados, `unsupported` que corta antes de convertir a `[]rune`, tope de notices por conexión.
- **T7.** (Kotlin, solo si Gradle y el SDK funcionan en este PC: `android/gradlew` o `gradle` y `ANDROID_HOME`; si no, la tarea se difiere y se dice.) V21: casos de borde de `TermWebPolicyTest`; V42: tests de socket viejo y de reentrada de `Bridge`, y que ningún reductor lanza con ningún payload (fuzz acotado de `ChatReducer`, `Reducer`, `RemoteReducer`); V43: herramientas atascadas en `running`, `Markdown.MAX_BLOCKS` y la imagen dentro de un enlace, descuento de borradores en `PickMultipleVisualMedia`, pila de sinks de `TerminalFeed`; solo lógica sin UI (no tocar `ui/**`).
