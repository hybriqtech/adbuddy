# Ledger · M6b skills de contexto

Plan: `docs/superpowers/plans/2026-10-01-adbuddy-m6b-skills.md` · Spec: `docs/superpowers/specs/2026-10-01-adbuddy-m6b-skills-design.md` · Rama: `m6a` (M6b va en el mismo PR).
Supervisor: adbuddy-e5 (2026-10-01 21:50: Sonnet para sesión e implementadores; opus solo en revisión del plan, seguridad y revisión final; un subagente a la vez; tras revisión final limpia, push y PR sin pedir ok; el merge lo hace el supervisor).

## Rulings

- Ruling: M6b = solo las nueve skills (+ instalación, Gemini, tests); i18n, README completo y `adbuddy update` quedan fuera — el handoff y la spec de M6a lo separan, y así cabe en un PR revisable — cuesta un hito más para la i18n.
- Ruling: skills en inglés, ejemplos anotados con el nivel del clasificador y comprobados por test — es la única forma de que "recetas probadas" sea verdad sin móvil — cuesta acoplar skills y clasificador (se rompe en CI, que es lo buscado).
- Ruling: la aprobación de la spec y del plan la da el supervisor por delegación del usuario (memoria `trabajar-autonomamente`) — el usuario fijó el alcance y no interviene — cuesta que el usuario vea la spec solo en el PR.
- Ruling: se aplican los 25 hallazgos de la revisión opus del plan (4 bloqueantes: YAML sin comillas, test existente de `.claude/`, `mv` a carpeta existente = 🔴, `mv -n`/`--help` = 🔴; los demás de niveles reales, `build-utility-apk` como vía alrededor del shim, privacidad, test de flags, T2/T3/T6 con revisor opus) — todos eran verificables en el código real — cuesta más texto en los briefs.

## Tareas

| T | Estado | Notas |
|---|---|---|
| T1 | pendiente | |
| T2 | pendiente | |
| T3 | pendiente | |
| T4 | pendiente | |
| T5 | pendiente | |
| T6 | pendiente | |
| T7 | pendiente | |
| T8 | pendiente | |
| T9 | pendiente | |
| Final | pendiente | revisión opus de la rama, push y PR |
