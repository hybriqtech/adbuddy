# SDD ledger — plan: docs/superpowers/plans/2026-10-01-adbuddy-m6b-skills.md

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

## Pre-flight (la revisión opus del plan hizo la réplica acumulada; filas de pares)

| Par / tarea | Productor → consumidor | Resultado |
|---|---|---|
| T1 → T2 | `Skills()`, `SkillNames()`, `requiredSkills` | T2 consume `Skills()`; coincide |
| T1 → T3 | `Skills()` | coincide |
| T2 → T9 | `skillExamples`, `splitExample` | mismo paquete `cli`; coincide |
| T3 → T4 | `ensureRealDirs`, `Result.note`, `loadSettings` | T4 usa `ensureRealDirs(o.Dir, ".gemini")`; coincide |
| T1..T4 (cada una sola) | réplica opus | correcciones aplicadas (7ea5eb1): YAML con comillas, test `.claude/` |
| T5..T8 | `requiredSkills` en el mismo fichero | en serie |
| T5..T9 | niveles reales vs briefs | corregidos según la revisión; el test de T2 es la autoridad |

Rulings de proceso: implementadores `adbuddy-implementer` (Sonnet); revisores según el reparto del plan (opus en T2, T3, T5-T8 y final).

Task 1: complete (commits 7ea5eb1..981c5cd, review clean)
Task 1: minor (deferred): lint usa t.Fatalf en `frontmatter` (un SKILL.md roto esconde los demás; pasar a t.Errorf+continue o t.Run); `quotedDesc` busca en todo el fichero, no solo en el frontmatter; len(desc) cuenta bytes; sin caso negativo del lint.

Task 2: review opus (26958c5): spec ✅, calidad "Necesita arreglos" — 4 importantes (metacaracteres del shell del PC sin comillas, bloques esquivables, recetas prohibidas esquivables, sin pruebas negativas) + 3 menores
Task 2: fix round 1/5 (7 addressed por el implementador, re-revisión PENDIENTE; commits 26958c5..b82d05c)
Task 2: Ruling: el test prohíbe `rm` con `-r`/`--recursive` (más estricto que el plan) — el implementador lo eligió y es coherente con §2.3 — CONFLICTO con el brief de T6 (`storage-cleanup` vacía la papelera con `rm -r /sdcard/ADBuddy-papelera`, 🔴). Decidir en T6: o la guarda admite exactamente `rm -r /sdcard/ADBuddy-papelera` (lista blanca explícita en el test) o el brief cambia a `rm` de ficheros uno a uno (🔴 cada uno); recomiendo la lista blanca de esa única orden — cuesta una excepción en la guarda.
PAUSA 2026-10-01 (supervisor adbuddy-e5: ventana de 5 h al 21% en 45 min, proyección 140%): sin subagentes nuevos hasta que él reanude por SendMessage (proyección <70% o reinicio de las 02:50).
