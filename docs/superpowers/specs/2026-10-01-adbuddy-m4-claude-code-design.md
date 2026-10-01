# ADBuddy M4 · Adaptador de Claude Code · Diseño

Fecha: 2026-10-01. Hito M4 de la hoja de ruta (v1 §13). Parte de: v1 §5.4, protocolo M3-M5 §5, M3 §3.3/§5/§7 y el spike (`2026-10-01-adbuddy-m4-spike-claude-code.md`, Claude Code 2.1.286). Todo lo que dice del formato de Claude Code está grabado en `host/internal/claudecode/testdata/cc-2.1.286/`; lo que el spike no pudo observar está marcado **[validar]** y se cierra en la validación del hito, nunca "de memoria".

## 1. Qué resuelve

Con Claude Code, el chat del móvil deja de ser "terminal + mensajes que escribo yo": la persona **lee lo que responde el agente como burbujas** (markdown seguro), ve **qué herramienta está usando** y **contesta los permisos con botones** (Sí / Sí, siempre / No). La terminal sigue ahí como respaldo. Los demás agentes siguen en vista de terminal (v1.1 trae Codex).

### 1.1 Criterios de éxito
1. Una respuesta de Claude Code aparece en el móvil como burbuja `assistant` (varias líneas del transcript → un mensaje, upsert por `message.id`).
2. Cada herramienta aparece como `tool.activity` (`running` → `done`/`error`) sin volcar nunca el contenido de `tool_result` al móvil.
3. Un `PermissionRequest` aparece en el móvil; responder allí desbloquea a Claude Code; responder en la terminal retira la tarjeta del móvil.
4. Si el formato del transcript cambia o falla el parseo, el chat **degrada** a `last_assistant_message` en cada `Stop` más la terminal, sin perder permisos ni estado.
5. Nada de M4 debilita M2: el shim clasifica igual aunque Claude Code ya haya permitido la llamada (§5.3).

### 1.2 Fuera de alcance
Codex/Gemini (v1.1); notificaciones con botones de permiso (§7.3); subagentes (`isSidechain` se ignora salvo `tool.activity`); historial persistente (v1 §14); mover las preguntas 🔴 de ADBuddy a tarjetas (§7.4); identidad del par del socket IPC (§4.3, ruling).

## 2. Piezas

| Pieza | Dónde | Hace |
|---|---|---|
| Hooks | `.claude/settings.json` del entorno | Claude Code ejecuta `adbuddy hook <evento>` en seis eventos (§3) |
| `adbuddy hook` | `host/internal/cli/hook.go` | Reenvía stdin al daemon por IPC (`KindHook`) y escribe en stdout la decisión. **Falla abierto** (§3.3) |
| Módulo `claudecode` | `host/internal/claudecode/` | Aislado y sin dependencias del resto: tipos de hook, decisión, regla "siempre", parser y lector del transcript |
| Adaptador | `host/internal/cli/phone_agent.go` | `agentArea`: estado, burbujas, `tool.activity`, permisos, degradación |
| App | `ChatReducer`, `Markdown.kt`, `ui/` | Pinta markdown seguro, actividad y tarjetas de permiso |

## 3. Hooks

### 3.1 Eventos y qué hace cada uno
Se instalan solo estos (cada hook es un proceso nuevo: nada de `PreToolUse`/`PostToolUse`, el transcript ya trae las herramientas):

| Evento | El daemon hace |
|---|---|
| `SessionStart` | Valida y fija `session_id` + `transcript_path` y arranca el lector (§4.1). Marca el adaptador como **activo**: `session.state.view = chat` |
| `UserPromptSubmit` | `state` → `working` (sin esperar al PTY); limpia `atención` |
| `Stop` | `state` → `idle`; relee el transcript y, si falta el último texto, emite el respaldo (§4.4); limpia `atención` |
| `Notification` | `permission_prompt` activa `atención` (§6.3); `idle_prompt` y `agent_needs_input` → `state` `waiting` **[validar los nombres reales de `notification_type`]** |
| `PermissionRequest` | Crea la petición de permiso y **espera** la respuesta (§5) |
| `SessionEnd` | Cierra el lector; resuelve permisos pendientes de esa sesión (`by: agent`) |

### 3.2 Instalación
`workspace.Init` añade a `.claude/settings.json` (fusionando, sin tocar nada más y sin duplicar) un bloque `hooks` con un hook `type: command` por evento: `"<entorno>/.adbuddy/bin/adbuddy" hook <Evento>` (ruta absoluta con `/`; Claude Code usa bash también en Windows, comprobado en el spike). `PermissionRequest` lleva `timeout: 610` (s). Si ya hay un hook con `adbuddy hook` y otra ruta, se actualiza esa entrada. Los hooks del usuario en ese fichero se conservan.

