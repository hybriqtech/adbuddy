# T1 · Mediciones en el emulador propio (2026-10-03)

Plan: `docs/superpowers/plans/2026-10-03-validaciones-manuales.md`, lista "Por medir" (1-12).

Entorno: AVD `adbuddy-dev34` (`system-images;android-34;google_apis;x86_64`, userdebug), consola 5600, servidor adb propio 5060, `ADB_MDNS=0`, `ADBUDDY_EMU_STABLE_SECS=120` (arranque estable). Todas las órdenes con `adb -P 5060 -s emulator-5600`; `adb -P 5060 devices` listó solo `emulator-5600` en cada paso. App: `app-debug.apk` de la rama (`gradle --no-daemon --offline :app:assembleDebug :app:assembleDebugAndroidTest`), instalada con `adb install`. Al empezar había un emulador ajeno (`adbuddy-ci-api34`, puerto 5590); se esperó a que terminara y no se tocó. Al acabar volvió a arrancar otro de la CI, que tampoco se tocó.

Las salidas van resumidas, sin IPs ni seriales. El nombre del PC que sale en "Dispositivos emparejados" también se ha quitado.

Estado de partida: `persist.sys.locale` vacío (`ro.product.locale=en-US`); `notification_bubbles` global `null` y secure `1`; `adb_enabled=1`, `adb_wifi_enabled=0`; `development_settings_enabled` sin definir (opciones de desarrollador apagadas); solo el usuario 0; la app no estaba instalada. Se guardó `settings list global|secure|system` y `getprop` (persist y adb) para compararlos al final.

## 1. `allow_listener` con un listener `exported="false"`

```
$ adb shell cmd notification allow_listener tech.hybriq.adbuddy/tech.hybriq.adbuddy.listener.AgentNotificationListener   # rc=0
$ adb shell dumpsys notification   # sección "Live notification listeners (8)"
  ComponentInfo{tech.hybriq.adbuddy/tech.hybriq.adbuddy.listener.AgentNotificationListener} (user 0): ...INotificationListener$Stub$Proxy@...
$ adb shell settings get secure enabled_notification_listeners
  ...:tech.hybriq.adbuddy/tech.hybriq.adbuddy.listener.AgentNotificationListener
$ adb shell cmd notification disallow_listener <mismo>   # 2 s después: 0 líneas de hybriq en Live, y fuera del ajuste
$ adb shell cmd notification allow_listener <mismo>      # 2 s después: vuelve a Live
```

**Resultado:** enlaza aunque sea `exported="false"`, porque lo enlaza el sistema. `allow_listener` arranca el proceso solo. `disallow_listener` lo desenlaza y lo quita de `enabled_notification_listeners` en menos de 2 s.

## 2. Estilos de `cmd notification post`

```
$ adb shell cmd notification post --help
  -t|--title  -i|--icon  -I|--large-icon  -S|--style <style>  -c|--content-intent
  styles: bigtext | bigpicture --picture <iconspec> | inbox --line .. | messaging --conversation <title> --message <who>:<text> ... | media
$ adb shell "cmd notification post -S bigtext -t 'T1 big' t1big 'Line one ...'"
$ adb shell "cmd notification post -S messaging --conversation 'T1 conv' --message 'Ana:hola 482913' --message 'Bob:ok' t1msg 'fallback text'"
$ adb shell "cmd notification post -S inbox --line l1 --line l2 t1inbox 'inbox text'"
$ adb shell dumpsys notification --noredact
  pkg=com.android.shell id=2020 tag=t1big  channel=shell_cmd vis=PRIVATE  tickerText=null
     android.template=BigTextStyle  android.text=String (Line one ...)  android.bigText=null
  pkg=com.android.shell id=2020 tag=t1msg  tickerText=null
     android.template=MessagingStyle  android.title=T1 conv: Bob  android.text=String (ok)
     android.messages=Bundle[] (2) [Ana: hola 482913] [Bob: ok]
  pkg=com.android.shell id=2020 tag=t1inbox tickerText=null  android.template=InboxStyle
```

