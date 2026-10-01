# Handoff · M6b (pausa por presupuesto, 2026-10-01)

Retomar cuando adbuddy-e5 lo pida por SendMessage. Sesión del controlador: la misma si sigue viva; si no, leer este fichero y `progress.md`.

## Dónde estamos
- Rama `m6a` (M6b se apila y va en el mismo PR). Spec `docs/superpowers/specs/2026-10-01-adbuddy-m6b-skills-design.md`, plan `docs/superpowers/plans/2026-10-01-adbuddy-m6b-skills.md` (revisado por opus; correcciones en 7ea5eb1), ledger `progress.md` (esta carpeta).
- T1 completa (981c5cd, revisión limpia). T2: implementada (26958c5) y endurecida en la ronda 1 (b82d05c); **falta la re-revisión acotada** (`scripts/review-package PLAN 26958c5 HEAD` + `re-review-prompt.md`; hallazgos en `task-2-findings-r1.md`; revisor `adbuddy-rereviewer`, Sonnet; si la deja limpia, T2 completa). T3-T9 y la revisión final: pendientes.

## Siguiente paso exacto
1. Re-revisión acotada de T2 (hallazgos 1-7 de `task-2-findings-r1.md`; ¿ADDRESSED?, ¿rotura nueva en el diff?).
2. T3 (`task-brief PLAN 3`, implementador `adbuddy-implementer` Sonnet, revisor `adbuddy-reviewer-opus`), luego T4 (reviewer Sonnet), T5-T8 en serie (opus), T9 (Sonnet), revisión final opus, push y PR sin pedir ok (el merge lo hace el supervisor).

## A decidir al llegar a T6
La guarda de T2 prohíbe `rm` con `-r`; el brief de T6 usa `rm -r /sdcard/ADBuddy-papelera` para vaciar la papelera. Ver el Ruling en `progress.md` (recomendado: lista blanca de esa única orden en la guarda).

## Reglas (supervisor y proyecto)
Sonnet en sesión e implementadores; opus solo en plan, seguridad y revisión final; nunca Haiku; un subagente a la vez; una revisión por tarea con lógica; trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`; identidad hybriqtech; nunca `--no-verify` ni `--force`; `git grep` de seriales e IPs antes de push; `gh` con `GH_CONFIG_DIR="$HOME/.config/gh-hybriqtech"` y `-R hybriqtech/adbuddy`; los implementadores no tocan `.superpowers/`.
