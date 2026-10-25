# Modo YOLO — diseño

Fecha: 2026-10-18. Estado: diseño aprobado por la persona usuaria (sección 1 en la conversación del 2026-10-18).

## Propósito

Un modo que la persona usuaria activa desde el móvil, durante un tiempo acotado, en el que el agente opera directamente: **ninguna confirmación y ningún bloqueo fijo**, salvo la pausa. Es una decisión explícita de la persona usuaria, no un hueco: el ruling "un hueco que deje ejecutar 🔴 sin confirmación no se aparca nunca" sigue vigente para todo lo que no sea este modo activado a propósito.

## Qué decide la persona usuaria (ya decidido)

- **Alcance: absolutamente todo.** 🔴 normales y especiales, bloqueos fijos y tarjetas de permiso de Claude Code.
- **Pausa:** sigue frenando y además apaga YOLO.
- **Activación:** solo desde el móvil, con caducidad; no sobrevive a reiniciar el host.

## Rulings del diseño

- Ruling: el agente no puede activar YOLO; solo la app por el canal autenticado — si el agente pudiera, una inyección de prompt se quitaría los frenos sola — si me equivoco, la persona usuaria tiene que coger el móvil para activarlo, nada más.
- Ruling: el diario se sigue escribiendo siempre que se pueda; si no se puede, el comando corre igual y se avisa "sin diario" — conservar el rastro y el undo no limita al agente — si me equivoco, un intento de escritura extra por comando.
- Ruling: YOLO sobrevive a que la app se desconecte (hasta su caducidad) — la persona usuaria pidió que opere directamente y siempre queda `adbuddy yolo off` en el PC y la caducidad — si me equivoco, hasta 1 h sin poder apagarlo desde el móvil si se cae el enlace.
- Ruling: las tarjetas de permiso de Claude Code se responden `allow`, nunca `allow_always` — YOLO es temporal y `allow_always` deja reglas permanentes en `localSettings` — si me equivoco, ninguno: el efecto en la sesión es el mismo.
- Ruling: activar exige, como último paso, biometría fuerte (`BiometricPrompt` con `BIOMETRIC_STRONG` solo) si el móvil la tiene y, si no, la credencial del bloqueo de pantalla; si el móvil no tiene bloqueo de pantalla seguro, YOLO se activa igualmente tras un aviso específico y el mismo retardo de armado — con YOLO ya activo el agente puede dejar un proceso en segundo plano (la guarda no ve dentro de un script) que abra ADBuddy y pulse «hasta el fin de la sesión» después de la caducidad; un toque inyectado por adb no supera una autenticación del sistema, un PIN puede observarse (`getevent`, `screenrecord`) y sin bloqueo de pantalla nada prueba que sea la persona — si me equivoco: quien no tenga biometría fuerte asume el riesgo del PIN, y quien no tenga bloqueo de pantalla acepta que un toque inyectado puede terminar la activación (ambos escritos en `docs/safety.md`). (Decisiones de la persona usuaria, 2026-10-18.)
- Ruling: la propia app ADBuddy sigue protegida con YOLO activo (es la excepción a «absolutamente todo»): ADBuddy delante o en una mitad de pantalla dividida, lo que la app marca como protegido, la regla de «un comando que toca o lee la pantalla corre solo» y el «no sé qué hay delante» siguen bloqueando — si el agente pudiera tocar la pantalla de ADBuddy llegaría al interruptor de YOLO o a la pausa y el primer ruling (solo el móvil activa YOLO) dejaría de ser verdad — si me equivoco: con YOLO el agente no puede tocar ni leer ADBuddy. Las apps protegidas del usuario, la persiana de notificaciones y el aviso en pantalla sí se saltan. (Decisión de la persona usuaria, 2026-10-18.)

## Comportamiento en `Daemon.exec` (host/internal/daemon/daemon.go)

Con YOLO activo, cada puerta actual hace lo siguiente:

| Puerta | Hoy | Con YOLO |
|---|---|---|
| Pausa (`:430`, `:578`) | bloquea | **bloquea y apaga YOLO** |
| Otra pregunta esperando (`:435`) | bloquea | no aplica (no se pregunta nada); se ejecuta |
| Opción global de adb prohibida (`:440`) | bloquea | se ejecuta |
| Serial distinto del móvil activo (`:445`) | bloquea | se ejecuta |
| Diario no escribible para 🟡/🔴 (`:462-465`, `:605-611`) | bloquea | se ejecuta; aviso "sin diario" |
| Precheck que sube a 🔴 (`:453-483`) | confirma | se ejecuta |
| 🔴 normal (`:485-571`) | confirma | se ejecuta |
| 🔴 especial | confirma | se ejecuta |
| Protección de pantalla (`:592`, `refuse`) | bloquea | se ejecuta, salvo lo que protege a la propia app ADBuddy (ver el ruling siguiente) |
| Undo con inversa 🔴 | confirma | se ejecuta |

