# Fragmento: lecturas falsas de adb_enabled (rama ux/acceso-a17)

Cierre del plan `2026-10-19-acceso-lecturas-falsas` (adenda `2026-10-19-acceso-lecturas-falsas.md`). Las filas de aquí las integra el controlador en `pendientes-m3-m5.md` (sección UX1) y en `2026-10-03-validaciones-manuales.md`.

## Pendientes

| Id | Qué | Nota |
|---|---|---|
| UX-f | En Android 17 la lectura no confirma que `adb_enabled=0/1` se aplique | Adenda 2026-10-19 R18-R19 y R22 (comprobación por canal y 5555, que no ve una USB encendida sin modo tcpip). Lo mide V2 (UX1-2), condición para integrar. |
| UX-g | Minors de `AccessDiag`: `doubt` hace `trim()` y el switch no; sin test unitario de `AccessDiag.log`; sin test de lectura con error | El orden de `saved.adb_enabled_doubted` lo valida V2. Una lectura con error no debería contar para la duda: falta test. |
| UX-h | `CutConfirm.check`: `waited` no cuenta el tiempo del sondeo (hasta ~15 s en un caso patológico) y "Hecho" se decide con la primera muestra buena, sin estabilizar | Falla hacia avisar tarde, no hacia un falso "Hecho" persistente. |
| UX-i | Reactivar con la comprobación posterior al corte aún en curso (≤5 s) no la cancela | Posible falsa alarma o "Hecho" viejo; haría falta un contador de generación. |
| UX-j | El test estático `bothCutButtonsPassTheChannelAndCheckTheCut` no exige que la comprobación esté dentro de la rama de éxito | |
| UX-k | `AccessSwitch`: el registro (3 commits de `SharedPreferences`) no es atómico | Falla seguro. |

## Inventario (ficheros nuevos y clave)

- `control/CutConfirm.kt` (+ `CutCheck`): comprobación posterior al corte.
- `control/CutAccess`: `confirm`, `probe`, `waitMs`, `channelUp`.
- `AdbuddyApp`: `confirmCut`, `cutCheck`, `cutCheckShown`.
- `AccessSwitch`: `adbZeroIsDoubtful`, `ADB_DOUBTED`.
- `AccessDiag.doubt`.
- String `cut_still_connected`.

## Validaciones manuales (exigen el móvil real; con permiso explícito del usuario)

- **UX1-1: HECHA.** El 2026-10-19 la app leyó `raw:0` en `adb_enabled` mientras el shell decía `1`, y `adb_wifi_enabled` coincidía (adenda `2026-10-19-acceso-lecturas-falsas.md`).
- **UX1-2: pasa a "V2 rehecha de la adenda (§5), condición para integrar en main".** Prerrequisito: build de `ux/acceso-a17` instalada (permiso; instalar modifica el móvil). Pasos 1-7 de la sección "Validaciones manuales pendientes" del plan:
  1. Antes: `adb shell settings get global adb_enabled` y `adb_wifi_enabled` (esperado `1` y `1`), y `adb logcat -c`. Apuntar si se usó el modo remoto (`adb tcpip`) y la IP del móvil.
  2. Cortar desde el escudo con el chat conectado. Esperado: el PC pierde la conexión, la app dice "Acceso cortado" y, en unos segundos, "Hecho. El PC ya no puede tocar tu móvil." (no "El PC sigue conectado…").
  3. En Opciones de desarrollador, "Depuración por USB" y "Depuración inalámbrica" apagadas. Si la USB sigue encendida, R18 se queda corto: apuntarlo y abrir un plan.
  4. Si hubo modo remoto: `adb connect <ip>:5555` debe fallar. Con cable USB: `adb -d devices` no debe listar el móvil.
  5. Reactivar desde la app (escudo tachado, diálogo y confirmar): "El PC vuelve a tener acceso" y el PC reconecta (el puerto inalámbrico puede cambiar).
  6. `settings get global adb_enabled` / `adb_wifi_enabled` → `1` / `1`, y `adb logcat -d -s ADBuddyAccess` en este orden: `cut.before` (con `doubtful=adb_enabled(wifi+channel)`), `cut.after ok=true … saved.adb_enabled_doubted=1`, `cut.check=DONE`, `restore.before`, `restore.after ok=true … doubtful=adb_enabled(wifi)`.
  7. Si reactivar falla, el usuario enciende la depuración inalámbrica (y la USB) en Opciones de desarrollador. Apuntar la línea `restore.after`.
- **Línea de pasos del usuario "Estado del acceso y aspecto visual":** instalar la build de `ux/acceso-a17` (sin la lectura de UX1-1, ya hecha), cortar y volver a dar acceso desde el escudo (UX1-2), y mirar el icono monocromo y la cabecera con la fuente real. Cierra UX1-2, 3 y 6.
- **V3. Icono monocromo** con los iconos temáticos del lanzador del Pixel (Android 13+).
