# Ledger — /simplify de todo el proyecto

Plan: docs/superpowers/plans/2026-10-21-simplify.md. Rama refactor/simplify (base 26e058c9).

## Revisión (2026-10-21)
4 revisores Sonnet en solo lectura: reuse (25 hallazgos), simplification (18), efficiency (19), altitude (13). Deduplicados en las tareas H1-H5 y A1-A2 del plan.

Ruling: quedan fuera los refactors de las puertas de exec/ui/undo, la tabla de inversos, percentDecode, pathguard.Resolve, AccessSwitch, el guard sin app, el arranque de phoneapp, los perfiles de agente, serialQueue, el troceo de setup/serve, isDeviceLocked en ConfirmHost, la copia del árbol a11y, Matcher y la doble serialización de terminal/journal/bridge — porque o tocan el orden de las puertas de seguridad, o cambian conducta (percentDecode diverge hoy: va a /code-review), o son rediseños de más de dos paquetes — si me equivoco, quedan pendientes limpiezas de valor, que siguen anotadas en el plan.

## Tareas
- Plan revisado por opus (6 quitar, 10 ajustar, 2 añadir) y corregido. Ruling: H5 entero fuera (ReadTCP/Collect, Scale, sameScreen no equivalentes); chmod tolerante solo donde ya lo era (TolerateChmod); causeError fuera (cambia el fix en msgs()) — porque esto es un refactor sin cambios de conducta — si me equivoco, se pierden ahorros menores.
