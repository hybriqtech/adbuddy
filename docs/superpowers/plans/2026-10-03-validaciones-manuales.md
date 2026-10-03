# Inventario de las validaciones manuales pendientes

> Encargo del supervisor (f3): el usuario no quiere QA manual. Una fila por cada validación pendiente de los planes, con si se puede automatizar en **nuestro** emulador, cómo, y si no, por qué exactamente y cuál es el paso mínimo del usuario. Solo documentación: no se ha arrancado el emulador ni adb para escribirlo. Base: `origin/main` en 80d6b86, rama `docs/validaciones-manuales`.

## Fuentes y alcance

- Las secciones "Validaciones manuales pendientes" de `docs/superpowers/plans/*.md`: M1b (1-5), M2 (1-10), M3.0 (ninguna), M3a, M3b (1-10), M4 (remite a pendientes), M5a (1-10), M5b (11-21), M5c (22-35), M6a (1-6), M6b, M6c (1-9), v1 diferidos, diferidos B (36-41), diferidos L (42-43), ola 2 H, ola 2 M, pty-kill y tests-ui-jvm (1-3).
- `pendientes-m3-m5.md`: M4 (y la spec M4 §11), ola 1, ola 2 H, la revisión de seguridad (PROJECT_MEDIA), A4 (tarjetas) y bidi-host.
- **Lo que aún no está en `main`:** el texto "Automatizada en M2.5b" del plan de M2, las secciones "Validaciones que solo se pueden hacer en el Pixel" y "Validaciones ampliadas aún no hechas" de `pendientes-m3-m5.md` y el ledger de M2.5b (`.superpowers/sdd/2026-10-03-adbuddy-m2.5b-app-emulador/progress.md`) solo existen en la rama `feat/m2.5b-app-emulador` (1c35794, PR abierto, sin integrar a 80d6b86). Aquí se citan desde esa rama. "Hecha (M2.5b)" quiere decir "hecha en esa rama".
- `docs/safety.md` no tiene validaciones pendientes. Las specs (M3 §11, M4 §11, M5 §6, M6b §6, M6c §10, M2.5 §7) repiten lo que dicen los planes: no añaden filas. La única excepción es la spec M4 §11, que entra en las filas M4-a y M4-b.
- Las filas repetidas, que son la misma comprobación escrita en dos planes, se marcan con "=" y no se cuentan: M3a-c a M3a-g (= M3b-1 a M3b-6), M5c-35 (= M5b-18), UIJ-3 (= M5c-27/34), PTY-b (= O2M-a) y las de ola 2 H copiadas en pendientes.

## Resumen

**129 validaciones distintas**, sin contar las 4 filas repetidas. En el emulador propio:

- **33 son Sí.** Seis ya están hechas del todo: M2-1, M5b-20, M6c-8, desde la T3 M3b-6 y M5a-2, y desde la T5 VB-36. M3b-1 está hecha en ASCII (T3). La T5 hace además, en la API 34, M5a-5 (con un apaño por su hallazgo), M5a-9, y M5a-7 y M5a-8 en parte.
- **44 son Parcial.** De ellas, 18 tienen ya una parte hecha (M2-2, 3, 6 y 10 en Android 14; la lógica de M5b-11 a 17, 19 y 21; M5c-27 y 34 en la JVM; y en la T3, el canal de M2-7, M3b-4 y M6c-6). Lo que les falta casi siempre es Android 17.
- **52 son No.** 45 no dependen del móvil: son del PC o de la CI, y las corre un agente sin el usuario. 2 son revisiones de código o de texto (M6c-9 y O2M-c). Solo 5 necesitan de verdad a la persona, su cuenta o una red real: M1b-1, M5b-15, M5c-31, M6a-4 y M6a-6.

Leyenda:
- **Emulador:**
  - **Sí:** todo lo que pide la validación se puede comprobar en el emulador propio, o en la JVM si lo indica.
  - **Parcial:** una parte se comprueba en el emulador y otra necesita el Pixel o algo que no hay.
  - **No:** no se puede en el emulador. "No (PC)" y "No (CI)" son validaciones que no dependen del móvil.
  - "hecha" indica que un test ya la cubre.
- **El emulador propio** es el AVD `adbuddy-dev34`: imagen `system-images;android-34;google_apis;x86_64` rev 14, consola 5600 (siempre ≥ 5600), servidor adb propio 5060, `ADB_MDNS=0`, `ADBUDDY_EMU_STABLE_SECS=120`. Los instrumentados se lanzan por `am instrument` desde `host/internal/emutest`, nunca con `connected*`: UTP habla con el servidor 5037, el del móvil. Los tests usan el adb del emulador directamente, sin pasar por el shim.
- **Valor:**
  - **alto:** confirmaciones 🔴, guarda, consentimiento o fuga de códigos;
  - **medio:** una función que el usuario nota;
  - **bajo:** aspecto o una cifra informativa.
- **Orden:**
  - T1-T8: la tarea del plan de automatización que la cubre (al final);
  - Pixel o Usuario: entra en la checklist del usuario;
  - PC o CI: la corre un agente sin móvil;
  - Hecha: ya está;
  - fuera del tope: automatizable, pero no entra en las 8 tareas.

## Datos medidos (ledger de M2.5b y T1)

**Medido en el emulador propio** (rama `feat/m2.5b-app-emulador`, `progress.md`, apartados S1-S7, T2 y T3; 2026-10-02):

- **android-37.0 no sirve para la app.** La imagen `google_apis` rev 6 con el emulador 37.2.12 hace abortar a `surfaceflinger` cada 5-10 s (`Assertion failed: !rcEnc->featureInfo()->hasReadColorBufferDma`, hilo `RegionSampling`, en `mapper.ranchu.so`). Pasa con `swiftshader_indirect`, con `swangle_indirect` (que en Windows es el mismo SwiftShader) y con `host` (RTX 5080). El oráculo lo esquiva porque solo usa el shell.
- **android-34 aguanta.** Tras `sys.boot_completed`, los pid de `system_server` y `surfaceflinger` no cambian en 120 s y el buffer `crash` está vacío.
- **Cortar acceso y Reactivar** desde la app (R3): el emulador sale de `device` unos 2 s después del corte y vuelve solo a los ~17 s, sin `reconnect`. `adb_enabled` termina en 1.
- **Un corte que nadie deshace sobrevive al reinicio.** `adb_enabled=0` sigue en los datos del AVD (S7, `-no-snapshot` no lo borra) y hay que borrar `userdata-qemu.img` y `.qcow2`. De rebote, esto demuestra que **un `stop` y un `start` del AVD conservan sus datos**: un reinicio con datos sirve para la M5c-29.
- **El modo tcpip.** Tras `adb tcpip 5555`, el emulador vuelve como `emulator-5600` en ~2 s con `service.adb.tcp.port=5555`. `adb usb` lo devuelve en ~1 s con `service.adb.tcp.port=0`. adbd escucha **siempre** en 5555 porque es el transporte del emulador: los tests de tcpip tienen que mirar `service.adb.tcp.port`, no el socket.
- **Otros datos de la imagen:**
  - `su 0` funciona (userdebug, `context=u:r:su:s0`);
  - `pm grant … WRITE_SECURE_SETTINGS` funciona;
  - una app con targetSdk 37 se instala en API 34;
  - la app lee `adb_wifi_enabled` sin `SecurityException`;
  - `dumpsys activity services <pkg>` dice `isForeground=true`.

**Medido en la T1** (2026-10-03, en el mismo AVD; cada orden con su salida en `.superpowers/sdd/2026-10-03-validaciones-manuales/t1-mediciones.md`):

1. **El listener `exported="false"` enlaza.** `cmd notification allow_listener tech.hybriq.adbuddy/tech.hybriq.adbuddy.listener.AgentNotificationListener` lo enlaza (rc 0): sale en "Live notification listeners" y arranca el proceso. `disallow_listener` lo desenlaza y lo quita de `enabled_notification_listeners` en menos de 2 s.
2. **`cmd notification post`** admite `-S bigtext`, `bigpicture`, `inbox`, `messaging` y `media`, todas como `com.android.shell`, canal `shell_cmd` y `vis=PRIVATE`. Lo que no hace:
   - no rellena nunca `tickerText` ni deja elegir la visibilidad;
   - `-S bigtext` deja `android.bigText=null` y pone el texto en `android.text`;
   - `-S messaging` sí rellena `android.text` con el último mensaje.

   Por eso BigText, MessagingStyle sin `EXTRA_TEXT`, el ticker y la visibilidad van con notificaciones propias del instrumentado.
3. **La imagen trae** `com.google.android.apps.messaging`, `com.google.android.gm` y `com.google.android.gms`. El rol SMS lo tiene `com.google.android.apps.messaging`: la app de SMS por defecto ya es otra que ADBuddy.
4. **`cmd role set-bypassing-role-qualification` existe, pero no salta la cualificación del rol SMS.** `add-role-holder android.app.role.SMS tech.hybriq.adbuddy` falla (rc 255, "not qualified … missing … SMS_DELIVER … BROADCAST_SMS") con bypass y sin él. Una app de pruebas que vaya a ser titular tiene que declarar los componentes del rol.
5. **El perfil gestionado funciona sin DPC.**
   - `pm list features` trae `android.software.managed_users` (máximo 4 usuarios).
   - `pm create-user --profileOf 0 --managed adbuddy-work` crea el usuario 10, y `am start-user -w 10` lo arranca.
   - `pm install-existing --user 10 <pkg>` mete la app en el perfil, y `pm remove-user 10` lo limpia.
6. **`adb install` no queda restringida en la API 34.** `appops get … ACCESS_RESTRICTED_SETTINGS` da "No operations, default allow", con `packageSource=1` (OTHER). El interruptor del acceso a notificaciones sale `enabled=true`.
7. **El bloqueo con PIN se maneja entero por shell.**
   - `locksettings set-pin 1111`, `KEYCODE_SLEEP` (Asleep) y `KEYCODE_WAKEUP` dejan `isKeyguardShowing=true` y `deviceLocked=1`.
   - `cmd statusbar expand-notifications` abre la persiana sobre el bloqueo.
   - Un `input swipe` hacia arriba saca el bouncer ("Enter your PIN", `pinEntry`), e `input text 1111` con `KEYCODE_ENTER` desbloquea (`deviceLocked=0`).
   - `locksettings clear --old 1111` lo deja como estaba, con `adb_enabled=1`.
8. **Burbujas: `notification_bubbles` global es `null` y secure es `1`.** La preferencia de una app recién instalada es "nada", y en los tests se cambia con `cmd notification set_bubbles <pkg> 1`.
   - **La notificación de conversación de ADBuddy no puede ser burbuja** ni con la preferencia en "todas": `mAllowBubble=false`, `isBubble=false` y el sistema le quita el `BubbleMetadata`. Es la del servicio en primer plano (`flags=0x6a`).
   - La misma notificación sin servicio en primer plano (sonda temporal, mismo canal, atajo y metadata) sí lo es (`flags=0x1000`, `isBubble=true`). Sale en la ventana `Bubbles!`, y un `input tap` abre `ChatBubbleActivity` en `multi-window`. Ver "Hallazgos de la T1".
