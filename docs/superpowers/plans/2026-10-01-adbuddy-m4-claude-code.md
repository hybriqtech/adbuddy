# ADBuddy m4 · Adaptador de Claude Code · Plan de implementación (registro posterior)

> **Este plan es un registro posterior de lo ejecutado.** M4 no tuvo un plan formal previo: se ejecutó desde `handoff.md` y la spec, con una revisión opus de seguridad y revisiones por tarea, y este documento se escribió al final (orden del handoff y del supervisor adbuddy-e5). Recoge qué se hizo, en qué commit, qué hallazgo cerró cada ronda de arreglos y qué se decidió. No lleva código ni pasos para repetir: el código es el de los commits, y la autoridad es la spec.
>
> **Coste si el registro se equivoca:** puede no reflejar fielmente lo hecho. La fuente de verdad es `git log 244e56f..m4` y el ledger `.superpowers/sdd/2026-10-01-adbuddy-m4/progress.md`.

**Objetivo:** que, con Claude Code como agente, el chat del móvil muestre sus respuestas como burbujas (markdown seguro), la herramienta que usa en cada momento y los permisos como tarjetas con botones (Sí / Sí, siempre / No), sin debilitar nada de M2: el shim sigue clasificando igual aunque Claude Code ya haya permitido una llamada.

**Arquitectura:** rama `m4`, sobre `main` 244e56f. Seis hooks de Claude Code (`adbuddy hook <evento>`, que falla abierto) llevan los eventos al daemon por IPC (`KindHook`); un adaptador en el host lee el transcript de Claude Code y publica `chat.message`, `tool.activity` y `permission.*` por el protocolo de M3; la app pinta markdown, la franja de herramientas y las tarjetas. Los tipos de protocolo ya existían (M3.0): M4 no añade campos.

**Tecnologías:** las de M3. Nada nuevo en `go.mod` ni en `libs.versions.toml`.

**Spec y referencias:**
- Spec: `docs/superpowers/specs/2026-10-01-adbuddy-m4-claude-code-design.md`.
- Spike (Claude Code 2.1.286, grabaciones en `host/internal/claudecode/testdata/cc-2.1.286/`): `docs/superpowers/specs/2026-10-01-adbuddy-m4-spike-claude-code.md`.
- Revisión opus de seguridad del host: `.superpowers/sdd/2026-10-01-adbuddy-m4/review-security.md`.
- Ledger con todas las revisiones, rulings y menores diferidos: `.superpowers/sdd/2026-10-01-adbuddy-m4/progress.md`; informes de tarea `task-*-report.md` en el mismo directorio.
- Pendientes que salen de M4 (validaciones manuales y menores diferidos): `docs/superpowers/plans/pendientes-m3-m5.md`, sección "M4".

**Verificación:** `go vet ./...` y `go test -timeout 300s ./...` en el host (sin `-race`: el PC de desarrollo es Windows sin cgo, lo corre la CI de Linux); `:app:testDebugUnitTest` en la app. La prueba con Claude Code real (`ADBUDDY_REAL_CLAUDE=1`) usa un config temporal y nunca toca el móvil. Nada de Compose se ha visto en un móvil (ver las validaciones manuales de pendientes).

## Restricciones globales (las que se aplicaron)

