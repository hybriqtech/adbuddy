# SDD ledger — plan: docs/superpowers/plans/2026-10-18-app-visual-y-acceso.md

Ledger de UX1 (aspecto visual de la app y estado del acceso), 2026-10-18. Compañero `ux`, rama `ux/visual` desde 01ce0672. Spec: `docs/superpowers/specs/2026-10-18-app-visual-y-acceso.md`.

## Entorno

- Un solo Gradle a la vez en este árbol; comandos Gradle con timeout <= 20 min; nada de emuladores ni adb contra el móvil del usuario (puerto 5038 intocable).

## Rulings

- R-ux1. Re-revisión del arreglo del plan (`adbuddy-rereviewer`, 01ce0672): nada bloquea; 5 hallazgos de documentación/tests corregidos en el plan y la spec antes de la tarea 1 (spec §7.2 con claves duplicadas y `restore_confirm_body` viejo; texto "te lleva a Opciones de desarrollador" que el código no hace → "te dice cómo hacerlo"; `LaunchedEffect(status)` después de `val status`; test nuevo `theActionCardButtonRefusesCoveredTouches` en `TouchGuardSourceTest`; `theRestoreDialogClosesWhenTheCutEnds` con segundo corte) — por qué: eran errores del plan que habrían dado `Unresolved reference` o un filtro de toques sin test — qué cuesta si me equivoco: nada que ejecutar; solo texto de plan.
- R-ux2. También aplicados los menores 6 (centrado del estado vacío con `Box` + `Column` con scroll) y 7 (`theTerminalStartsFolded` para R15, con mutación). No aplicado el 8 (acortar `status_waiting`) — por qué: la clave también alimenta el titular de la notificación (`LinkStatus.WAITING`, `ConversationViewTest`), recortarla cambiaría tests y notificación fuera del alcance — qué cuesta: la tarjeta WAITING repite "Buscando tu PC…" en el cuerpo (el título es otro).
- R-ux3. Observación abierta (no bloquea): con registro 0/0 y lecturas falsas, `restore()` devuelve false para siempre y el estado queda en CUT; lo cubre R3 (canal vivo gana) y V1. Se anota para el líder, no se cambia.

## Tareas

(se rellena al avanzar)