9. **El listener se reenlaza solo** tras `am force-stop` (al momento, sin abrir la app), tras `su 0 kill -9` (el sistema lo reinicia en 1 s) y tras `adb reboot` (unos 6 s después del arranque, al segundo intento).
   - `am kill` **no mata** el proceso, porque lo retiene el listener enlazado: para el caso "tras kill" hay que usar `su 0 kill -9`.
   - `adb reboot` con `-no-snapshot` conserva los datos.
10. **Idioma.**
    - `cmd locale set-app-locales tech.hybriq.adbuddy --locales es|en` cambia el idioma sin reiniciar el proceso y renombra los canales al momento. Se ve con `dumpsys notification --noredact`; el redactado recorta los nombres a 3 letras.
    - `--locales` vacío vuelve al idioma del sistema.
    - El del sistema no tiene orden de shell: se cambia con `su 0 setprop persist.sys.locale es-ES` y `adb reboot`, y se restaura con `setprop persist.sys.locale ""` y otro reinicio.
11. **La pantalla partida se monta por shell.**
    - `am supports-split-screen-multi-window` da `true`.
    - `dumpsys activity service SystemUIService WMShell splitscreen moveToSideStage <taskId> 1` parte la pantalla: ADBuddy queda en `[0,0][1080,1187]` y Ajustes en `[0,1213][1080,2400]`, las dos `multi-window`.
    - `removeFromSideStage` no deja la pantalla limpia: la salida la mide la T5.
12. **El transporte del emulador no cambia** al poner `adb_wifi_enabled=1`; antes hay que aceptar "¿Permitir la depuración inalámbrica en esta red?".
    - **El usuario shell no puede leer `service.adb.tls.port`**: `getprop` devuelve una cadena vacía con rc 0 y un `avc: denied { read } … adbd_prop` para `shell`. Con `su 0` da el puerto (el mismo de Ajustes y del socket en escucha). Ver "Hallazgos de la T1".
    - El diálogo "Emparejar con código" (código de 6 cifras, dirección y puerto) se lee con `uiautomator dump`, llegando por Ajustes > Sistema > Opciones para desarrolladores.

Fuera de la lista también se midió:
- `appops set <pkg> ACTIVATE_VPN allow` se acepta.
- `cmd clipboard` no existe en la API 34 ("No shell command implementation").
- `PICK_IMAGES` abre el Photo Picker, y `OPEN_DOCUMENT` abre DocumentsUI y lista un fichero empujado a `/sdcard/Download`.

## Defectos del encargo (explicados, no rodeados)

- **"Automatizada en M2.5b" no está en `main`.** Está en `feat/m2.5b-app-emulador`, con el PR abierto. Si ese PR cambia antes de integrarse, hay que revisar las filas M2-1, 2, 3, 6 y 10 y la M6c-8.
- **"ADBuddy como app de SMS por defecto" no responde a la 33.** La 33(a) pregunta qué devuelve `Telephony.Sms.getDefaultSmsPackage` cuando **otra** app es la de SMS por defecto. Además, ADBuddy no declara los componentes del rol SMS, así que no lo cumple. Para probarlo hace falta una app de pruebas que tenga el rol (T7). La 24 no tiene que ver con el rol SMS: trata los ajustes restringidos del acceso a notificaciones.
- **La M5b-17 (a) y (b) se reclasifica.** test/m5b-auto las daba por "solo en el Pixel (D)", pero en la API 34 se pueden probar con el servicio de accesibilidad vinculado y una app de pruebas propia (T7). El Pixel solo confirma Android 17.

## Hallazgos de la T1 (defectos probables del producto; los adjudica el controlador)

- **La burbuja de ADBuddy no sale en la API 34 (T1-8).**
  - La notificación de conversación es la del servicio en primer plano de `BridgeService` (`ConversationNotifier.build` la devuelve para `startForeground`). El sistema no deja que sea burbuja: `mAllowBubble=false` y le quita el `BubbleMetadata`, aunque la preferencia de la app esté en "todas".
  - La misma notificación publicada aparte, sin el flag de servicio en primer plano, sí es burbuja y abre `ChatBubbleActivity`.
  - Afecta a la parte de la burbuja de M3b-5, M3b-8, M5c-34 y VB-38. Probablemente pasa igual en Android 17, pero no se ha medido.
  - Arreglo barato posible: publicar la conversación en una notificación aparte de la del servicio. Lo decide el controlador.
  - Mientras tanto, la T3 y la T5 pueden probar la guarda y la terminal con la burbuja abriendo `ChatBubbleActivity` desde una notificación de prueba sin servicio en primer plano, como hizo la sonda de la T1.
- **`getprop service.adb.tls.port` no da el puerto al usuario shell en la API 34 (T1-12).**
  - SELinux deniega la lectura (`adbd_prop`), y `getprop` devuelve una cadena vacía con rc 0. Por eso `readTLSPort` (`host/internal/cli/phone_remote.go`) devolvía 0 (ya no existe desde `fix/m5b-tls-port`).
  - El respaldo, `info.WifiADBPort`, no lo envía la app: `DeviceInfoSender` solo rellena `tailscaleIp`. Así que `tailscale_port` nunca se guarda.
  - Afecta a M5b-11 y, por arrastre, a M5b-12. En un Pixel de producción no hay `su`. Falta confirmarlo en Android 17.
  - **Corregido en `fix/m5b-tls-port`** (plan `2026-10-03-adbuddy-m5b-tls-port.md`): el host ya no usa `getprop`; toma el puerto del serial activo (`ip:puerto`, solo si `adb devices` lo da como `device`) o del servicio mDNS `_adb-tls-connect._tcp` de la misma instancia. adbd cambia de puerto al reiniciarse (`adb tcpip`, `adb usb`): el guardado vale hasta el siguiente `device.info`.

## Hallazgos de la T5 (los adjudica el controlador)

- **Sin la app, la guarda bloquea en todas las pantallas de la API 34.**
  - La ventana del fondo de pantalla se llama `com.android.systemui.wallpapers.ImageWallpaper`. Es de systemui y está siempre visible (`mViewVisibility=0x0`), también detrás de una app opaca. `fixedSystemUI` (`host/internal/uidump/window.go`) tiene `ImageWallpaper`, pero no este título.
  - Por eso `ParseSystemUIWindows` la da como desconocida en los 17 estados, y la guarda sin la app responde «a system window is on screen (com.android.systemui.wallpapers.ImageWallpaper)» a todo lo que toca o lee la pantalla, también con Ajustes delante y nada encima.
  - Es seguro, porque falla cerrado, pero deja inservible el respaldo por `adb` sin la app (M5a-5 necesitó un apaño). Muestra mínima, de `settings.windows.txt`:

    ```
      Window #13 Window{7e8b51b u0 com.android.systemui.wallpapers.ImageWallpaper}:
        mOwnerUid=10168 showForAllUsers=true package=com.android.systemui appop=NONE
        mViewVisibility=0x0 mHaveFrame=true mObscured=false
    ```
  - Arreglo barato posible: añadir ese título a `fixedSystemUI`; hay que comprobar antes que el fondo de pantalla nunca muestra texto de nadie. Los tests esperan hoy el bloqueo, con `TODO(adjudicar)`: `TestAPI34SystemUIWindows`, `TestTheGuardOnAPI34Screens` y `wallpaperReason` en `guard_test.go`. Con el arreglo, esos casos vuelven a «se ejecuta».
- **No son defectos, pero conviene saberlo:**
  - Con la pantalla apagada o bloqueada no hay `topResumedActivity`, pero `ResumedActivity` sigue nombrando la app de debajo, así que la guarda cree que esa app está delante. En el bloqueo, el foco `NotificationShade` bloquea igualmente.
  - Una burbuja plegada (`Bubbles!`) cuenta como ventana desconocida y bloquea sin la app.
  - En la API 34, un `uiautomator dump` no desvincula el servicio de accesibilidad.

## Tablas

### M1b · Instalación y conexión (1-5)

| # | Qué se valida | Emulador | Cómo | Por qué no / paso mínimo | Valor | Orden |
|---|---|---|---|---|---|---|
| M1b-1 | `adbuddy setup` real con el QR de "Emparejar con código QR" | No | — | Hacen falta la cámara del Pixel y el emparejamiento TLS real. Paso: escanear el QR una vez, que es la propia instalación. | medio | Pixel |
| M1b-2 | `setup --code` con dirección y código | Parcial | Medido (T1-12): el diálogo "Emparejar con código" de la Depuración inalámbrica del emulador se lee con `uiautomator dump` (código de 6 cifras, dirección y puerto), llegando por Ajustes > Sistema > Opciones para desarrolladores y aceptando antes "¿Permitir … en esta red?". El emparejamiento en sí no se ha probado. Con `ADB_MDNS=0`, la dirección se da a mano. | Los menús reales de Android 17. Paso: si el QR falla, emparejar por código. | medio | Pixel |
| M1b-3 | El QR se escanea en Windows Terminal y en Git Bash (mintty), con tema claro y oscuro | No (PC) | Un test de host que decodifique el QR del texto impreso cubre la codificación, no la fuente de la terminal. | Depende de la fuente de cada terminal y de la cámara. Paso: junto con M1b-1, en las dos terminales. | bajo | Pixel |
| M1b-4 | La descarga real de platform-tools 37.0.1 pasa el SHA-1 y `serve` arranca con ese adb | No (CI) | Un test de host con red, opcional y activado por variable, en la CI. | Necesita red, no el móvil. | medio | CI |
| M1b-5 | Los textos de la guía coinciden con los menús de Android 17 en inglés y en español | Parcial | `uiautomator dump` de las opciones de desarrollador en la API 34, en `en` y `es`. | Android 17 y sus capturas, solo en el Pixel. Paso: las capturas, al publicar la guía. | bajo | Pixel |

### M2 · App base (1-10)

