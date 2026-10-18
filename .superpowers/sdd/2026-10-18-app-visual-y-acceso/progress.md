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

### T1 — 5dc01e77 — CONFORME (opus, 1 ronda)
Tests: AccessDiagTest 3/3, AccessSwitchTest 10/10, NoHardcodedTextTest 3/3, StringsParityTest 4/4. Mutaciones (take(16), saltos de línea, null->raw:0, "-" por "0", quitar línea de i18n-exceptions) en rojo; el revisor las repitió en copia.
- Ruling: no endurecer el valor leído a `[0-9-]` (hallazgo menor 1) ni añadir el test de "log nunca lanza" (menor 2) — por qué: quien puede escribir el ajuste con adb ya puede leer el logcat, y `log` solo es diagnóstico (V1) que se quita o se deja al cerrar — qué cuesta si me equivoco: un valor raro de hasta 16 caracteres en logcat; si alguien quita el try/catch de `log` ningún test lo ve (el try/catch está a la vista en el código).
- Entorno: `gradle` no está en el PATH; se usa `C:\Users\Jose\AppData\Local\Gradle\gradle-9.8.0\bin` y `ANDROID_HOME=C:\Users\Jose\AppData\Local\Android\Sdk` (sin local.properties).