- Rama `m4`; sin push ni merge: el merge a `main` lo hace el supervisor. Nunca `--force`.
- Identidad local del repo (hybriqtech); trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` exacto (lo exige el hook `commit-msg` local); nunca `--no-verify`.
- El móvil del usuario no se toca; los tests usan un adb falso y, para Android, JVM.
- Un subagente a la vez, Sonnet salvo seguridad y revisión final (opus); sin código en la sesión del controlador; sin subagentes dentro de una tarea.
- Cada tarea compila y pasa sus tests por sí sola.

## Tareas, por commit

### Documentación previa
- `14ba8c0` docs: spike de Claude Code 2.1.286 (payloads de hooks, transcript y respuesta de permisos grabados).
- `24a90ac` docs: spec del adaptador (hooks, transcript, permisos) y regla exacta verificada.

### Host
1. **Módulo `claudecode`** (`fa3ed31`): paquete aislado y sin dependencias con tipos de hook, decisión, regla "Sí, siempre" exacta (`rule.go`), parser (`transcript.go`) y lector del transcript (`tail.go`, `paths.go`).
2. **`adbuddy hook <evento>`** (`dbd9fbe`): reenvía stdin al daemon por IPC (`ipc.KindHook`) y escribe la decisión; falla abierto y nunca sale con 2.
3. **Hooks en el workspace** (`6f05f63`): instala los seis hooks en `.claude/settings.json` del entorno.
4. **Adaptador** (`fe86637`): `cli/phone_agent.go` y `phone_perm.go`: burbujas y actividad desde el transcript, permisos con botones y degradación a `last_assistant_message` más terminal.
5. **Prueba real** (`c4d8fc1`): Claude Code real contra el adaptador (`ADBUDDY_REAL_CLAUDE=1`): allow, deny y regla exacta.
6. **Doctor** (`efe6057`): avisa de hooks globales de Claude Code que pueden chocar con los de ADBuddy; README del adaptador.

### App
7. **Markdown seguro** (`78520c7`): parser propio sin HTML, sin imágenes y solo enlaces `http(s)`, acotado.
8. **Reductor** (`87fb5b5`): `ChatReducer` trata `tool.activity` y los permisos del agente (upsert, resolved, snapshot, reset).
9. **PermissionUi** (`1908266`): lógica de las tarjetas de permiso, de la franja de herramientas y `permission.reply`.
10. **Composables** (`d8a5601`, WIP sin cablear): `MarkdownText`, `PermissionCard`, strings en/es y markdown en las burbujas.
11. **Cableado de la UI** (`5c26d1a`): `ChatActions`/`ChatRoot`/`ChatScreen` con las tarjetas encima del input y la franja como último item, detalle leído entero antes de responder y enlaces que muestran su dominio real.
12. **Notificaciones** (`ff084b9`): `ConversationView.asking` con aviso genérico y titular de "Listo" con el último mensaje del agente.

### Rondas de arreglos (cada una con su revisión; límite 5 por tarea, ninguna pasó de la 1)
- **A, seguridad del host (opus), `d8a5601..2449f17`:** cerró **I1** (la lista negra de "Sí, siempre" se saltaba con comodines, bidi y fastboot/scrcpy/gradlew: pasa a lista blanca ASCII; `0463fb9`), **I2** (`chat.send` no se frenaba con un permiso pendiente si el adaptador no estaba activo) e **I3** (la tarjeta cortaba el detalle a 2000 B pero el Sí aprobaba el comando entero: sin tarjeta si no muestra lo que aprueba), junto con **M4** (`recover` en el hook), todos en `4f40919`, y **M6** (test de que un Sí del móvil a un `adb shell rm` no salta el shim; `2449f17`).
- **A, ronda 1 (`2449f17..01b64db`):** cerró **N1** (`babd2cf`: sin atención del host cuando un permiso se deja a la terminal por tarjeta no apta, tope o ritmo, `chat.send` tecleaba Enter en el diálogo), **N2** (más herramientas excluidas: flutter, react-native, bundletool, android, expo, doas, pkexec, run0), **N3** (categorías Mn/Me y rellenos Hangul/Braille invisibles) y la parte host de **M-b** (rachas de blancos que esconden texto), los tres últimos en `01b64db`. Re-revisión limpia.
- **B, parser y lector (Sonnet), `01b64db..f7e7f1c`:** `ec9abd4` (**I-1**: id estable de los prompts de la terminal; solo JSON roto cuenta como línea mala; tope al texto acumulado por mensaje, M5) y `f7e7f1c` (el lector detecta el fichero reemplazado, descarta la línea a medias con `fromEnd` y revalida que no sea enlace). Revisión limpia.
- **C, reductor y PermissionUi (Sonnet), `f7e7f1c..782db14`:** `387c6f1` (**I-2**: una tarjeta no se contesta dos veces ni recién promovida; una sola vía y decisiones validadas), `665c032` y `5c6d791` (Markdown O(n²) en `[` sin cerrar y en `[` que comparten `]`), `fe3c4a4` (el snapshot se queda con la última versión y un estado final de herramienta no vuelve a `running`), `472ebe7` (sin `t!!`), `782db14` (la guarda de permisos falla cerrada, se rearma al reconectar y el controlador sigue las tarjetas del bridge). Re-revisión limpia.
- **D, cableado de la UI (opus, por ser seguridad de app), `782db14..71b793c`:** `ad3ff16` (**I1**: un enlace con userinfo no es enlace y el texto del enlace no lleva bidi ni invisibles), `3ca8390` (**Me5**: validación manual del detalle largo en la spec) y `71b793c` (**Me1** la guarda se relee por época del controlador, **Me2** la etiqueta de Claude Code pesa más que el resumen, **Me4** No no espera al final del detalle). Re-revisión limpia.
- **E, `ConversationView.asking` y titular (Sonnet con lente de privacidad), `71b793c..8cde298`:** `0c26ec2` (**I2**: `asking` solo con el enlace conectado y sin pausa; **M1** acciones y aviso "Listo" como funciones puras con tests) y `eeb8290` (**M3/M4/M5**: el titular de "Listo" sanea separadores e invisibles Unicode, quita solo delimitadores de markdown y acota el texto antes de las regex), más `8cde298` (docs: el aviso de permiso conserva Cortar acceso y es silencioso; canal alto en pendientes). Re-revisión limpia.
- **F, documentación y acotado de entradas (`5edb8e8`, `ac39360`, `ddae3dc` y este registro):** en el host, acotado y validación de las entradas de texto del transcript que llegan al móvil (versión de Claude Code e ids de línea, de mensaje y de prompt; M2 y M3 de la revisión de seguridad); README de Android; sección M4 de pendientes.

## Rulings tomados

Copiados tal cual del ledger (`progress.md`) y de la spec §12.

### Del ledger
- Ruling: no hay plan formal de M4; se ejecuta desde handoff.md y la spec, y el plan se escribe como registro tras las revisiones — por orden del handoff/supervisor adbuddy-e5 — cuesta: el plan podría no reflejar fielmente lo hecho.
- Ruling: un subagente a la vez, Sonnet salvo seguridad/final (opus) — orden del supervisor — cuesta: más lento.
- Ruling: arreglar I1-I3 y M4 antes de integrar (recomendación del revisor); M1,M2,M3,M5,M6 -> M6 se añade en el arreglo de I1 (test spec 5.3); M1-M3,M5,M7 se aplazan como menores y los triará la revisión final — son menores sin ejecución sin confirmación — cuesta: queda superficie menor abierta.
- Ruling: orden de tareas — A (host: seguridad I1,I2,I3,M4,M6) -> B (host: parser I-1 + menores de tail/transcript/M5) -> C (android: I-2, Markdown O(n^2), reductor) -> D cableado app -> E ConversationView.asking+headline -> F docs + plan registro; cada una con su revisión (opus para A, Sonnet el resto), revisión final opus — secuencial por orden del supervisor — cuesta: más lento que paralelizar host/android.
- Ruling: aplazados como menores (los triará la revisión final): M1 TOCTOU, M2 versión sin validar, M3 ids sin validar, M7 retirada por tool+resumen, Markdown MAX_BLOCKS/imagen anidada, ToolStrip running — no ejecutan nada sin confirmación — cuesta: superficie menor abierta.
- Ruling: N2 — ampliar palabras excluidas (android, flutter, bundletool, react-native, expo, doas, pkexec, run0) y documentar en spec §5.3 que ningún filtro léxico cubre make install/scripts/alias — la lista blanca de primeras palabras es mayor cambio de producto — cuesta: "Sí, siempre" ofrecido para scripts arbitrarios.
- Ruling: M-b — parte host (CardSafe rechaza rachas largas de blancos y blancos al final de línea) en la ronda 1; parte app (softWrap=true, sin tope de alto o aviso de "hay más") en la tarea D de cableado — cuesta: hasta cablear, la tarjeta no está en uso.
- Ruling: ronda 1 de C arregla Importantes 1-2 y Menores 1,3,4,5,6; Menor 2 (conflación connected) se difiere — acotado por la caducidad del host — cuesta: tarjeta con answering hasta resolved/600 s en caso raro.
- Ruling: defaults answering/guardLeftMs se quedan opcionales hasta D, que debe pasarlos explícitos y usar chat.canAnswer(id) + app.clock::now, y entonces hacerlos obligatorios (PermissionCard.kt dejó de compilar al hacerlo).
- Ruling: ronda 1 de D arregla I1, Me1, Me2, Me3, Me4, Me5; Me6 se ignora — historial ya hecho, reescribir sin push no aporta — cuesta: commit grande en la PR.
- Ruling: I1 — se mantiene "Cortar acceso" en el aviso asking (acción segura que solo estrecha el acceso; el aviso sin él obligaría a abrir la app para cortar) y se documenta en spec §7.4 — cuesta: lectura literal de "sin botones" incumplida.
- Ruling: I3 — el aviso asking se queda silencioso en M4 (mismo canal LOW) y se apunta en pendientes-m3-m5.md y en la PR como decisión de producto (canal de importancia alta con versión pública genérica) — cambiar el canal es una decisión de producto de M3/M5, no de este hito — cuesta: un permiso pendiente con la app en segundo plano puede pasar desapercibido hasta que se mire el móvil; el permiso caduca a los 600 s y vuelve a la pregunta de la terminal.
- Ruling: ronda 1 de E arregla I2, M1, M2, M3, M4, M5; M6 se ignora.
- Ruling: tarea F incluye además un arreglo host mínimo de entradas del transcript (M2 versión y M3 ids/uuid acotados y validados) — son las únicas entradas de texto no validadas que llegan al móvil — cuesta: un commit más a revisar.
- Ruling (tarea B): user-prompt ya estaba en ignoredPrompts, no listado en spec 4.2 -> se mantiene (inocuo, ignora prompts internos).

### De la spec §12
- **Ruling:** hooks de solo seis eventos, sin `PreToolUse`/`PostToolUse` — cada hook es un proceso por herramienta y el transcript ya las trae — si me equivoco, una latencia de "running" mayor y se añade `PostToolUse`.
- **Ruling:** "Sí, siempre" solo para Bash, regla exacta y sin adb/shim — es una regla persistente en `settings.local.json` y el supervisor exige que nunca toque el móvil ni 🔴 — si me equivoco, la persona repite un "Sí" más.
- **Ruling:** `adbuddy hook` falla abierto y nunca sale con 2 — un hook roto no puede bloquear a Claude Code ni dejarlo sin respuesta — si me equivoco, un permiso que debía ir al móvil cae a la terminal.
- **Ruling:** sin móvil conectado, `PermissionRequest` no espera — la persona está en el PC — si me equivoco, un permiso se queda sin botones en el móvil que acaba de conectar (y lo resuelve en el PC).
- **Ruling:** el id del permiso lo pone el daemon — el payload no trae `tool_use_id` (spike).
- **Ruling:** sin identidad del par del socket en M4 (ver §4.3) — el hook solo puede pedir, no conceder — si me equivoco, un agente hostil molesta con tarjetas falsas (acotado a 8).
- **Ruling:** las preguntas 🔴 no pasan a tarjetas (§7.5).
- **Ruling:** el lector empieza por el final en `resume`/`compact` — no se reenvía historia — si me equivoco, el chat arranca vacío tras `--continue` (la historia vive en la terminal).

## Revisiones

- **Opus de seguridad del host** (`review-security.md`): con reservas; confirmó que (a) un hook que falla abierto solo devuelve a Claude Code a su pregunta normal y (b) el shim nunca depende del hook. Importantes I1-I3, menores M1-M7: ver las rondas A.
- **Sonnet de parser y app:** importantes I-1 (ids de prompts de la terminal) e I-2 (tarjeta contestable en el hueco de la siguiente): rondas B y C.
- **Por tarea:** C y E con Sonnet, D con opus; todas cerraron limpias en la ronda 1.
- **Pendiente al escribir este registro:** la revisión final opus de toda la rama y la validación manual en un móvil (pendientes, sección "M4").

## Validaciones manuales pendientes

Viven en `pendientes-m3-m5.md`, sección "M4": las de la spec §11, la duda principal (cómo se entera el daemon de que un permiso se contestó en la terminal), el desplazamiento del detalle largo de la tarjeta y el aviso de permiso silencioso.