### 3.3 `adbuddy hook <evento>` — falla abierto
- Lee el JSON de stdin (máx. 1 MiB; más → ignora), llama al daemon por IPC (`KindHook`, `Argv: [evento]`, el JSON como stdin) y copia su stdout al suyo.
- **Siempre sale con 0.** En Claude Code un `exit 2` **bloquea** (p. ej. `Stop` seguiría trabajando; `PermissionRequest` deniega): un hook roto nunca debe bloquear al agente. Daemon caído, secreto ilegible, error de IPC o respuesta ilegible → sin salida y `exit 0`; Claude Code cae a su comportamiento normal (la pregunta en la terminal).
- stdout solo lleva el JSON de la decisión (nada de logs); los avisos van a stderr.

### 3.4 Hooks globales del usuario (v1 §5.4)
Los hooks se combinan con los de `~/.claude/settings.json`. Si el usuario tiene un `PermissionRequest` o `PreToolUse` global que responde a la misma petición, puede ganar el suyo o chocar con el nuestro. `adbuddy doctor` lo avisa (lee ese fichero, solo lectura) y la documentación lo explica. No se intenta desactivarlos.

## 4. Mensajes: transcript

### 4.1 Lector (módulo `claudecode`, aislado)
- **Origen:** `transcript_path` de `SessionStart`. **Validación (el JSON viene de un proceso local que no es de fiar, §4.3):** ruta absoluta, extensión `.jsonl`, regular (no enlace), y, resueltos los enlaces simbólicos, dentro de `<config de Claude Code>/projects/` (`$CLAUDE_CONFIG_DIR` o `~/.claude`). Si no, se ignora el evento y se avisa en el log del host.
- **Lectura:** sondeo cada 300 ms mientras haya sesión; solo líneas completas (la última sin `\n` espera); línea > 8 MiB: se salta; tamaño < offset (truncado/reescrito): empieza de cero; el fichero puede no existir aún: reintenta. Offset inicial: **0** si `source` es `startup` o `clear`; **el final** si `resume` o `compact` (no se reenvía historia vieja).
- **`session_id` nuevo** (`/clear`, `--resume`, `/compact`): cambia el lector. Un `SessionStart` por sesión viva.

### 4.2 Parser (un solo fichero, `transcript.go`)
Solo se leen estos campos; todo lo desconocido se ignora (la lista de `type` crece con la versión):
- `type: assistant`, no `isSidechain`: `message.id`, `message.content[]`. Cada línea trae **un bloque** de un mismo `message.id` (spike): el texto del mensaje es la concatenación en orden de los bloques `text` vistos; cada línea con texto hace upsert de `chat.message{id: message.id, role: assistant, text}`. Los bloques `thinking` se **descartan** (vacíos o no, no se muestran). Texto > 256 KiB: se corta con `…`.
- `tool_use` (`id`, `name`, `input`) → `tool.activity{id, messageId, summary, status: running}`.
- `type: user` con `content` lista y bloques `tool_result` (`tool_use_id`, `is_error`) → `tool.activity` de ese `id` con `done`/`error`. **El texto del resultado no se lee ni se envía** (puede ser enorme o tener secretos).
- `type: user` con `content` cadena: es un prompt. Si coincide (texto normalizado) con un `chat.send` de los últimos 2 min, se descarta (ya está en el chat con su `id`); si no, `chat.message{role: user, id: "t_"+uuid}` (se escribió en la terminal). Las cadenas que empiezan por `<command-`, `<local-command`, `<system-reminder` o `<bash-` se descartan **[validar]** con `/clear`, `/compact`, `!cmd` reales.
- **Resumen de herramienta** (`summary`, ≤ 120 caracteres, una línea): Bash → `description` o el comando; Read/Edit/Write → verbo + nombre del fichero (sin ruta); Grep/Glob → verbo + patrón; WebFetch → dominio; MCP (`mcp__srv__tool`) → `srv: tool`; otro → el nombre. Nunca el `input` entero.