La clasificación se sigue haciendo igual (sirve para el diario y para el aviso). `rules.yaml` no cambia.

Cada comando que se ejecuta gracias a YOLO (es decir, que sin YOLO se habría confirmado o bloqueado) escribe en stderr del shim una línea `⚠ YOLO: <motivo saltado>`, para que el agente lo sepa. Cada entrada del diario lleva `yolo: true` cuando YOLO estaba activo.

## Estado

- `YoloState{until time.Time, session bool}` en el daemon, solo en memoria, protegido como el resto del estado del daemon. Activo si `session` o `now < until`.
- Duraciones que acepta el host: 15 min, 1 h o hasta el fin de la sesión (`session`: hasta que acabe `adbuddy serve`). El host rechaza otros valores (`error`).
- Se apaga por: `yolo.set {off}` desde la app, caducidad (temporizador del host), pausa, `adbuddy yolo off` en el PC o fin del proceso.
- Cada cambio (incluida la caducidad) emite `yolo.state` a la app y se apunta en el diario como evento.

## Protocolo (host/internal/bridge/protocol.go y android bridge/Protocol.kt)

Sigue el patrón de `remote.set` / `remote.state`:

- App → host: `yolo.set {"minutes": 15 | 60}`, `yolo.set {"session": true}` o `yolo.set {"off": true}`.
- Host → app: `yolo.state {"active": bool, "until": <epoch ms o 0>, "session": bool}`; también dentro de `snapshot` y tras `welcome`.
- Solo se acepta por el canal de la app autenticado con el token. El IPC del shim y la CLI no tienen operación de activar; la CLI solo tiene `adbuddy yolo off` y `adbuddy yolo status`.

## Claude Code (host/internal/claudecode/hook.go, bridge/protocol_chat.go)

Con YOLO activo, cada `permission.request` se resuelve en el host como `allow` sin enviarse a la app como pregunta. La app recibe `permission.resolved` y lo muestra en el chat como "aprobado por YOLO".

## App Android

- **Activar:** interruptor en Ajustes (y acceso desde la cabecera del chat). Abre un aviso que enumera lo que se salta (todas las confirmaciones, otros dispositivos, sin diario si falla, apps protegidas, permisos del agente) y obliga a elegir la duración. Se confirma, tras el retardo de armado de `ConfirmArming` (`ConfirmUi.ARMING_MS`), con **biometría fuerte** (`BiometricPrompt` con `BIOMETRIC_STRONG` solo) si el móvil la tiene y, si no, con la **credencial del bloqueo de pantalla** (PIN, patrón o contraseña); si el móvil no tiene bloqueo seguro, se muestra un aviso específico del riesgo (un toque inyectado podría terminar la activación) con su propio retardo de armado y, si se acepta, se activa. No basta con comprobar que el móvil está desbloqueado (`UnlockWait`, `isDeviceLocked`): la app está delante, así que lo está siempre, y un proceso en segundo plano del agente podría tocar el botón de activar.
- **Mientras está activo:** banda fija de color de alerta en todas las pantallas con la cuenta atrás (o "hasta fin de sesión") y el botón Apagar; notificación persistente con la acción Apagar (no requiere desbloqueo: apagar siempre es seguro). Las entradas del diario con `yolo` llevan una marca.
- **Apagar** nunca pide confirmación.
- Textos en todos los idiomas de M6c.

## Tests

- Host, con adb falso: una prueba por fila de la tabla (con YOLO se ejecuta; sin YOLO, el comportamiento actual), validada con una mutación que la ponga en rojo. Caducidad con reloj inyectado. La pausa apaga YOLO. `yolo.set` desde el IPC del shim no existe (el agente no puede activarlo). Valores de duración inválidos se rechazan. Permisos de Claude Code: `allow`, nunca `allow_always`. Ida y vuelta del protocolo.
- Android: reducer y parser de `yolo.state`; tests de UI (aviso, duración, armado, banda, apagar); instrumentado en emulador del flujo activar → banda → apagar desde la notificación. Capturas para la galería de M6d si aplica.
- Nada se prueba en el móvil real de la persona usuaria; lo que no se pueda automatizar va a "Validaciones manuales pendientes" del plan.

## Fuera de alcance

- Activar desde el PC, YOLO persistente entre reinicios, YOLO parcial (por nivel o por comando).
- Cambios en el clasificador.
