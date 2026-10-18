# Fragmento: aspecto visual y estado del acceso (rama ux/visual)

Cierre del plan `2026-10-18-app-visual-y-acceso.md`. Las filas de aquí las integra el controlador en `pendientes-m3-m5.md`.

## Pendientes

| Id | Qué | Nota |
|---|---|---|
| P4 | Icono de la app | Hecho (icono adaptativo y monocromo; iconos propios por tipo de tarjeta). |
| Bug 17 | `Toast` del consentimiento | Aparcado por R13. |
| UX-a | El nivel de cada entrada del diario solo se distingue por color en la barra (más su nombre para TalkBack) | En modo claro rojo/ámbar/verde tienen casi la misma luminancia. Mejora posible: texto o icono de nivel visible, o ancho de barra distinto. |
| UX-b | Sanear `SessionState.agent` (caracteres bidi/control, longitud) | Se pinta en el título de la tarjeta y en la cabecera. |
| UX-c | El bloque del comando del diálogo 🔴 en modo claro apenas se distingue del fondo (`surfaceContainerHighest`) | Subir contraste o añadir borde. |
| UX-d | Falta captura de un error largo del host en los chips de borrador | |
| UX-e | El recuento de `ConfirmDialogScreensTest` no detecta un bloque recortado sin scroll | Lo cubren los tests protegidos (`TouchGuard*`, `ConfirmReopenTest`, `ConfirmDialogButtonsSourceTest`). |

## Inventario (ficheros nuevos principales)

- `control/AccessDiag` (diagnóstico `ADBuddyAccess`), `a11y/OverlayColors` (colores fijos de la superposición).
- `ui/theme/`: `Palette`, `Theme`, `AdbuddyColors`, `Type`, `Shapes`, `ThemePrefs`.
- `ui/`: `HomeNotice`, `ActionCard` (+ `cardIconRes`), `StatusPill`, `RestoreAccessDialog`, `ChatHeaderBar` (en `ChatParts.kt`).
- Iconos: `ic_shield`, `ic_shield_off`, `ic_card_question`, `ic_card_pause`, `ic_card_update`, `ic_launcher_foreground`, `ic_launcher_monochrome`.
- Capturas en `android/app/src/test/screenshots/` (`-Padbuddy.screens=record|verify`).

## Validaciones manuales (exigen el móvil real; con permiso explícito del usuario)

- **V1. Diagnóstico de "cortado".** En el Pixel (Android 17, app 0.2.0) `adb shell` da `adb_enabled=1` y `adb_wifi_enabled=1` y la app decía "cortado". Con la build de la tarea 1 instalada (instalar modifica el móvil: pedir permiso), `adb logcat -d -s ADBuddyAccess` junto a `adb shell settings get global adb_enabled` / `adb_wifi_enabled`. Si la app lee `raw:0` o un error con `adb shell` diciendo `1`, Android 17 da un valor falso a la app: la regla de la tarea 2 es la definitiva, y si la relectura de `restore()` falla siempre en el Pixel, plan aparte. Si lee `raw:1`, el "cortado" venía de otra parte (p. ej. un registro de corte antiguo).
- **V2. Cortar y volver a dar acceso en el Pixel** (modifica el móvil). Prerrequisito: V1. Si V1 muestra lecturas fiables: cortar desde el escudo, el PC pierde la conexión y la app dice "Acceso cortado"; "Volver a dar acceso" devuelve USB e inalámbrica a 1 y el PC reconecta (el puerto puede cambiar).
- **V3. Icono monocromo** con los iconos temáticos del lanzador del Pixel (Android 13+).
- **V4.** Superposición en API 30: `GuardOnEmulatorTest` no se ha ejecutado (no había emulador).
- **V5.** Icono en el lanzador del emulador (adaptativo y monocromo).
- **V6.** En el Pixel, comprobar que la cabecera, el escudo y la tarjeta se ven bien con el tamaño de fuente real del usuario.