**Resultado:**
- Admite `bigtext`, `bigpicture`, `inbox`, `messaging` y `media`.
- **No rellena `tickerText`** (siempre `null`) y no hay opción para la visibilidad: todas salen `vis=PRIVATE`, canal `shell_cmd` y paquete `com.android.shell`.
- `-S bigtext` **deja `android.bigText=null`**: el texto va en `android.text`. No sirve para probar "BigText entera".
- `-S messaging` **sí rellena `android.text`** con el último mensaje. No sirve para probar "MessagingStyle sin `EXTRA_TEXT`".
- Esos dos casos, el ticker (M5b-18) y la visibilidad tienen que ser notificaciones propias del instrumentado.

## 3. Paquetes de la imagen y rol SMS

```
$ adb shell pm path com.google.android.apps.messaging   -> sí
$ adb shell pm path com.google.android.gm               -> sí
$ adb shell pm path com.google.android.gms              -> sí
$ adb shell pm path com.android.messaging               -> no
$ adb shell cmd role get-role-holders android.app.role.SMS
  com.google.android.apps.messaging
```

**Resultado:** la imagen trae Mensajes, Gmail y GMS. La app de SMS por defecto ya es otra (Mensajes de Google), así que la M5c-33(a) se puede probar sin app de pruebas.

## 4. `set-bypassing-role-qualification` y `add-role-holder` con un paquete que no cumple

```
$ adb shell cmd role help   -> ... add-role-holder, remove-role-holder, clear-role-holders, set-bypassing-role-qualification true|false
$ adb shell cmd role add-role-holder android.app.role.SMS tech.hybriq.adbuddy
  Error: see logcat for details.  java.util.concurrent.ExecutionException: java.lang.RuntimeException: Failed   (rc=255)
  logcat: Role: tech.hybriq.adbuddy not qualified for android.app.role.SMS due to missing RequiredComponent{...SMS_DELIVER... BROADCAST_SMS}
          RoleControllerServiceImpl: Package does not qualify for the role
$ adb shell cmd role set-bypassing-role-qualification true   # rc=0
$ adb shell cmd role add-role-holder android.app.role.SMS tech.hybriq.adbuddy
  el mismo error (rc=255), el mismo "not qualified" en logcat; el titular sigue siendo com.google.android.apps.messaging
$ adb shell cmd role set-bypassing-role-qualification false   # restaurado
```

**Resultado:**
- La orden existe en la API 34, pero **no salta la cualificación del rol SMS**.
- `add-role-holder` falla con un paquete sin los componentes del rol, haya bypass o no.
- Para que el titular sea una app de pruebas, la variante `sms` de la T7 tiene que declarar los componentes: el receptor `SMS_DELIVER` con `BROADCAST_SMS` y los demás del rol.

## 5. Perfil gestionado

```
$ adb shell pm list features   -> feature:android.software.managed_users, feature:android.software.device_admin
$ adb shell pm get-max-users   -> Maximum supported users: 4
$ adb shell pm create-user --profileOf 0 --managed adbuddy-work   -> Success: created user id 10
$ adb shell am start-user -w 10                                   -> Success: user started
$ adb shell pm install-existing --user 10 tech.hybriq.adbuddy     -> installed for user: 10
$ adb shell pm list users   -> 0:Owner running, 10:adbuddy-work running
$ adb shell pm remove-user 10   -> Success: removed user   (limpieza; queda solo el 0)
```

**Resultado:** funciona sin DPC. Las apps se meten en el perfil con `pm install-existing --user 10` (o con `adb install --user 10`).

## 6. `ACCESS_RESTRICTED_SETTINGS` tras `adb install`