| # | Qué se valida | Emulador | Cómo | Por qué no / paso mínimo | Valor | Orden |
|---|---|---|---|---|---|---|
| M2-1 | `TokenProvider` instrumentado | Sí, hecha (M2.5b) | `TokenProviderTest` por `am instrument` desde `TestInstrumentedSuite`, y `TestTheShellUserIsAnswered` | — | alto | Hecha |
| M2-2 | `content write` por stdin en Android 11-17 | Parcial, hecha en la API 34 | `TestTheHandshakeOnStdinReachesTheApp` | La imagen 37 entra en bucle y la 11-13 no está instalada. Paso: ninguno aparte; el primer `serve` en el Pixel lo demuestra, porque sin handshake no hay canal. | alto | Hecha, y Pixel (implícita) |
| M2-3 | El proveedor arranca el servicio en primer plano | Parcial, hecha en la API 34 | `TestTheServiceIsInTheForegroundAfterTheHandshake` | Las restricciones de servicios en primer plano cambian en 15-17. Paso: implícito, en el primer `serve` con la app en segundo plano. | alto | Hecha, y Pixel (implícita) |
| M2-4 | Accesibilidad por adb en un Android 13-15 de otro fabricante, con otro servicio ya activo | Parcial | T7: el servicio de la app de pruebas se activa antes, y se comprueba que `enabled_accessibility_services` lo conserva tras `setup`. | No hay imagen de otro fabricante. Paso: opcional, con un móvil de otra marca. | medio | T7 / opcional |
| M2-5 | Con el móvil bloqueado, Permitir desde la notificación pide desbloquear y Rechazar no | Parcial | T4: PIN (T1-7), `cmd statusbar expand-notifications`, `uiautomator dump` e `input tap` sobre cada botón. El arnés (T3) ve si llega `confirm.response`. | Android 17 y la biometría real. Paso: con el Pixel bloqueado, Rechazar y Permitir una pregunta de prueba. | alto | T4 + Pixel |
| M2-6 | Cortar acceso y Reactivar, con y sin tcpip | Parcial, hecha en local en la API 34 | `TestCutAccessAndReactivate` con `ADBUDDY_EMULATOR_CUT=1`. La CI no la corre: un corte sin deshacer deja el AVD sin adb (S7). | Faltan la depuración inalámbrica real y ver que el puerto tcpip queda cerrado; en el emulador, el 5555 es su transporte. Paso: Cortar y Reactivar una vez en el Pixel. | alto | Hecha + Pixel |
| M2-7 | Extremo a extremo: `setup --apk`, `serve`, una 🔴 y Permitir | Parcial, el canal hecho (T3) | Hecho en T3: `TestTheLinkCarriesDeviceInfo` (`host/internal/emutest/link_test.go`) instala el APK de debug confiado como `setup --apk`, abre el bridge en 127.0.0.1, hace `adb reverse` y `phoneapp.Handshake`, y recibe por el WebSocket con token el `hello` (versión del protocolo y de la app instalada) y el `device.info` con `protectedApps`. Falta la pregunta 🔴 y Permitir: T4, con el mismo arnés. | El agente real (ruling de M2.5b: cuesta mucho y demuestra poco más). Paso: implícito, en el uso real. | medio | T4 (el canal, hecho en T3) |
| M2-8 | Permitir tras desbloquear desde el bouncer | Parcial | T4: tocar Permitir bloqueado, teclear el PIN en el bouncer (`input text` y `KEYCODE_ENTER`) y comprobar que llega `allow` con `isDeviceLocked` ya en falso. | Android 17 y la biometría. Paso: junto con M2-5. | alto | T4 + Pixel |
| M2-9 | La confirmación no llega a relojes ni espejos (`setLocalOnly`) | Parcial | T4: `dumpsys notification --noredact` muestra el flag `LOCAL_ONLY` de la pregunta. | Un reloj Wear o Phone Link reales. Paso: opcional, si el usuario tiene uno. | medio | T4 / opcional |
| M2-10 | El `ContentObserver` de `adb_wifi_enabled` recibe los cambios | Parcial, hecha en la API 34 | `AdbWifiObserverTest` | Que una app con targetSdk 37 pueda leer el ajuste en Android 17. Paso: junto con M2-6. | medio | Hecha + Pixel |

### M3a · Host del chat

| # | Qué se valida | Emulador | Cómo | Por qué no / paso mínimo | Valor | Orden |
|---|---|---|---|---|---|---|
| M3a-a | `adbuddy serve` lanza Claude Code | No (PC) | En el PC con `ADBUDDY_REAL_CLAUDE=1` y una configuración temporal (como en M4). | Claude Code real; no toca el móvil. | medio | PC |
| M3a-b | `adbuddy attach` y Ctrl+], con dos terminales a la vez | No (PC) | Test de `cli` con dos clientes IPC. | Terminales reales. | bajo | PC |
| M3a-c a g | Chat, terminal, ficheros, captura y reconexión | = M3b-1 a 6 | — | — | — | — |
| M3a-f | `adbuddy send` desde Claude Code | No (PC) | `ADBUDDY_REAL_CLAUDE=1`; la mitad del móvil va en M3b-3. | Claude Code real. | bajo | PC |
| M3a-h | `/exit` llega a `stopped` aunque un nieto con `setsid` tenga abierto el esclavo (Linux y macOS) | No (CI) | `internal/pty` en `host.yml` | No es del móvil. | medio | CI |
| M3a-i | Los tests solo de Unix pasan en la CI de Linux y de macOS | No (CI) | `host.yml` | — | medio | CI |
| M3a-j | `--continue` sin conversación previa en Windows: cuánto tarda en fallar y el reintento | No (PC) | `ADBUDDY_REAL_CLAUDE=1` | Claude Code real. | bajo | PC |

### M3b · App del chat (1-10)

| # | Qué se valida | Emulador | Cómo | Por qué no / paso mínimo | Valor | Orden |
|---|---|---|---|---|---|---|
| M3b-1 | Un mensaje multilínea con Unicode llega como uno solo; cortar el canal deja "No enviado" y Reintentar no duplica | Sí; hecha en ASCII (T3) | Hecho en T3: `TestTheChatSendsOneMessage`. Dos líneas escritas en el chat (`input text` y `KEYCODE_ENTER`) llegan en un solo `chat.send`; con el `chat.send` en vuelo, parar el listener del arnés deja "Not sent" y "Retry"; tras reconectar, Retry reenvía el **mismo id** (el host lo toma como el mismo mensaje en `onChatSend`) y el chat lo muestra una vez. Falta el Unicode: la API 34 no tiene portapapeles por shell (T1) y el `type` de ADBuddy (M5a-2) no sirve aquí, porque la app se niega a actuar sobre sí misma ("ADBuddy does not act on itself", `UiExecutor`). Queda para un instrumentado que escriba en el campo del chat. | — | medio | Hecha en ASCII; Unicode, instrumentado (fuera del tope) |
| M3b-2 | Terminal: colores, repintado, teclas rápidas, giro, vuelta tras perder la conexión, scroll y el enlace que pide confirmar el dominio | Parcial | T3: WebView real; giro con `settings put system user_rotation 1`; teclas con `uiautomator dump` e `input tap`; el diálogo del dominio se ve en el volcado. | El juicio visual de los colores. Paso: mirar la terminal en la pasada visual. | medio | T3 + Pixel |
| M3b-3 | Ficheros: ida a `inbox/` con hash, 100 MiB que se cortan y siguen, `send` de vuelta, abrir y compartir, caché ≤ 500 MiB | Sí | T3: el arnés hace de host; el corte se hace parando el listener; la caché se mide con `run-as tech.hybriq.adbuddy du` (build de debug). El fichero se elige con UiAutomator: la app usa `OpenDocument` (DocumentsUI, que lista en Recientes un fichero empujado a `/sdcard/Download`) y `PickVisualMedia` (el Photo Picker se abre; sin medios sale vacío). Medido en la T1. | — | medio | T3b (pendiente) |
| M3b-4 | Captura desde el botón y desde la notificación; con Cortar acceso, el texto del host | Parcial, hecha (T3) | Hecho en T3: `TestTheScreenshotGoesToTheHost`. El botón (Adjuntar, Captura) y la acción "Send screenshot" de la notificación (persiana con `cmd statusbar expand-notifications`) mandan `capture.request`; el arnés contesta `capture.ready` y la captura va en el siguiente `chat.send`; con el rechazo del acceso cortado (`failed` y un texto que es copia literal de `phone_files.go`), ese texto sale en pantalla. | Un corte real deja el AVD sin adb (S7): el caso "cortado" se simula con la respuesta del host. | bajo | Hecha |
| M3b-5 | Notificación: "Trabajando…", responder bloqueado, Enviar captura, Pausar, Cortar acceso sin desbloquear, burbuja, "Listo" a los ≥ 10 s y silenciar el canal | Parcial | T4 para el bloqueo; T3 para las acciones con UiAutomator; canales con `dumpsys notification --noredact`. Burbuja (T1-8): la notificación de conversación es la del servicio en primer plano y en la API 34 no puede ser burbuja (ver "Hallazgos de la T1"); hasta que se arregle, esa parte es No en el emulador. | La burbuja: defecto que hay que adjudicar, y luego Android 17. Paso: abrirla una vez en el Pixel. | alto | T4 + Pixel; burbuja, defecto |
| M3b-6 | Reconexión con la app en segundo plano y con el servicio recién reiniciado | Sí, hecha (T3) | `TestTheLinkComesBack`, con la app sin abrir: el host para y vuelve en el mismo puerto y la app reconecta sola (nuevo `hello` y `device.info`); `su 0 kill -9` del proceso (T1-9: `am kill` no lo mata) y el servicio reiniciado reconecta con el enlace guardado, sin handshake; tras `am force-stop`, otro handshake devuelve el canal y `BridgeService` vuelve al primer plano. | — | medio | Hecha |
| M3b-7 | Diseño en claro, oscuro y letra grande, color dinámico en 17, paleta propia en 11, barras y teclado | Parcial | JVM (Roborazzi) de las pantallas que faltan: chat, terminal, actividad y consentimiento. En la API 34: `cmd uimode night yes` y `settings put system font_scale 2.0`; insets y teclado con `uiautomator dump`. | Android 17, y Android 11 sin imagen. Paso: la pasada visual. | bajo | Pixel (la JVM, fuera del tope) |
| M3b-8 | Nada de M2 se debilita: la 🔴 en el chat y en la notificación, Pausar y Cortar; con la burbuja sobre otra app y una 🔴, `input tap` y `uiautomator dump` del agente quedan bloqueados | Parcial, la guarda sin la app hecha (T5) | Hecho en T5 (`TestTheGuardWithTheBubble`, `host/internal/emutest/guard_test.go`): la burbuja de una notificación de prueba sin servicio en primer plano (`GuardOnEmulatorTest`, `-e adbuddyGuard bubble`; la de ADBuddy no puede ser burbuja, T1-8) sobre Ajustes. Plegada, la ventana `Bubbles!` de systemui hace bloquear. Abierta, `Resumed()` da `ChatBubbleActivity` primero (y Ajustes debajo), y el daemon real del host sobre el adb del emulador, sin la app, bloquea `input tap`, `uiautomator dump`, `screencap` y `adbuddy tap --xy` con «ADBuddy itself is in front», con la accesibilidad apagada y encendida (`EnableAccessibility`). Muestras `bubble-*` en `uidump/testdata/api34`. **No cubre:** la 🔴 (T4); `guardedOnScreen` de la app, porque la guarda con la app conectada no se ha probado aquí (el test lee `dumpsys`); y la burbuja real de ADBuddy (defecto T1-8). | El formato de `dumpsys` de Android 17. Paso: la pasada de solo lectura. | alto | Hecha la guarda (T5) + T4 |
| M3b-9 | Con el diálogo de confianza de Claude Code en pantalla, `chat.send` da "busy" | No (PC) | `ADBUDDY_REAL_CLAUDE=1` en una carpeta nueva. | Claude Code real. | medio | PC |
| M3b-10 | Sin internet, la WebView de la terminal sigue y nada pide red | Sí | T3: `svc wifi disable` y `svc data disable`; el adb del emulador no va por esa red. | — | bajo | T3b (pendiente) |

### M4 · Adaptador de Claude Code (pendientes "M4" y spec §11)

