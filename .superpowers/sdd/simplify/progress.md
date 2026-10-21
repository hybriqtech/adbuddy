# Ledger — /simplify de todo el proyecto

Plan: docs/superpowers/plans/2026-10-21-simplify.md. Rama refactor/simplify (base 26e058c9).

## Revisión (2026-10-21)
4 revisores Sonnet en solo lectura: reuse (25 hallazgos), simplification (18), efficiency (19), altitude (13). Deduplicados en las tareas H1-H5 y A1-A2 del plan.

Ruling: quedan fuera los refactors de las puertas de exec/ui/undo, la tabla de inversos, percentDecode, pathguard.Resolve, AccessSwitch, el guard sin app, el arranque de phoneapp, los perfiles de agente, serialQueue, el troceo de setup/serve, isDeviceLocked en ConfirmHost, la copia del árbol a11y, Matcher y la doble serialización de terminal/journal/bridge — porque o tocan el orden de las puertas de seguridad, o cambian conducta (percentDecode diverge hoy: va a /code-review), o son rediseños de más de dos paquetes — si me equivoco, quedan pendientes limpiezas de valor, que siguen anotadas en el plan.

## Tareas
- Plan revisado por opus (6 quitar, 10 ajustar, 2 añadir) y corregido. Ruling: H5 entero fuera (ReadTCP/Collect, Scale, sameScreen no equivalentes); chmod tolerante solo donde ya lo era (TolerateChmod); causeError fuera (cambia el fix en msgs()) — porque esto es un refactor sin cambios de conducta — si me equivoco, se pierden ahorros menores.
- H1 6eaf3377: CONFORME (reviewer, ronda 1; mutación del Remove en rojo).
- A1 72107008: CONFORME en conducta (reviewer). Menores (startFirst a su propio fichero e internal; orden de imports) — Ruling: se arreglan dentro de A2 en lugar de otra ronda — si me equivoco, un commit de A2 algo más mezclado.
- H2 9a3a3af7: CONFORME (reviewer; GOOS=windows vet OK). Menor: reflujar comentarios largos en common.go y remotecmd.go → se hace en H3.
- A2 3ca38e23+6862cef6: CONFORME (opus; 4 mutaciones en rojo; umbral 7→3 justificado: quedan 3 contenedores + users>=9). Ruling: completar isConnected en los 11 sitios restantes (no YoloProtection) y cerrar ::AlertDialog en el test, en un commit corto — coherencia con el plan — si me equivoco, un commit pequeño de más.
- H3 069267bf: CONFORME (opus). Menor: comentario de callHelper desplazado en helpers.go:42-47 → se arregla en H4.
- H4 a4af79df: CONFORME (opus; movimiento byte a byte; fuzz rojo con 2 de 3 mutaciones). Ruling: añadir test de los 256 bytes de Unquote (rechazo de lo permisivo) — barato y cierra la dirección que importa al undo — si me equivoco, un test de más.
- A2 remate 0525440f: CONFORME (rereviewer; BubbleFlowTest 5/5 y suite 1432/0; el fallo intermitente 'performMeasureAndLayout' no se atribuye a A2, se vigila en la CI).
- H4 remate fd8d1ec2: test de 256 bytes, mutación en rojo.
- Revisión final opus: INTEGRABLE (host 34 paquetes ok, GOOS=windows vet limpio; android 1432/0, lint y androidTest compilan; sin IPs ni seriales). Diferidos menores: startFirst en ui/ usado por notify/ y listener/ (moverlo a util), TextLinkStyles sin remember en MarkdownText.kt:52. Pendiente: PR + CI con emulador.