```
$ adb install app-debug.apk   -> Success
$ adb shell appops get tech.hybriq.adbuddy ACCESS_RESTRICTED_SETTINGS
  No operations.  Default mode: allow
$ adb shell dumpsys package tech.hybriq.adbuddy
  installerPackageName=null  initiatingPackageName=com.android.shell  packageSource=1 (OTHER)
$ adb shell am start -a android.settings.NOTIFICATION_LISTENER_DETAIL_SETTINGS --es android.provider.extra.NOTIFICATION_LISTENER_COMPONENT_NAME <listener>
$ uiautomator dump   -> "Allow notification access"  android:id/switch_widget checkable=true checked=false enabled=true
```

**Resultado:** una instalación por adb **no queda restringida** en la API 34 (`packageSource=OTHER`, appop en `allow`, interruptor activo). El caso del interruptor en gris no sale con `adb install`. Habría que simularlo con `appops set … ACCESS_RESTRICTED_SETTINGS ignore` o instalar con origen "fichero local". Ninguna de las dos se ha probado.

## 7. Bloqueo con PIN

```
$ adb shell locksettings set-pin 1111            -> Pin set to '1111'
$ adb shell input keyevent KEYCODE_SLEEP         -> dumpsys power: mWakefulness=Asleep
$ adb shell input keyevent KEYCODE_WAKEUP        -> dumpsys window: mCurrentFocus=NotificationShade, mDreamingLockscreen=true, isKeyguardShowing=true
                                                    dumpsys trust: deviceLocked=1
$ adb shell cmd statusbar expand-notifications   -> la persiana se abre sobre el bloqueo (uiautomator: notificaciones del shell, "Device locked")
$ adb shell input swipe 540 1800 540 600 300     -> uiautomator: "Enter your PIN", com.android.systemui:id/pinEntry, teclas 0-9
$ adb shell input text 1111 ; input keyevent KEYCODE_ENTER
                                                 -> isKeyguardShowing=false, deviceLocked=0, foco en el lanzador
$ adb shell locksettings clear --old 1111        -> Lock credential cleared
$ adb shell settings get global adb_enabled      -> 1
```

**Resultado:** todo el ciclo funciona por shell: PIN, dormir, despertar, bouncer, teclear el PIN y desbloquear. La limpieza deja el AVD sin PIN y con `adb_enabled=1`.

## 8. Burbujas

```
$ adb shell settings get global notification_bubbles   -> null
$ adb shell settings get secure notification_bubbles   -> 1
# notificación de conversación real: handshake por stdin (content write .../handshake, puerto sin host) -> BridgeService en primer plano
$ adb shell dumpsys notification --noredact
  pkg=tech.hybriq.adbuddy id=1 channel=conversation shortcut=conversation flags=0x6a (ONGOING|ONLY_ALERT_ONCE|NO_CLEAR|FOREGROUND_SERVICE)
  mImportance=LOW  mAllowBubble=false  isBubble=false
$ Ajustes > Notificaciones > Conversaciones (android.settings.CONVERSATION_SETTINGS) -> "Recent conversations: ADBuddy"
$ adb shell cmd notification set_bubbles tech.hybriq.adbuddy 1   -> Ajustes de burbujas de la app: "All conversations can bubble" seleccionado
# repetida (nuevo handshake): id=1 sigue flags=0x6a, mAllowBubble=false, isBubble=false, y sin BubbleMetadata
# sonda temporal (instrumentado T1BubbleProbe, no versionado): misma notificación (canal conversation, atajo
# conversation, MessagingStyle, BubbleMetadata a ChatBubbleActivity) pero sin servicio en primer plano, id=9901
  pkg=tech.hybriq.adbuddy id=9901 flags=0x1000 (FLAG_BUBBLE)  mAllowBubble=true  isBubble=true
$ dumpsys window windows -> "Window{... Bubbles!}";  uiautomator --windows: com.android.systemui:id/bubble_view "Probe from ADBuddy and 1 more" [0,450][150,608]
$ adb shell input tap 75 529 -> mCurrentFocus=Bubbles!; Task #13 tech.hybriq.adbuddy/.ChatBubbleActivity mode=multi-window bounds=[42,361][1038,1936]
$ adb shell input keyevent KEYCODE_BACK -> se pliega, foco en el lanzador
```

