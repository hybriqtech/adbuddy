# Area A4 — Tarjetas y confirmaciones (segunda mirada independiente, solo lectura; opus medium; guardado por puffin)
Rama local docs/revision-tarjetas (= origin/main a4766fe). Veredicto: 1 CRITICO, 2 IMPORTANTES, 2 MENORES.

## CRITICO
**1. «Permitir durante la sesión» sobre `adb shell` / `adb exec-out` sin argumentos (o `shell -t`) deja al agente ejecutar después lo que quiera por stdin sin preguntar.**
- Donde: host/internal/classify/classify.go:203-204, host/internal/daemon/daemon.go:503-506 y :603, host/internal/bridge/confirm.go:66-70. El test lo fija como no Special: classify/hardening_test.go:30 (special=false).
- Que pasa: se clasifican como 🔴 `shell.interactive` con `redFinding` (sin Special) => el host ofrece AllowSession; la clave de sesión es regla+cwd+argv, SIN el stdin; lo que se ejecuta es lo que llega por stdin, que el daemon reenvia a adb y nunca se analiza ni se muestra.
- Escenario: `echo ls | adb shell` -> el usuario ve «Abrir una shell interactiva» y pulsa «Permitir durante la sesión» -> despues `echo 'pm uninstall com.banco; svc wifi disable; pm disable tech.hybriq.adbuddy' | adb shell` se ejecuta sin preguntar. Se salta el invariante «un Special nunca se permite por sesión» y la comprobación de que nombre la app (classify.go:113).
- Correccion: marcar `shell.interactive` como Special (como `unanalysable`, wrappers.go:248); segunda barrera: ningun comando cuyo efecto dependa del stdin lleva clave de sesión (clave vacia, como con `reason`); tests en bridge y daemon de que `["shell"]` y `["exec-out"]` no llevan AllowSession.

## IMPORTANTES
**2. Un comando largo se recorta y no se puede ver el resto antes de permitir.** android/.../confirm/ConfirmUi.kt:89 (tope 300 en notificación, MAX_COMMAND l.137; 4000 en el dialogo, ui/Screens.kt:209); `display` de titulos a 80 runas (classify.go:312-318). Un 🔴 Special con ~4000 espacios/comentarios de relleno y la parte dañina al final: la notificación ofrece Permitir tras desbloquear sin abrir el dialogo, y el dialogo tampoco muestra todo. `Visible` no compacta los blancos. Correccion: si rawCommand supera el tope, sin Permitir en la notificación y dialogo con el texto completo, con scroll y botones desactivados hasta el final (como PermissionCard l.129-171); «N caracteres ocultos»; compactar rachas de blancos como «␠×N».
**3. El aviso ⚠ Special puede quedar fuera de la vista en la notificación.** ConfirmUi.kt:85-90, ConfirmNotifier.kt:79-87: orden detalle, ⚠, comando, pie; el detalle lleva una linea por hallazgo 🔴 distinto, controlada por el agente (muchos fragmentos, variables {N} sin tope en baseVars classify.go:348-350). Un comando con ~15 fragmentos 🔴 inocuos y un `svc wifi disable` a mitad: el BigText se corta antes del ⚠ y Permitir sigue ahi. Correccion: ⚠ antes del detalle; detalle de la notificación a ~3 lineas + «y N más»; para Special, sin Permitir en la notificación (obligar al dialogo).

## MENORES
**4. La barra invertida literal no se distingue de un caracter escapado** (classify/visible.go:41-62 no escapa `\`): una cadena `\n`/`‮` escrita por el agente se ve igual que un salto de linea real escapado. Sin descenso de nivel conocido. Correccion: escapar `\` como `\` en Visible.
**5. Carrera caducidad/respuesta** (bridge/confirm.go:110-114, :176-206): `select` elige al azar si coinciden; puede aceptar un Allow milisegundos tarde. Opcional: comprobar ctx.Err() tras leer p.reply.

## Revisado y correcto
- Request.Shown escapa titulo, comando y detalle linea a linea (RTL, Cf, control, ANSI) con nota de RTL; lo usan terminal (confirm.go:167) y canal de la app (bridge/confirm.go:82); el Fallback recibe el original y lo escapa.
- Lo ejecutado es el req.Argv clasificado; RawCommand sale de DisplayCommand(req.Argv); el detalle lista todos los 🔴 (Result.Detail). Sin TOCTOU salvo el stdin del punto 1.
- Autoaprobación: canal solo en 127.0.0.1, comprueba Host, token de 64 hex en tiempo constante, cambia en cada traspaso y viaja por stdin a `content write` (no en argv); ConfirmReceiver no exportado y PendingIntent inmutable; Permitir exige movil desbloqueado (setAuthenticationRequired + isDeviceLocked); el dialogo filtra toques si otra ventana lo tapa; con una pregunta pendiente, admit y waitChanges bloquean comandos y ayudas de UI del agente y el guard exige que lo que toca/lee la pantalla vaya solo.
- Reenvios/duplicados: respuesta con id no pendiente no hace nada; id duplicado o vacio se deniega; sesion solo si p.req.AllowSession; Special y lo elevado por precheck no llevan clave; AppGone mueve la pregunta a la terminal; DenyAll devuelve Nobody. Caducidad con reloj del host en ambos lados; fecha ilegible = caducada.
- Deshacer: la inversa 🔴 se reclasifica, el texto sale del clasificador y no del diario, sin clave de sesión y se rechaza si no corresponde al cambio registrado.
- NotificationConsent/NotificationGate: solo el boton de la pantalla marca consentimiento; al desvincularse el listener se revoca sincronamente; se recomprueba tras leer; lo incognoscible cuenta como sin acceso o secreto; el informe excluye ADBuddy, apps protegidas, otros usuarios y lo oculto en pantalla de bloqueo.

## Lo que los tests no cubren
Que `shell.interactive`/`exec-out` sin argumentos no ofrezcan AllowSession; la notificación con detalle largo (⚠ visible); un comando por encima del tope con relleno de espacios.