| # | Qué se valida | Emulador | Cómo | Por qué no / paso mínimo | Valor | Orden |
|---|---|---|---|---|---|---|
| M4-a | `Notification` y sus tipos; permisos de Edit, Write, WebFetch y MCP; deny con `message`; `/clear`, `/compact` y `--resume`; prompts con `/` y `!` | No (PC) | `ADBUDDY_REAL_CLAUDE=1`, grabando en `claudecode/testdata`. | Claude Code real. La vista en el móvil va con el arnés T3. | medio | PC |
| M4-b | Si se responde en la terminal, ¿llega siempre el cierre del hook o el fin de la herramienta, y cuánto tarda? | No (PC) | Igual que M4-a. | Claude Code real. | alto | PC |
| M4-c | Detalle largo de la tarjeta: Sí y "Sí, siempre" inactivos hasta el final, con letra grande, giro y teclado; No siempre disponible | Sí (JVM) | T4: Robolectric con Compose ui-test (la infraestructura de tests-ui-jvm): `PermissionCard` con `font_scale` 2 y los calificadores de giro. | — | alto | T4 |
| M4-d | El aviso de permiso es silencioso (canal LOW) y en el bloqueo no muestra texto | Parcial | T4: `dumpsys notification` (importancia y `publicVersion`). | El aspecto. Paso: la pasada visual. | medio | T4 |

### M5a · Ayudas (1-10)

| # | Qué se valida | Emulador | Cómo | Por qué no / paso mínimo | Valor | Orden |
|---|---|---|---|---|---|---|
| M5a-1 | `screen` tarda menos de 500 ms | Parcial | T3 y T5: medirlo en el emulador como guarda de regresión. | La cifra que vale es la del Pixel en su Wi-Fi. Paso: un `adbuddy screen` cronometrado. | bajo | Pixel |
| M5a-2 | `type "¿Qué tal? 👋"` en un campo de texto | Sí, hecha (T3) | `TestTypeWritesUnicodeIntoAField`: accesibilidad activada como en `setup` (`phoneapp.EnableAccessibility`), un `ui.request` `setText` con "¿Qué tal? 👋" por el canal real en la búsqueda de Ajustes (`android.settings.APP_SEARCH_SETTINGS`), y `uiautomator dump` lee ese texto en el campo. El test envía el `ui.request` por el canal; no pasa por la CLI `adbuddy type` ni por el argv Unicode del PC. Los ajustes de accesibilidad se restauran al final. | — | medio | Hecha |
| M5a-3 | Superposición: aspecto, la píldora pausa, la marca y que no intercepta toques | Parcial, hecha salvo el aspecto (T5) | Hecho en T5 (`TestTheOverlayOnTheEmulator`, `overlay_test.go`), con el arnés de T3 y la accesibilidad encendida. Con la superposición en pantalla (sus dos ventanas de `tech.hybriq.adbuddy` visibles en `dumpsys window windows`), un `input tap` sobre «Network & internet» atraviesa el marco y abre esa pantalla de Ajustes, como muestra el volcado de la app. Un `input tap` en la píldora hace que la app mande `pause` al host y quite la superposición. **No cubre:** la marca, ni que la app se tenga por pausada después: eso lo decide el estado de sesión que manda el host, y el arnés no lo manda. | El aspecto. Paso: la pasada visual. | medio | Hecha (T5) + Pixel (aspecto) |
| M5a-4 | Con una app de banca delante, `input tap`, `screencap`, `screen` y `tap` se bloquean | Parcial | T7: una app de pruebas con nombre de banca, que la heurística propone; la guarda del host se prueba contra el emulador. | Una app de banca real (FLAG_SECURE, detección de emulador). Paso: abrir su app de banca y pedir un `adbuddy screen`; debe responder Blocked. | alto | T7 + Pixel |
| M5a-5 | Respaldo: sin accesibilidad usa `uiautomator`; con ella y la app desconectada, `UI_UNAVAILABLE` | Sí, hecha con un apaño (T5) | Hecho en T5 (`TestTheFallbackWithoutTheApp`): el daemon real sobre el adb del emulador. Con el servicio apagado, `adbuddy screen` sale por `uiautomator` (el volcado de Ajustes). Con el servicio encendido y vinculado, da `UI_UNAVAILABLE`, y el servicio sigue vinculado. **El apaño:** la guarda que va delante bloquearía siempre sin la app (hallazgo del fondo de pantalla, «Hallazgos de la T5»), así que el test simula solo la respuesta `foreground` de la app (Ajustes), y todo lo demás da «no hay app». Medido de paso: en la API 34, un `uiautomator dump` con el servicio encendido no lo desvincula. | — | medio | Hecha (T5); sin apaño cuando se adjudique el hallazgo |
| M5a-6 | `dumpsys activity activities` en 11, 13-15 de otro fabricante y 17 | Parcial, hecha en la API 34 (T5) | Hecho en T5: `uidump/testdata/api34/*.activities.txt` (17 estados) y `TestAPI34Resumed`. Cubre Ajustes, ADBuddy, el teclado, la pantalla partida (las dos mitades, por el `topResumedActivity` de cada tarea), la burbuja abierta (dos tareas), la persiana, el heads-up, y la pantalla apagada y el bloqueo (sin `topResumedActivity`, con el `ResumedActivity` de la app de debajo). Si se quita `topResumedActivity` del patrón, la pantalla partida pierde una mitad (mutación). | 11 y 13 sin imagen, otros fabricantes y 17. Paso: la pasada de solo lectura. | alto | Hecha en 34 (T5) + Pixel |
| M5a-7 | Ventanas: diálogo, teclado y pantalla partida; `tap --xy` sobre la barra con una 🔴 pendiente da `protected` | Sí, hecha en parte (T5) | Hecho en T5. **La guarda sin la app** (`TestTheGuardOnRealScreens`): con el diálogo de volumen de systemui sobre Ajustes (una tecla de volumen con el panel oculto solo lo muestra; se comprueba que el volumen no cambia), `VolumeDialogImpl` sale como ventana desconocida y bloquea. Con el teclado en la búsqueda de Ajustes, la ventana `InputMethod` no es de systemui y no cuenta. Con la pantalla partida (ADBuddy y Ajustes, `WMShell splitscreen moveToSideStage`), se ven las dos mitades, se bloquea con «tech.hybriq.adbuddy is a protected app» e Inicio tampoco pasa. **La salida medida:** Inicio, esperar a que acabe y abrir una app deja las dos tareas a pantalla completa (un `am start` durante la animación vuelve a montar la pareja). **La 🔴** (`TestTheStatusBarWithAQuestionWaiting`, con el arnés y la accesibilidad): un `tap --xy` en la barra de estado con una pregunta esperando da `protected` («A question of ADBuddy is waiting on the phone»). **No cubre:** que `screen` muestre cada ventana en su `window` (`dialog`, `ime`, `split`), porque no se ha mirado el volcado de la app en esos estados. | — | alto | Hecha en parte (T5); falta el `window` del volcado de la app |
| M5a-8 | Marco: dos `tap --xy` seguidos no dan `protected`; un `tap` por texto con `ACTION_CLICK` fallido toca; el marco no sale en `getWindows()` | Sí, hecha en parte (T5) | Hecho en T5 (`TestTheOverlayOnTheEmulator`): dos `tap --xy` seguidos en un punto libre de Ajustes, con la superposición visible, no dan `protected`. Eso es lo que pasaría si el marco saliera en `getWindows()`: la mutación de quitarle `FLAG_NOT_TOUCHABLE` hace que salga y que el toque dé «ADBuddy does not act on itself». No hay instrumentado que consulte `getWindows()`: se comprueba su consecuencia. **No cubre:** el `tap` por texto con `ACTION_CLICK` fallido, porque en Ajustes no hay ningún elemento cuyo clic falle; necesita la app de pruebas (T7). | — | medio | Hecha en parte (T5) + T7 |
| M5a-9 | `ServiceOn` y `Asleep` contra salidas reales | Sí, hecha en la API 34 (T5) | Hecho en T5: `TestAPI34Asleep`, con `dumpsys power` con la pantalla encendida, apagada (`KEYCODE_SLEEP`), en Doze (`dumpsys deviceidle force-idle`) y bloqueada encendida y apagada. `TestAPI34ServiceOn`, con `dumpsys accessibility` con el servicio apagado, encendido, y encendido con la burbuja abierta. `ServiceOn` lee `dumpsys accessibility`, que es lo que usa `daemon/ui.go`, no `settings get secure enabled_accessibility_services` como decía la fila. En el emulador no sale `Dozing` (no hay pantalla ambiente): en Doze, `mWakefulness=Asleep`. | El formato de 17. Paso: la pasada de solo lectura. | alto | Hecha en 34 (T5) + Pixel |
| M5a-10 | Notificaciones de apps protegidas en la persiana o en heads-up dentro de `screen` y `screenshot` (límite; decidir en la spec §4.3) | Parcial | T7: un heads-up de la app de pruebas protegida y `screen`; ¿salen sus nodos de `com.android.systemui`? | La decisión es de producto. Sin paso del usuario: decide el coordinador con la medida. | medio | T7 |

### M5b · Modo remoto (11-21)