**Resultado:**
- En la API 34 la preferencia de burbujas de una app recién instalada es "nada" (`mAllowBubble=false`). El usuario tiene que elegirla; en los tests vale `cmd notification set_bubbles <pkg> 1`.
- **Hallazgo:** la notificación de conversación de ADBuddy no puede ser burbuja ni con la preferencia en "todas". El sistema le quita el `BubbleMetadata`. La misma notificación sin el flag de servicio en primer plano sí es burbuja: el canal, el atajo, el metadata y la actividad son idénticos, y solo cambia `FLAG_FOREGROUND_SERVICE`.
- Es coherente con la exclusión de las notificaciones de servicio en primer plano en el `BubbleExtractor` de AOSP. Eso no se ha leído en la fuente: lo que hay es la medida.
- `ChatBubbleActivity` sí se abre como burbuja y se pliega con atrás.
- Nota aparte: tras `am instrument` el proceso se reinicia y `startForegroundService` falla con `ForegroundServiceStartNotAllowedException` hasta que se hace `dumpsys deviceidle whitelist +tech.hybriq.adbuddy`, que es lo que hace `setup`.

## 9. Reinicio, `force-stop` y `kill`

```
$ adb shell am force-stop tech.hybriq.adbuddy
  3 s después: Live=1, pid nuevo; logcat: "enabling notification listener ... binding ... Start proc ... for service AgentNotificationListener ... connected" (<0,5 s)
$ adb shell am kill tech.hybriq.adbuddy   -> no lo mata (mismo pid; el proceso está retenido por el listener enlazado)
$ adb shell su 0 kill -9 <pid>            -> "Scheduling restart of crashed service ... in 1000ms"; pid nuevo y "connected" en ~1,4 s
$ adb reboot   (con persist.sys.locale=es-ES, ver 10)
  boot_completed en ~15 s; logcat: 01:33:16 "Unable to bind notification listener service" (primer intento)
                                   01:33:22 "notification listener service connected"   -> Live=1 sin abrir la app
```

**Resultado:**
- El listener se reenlaza solo tras `force-stop` (al momento, sin abrir la app), tras `kill -9` (en 1 s) y tras reiniciar (unos 6 s después del arranque; el primer intento falla y el segundo enlaza).
- `am kill` no sirve para la prueba "tras kill": no mata el proceso. Hay que usar `su 0 kill -9`.
- Un `adb reboot` con `-no-snapshot` conserva los datos: la app, su idioma y el listener permitido.

## 10. Idioma

```
$ adb shell cmd locale set-app-locales tech.hybriq.adbuddy --locales es   -> get-app-locales: [es]
$ adb shell dumpsys notification --noredact   -> canales: "El agente ha terminado", "Conversación con el agente", "Confirmaciones"
$ adb shell cmd locale set-app-locales tech.hybriq.adbuddy --locales en   -> "Agent finished", "Conversation with the agent", "Confirmations"
  (mismo pid antes y después: sin reiniciar el proceso)
  (el dumpsys redactado recorta los nombres de canal a 3 letras: "Con..."; hay que usar --noredact)
$ adb shell su 0 setprop persist.sys.locale es-ES ; adb reboot
  -> Ajustes en español ("Ajustes", "Buscar ajustes", "Redes e Internet"); ADBuddy sigue en inglés por su idioma propio
$ adb shell cmd locale set-app-locales tech.hybriq.adbuddy   (sin --locales) -> []  -> ADBuddy en español ("Paso 1 de 2", "Apps protegidas")
```

**Resultado:**
- El idioma por app funciona sin reiniciar y renombra los canales al momento.
- El idioma del sistema no tiene orden de shell en la API 34. Se cambia con `su 0 setprop persist.sys.locale es-ES` y un reinicio.
- Restaurado al final con `setprop persist.sys.locale ""` y otro reinicio: Ajustes vuelve a "Settings".

## 11. Pantalla partida

