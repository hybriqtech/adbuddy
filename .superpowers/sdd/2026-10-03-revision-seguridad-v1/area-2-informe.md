# Área A2 — IPC / peer / daemon / sesión del agente · Informe de revisión de seguridad

Revisor independiente, solo lectura. Base: `docs/revision-seguridad-v1` (= main 5dd5e1a).
Pruebas: tests desechables sobre una copia del módulo `host` en el scratchpad (borrada al terminar; el árbol del repo queda limpio). No se tocó el móvil ni la red.

Marco: `docs/safety.md` ya dice que un agente decidido a saltarse ADBuddy puede hacerlo (otro adb contra el servidor 5038, procesos fuera de su árbol), y la sección "Ola 2 H" de `pendientes-m3-m5.md` documenta los límites de V07. Esos caminos no se repiten aquí como hallazgos. Me he centrado en lo que falla **dentro** del modelo prometido: pausa, confirmaciones, atribución, el árbol del agente y los permisos del secreto.

---

## Hallazgos

### A2-1 · MEDIO · La pausa no detiene los cambios que ya están en marcha — `host/internal/daemon/daemon.go:571-603`, `cli/serve.go:231` (`link.OnPause(d.CancelConfirmations)`)

**Qué pasa.** Al pausar desde el móvil solo se retiran las preguntas abiertas (`CancelConfirmations`). Una orden ya admitida sigue con el `ctx` de su petición, que la pausa no cancela. El clasificador da 🟡 a una línea con espera o bucle delante de un cambio (confirmado con test): `sleep 600; settings put global …`, `sleep 600; pm disable-user --user 0 …` y `while true; do settings put …; sleep 1; done`. La guarda de pantalla obliga a que `input` vaya sola, pero no hay nada parecido para los demás cambios 🟡.

**Escenario.**
1. El agente (con una inyección de prompt) lanza un cambio 🟡 con una espera larga delante, o un bucle de cambios 🟡.
2. La persona ve algo raro y pulsa **Pausa**.
3. El cambio pendiente se ejecuta igualmente mientras está en pausa, o el bucle sigue cambiando cosas. Termina como `ok` en el diario. Confirmado con test: un `sleep 600; settings put global a 2` admitido antes de la pausa acaba con `exit 0` y `status ok` después de pausar (`gatedADB` y `CancelConfirmations`).

Efecto secundario: mientras ese cambio sigue en vuelo, `waitChanges` no deja que aparezca ninguna pregunta 🔴, tampoco la de `adbuddy remote on`. Falla de forma segura, pero bloquea.

**Por qué no lo cubre un control existente.** `safety.md` dice que la pausa "rechaza las órdenes adb **nuevas**", así que el texto es técnicamente cierto. Pero presenta la pausa como el freno del uso del móvil por parte del agente, y la persona esperará que pare lo que está en marcha. La única forma de cortarlo es **Cortar acceso** o parar `serve`.

**Arreglo mínimo.** Al recibir la pausa, cancelar el contexto de los cambios en vuelo (el registro `inFlight` ya existe; basta con guardar el `cancel` de cada uno en `flight` y llamarlo desde `CancelConfirmations`). Así acaban como `error` o incierto, igual que con Ctrl+C en `serve`. Como alternativa, o además, decirlo en `safety.md` ("una orden que ya corre no se para") y que la guarda trate `sleep` y los bucles delante de un cambio como hace con la pantalla.

**Nota relacionada (A1/precheck).** La espera también alarga sin límite la distancia entre el precheck y el cambio. Por ejemplo, en `sleep 60; mv a b` se comprueba que `b` no existe al admitir la orden, y el `mv` se ejecuta un minuto después. Si en ese tiempo aparece `b`, se sobrescribe sin 🔴. Es difícil de provocar a propósito, porque los cambios con `Op` se serializan (`d.changes`) y lo que escribe ficheros sin `Op` sale 🔴. Pero el precheck deja de describir el estado real.

**Confianza.** Confirmado con test (daemon) y con el clasificador.

---

### A2-2 · BAJO · Al parar o reiniciar el agente, su árbol deja de contar como agente antes de que muera — `host/internal/session/session.go:359-383` (`stopLocked`) y `session/tree.go:18-21`