| # | Qué se valida | Emulador | Cómo | Por qué no / paso mínimo | Valor | Orden |
|---|---|---|---|---|---|---|
| M5b-11 | `getprop service.adb.tls.port` da el puerto vigente en Android 17 | Parcial, lógica hecha (`TestDeviceInfoTakesThePortFromTheConnection`, `TestTheSavedPortIsACandidateAddress`); el `getprop` que fallaba en la API 34 ya no se usa (`fix/m5b-tls-port`) | Medido (T1-12): con `adb_wifi_enabled=1`, `adb shell getprop service.adb.tls.port` devuelve una cadena vacía (SELinux: `avc: denied { read } … adbd_prop` para `shell`), mientras que `su 0 getprop` da el puerto de Ajustes. `readTLSPort` devuelve 0 y la app no manda `wifiAdbPort` (ver "Hallazgos de la T1"). | Defecto corregido en `fix/m5b-tls-port` (el puerto sale de la conexión del host, no de `getprop`); queda comprobar en el Pixel que `tailscale_port` es el de Ajustes, que `adb connect <ip-tailscale>:<tailscale_port>` conecta, y el puerto de Ajustes antes y después de `remote on`, `remote off` y del `adb usb` del invariante (validaciones 1 a 3 de ese plan). Paso: la pasada de solo lectura, y la parte del modo remoto junto con M5b-12. | medio | Pixel; defecto |
| M5b-12 | `tcpip 5555` por Tailscale con solo datos; `remote on` y `off`; caducidad | Parcial, lógica hecha (adb falso) | T8: `remote.New(adb.Runner{Serial: emu})`, `Enable` y `Disable`, leyendo `service.adb.tcp.port` (medido en S5). | Tailscale con datos móviles. Paso: fuera de casa o con el Wi-Fi apagado, `remote on`, y `serve` reconecta. | medio | T8 + Pixel |
| M5b-13 | Segundos que tarda en verse el estado tras `adb tcpip`; "desconocido" nunca se muestra como "apagado" | Parcial, lógica hecha (`BridgeStateTest`) | T8: medido con el arnés. | La cifra del Pixel, informativa. Sin paso. | bajo | T8 |
| M5b-14 | La persiana y los ajustes rápidos dan `NotificationShade`; la pantalla de bloqueo; persianas de otros fabricantes | Parcial, hecha en la API 34 (T5) | Hecho en T5 (`TestTheGuardOnRealScreens`, `TestAPI34Focus`, `TestTheGuardOnAPI34Screens`). Con `cmd statusbar expand-notifications` y con `expand-settings`, el foco es `NotificationShade` y su ventana está visible. El daemon bloquea `screencap`, `input tap`, `uiautomator dump`, `adbuddy screen` y `keyevent --longpress 3` con «the notification shade is open», y Back pasa y la cierra (desde los ajustes rápidos hacen falta dos: el primero vuelve a la persiana). **(a) El bloqueo con PIN:** el foco es `NotificationShade`; se bloquea todo salvo Back, Home y, con la pantalla apagada, la tecla de despertar exacta (224); con la pantalla encendida, 224 se bloquea; Back y Home no desbloquean. Muestras `shade`, `quick-settings`, `locked-off` y `locked`. | El formato de 17 y otros fabricantes. Paso: la pasada de solo lectura con la persiana bajada. | alto | Hecha en 34 (T5) + Pixel |
| M5b-15 | Pasar de la Wi-Fi a datos en mitad de una sesión | No, cota hecha (`TestWatchPhoneSwitchBudget`, 68 s) | — | Un cambio real de red con Tailscale. Paso: junto con M5b-12, apagar el Wi-Fi con `serve` en marcha. | medio | Pixel |
| M5b-16 | `NetworkCallback` de la VPN | Parcial, lógica hecha (`IpReporterTest`) | T7: un `VpnService` en la app de pruebas, con `appops set <pkg> ACTIVATE_VPN allow` (se acepta por shell, T1). | Tailscale real. Paso: junto con M5b-12. | bajo | T7 / Pixel |
| M5b-17 | Heads-up (bloquea el respaldo por adb): muestra de `dumpsys window windows`; (a) evento sin `Notification`; (b) actualización con el mismo id; (c) "tiempo para actuar" | Parcial, ventana de 8 s hecha (`NotificationWindowTest`); la muestra hecha (T5) | **La muestra, hecha en T5** (`TestTheGuardWithAHeadsUp`, `TestAPI34SystemUIWindows`). Se hace con una notificación propia de importancia alta (`GuardOnEmulatorTest -e adbuddyGuard headsup`), porque `cmd notification post` no sirve: su canal no es de importancia alta y no sale heads-up. Durante el heads-up, la ventana `NotificationShade` está visible (`mViewVisibility=0x0`) con el foco en la app, y el respaldo bloquea por ella. Al irse, la notificación sigue en la persiana y solo quedan las ventanas fijas (y la del fondo de pantalla: hallazgo de la T5). Para leer a tiempo se alarga «Tiempo para actuar» a 15 s (`accessibility_interactive_ui_timeout_ms`, restaurado), sin medir cuánto dura el heads-up. T7: (a) y (b) con la app de pruebas y el servicio vinculado. (c): sin medir; `settings put secure accessibility_interactive_ui_timeout_ms 20000` y medir. | Android 17. Paso: la pasada de solo lectura con una notificación de prueba. | alto | Muestra hecha (T5); T7 + Pixel |
| M5b-18 | `--reveal` equivale a `--noredact`; el `dumpsys notification` redactado deja ver 8 caracteres del `tickerText` | Sí | T2: una notificación propia con `setTicker` de más de 16 caracteres (instrumentado) y `dumpsys notification` desde el shell, comparando `--reveal` con `--noredact`. | — | alto | T2 |
| M5b-19 | Overlay real: marco y píldora; sin permiso, `click` responde FAILED | Parcial, hecha salvo el aspecto (T5) | Hecho en T5 (`TestTheOverlayOnTheEmulator`): el marco y la píldora salen al actuar (dos ventanas de ADBuddy) y los toques los atraviesan (M5a-3). **La premisa de "sin permiso" no se cumple:** la superposición es `TYPE_ACCESSIBILITY_OVERLAY` y no necesita `SYSTEM_ALERT_WINDOW`. Con `appops set … SYSTEM_ALERT_WINDOW deny`, `click` sigue siendo OK y la superposición sale igual. El FAILED («can't show the on-screen notice») solo llega si `addView` falla, y eso no se provoca por shell: lo cubre la JVM (`OverlayTest`, `UiExecutorTest`). | El aspecto. Paso: la pasada visual. | medio | Hecha (T5) + Pixel (aspecto) |
| M5b-20 | El host republica `remote.state` al hacer `attach` | Sí, hecha | `TestAttachPublishesRemoteStateThroughTheLink` | — | medio | Hecha |
| M5b-21 | `unknownSince` y CLOSING: el tick de la tarjeta | Sí (JVM), parte hecha (`RemoteUiTest`) | Compose con `mainClock` en Robolectric para el tick de `RemoteCard`. | — | bajo | fuera del tope |

### M5c · Notificaciones (22-35)

| # | Qué se valida | Emulador | Cómo | Por qué no / paso mínimo | Valor | Orden |
|---|---|---|---|---|---|---|
| M5c-22 | El listener (`exported="false"`) enlaza; los estados de la pantalla; al revocar, `UI_UNAVAILABLE` | Sí | T6: `cmd notification allow_listener` y `disallow_listener` (T1-1: enlaza y desenlaza en menos de 2 s); el instrumentado espera a `AgentNotificationListener.connected` y lee por `NotificationGate.read`, la misma cadena que `UiHelpers.kt:61` y `UiExecutor.kt:157`. | — | alto | T6 |
| M5c-23 | ¿Se lee `enabled_notification_listeners` en 17? ¿`null` o vacío? Permitido sin enlazar da "Desconocido" | Parcial | T6: la lectura en la API 34, con y sin listener, y con el proceso muerto. | Android 17. Paso: ver la pantalla de acceso en la pasada visual. | medio | T6 + Pixel |
| M5c-24 | Ajuste restringido (instalada por adb): el interruptor sale en gris; el atajo abre la entrada de ADBuddy; el aviso | Parcial | T1-6: una instalación por `adb install` **no** queda restringida en la API 34 (`ACCESS_RESTRICTED_SETTINGS` en `allow`, `packageSource=OTHER`, interruptor `enabled=true`), así que el caso en gris hay que simularlo (`appops set … ACCESS_RESTRICTED_SETTINGS ignore`, sin probar) o instalar como fichero local. T6: `am start -a android.settings.NOTIFICATION_LISTENER_DETAIL_SETTINGS` con el componente; `dumpsys activity` y `uiautomator dump` (¿el interruptor está `enabled`?). | Android 15-17 endurecen los ajustes restringidos. Paso: activar el acceso desde el onboarding en el Pixel (el uso normal). | medio | T6 + Pixel |
| M5c-25 | Mensajes, Gmail y GMS protegidos antes de confirmar; su notificación sale vacía; su heads-up y su primer plano se bloquean | Parcial | T7: los receptores reales, porque la imagen trae Mensajes, Gmail y GMS (T1-3), más una app de pruebas con un paquete de `CODE_RECEIVERS`. La lista del `device.info` del arnés (T3) es lo que sería `protected.json`. La guarda del host se prueba con la app delante. | La lista con las apps reales del Pixel. Paso: mirar `protected.json` en la pasada de solo lectura. | alto | T7 |
| M5c-26 | Contenido: SECRET sale vacía; BigText entera; `MessagingStyle` sin `EXTRA_TEXT`; `ongoing` y `postedAt`; con más de 50, `truncated`; la pregunta 🔴 sale sin texto | Sí | T6: notificaciones propias del instrumentado (BigText, `MessagingStyle` sin `EXTRA_TEXT`, `VISIBILITY_SECRET`, `ongoing`, 51 a la vez), porque `cmd notification post` deja `android.bigText=null`, rellena `android.text` en `-S messaging` y no elige visibilidad (T1-2); `cmd notification post` vale para notificaciones ajenas de relleno, leídas por `AgentNotificationListener.read()` y `NotificationReport.build`. | — | alto | T6 |
| M5c-27 | Lo que queda de las pantallas tras la JVM: color dinámico, Android 17, insets y gestos, giro, la lista en el PC; el título al 200 % en español | Parcial, JVM hecha (tests-ui-jvm) | En la API 34: gestos (`cmd overlay enable com.android.internal.systemui.navbar.gestural`), giro e insets; la lista en el PC con el arnés. | Android 17 y el hallazgo del título. Paso: la pasada visual, incluido el título. | bajo | Pixel |
| M5c-28 | Sin pulsar "Dejar que el agente lea", `NotConfirmed`; revocar y volver a permitir vuelve a pedirlo; `tap` sobre el botón de ADBuddy da `protected`; `input tap` con ADBuddy delante se bloquea | Sí | T6: los estados del gate. **T5, hecha** (`TestTheGuardOnRealScreens/adbuddy`): con ADBuddy delante, el daemon real sin la app bloquea `adb shell input tap`, `uiautomator dump`, `screencap`, Back, `adbuddy tap --xy`, `adbuddy screen` y `adbuddy screenshot` con «ADBuddy itself is in front», y deja pasar `input keyevent 3` (V44). En el respaldo de `uiautomator`, `Find` del botón «Not now» de ADBuddy da `ErrProtected`. Es la guarda por `dumpsys`; el `tap` de la app sobre su propio botón, con la app conectada, es de la T6. | — | alto | T6 + Hecha la parte de T5 |
| M5c-29 | El listener sigue enlazado en segundo plano, tras `kill` y tras reiniciar | Sí | T6 (emutest): `am force-stop` y `su 0 kill -9` (T1-9: se reenlaza solo, al momento y en 1 s; `am kill` no mata el proceso), y `adb reboot` con sus datos (se reenlaza solo unos 6 s después del arranque). `dumpsys notification` muestra el listener. | — | medio | T6 |
| M5c-30 | Perfil de trabajo o segundo usuario: sale vacía; oculta de la pantalla de bloqueo: vacía; PRIVATE y SECRET; el `Ranking` de 17; `hasSensitiveContent` | Parcial | T7: `pm create-user --profileOf 0 --managed`, `am start-user -w` y `pm install-existing --user <id>` (T1-5: funciona sin DPC) y la app de pruebas dentro del perfil; `VISIBILITY_PRIVATE` y canal oculto (no medido en T1: si se respeta `setLockscreenVisibility` lo mide la T7). | El `Ranking` de Android 17. `hasSensitiveContent` se responde leyendo el SDK, no con una prueba. | alto | T7 |
| M5c-31 | ¿Los reenviadores (KDE Connect, acompañantes de reloj, historiales) sacan notificaciones protegidas con su propio paquete? | No | — | Depende de apps de terceros concretas, y es un límite documentado (§3.5) con decisión de producto pendiente. Paso: opcional, si usa alguno, mirarlo una vez. | medio | Usuario (opcional) |
| M5c-32 | Revocar borra la marca aunque nadie mire: (a) con la app viva; (b) con la app muerta | Sí | T6 (emutest): un instrumentado confirma; el host hace `disallow_listener` y `allow_listener`; otro `am instrument` lee el gate y espera `NotConfirmed`. En (b), con `am force-stop` entre medias. | — | alto | T6 |
| M5c-33 | (a) `getDefaultSmsPackage` con otra app de SMS por defecto; (b) un receptor instalado después del arranque | Sí | (a) La app de SMS por defecto de la imagen ya es otra, Mensajes de Google (T1-3): ADBuddy llama a `ProtectedApps.defaultSms` y debe dar `com.google.android.apps.messaging`. Para otro titular, una app de pruebas con los componentes del rol SMS y `cmd role add-role-holder android.app.role.SMS <pkg>`: `set-bypassing-role-qualification` no salta la cualificación del rol SMS (T1-4). (b): instalar el receptor de prueba después del arranque. | ADBuddy como app de SMS no responde a la pregunta. | alto | T7 |
| M5c-34 | Lo que queda de la burbuja tras la JVM: `tap`, `input tap` y la guarda, y la Bubbles API real | Parcial, JVM hecha (`BubbleFlowTest`); la guarda hecha (T5) | **La guarda, hecha en T5** (`TestTheGuardWithTheBubble`, como en M3b-8): con la burbuja plegada sobre Ajustes, bloquea `Bubbles!`; con la burbuja abierta (`ChatBubbleActivity` en multi-window), `input tap`, `uiautomator dump`, `screencap` y `adbuddy tap --xy` se bloquean con «ADBuddy itself is in front», con la accesibilidad apagada y encendida, y por `dumpsys` (sin la app). La notificación de prueba no tiene servicio en primer plano y abre `ChatBubbleActivity` (T1-8: así sí es burbuja). El `tap` de la app y la Bubbles API real con la notificación de ADBuddy: **no**, en la API 34, porque es la del servicio en primer plano y no puede ser burbuja (ver «Hallazgos de la T1»). | La burbuja real: defecto que hay que adjudicar, y luego Android 17. Paso: junto con M3b-5. | alto | Hecha la guarda (T5) + Pixel; defecto |
| M5c-35 | El ticker en el `dumpsys notification` redactado | = M5b-18 | T2 | — | alto | T2 |

### M6a · Releases (1-6)

| # | Qué se valida | Emulador | Cómo | Por qué no / paso mínimo | Valor | Orden |
|---|---|---|---|---|---|---|
| M6a-1 | `release-check.yml` en la CI (R8, nombre del APK, `META-INF`, Signing Block) | No (CI) | En Actions, con el PR. | — | medio | CI |
| M6a-2 | La app de release (R8) funciona: la serialización con kotlinx | Sí | T8: el APK de `release-check` firmado con una clave de prueba solo para el emulador (nunca la de release), instalado en la API 34; el arnés (T3) comprueba el saludo y los mensajes. | — | alto | T8 |
| M6a-3 | Lo que la spec da por hecho de Android: APK de ataque, salidas de `pm path`/`stat`/`base64`, `install -r`, `base.apk` idéntico, autoridad duplicada | Parcial | T8: todo en la API 34. | Las API 30 y 33 piden imágenes no instaladas (decide el usuario), y la última es la 37 rota. | alto | T8 |
| M6a-4 | Acciones del usuario: clave, environment, secretos, ruleset, PR con `SigningCertSHA256`, primera etiqueta | No | — | La cuenta de GitHub y el keystore son del usuario y el agente no los toca nunca. Paso: `docs/releasing.md`, "User actions". | alto | Usuario |
| M6a-5 | `gh release upload` a un borrador por nombre de etiqueta | No (CI) | En la primera release o en un fork. | — | bajo | CI |
| M6a-6 | La app de debug del usuario tras M6a | No | — | Es un aviso, no una prueba. Paso: `adbuddy setup --apk <APK>` una vez. | bajo | Usuario |

### M6b · Skills

| # | Qué se valida | Emulador | Cómo | Por qué no / paso mínimo | Valor | Orden |
|---|---|---|---|---|---|---|
| M6b-a | `claude`, `codex`, `gemini` y `opencode` listan las nueve skills; Gemini carga `AGENTS.md` | No (PC) | En el PC con las CLIs instaladas, solo lectura. | — | bajo | PC |
| M6b-b | Pasada de solo lectura de `device-profile`, `permissions-audit`, `storage-cleanup` y `debloat` | Parcial | En el emulador, las órdenes de ejemplo de cada skill por el shim: nivel y salida. | El perfil real del Pixel. Paso: la pasada de solo lectura. | medio | Pixel |
| M6b-c | Un cambio real y deshecho con `battery` (animaciones) y `organize-files` | Sí | `settings put global animator_duration_scale 0.5` por el shim, `adbuddy undo` y comprobar el valor; una carpeta de prueba en `/sdcard`. | — | medio | fuera del tope |
| M6b-d | `home-screen` en el lanzador del Pixel (`input swipe`, `draganddrop`, `motionevent`) | Parcial | Launcher3 de la API 34. | El lanzador del Pixel es otro. Paso: opcional. | bajo | Usuario (opcional) |
| M6b-e | `build-utility-apk` de principio a fin sin SDK | No (PC) | Un PC o contenedor limpio. | — | bajo | PC |
| M6b-f | `adbuddy env init` agrupa bien en Windows y en Linux | No (CI) | Test de host en las dos CI. | — | bajo | CI |
| M6b-g | Un script de `adb shell` desde PowerShell 5.1 y desde Codex | No (PC) | En el PC. | — | medio | PC |
| M6b-h | Gemini CLI con la confianza de carpetas, y `save_memory` | No (PC) | En el PC. | — | bajo | PC |
| M6b-i | Una unión `mklink /J` se rechaza en una instalación real | No (PC) | Test de host en Windows (este PC). | — | medio | PC |

### M6c · Idiomas (1-9)

| # | Qué se valida | Emulador | Cómo | Por qué no / paso mínimo | Valor | Orden |
|---|---|---|---|---|---|---|
| M6c-1 | Pixel en español: todas las pantallas, tarjetas y errores forzados | Parcial | El emulador en `es-ES` (T1-10: `su 0 setprop persist.sys.locale es-ES` y `adb reboot`) y `uiautomator dump`: ni claves ni inglés. La JVM ya pinta M5c en español. | Android 17. Paso: la pasada visual. | medio | Pixel |
| M6c-2 | Idioma por app (13+) sin reiniciar, incluidos los canales | Sí | `cmd locale set-app-locales tech.hybriq.adbuddy --locales en` y `dumpsys notification --noredact` para los nombres de los canales (T1-10: cambian al momento, sin reiniciar el proceso). | — | bajo | fuera del tope |
| M6c-3 | Notificaciones con el móvil bloqueado y desbloqueado; hora en 24 h | Parcial | T4: la versión pública en la pantalla de bloqueo, vista con `dumpsys notification` y `uiautomator dump`. | — | medio | T4 |
| M6c-4 | Plurales (1 y 2 esperando) | Sí (JVM) | `getQuantityString` en Robolectric, en español. | — | bajo | fuera del tope |
| M6c-5 | Windows en español sin `language` ni `LANG`; acentos en Windows Terminal y en `cmd.exe` | No (PC) | En el PC (`Get-UICulture`). | Windows en español. | bajo | PC |
| M6c-6 | El PC y el móvil en idiomas distintos | Parcial, hecha (T3) | `TestSpanishAppEnglishHost`: la app en español con su idioma propio (`cmd locale set-app-locales … --locales es`; el del sistema pide reinicio, T1-10) y el host en inglés (un literal del test: el host no se ejecuta). La pregunta 🔴 que llega por el canal real sale como "El agente quiere: Restart the phone", con los botones "Permitir" y "Rechazar" (`dumpsys notification --noredact`). | Si la mezcla molesta lo decide el usuario. | bajo | Hecha (el juicio, del usuario) |
| M6c-7 | Una 🔴 con razón, en español, en el móvil y en la terminal | Sí | T4: `confirm.request` en español por el arnés, más el test de terminal del host. | — | medio | T4 |
| M6c-8 | `LocalesTest` en el emulador | Sí, hecha (M2.5b) | `TestInstrumentedSuite` | — | bajo | Hecha |
| M6c-9 | El castellano de M5c | No | — | Es una revisión de texto, no del dispositivo. | bajo | Revisión |

### v1 · Diferidos, B (36-41) y L (42-43)

| # | Qué se valida | Emulador | Cómo | Por qué no / paso mínimo | Valor | Orden |
|---|---|---|---|---|---|---|
| VD-1 | V43 (herramientas atascadas en `running`) y V05 con Claude Code real | No (PC) | `ADBUDDY_REAL_CLAUDE=1` | Claude Code real. | medio | PC |
| VB-36 | Android decodifica `%XX` en `android-app://` | Sí, hecha (T5) | Hecho en T5 (`TestAnEncodedAppURIOpensTheApp`). **Medido:** con `am start -a VIEW -d android-app://com%2Eandroid%2Esettings`, `am` muestra el host decodificado (`dat=android-app://com.android.settings/…`), pero ese VIEW lo resuelve la app de Google (`AppIndexingActivity`) y no abre Ajustes. La forma posicional (`am start android-app://…`) deja el paquete codificado y no resuelve. Donde sí se abre la app es con una URI `intent:` con `package=` o `component=` codificados: abre Ajustes y ADBuddy. El daemon real, aunque la persona diga que sí a la pregunta 🔴 del clasificador, bloquea las dos URI `intent:` de ADBuddy (una y dos capas de codificación) y la `android-app://`, y ADBuddy no sale delante. | — | medio | Hecha (T5) |
| VB-37 | "Apagar" con el modo remoto apagado no corta la conexión; un 5555 o un 7777 abiertos a mano se cierran | Sí | T8: `remote` contra el emulador; el registro de estados (`device` o `gone`) muestra si adbd se reinició, porque `adb usb` lo reinicia en ~1 s (S5). | — | medio | T8 |
| VB-38 | La burbuja sobre la terminal sigue recibiendo salida | Parcial | T3: abrir y plegar `ChatBubbleActivity` como burbuja con UiAutomator. Medido en T1-8: con la notificación de ADBuddy no sale, porque es la del servicio en primer plano; con una notificación de prueba sin ese flag, sí sale, se abre con `input tap` y se pliega con atrás. | La burbuja real: defecto que hay que adjudicar, y luego Android 17. Paso: junto con M3b-5. | bajo | T3 + Pixel; defecto |
| VB-39 | Un badge en el chat se ve como enlace y no carga la imagen | Sí (JVM) | Robolectric: `MarkdownText` con el badge; MockWebServer no recibe ninguna petición. | — | medio | fuera del tope |
| VB-40 | Dos `serve` a la vez en la misma carpeta de datos | Parcial | Con el emulador (no medido en T1: necesita `serve`): el servidor adb de `serve` (5038) no debe ver nunca el móvil del usuario. El `flock`, en la CI. | — | bajo | CI |
| VB-41 | `persist.adb.tcp.port` frente a `adb usb` y al reinicio de adbd | Sí | T8: `su 0 setprop persist.adb.tcp.port 5555` (`su 0` medido), `adb usb`, reinicio de adbd y `service.adb.tcp.port`. | — | medio | T8 |
| VL-42 | Dos `serve` sin móvil: el segundo sale con `ALREADY_RUNNING` | No (PC) | Test de `cli` con un adb real y ningún dispositivo. | — | bajo | PC |
| VL-43 | En la CI de Linux y macOS, `TestSendFIFOSwappedInBeforeOpen` corre y pasa | No (CI) | — | — | bajo | CI |

### Ola 1, ola 2 H, ola 2 M y pty-kill (pendientes y planes)

| # | Qué se valida | Emulador | Cómo | Por qué no / paso mínimo | Valor | Orden |
|---|---|---|---|---|---|---|
| O1-a | La lista blanca de lectura de `dumpsys` (`activity`, `window`, `procstats`, `package`, `meminfo`, `batterystats`) no cambia estado | Parcial, el indicio hecho (T5) | Hecho en T5 (`TestTheDumpsysReadsChangeNothingSeen`): 86 formas, cada una 🟢 para el clasificador, corren en el emulador. `settings list global`, `secure` y `system` quedan iguales, y también los ficheros de `/data/system/procstats` y los de batterystats. Sus salidas no se graban: pesan y no las lee nada. | Que algo no cambie estado no se demuestra observando: se contrasta con AOSP. | alto | Indicio hecho (T5); AOSP |
| O1-b | `.claude` o `projects` movidos con una junction o un symlink | No (PC) | Test de host en Windows. | — | bajo | PC |
| O1-c | PC con política de empresa o sin permiso para crear un Job object | No (PC) | — | Un PC con esa política. Paso: opcional. | bajo | Usuario (opcional) |
| O1-d | En la CI de Linux y macOS pasan los tests Unix de V14, V26, V16, V28 y V24 | No (CI) | — | — | medio | CI |
| O1-e | Claude Code real: una pregunta dejada en la terminal retiene el chat; el nombre de `elicitation_dialog` | No (PC) | `ADBUDDY_REAL_CLAUDE=1` | — | medio | PC |
| O2H-a | Workspace dentro de OneDrive con Files-On-Demand | No (PC) | — | El OneDrive del usuario. Paso: opcional. | bajo | Usuario (opcional) |
| O2H-b | `attach` desde fuera del árbol del agente: otra terminal, IDE, ssh, consola elevada, WSL1 | No (PC) | Test de `ipc` con un cliente lanzado fuera del job. | Terminales concretas. | medio | PC |
| O2H-c | `SIO_AF_UNIX_GETPEERPID` en Windows 10 y 11 | No (PC) | Este PC (Windows 11) y la CI de Windows. | Windows 10. | medio | PC |
| O2H-d | Un agente que vacía `ADBUDDY_SOURCE` recibe `AGENT_SESSION` | No (PC) | Test de `cli` con un hijo real del árbol del agente. | — | alto | PC |
| O2H-e | Un agente en pausa que ejecuta `adbuddy doctor` no llama a adb | No (PC) | Test de `cli`. | — | bajo | PC |
| O2H-f | La carpeta `journal` movida con una junction se rechaza | No (PC) | Test de host en Windows. | — | bajo | PC |
| O2H-g | `serve` en un sandbox con espacio de PID propio | No (CI) | `unshare --pid` en la CI de Linux. | — | bajo | CI |
| O2H-h | WSL1 como cliente intermediario | No (PC) | — | WSL1. Paso: opcional. | bajo | Usuario (opcional) |
| O2H-i | macOS: `getsid` sobre otra sesión | No (CI) | `host.yml` en macOS. | — | bajo | CI |
| O2M-a | K1: carga real del PTY con ConPTY y con Unix | No (CI) | CI de Linux; en Windows, con un agente real. | — | medio | CI |
| O2M-b | K1: `Kill` desatasca el `Write` en Windows | No (CI) | La CI de Linux lo ejecuta (en Windows hace `t.Skip`). | conhost no bloquea. | bajo | CI |
| O2M-c | K1 con #19: el `guard` no depende de un lock que se tome con `chat.mu` retenido | No | — | Es una revisión de código. | medio | Revisión |
| O2M-d | J2.3: `/compact` real | No (PC) | `ADBUDDY_REAL_CLAUDE=1` | — | bajo | PC |
| O2M-e | J2.1: con `Edit` y `Write`, la entrada canónica del hook coincide con la del transcript | No (PC) | `ADBUDDY_REAL_CLAUDE=1`, grabando en `claudecode/testdata`. | — | alto | PC |
| PTY-a | macOS: `Stop` con un agente que no lee | No (CI) | — | — | bajo | CI |
| PTY-b | Linux real con un agente que no lee | = O2M-a | — | — | — | — |

### Revisión de seguridad, tarjetas (A4), bidi y M2.5b

| # | Qué se valida | Emulador | Cómo | Por qué no / paso mínimo | Valor | Orden |
|---|---|---|---|---|---|---|
| SEG-1 | En Android 14+, `appops set <pkg> PROJECT_MEDIA allow` sigue saltándose el diálogo de consentimiento | Sí (API 34) | T7: la app de pruebas lanza `createScreenCaptureIntent` con y sin el appop; `uiautomator dump` muestra si aparece el diálogo. | — | medio | T7 |
| A4-1 | Una pregunta especial o de más de 300 caracteres: la notificación solo trae Rechazar y Abrir; ninguna ofrece "Permitir durante la sesión"; el diálogo no activa Permitir hasta el final; con más de 20 000 dice cuántos oculta y solo deja Rechazar | Sí | T4: `uiautomator dump` de la persiana (las acciones) y del diálogo, que se desplaza con `input swipe`. Además, un test JVM del diálogo para B3: `unread` sale del mismo `ScrollState`. | — | alto | T4 |
| BIDI-1 | Una 🔴 con un nombre en hebreo o árabe en la app real; la tarjeta que cae a la terminal | Sí | T4: el arnés manda la pregunta; se mira con `uiautomator dump` y `screencap`. | — | medio | T4 |
| M25B-1 | Primera ejecución del job `emulator` con los tests de la app en el runner Windows | No (CI) | En Actions, con el PR de M2.5b. | — | medio | CI |
| M25B-2 | El job `emulator` en un runner Linux | No (CI) | — | — | bajo | CI |
| UIJ-1 | Las imágenes de referencia en Linux y macOS | No (CI) | — | — | bajo | CI |
| UIJ-2 | Las imágenes en otra máquina Windows | No (CI) | — | — | bajo | CI |
| UIJ-3 | Lo que sigue para el Pixel de M5c-27 y 34 | = M5c-27/34 | — | — | — | — |

## Imágenes para lo que depende de Android 15-17

No se ha instalado nada. Las opciones, en orden, y lo que necesita cada una:

1. **Actualizar el emulador** (37.2.12) y la imagen `android-37.0` a su última revisión, con `sdkmanager --update`. El aborto ocurre porque el `mapper.ranchu.so` de la imagen exige que el host anuncie `hasReadColorBufferDma`, y un emulador más nuevo podría anunciarlo. Es la única vía a Android 17 en un emulador. Coste: una actualización del SDK (la decide el usuario), y luego repetir el S1 de M2.5b (120 s sin cambios de pid ni líneas en `crash`).
2. **`system-images;android-36;google_apis;x86_64`** (Android 16, unos 1,5-2 GB). Cubre edge-to-edge obligatorio (35+), los cambios de servicios en primer plano de 15-16 y los ajustes restringidos de 15 (M2-3, M5c-24, M3b-7). No cubre 17. Si su mapper tiene la misma aserción, no se ha podido medir (T1: la imagen no está instalada); se mide con el S1 cuando se instale. Se usa con `ADBUDDY_EMU_IMAGE` (el script ya la lee) y un AVD `adbuddy-dev36`.
3. **`android-35`**: lo mismo, sin lo de la 16.
4. **`android-30` y `android-33`**: para M6a-3 y M2-2 (Android 11-13). Mismo coste.

Mientras no haya imagen nueva, todo se instrumenta en la API 34 y la diferencia con Android 17 queda en la checklist del Pixel.

## Plan de automatización propuesto

Las tareas van en orden de valor y en el emulador propio: AVD `adbuddy-dev34`, consola ≥ 5600, servidor adb propio, mDNS apagado y la guarda de `emutest`, que solo acepta `emulator-*` y rechaza los puertos 5037 y 5038. Cada tarea compila y pasa por sí sola. Las de T3 en adelante que usan el canal real dependen de T3.

**T1 (S). Medir lo que está "por medir"** (la lista de arriba, del 1 al 12). **Hecha el 2026-10-03:** los resultados están en "Datos medidos" y en las filas, las órdenes con su salida en `.superpowers/sdd/2026-10-03-validaciones-manuales/t1-mediciones.md`, y los dos defectos en "Hallazgos de la T1". Ninguna fila cambia de categoría. La parte de la burbuja de M3b-5, M3b-8, M5c-34 y VB-38, y la M5b-11, quedan señaladas como defecto pendiente de adjudicar.
- Ficheros: el ledger del plan y este documento, donde cada "por medir" se cambia por el resultado.
- Aceptación: cada orden con su salida en el ledger, y la limpieza hecha (`pm remove-user`, `locksettings clear`, `disallow_listener`, desinstalar). No lleva RED ni GREEN porque es una medición. Si una medida tumba una fila "Sí", la fila baja a Parcial antes de T2.

**T2 (S). El ticker del `dumpsys notification` redactado** (M5b-18, M5c-35).
- Ficheros: `android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/TickerOnEmulatorTest.kt` (publica una notificación propia con `setTicker("G-482913 is your ADBuddy test code")`), `host/internal/emutest/notification_dump_test.go` (redactado, `--reveal` y `--noredact`) y la lista de `requiredClasses`. Si se confirma la fuga: `host/internal/classify` (`dumpsys notification` pasa a 🔴), su caso en `testdata/differential.jsonl` y regrabar el oráculo según `host/README.md`; `habitual.jsonl` no lo usa.
- RED: un test de `classify` que exige 🔴 para `dumpsys notification` falla hoy (es 🟢). GREEN: tras el cambio. Si el emutest **no** confirma la fuga, se queda como guarda de regresión y su RED es la mutación de buscar el prefijo en `--noredact`, que sí lo trae.

**T3b (M). Ficheros y terminal sobre el arnés de T3 (M3b-3 y M3b-10).**
- Ficheros: `host/internal/emutest/files_test.go` y `host/internal/emutest/terminal_test.go`, con `openLink`. M3b-3: el arnés hace de host de ficheros (`attach.begin`, `attach.ready`, tramas binarias, `attach.end`, `file.get`, el corte y la continuación de 100 MiB, la caché con `run-as`), eligiendo el fichero con UiAutomator. M3b-10: `svc wifi disable` y `svc data disable`, el flujo de `terminal.watch` y `terminal.output` y la lectura de la terminal (xterm.js pinta en un canvas), vigilando que nada pida red.
- Depende de T3 (hecha). RED: por mutación, como en T3.

**T3 (M). Arnés del canal real en `emutest`.**
- Ficheros: `host/internal/emutest/link_test.go`, con un `openLink(t)` que abre un listener del bridge en 127.0.0.1, hace `adb reverse`, ejecuta `phoneapp.Handshake` y espera `hello` y `device.info`. Limpieza con `adb reverse --remove-all`. Reutiliza `internal/bridge` y `internal/phoneapp`.
- RED: `TestTheLinkCarriesDeviceInfo` falla con "no device.info in 10 s" si se quita el `reverse` (mutación). GREEN: con el arnés. Cubre M2-7, M3b-1, 3, 4, 6 y 10, M5a-2 y M6c-6, y lo usan T4, T6, T7 y T8.
- **Hecha el 2026-10-03** (rama `test/link-harness`). API del arnés, en `link_test.go`: `openLink(t)` devuelve un `*link` con la app instalada, en segundo plano, fuera de la optimización de batería y con `POST_NOTIFICATIONS`, conectada y con su primer `device.info` en `Info`; `send` (evento o stream según `HostClasses`), `reply`, `publishRID`, `waitFor(t, tipo, tope)`, `take`, `drain`, `Hellos`, `waitHellos`, `handshake`, `cut` y `resume` (parar el host y volver en el mismo puerto), `connected` y `uiRequest`. Para la pantalla: `e.screen`, `e.waitNode`, `e.tap`, `l.openChat` (salta los dos pasos del primer uso) y `poll`. Limpieza: el bridge para, `adb reverse --remove-all` en el emulador, `deviceidle whitelist -`, ajustes restaurados y desinstalación.
- Cubre del todo M3b-6 y M5a-2; M3b-4 y M6c-6 en lo que el emulador permite; M3b-1 en ASCII; de M2-7, el canal (la 🔴, T4). Cada test tiene su RED por mutación en el informe de la tarea.
- **No cubre M3b-3 ni M3b-10.** M3b-3 pide que el arnés haga de host de ficheros (`attach.begin`, `attach.ready`, tramas binarias y `attach.end`, `file.get` con descarga por tramas, el corte y la continuación de 100 MiB) y manejar DocumentsUI: es una tarea M aparte, no "poco". M3b-10 pide ver que la WebView de la terminal sigue sin red y que nada pide red: hace falta el flujo de `terminal.watch` y `terminal.output` y leer el contenido de xterm.js, que se pinta en un canvas, más vigilar el tráfico. Tampoco es poco. Las dos pasan a la tarea T3b.

**T4 (M). Confirmaciones 🔴 en el emulador** (M2-5, M2-8, M2-9, A4-1, M3b-5 en la parte del bloqueo, M4-c, M4-d, M6c-3, M6c-7, BIDI-1).
- Ficheros:
  - `host/internal/emutest/confirm_test.go`: `confirm.request` normal, especial, de más de 300 y de más de 20 000 caracteres por T3; PIN y bloqueo; persiana con `cmd statusbar expand-notifications`, `uiautomator dump`, `input tap` y bouncer.
  - `android/app/src/test/kotlin/tech/hybriq/adbuddy/confirm/ConfirmDialogScrollTest.kt`: Robolectric; Permitir solo al final; B3.
  - `android/app/src/test/kotlin/tech/hybriq/adbuddy/ui/PermissionCardScrollTest.kt`.
- RED por mutación:
  - `setAuthenticationRequired(false)` en `ConfirmNotifier.kt:127` hace que `TestAllowFromTheLockedShadeAsksToUnlock` reciba `allow` sin bouncer;
  - quitar `!unread` hace que fallen los dos tests de desplazamiento;
  - ofrecer Permitir en la notificación de una especial hace que falle el volcado de A4-1.
- GREEN: con el código de hoy.
- Si UiAutomator por el shell resulta inestable, la tarea pasa a L.

**T5 (M). La guarda con muestras reales de la API 34** (M5a-3, 5, 6, 7, 8, 9; M5b-14, la muestra de M5b-17, M5b-19; M3b-8; M5c-28 y 34 en la parte de la guarda; VB-36; O1-a).
- Ficheros: `host/internal/emutest/guard_test.go`, que pone cada estado (ADBuddy delante, persiana, ajustes rápidos, bloqueo, heads-up con `cmd notification post`, diálogo, teclado, burbuja), graba las salidas y prueba la decisión de la guarda; `host/internal/uidump/testdata/api34/*.txt`, y los tests de `uidump` y `daemon` que las leen (`Foreground`, `ParseFocus`/`IsShade`, `ParseSystemUIWindows`, `ServiceOn`, `Asleep`).
- RED: cada muestra nueva se pasa primero por un test que espera la decisión correcta. Si pasa a la primera, la RED es la mutación de quitar `NotificationShade` o el título de systemui de su lista.
- GREEN: las muestras quedan versionadas sin seriales ni IPs (`git grep` antes de commitear).
- **Hecha el 2026-10-03** (rama `test/guard-samples`):
  - `host/internal/emutest/guard_test.go` pone cada estado y pregunta al daemon real del host, sobre el adb del emulador, qué decide. `overlay_test.go` prueba la superposición con el arnés de T3. `GuardOnEmulatorTest.kt` publica el heads-up y la burbuja.
  - `host/internal/uidump/testdata/api34/` tiene 17 estados, con su `README.md`. Los leen `uidump/api34_test.go` y `daemon/guard_api34_test.go`.
  - Cubre, en lo que dice cada fila: M5a-3, 5, 6, 7 (en parte), 8 (en parte) y 9; M5b-14, la muestra de M5b-17 y M5b-19; M3b-8 y M5c-34 en la guarda sin la app; la parte de T5 de M5c-28; VB-36; y el indicio de O1-a.
  - El hallazgo del fondo de pantalla está en «Hallazgos de la T5».
  - Las pruebas suman unos 3 minutos al job del emulador de la CI (`go test -timeout 20m ./internal/emutest`).

**T6 (M). Listener y consentimiento** (M5c-22, 23, 24, 26, 28 en la parte de la app, 29 y 32).
- Ficheros: `android/app/src/androidTest/kotlin/tech/hybriq/adbuddy/NotificationListenerOnEmulatorTest.kt` (lee por `NotificationGate.read` y `NotificationReport.build`), `host/internal/emutest/listener_test.go` (`allow_listener` y `disallow_listener`, `su 0 kill -9` (`am kill` no mata el proceso: T1-9), `am force-stop`, reinicio del AVD y dos `am instrument` para la 32), y `allowedSkips` o `requiredClasses` en `instrumented_test.go` para los tests que solo arrancan desde el host.
- RED por mutación: quitar `revoke()` de `onListenerDisconnected` hace fallar la 32(a); `secret = false` en `raw()` hace fallar la 26. GREEN: con el código de hoy. Si la 32(b) muestra que la marca sobrevive, es el riesgo aceptado del ruling 15: se anota y el arreglo barato (borrarla también en `onListenerConnected`) lo adjudica el controlador.

**T7 (L). App de pruebas y perfiles** (M5c-25, 30, 33; M5a-4, 10; M5b-16, M5b-17 (a) y (b); M2-4; SEG-1).
- Ficheros: el módulo nuevo `android/fixtures/poster` en `settings.gradle.kts`, con su variante `bank` (nombre de banca), `receiver` (un paquete de `CODE_RECEIVERS` que la imagen no traiga) y `sms` (los componentes del rol). Publica notificaciones por broadcast (visibilidad, ticker, actualización con el mismo id), y trae un `VpnService`, un servicio de accesibilidad mínimo y `createScreenCaptureIntent`. Además, `host/internal/emutest/fixture_test.go`, y que `android.yml` compile el módulo.
- RED por mutación: que `ProtectedApps.defaultSms` devuelva `null` hace fallar `TestTheDefaultSmsAppIsProposed`; quitar `sbn.user != Process.myUserHandle()` hace fallar el test del perfil. GREEN: con el código de hoy.
- Las variantes con nombres de paquete de terceros solo se instalan en el emulador.

**T8 (M). adb, modo remoto y release en la API 34** (M5b-12, M5b-13, VB-37, VB-41, M6a-2, M6a-3).
- Ficheros: `host/internal/emutest/remote_test.go` (`Enable`, `Disable` y `Reconcile` leyendo `service.adb.tcp.port`, nunca el socket); `host/internal/emutest/release_test.go` (el APK de release firmado con una clave temporal creada en `t.TempDir()`, nunca la de release, y el saludo por T3); `host/internal/emutest/install_test.go` (las salidas de `pm path`, `stat` y `base64`, `install -r`, `base.apk` byte a byte, la autoridad duplicada y los APK de ataque de la spec §5).
- RED por mutación: un `Disable` que lanza siempre `adb usb` hace que la VB-37 vea `gone`. GREEN: con el código de hoy.

**Fuera del tope** (S cada una; todas son JVM o baratas): M5b-21 (tick), VB-39 (badge), M6c-2 y M6c-4, M6b-c (`battery` y `undo`) y M3b-7 (las pantallas que faltan en la JVM).

## Paso mínimo del usuario

Lo que de verdad necesita a la persona: el Pixel 9 Pro XL con Android 17 en una sesión con su permiso, y nada más. Cada punto cierra la parte de Android 17 de las filas que se citan.

**En el Pixel (una sesión, unos 30 minutos):**

- [ ] **Instalar y emparejar** con `adbuddy setup`: el QR en Windows Terminal y en Git Bash, o el código si el QR falla. Cierra M1b-1, 2 y 3, y de paso M2-2 y M2-3 en Android 17.
- [ ] **Activar el acceso a notificaciones** desde el onboarding, que en 17 pasa por los ajustes restringidos. Cierra M5c-22, 23 y 24 en 17.
- [ ] **Con el móvil bloqueado**, Rechazar y Permitir una pregunta 🔴 de prueba, con la biometría. Cierra M2-5 y M2-8.
- [ ] **Cortar acceso y Reactivar** una vez, con la depuración inalámbrica real. Cierra M2-6 y M2-10.
- [ ] **Pasada visual:** pantallas en claro, oscuro, al 200 % y en español; abrir la burbuja una vez; mirar la terminal. Cierra M3b-2, 5 y 7, M5c-27 (incluido el título al 200 % en español) y 34, M6c-1, M5a-3, M5b-19 y VB-38.
- [ ] **Dar permiso a una pasada de solo lectura.** El agente graba `dumpsys`, `getprop` y `settings get` con la persiana bajada, en la pantalla de bloqueo y con una notificación de prueba de una app propia; mira `protected.json`; cronometra un `adbuddy screen`; corre las skills de solo lectura. Cierra M5a-1, 6 y 9, M5b-11, 14 y 17, M5c-25 y 30, M3b-8 y M6b-b en 17.
- [ ] **Abrir la app del banco** y pedir un `adbuddy screen`, que debe responder Blocked. Cierra M5a-4.
- [ ] **Fuera de casa, o con el Wi-Fi apagado,** con Tailscale: `adbuddy remote on`, y `serve` reconecta. Cierra M5b-12, 15 y 16.

**En la cuenta de GitHub:** la clave, el environment y los secretos de la release (M6a-4), y un `adbuddy setup --apk` tras M6a (M6a-6).

**Opcional,** solo si los tiene a mano:
- un móvil de otra marca (M2-4, M5a-6, M5b-14);
- un reloj Wear o Phone Link (M2-9);
- KDE Connect o un acompañante de reloj (M5c-31);
- el lanzador para `home-screen` (M6b-d);
- OneDrive, WSL1 o un PC con política de empresa (O2H-a, O2H-h, O1-c).

Lo marcado "PC" y "CI" no es del usuario. Las pruebas con Claude Code real (`ADBUDDY_REAL_CLAUDE=1`) y los tests de host en Windows los corre un agente en este PC sin tocar el móvil, y lo de la CI se ve en Actions.
