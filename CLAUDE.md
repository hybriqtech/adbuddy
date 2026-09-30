# ADBuddy

Propósito: app open source para chatear desde el móvil con un agente de código del PC (Claude Code, Codex, Gemini CLI…) y dejar que gestione el Android por adb Wi-Fi de forma segura (clasificación de comandos, diario de cambios, confirmaciones en el móvil).

Cuenta GitHub: hybriqtech (remoto por alias SSH `github.com-videal`)

Diseño v1: `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md`

Hoja de ruta: §13 de la spec. Pendientes y notas para hitos futuros: `docs/superpowers/plans/pendientes-m3-m5.md`.

## Forma de trabajar (aprendido en M2)

- **Proceso:** cada hito tiene spec → plan (`superpowers:writing-plans`) → ejecución con `superpowers:subagent-driven-development`. Ledger en `.superpowers/sdd/<plan>/progress.md`, con cada decisión como `Ruling: qué — por qué — qué cuesta si me equivoco`.
- **Revisión del plan antes de ejecutarlo (obligatoria).** Un `adbuddy-reviewer-opus` revisa el plan completo (código incluido) frente a la spec antes de la tarea 1, y el plan se corrige con sus hallazgos. En M2, casi todo lo que encontraron las revisiones de la app (T8-T10) ya estaba en el código del plan.
- **Cada tarea compila y pasa sus tests por sí sola** (réplica acumulada tarea a tarea antes de dar el plan por bueno); nada de tareas que usen clases de otras posteriores.
- **Revisores:** `adbuddy-reviewer-opus` para seguridad (clasificador, shim, IPC, diario, confirmaciones, canal y token, app) y para la revisión final; `adbuddy-reviewer` para el resto; `adbuddy-rereviewer` para re-revisiones acotadas de diffs pequeños. Nunca modelos Haiku. Pasa a los revisores límites de tiempo (`go test -timeout`, tope de minutos): uno se colgó más de una hora.
- **Límite de rondas:** 5 por tarea; al llegar, el controlador adjudica. Un hueco que deja ejecutar 🔴 sin confirmación no se aparca nunca.
- **Paralelismo:** implementadores de `host/` y de `android/` pueden ir a la vez (árboles disjuntos, cada uno añade solo sus rutas); dos implementadores en el mismo árbol, no.
- **Commits:** identidad local hybriqtech, trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` (los implementadores a veces ponen otro: comprobarlo). Push solo de ramas como copia, tras `git grep` de seriales e IPs; nunca `--force`; a `main` integra el usuario.
- **Clasificador (`host/internal/classify`):** lista blanca (M2.5). El evaluador de shell solo admite lo que lista (construcciones, expansiones, builtins y variables propias en minúsculas); todo lo demás es 🔴 especial. Nada sobre mksh se da por sabido "de memoria": cada afirmación lleva un caso en `testdata/differential.jsonl` o en el generador, grabado contra el mksh del emulador en `testdata/oracle.jsonl` (`scripts/emulator.sh` y `TestRecordOracle`; ver `host/README.md`). Ampliar la lista blanca es añadir el caso al evaluador, sus usos al generador y regrabar. `testdata/habitual.jsonl` es el contrato de lo que no puede subir.
- **Validaciones en dispositivo:** lo que no se puede probar sin móvil se apunta en la sección "Validaciones manuales pendientes" del plan. Nunca se prueba nada que modifique el móvil del usuario sin su permiso explícito; los tests usan un adb falso y, para Android, un emulador.
