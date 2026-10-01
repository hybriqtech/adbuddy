# Spike M4 · Qué escribe y espera Claude Code 2.1.286

Fecha: 2026-10-01. Claude Code 2.1.286 en el PC (Windows). Grabado en un directorio aislado con su propio `.claude/settings.json` (hooks que vuelcan stdin a ficheros) y `claude -p` mínimos, sin tocar la configuración del usuario. Grabaciones saneadas (sin rutas personales) en `host/internal/claudecode/testdata/cc-2.1.286/`: `hooks/` (payloads) y `transcript-*.jsonl` (transcripts recortados: sin `attachment`, `hookInfos` vacío, firmas de `thinking` sustituidas).

Todo lo de abajo está observado, no supuesto. Lo que NO se pudo observar sin TUI interactiva va en "Sin observar".

## Payloads de hooks (stdin, JSON)
Campos comunes: `session_id`, `transcript_path`, `cwd`, `hook_event_name`. Los de turno añaden `prompt_id`, `permission_mode`, `effort.level` (no en `SessionStart`/`SessionEnd`).

| Evento | Campos propios observados |
|---|---|
| `SessionStart` | `source` (`startup`) |
| `UserPromptSubmit` | `prompt` |
| `PreToolUse` | `tool_name`, `tool_input`, `tool_use_id` |
| `PermissionRequest` | `tool_name`, `tool_input`, `permission_suggestions[]` (p. ej. `{"type":"addDirectories","directories":[…],"destination":"session"}`); **sin `tool_use_id`** |
| `PostToolUse` | `tool_name`, `tool_input`, `tool_response` (Bash: `stdout`, `stderr`, `interrupted`, `isImage`…), `tool_use_id`, `duration_ms` |
| `Stop` | `stop_hook_active`, `last_assistant_message`, `background_tasks[]`, `session_crons[]` |
| `SessionEnd` | `reason` (`other`) |

- `transcript_path` viene en **todos**, y es una ruta Windows con `\\`. **Si el fichero ya existe en `SessionStart` no se comprobó** (no se miró): el lector tolera que falte y reintenta.
- `permission_mode` lo hereda de la configuración global del usuario (aquí `auto`); con `--permission-mode default` el Bash fuera de la lista pasa por `PermissionRequest`. En modo `auto` **no hay `PermissionRequest`**.
- `PermissionRequest` llega **después** de `PreToolUse` de la misma herramienta (~100 ms).
- En `claude -p` (sin TUI) el hook `PermissionRequest` se ejecuta igual; sin decisión del hook, el permiso se deniega y el modelo lo cuenta.

## Respuesta a `PermissionRequest` (stdout del hook, exit 0)
Observado que funciona:
```json
{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow",
 "updatedPermissions":[{"type":"addRules","rules":[{"toolName":"Bash","ruleContent":"echo tres:*"}],
 "behavior":"allow","destination":"localSettings"}]}}}
```
- El comando se ejecutó (`allow`).
- `updatedPermissions` con `destination:"localSettings"` hizo que **Claude Code escribiera él mismo** `.claude/settings.local.json` con `permissions.allow: ["Bash(echo tres:*)"]`. Así "Sí, siempre" no necesita que ADBuddy edite el JSON del usuario: devuelve la regla en la respuesta. `ruleContent` es la sintaxis de regla de Claude Code (`prefijo:*`).
- **Regla exacta verificada:** con `Bash(touch uno.txt)` en `settings.local.json`, `touch uno.txt` se ejecuta; `touch uno.txt dos.txt` y `touch uno.txt && touch tres.txt` se deniegan. `echo` simple no sirve de prueba (siempre permitido).
- Sin decisión (hook sin salida): denegado (ver arriba). `deny` con `message`: documentado por Anthropic, **no verificado aquí**.

## Transcript JSONL (`transcript_path`)
Una línea JSON por entrada; `type` ∈ `queue-operation`, `attachment`, `user`, `assistant`, `system`, `last-prompt`, `cost-state`, `atis-latch`… (la lista crece con la versión: el parser ignora lo desconocido).
- **`user`**: `message.role=user`, `message.content` = **cadena** (prompt) o **lista** con bloques `tool_result` (`tool_use_id`, `content`, `is_error`). Los `tool_result` van con `type:"user"`: no son mensajes del usuario. Campo extra `toolUseResult` (objeto por herramienta). `promptId` enlaza con el hook. Si el permiso se deniega: `toolDenialKind`.
- **`assistant`**: `message.id` (`msg_…`), `message.content` = lista de bloques `thinking` (texto vacío + `signature`), `text`, `tool_use` (`id`, `name`, `input`). **Un mismo mensaje del modelo se escribe en varias líneas** (una por bloque) con el mismo `message.id` y `uuid` distinto: el `id` de `chat.message` sale de `message.id` (upsert, contrato de M3).
- `message.stop_reason`: `tool_use` | `end_turn`. `timestamp` ISO en cada línea. `isSidechain` (subagentes) presente.
- **Orden `Stop` / transcript: sin comprobar.** El adaptador no puede asumir que el último mensaje ya esté escrito al llegar `Stop`; de ahí `last_assistant_message` como respaldo y la relectura tras `Stop`.
- Codificación UTF-8; las líneas pueden ser grandes (cientos de KB con adjuntos): el lector no usa un tope de línea fijo pequeño.

## Consecuencias para el diseño
1. `PermissionRequest` no trae `tool_use_id`: el id de `permission.request` lo genera el daemon.
2. "Sí, siempre" = `updatedPermissions` en la respuesta (o, si se prefiere control propio, `PermissionRequest.permission_suggestions` reenviadas al móvil como opciones). Decisión en el spec.
3. Nada de `PermissionRequest` en modo `auto`/`bypassPermissions`: la UI de permisos solo aparece cuando Claude Code pregunta de verdad.
4. El transcript es la fuente de burbujas y `tool.activity`; los hooks, la del estado y los permisos.

## Sin observar (hay que grabarlo en una sesión real en el PTY de ADBuddy, en la validación del hito)
- `Notification` (`permission_prompt`, `idle_prompt`) y cuándo se emite.
- `PermissionRequest` de herramientas que no son Bash (Edit, Write, WebFetch, MCP) y sus `permission_suggestions`.
- Respuesta `deny` con `message` e `interrupt`.
- Qué pasa si el usuario responde en la TUI mientras el hook espera (¿Claude Code cancela el hook? ¿se le mata el proceso?): decide cómo se emite `permission.resolved by=terminal`.
- Orden `Stop` / último volcado del transcript, y `/clear`, `/compact`, `--resume` (nuevo `session_id`, `transcript_path`).
- Subagentes (`isSidechain`, `SubagentStop`).
