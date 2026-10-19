# ADBuddy

Propósito: app open source para chatear desde el móvil con un agente de código del PC (Claude Code, Codex, Gemini CLI…) y dejar que gestione el Android por adb Wi-Fi de forma segura (clasificación de comandos, diario de cambios, confirmaciones en el móvil).

Cuenta GitHub: hybriqtech (remoto por alias SSH `github.com-hybriqtech`)

Diseño v1: `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md`

Hoja de ruta: §13 de la spec. Pendientes y notas para hitos futuros: `docs/superpowers/plans/pendientes-m3-m5.md`.

## Forma de trabajar

- **Proceso:** cada hito tiene spec → plan (`superpowers:writing-plans`) → ejecución con `superpowers:subagent-driven-development`. Ledger en `.superpowers/sdd/<plan>/progress.md`, con cada decisión como `Ruling: qué — por qué — qué cuesta si me equivoco`. Estado del líder en `C:/Users/Jose/orca/workspaces/adbuddy/ESTADO.json`.
- **Revisión del plan antes de ejecutarlo:** un `adbuddy-reviewer-opus` revisa el plan completo (código incluido) frente a la spec antes de la tarea 1, y el plan se corrige con sus hallazgos. Es obligatoria porque la mayoría de los fallos que encuentran las revisiones de tareas ya estaban en el código del plan.
- **Cada tarea compila y pasa sus tests por sí sola** (réplica acumulada tarea a tarea antes de dar el plan por bueno); ninguna tarea usa clases de otras posteriores.
- **Revisores:** `adbuddy-reviewer-opus` para seguridad (clasificador, shim, IPC, diario, confirmaciones, canal y token, app) y para la revisión final; `adbuddy-reviewer` para el resto; `adbuddy-rereviewer` para re-revisiones acotadas de diffs pequeños. Pasa a los revisores límites de tiempo (`go test -timeout`, tope de minutos): uno se colgó más de una hora.
- **Límite de rondas:** 5 por tarea; al llegar, el controlador adjudica. Un hueco que deja ejecutar 🔴 sin confirmación no se aparca nunca.
- **Paralelismo:** implementadores de `host/` y de `android/` pueden ir a la vez (árboles disjuntos, cada uno añade solo sus rutas); dos implementadores en el mismo árbol, no.
- **Coste:** el 80 % del gasto son lecturas de caché, o sea contexto largo releído en cada llamada. Mantén los contextos cortos: sesiones y compañeros por tarea, que se apagan al acabar, y `autoCompactWindow` a 300k. Implementador Sonnet (`adbuddy-implementer`) por defecto; `adbuddy-implementer-opus` solo en las rondas 4-5 o con un ruling escrito de concurrencia/IPC en el plan. Revisor opus solo para seguridad y la revisión final; el resto con `adbuddy-reviewer` o `adbuddy-rereviewer`.
- **Commits:** identidad local hybriqtech, trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` (los implementadores a veces ponen otro: compruébalo). Push solo de ramas como copia, tras `git grep` de seriales e IPs; sin `--force`; a `main` integra el usuario.
- **Clasificador (`host/internal/classify`):** lista blanca (M2.5). El evaluador de shell solo admite lo que lista (construcciones, expansiones, builtins y variables propias en minúsculas); todo lo demás es 🔴 especial. No des nada sobre mksh por sabido de memoria: cada afirmación lleva un caso en `testdata/differential.jsonl` o en el generador, grabado contra el mksh del emulador en `testdata/oracle.jsonl` (`scripts/emulator.sh` y `TestRecordOracle`; ver `host/README.md`). Ampliar la lista blanca es añadir el caso al evaluador, sus usos al generador y regrabar. `testdata/habitual.jsonl` es el contrato de lo que no puede subir.
- **Validaciones en dispositivo:** lo que no se puede probar sin móvil se apunta en la sección "Validaciones manuales pendientes" del plan. No pruebes nada que modifique el móvil del usuario sin su permiso explícito; los tests usan un adb falso y, para Android, un emulador.

## Hooks de git (prek)

Los hooks de commit (gofmt, builtins) y de push (`go vet`, `go test` sin `internal/cli`) están descritos en `docs/hooks.md`. Si un hook para el commit, arregla y repite; no uses `--no-verify`. La CI no cambia.

## Compactación

Al resumir una sesión larga, conserva:

- Ruta del ledger activo (`.superpowers/sdd/<plan>/progress.md`), de `C:/Users/Jose/orca/workspaces/adbuddy/ESTADO.json` y del relevo, si lo hay (`C:/Users/Jose/orca/workspaces/adbuddy/handoffs/`): tras compactar, reléelos antes de seguir.
- Plan, tarea y paso actuales, con la ronda de revisión de cada tarea abierta y los agentes o compañeros en marcha (rama, worktree).
- Ficheros y ramas tocados, y commits aún sin integrar.
- Decisiones vigentes con su porqué (los `Ruling:` del ledger), sin las ya superadas.
- Comandos de test usados y su último resultado.
- Validaciones en dispositivo pendientes y lo que espera respuesta o permiso del usuario.
