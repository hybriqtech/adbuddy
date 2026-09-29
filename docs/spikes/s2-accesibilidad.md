# Spike S2 · Activar la accesibilidad por adb en una app instalada fuera de Play

> Spike desechable. APK mínimo `tech.hybriq.adbuddy.spike.a11y` con un servicio de accesibilidad que solo escribe en logcat.

## Preguntas

| # | Pregunta | Resultado | Evidencia (sección del registro) |
|---|---|---|---|
| Q1 | Instalado con `adb install`, ¿se activa con `settings put secure enabled_accessibility_services`? | **Sí.** Instalador `com.android.shell` (`installerPackageName=null`, `packageSource=1`), `ACCESS_RESTRICTED_SETTINGS` en `default`. Tras escribir los dos ajustes (`enabled_accessibility_services` y `accessibility_enabled=1`) con `settings put`, el servicio aparece en *Bound services* y registra `CONNECTED` al instante. | Caso A: instalado con adb install; Caso A: tras activar por adb |
| Q2 | ¿Cuánto tarda leer el árbol de una ventana (ms y nodos) frente a `uiautomator dump`? | **En la misma pantalla (Ajustes, caso B): servicio 31–49 ms (82 y 78 nodos) frente a `uiautomator dump` 2498 ms (76 nodos en el XML)**, unas 50–80 veces más rápido. En el caso A: 62–98 ms para 82 nodos (launcher) frente a 3631 ms de `uiautomator dump` en Ajustes. Solo hay dos muestras de cada lado y el tiempo de `uiautomator` incluye el viaje por adb. **Hallazgo:** `uiautomator dump` desconecta el servicio y lo reconecta al terminar (sale un segundo `CONNECTED` en los dos casos); probablemente porque UiAutomation suprime los demás servicios de accesibilidad mientras está activo (inferencia, no verificada). **Hallazgo:** `am start` sobre una tarea ya abierta ("brought to the front") no emite `TYPE_WINDOW_STATE_CHANGED`; con `am start -S` sí. | Caso A: tras activar por adb; logcat A11yProbe completo; Q2 en la misma pantalla (Ajustes) |
| Q3 | Instalado desde el navegador (flujo real del QR 1), ¿qué instalador figura y qué devuelve `appops get … ACCESS_RESTRICTED_SETTINGS`? | `installerPackageName=com.google.android.packageinstaller`, `initiatingPackageName=com.google.android.packageinstaller` (no figura el navegador), `packageSource=4` (fichero descargado). `ACCESS_RESTRICTED_SETTINGS: deny`. En la interfaz, la entrada de Accesibilidad está deshabilitada ("Controlada por ajustes restringidos") y al tocarla sale el diálogo "Se ha denegado el acceso a la aplicación", con un único botón "Cerrar". | Caso B: instalado desde el navegador; Caso B: ajuste restringido en la interfaz |
| Q4 | En ese caso, ¿se activa por adb pese al ajuste restringido? | **Sí.** Con `ACCESS_RESTRICTED_SETTINGS` en `deny`, `settings put secure enabled_accessibility_services` (más `accessibility_enabled=1`) conecta el servicio: sale `CONNECTED`, aparece en *Bound services* y recibe eventos. El bloqueo solo afecta al interruptor de la interfaz, no al ajuste escrito por adb (shell). | Caso B: activado por adb |
| Q5 | Si no, ¿lo desbloquea `appops set … ACCESS_RESTRICTED_SETTINGS allow`? | **No aplica:** el paso 6 no se ejecutó porque en el paso 5 el servicio ya se conectó. No se ha probado si `appops set … allow` desbloquea también el interruptor de la interfaz. | Caso B: activado por adb |
| Q6 | ¿Aparece algún aviso de Play Protect o de verificación de desarrollador al instalar desde el navegador? | **Sí, Play Protect.** Según el usuario, salió un aviso de Play Protect y tuvo que pulsar "Instalar de todas formas" (no dio el texto exacto). No mencionó ningún aviso de verificación de desarrollador ni espera de 24 h. Con `adb install` (caso A) no salió ningún aviso. | Q6 (👤) |

## Conclusión e impacto en la spec (§3.1 onboarding, §9 casos límite, §12 publicación)

Resultado en Pixel 9 Pro XL, Android 17 (SDK 37). Solo se ha probado en ese dispositivo y versión.

1. **El onboarding de §3.1 puede activar la accesibilidad por adb con cualquiera de las dos instalaciones.**
   - Con `adb install` no hay ajuste restringido (`default`).
   - Con la instalación desde el navegador (el QR 1) el ajuste restringido queda en `deny`. Aun así, escribir `enabled_accessibility_services` y `accessibility_enabled` desde el shell de adb conecta el servicio.
   - El paso 7 de §3.1 funciona tal como está diseñado, sin depender del origen de la instalación.
