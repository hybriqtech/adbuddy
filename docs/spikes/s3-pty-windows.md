# Spike S3 · Claude Code dentro de ConPTY desde Go (Windows)

> Spike desechable. Windows 11, Go 1.27.0 (winget), `github.com/UserExistsError/conpty` v0.1.4, Claude Code 2.1.284.

## Resultados

Ejecución definitiva (2026-09-29 20:50, 37 s), con `main.go` tal como está en el repo:

| Check | Resultado | Detalle |
|---|---|---|
| C1 arranque | PASS | 3292 bytes; diálogo de confianza: true |
| C2 bracketed paste activo | PASS | busca ESC[?2004h |
| C3 una línea + Unicode | PASS | esperado "ÑANDÚ42" |
| C4 multilínea | PASS | esperado "TRES" |
| C5 tras redimensionar a 80x30 | PASS | esperado "AZUL" |
| C6 Esc interrumpe | PASS | busca 'Interrupted' tras enviar Esc |
| C7 colores | PASS | busca ESC[38;5; o ESC[38;2; |
| C8 /exit termina | PASS | código 0, error <nil> |

Primera ejecución (2026-09-29 20:40), con el `main.go` original del plan, que en el diálogo de confianza solo enviaba `\r`:

| Check | Resultado | Detalle |
|---|---|---|
| C1 arranque | PASS | 1374 bytes; diálogo de confianza: true |
| C2 bracketed paste activo | PASS | busca ESC[?2004h |
| C3 una línea + Unicode | FAIL | esperado "ÑANDÚ42" |
| C4 multilínea | FAIL | esperado "TRES" |
| C5 tras redimensionar a 80x30 | FAIL | esperado "AZUL" |
| C6 Esc interrumpe | FAIL | busca 'Interrupted' tras enviar Esc |
| C7 colores | PASS | busca ESC[38;5; o ESC[38;2; |
| C8 /exit termina | PASS | código 1, error <nil> |

En el diálogo de confianza, la opción marcada por defecto es `❯ No, exit`. El Enter la eligió y Claude Code terminó con código 1 antes de recibir ningún prompt. C3-C6 fallaron por eso y no por ConPTY. C2 y C7 pasaron solo con lo que se pintó hasta entonces. El `raw.log` de esa ejecución (2568 bytes) muestra `ESC[?2004h` al arrancar y `ESC[?2004l` al salir. Corrección mínima en `main.go`: enviar flecha abajo (`ESC[B`), esperar 300 ms y pulsar Enter (se elige "Yes, I trust this folder").

Nota sobre C1: se mide antes de responder al diálogo de confianza, así que da PASS aunque Claude salga justo después, como en la ejecución 1. La viabilidad no se apoya en C1, sino en C3-C6.

## Observaciones

- **Diálogo de confianza: sí aparece** en una carpeta nueva ("Quick safety check: Is this a project you created or one you trust?"), y **la opción por defecto es "No, exit"**. Si el host envía un Enter a ciegas, la sesión se cierra. Dato para §5.1: el host tiene que preconfiar la carpeta antes de lanzar Claude Code o detectar el diálogo y elegir "Yes" de forma explícita. *Inferencia, no verificado:* la confianza se guarda en la configuración del usuario. Después de aceptar el diálogo no encontré la ruta de `work/` en `~/.claude.json` ni en `~/.claude/*.json`. *Pista, también inferencia:* `~/.claude.json` guarda los proyectos por la raíz git, con barras `/` (`projects["C:/Users/Jose/Documents/projects/adbuddy"]`, que tiene `hasTrustDialogAccepted: true`), no por la ruta de `work/`, y aun así el diálogo salió en `work/`. Queda pendiente para el plan de M3/M4 averiguar dónde y con qué clave guarda Claude Code 2.1.284 esa confianza, porque de eso depende cómo preconfiar desde el host. Efecto lateral: la sesión del spike sobrescribió las estadísticas `last*` del proyecto adbuddy en `~/.claude.json` (`lastCost`, `lastAPIDuration`, etc.).
- **Colores: sí.** Truecolor (`ESC[38;2;r;g;bm`) desde la primera pantalla. Además hay hipervínculos OSC 8 (`ESC]8;id=…;https://code.claude.com/docs/en/security`), así que el renderizador de la app tendrá que ignorarlos o soportarlos.
- **Influyen dos cosas en la sesión: la configuración global del usuario y el entorno heredado.**
  - **Verificado, viene de `~/.claude/settings.json`:**
    - los hooks globales se ejecutan en cada turno ("running UserPromptSubmit hook" y "running Stop hook");
    - el modo de permisos es `defaultMode: auto` (`⏵⏵ auto mode on`);
    - el modelo es `model: opus` (la sesión muestra "Opus 5.5").
    - `settings.json` también define un hook `SessionStart`. No aparece texto suyo en la salida: probablemente se ejecutó sin mostrar nada.
  - **Inferencia, heredado del entorno:** `main.go` llama a `conpty.Start` sin `ConPtyEnv`, así que el hijo heredó el entorno de la sesión de Claude Code que lanzó el spike. Entre otras variables: `CLAUDECODE`, `CLAUDE_CODE_CHILD_SESSION`, `CLAUDE_CODE_SESSION_ID`, `CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_MESSAGING_*`, `CLAUDE_EFFORT` y `ANTHROPIC_DEFAULT_*_MODEL`. `settings.json` tiene `effortLevel: "medium"`, pero la sesión mostraba "xhigh effort": el esfuerzo casi seguro viene de `CLAUDE_EFFORT` heredado, y quizá también influyeron las variables de modelo.
  - Además aparecen consejos que mencionan skills ("try /code-review low. It runs the built-in skill…").

  Para §5.1: la sesión de ADBuddy recibe la configuración global del usuario y, si no se controla, el entorno del proceso que la lanza.
- **Codificación: sin problemas.** `ñandú` llegó bien en la entrada y la respuesta `● ÑANDÚ42` salió bien. Hay 0 caracteres U+FFFD en `plain.log`. Algunas palabras aparecen pegadas o con letras de menos en `plain.log` ("Cunlas líneas deeste mensaje"), pero no es un problema de codificación: al quitar las secuencias CSI se pierden los movimientos de cursor de los repintados parciales. Un host real necesita un emulador de terminal (p. ej. un VT en memoria) para reconstruir la pantalla, no un simple filtro de ANSI.
- **Multilínea:** con bracketed paste, las tres líneas llegaron como un solo mensaje (el prompt se muestra con sus 3 líneas y la respuesta es `● TRES`). No hace falta Alt+Enter.
- **Redimensionar:** tras `Resize(80, 30)`, Claude Code repinta la pantalla completa (vuelve a salir el banner) y sigue respondiendo (`● AZUL`).
- **Esc:** interrumpe la generación larga hacia el número 341 ("⎿ Interrupted · What should Claude do instead?").
- **`/exit`:** termina el proceso con código 0. En `Get-Process claude` no queda ningún proceso huérfano.
- Las respuestas de C3-C5 tardaron de 1 a 2 s ("Cooked for 2s"). La ejecución completa duró 37 s.

## Conclusión e impacto en la spec (§5.1 sesión, §5.4 entrada por bracketed paste)

**ConPTY desde Go es viable para §5.1.** C1, C3, C4, C5, C6 y C8 dan PASS con `UserExistsError/conpty` v0.1.4, así que no hace falta la alternativa `node-pty`. **§5.4 queda validada:** Claude Code activa `ESC[?2004h` y un pegado `ESC[200~…ESC[201~` seguido de `\r` envía como un único mensaje el texto multilínea con Unicode.

Impactos:

1. **§5.1:** gestionar la confianza de la carpeta antes de arrancar, porque el diálogo sale por defecto en "No, exit". Enviar Enter a ciegas cierra la sesión.
2. **§5.1, configuración global (verificado):** la sesión aplica los hooks, el modo de permisos (`auto`) y el modelo de `~/.claude/settings.json`. **Pregunta de producto abierta para la spec:** ¿ADBuddy respeta la configuración global del usuario (hooks, modo) o la aísla?
3. **§5.1, entorno controlado:** el host de ADBuddy debe lanzar el agente con un entorno explícito (p. ej. `ConPtyEnv`). Si el host se arranca desde otra sesión de agente, tiene que eliminar las variables heredadas `CLAUDECODE*`, `CLAUDE_CODE_*` y `CLAUDE_EFFORT`. Este spike no lo hizo, y el esfuerzo "xhigh" observado es, por inferencia, consecuencia de ello.
4. **Renderizado para el móvil:** la salida es una TUI con repintados parciales, truecolor y OSC 8. Hace falta un emulador de terminal en el host (o en la app) para obtener texto fiable; quitar las secuencias ANSI no basta.