**Qué pasa.** `stopLocked` pone `m.cur = nil` **antes** de colgar y de matar el árbol, y espera hasta `stopWait` a que el proceso acabe. Mientras tanto, `InAgentTree` responde `(false, true)` para cualquier PID. Los procesos del árbol anterior que sigan vivos se tratan como humanos en `attach`, `new`, `stop` y `remote`. Pasa en todas las plataformas, no solo en el residuo Unix ya anotado de los descendientes que cambian de grupo. Pasa también durante el `stopLocked` que hace `startLocked` en cada `new`, también el que se pide desde el móvil.

**Impacto.** Limitado. `remote on` sigue preguntando a la persona, y durante la parada no hay sesión a la que hacer `attach`. Sí puede pedir `new` o `stop` como si fuera humano, o colocar un `attach` en el momento del arranque siguiente. No ejecuta 🔴 sin confirmación.

**Arreglo mínimo.** Mientras dura la parada, mantener el `proc` saliente en una lista de "terminando" y que `InAgentTree` lo consulte también (o no poner `cur = nil` hasta después del `Kill`).

**Confianza.** Lectura del código; no probado.

---

### A2-3 · BAJO · `exec` y `ui` siguen atribuyendo la orden según lo que declara el cliente — `daemon/daemon.go:416-419`, `daemon/ui.go:109-115`, `cli/phone.go:326`

**Qué pasa.** V07 hace que `serve` decida por el par del socket, pero solo en `attach`, `session` y `remote`. En las órdenes adb y en los helpers, `Source` sigue saliendo de `ADBUDDY_SOURCE`. Una orden del agente sin esa variable queda en el diario y en el panel de actividad del móvil como `user`, y no actualiza `session.state.activity` ("lo que hace el agente").

**Impacto.** Atribución engañosa en el diario y en el móvil: la persona puede creer que un cambio fue suyo. No cambia el nivel ni las confirmaciones (comprobado: nada más usa `SourceUser` o `SourceAgent` para decidir).

**Arreglo mínimo.** En `serveHandler`, calcular `peerIsAgent` también para `KindExec` y `KindUI` y pasarlo al daemon (por ejemplo, forzando `req.Env["ADBUDDY_SOURCE"]="agent"` cuando el par está en el árbol).

**Confianza.** Confirmado leyendo el código.

---

### A2-4 · BAJO · En Windows, el secreto y la carpeta de datos solo están protegidos por la ACL heredada — `ipc/ipc.go:149-200`, `ipc/server.go:199`, `paths/paths.go:54`