### 4.3 Seguridad de la entrada
`adbuddy hook` habla con el daemon con el secreto de M1 y el agente es del mismo usuario del SO, así que puede llamarlo (y falsificar payloads). **Ruling: sin identidad del par del socket en M4** (pendiente de M3): el daño posible es bajo y se acota con (a) validación de `transcript_path` (§4.1), `session_id` (UUID) y `cwd` (dentro del entorno, comparación sin mayúsculas en Windows); (b) como mucho **8 permisos pendientes** y 1 por segundo; (c) los hooks no pueden **conceder** nada: la decisión solo sale hacia quien la pidió y solo la fija el móvil. Un agente que falsifica `Stop` solo escribe una burbuja de texto que ya podría escribir de verdad. `ADBUDDY_SOURCE` no se usa para decidir nada aquí.

### 4.4 Degradación
`Degraded` = no se puede leer el fichero tras 5 s de `SessionStart`, o 20 líneas seguidas con JSON inválido, o versión de Claude Code (`version` de las líneas) fuera de la lista probada (`claudecode.TestedVersions`, hoy 2.1.x): en ese caso el adaptador:
1. pone `view = terminal` y avisa **una vez** con `notice` ("Claude Code <v> is not a tested version: chat falls back to the terminal");
2. en cada `Stop`, emite `chat.message{role: assistant, id: "last_"+prompt_id, text: last_assistant_message}`;
3. sigue sirviendo permisos (no dependen del transcript).

**Respaldo sin degradar:** en `Stop` se relee el transcript (hasta 3 veces en 1,5 s); si sigue sin aparecer un texto igual a `last_assistant_message`, se emite el respaldo del punto 2. Si el transcript lo trae después, el lector **omite** un texto igual a un respaldo ya emitido en ese turno (no se duplica).

## 5. Permisos

### 5.1 Flujo
1. Llega `PermissionRequest`. Si **no hay app conectada**, el daemon responde sin decisión al instante (la persona está en el PC: pregunta normal de la terminal). Con 8 pendientes ya, igual.
2. Si hay app: id `p_<aleatorio>` (el payload no trae `tool_use_id`), `permission.request{id, tool, summary, detail, expiresAt = ahora+600 s, allowAlways}` como evento (entra en el `snapshot.permissions`). `summary` = resumen de herramienta (§4.2); `detail` = comando completo (Bash) o ruta (Edit/Write/Read) o `tool_input` recortado, ≤ 2000 caracteres.
3. Espera la primera de: `permission.reply` del móvil; resolución por la terminal (§5.2); 600 s (`timeout`); `SessionEnd`; cierre de la conexión del hook.
4. Respuesta del hook (stdout):
   - `allow` → `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}`
   - `deny` → `…"decision":{"behavior":"deny","message":"Denied from the phone."}` **[validar el texto que ve el modelo]**
   - `allow_always` → `allow` más `updatedPermissions` (§5.3)
   - `timeout`, terminal o error → **sin salida**: Claude Code sigue con su pregunta en la terminal (en `-p` deniega, observado).
5. `permission.resolved{id, decision, by}` siempre, y se quita de `snapshot.permissions`. Una `permission.reply` tardía o repetida recibe el `notice` de respuesta tardía de M2 (`bridge.StaleReplyText`).

### 5.2 Responder en la terminal
Mientras el hook espera, Claude Code también enseña su diálogo. Cómo se entera el daemon de que ya se contestó ahí **[validar]**: (a) Claude Code cierra la conexión del hook (el daemon ya lo trata como resolución `terminal`); (b) si no la cierra, la primera línea nueva de `tool_result` o `assistant` en el transcript tras la petición, o un `Stop`/`UserPromptSubmit`/`PostToolUse`-equivalente, la resuelve `by: terminal`. Hasta validarlo, (b) es la regla; un falso positivo solo retira antes la tarjeta (la persona contesta en la terminal).

### 5.3 "Sí, siempre" — la regla es exacta y nunca toca adb
`allowAlways` es **false** salvo que se cumpla todo esto:
- la herramienta es **Bash** (v1: Edit/Write/Read/WebFetch/MCP no ofrecen "siempre");
- el comando, recortado, tiene ≤ 200 caracteres, una sola línea y **ninguno** de los caracteres `( ) * \ " '` ni la secuencia `:*` (cambian la sintaxis de una regla de Claude Code o la hacen más amplia);
- **no contiene** `adb` ni `adbuddy` como palabra (sin mayúsculas), ni `ANDROID_SERIAL`, ni `.adbuddy` (nada que toque el shim ni al móvil).

