# SDD ledger — plan: docs/superpowers/plans/2026-10-18-modo-yolo.md

Ledger del modo YOLO, 2026-10-18.

Rama `feat/modo-yolo` (desde main a03ceb71 + la spec 7ba92f79). Plan: `docs/superpowers/plans/2026-10-18-modo-yolo.md`. Spec: `docs/superpowers/specs/2026-10-18-adbuddy-modo-yolo-design.md`. Compañero `yolo`.

## Entorno (sin rutas personales)

- Host: `cd host && go vet ./... && go test -timeout 20m ./...`. Sin `-race` en este PC (solo en la CI).
- App: Gradle 9.8.0 de `$LOCALAPPDATA/Gradle`, `ANDROID_HOME=$LOCALAPPDATA/Android/Sdk`, `--offline --no-daemon`; no hay `gradlew`. Imágenes de pantalla: `-Padbuddy.screens=record` solo en la clase tocada.
- Emulador (solo T9): AVD `adbuddy-dev34` (API 34), `ADBUDDY_EMU_IMAGE='system-images;android-34;google_apis;x86_64' ADBUDDY_EMU_AVD=adbuddy-dev34 ADBUDDY_EMU_STABLE_SECS=120`; el líder reparte el turno (máx. 2 locales, ≥ 9 GB libres).
- Esta fase (plan + revisión) no arrancó ningún emulador ni tocó ningún móvil.

## Verificación de premisas de la spec contra el código (2026-10-18)

- Números de línea de `daemon.go` (:430, :435, :440, :445, :462-465, :453-483, :485-571, :578, :592, :605-611): coinciden.
- Patrón `remote.set`/`remote.state`: existe (`cli/phone_remote.go`, `area` de `phone_areas.go`); `yolo.set` lo repite con un `area` propio.
- `UnlockWait` y `ConfirmArming`: existen, pero están hechos para botones de notificación entregados con el móvil bloqueado; el aviso de activación vive dentro de la app, así que se reutiliza la regla (bloqueo leído de nuevo al tocar + `ConfirmUi.ARMING_MS`), no las clases.
- Hook de Claude Code: **no** está en `claudecode/hook.go` (solo da `PermissionOutput`); se resuelve en `cli/phone_perm.go` (`permissionRequest`), llamado desde `phone_agent.go`.
- «Serial distinto: se ejecuta»: el ejecutor está atado al móvil activo (`adb.Runner.Command` antepone `-s Serial`), así que solo puede ejecutarse en el móvil activo.
- Opciones globales prohibidas: `classify.SplitGlobal` no las pasa a adb, así que «se ejecuta» es «se ejecuta sin ellas, por el servidor de adb de ADBuddy».
- La spec habla de `protocol_chat.go` para los permisos; ahí solo está el tipo `PermissionResolved`.

## Rulings