2. **El paso manual de §9 ("Permitir ajustes restringidos") no hace falta en el flujo normal, porque adb basta. Tampoco hace falta `appops`.**
   - La fila de §9 debería quedar como respaldo: por si una versión futura de Android o un fabricante cierran esta vía, y para quien active el servicio desde la interfaz sin adb.
   - En ese caso, la entrada aparece deshabilitada y el diálogo solo ofrece "Cerrar", así que la guía tiene que llevar a *Info de la app → ⋮ → Permitir ajustes restringidos*.
   - Queda sin probar si `appops set … ACCESS_RESTRICTED_SETTINGS allow` sirve como alternativa automática (Q5).
3. **La verificación de desarrolladores de Google sí cambia el orden recomendado del onboarding.**
   - La verificación se aplica a instalaciones fuera de Play desde el 30-09-2026 en BR, ID, SG y TH, y en todo el mundo en 2027.
   - Las instalaciones por adb están exentas de la verificación y, además, no dejan la app en ajuste restringido ni muestran aviso de Play Protect. Desde el navegador sí salió Play Protect ("Instalar de todas formas").
   - Recomendación: invertir el orden. Primero emparejar (QR 2, pasos 3–5), y después instalar la app desde el host con `adb install` (hoy es el paso 6, que pasaría a ser la vía principal). El QR 1 del APK quedaría como alternativa, por ejemplo para quien use IzzyOnDroid.
   - Consecuencia: la guía de "activar opciones de desarrollador y depuración inalámbrica" ya no puede estar dentro de la app, porque aún no está instalada. Tendría que darla el terminal o una página web.
   - Contra §12: publicar en GitHub/IzzyOnDroid sigue siendo viable, pero conviene preparar la verificación de desarrollador antes de 2027 para quien instale el APK sin adb.
4. **Efectos para el diseño.**
   - Leer el árbol con el servicio es unas 50–80 veces más rápido que `uiautomator dump` (el tiempo de `uiautomator` incluye el transporte por adb), lo que confirma que el servicio es la vía principal para leer la pantalla.
   - `uiautomator dump` desconecta temporalmente el servicio de ADBuddy. Si el agente lo usa como respaldo, el servicio pierde eventos durante unos segundos y se reconecta solo.

Efectos laterales del spike:
- `a11y.sh enable` borró el buffer de logcat del móvil (`logcat -c`) dos veces, una por caso.
- El primer intento del caso B, con el puerto 8765, hizo `adb reverse tcp:8765`, que apuntaba al servidor local `autotube` del usuario. El navegador del móvil cargó un 404 de ese servidor: exposición breve de un servicio local del PC al móvil. El `reverse` se eliminó.
- El navegador pudo quedarse con "Instalar apps desconocidas" concedido. Se avisó al usuario para que lo revoque.
- El APK del caso B queda en la carpeta Descargas del móvil hasta que el usuario lo borre. Se avisó al usuario.

## Registro

### Snapshot inicial (2026-09-29T18:11:54Z)
```
ORIG_SVCS=null
ORIG_ENABLED=0
```

### Caso A: instalado con adb install (2026-09-29T18:12:06Z)
```
    installerPackageName=null
    initiatingPackageName=com.android.shell
    packageSource=1
ACCESS_RESTRICTED_SETTINGS: No operations.
Default mode: default
```

### Caso A: tras activar por adb (2026-09-29T18:12:23Z)
```
enabled_accessibility_services=tech.hybriq.adbuddy.spike.a11y/.ProbeService
     Bound services:{Service[label=ADBuddy S2 probe, feedbackType[FEEDBACK_GENERIC], capabilities=1, eventTypes=TYPE_WINDOW_STATE_CHANGED, notificationTimeout=100, requestA11yBtn=false]}
     Enabled services:{{tech.hybriq.adbuddy.spike.a11y/tech.hybriq.adbuddy.spike.a11y.ProbeService}}
--- logcat A11yProbe
--------- beginning of main
09-29 20:12:17.087 25848 25848 I A11yProbe: CONNECTED
09-29 20:12:24.098 25848 25848 I A11yProbe: WINDOW pkg=com.google.android.apps.nexuslauncher nodes=82 ms=62
```
- uiautomator dump en Ajustes: 3631 ms

Logcat A11yProbe completo, leído 2 s después del paso anterior (2026-09-29T18:13:02Z); no aparece ningún WINDOW de com.android.settings:
```
--------- beginning of main
09-29 20:12:17.087 25848 25848 I A11yProbe: CONNECTED
09-29 20:12:24.098 25848 25848 I A11yProbe: WINDOW pkg=com.google.android.apps.nexuslauncher nodes=82 ms=62
09-29 20:12:39.803 25848 25848 I A11yProbe: CONNECTED
09-29 20:12:41.358 25848 25848 I A11yProbe: WINDOW pkg=com.google.android.apps.nexuslauncher nodes=82 ms=98
```

### Caso A: restauración (2026-09-29T18:13:02Z)
```
restaurado: enabled_accessibility_services=null accessibility_enabled=0
OK: coincide con el snapshot
Success
```

### Caso B: descarga servida (2026-09-29T18:15:04Z)