```
$ adb shell am supports-split-screen-multi-window   -> true
$ adb shell am start --help   -> --windowingMode <WINDOWING_MODE>
$ adb shell dumpsys activity service SystemUIService WMShell help   -> splitscreen moveToSideStage <taskId> <SideStagePosition> | removeFromSideStage <taskId> | setSideStagePosition
# Ajustes (task 11) y luego ADBuddy (task 12) a pantalla completa
$ adb shell dumpsys activity service SystemUIService WMShell splitscreen moveToSideStage 11 1
$ adb shell am stack list
  task 12 ADBuddy  bounds=[0,0][1080,1187]     visible=true
  task 11 Ajustes  bounds=[0,1213][1080,2400]  visible=true   (las dos mode=multi-window; foco en Ajustes)
$ ... splitscreen removeFromSideStage 11   -> las dos tareas se quedan en la raíz partida con bounds [0,0][1080,1187] (no vuelve limpia)
```

**Resultado:** la pantalla partida se monta por shell con el `WMShell` de systemui. La salida limpia no se ha medido: `removeFromSideStage` deja la raíz partida, y el reinicio de la 9 la limpió. La T5 tiene que medir la salida (`HOME`, `am task remove` o `am start --windowingMode 1`).

## 12. Depuración inalámbrica y `service.adb.tls.port`

```
$ adb shell settings put global adb_wifi_enabled 1
  -> vuelve a 0 y systemui muestra WifiDebuggingActivity: "¿Permitir la depuración inalámbrica en esta red?"
     [Permitir siempre en esta red] [Cancelar] [Permitir]
$ input tap sobre "Permitir" (sin "siempre")   -> adb_wifi_enabled=1; adbd: "mDNS service _adb-tls-connect._tcp registered"
$ adb -P 5060 devices -l   -> emulator-5600 device (el transporte no cambia en ningún momento)
$ adb shell getprop service.adb.tls.port   -> (vacío), rc=0
  logcat: getprop: avc: denied { read } for name="u:object_r:adbd_prop:s0" scontext=u:r:shell:s0 ; "Access denied finding property service.adb.tls.port"
$ adb shell su 0 getprop service.adb.tls.port   -> 39673
$ adb shell su 0 netstat -ltn                   -> LISTEN [::]:39673 (además de 5555, el transporte del emulador)
# Ajustes > Sistema > Opciones para desarrolladores > Depuración inalámbrica (uiautomator):
  "Usar depuración inalámbrica", "Dirección IP y puerto" = <IP>:39673, "Emparejar dispositivo con código QR",
  "Emparejar dispositivo con código de sincronización", "Dispositivos emparejados"
  -> "Emparejar con dispositivo": "Código de vinculación de Wi‑Fi" <6 cifras>, "Dirección IP y puerto" <IP>:<puerto>, "Cancelar"
$ adb shell settings put global adb_wifi_enabled 0   -> service.adb.tls.port vacío, persist.adb.tls_server.enable=0
```

**Resultado:**
- **Hallazgo:** en la API 34 el usuario shell **no puede leer** `service.adb.tls.port`: SELinux lo deniega y `getprop` devuelve una cadena vacía con rc 0.
- Root sí la lee, y coincide con el puerto de Ajustes y con el socket en escucha.
- Con eso, `readTLSPort` (`host/internal/cli/phone_remote.go:446`) devuelve 0. Su respaldo, `info.WifiADBPort`, la app no lo envía nunca (`DeviceInfoSender` solo rellena `tailscaleIp`). Así que `tailscale_port` no se guardaría. Falta confirmarlo en Android 17, pero un Pixel de producción no tiene `su`.
- El diálogo "Emparejar con código" se lee con `uiautomator dump` (M1b-2).
- Las opciones de desarrollador estaban apagadas en el AVD. `settings put global development_settings_enabled 1` no basta para que `android.settings.APPLICATION_DEVELOPMENT_SETTINGS` abra la pantalla (sigue resolviendo a `DevelopmentSettingsDisabledActivity`); se llegó navegando por Ajustes.

## Otros "por medir" de las tablas (fuera de la lista 1-12)

