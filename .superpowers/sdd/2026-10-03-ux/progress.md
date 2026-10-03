# Ledger UX (rama ux/revision)

Encargo: revisión completa de UX (app Android + CLI `adbuddy`) y aplicar mejoras. Fase 1 = revisión solo lectura -> docs/superpowers/plans/2026-10-03-ux-revision.md.

## Estado
- Fase 1: en curso. 3 revisores de solo lectura lanzados (ux-rev-chat, ux-rev-onboarding, ux-rev-cli), areas disjuntas.

## Rulings
- Ruling: 3 revisores adbuddy-reviewer (Sonnet) en paralelo, uno por area (chat/confirm/notify/term/files; onboarding/ajustes/permisos/a11y; CLI host) — el encargo permite 2-3 y asi no inundo mi contexto — cuesta: hallazgos superficiales; mitigo verificando el top 10 yo mismo antes de enviarlo.

## Fase 1 terminada
- 3 revisiones recibidas (C1-C27, O1-O21, H1-H16). Informe: docs/superpowers/plans/2026-10-03-ux-revision.md. Top 10 re-verificado por mi (C2, O1, O5, O6 leidos en codigo).
- Ruling: C2 queda como "S, SEG": el diseño actual (la notificacion reabre el dialogo, MainActivity.kt:85) es intencionado; la mejora añade un banner que reabre la misma tarjeta, sin cambiar semantica — si me equivoco: un banner mal hecho podria dar un atajo; por eso plan con revisor opus.
- Ruling: C8/C9/C10/O9/O12/O21 fuera del top aunque baratos y agrupados en P5 (solo con opus) — tocan consentimiento o semantica visible de confirmacion — cuesta: mejoras de seguridad percibida tardan mas.
- Ruling: H14 (platformtools) no se toca sin avisar al dueño del paquete — fuera de mi arbol — cuesta: mensaje de descarga sigue en ingles.
- Esperando OK del lider a la lista antes de la fase 2 (plan con superpowers:writing-plans).