Desviación: el puerto 8765 del plan estaba ocupado por otro proceso del PC, así que el APK se sirve en 127.0.0.1:8766 con un servidor inline equivalente a `serve.py` (`adb reverse tcp:8766 tcp:8766`). `curl -sI` desde el PC: `HTTP/1.0 200 OK`.

Q6 (👤, 2026-09-29T18:22:10Z): el usuario confirma "Instalada". Corrección posterior del usuario: salió un aviso de **Play Protect** y tuvo que pulsar **"Instalar de todas formas"** (no dio el texto exacto del aviso).

### Caso B: instalado desde el navegador (2026-09-29T18:22:10Z)
```
    installerPackageName=com.google.android.packageinstaller
    initiatingPackageName=com.google.android.packageinstaller
    packageSource=4
ACCESS_RESTRICTED_SETTINGS: ACCESS_RESTRICTED_SETTINGS: deny
```

### Caso B: ajuste restringido en la interfaz (2026-09-29T18:25:57Z)

Paso 4 delegado por el usuario: lo hizo el ejecutor por adb (`am start -a android.settings.ACCESSIBILITY_SETTINGS`, `uiautomator dump` e `input tap`), sin capturas.

- En Ajustes → Accesibilidad, bajo "Aplicaciones descargadas", la entrada aparece como `ADBuddy S2 probe` con el resumen **"Controlada por ajustes restringidos"** y el nodo tiene `enabled="false"`.
- Al tocar la entrada no se abre la pantalla del servicio ni llega a verse el interruptor: aparece directamente un diálogo de `com.android.permissioncontroller` (`enhanced_confirmation_dialog`), con este texto literal:
  - Título: "Se ha denegado el acceso a la aplicación"
  - Mensaje: "El acceso a este permiso puede poner en riesgo tu información personal y financiera. Es posible que la aplicación no funcione correctamente sin este permiso restringido. Consulta cómo permitir el acceso."
  - Botones: "Cerrar" (único)
- Se cerró con `KEYCODE_BACK` (vuelve a la lista de Accesibilidad) y `KEYCODE_HOME`. Se borró `/data/local/tmp/s2ui.xml`.
- Después: `enabled_accessibility_services=null` (no se activó desde la interfaz).

### Caso B: activado por adb (2026-09-29T18:26:13Z)
```
enabled_accessibility_services=tech.hybriq.adbuddy.spike.a11y/.ProbeService
     Bound services:{Service[label=ADBuddy S2 probe, feedbackType[FEEDBACK_GENERIC], capabilities=1, eventTypes=TYPE_WINDOW_STATE_CHANGED, notificationTimeout=100, requestA11yBtn=false]}
     Enabled services:{{tech.hybriq.adbuddy.spike.a11y/tech.hybriq.adbuddy.spike.a11y.ProbeService}}
--- logcat A11yProbe
--------- beginning of main
09-29 20:26:07.031 30842 30842 I A11yProbe: CONNECTED
09-29 20:26:10.732 30842 30842 I A11yProbe: WINDOW pkg=com.google.android.googlequicksearchbox nodes=122 ms=129
09-29 20:26:14.084 30842 30842 I A11yProbe: WINDOW pkg=com.google.android.apps.nexuslauncher nodes=82 ms=102
```

### Q2 en la misma pantalla (Ajustes) (2026-09-29T18:26:33Z)

`am start -S -a android.settings.SETTINGS` (reinicia Ajustes), `pause 3` y, sin cambiar de pantalla, `uiautomator dump`.
```
Servicio: 09-29 20:26:27.633 30842 30842 I A11yProbe: WINDOW pkg=com.android.settings nodes=82 ms=31
09-29 20:26:27.983 30842 30842 I A11yProbe: WINDOW pkg=com.android.settings nodes=78 ms=49
uiautomator dump en Ajustes: 2498 ms (nodos en el XML: 76)
--- logcat A11yProbe completo tras el dump y HOME
--------- beginning of main
09-29 20:26:07.031 30842 30842 I A11yProbe: CONNECTED
09-29 20:26:10.732 30842 30842 I A11yProbe: WINDOW pkg=com.google.android.googlequicksearchbox nodes=122 ms=129
09-29 20:26:14.084 30842 30842 I A11yProbe: WINDOW pkg=com.google.android.apps.nexuslauncher nodes=82 ms=102
09-29 20:26:27.633 30842 30842 I A11yProbe: WINDOW pkg=com.android.settings nodes=82 ms=31
09-29 20:26:27.983 30842 30842 I A11yProbe: WINDOW pkg=com.android.settings nodes=78 ms=49
09-29 20:26:32.546 30842 30842 I A11yProbe: CONNECTED
09-29 20:26:34.166 30842 30842 I A11yProbe: WINDOW pkg=com.google.android.apps.nexuslauncher nodes=82 ms=90
```

### Caso B: restauración (2026-09-29T18:26:41Z)
```
restaurado: enabled_accessibility_services=null accessibility_enabled=0
OK: coincide con el snapshot
restore exit=0
Success
reverse activos: []
pm list packages tech.hybriq.adbuddy.spike.a11y: []
```