```
$ adb shell appops set tech.hybriq.adbuddy ACTIVATE_VPN allow   -> rc=0; get: ACTIVATE_VPN: allow   (devuelto a default)
$ adb shell cmd clipboard help                                    -> "No shell command implementation."
$ adb shell am start -W -a android.provider.action.PICK_IMAGES    -> PhotoPickerActivity ("No hay fotos ni vídeos" sin medios)
$ adb shell "am start -W -a android.intent.action.OPEN_DOCUMENT -c android.intent.category.OPENABLE -t '*/*'"
  -> com.google.android.documentsui/.picker.PickActivity; lista un fichero empujado a /sdcard/Download ("t1-probe.txt", en Recientes)
```

- M5b-16: el appop `ACTIVATE_VPN` se acepta por shell.
- M3b-1: **no hay portapapeles por shell** en la API 34. El texto Unicode no se puede meter así; tiene que ir por el `type` de ADBuddy (accesibilidad, M5a-2) o por un instrumentado.
- M3b-3: la app usa `OpenDocument` (DocumentsUI) y `PickVisualMedia` (Photo Picker), y los dos se abren y se leen con UiAutomator. Un fichero empujado aparece en Recientes.
- No medidos (no son de la lista y necesitan app de pruebas o `serve`): M5c-30 (`setLockscreenVisibility`, T7) y VB-40 (el servidor 5038 de `serve`).
- No medible: si el mapper de `android-36` tiene la misma aserción. La imagen no está instalada.

## Trampas para quien escriba los tests

- El `grep -E` con alternancias (`a|b`) del shell del emulador (toybox) da `Segmentation fault`. Hay que filtrar en el PC.
- `adb shell` une los argumentos y los pasa al shell del móvil, así que `-t '*/*'` se expande como glob (`bin/abb`). Hay que mandarlo entre comillas dentro de la orden.
- Justo tras el primer arranque puede salir el ANR "System UI isn't responding". Se quita con `am broadcast -a android.intent.action.CLOSE_SYSTEM_DIALOGS`.
- `am instrument` reinicia el proceso de la app: el servicio en primer plano cae, y hay que volver a hacer el handshake, con la app en la lista blanca de `deviceidle`.

## Limpieza y estado final

- Hecho: `cmd notification disallow_listener`, `dumpsys deviceidle whitelist -tech.hybriq.adbuddy`, `cmd notification set_bubbles tech.hybriq.adbuddy 0`, `uninstall tech.hybriq.adbuddy.test` y `tech.hybriq.adbuddy`, `pm remove-user 10`, `locksettings clear --old 1111`, `cmd role set-bypassing-role-qualification false`, `adb_wifi_enabled 0`, `settings delete global development_settings_enabled`, `rm /sdcard/Download/t1-probe.txt`, `setprop persist.sys.locale ""` y un reinicio.
- También se borraron los restos de `am instrument` y de las opciones de desarrollador (`debug_app`, `wait_for_debugger`, `custom_bugreport_handler_app/_user`), y se devolvió `phenotype_test_setting=V15AboveInEnglish`, que GMS había cambiado con el idioma.
- Diferencia final con el estado de partida: solo `boot_count`, `Phenotype_boot_count`, `persist.sys.boot.reason.history`, `persist.logd.logpersistd=[]` y `persist.sys.locale=[]` (vacío, equivale a no definido). `system` sin diferencias.
- Comprobado: `adb_enabled=1`, el bloqueo desactivado, solo el usuario 0, ningún paquete `hybriq`, el titular del rol SMS sigue siendo Mensajes y Ajustes está en inglés.
- La sonda `T1BubbleProbe.kt` se creó en `android/app/src/androidTest/...` solo para la medida 8 y se borró antes del commit.
- `scripts/emulator.sh stop`: no queda ningún proceso de `adbuddy-dev34`. Al final el `tasklist` muestra otro emulador de la CI (`adbuddy-ci-api34`, puerto 5590), que no se tocó.