Con `allow_always` la respuesta lleva `updatedPermissions:[{"type":"addRules","rules":[{"toolName":"Bash","ruleContent":"<comando exacto>"}],"behavior":"allow","destination":"localSettings"}]`. **Sin comodines construidos a partir del comando**: nunca `…:*`. Claude Code escribe él mismo `.claude/settings.local.json` del entorno (observado en el spike), así que ADBuddy no edita ese JSON. Si el móvil manda `allow_always` para una petición con `allowAlways: false`, se trata como `allow` (el host no se fía de la app). **Comprobado en el spike (2.1.286, `claude -p --permission-mode default`):** con `Bash(touch uno.txt)` en `settings.local.json`, `touch uno.txt` se ejecuta y `touch uno.txt dos.txt` y `touch uno.txt && touch tres.txt` se deniegan: la regla sin `:*` es exacta. (`echo …` simple no sirve de prueba: Claude Code lo permite siempre.)

**Regla de oro (v1 §4):** el shim clasifica y registra **toda** llamada a `adb` aunque Claude Code ya la haya permitido (`Bash(adb:*)` está preaprobado desde M1, y un "Sí" del móvil permite el comando entero). Un permiso de Claude Code no es una confirmación 🔴: un 🔴 sigue pidiendo la suya en el móvil, aparte. Un test del adaptador lo comprueba (permitir un `adb shell rm …` no salta el shim).

## 6. Estado y entrada

### 6.1 `session.state`
`view = chat` cuando el adaptador está activo para la conversación actual (llegó `SessionStart`) y no `Degraded`; si no, `terminal`. `state`: los hooks mandan sobre el sondeo de salida del PTY **mientras haya adaptador** (`UserPromptSubmit` → `working`, `Stop` → `idle`, `Notification` → `waiting`); sin adaptador, como en M3. Un cambio de `conversation` (nuevo proceso) reinicia adaptador, lector, permisos y atención.

### 6.2 `chat.send` y el diálogo en pantalla
Hoy `Busy` solo reconoce los diálogos de Claude Code en inglés (pendiente de M3). Con adaptador, la defensa deja de ser el texto de la pantalla: `chat.send` responde `error busy` ("Answer the pending question first.") si hay **permisos pendientes o `atención`** (el diálogo está en pantalla aunque no lo reconozcamos). `Busy` se mantiene para el diálogo de confianza y para agentes sin adaptador.

### 6.3 `atención`
Se activa con `Notification` `permission_prompt` y con un `PermissionRequest` pendiente o resuelto en la terminal; se limpia con `UserPromptSubmit`, `Stop`, `SessionEnd`, un `tool_result` posterior y 120 s de caducidad (por si se pierde un evento: es solo un freno de `chat.send`).

## 7. App

1. **Markdown seguro** (`Markdown.kt`, parser propio sin dependencias): párrafos, `#`, listas, citas, `**negrita**`, `*cursiva*`, `` `código` `` y bloques con ``` ```; enlaces `[t](url)` solo `http(s)`, **se muestran con su dominio y se abren al pulsar** (protocolo §5.1); **sin HTML** (se pinta como texto) **y sin imágenes** (`![x](u)` → el texto `x`, nunca se carga la URL); entrada acotada (el texto del host ya va cortado).
2. **Actividad de herramienta:** `tool.activity` bajo su mensaje (o suelta): chip con el resumen y estado; upsert por `id`; se conservan las últimas 200.
3. **Tarjeta de permiso:** `PermissionCard` con herramienta, `summary`, el `detail` completo desplazable y cuenta atrás; botones Sí / Sí, siempre (solo si `allowAlways`) / No; manda `permission.reply` (sin `rid`) y espera `permission.resolved` para quitarla (no la quita ella). Se distingue visualmente de una pregunta 🔴 de ADBuddy: etiqueta "Claude Code pregunta" frente a "ADBuddy: acción delicada". Si hay sesión nueva (`Reset.NEW_SESSION`) o se desconecta, las tarjetas se olvidan según la regla de cada `Reset` (el `snapshot` las trae de nuevo).
4. **Notificaciones:** un permiso pendiente con la app en segundo plano muestra un aviso **genérico** ("Claude Code necesita una respuesta") que abre el chat; **sin botones y sin el comando** (la pantalla de bloqueo, M3b). El aviso "Listo" usa el último mensaje `assistant` (primeras 100 letras) si lo hay.
5. **Preguntas 🔴 de ADBuddy:** siguen en `ConfirmDialog` (M2). **Ruling:** no se mueven a tarjetas en M4: son otra clase de pregunta, de otro dueño (el shim) y con otro riesgo, y mezclarlas con las del agente invita a contestar una por otra. Coste si me equivoco: una tarea de UI en un hito posterior.