**Qué pasa.** `MkdirAll(…, 0o700)` y `CreateTemp` (0600) no hacen nada con las ACL de Windows. En la ruta por defecto (`%LOCALAPPDATA%\ADBuddy`) la ACL heredada ya es solo del usuario, de SYSTEM y de Administradores: correcto. Pero `install.md` documenta `ADBUDDY_DATA_DIR`, y el error de ruta de socket demasiado larga sugiere a la persona que lo use. Una carpeta creada bajo `C:\` hereda `Authenticated Users:(OI)(CI)(IO)(M)` (comprobado con `icacls C:\` en este PC; no se creó ninguna carpeta). Así, otros usuarios locales podrían leer `ipc.secret`, que es también la clave de firma del diario, y modificar lo que haya en esa carpeta (las platform-tools que ejecuta `serve`).

**Por qué no lo cubre un control existente.** El comentario dice "readable only by the user", y eso solo es cierto con la ruta por defecto. En Unix, 0700 y 0600 sí protegen.

**Arreglo mínimo.** En Windows, crear la carpeta de datos con una DACL protegida solo para el usuario (sin herencia), o comprobarla al arrancar y avisar o negarse. Como mínimo, decirlo en `install.md` junto a `ADBUDDY_DATA_DIR`.

**Confianza.** Confirmado leyendo el código y la ACL de `C:\`; no se reprodujo creando la carpeta.

---

### A2-5 · BAJO (coherencia) · Las órdenes de la CLI reservadas a la persona solo miran la variable de entorno — `cli/setup.go:91`, `cli/connect.go:50`, `cli/doctor.go:162` (`--fix`), `cli/remotecmd.go:74` (`off` sin `serve`), `cli/serve.go:55`

**Qué pasa.** Estas órdenes actúan con el adb real sin pasar por `serve`, así que V07 no las cubre: solo comparan `ADBUDDY_SOURCE`. Para `serve`, `safety.md` ya dice que es "un control contra errores, no una barrera". Para `setup` (que decide qué app recibe el token y los permisos) y `connect` no lo dice.

**Arreglo mínimo.** Documentarlo junto a lo de `serve`. Opcionalmente, cuando haya un `serve` vivo, preguntarle por el par (un `KindPing` que devuelva si el que llama está en el árbol del agente) antes de seguir.

**Confianza.** Lectura del código. No da al agente más de lo que `safety.md` ya admite.

---

## Revisado y dado por correcto

- `ipc/server.go` `handle()`: el par se captura antes de leer; el plazo de 5 s y el hueco de pre-autenticación se liberan una sola vez (`OnceFunc`); comparación del secreto en tiempo constante; un rechazo mantiene su hueco ≤2 s más, así que está acotado.
- Cupo de pre-autenticación (32): ocuparlo es una denegación de servicio local y acotada que ya reconoce el código. No afecta a la pausa, a las confirmaciones ni a Cortar acceso, que no van por IPC.
- `ReadFrame`: no reserva memoria antes de recibir los bytes; `maxStdin` es de 16 MiB por petición. Las conexiones autenticadas no tienen límite, pero eso solo permite al agente tumbar su propio `serve`, y falla cerrado.
- `Listen`: candado por carpeta de datos antes de Stat/Dial/Remove, socket con 0600 en Unix y carpeta 0700. `EnsureSecret`: temporal + rename bajo candado, 256 bits.
- `peer_windows.go`: probado aquí que un cliente que cierra antes del `accept` sigue identificado (`known`, mismo PID), y que un cliente ya terminado da `Same()=false`, es decir, agente. Falla cerrado.
- `peerIsAgent`: la declaración solo añade rechazos; `Err` o `PID<=0` cuentan como agente; `Same()` se comprueba después de consultar el árbol (un PID reciclado entre medias cuenta como agente).
- `tree_unix.go` / `tree_windows.go`: ESRCH y errores cuentan como "dentro"; job de Windows sin `BREAKAWAY_OK` y con `KILL_ON_JOB_CLOSE`; con más de 4096 procesos da error, es decir, agente (ya anotado).
- `handleRemote`/`confirmRemoteOn`: rechaza al agente, pregunta siempre (`Special`, sin `Key`, ID nuevo), pasa por la puerta de preguntas del daemon y comprueba la pausa antes y después.
- `handleSessionIn`: solo agentes conocidos, nunca una ruta que venga en la petición.
- `daemon.exec`: comprueba la pausa en cada espera (candado, pregunta, guarda); una 🔴 bloquea las demás; las órdenes en vuelo hacen esperar a la pregunta; solo se acepta el teléfono activo; `-H/-P/-L` prohibidos (A1).
- `daemon.ui`: la misma puerta, la guarda y la pausa.
- `handleHook`/`permissionRequest`: la respuesta del móvil solo vuelve a quien la pidió y no se escribe nada en el PTY. Una tarjeta falsificada solo retiene el chat (ya anotado).
- `agent.Env`: no se filtra el entorno, por decisión de producto; el secreto no va ni en el entorno ni en los argumentos (el shim lo lee del disco).
- Entrega del token del canal del móvil por stdin de `content write`, no en argv (detalle en A4).

## Preguntas abiertas

1. Pausa (A2-1): ¿debe parar lo que ya está en marcha? Si no, conviene decirlo en `safety.md` y en el texto del móvil.
2. La lista de formas de salir del árbol en "Ola 2 H" se podría completar con las capas de compatibilidad de Windows que crean procesos fuera del job. Mejor validarlo en el PC real (está en las validaciones manuales de WSL1) que dar ejemplos de memoria.
3. Linux y macOS (`SO_PEERCRED`, `LOCAL_PEERPID`, `getsid`) no se pueden ejecutar en este PC: dependen de la CI.