- R1. Con YOLO se saltan las apps protegidas del usuario, la persiana de notificaciones y ventanas del sistema, el fallo del aviso en pantalla y la ocultación de las notificaciones de las apps protegidas; **no** se salta nada de lo que protege a ADBuddy (ADBuddy delante o en una mitad de pantalla dividida, la regla «un comando que toca o lee la pantalla corre solo», y el «no sé qué hay delante» que falla cerrado) — por qué: si el agente pudiera tocar la pantalla de ADBuddy podría llegar al interruptor de YOLO o a la pausa, y el primer ruling de la spec (solo el móvil activa YOLO) dejaría de ser verdad con un móvil sin bloqueo — qué cuesta si me equivoco: con YOLO el agente no puede tocar ni leer ADBuddy y, si `dumpsys` falla, tampoco la pantalla; la tabla de la spec dice «protección de pantalla: se ejecuta» sin esta excepción. **Pregunta a la persona usuaria** (dudas del informe).
- R2. La pausa se pregunta siempre por `d.paused()`, que apaga YOLO al verla (un `sed` mecánico sustituye todos los `d.o.Paused()`) — por qué: un único sitio, imposible dejar una puerta que pregunte la pausa sin apagar YOLO — qué cuesta: un `Off` más en cada comprobación (no hace nada si ya está apagado).
- R3. «Serial distinto» y «opción global prohibida» se levantan quitando el destino/la opción, no abriendo el acceso: el comando se ejecuta en el móvil activo y por el servidor de adb propio, y el agente lo lee en la línea `⚠ YOLO:` — por qué: el ejecutor no puede alcanzar otro móvil u otro servidor; hacerlo exigiría un cambio de diseño — qué cuesta: un comando pensado para otro móvil (`-s otro rm …`) se ejecuta en el activo; la línea lo dice, pero ya habrá corrido.
- R4. `KindYolo` solo tiene `off` y `status`; cualquier otro `Argv` es `YOLO_ONLY_FROM_THE_PHONE`; la CLI ni siquiera tiene `on`; y un test de código fuente (`TestYoloIsOnlyTurnedOnByThePhoneLink`) falla si `EnableSession`/`YoloShort`/`YoloLong` aparecen fuera de `daemon/yolo.go` y `cli/phone_yolo.go` — por qué: una inyección de prompt no debe poder quitarse los frenos — qué cuesta: añadir un sitio legítimo de activación exige tocar el test, que es lo que queremos que pase por revisión.
- R5. `daemon.Yolo`: `notifyMu` antes que `mu`; los observadores corren con `notifyMu` y sin `mu`, y no pueden activar ni apagar — por qué: los cambios y sus avisos (diario, `yolo.state`) salen en orden y sin interbloqueo con el bloqueo del servidor del bridge (`Snapshot` solo lee `State()`) — qué cuesta: un observador que llamara a `Off` se bloquearía; el comentario y el diseño lo prohíben, y no hay ninguno que lo haga. Si el controlador quiere tratar T2/T6 como «concurrencia/IPC», este ruling es el escrito que pide `adbuddy-implementer-opus`.
- R6. `yolo.set` con el agente en pausa se rechaza (`failed`); `off` se acepta siempre; los tres campos van siempre en el JSON y se exige exactamente uno — por qué: la pausa apagaría el modo en el siguiente comando, mejor decirlo; y un `{}` o dos campos son una petición mal formada — qué cuesta: la persona tiene que reanudar antes de activar.
- R7. Permisos de Claude Code con YOLO: `allow` en el host sin tarjeta, aunque la app no esté conectada y aunque la tarjeta no pudiera enseñar la orden (`CardSafe` falso); nunca `allow_always`; `permission.resolved` lleva `by: "yolo"` y un `summary` escapado con `classify.Visible` — por qué: YOLO sobrevive a la app y nadie lee tarjetas; el resumen es texto del agente — qué cuesta: un campo aditivo más en el protocolo (`permission.resolved.summary`), que necesita el visto bueno del líder; y una orden con caracteres ocultos se aprueba (el resumen que ve la persona sale escapado).
- R8. Activar YOLO no toca lo que ya está pendiente (preguntas 🔴 ni tarjetas de permiso) — por qué: lo pendiente lo decide la persona que lo vio, y resolverlo sin que lo lea sería una aprobación que nadie dio — qué cuesta: una vez al activar, la persona responde lo que ya tenía en pantalla (las nuevas ya no se preguntan).
- R9. Cada entrada del diario lleva `yolo: true` si YOLO estaba activo al empezar (verdes incluidas); los cambios del modo son entradas verdes con regla `yolo.on`/`yolo.off` (nada que deshacer); una inversa 🔴 de `undo` que corre sin preguntar queda con regla `undo-yolo` — por qué: la regla `undo-confirmed` dice que la persona confirmó — qué cuesta: las pantallas que filtren por `undo-confirmed` no verían estas (no hay ninguna).
- R10. Con el diario sin poder escribirse, el comando corre igual y el agente lo lee (`… ran without a record and cannot be undone`); la entrada final no se escribe y `OnWarning` lo cuenta — por qué: spec; el rastro no limita al agente pero sin él no hay `undo` — qué cuesta: ese comando no se puede deshacer.
- R11. Las preguntas de `serve` mismo (`AskUser`: `adbuddy remote on`) no se levantan — por qué: son órdenes de la persona desde su PC, no del agente (el agente no puede pedir `remote on`), y quien las contesta es la persona — qué cuesta: nada.
- R12. En la app, las protecciones solo se levantan con el modo activo **y el enlace arriba**; con el enlace caído o el estado desconocido protegen como antes — por qué: el estado de la app podría estar viejo (el host pudo apagarlo) y es la parte que falla cerrada — qué cuesta: con el enlace caído la app no recibe peticiones de todos modos.
- R13. La banda de la app sigue con el enlace caído; un estado ilegible es «desconocido» y no se pinta como apagado — por qué: el modo vive en el host y sobrevive a la app (spec); un falso «apagado» escondería un modo encendido — qué cuesta: tras un reinicio del host con la app desconectada, la banda sigue hasta que el nuevo `welcome` (sesión distinta) borra el estado.
- R14. El aviso de activación no tiene duración por defecto, deja el botón sin enviar hasta pasar `ConfirmUi.ARMING_MS` (de nuevo cada vez que la app vuelve al primer plano), lee `isDeviceLocked` otra vez al tocar y exige el PC conectado — por qué: spec («desbloqueo y retardo de armado»), con las reglas de Allow de una pregunta roja — qué cuesta: 0,6 s de espera al activar.
- R15. La notificación persistente de YOLO es `ongoing`, `setLocalOnly`, pública en la pantalla de bloqueo y su «Apagar» no pide desbloqueo; sin conexión para mandar el `off` lo dice y remite a `adbuddy yolo off` — por qué: spec («apagar siempre es seguro») — qué cuesta: cualquiera con el móvil en la mano puede apagar YOLO desde la pantalla de bloqueo (es lo que se quiere).
- R16. Protocolo: tres campos siempre presentes en `yolo.set` (sin `omitempty`) para que Go y Kotlin redondeen el mismo fixture; `yolo.state` es un evento y la respuesta con `rid` repite el estado que el observador ya publicó (la app lee los dos igual) — por qué: simplicidad y simetría con `remote.state` — qué cuesta: un mensaje repetido por cada cambio pedido desde la app.
- R17. La caducidad se mide con el reloj de pared (`Round(0)`) y `Active()` la mira sin esperar al temporizador, que solo sirve para avisar; si el temporizador salta pronto, espera el resto — por qué: el PC puede dormirse con el modo activo y el temporizador monotónico podría no contar el sueño — qué cuesta: la notificación de «apagado» se retrasa hasta el siguiente comando o la siguiente comprobación si nadie pregunta.
- R18. La entrada «YOLO mode…» del menú del chat abre el aviso (o apaga, si ya está activo); el aviso lo pinta `ChatRoot` por encima del chat y de Ajustes — por qué: un solo sitio para el aviso y para `yolo.set` — qué cuesta: nada.

## Revisión del plan (`adbuddy-reviewer-opus`)

(pendiente: se rellena tras la revisión)

## Tareas

(se rellena durante la ejecución: una línea por tarea con SHA, rondas y revisor)