## 8. Protocolo
Los tipos de M4 **ya están** en `protocol_chat.go`/`Protocol.kt` y en las fixtures de M3.0 (`ToolActivity`, `PermissionRequest/Reply/Resolved`, `Snapshot.permissions`). M4 no añade campos. Si la implementación necesita uno, va en el mismo commit en Go, Kotlin y fixtures (protocolo §10) y se avisa al supervisor (`protocol/` es suyo).

## 9. Fallos y casos límite
- Daemon caído / hook sin respuesta: Claude Code pregunta en la terminal (§3.3).
- Móvil desconectado a mitad de un permiso: la petición sigue viva hasta 600 s; al reconectar llega en el `snapshot` y se contesta.
- Dos `PermissionRequest` a la vez: dos tarjetas, cada una con su id; cada hook espera la suya.
- `Stop` sin texto (solo herramientas): no hay burbuja nueva.
- Texto con markdown roto o enorme: se corta (host) y se pinta como texto (app); nunca lanza.
- Transcript reescrito/rotado (`/compact`): vuelve a empezar por el final (`compact`) sin duplicar (upsert por `id`).
- Varias líneas con el mismo `message.id` en `chat.message` ya enviado: upsert, el `snapshot` guarda el último.

## 10. Tests
- **Módulo:** repetición de las grabaciones reales (`testdata/cc-2.1.286`: payloads de los 7 eventos y 2 transcripts) y casos hechos a mano (línea parcial, truncado, línea enorme, tipos desconocidos, versión desconocida, `thinking`, sidechain). Cada afirmación del formato lleva su caso, como en el clasificador.
- **Adaptador:** hooks simulados con `bridge` falso: ciclo completo de permisos (phone/terminal/timeout/SessionEnd), tope de 8, `allow_always` degradado a `allow` si `allowAlways` es false, que `Stop` sin transcript emite respaldo, que `chat.send` da `busy` con permiso pendiente, que el shim no se salta (§5.3).
- **`adbuddy hook`:** falla abierto (daemon caído, JSON roto, respuesta rota → vacío y 0), nunca sale con 2.
- **Real (manual, con `ADBUDDY_REAL_CLAUDE=1`):** `claude -p` en un entorno temporal con hooks reales contra el daemon y un móvil falso: burbujas, `tool.activity`, `allow`/`deny`/`allow_always` y que la regla exacta no se amplía (§5.3). No toca el móvil del usuario ni `~/.claude`.
- **App:** `Markdown` (HTML, imágenes, enlaces `javascript:`), `ChatReducer` (actividad, permisos, snapshot, resets), JVM.
- **CI:** los tests con goroutines suben a la CI de ubuntu con `-race` (no corre en el PC).

## 11. Validaciones manuales pendientes (no las corre el agente)
`Notification` y sus tipos; permisos de Edit/Write/WebFetch/MCP; deny con `message`; qué hace Claude Code con el hook si se responde en la terminal (§5.2); `/clear`, `/compact`, `--resume`; prompts con `/` y `!`; cómo se ve todo en el móvil real.

## 12. Decisiones
- **Ruling:** hooks de solo seis eventos, sin `PreToolUse`/`PostToolUse` — cada hook es un proceso por herramienta y el transcript ya las trae — si me equivoco, una latencia de "running" mayor y se añade `PostToolUse`.
- **Ruling:** "Sí, siempre" solo para Bash, regla exacta y sin adb/shim — es una regla persistente en `settings.local.json` y el supervisor exige que nunca toque el móvil ni 🔴 — si me equivoco, la persona repite un "Sí" más.
- **Ruling:** `adbuddy hook` falla abierto y nunca sale con 2 — un hook roto no puede bloquear a Claude Code ni dejarlo sin respuesta — si me equivoco, un permiso que debía ir al móvil cae a la terminal.
- **Ruling:** sin móvil conectado, `PermissionRequest` no espera — la persona está en el PC — si me equivoco, un permiso se queda sin botones en el móvil que acaba de conectar (y lo resuelve en el PC).
- **Ruling:** el id del permiso lo pone el daemon — el payload no trae `tool_use_id` (spike).
- **Ruling:** sin identidad del par del socket en M4 (ver §4.3) — el hook solo puede pedir, no conceder — si me equivoco, un agente hostil molesta con tarjetas falsas (acotado a 8).
- **Ruling:** las preguntas 🔴 no pasan a tarjetas (§7.5).
- **Ruling:** el lector empieza por el final en `resume`/`compact` — no se reenvía historia — si me equivoco, el chat arranca vacío tras `--continue` (la historia vive en la terminal).
