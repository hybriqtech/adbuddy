# ADBuddy · VB-41 · Un puerto de `persist.adb.tcp.port` nunca se da por cerrado · Plan de implementación

> **Plan v2:** incorpora la revisión opus del plan (`.superpowers/sdd/2026-10-04-adbuddy-vb41-persist-port/plan-review.md`: 0 críticos, 2 importantes, 5 menores) y las decisiones del controlador: **I1** se arregla aquí (el vigilante del móvil relee el puerto cuando el móvil vuelve y avisa; ruling 9; doctor dice lo que hace `serve` de verdad); **I2** el ruling 3 queda como suposición, con su validación en emulador y el límite en pendientes, y la lectura de persist como `shell` pasó a **bloqueante antes de integrar** (medida después en el emulador: ya no bloquea); **M1** la lectura tiene plazo propio (`device.ReadTCPWait`); **M2** `Controller.Note` desde `serve` tras el invariante; **M3** comentario viejo de `persist_test.go`; **M4** `State.Persist` es la última lectura, dicho en el código; **M5** dos tests más. Encargo: `.superpowers/sdd/2026-10-04-adbuddy-vb41-persist-port/plan-brief.md`.
>
> **Para quien ejecute:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`. Los pasos usan casillas (`- [ ]`). Implementador y re-revisiones en Sonnet (`adbuddy-implementer`, `adbuddy-rereviewer`; si BLOCKED o ronda 4-5, `adbuddy-implementer-opus`). Las Tareas 1 y 2 tocan el modo remoto, el invariante del host y lo que el host afirma de un puerto abierto (🔒): las revisa `adbuddy-reviewer-opus`; la Tarea 3 (docs), `adbuddy-reviewer`. Nunca Haiku. **Antes de la Tarea 1, `adbuddy-reviewer-opus` revisa este plan completo, código incluido** (CLAUDE.md), y el plan se corrige con sus hallazgos.

**Meta:** que el host deje de dar por cerrado un puerto TCP de adbd que no ha leído. Hoy decide si adbd escucha por red leyendo **solo** `service.adb.tcp.port` (`remote.Disable`, `Reconcile`, `Enable`, `portClosed` en `host/internal/remote/remote.go`; `device.TCPPort` y `EnsureNoTCPPort` en `host/internal/device/device.go`, que usan el invariante de `serve`/`setup` y `doctor`). Pero adbd (AOSP) al arrancar toma `persist.adb.tcp.port` cuando `service.adb.tcp.port` está vacío, como tras un reinicio. Medido en el emulador API 34 (T8, 2026-10-03; `.superpowers/sdd/2026-10-04-adbuddy-validaciones-t8/evidencia/run3.txt` y la fila VB-41 del inventario): `persist.adb.tcp.port=5555` **no** se limpia con `remote off` ni con `adb usb`; tras `adb reboot`, `service.adb.tcp.port` queda vacío y el host lo toma por cerrado. En un móvil real, persist más un reinicio abre el 5555 en cualquier red sin que ADBuddy lo vea (eso sale de leer AOSP: el emulador escucha siempre en el 5555 y no deja ver el listener real). El usuario `shell` no puede fijar persist (`setprop` sale con 1, medido): el riesgo real son móviles con root o con persist ya fijado por otra herramienta.

**Arquitectura:** una sola lectura del puerto para todo el host, `device.ReadTCP`, que lee las dos propiedades y devuelve `device.TCP{Now, Persist}`: `Now` es el puerto en el que escucha adbd (`service.adb.tcp.port`, o `persist.adb.tcp.port` si aquel está vacío) y `Persist` es persist cuando nombra un puerto; si cualquiera de las dos lecturas falla, error (desconocido, nunca cerrado). `EnsureNoTCPPort` decide con `Now` y devuelve también lo leído. El controlador del modo remoto lee con `device.ReadTCP` antes de cada `adb usb` y de cada decisión, recuerda lo último que dijo de persist y lo devuelve en `remote.State{Persist, PersistUnknown}`. Cuando persist sigue fijado, el host **avisa** (no puede borrarlo sin root y no usará `su`): `adbuddy doctor` (⚠ con qué propiedad es, que no la fijó ADBuddy y cómo quitarla), el log de `serve` (invariante al arrancar, cada "off", la caducidad y cada vez que el móvil vuelve tras perderse: el vigilante relee el puerto, ruling 9; una vez por suceso o por cambio) y `adbuddy remote off|status` (texto y `--json`: `persistPort`, `persistUnknown`; desde el arranque, porque `serve` pasa la lectura del invariante al controlador con `Note`). Cada lectura que precede a otra cosa (`adb usb`, el vigilante) tiene su propio plazo corto, `device.ReadTCPWait`. `adb usb` solo corre cuando hay algo que cerrar ahora (`Now`): con `service.adb.tcp.port` a `0` (lo que deja `adb usb`) persist no se toca y solo se avisa, así que no hay bucle de reinicios. **No cambian:** el protocolo con la app (`remote.state` y la tarjeta), el clasificador, el shim, la comprobación de identidad, lo que hace el vigilante del móvil para encontrarlo (solo añade una lectura del puerto cuando vuelve; nunca abre ni cierra), ni el camino de `adbuddy remote off` sin serve con el modo ya apagado (ruling 7).

**Tecnologías:** Go 1.27 (`host/`). Nada nuevo en `go.mod`. `remote` pasa a importar `device` (sin ciclo: `device` solo importa `classify`).

**Fuentes:** encargo (arriba) · `CLAUDE.md` · spec v1 `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md` §3.3 (modo remoto, invariante) · spec M5 `docs/superpowers/specs/2026-10-01-adbuddy-m5-ayudas-remoto-design.md` §7.2 (el encargo dice §7.1, pero lo que habla de `adb usb`, el invariante y la caducidad es la §7.2; la §7.1 es el puerto de la depuración inalámbrica y no cambia) · plan de diferidos B `docs/superpowers/plans/2026-10-02-adbuddy-v1-diferidos-b.md` (V38 y validación 41) · plan T8 `docs/superpowers/plans/2026-10-04-adbuddy-validaciones-t8.md` (Tarea 5: `TestOffClosesAPersistTCPPort`, RED esperado) · inventario `docs/superpowers/plans/2026-10-03-validaciones-manuales.md` (fila VB-41) · `docs/superpowers/plans/pendientes-m3-m5.md` (diferidos B, T2) · evidencia T8 (`run3.txt`, `t235.txt`).

**Verificación del plan (réplica acumulada):** el plan se escribió sobre una copia (`git archive` de 8f881bf, la punta de la rama) implementando cada paso con su RED y su GREEN reales, y después **se volvió a aplicar entero desde el texto de este plan** con un script (cada par "Reemplazar / por" en orden, exigiendo que el ancla aparezca exactamente las veces dichas, y cada "Añadir al final") sobre otro `git archive` limpio de 8f881bf. Las salidas citadas son las de esa réplica (detalle en "Réplica acumulada"). Todo sin emulador ni móvil: adb falsos en memoria (`device.fakeADB`, `remote.env`, `cli.ctlADB`) y el `fakeadb` de `testutil`.

## Global Constraints

- Rama `fix/vb41-persist-tcp-port`, apilada sobre `test/validaciones-t8` (8f881bf). Commits con identidad local hybriqtech (nunca `git config --global`) y trailer **exacto** `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` (comprobarlo con `git log -1 --format=%B`). Nunca `--no-verify` ni `--force`, sin `git stash`. Sin push ni PR salvo que lo pida el controlador; a `main` integra el usuario. `git grep` de seriales e IPs antes de cualquier push.
- **Regla inviolable:** ningún cambio hace que el host afirme «cerrado» sin haberlo leído, y ninguno deja ejecutar un 🔴 sin confirmar (este plan no toca el clasificador, el shim ni las confirmaciones).
- **El host nunca ejecuta `su`** ni intenta escribir `persist.adb.tcp.port`. `adb usb` solo cuando hay algo que cerrar ahora (ruling 3).
- **Sin IPs RFC 1918** en código, tests ni docs: `100.64.0.0/10` (p. ej. `100.101.102.103`), `192.0.2.x`. Sin seriales reales: `phoneSerial` (`adb-AAAA1111-bbb._adb-tls-connect._tcp`) y `AAAA1111`.
- **El móvil del usuario no se toca. No se arranca ningún emulador en este plan** y nunca se matan procesos por nombre de imagen. El test de emulador (`TestOffClosesAPersistTCPPort`) se actualiza y compila aquí; se corre aparte (ver "Validaciones pendientes").
- **Cada tarea compila y pasa sus tests por sí sola.**
- **Go:** `cd host && go build ./... && go vet ./... && gofmt -l .` y `go test -count=1 -timeout 15m ./...` al final de cada tarea de código. Este PC no corre `-race`: la concurrencia se razona en la tarea y la corre la CI.
- `host/**` es LF (`.gitattributes`), los catálogos `host/internal/i18n/catalog/*.toml` también. `docs/**` está en **CRLF** en el árbol de trabajo (`core.autocrlf=true`): sus ediciones usan anclas de **una sola línea**.
- Textos nuevos de cara al usuario: solo los de "Textos nuevos", en **los dos** catálogos (`TestCatalogParity`, `TestKeysExist`, `TestNoTextLeftInTheCode`).
- Pasar a los revisores límites de tiempo (`go test -timeout 15m`, tope de 30 minutos por revisión).
- Windows con Git Bash: sin `sleep` en primer plano; los comandos largos con timeout amplio o en segundo plano.

## Rulings de este plan (van al ledger tal cual)

1. **Ruling: el puerto en el que escucha adbd es `service.adb.tcp.port`, o `persist.adb.tcp.port` cuando aquel está vacío; cualquier valor que no sea vacío ni `0` es un puerto, también uno no numérico, negativo o enorme.** — Es la regla de adbd de AOSP al arrancar (lee persist solo con service vacío). `adb usb` deja service a `0` (medido en S1 y en la T8) y con él a `0` adbd ignora persist hasta el siguiente reinicio. Un valor raro cuenta como abierto: el lado seguro, como hoy con service (`"abc"` ya es abierto en `TestEnsureNoTCPPortEdgeCases`). Los valores se recortan a 16 runas y se hacen imprimibles al leerlos (`device.portValue`), así que todo el que los muestra (doctor, log, CLI) está a salvo. — Si me equivoco: un `adb usb` de más (un reinicio de adbd y un corte breve) por un valor que adbd no habría aceptado.
2. **Ruling: si cualquiera de las dos lecturas falla, el puerto es desconocido, nunca cerrado; cada sitio conserva su política ante lo desconocido.** — Invariante: `DEVICE_UNREADABLE` sin `adb usb` (como hoy con service). `Disable`: `adb usb` igualmente (como hoy: "a port that cannot be read is closed all the same") y estado `PersistUnknown`. `Reconcile` y `Enable`: no tocan la caducidad (como hoy). Doctor: «no se pudo leer», con un texto que ya no nombra solo service. — Si me equivoco: con persist ilegible y service abierto, el invariante para `serve` en vez de cerrar; es lo que ya pasa hoy si falla la lectura de service, y se arregla volviendo a lanzar `serve`.
3. **Ruling (descansa en una suposición sin medir, revisión I2): persist fijado es «algo que cerrar» solo mientras `service.adb.tcp.port` esté vacío; con service a `0` solo se avisa.** — Suposiciones: que service a `0` implica que adbd arrancó después con él a `0` (cierto tras `adb usb`, medido; **no** se sabe tras un `setprop service.adb.tcp.port 0` suelto, y fijar persist no reinicia adbd, `run3.txt:13`: lo mide la validación 1 (e)), y que adbd solo escucha según esas dos propiedades (AOSP también mira `service.adb.listen_addrs` y abre el 5555 por defecto en un dispositivo sin USB: límite en pendientes). Por eso el aviso dice «cerrado por ahora» y nunca «cerrado» a secas. El encargo (criterio 2) dice "si persist no está vacío ni 0, intentar `adb usb`". Con service a `0` adbd no escucha en persist (AOSP), así que ese `adb usb` solo reiniciaría adbd y cortaría la conexión del móvil en cada arranque de `serve` y en cada "off", sin cerrar nada; y como `adb usb` deja service a `0`, esta regla es justo lo que impide el bucle (criterio 3): el primer `adb usb` tras un reinicio cierra, los siguientes no corren. — Si me equivoco: un móvil cuyo adbd sí escuchara en persist con service a `0` (no es AOSP) seguiría con el puerto abierto, pero avisado en doctor, en el log y en `remote off`, nunca callado.
4. **Ruling: la prueba de aceptación en emulador se redefine: tras "off" con persist fijado, adbd queda sin puerto ahora (service vacío o `0` y adbd reiniciado si había algo que cerrar), el host devuelve un estado que nombra persist (`remote.State{Persist: "5555"}`, `device.TCP{Persist: "5555"}`), persist sigue como estaba y una segunda pasada no reinicia adbd.** — Sin root el host no puede vaciar persist (shell sale con 1, medido) y no usará `su`; exigir que lo vacíe haría el test imposible sin meter `su` en producción. El test, además, **mide la premisa no medida del diseño**: que el usuario `shell` puede leer persist (el host lo lee sin `su`; la T8 lo leyó siempre con `su 0`). Los unitarios con adb falso (RED reales antes del código) son el contrato; el de emulador, la verificación. — Si me equivoco (shell no puede leer persist: SELinux devolvería vacío con código 0, como con `service.adb.tls.port`): el test de emulador falla con `Persist ""` y el diseño no sirve tal cual; por eso esa validación es **bloqueante antes de integrar** (revisión I2).
5. **Ruling: el controlador lee el puerto antes de `adb usb`, no después.** — Tras `adb usb` adbd se reinicia y la conexión cae: una lectura inmediata fallaría (siempre «desconocido») o leería el adbd viejo. `adb usb` no cambia persist (medido en la T8), así que lo leído justo antes vale para después. La relectura con espera la hace quien puede esperar: el invariante (ya espera a que el móvil vuelva y relee). — Si me equivoco: si algo cambiara persist entre la lectura y el `adb usb`, el estado diría el valor viejo hasta la siguiente lectura (doctor, el siguiente `serve`, el siguiente "off").
6. **Ruling: el aviso va al host (doctor, log de `serve`, `adbuddy remote off|status` y su `--json`), no a la tarjeta de la app.** — `remote.state` es protocolo: cambiarlo pide el ok de adbuddy-e5, fixtures en `protocol/` y la app. La tarjeta habla del modo, y el modo sí está apagado; el puerto está cerrado ahora (por `adb usb` o porque service está a `0`) y solo vuelve tras reiniciar el móvil. Queda en pendientes. — Si me equivoco: quien solo mira el móvil no ve el aviso hasta abrir doctor o mirar la terminal de `serve`.
7. **Ruling: `adbuddy remote off` sin serve y con el modo ya apagado en `config.toml` sigue sin tocar adb.** — No afirma nada del puerto (dice "remote mode is already off"); leer pediría arrancar el servidor adb y esperar al móvil (hasta `deviceWait`) en un camino que hoy es instantáneo. El invariante al arrancar `serve` y doctor sí leen. Con el modo encendido, `remote off` sin serve sí lee persist antes de su `adb usb` y lo dice. — Si me equivoco: quien nunca arranca `serve` ni doctor no se entera de un persist ajeno; ese puerto no lo abrió ADBuddy.
8. **Ruling: cada aviso se dice una vez por suceso, nunca por tick.** — Invariante: una vez por arranque de `serve`/`setup`. Cada "off" pedido (app o CLI): una vez, en el log de `serve` y en la respuesta. Caducidad: una vez (`Expire` solo cambia una vez; después la caducidad está vacía y no hace nada). Vigilante: solo cuando el móvil vuelve tras perderse y solo si la lectura cambió desde la última que se dijo (empieza con la del invariante). El controlador recuerda la última lectura (`State`, también la del invariante y la del vigilante por `Note`) para que `remote status` lo diga sin volver a leer; es la última lectura, no un valor vivo (revisión M4). — Si me equivoco: un aviso repetido si el usuario repite el "off"; nada se repite solo.
9. **Ruling (revisión I1): cuando el móvil vuelve con `serve` en marcha (tras una falta o un cambio de serial), el vigilante relee el puerto y avisa; no ejecuta `adb usb`.** — Un reinicio del móvil con `serve` arrancado le hace tomar persist, y hasta ahora nadie lo releía (el invariante solo corre al arrancar `serve`; `phone_watch.go` "no applyInvariant"). Cerrar ahí sería un `adb usb` a mitad de sesión que corta el móvil otra vez y puede cruzarse con el modo remoto (el vigilante no toma el cerrojo del controlador); avisar es seguro y basta para que el usuario no crea cerrado lo que no lo está: un puerto abierto con el modo apagado sale como "⚠ adb listens on network port N on the phone again and remote mode is off … Run `adbuddy remote off`" (y `remote off` con `serve` sí lo cierra), y persist, como en el resto. El 5555 con el modo encendido no se avisa como abierto (es el esperado; `RemoteOn` se lee después del puerto: `Enable` guarda la caducidad antes de `adb tcpip`). Doctor dice lo que hace `serve`: lo cierra al arrancar y, al reconectar el móvil, solo avisa. — Si me equivoco: el puerto sigue abierto hasta que el usuario haga `remote off` o reinicie `serve`, pero avisado en el log y en doctor.

## Revisión de sitios (criterio 1 del encargo: nunca «cerrado» sin leer)

| Sitio | Hoy | Tras el plan |
|---|---|---|
| `remote.Disable`, modo apagado | lee service; vacío o `0` → `State{}` sin `adb usb`; lectura fallida → `adb usb` y `State{}` | lee las dos; `Now` vacío → `State{Persist}` sin `adb usb` (con service a `0` y persist fijado, solo aviso); `Now` abierto (también persist tras un reinicio) → `adb usb`; lectura fallida → `adb usb` y `State{PersistUnknown: true}` |
| `remote.Disable` modo encendido, `Expire` | `adb usb` sin leer → `State{}` | lee antes (ruling 5), `adb usb`, `State{Persist}` o `State{PersistUnknown}` |
| reintento tras un guardado fallido (`closedFor`) | relee service: vacío o `0` → no repite `adb usb` | relee las dos: solo `Now` vacío salta `adb usb` |
| `remote.Reconcile` | service distinto de 5555 → caducidad vieja y se borra: tras un reinicio con persist 5555, el modo pasaba a "off" con el 5555 abierto | `Now` distinto de 5555 → se borra; persist 5555 con service vacío → el modo sigue encendido (y se cierra al caducar); lectura fallida → nada cambia |
| `remote.Enable` (tcpip falla, no por "sin móvil") | borra la caducidad si service no es 5555 | la borra si `Now` no es 5555; persist ilegible → no la borra |
| `device.EnsureNoTCPPort` (invariante) | service vacío o `0` → nada | `Now` vacío → nada (y devuelve persist para el aviso); persist ilegible → error |
| `applyInvariant` (`serve`, `setup`) | relee service tras `adb usb` | relee las dos; si sigue abierto por persist → `DEVICE_PORT_OPEN` con un arreglo que no dice "reinicia el móvil"; persist fijado → aviso en el log, una vez |
| `adbuddy doctor` | service vacío o `0` → ✓ cerrado | ✓ solo con `Now` vacío y persist vacío; persist fijado → ⚠ con qué hacer; lectura fallida → ⚠ "could not read adb's network port (…)" |
| `adbuddy remote off` / `status` (serve) | "Remote mode is off: port 5555 is closed." | con persist: "Remote mode is off. But adb's network port is not closed for good: …"; ilegible: "… could not be read …"; `--json` con `persistPort` / `persistUnknown` |
| `adbuddy remote off` sin serve, modo encendido | `adb usb` → "closed" | lee persist antes y lo dice |
| `adbuddy remote off` sin serve, modo apagado | no toca adb: "remote mode is already off" | igual (ruling 7) |
| log de la caducidad (`watchRemote`) | "Remote mode expired: port 5555 closed." | igual, más el aviso de persist |
| `serve` al arrancar (lectura del invariante) | no llega al controlador | `Controller.Note`: `remote status`/`--json` dicen persist desde el arranque (revisión M2) |
| el móvil vuelve con `serve` en marcha (`phone_watch`) | nadie relee el puerto (un reinicio con persist lo reabre toda la sesión) | relee con `ReadTCP` y avisa una vez por cambio: puerto abierto con el modo apagado, persist, o lectura fallida; nunca `adb usb` (ruling 9) |
| tarjeta de la app | apagado | igual (ruling 6) |

## Textos nuevos de cara al usuario

En `host/internal/i18n/catalog/en.toml` y `es.toml` (Tarea 2):

| Clave | Inglés | Español |
|---|---|---|
| `doctor.port.read_failed` (cambia) | could not read adb's network port (service.adb.tcp.port and persist.adb.tcp.port): %[1]s | no se pudo leer el puerto de red de adb (service.adb.tcp.port y persist.adb.tcp.port): %[1]s |
| `doctor.port.persist` | closed for now, but persist.adb.tcp.port is %[1]s: adb listens on port %[1]s again, on any network, every time the phone restarts | cerrado por ahora, pero persist.adb.tcp.port vale %[1]s: adb vuelve a escuchar en el puerto %[1]s, en cualquier red, cada vez que el móvil se reinicia |
| `doctor.port.persist_fix` | ADBuddy did not set it and cannot clear it without root: … only closes the port until the next restart. Turn off whatever set it (a ROM's 'ADB over network' setting, a root app or a command run earlier), or empty it as root (`setprop persist.adb.tcp.port ''`) and restart the phone. | ADBuddy no la fijó y no puede borrarla sin root: … solo cierra el puerto hasta el siguiente reinicio. Desactiva lo que la fijó (…), o vacíala como root (`setprop persist.adb.tcp.port ''`) y reinicia el móvil. |
| `err.invariant.device_port_open.persist_fix` | persist.adb.tcp.port is %[2]s on the phone and keeps the port open; … restart the phone and run `%[1]s` again. | persist.adb.tcp.port vale %[2]s en el móvil y mantiene el puerto abierto; … reinicia el móvil y vuelve a ejecutar `%[1]s`. |
| `remote.text.off_mode` | Remote mode is off. | El modo remoto está desactivado. |
| `remote.text.persist` | But adb's network port is not closed for good: persist.adb.tcp.port is %[1]s (ADBuddy did not set it), … `adbuddy doctor` says how to remove it. | Pero el puerto de red de adb no queda cerrado del todo: persist.adb.tcp.port vale %[1]s (ADBuddy no la fijó), … `adbuddy doctor` explica cómo quitarla. |
| `remote.text.persist_unknown` | But persist.adb.tcp.port could not be read: … Run `adbuddy doctor` to check. | Pero no se pudo leer persist.adb.tcp.port: … Ejecuta `adbuddy doctor` para comprobarlo. |
| `remote.persist.log` | ⚠ persist.adb.tcp.port is %[1]s on the phone (ADBuddy did not set it): … | ⚠ persist.adb.tcp.port vale %[1]s en el móvil (ADBuddy no la fijó): … |
| `remote.persist.unknown_log` | ⚠ Could not read persist.adb.tcp.port: … | ⚠ No se pudo leer persist.adb.tcp.port: … |
| `phone_watch.port_open` | ⚠ adb listens on network port %[1]s on the phone again and remote mode is off: serve does not close it while it runs. Run `adbuddy remote off` to close it now. | ⚠ adb vuelve a escuchar en el puerto de red %[1]s del móvil y el modo remoto está desactivado: serve no lo cierra mientras está en marcha. Ejecuta `adbuddy remote off` para cerrarlo ya. |
| `doctor.port.serve_running` | serve closes it only when it starts; when the phone reconnects while serve runs, it only warns. Run `adbuddy remote off` to close it now. | serve solo lo cierra al arrancar; si el móvil se reconecta con serve en marcha, solo avisa. Ejecuta `adbuddy remote off` para cerrarlo ya. |
| `doctor.port.nothing_to_do` (cambia) | Nothing to do: serve closes it when it starts (remote mode is off). | No hay nada que hacer: serve lo cierra al arrancar (el modo remoto está desactivado). |

Cambia también `doctor.port.persist_fix`: dice que `serve` ejecuta `adb usb` al arrancar y que, si el móvil se reinicia con `serve` en marcha, solo avisa (revisión I1). El texto completo de cada uno está en el paso 3 de la Tarea 2. "Quién la fijó": el host no puede saberlo; dice que **no** fue ADBuddy y nombra las fuentes habituales.

## Mapa de ficheros

| Fichero | Tarea | Cambio |
|---|---|---|
| `host/internal/device/device.go` | 1 | `TCP`, `ReadTCP`, `portValue`, `maxPortRunes`; `EnsureNoTCPPort` devuelve `(TCP, bool, error)` y decide con `TCP.Now`; fuera `TCPPort` |
| `host/internal/device/device_test.go` | 1 | adb falso con persist sin fijar por defecto, `ensureRan`, tests de persist, `TestReadTCP` (en lugar de `TestTCPPort`) |
| `host/internal/remote/remote.go` | 1 | `State.Persist`, `State.PersistUnknown`, `readTCP`, `disable(ctx, t, rerr)`; fuera `portClosed`; todas las lecturas por `device.ReadTCP` |
| `host/internal/remote/remote_test.go` | 1 | el adb falso responde persist; tests nuevos; secuencias de eventos con la lectura de las dos propiedades |
| `host/internal/cli/invariant.go` | 1, 2 | T1: firma nueva y relectura con `ReadTCP`; T2: aviso (`tellPersist`) y arreglo de `DEVICE_PORT_OPEN` con persist |
| `host/internal/cli/doctor.go` | 1, 2 | T1: `ReadTCP`; T2: ⚠ de persist |
| `host/internal/cli/remotecmd.go` | 2 | `remoteInfo.PersistPort/PersistUnknown`, textos, `remoteOffWithoutServe` lee persist |
| `host/internal/cli/phone_remote.go` | 2 | aviso en el log tras un "off" y al caducar |
| `host/internal/cli/phone_watch.go` | 2 | `phoneWatch.Port/Seen/ReadWait`, `recheckPort` al volver el móvil, `switchTo(ctx, …)` |
| `host/internal/cli/serve.go` | 2 | `invariantTCP`, `ctl.Note`, la lectura al vigilante |
| `host/internal/i18n/catalog/{en,es}.toml` | 2 | claves de "Textos nuevos" |
| `host/internal/cli/{invariant,phone_remote,remotecmd,doctor,doctor_lang,messages_lang}_test.go` | 1, 2 | T1: llamadas esperadas con la lectura de persist; T2: tests nuevos |
| `host/cmd/adbuddy/e2e_test.go` | 1 | la lista exacta de llamadas a adb incluye la lectura de persist |
| `host/internal/emutest/persist_test.go`, `remote_test.go` | 1 | aceptación redefinida (ruling 4); `hostReadsPersistPort = true` |
| `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md` | 3 | §3.3 |
| `docs/superpowers/specs/2026-10-01-adbuddy-m5-ayudas-remoto-design.md` | 3 | §7.2 |
| `docs/troubleshooting.md` | 3 | doctor, `DEVICE_PORT_OPEN`, síntoma nuevo |
| `docs/superpowers/plans/pendientes-m3-m5.md` | 3 | diferidos B T2 resuelto; sección nueva VB-41 |
| `docs/superpowers/plans/2026-10-03-validaciones-manuales.md` | 3 | fila VB-41 |

---

### Task 1: el host lee `persist.adb.tcp.port` y nunca da por cerrado lo que no leyó (remote, device) 🔒

**Files:** los de la Tarea 1 en el mapa.

- Produces: `type device.TCP struct { Now, Persist string }`; `func device.ReadTCP(ctx context.Context, a device.ADB) (device.TCP, error)`; `func device.EnsureNoTCPPort(ctx context.Context, a device.ADB, keep string) (device.TCP, bool, error)` (antes `(bool, error)`); `remote.State.Persist string`, `remote.State.PersistUnknown bool`; `func (c *remote.Controller) readTCP(ctx context.Context) (device.TCP, error)`; `func (c *remote.Controller) disable(ctx context.Context, t device.TCP, rerr error) (remote.State, error)` (antes `disable(ctx)`); `func (c *remote.Controller) Note(t device.TCP)`; `var device.ReadTCPWait = 10 * time.Second` (y el campo `readWait` del controlador, que `New` copia).
- Desaparecen: `device.TCPPort`, `remote.portClosed`.

**Concurrencia (razonada; la CI corre `-race`):** `persist` y `persistUnknown` del controlador van bajo un mutex propio, `pmu`, que solo toman `readTCP` (escribe) y `State` (lee). `State` se llama con `mu` tomado (desde `Enable`/`Disable`/…) y sin él (`remoteLink.refresh`, `handleRemote` "status"): por eso no puede tomar `mu`, y `pmu` nunca se toma con otro cerrojo de este paquete dentro, así que no hay orden de cerrojos que respetar. `readTCP` corre siempre con `mu` tomado (una lectura a la vez); `Note` solo toma `pmu` (la llaman `serve` antes de lanzar gorrutinas y el vigilante desde la suya). `device.ReadTCPWait` es una variable de test: `New` la copia en `readWait`, así que ninguna gorrutina del controlador la lee después (lo mismo hace el vigilante con `ReadWait`, que `serve` lee antes de lanzarlo). El adb falso `remote.env` lo usa un solo hilo, como hoy (`persHang` espera al contexto de la llamada).

- [ ] **Step 1: tests que fallan con el código de hoy.** Todos compilan con el código actual: los de `device` pasan por `EnsureNoTCPPort` a través de un ayudante (`ensureRan`) cuya única línea cambia en el paso 3; los de `remote` miran eventos y caducidad, no campos nuevos. Las ediciones van en este orden (el reemplazo múltiple antes de añadir `ensureRan`, que no debe tocarse).

Edit en `host/internal/device/device_test.go` (`replace_all: true`, 5 apariciones):

**Reemplazar:**
```go
EnsureNoTCPPort(context.Background(), f, 
```

**por:**
```go
ensureRan(f, 
```

(El ancla termina en un espacio tras la coma: `EnsureNoTCPPort(context.Background(), f, ` → `ensureRan(f, `.)

Edit en `host/internal/device/device_test.go`:

**Reemplazar:**
```go
	if _, err := EnsureNoTCPPort(context.Background(), &fakeADB{}, ""); err == nil {
```

**por:**
```go
	if _, err := ensureRan(&fakeADB{}, ""); err == nil {
```

Edit en `host/internal/device/device_test.go`:

**Reemplazar:**
```go
type fakeADB struct {
```

**por:**
```go
const (
	serviceProp = "shell getprop service.adb.tcp.port"
	persistProp = "shell getprop persist.adb.tcp.port"
)

// ensureRan is EnsureNoTCPPort's answer of whether it ran `adb usb`.
func ensureRan(f *fakeADB, keep string) (bool, error) {
	return EnsureNoTCPPort(context.Background(), f, keep)
}

type fakeADB struct {
```

Edit en `host/internal/device/device_test.go`:

**Reemplazar:**
```go
	out, ok := f.answers[key]
	if !ok {
		return 1, nil
	}
```

**por:**
```go
	out, ok := f.answers[key]
	if !ok && key == persistProp {
		out, ok = "\n", true // unset, unless the test says otherwise
	}
	if !ok {
		return 1, nil
	}
```

Añadir al final de `host/internal/device/device_test.go`:

```go

// VB-41: AOSP adbd takes persist.adb.tcp.port at its start when
// service.adb.tcp.port is empty (after a reboot), so the invariant reads both.
// Any persist value but empty or 0 is a port to close while the service
// property is empty; with the service property at 0, `adb usb` already ran
// since the boot and running it again would only restart adbd (no loop).
func TestEnsureNoTCPPortReadsPersist(t *testing.T) {
	for _, c := range []struct {
		name, service, persist, keep string
		wantUSB                      bool
	}{
		{"persist after a reboot", "", "5555", "", true},
		{"another persist port", "", "7777", "5555", true},
		{"persist is the kept port", "", "5555", "5555", false},
		{"service at 0", "0", "5555", "", false},
		{"persist 0", "", "0", "", false},
		{"nothing", "", "", "", false},
		{"not a number", "", "abc", "", true},
		{"huge", "", "99999999999999999999999", "", true},
		{"negative", "", "-1", "", true},
		{"both set", "5555", "7777", "", true},
	} {
		f := &fakeADB{answers: map[string]string{serviceProp: c.service + "\n", persistProp: c.persist + "\r\n", "usb": "restarting in USB mode\n"}}
		ran, err := ensureRan(f, c.keep)
		usb := false
		for _, call := range f.calls {
			usb = usb || call == "usb"
		}
		if err != nil || ran != c.wantUSB || usb != c.wantUSB {
			t.Errorf("%s: ran %v usb %v err %v (calls %q)", c.name, ran, usb, err, f.calls)
		}
	}
}

// A persist property that cannot be read leaves the port unknown, never
// closed: an error, and no `adb usb` (as with the service property).
func TestEnsureNoTCPPortPersistUnreadable(t *testing.T) {
	for _, service := range []string{"", "0", "5555"} {
		f := &fakeADB{answers: map[string]string{serviceProp: service + "\n", "usb": ""}, stderr: map[string]string{persistProp: "error: device offline\x1b[2J"}}
		ran, err := ensureRan(f, "")
		if err == nil || ran || strings.ContainsRune(err.Error(), 0x1b) {
			t.Errorf("service %q: ran %v err %v", service, ran, err)
		}
		for _, c := range f.calls {
			if c == "usb" {
				t.Errorf("service %q: adb usb ran with persist unknown", service)
			}
		}
	}
}
```

Edit en `host/internal/remote/remote_test.go`:

**Reemplazar:**
```go
const getprop = "shell getprop service.adb.tcp.port"
```

**por:**
```go
const (
	getprop    = "shell getprop service.adb.tcp.port"
	getpersist = "shell getprop persist.adb.tcp.port"
)
```

Edit en `host/internal/remote/remote_test.go`:

**Reemplazar:**
```go
	portFails bool
	tcpipFail bool
```

**por:**
```go
	portFails bool
	persist   string // answer of getprop persist.adb.tcp.port
	persFails bool
	tcpipFail bool
```

Edit en `host/internal/remote/remote_test.go`:

**Reemplazar:**
```go
		io.WriteString(stdout, e.port+"\r\n")
	case "tcpip 5555":
```

**por:**
```go
		io.WriteString(stdout, e.port+"\r\n")
	case getpersist:
		if e.persFails {
			io.WriteString(stderr, "error: device offline")
			return 1, nil
		}
		io.WriteString(stdout, e.persist+"\r\n")
	case "tcpip 5555":
```

Añadir al final de `host/internal/remote/remote_test.go`:

```go

// VB-41: AOSP adbd takes persist.adb.tcp.port at its start when
// service.adb.tcp.port is empty (after a reboot). "off" with the mode off
// reads both: a persist port with the service property empty is open and is
// closed; with the service property at 0 `adb usb` already ran since the boot
// and is not run again (no loop). A persist property that cannot be read is
// not known to be closed: it is closed all the same, as an unreadable port.
func TestDisableWithoutTheModeReadsPersistToo(t *testing.T) {
	for _, c := range []struct {
		name, port, persist string
		persFails, wantUSB  bool
	}{
		{"persist after a reboot", "", "5555", false, true},
		{"another persist port", "", "7777", false, true},
		{"not a number", "", "abc", false, true},
		{"huge", "", "99999999999999999999999", false, true},
		{"persist unreadable", "", "", true, true},
		{"persist 0", "", "0", false, false},
		{"nothing", "", "", false, false},
		{"service at 0", "0", "5555", false, false},
		{"both set", "5555", "5555", false, true},
	} {
		e := newEnv()
		e.port, e.persist, e.persFails = c.port, c.persist, c.persFails
		if _, err := e.controller().Disable(context.Background()); err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if e.ran("usb") != c.wantUSB {
			t.Errorf("%s: adb usb ran %v, want %v (%q)", c.name, e.ran("usb"), c.wantUSB, e.events)
		}
	}
}

// After a reboot with persist.adb.tcp.port=5555 and a live expiry, adbd
// listens on 5555 again: the mode is still on, not a stale expiry to clear
// (that would leave 5555 open with the host saying "off").
func TestReconcileKeepsTheModeOnAPersistPort(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(time.Hour)
	e.persist = "5555"
	s, err := e.controller().Reconcile(context.Background())
	if err != nil || !s.Enabled || !e.until.Equal(t0.Add(time.Hour)) || e.ran("usb") {
		t.Fatalf("%+v %v until %v (%q)", s, err, e.until, e.events)
	}
}

// Reconcile with persist.adb.tcp.port unreadable changes nothing, as with the
// service property unreadable.
func TestReconcilePersistUnreadableChangesNothing(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(time.Hour)
	e.persFails = true
	s, err := e.controller().Reconcile(context.Background())
	if err == nil || !s.Enabled || !e.until.Equal(t0.Add(time.Hour)) || e.ran("usb") {
		t.Fatalf("%+v %v until %v (%q)", s, err, e.until, e.events)
	}
}

// tcpip failing for any reason but "no phone": the expiry is cleared only when
// the port reads closed, persist.adb.tcp.port included.
func TestEnableTcpipFailsWithAPersistPortKeepsUntil(t *testing.T) {
	for name, set := range map[string]func(*env){
		"persist 5555":       func(e *env) { e.persist = "5555" },
		"persist unreadable": func(e *env) { e.persFails = true },
	} {
		e := newEnv()
		e.tcpipFail = true
		set(e)
		s, err := e.controller().Enable(context.Background())
		if err == nil || !e.until.Equal(t0.Add(Lifetime)) || !s.Enabled {
			t.Errorf("%s: until %v state %+v err %v", name, e.until, s, err)
		}
	}
}

// After `adb usb` closed the port and clearing the expiry failed, the retry
// skips `adb usb` only if the port reads closed: a persist port with the
// service property empty is open.
func TestRetryAfterAFailedSaveSeesAPersistPort(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(time.Hour)
	e.now = t0.Add(2 * time.Hour)
	e.saveFail = true
	ctl := e.controller()
	if _, _, err := ctl.Expire(context.Background()); err == nil {
		t.Fatal("the save did not fail")
	}
	e.saveFail, e.events = false, nil
	e.persist = "5555"
	if _, changed, err := ctl.Expire(context.Background()); err != nil || !changed || !e.ran("usb") {
		t.Fatalf("changed %v err %v (%q)", changed, err, e.events)
	}
}
```

- [ ] **Step 2: comprobar el RED.**

Run: `cd host && go vet ./internal/device/ ./internal/remote/ && go test -count=1 -timeout 5m ./internal/device/ ./internal/remote/`
Expected: `vet` limpio; `FAIL` en los dos paquetes, sin errores de compilación ni `panic`, y solo por la lectura de persist que hoy no existe:
- `device`: `--- FAIL: TestEnsureNoTCPPortReadsPersist` (5 casos: `persist after a reboot`, `another persist port`, `not a number`, `huge`, `negative`, todos `ran false usb false err <nil> (calls ["shell getprop service.adb.tcp.port"])`) y `--- FAIL: TestEnsureNoTCPPortPersistUnreadable` (`service "": ran false err <nil>`, ídem `"0"`, y `service "5555": ran true err <nil>` + `adb usb ran with persist unknown`).
- `remote`: `TestDisableWithoutTheModeReadsPersistToo` (5 casos con `adb usb ran false, want true`), `TestReconcileKeepsTheModeOnAPersistPort` y `TestReconcilePersistUnreadableChangesNothing` (`... <nil> until 0001-01-01 ... ["adb shell getprop service.adb.tcp.port" "save zero"]`), `TestEnableTcpipFailsWithAPersistPortKeepsUntil` (los dos casos, `until 0001-01-01`), `TestRetryAfterAFailedSaveSeesAPersistPort` (`["adb shell getprop service.adb.tcp.port" "save zero"]`).

- [ ] **Step 3: `device.ReadTCP` y `EnsureNoTCPPort` con persist.**

Edit en `host/internal/device/device.go`:

**Reemplazar:**
```go
// TCPPort returns the TCP port adbd listens on, or "" when it listens on
// none (service.adb.tcp.port empty or 0). It only reads.
func TCPPort(ctx context.Context, a ADB) (string, error) {
	port, err := shell(ctx, a, "getprop service.adb.tcp.port")
	if err != nil || port == "0" {
		return "", err
	}
	return port, nil
}

// EnsureNoTCPPort applies the host invariant (spec §3.3): if adbd listens on
// a TCP port (service.adb.tcp.port neither empty nor 0) and remote mode is
// not on, `adb usb` closes it. adbd restarts, so the caller must wait for
// the phone again when this returns true. keep is a port that must stay open
// (remote mode's 5555); another open port is still closed.
func EnsureNoTCPPort(ctx context.Context, a ADB, keep string) (bool, error) {
	port, err := TCPPort(ctx, a)
	if err != nil || port == "" || keep != "" && port == keep {
		return false, err
	}
	var out, errb bytes.Buffer
	code, err := a.Run(ctx, "", []string{"usb"}, nil, &out, &errb)
	if err != nil {
		return false, err
	}
	if code != 0 {
		return false, fmt.Errorf("adb usb: %s", classify.Printable(strings.TrimSpace(errb.String()+" "+out.String())))
	}
	return true, nil
}
```

**por:**
```go
// TCP is adbd's TCP port as its properties read (spec §3.3, VB-41). Both
// fields come from the phone: trimmed, cut to maxPortRunes and printable, so
// they can be shown as they are.
type TCP struct {
	// Now is the port adbd listens on, "" when none: service.adb.tcp.port,
	// or persist.adb.tcp.port when service.adb.tcp.port is empty (AOSP adbd
	// takes persist at its start only then, as after a reboot). Any value but
	// empty or 0 counts as a port, even one that is not a number: never a
	// false "closed".
	Now string
	// Persist is persist.adb.tcp.port when it names a port ("" when empty or
	// 0). `adb usb` does not clear it and it survives a reboot (measured,
	// VB-41): adbd takes it again at its next start with service.adb.tcp.port
	// empty. ADBuddy never sets it and cannot clear it: the shell user may
	// not (setprop exits 1) and the host never runs su.
	Persist string
}

// maxPortRunes bounds a port property as the host keeps and shows it.
const maxPortRunes = 16

// ReadTCPWait bounds one ReadTCP where a caller has a longer budget for more
// than the read (the remote controller and `remote off` before `adb usb`, the
// watcher of the connection): a hung getprop must not eat it. A variable so
// a test can shorten it; read it before starting goroutines.
var ReadTCPWait = 10 * time.Second

// portValue is a property read as a port: "" when empty or 0; otherwise the
// value, cut to maxPortRunes and printable.
func portValue(v string) string {
	if v == "" || v == "0" {
		return ""
	}
	if utf8.RuneCountInString(v) > maxPortRunes {
		v = string([]rune(v)[:maxPortRunes]) + "…"
	}
	return classify.Printable(v)
}

// ReadTCP reads adbd's TCP port: service.adb.tcp.port, then
// persist.adb.tcp.port. It only reads. If either read fails the error says
// so: the port is unknown then, never closed.
func ReadTCP(ctx context.Context, a ADB) (TCP, error) {
	service, err := shell(ctx, a, "getprop service.adb.tcp.port")
	if err != nil {
		return TCP{}, err
	}
	persist, err := shell(ctx, a, "getprop persist.adb.tcp.port")
	if err != nil {
		return TCP{}, err
	}
	t := TCP{Now: portValue(service), Persist: portValue(persist)}
	if service == "" {
		t.Now = t.Persist
	}
	return t, nil
}

// EnsureNoTCPPort applies the host invariant (spec §3.3): if adbd listens on
// a TCP port (TCP.Now) other than keep (remote mode's 5555, "" when the mode
// is off), `adb usb` closes it: adbd sets service.adb.tcp.port to 0 and
// restarts, and with it at 0 ignores persist.adb.tcp.port until the next
// reboot. It returns what it read and whether it ran `adb usb`: adbd
// restarts, so the caller must wait for the phone and read the port again. A
// persist port with service.adb.tcp.port at 0 is nothing to close now
// (`adb usb` would only restart adbd again, at every start): the caller warns
// about it. A read that fails is an error, and nothing runs.
func EnsureNoTCPPort(ctx context.Context, a ADB, keep string) (TCP, bool, error) {
	t, err := ReadTCP(ctx, a)
	if err != nil || t.Now == "" || keep != "" && t.Now == keep {
		return t, false, err
	}
	var out, errb bytes.Buffer
	code, err := a.Run(ctx, "", []string{"usb"}, nil, &out, &errb)
	if err != nil {
		return t, false, err
	}
	if code != 0 {
		return t, false, fmt.Errorf("adb usb: %s", classify.Printable(strings.TrimSpace(errb.String()+" "+out.String())))
	}
	return t, true, nil
}
```

(`utf8` y `time` ya están importados en `device.go`.)

Edit en `host/internal/device/device_test.go`:

**Reemplazar:**
```go
	return EnsureNoTCPPort(context.Background(), f, keep)
```

**por:**
```go
	_, ran, err := EnsureNoTCPPort(context.Background(), f, keep)
	return ran, err
```

Edit en `host/internal/device/device_test.go`:

**Reemplazar:**
```go
// TCPPort only reads the property: it never runs `adb usb`.
func TestTCPPort(t *testing.T) {
	for raw, want := range map[string]string{"": "", "0": "", "5555": "5555", " 5555\r": "5555"} {
		f := &fakeADB{answers: map[string]string{"shell getprop service.adb.tcp.port": raw + "\n", "usb": ""}}
		port, err := TCPPort(context.Background(), f)
		if err != nil || port != want {
			t.Errorf("%q: port %q err %v", raw, port, err)
		}
		if strings.Join(f.calls, "|") != "shell getprop service.adb.tcp.port" {
			t.Errorf("%q: calls %q", raw, f.calls)
		}
	}
	if _, err := TCPPort(context.Background(), &fakeADB{}); err == nil {
		t.Fatal("an unreadable property must be an error")
	}
}
```

**por:**
```go
// ReadTCP only reads the two properties: it never runs `adb usb`. Now is the
// service property, or persist's when the service property is empty; any
// value but empty or 0 is a port, kept short and printable.
func TestReadTCP(t *testing.T) {
	for _, c := range []struct {
		service, persist string
		want             TCP
	}{
		{"", "", TCP{}},
		{"0", "", TCP{}},
		{" 5555\r", "", TCP{Now: "5555"}},
		{"", "5555", TCP{Now: "5555", Persist: "5555"}},
		{"0", "5555", TCP{Persist: "5555"}},
		{"5555", "7777", TCP{Now: "5555", Persist: "7777"}},
		{"", "0", TCP{}},
		{"", "abc", TCP{Now: "abc", Persist: "abc"}},
		{"", "-1", TCP{Now: "-1", Persist: "-1"}},
		{"0", "99999999999999999999999", TCP{Persist: "9999999999999999…"}},
		{"0", "55\x1b[2J55", TCP{Persist: `55\u001b[2J55`}},
	} {
		f := &fakeADB{answers: map[string]string{serviceProp: c.service + "\n", persistProp: c.persist + "\r\n", "usb": ""}}
		got, err := ReadTCP(context.Background(), f)
		if err != nil || got != c.want {
			t.Errorf("service %q persist %q: %+v err %v, want %+v", c.service, c.persist, got, err, c.want)
		}
		if strings.Join(f.calls, "|") != serviceProp+"|"+persistProp {
			t.Errorf("service %q persist %q: calls %q", c.service, c.persist, f.calls)
		}
	}
	// Either property unreadable: an error, never a closed port.
	if got, err := ReadTCP(context.Background(), &fakeADB{}); err == nil || got != (TCP{}) {
		t.Fatalf("service unreadable: %+v %v", got, err)
	}
	f := &fakeADB{answers: map[string]string{serviceProp: "0\n"}, stderr: map[string]string{persistProp: "error: closed"}}
	if got, err := ReadTCP(context.Background(), f); err == nil || got != (TCP{}) {
		t.Fatalf("persist unreadable: %+v %v", got, err)
	}
}

// EnsureNoTCPPort returns what it read, before `adb usb`: the caller warns
// about a persist port.
func TestEnsureNoTCPPortReturnsTheRead(t *testing.T) {
	f := &fakeADB{answers: map[string]string{serviceProp: "\n", persistProp: "5555\n", "usb": ""}}
	got, ran, err := EnsureNoTCPPort(context.Background(), f, "")
	if err != nil || !ran || got != (TCP{Now: "5555", Persist: "5555"}) {
		t.Fatalf("%+v %v %v", got, ran, err)
	}
	f = &fakeADB{answers: map[string]string{serviceProp: "0\n", persistProp: "5555\n", "usb": ""}}
	got, ran, err = EnsureNoTCPPort(context.Background(), f, "")
	if err != nil || ran || got != (TCP{Persist: "5555"}) {
		t.Fatalf("service at 0: %+v %v %v", got, ran, err)
	}
}
```

Run: `cd host && go vet ./internal/device/ && go test -count=1 -timeout 5m ./internal/device/`
Expected: `ok`. (`go build ./...` falla todavía en `internal/cli` y `internal/emutest`, que usan `TCPPort` y la firma vieja: los arreglan los pasos 5 y 6.)

- [ ] **Step 4: el controlador lee con `device.ReadTCP` y devuelve persist en su estado.**

Edit en `host/internal/remote/remote.go`:

**Reemplazar:**
```go
	"github.com/hybriqtech/adbuddy/host/internal/classify"
)
```

**por:**
```go
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/device"
)
```

Edit en `host/internal/remote/remote.go`:

**Reemplazar:**
```go
	Address   string // ip:port while enabled
	ExpiresAt time.Time
}
```

**por:**
```go
	Address   string // ip:port while enabled
	ExpiresAt time.Time
	// Persist is persist.adb.tcp.port as the controller last read it (or was
	// told with Note), when it names a port (device.TCP.Persist): `adb usb`
	// does not clear it, and adbd listens on it again when the phone restarts
	// (VB-41). ADBuddy never sets it and cannot clear it without root. It is
	// the last read, not a live value: a persist set or emptied since shows
	// at the next read.
	Persist string
	// PersistUnknown says the controller's last read of the port failed:
	// whether a port opens again when the phone restarts is not known (never
	// a false "closed").
	PersistUnknown bool
}
```

Edit en `host/internal/remote/remote.go`:

**Reemplazar:**
```go
	closedFor time.Time
}
```

**por:**
```go
	closedFor time.Time
	// readWait bounds each read of the port (device.ReadTCPWait as New read
	// it, before any goroutine).
	readWait time.Duration
	// pmu guards persist and persistUnknown, what the last read of the port
	// said (readTCP, Note). State takes only pmu: the snapshot and `status`
	// call it while a change may hold mu.
	pmu            sync.Mutex
	persist        string
	persistUnknown bool
}
```

Edit en `host/internal/remote/remote.go`:

**Reemplazar:**
```go
	return &Controller{adb: a, st: s, now: now}
```

**por:**
```go
	return &Controller{adb: a, st: s, now: now, readWait: device.ReadTCPWait}
```

Edit en `host/internal/remote/remote.go`:

**Reemplazar:**
```go
func (c *Controller) State() State {
	until := c.until()
	if until.IsZero() {
		return State{}
	}
	s := State{Enabled: true, Closing: !until.After(c.now()), ExpiresAt: until}
	if ip := c.st.TailscaleIP(); ip != "" {
		s.Address = net.JoinHostPort(ip, Port)
	}
	return s
}
```

**por:**
```go
func (c *Controller) State() State {
	var s State
	if until := c.until(); !until.IsZero() {
		s = State{Enabled: true, Closing: !until.After(c.now()), ExpiresAt: until}
		if ip := c.st.TailscaleIP(); ip != "" {
			s.Address = net.JoinHostPort(ip, Port)
		}
	}
	c.pmu.Lock()
	s.Persist, s.PersistUnknown = c.persist, c.persistUnknown
	c.pmu.Unlock()
	return s
}

// readTCP reads adbd's TCP port (device.ReadTCP: service.adb.tcp.port, or
// persist.adb.tcp.port when it is empty) and keeps what it said of
// persist.adb.tcp.port for State; a read that fails makes it unknown. The
// read has its own deadline (readWait): a hung getprop must not eat the
// budget of the `adb usb` that follows.
func (c *Controller) readTCP(ctx context.Context) (device.TCP, error) {
	rctx, cancel := context.WithTimeout(ctx, c.readWait)
	defer cancel()
	t, err := device.ReadTCP(rctx, c.adb)
	c.pmu.Lock()
	c.persist, c.persistUnknown = t.Persist, err != nil
	c.pmu.Unlock()
	return t, err
}

// Note keeps a read of the port made elsewhere (serve's invariant at its
// start, the watcher when the phone comes back) for State, as if the
// controller had made it: `adbuddy remote status` says what
// persist.adb.tcp.port holds from serve's start. It takes only pmu.
func (c *Controller) Note(t device.TCP) {
	c.pmu.Lock()
	c.persist, c.persistUnknown = t.Persist, false
	c.pmu.Unlock()
}
```

Edit en `host/internal/remote/remote.go`:

**Reemplazar:**
```go
		// is seen NOT to be open. If it cannot be read, the expiry stays (a
		// false "on" is better than a false "off" with the port open);
		// Reconcile clears it at the next start if the port turns out closed.
		if port, perr := c.adbCmd(ctx, "shell", "getprop service.adb.tcp.port"); perr == nil && port != Port {
```

**por:**
```go
		// is seen NOT to be open (persist.adb.tcp.port included). If it cannot
		// be read, the expiry stays (a false "on" is better than a false "off"
		// with the port open); Reconcile clears it at the next start if the
		// port turns out closed.
		if t, perr := c.readTCP(ctx); perr == nil && t.Now != Port {
```

Edit en `host/internal/remote/remote.go`:

**Reemplazar:**
```go
// adb fails the mode stays on (and says so) and is tried again. With no
// expiry saved (off as far as the host knows) it first reads the port:
// `adb usb` would restart adbd, and drop the phone's connection, for nothing.
// A port that reads open (5555 or any other), or cannot be read, is closed all the same.
func (c *Controller) Disable(ctx context.Context) (State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st.Until().IsZero() {
		if port, err := c.adbCmd(ctx, "shell", "getprop service.adb.tcp.port"); err == nil && portClosed(port) {
			return State{}, nil
		}
	}
	return c.disable(ctx)
}

// portClosed says whether service.adb.tcp.port reads as no network port (empty
// or 0, the criterion of device.EnsureNoTCPPort, spec 3.3): any other value,
// 5555 or not, is open and `adb usb` closes it.
func portClosed(port string) bool { return port == "" || port == "0" }

// disable runs `adb usb` and clears the expiry. When clearing failed after
// `adb usb` closed the port, the next call for the same expiry skips `adb usb`
// (closedFor: each one restarts adbd and the phone's connection) only if the
// port reads as closed: someone may have reopened 5555 meanwhile, and an
// unreadable port is not known to be closed.
func (c *Controller) disable(ctx context.Context) (State, error) {
	until := c.st.Until()
	if !c.closedFor.IsZero() && until.Equal(c.closedFor) {
		if port, err := c.adbCmd(ctx, "shell", "getprop service.adb.tcp.port"); err != nil || !portClosed(port) {
			c.closedFor = time.Time{}
		}
	}
```

**por:**
```go
// adb fails the mode stays on (and says so) and is tried again. It reads the
// port first (device.TCP), so the state it returns says what
// persist.adb.tcp.port holds. With no expiry saved (off as far as the host
// knows), a port that reads closed is left alone: `adb usb` would restart
// adbd, and drop the phone's connection, for nothing. A port that reads open
// (5555, any other, or persist's after a restart), or cannot be read, is
// closed all the same.
func (c *Controller) Disable(ctx context.Context) (State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, err := c.readTCP(ctx)
	if c.st.Until().IsZero() && err == nil && t.Now == "" {
		return c.State(), nil
	}
	return c.disable(ctx, t, err)
}

// disable runs `adb usb` and clears the expiry. t and rerr are the read of
// the port just before (readTCP). When clearing failed after `adb usb` closed
// the port, the next call for the same expiry skips `adb usb` (closedFor: each
// one restarts adbd and the phone's connection) only if that read says
// closed: someone may have reopened the port meanwhile, and an unreadable port
// is not known to be closed. The state it returns keeps what the read said of
// persist.adb.tcp.port: `adb usb` does not change it (measured, VB-41), and
// right after it adbd restarts and cannot be read.
func (c *Controller) disable(ctx context.Context, t device.TCP, rerr error) (State, error) {
	until := c.st.Until()
	if !c.closedFor.IsZero() && until.Equal(c.closedFor) && (rerr != nil || t.Now != "") {
		c.closedFor = time.Time{}
	}
```

Edit en `host/internal/remote/remote.go`:

**Reemplazar:**
```go
	c.closedFor = time.Time{}
	return State{}, nil
}
```

**por:**
```go
	c.closedFor = time.Time{}
	return c.State(), nil
}
```

Edit en `host/internal/remote/remote.go`:

**Reemplazar:**
```go
	s, err := c.disable(ctx)
	return s, err == nil, err
```

**por:**
```go
	t, rerr := c.readTCP(ctx)
	s, err := c.disable(ctx, t, rerr)
	return s, err == nil, err
```

Edit en `host/internal/remote/remote.go`:

**Reemplazar:**
```go
// with the expiry passed: closed now. If the port cannot be read, nothing
// changes (the next start looks again).
func (c *Controller) Reconcile(ctx context.Context) (State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	until := c.until()
	if until.IsZero() {
		return State{}, nil
	}
	port, err := c.adbCmd(ctx, "shell", "getprop service.adb.tcp.port")
	if err != nil {
		return c.State(), err
	}
	switch {
	case port != Port:
		if err := c.st.SetUntil(time.Time{}); err != nil {
			return c.State(), err
		}
		return State{}, nil
	case until.After(c.now()):
		return c.State(), nil
	}
	return c.disable(ctx)
}
```

**por:**
```go
// with the expiry passed: closed now. The port is device.TCP.Now: after a
// restart with persist.adb.tcp.port=5555, adbd listens on 5555 again and the
// mode is still on. If the port cannot be read, nothing changes (the next
// start looks again).
func (c *Controller) Reconcile(ctx context.Context) (State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	until := c.until()
	if until.IsZero() {
		return State{}, nil
	}
	t, err := c.readTCP(ctx)
	if err != nil {
		return c.State(), err
	}
	switch {
	case t.Now != Port:
		if err := c.st.SetUntil(time.Time{}); err != nil {
			return c.State(), err
		}
		return c.State(), nil
	case until.After(c.now()):
		return c.State(), nil
	}
	return c.disable(ctx, t, nil)
}
```

Las secuencias exactas de eventos de los tests de hoy cambian: cada decisión lee ahora las dos propiedades (y `Disable` con el modo encendido y `Expire` leen antes de `adb usb`, ruling 5). Un `getprop` de service que falla corta la lectura: esos casos siguen con un solo `getprop`.

Edit en `host/internal/remote/remote_test.go`:

**Reemplazar:**
```go
	getpersist = "shell getprop persist.adb.tcp.port"
)
```

**por:**
```go
	getpersist = "shell getprop persist.adb.tcp.port"
	// readPort is the read of the port as the events log it: both properties.
	readPort = "adb " + getprop + "|adb " + getpersist
)
```

Edit en `host/internal/remote/remote_test.go` (`replace_all: true`, 4 apariciones: `TestDisableRunsUsbThenClears`, `TestExpire`, `TestUntilBeyondLifetimeCountsAsExpired`, `TestEnableForgetsTheExpiryItsPortWasClosedFor`):

**Reemplazar:**
```go
	if got := strings.Join(e.events, "|"); got != "adb usb|save zero" {
```

**por:**
```go
	if got := strings.Join(e.events, "|"); got != readPort+"|adb usb|save zero" {
```

Edit en `host/internal/remote/remote_test.go`:

**Reemplazar:**
```go
		if got := strings.Join(e.events, "|"); got != "adb "+getprop+"|save zero" {
```

**por:**
```go
		if got := strings.Join(e.events, "|"); got != readPort+"|save zero" {
```

Edit en `host/internal/remote/remote_test.go`:

**Reemplazar:**
```go
	if got := strings.Join(e.events, "|"); got != "adb "+getprop+"|adb usb|save zero" {
```

**por:**
```go
	if got := strings.Join(e.events, "|"); got != readPort+"|adb usb|save zero" {
```

Edit en `host/internal/remote/remote_test.go`:

**Reemplazar:**
```go
		{"closed", "", false, "adb " + getprop},
		{"zero", "0", false, "adb " + getprop},
		{"another port", "37123", false, "adb " + getprop + "|adb usb|save zero"},
		{"open", "5555", false, "adb " + getprop + "|adb usb|save zero"},
		{"unreadable", "", true, "adb " + getprop + "|adb usb|save zero"},
```

**por:**
```go
		{"closed", "", false, readPort},
		{"zero", "0", false, readPort},
		{"another port", "37123", false, readPort + "|adb usb|save zero"},
		{"open", "5555", false, readPort + "|adb usb|save zero"},
		{"unreadable", "", true, "adb " + getprop + "|adb usb|save zero"},
```

Edit en `host/internal/remote/remote_test.go`:

**Reemplazar:**
```go
		s, err := e.controller().Disable(context.Background())
		if err != nil || s != (State{}) {
			t.Fatalf("%s: state %+v err %v", c.name, s, err)
		}
```

**por:**
```go
		s, err := e.controller().Disable(context.Background())
		// Unreadable: off, but whether a port comes back after a restart is
		// not known (VB-41).
		if err != nil || s != (State{PersistUnknown: c.portFails}) {
			t.Fatalf("%s: state %+v err %v", c.name, s, err)
		}
```

Edit en `host/internal/remote/remote_test.go`:

**Reemplazar:**
```go
	if got := strings.Join(e.events, "|"); got != "adb usb|adb "+getprop {
```

**por:**
```go
	if got := strings.Join(e.events, "|"); got != readPort+"|adb usb|"+readPort {
```

Edit en `host/internal/remote/remote_test.go`:

**Reemplazar:**
```go
	if got := strings.Join(e.events, "|"); got != "adb usb|adb "+getprop+"|adb "+getprop+"|save zero" {
```

**por:**
```go
	if got := strings.Join(e.events, "|"); got != readPort+"|adb usb|"+readPort+"|"+readPort+"|save zero" {
```

Edit en `host/internal/remote/remote_test.go`:

**Reemplazar:**
```go
	if got := strings.Join(e.events, "|"); got != "adb usb|save 12h0m0s|adb tcpip 5555|adb usb|save zero" {
```

**por:**
```go
	if got := strings.Join(e.events, "|"); got != readPort+"|adb usb|save 12h0m0s|adb tcpip 5555|"+readPort+"|adb usb|save zero" {
```

Edit en `host/internal/remote/remote_test.go`:

**Reemplazar:**
```go
		{"closed", "", false, "adb " + getprop + "|save zero"},
		{"zero", "0", false, "adb " + getprop + "|save zero"},
		{"another port", "37123", false, "adb " + getprop + "|adb usb|save zero"},
		{"reopened", "5555", false, "adb " + getprop + "|adb usb|save zero"},
```

**por:**
```go
		{"closed", "", false, readPort + "|save zero"},
		{"zero", "0", false, readPort + "|save zero"},
		{"another port", "37123", false, readPort + "|adb usb|save zero"},
		{"reopened", "5555", false, readPort + "|adb usb|save zero"},
```

Edit en `host/internal/remote/remote_test.go`:

**Reemplazar:**
```go
		if s, changed, err := ctl.Expire(context.Background()); err != nil || !changed || s != (State{}) {
			t.Fatalf("%s: %+v %v %v", c.name, s, changed, err)
		}
```

**por:**
```go
		if s, changed, err := ctl.Expire(context.Background()); err != nil || !changed || s != (State{PersistUnknown: c.portFails}) {
			t.Fatalf("%s: %+v %v %v", c.name, s, changed, err)
		}
```

Añadir al final de `host/internal/remote/remote_test.go`:

```go

// Every "off" returns what it read of persist.adb.tcp.port, and State keeps
// it: off is never a plain State{} while persist names a port, or while it
// could not be read.
func TestOffSaysWhatPersistHolds(t *testing.T) {
	type off func(*env, *Controller) (State, error)
	disable := func(_ *env, c *Controller) (State, error) { return c.Disable(context.Background()) }
	expire := func(e *env, c *Controller) (State, error) {
		e.now = t0.Add(2 * time.Hour)
		s, _, err := c.Expire(context.Background())
		return s, err
	}
	reconcile := func(e *env, c *Controller) (State, error) {
		e.now, e.port = t0.Add(2*time.Hour), "5555"
		return c.Reconcile(context.Background())
	}
	for _, c := range []struct {
		name  string
		until bool // the mode is on (an expiry saved)
		port  string
		do    off
		keeps bool // an unreadable port changes nothing (and is an error)
	}{
		{"Disable, mode off, service at 0", false, "0", disable, false},
		{"Disable, mode off, service empty", false, "", disable, false},
		{"Disable, mode on", true, "5555", disable, false},
		{"Expire", true, "5555", expire, false},
		{"Reconcile past the expiry", true, "5555", reconcile, true},
	} {
		for _, p := range []struct {
			persist   string
			persFails bool
			want      State
		}{
			{"5555", false, State{Persist: "5555"}},
			{"7777", false, State{Persist: "7777"}},
			{"", false, State{}},
			{"0", false, State{}},
			{"", true, State{PersistUnknown: true}},
		} {
			e := newEnv()
			if c.until {
				e.until = t0.Add(time.Hour)
			}
			e.port, e.persist, e.persFails = c.port, p.persist, p.persFails
			ctl := e.controller()
			s, err := c.do(e, ctl)
			if c.keeps && p.persFails {
				if err == nil || !s.Enabled || !s.PersistUnknown || e.ran("usb") {
					t.Errorf("%s, persist unreadable: %+v %v (%q)", c.name, s, err, e.events)
				}
				continue
			}
			if err != nil || s != p.want || ctl.State() != p.want {
				t.Errorf("%s, persist %q (fails %v): returned %+v, State %+v, err %v; want %+v (%q)", c.name, p.persist, p.persFails, s, ctl.State(), err, p.want, e.events)
			}
		}
	}
}

// No restart loop: with persist set, `adb usb` runs once (the service property
// was empty, as after a reboot); it leaves the service property at 0, and the
// next "off" only reads and says so again.
func TestPersistDoesNotRestartAdbdAgain(t *testing.T) {
	e := newEnv()
	e.persist = "5555"
	ctl := e.controller()
	for i := 0; i < 3; i++ {
		s, err := ctl.Disable(context.Background())
		if err != nil || s != (State{Persist: "5555"}) {
			t.Fatalf("off %d: %+v %v", i+1, s, err)
		}
		e.port = "0" // what adb usb leaves
	}
	if got := strings.Join(e.events, "|"); got != readPort+"|adb usb|save zero|"+readPort+"|"+readPort {
		t.Fatalf("events %q: want one adb usb", got)
	}
}

// A later read replaces the earlier one: persist emptied (with root) is no
// longer reported, and a read that fails turns it unknown.
func TestStateKeepsTheLastReadOfPersist(t *testing.T) {
	e := newEnv()
	e.port, e.persist = "0", "5555"
	ctl := e.controller()
	if s, _ := ctl.Disable(context.Background()); s.Persist != "5555" {
		t.Fatalf("%+v", s)
	}
	e.persist = ""
	if s, _ := ctl.Disable(context.Background()); s != (State{}) || ctl.State() != (State{}) {
		t.Fatalf("emptied: %+v %+v", s, ctl.State())
	}
	e.persFails = true
	if s, _ := ctl.Disable(context.Background()); s != (State{PersistUnknown: true}) {
		t.Fatalf("unreadable: %+v", s)
	}
}

// `adb usb` failing with a persist port: an error, the mode as it was (on
// stays on, with its expiry), and the persist port still in the state.
func TestUsbFailsWithAPersistPort(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(time.Hour)
	e.persist, e.usbFail = "5555", true
	s, err := e.controller().Disable(context.Background())
	if err == nil || !s.Enabled || s.Persist != "5555" || !e.until.Equal(t0.Add(time.Hour)) {
		t.Fatalf("mode on: %+v %v until %v", s, err, e.until)
	}
	e = newEnv()
	e.persist, e.usbFail = "5555", true
	if s, err := e.controller().Disable(context.Background()); err == nil || s != (State{Persist: "5555"}) || !e.ran("usb") {
		t.Fatalf("mode off: %+v %v (%q)", s, err, e.events)
	}
}
```

Para `TestReadTCPHasItsOwnWait`, el adb falso de `remote` aprende a colgarse en la lectura de persist hasta que acabe su contexto (revisión M1), y para `TestNoteFeedsState` el test importa `device` (revisión M2):

Edit en `host/internal/remote/remote_test.go`:

**Reemplazar:**
```go
func (e *env) Run(_ context.Context, _ string, args []string, _ io.Reader, stdout, stderr io.Writer) (int, error) {
```

**por:**
```go
func (e *env) Run(ctx context.Context, _ string, args []string, _ io.Reader, stdout, stderr io.Writer) (int, error) {
```

Edit en `host/internal/remote/remote_test.go`:

**Reemplazar:**
```go
	case getpersist:
		if e.persFails {
```

**por:**
```go
	case getpersist:
		if e.persHang {
			<-ctx.Done()
			return -1, ctx.Err()
		}
		if e.persFails {
```

Edit en `host/internal/remote/remote_test.go`:

**Reemplazar:**
```go
	persFails bool
	tcpipFail bool
```

**por:**
```go
	persFails bool
	persHang  bool // getprop persist.adb.tcp.port waits for its context to end
	tcpipFail bool
```

Añadir al final de `host/internal/remote/remote_test.go`:

```go

// The read of the port has its own deadline (device.ReadTCPWait): a hung
// getprop of persist ends as "unknown" and `adb usb` still runs within the
// caller's budget.
func TestReadTCPHasItsOwnWait(t *testing.T) {
	old := device.ReadTCPWait
	device.ReadTCPWait = 20 * time.Millisecond
	defer func() { device.ReadTCPWait = old }()
	e := newEnv()
	e.until = t0.Add(time.Hour)
	e.persHang = true
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	s, err := e.controller().Disable(ctx)
	if err != nil || s != (State{PersistUnknown: true}) || !e.ran("usb") || time.Since(start) > 5*time.Second {
		t.Fatalf("%+v %v after %s (%q)", s, err, time.Since(start), e.events)
	}
}

// Note: a read made elsewhere (serve's invariant) feeds State from the start;
// a later read of the controller replaces it.
func TestNoteFeedsState(t *testing.T) {
	e := newEnv()
	ctl := e.controller()
	ctl.Note(device.TCP{Persist: "5555"})
	if s := ctl.State(); s != (State{Persist: "5555"}) {
		t.Fatalf("%+v", s)
	}
	e.port = "0"
	if s, err := ctl.Disable(context.Background()); err != nil || s != (State{}) || ctl.State() != (State{}) {
		t.Fatalf("after a read: %+v %v", s, err)
	}
	e.events = nil
	ctl.Note(device.TCP{})
	if len(e.events) != 0 {
		t.Fatalf("Note ran adb: %q", e.events)
	}
}
```

Edit en `host/internal/remote/remote_test.go`:

**Reemplazar:**
```go
	"github.com/hybriqtech/adbuddy/host/internal/adb"
)
```

**por:**
```go
	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/device"
)
```

Run: `cd host && gofmt -l ./internal/remote ./internal/device && go vet ./internal/remote/ ./internal/device/ && go test -count=1 -timeout 5m ./internal/remote/ ./internal/device/`
Expected: `gofmt` sin salida, `vet` limpio, `ok` en los dos. Mutaciones (comprobadas en la réplica, restaurar después): sin `if service == "" { t.Now = t.Persist }` en `ReadTCP` fallan 8 tests entre `device` y `remote`; con `c.persist = ""` fijo en `readTCP` fallan `TestOffSaysWhatPersistHolds`, `TestPersistDoesNotRestartAdbdAgain` y `TestStateKeepsTheLastReadOfPersist`. Sin el plazo propio de `readTCP` (`rctx` = `ctx`), `TestReadTCPHasItsOwnWait` falla a los 10 s.

- [ ] **Step 5: los llamadores de `cli` (sin textos nuevos todavía) y las llamadas esperadas.**

Edit en `host/internal/cli/invariant.go`:

**Reemplazar:**
```go
	restarted, err := device.EnsureNoTCPPort(ctx, runner, keep)
```

**por:**
```go
	_, restarted, err := device.EnsureNoTCPPort(ctx, runner, keep)
```

Edit en `host/internal/cli/invariant.go`:

**Reemplazar:**
```go
	port, err := device.TCPPort(ctx, runner)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return NewErrorT("DEVICE_UNREADABLE", "error.cause", "err.invariant.device_unreadable.fix_2", err.Error())
	}
	if port != "" && port != keep {
```

**por:**
```go
	t, err := device.ReadTCP(ctx, runner)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return NewErrorT("DEVICE_UNREADABLE", "error.cause", "err.invariant.device_unreadable.fix_2", err.Error())
	}
	if t.Now != "" && t.Now != keep {
```

Edit en `host/internal/cli/doctor.go`:

**Reemplazar:**
```go
	port, err := callWithin(ctx, func(c context.Context) (string, error) { return device.TCPPort(c, phone) })
	switch {
	case err != nil:
		return mk("network port", checkWarn, i18n.M("doctor.port.read_failed", causeMsg(err))).withGuide(unlockAndRetry)
	case port == "" || port == "0":
```

**por:**
```go
	t, err := callWithin(ctx, func(c context.Context) (device.TCP, error) { return device.ReadTCP(c, phone) })
	port := t.Now
	switch {
	case err != nil:
		return mk("network port", checkWarn, i18n.M("doctor.port.read_failed", causeMsg(err))).withGuide(unlockAndRetry)
	case port == "":
```

Edit en `host/internal/cli/invariant_test.go`:

**Reemplazar:**
```go
	phoneGone   = "List of devices attached\n"
)
```

**por:**
```go
	phoneGone   = "List of devices attached\n"
)

// tcpPortCalls is how the host reads adbd's TCP port (device.ReadTCP).
const tcpPortCalls = "shell getprop service.adb.tcp.port|shell getprop persist.adb.tcp.port"
```

Edit en `host/internal/cli/invariant_test.go`:

**Reemplazar:**
```go
	getprop := "shell getprop service.adb.tcp.port"
```

**por:**
```go
	getprop := tcpPortCalls
```

Edit en `host/internal/cli/invariant_test.go`:

**Reemplazar:**
```go
		if got[len(got)-1] != getprop || usb != 1 {
```

**por:**
```go
		if !strings.HasSuffix(strings.Join(got, "|"), "|"+getprop) || usb != 1 {
```

Edit en `host/internal/cli/phone_remote_test.go`:

**Reemplazar:**
```go
	if calls := g.phone.called(); len(calls) != 1 || calls[0] != "usb" {
```

**por:**
```go
	if calls := g.phone.called(); strings.Join(calls, "|") != tcpPortCalls+"|usb" {
```

Edit en `host/internal/cli/phone_remote_test.go`:

**Reemplazar:**
```go
	if calls := g.phone.called(); strings.Join(calls, "|") != "tcpip 5555|usb" {
```

**por:**
```go
	if calls := g.phone.called(); strings.Join(calls, "|") != "tcpip 5555|"+tcpPortCalls+"|usb" {
```

Edit en `host/internal/cli/remotecmd_test.go`:

**Reemplazar:**
```go
	if len(calls) != 2 || calls[0] != "tcpip 5555" || calls[1] != "usb" || !h.g.store.Until().IsZero() {
```

**por:**
```go
	if strings.Join(calls, "|") != "tcpip 5555|"+tcpPortCalls+"|usb" || !h.g.store.Until().IsZero() {
```

Edit en `host/cmd/adbuddy/e2e_test.go`:

**Reemplazar:**
```go
	want := []string{"devices", "shell getprop service.adb.tcp.port", "shell dumpsys package
```

**por:**
```go
	want := []string{"devices", "shell getprop service.adb.tcp.port", "shell getprop persist.adb.tcp.port", "shell dumpsys package
```

(El ancla es el principio de la línea; el resto de la línea no cambia.)

- [ ] **Step 6: la aceptación en emulador (ruling 4). Solo compila aquí: no se arranca ningún emulador.**

Edit en `host/internal/emutest/persist_test.go`:

**Reemplazar:**
```go
// closedAfterOff is what this test asks of the host's "off" with a persist
// port set: persist.adb.tcp.port emptied, service.adb.tcp.port closed (empty
// or 0: the values measured after adb usb and at boot, and the host's own
// criterion, portClosed; -1 is not measured and the host would take it as
// open, the safe side) and adbd restarted (a new pid that stays: it cannot
// still be listening on the persist port it took when it started).
func (e *emu) closedAfterOff(t *testing.T, persist func() string, before string) {
	t.Helper()
	if p := persist(); p != "" {
		t.Errorf("persist.adb.tcp.port = %q after off, want it emptied: adbd takes it again at its next start with service.adb.tcp.port empty (a reboot)", p)
	}
```

**por:**
```go
// closedAfterOff is what this test asks of the host's "off" with a persist
// port set (plan 2026-10-04-adbuddy-vb41-persist-port, ruling 4): the host
// cannot empty persist.adb.tcp.port (the shell user may not, and it never
// runs su), so it leaves it as it was; service.adb.tcp.port closed (empty or
// 0: the values measured after adb usb and at boot, and the host's own
// criterion; -1 is not measured and the host would take it as open, the safe
// side) and adbd restarted (a new pid that stays: it cannot still be
// listening on the persist port it took when it started).
func (e *emu) closedAfterOff(t *testing.T, persist func() string, before string) {
	t.Helper()
	if p := persist(); p != persistPort {
		t.Errorf("persist.adb.tcp.port = %q after off, want %s as it was: the host never sets it", p, persistPort)
	}
```

Edit en `host/internal/emutest/persist_test.go`:

**Reemplazar:**
```go
// Security of VB-41 (supervisor's ruling): "off" must leave adbd without a
// TCP port, persist.adb.tcp.port included. The host's two "off" paths, the
// real remote controller's Disable (mode off) and device.EnsureNoTCPPort (the
// invariant at serve's start), run against the emulator's adbd with
// persist.adb.tcp.port=5555. RED EXPECTED TODAY: remote.go and device.go read
// only service.adb.tcp.port and never touch persist, so persist stays 5555
// (and with service.adb.tcp.port empty they do not even restart adbd). A red
// run is the finding: the fix goes in its own branch and plan; this test is
// its acceptance test.
```

**por:**
```go
// Security of VB-41 (plan 2026-10-04-adbuddy-vb41-persist-port): "off" must
// leave adbd without a TCP port now, and never report a plain "closed" while
// persist.adb.tcp.port names a port. The host's two "off" paths, the real
// remote controller's Disable (mode off) and device.EnsureNoTCPPort (the
// invariant at serve's start), run against the emulator's adbd with
// persist.adb.tcp.port=5555. The host reads persist as the shell user (no
// su): this test is also the measurement that the shell user can read it.
// Without root the host cannot empty it, so the test wants it reported, not
// emptied (ruling 4 of the plan).
```

Edit en `host/internal/emutest/persist_test.go`:

**Reemplazar:**
```go
	// persist set and the port opened by hand (adb tcpip): Disable with the
	// mode off sees 5555 and runs adb usb, which leaves persist as it was.
	t.Run("Disable after adb tcpip", func(t *testing.T) {
		persist := e.withPersist(t)
		pid, _ := e.adbd(t)
		e.run(t, "tcpip", remote.Port)
		pid = e.restartedTo(t, pid, remote.Port)
		ctl := remote.New(e.adb, &memStore{}, nil)
		if s, err := ctl.Disable(e.ctx); err != nil || s != (remote.State{}) {
			t.Fatalf("Disable: %+v, %v", s, err)
		}
```

**por:**
```go
	// persist set and the port opened by hand (adb tcpip): Disable with the
	// mode off sees 5555 and runs adb usb, which leaves persist as it was;
	// the state it returns says so.
	t.Run("Disable after adb tcpip", func(t *testing.T) {
		persist := e.withPersist(t)
		pid, _ := e.adbd(t)
		e.run(t, "tcpip", remote.Port)
		pid = e.restartedTo(t, pid, remote.Port)
		ctl := remote.New(e.adb, &memStore{}, nil)
		if s, err := ctl.Disable(e.ctx); err != nil || s != (remote.State{Persist: persistPort}) {
			t.Fatalf("Disable: %+v, %v; want off with Persist %s (read as the shell user)", s, err, persistPort)
		}
```

Edit en `host/internal/emutest/persist_test.go`:

**Reemplazar:**
```go
		restarted, err := device.EnsureNoTCPPort(e.ctx, e.adb, "")
		if err != nil {
			t.Fatalf("EnsureNoTCPPort: %v", err)
		}
		t.Logf("EnsureNoTCPPort ran adb usb: %v", restarted)
		poll(10*time.Second, func() bool { now, _, ok := e.adbdNow(); return ok && now != pid })
		e.closedAfterOff(t, persist, pid)
	})
```

**por:**
```go
		read, restarted, err := device.EnsureNoTCPPort(e.ctx, e.adb, "")
		if err != nil || !restarted || read != (device.TCP{Now: persistPort, Persist: persistPort}) {
			t.Fatalf("EnsureNoTCPPort: read %+v, ran adb usb %v, %v; want the persist port read (as the shell user) and closed", read, restarted, err)
		}
		poll(10*time.Second, func() bool { now, _, ok := e.adbdNow(); return ok && now != pid })
		e.closedAfterOff(t, persist, pid)
		// No loop: with service.adb.tcp.port at 0 now, the invariant reads
		// the persist port and leaves adbd alone.
		pid, port = e.adbd(t)
		again, restarted, err := device.EnsureNoTCPPort(e.ctx, e.adb, "")
		if err != nil || restarted || again != (device.TCP{Persist: persistPort}) {
			t.Fatalf("EnsureNoTCPPort again: read %+v, ran adb usb %v, %v; want only the persist port, and no adb usb", again, restarted, err)
		}
		e.sameAdbd(t, pid, port)
	})
```

Edit en `host/internal/emutest/persist_test.go`:

**Reemplazar:**
```go
	// persist set and service.adb.tcp.port empty, as after a reboot: adbd took
	// the persist port at its start (AOSP adbd reads persist only when the
	// service property is empty), and the host reads "closed".
```

**por:**
```go
	// persist set and service.adb.tcp.port empty, as after a reboot: adbd took
	// the persist port at its start (AOSP adbd reads persist only when the
	// service property is empty), and the host reads it there (device.ReadTCP).
```

Edit en `host/internal/emutest/remote_test.go`:

**Reemplazar:**
```go
// hostReadsPersistPort says whether remote and device.EnsureNoTCPPort read
// persist.adb.tcp.port. They do not (remote.go and device.go read only
// service.adb.tcp.port; pendientes, diferidos B, T2). Hypothesis the reboot
// step measures: after a reboot with persist set, service.adb.tcp.port reads
// empty while adbd listens on the persist port (AOSP adbd reads persist only
// when the service property is empty), and the host reads "closed". If it is
// measured so, it is a finding for the controller; with a fix, set this to
// true and the reboot step then wants Disable to close the port.
const hostReadsPersistPort = false
```

**por:**
```go
// hostReadsPersistPort says whether remote and device.EnsureNoTCPPort read
// persist.adb.tcp.port. They do since the fix of VB-41 (plan
// 2026-10-04-adbuddy-vb41-persist-port: device.ReadTCP). Measured before it
// (T8): after a reboot with persist set, service.adb.tcp.port reads empty
// while adbd listens on the persist port (AOSP adbd reads persist only when
// the service property is empty), and the host read "closed". With the fix
// the reboot step wants Disable to close the port.
const hostReadsPersistPort = true
```

Edit en `host/internal/emutest/remote_test.go`:

**Reemplazar:**
```go
	// With the service property at 0 a restart keeps it at 0 (AOSP adbd then
	// ignores persist), and "off" with the mode off leaves adbd alone.
	_, _, _, _ = e.adb.Output(e.ctx, "shell", "su 0 setprop ctl.restart adbd")
	pid = e.restartedTo(t, pid, "0")
	ctl := remote.New(e.adb, &memStore{}, nil)
	if s, err := ctl.Disable(e.ctx); err != nil || s != (remote.State{}) {
		t.Fatalf("Disable: %+v, %v", s, err)
	}
```

**por:**
```go
	// With the service property at 0 a restart keeps it at 0 (AOSP adbd then
	// ignores persist), and "off" with the mode off leaves adbd alone; it
	// reports the persist port (VB-41).
	_, _, _, _ = e.adb.Output(e.ctx, "shell", "su 0 setprop ctl.restart adbd")
	pid = e.restartedTo(t, pid, "0")
	ctl := remote.New(e.adb, &memStore{}, nil)
	if s, err := ctl.Disable(e.ctx); err != nil || s != (remote.State{Persist: remote.Port}) {
		t.Fatalf("Disable: %+v, %v", s, err)
	}
```

Edit en `host/internal/emutest/remote_test.go`:

**Reemplazar:**
```go
	// After a reboot the service property is empty and persist is still set:
	// adbd takes the persist port, and the host reads "closed".
```

**por:**
```go
	// After a reboot the service property is empty and persist is still set:
	// adbd takes the persist port, and the host reads it there (device.ReadTCP)
	// and closes it with adb usb.
```

Edit en `host/internal/emutest/remote_test.go`:

**Reemplazar:**
```go
	if s, err := ctl.Disable(e.ctx); err != nil || s != (remote.State{}) {
		t.Fatalf("Disable after the reboot: %+v, %v", s, err)
	}
```

**por:**
```go
	if s, err := ctl.Disable(e.ctx); err != nil || s != (remote.State{Persist: remote.Port}) {
		t.Fatalf("Disable after the reboot: %+v, %v", s, err)
	}
```

- [ ] **Step 7: suite completa y commit.**

Run: `cd host && go build ./... && go vet ./... && gofmt -l . && go test -count=1 -timeout 15m ./...`
Expected: sin salida de `gofmt`; todos los paquetes con tests en `ok` (`internal/emutest` salta sin `ADBUDDY_EMULATOR_SERIAL`; compila).

```bash
git add host/internal/device host/internal/remote host/internal/cli/invariant.go host/internal/cli/doctor.go host/internal/cli/invariant_test.go host/internal/cli/phone_remote_test.go host/internal/cli/remotecmd_test.go host/cmd/adbuddy/e2e_test.go host/internal/emutest/persist_test.go host/internal/emutest/remote_test.go
git commit -m "fix(remote): leer persist.adb.tcp.port y no dar por cerrado lo que no se leyó (VB-41)

device.ReadTCP lee service.adb.tcp.port y persist.adb.tcp.port: el puerto
de adbd es service, o persist si aquel está vacío (AOSP); una lectura fallida
es desconocido. EnsureNoTCPPort, Disable, Reconcile, Enable y Expire deciden
con él, y el estado del controlador dice lo que guarda persist. Sin root el
host no puede vaciarlo: avisa (Tarea 2).

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: avisar de persist en doctor, en el log de serve y en `adbuddy remote` (textos en/es) 🔒

**Files:** `host/internal/cli/{invariant,doctor,remotecmd,phone_remote,phone_watch,serve}.go`, sus tests, `doctor_lang_test.go`, `messages_lang_test.go` y los dos catálogos.

- Produces: `func tellPersist(log io.Writer, persist string, unknown bool)` y `func invariantTCP(ctx context.Context, runner adb.Runner, wait time.Duration, log io.Writer, command, keep string) (device.TCP, error)` (en `invariant.go`; `applyInvariant` la envuelve y no cambia de firma); `phoneWatch.Port device.TCP`, `phoneWatch.Seen func(device.TCP)`, `phoneWatch.ReadWait time.Duration`; `func (w *watcher) recheckPort(ctx context.Context)`; `switchTo(ctx, serial, where)` (antes sin `ctx`); `remoteInfo.PersistPort string` (`json:"persistPort,omitempty"`) y `remoteInfo.PersistUnknown bool` (`json:"persistUnknown,omitempty"`); las claves de "Textos nuevos".

**Concurrencia:** `recheckPort` corre en la gorrutina del vigilante (un paso a la vez, como hoy) y su estado (`said`, `unknown`) es solo suyo; llama a `Seen` (`ctl.Note`, que toma `pmu`) y a `RemoteOn` (el almacén, seguro para uso concurrente). `ReadWait` se lee en `serve` antes de lanzar la gorrutina. `tellPersist` escribe en el `log` de `serve` (ya lo comparten `watchRemote`, `remoteLink` y el invariante; `syncBuffer` en los tests). En `ctlADB`, `persist` se lee bajo `a.mu` como el resto de campos; los tests que lo cambian con `serve` en marcha lo hacen bajo `h.g.phone.mu` o antes de lanzar la gorrutina.

- [ ] **Step 1: tests que fallan con el código de la Tarea 1.** Compilan: miran textos, el log y el JSON en bruto, no campos nuevos.

Edit en `host/internal/cli/phone_remote_test.go`:

**Reemplazar:**
```go
type ctlADB struct {
	mu     sync.Mutex
	calls  []string
	fail   map[string]bool
	hang   map[string]bool // wait for the context to end
	gate   chan struct{}   // `tcpip 5555` waits for it to close
	port   string          // answer of getprop service.adb.tcp.port
	stderr string          // what a failing command says (default "error: closed")
}
```

**por:**
```go
type ctlADB struct {
	mu      sync.Mutex
	calls   []string
	fail    map[string]bool
	hang    map[string]bool // wait for the context to end
	gate    chan struct{}   // `tcpip 5555` waits for it to close
	port    string          // answer of getprop service.adb.tcp.port
	persist string          // answer of getprop persist.adb.tcp.port
	stderr  string          // what a failing command says (default "error: closed")
}
```

Edit en `host/internal/cli/phone_remote_test.go`:

**Reemplazar:**
```go
	hang, gate, fail, port, serr := a.hang[key], a.gate, a.fail[key], a.port, a.stderr
```

**por:**
```go
	hang, gate, fail, port, persist, serr := a.hang[key], a.gate, a.fail[key], a.port, a.persist, a.stderr
```

Edit en `host/internal/cli/phone_remote_test.go`:

**Reemplazar:**
```go
	if key == "shell getprop service.adb.tcp.port" {
		io.WriteString(stdout, port+"\n")
	}
```

**por:**
```go
	if key == "shell getprop service.adb.tcp.port" {
		io.WriteString(stdout, port+"\n")
	}
	if key == "shell getprop persist.adb.tcp.port" {
		io.WriteString(stdout, persist+"\n")
	}
```

Añadir al final de `host/internal/cli/invariant_test.go`:

```go

// VB-41: a persist port is said once in serve's log: with no `adb usb` when
// the service property is at 0 (adb usb already ran since the boot), and
// after the `adb usb` that closed it when the service property was empty (as
// after a reboot). A port still open after `adb usb` because of persist gets
// a fix that does not say "restart the phone" (that opens it again).
func TestApplyInvariantWarnsAboutAPersistPort(t *testing.T) {
	setVar(t, &usbDropWait, 300*time.Millisecond)
	setVar(t, &invariantPoll, 20*time.Millisecond)
	persistRule := testutil.FakeRule{Match: `^shell getprop persist\.adb\.tcp\.port$`, Stdout: "5555\n"}
	const warning = "persist.adb.tcp.port is 5555"
	t.Run("service at 0", func(t *testing.T) {
		r, log := invariantRunner(t, testutil.FakeRule{Match: tcpPortRule, Stdout: "0\n"}, persistRule)
		var errb strings.Builder
		if err := applyInvariant(context.Background(), r, time.Second, &errb, "adbuddy serve", ""); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(loggedCalls(t, log), "|"); got != tcpPortCalls {
			t.Fatalf("calls %q: adb usb must not run", got)
		}
		if strings.Count(errb.String(), warning) != 1 {
			t.Fatalf("stderr %q", errb.String())
		}
	})
	t.Run("after a reboot", func(t *testing.T) {
		r, log := invariantRunner(t,
			testutil.FakeRule{Match: tcpPortRule, Stdout: "\n", Times: 1},
			testutil.FakeRule{Match: tcpPortRule, Stdout: "0\n"},
			persistRule,
			testutil.FakeRule{Match: "^usb$", Stdout: "restarting in USB mode\n"},
			testutil.FakeRule{Match: "^devices$", Stdout: phoneGone, Times: 1},
			testutil.FakeRule{Match: "^devices$", Stdout: phoneOnline},
		)
		var errb strings.Builder
		if err := applyInvariant(context.Background(), r, time.Second, &errb, "adbuddy serve", ""); err != nil {
			t.Fatal(err)
		}
		got := strings.Join(loggedCalls(t, log), "|")
		if strings.Count(got, "usb") != 1 || !strings.HasPrefix(got, tcpPortCalls+"|usb|") || !strings.HasSuffix(got, "|"+tcpPortCalls) {
			t.Fatalf("calls %q: want one adb usb and the port read again", got)
		}
		if strings.Count(errb.String(), warning) != 1 {
			t.Fatalf("stderr %q", errb.String())
		}
	})
	t.Run("still open after adb usb", func(t *testing.T) {
		r, _ := invariantRunner(t,
			testutil.FakeRule{Match: tcpPortRule, Stdout: "\n"},
			persistRule,
			testutil.FakeRule{Match: "^devices$", Stdout: phoneGone, Times: 1},
			testutil.FakeRule{Match: "^devices$", Stdout: phoneOnline},
		)
		err := applyInvariant(context.Background(), r, time.Second, &strings.Builder{}, "adbuddy serve", "")
		fix := AsError(err).Fix
		if errorCode(err) != "DEVICE_PORT_OPEN" || !strings.Contains(fix, "persist.adb.tcp.port is 5555") || !strings.Contains(fix, "adbuddy serve") || strings.Contains(fix, "that closes a port") {
			t.Fatalf("%v (fix %q)", err, fix)
		}
	})
}
```

Añadir al final de `host/internal/cli/doctor_test.go`:

```go

// VB-41: a persist port is never "closed": doctor says it, what it does and
// how to remove it, whether adbd listens on it now (the service property
// empty, as after a reboot) or not (at 0, after adb usb). A persist property
// that cannot be read is "could not read", never "closed".
func TestDoctorNetworkPortPersist(t *testing.T) {
	persist := testutil.FakeRule{Match: `^shell getprop persist\.adb\.tcp\.port$`, Stdout: "5555\n"}
	for name, service := range map[string]string{"service at 0": "0\n", "after a reboot": "\n"} {
		t.Run(name, func(t *testing.T) {
			doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^shell getprop service\.adb\.tcp\.port$`, Stdout: service}, persist))
			out, _, code := runWithInput(t, "", "doctor", "--json")
			c := statusOf(doctorJSON(t, out), "network port")
			if code != 0 || c.Status != checkWarn || !strings.Contains(c.Detail, "5555") ||
				!strings.Contains(c.Guidance, "did not set it") || !strings.Contains(c.Guidance, "setprop persist.adb.tcp.port ''") || strings.Contains(c.Guidance, "Nothing to do") {
				t.Fatalf("exit %d %+v", code, c)
			}
		})
	}
	t.Run("unreadable", func(t *testing.T) {
		doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^shell getprop persist\.adb\.tcp\.port$`, Stderr: "error: closed\n", Exit: 1}))
		out, _, _ := runWithInput(t, "", "doctor", "--json")
		c := statusOf(doctorJSON(t, out), "network port")
		if c.Status != checkWarn || !strings.HasPrefix(c.Detail, "could not read adb's network port") {
			t.Fatalf("%+v", c)
		}
	})
}
```

Añadir al final de `host/internal/cli/phone_remote_test.go`:

```go

// VB-41: an "off" from the app that finds persist.adb.tcp.port set says so
// in serve's log, once.
func TestRemoteSetOffSaysThePersistPort(t *testing.T) {
	g := newRemoteRig(t, withTailscale(), "")
	g.phone.persist = "5555"
	if err := g.store.SetUntil(g.clock.now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	g.r.handleSet(setMsg("r2", `{"enabled":false}`))
	g.waitMsgs(t, 1)
	if strings.Count(g.log.String(), "persist.adb.tcp.port is 5555") != 1 {
		t.Fatalf("log %q", g.log.String())
	}
}

// VB-41: the expiry that closes the port says the persist port too, once.
func TestWatchRemoteSaysThePersistPortWhenItExpires(t *testing.T) {
	g := newRemoteRig(t, withTailscale(), "")
	g.phone.persist = "7777"
	if err := g.store.SetUntil(g.clock.now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	g.r.refresh()
	go watchRemote(g.ctx, g.log, g.ctl, g.r, 5*time.Millisecond, time.Second)
	g.clock.advance(2 * time.Hour)
	g.waitMsgs(t, 1)
	waitUntil(t, "the warning", func() bool { return strings.Contains(g.log.String(), "persist.adb.tcp.port is 7777") })
	time.Sleep(30 * time.Millisecond) // more ticks: nothing more to say
	if n := strings.Count(g.log.String(), "persist.adb.tcp.port is 7777"); n != 1 || g.phone.count("usb") != 1 {
		t.Fatalf("said %d times, usb %d: %q", n, g.phone.count("usb"), g.log.String())
	}
}
```

Añadir al final de `host/internal/cli/remotecmd_test.go`:

```go

// VB-41: `adbuddy remote off` never says "closed" while persist.adb.tcp.port
// names a port, or could not be read; status and --json keep saying it.
func TestRemoteOffThroughServeSaysThePersistPort(t *testing.T) {
	h := newRemoteHost(t, withTailscale(), confirm.Allow)
	h.g.phone.mu.Lock()
	h.g.phone.persist = "5555"
	h.g.phone.mu.Unlock()
	out, errOut, code := run(t, "remote", "off")
	if code != 0 || !strings.Contains(out, "persist.adb.tcp.port is 5555") || strings.Contains(out, "is closed") {
		t.Fatalf("off: exit %d stdout %q stderr %q", code, out, errOut)
	}
	if out, _, code = run(t, "remote", "status"); code != 0 || !strings.Contains(out, "persist.adb.tcp.port is 5555") {
		t.Fatalf("status: exit %d stdout %q", code, out)
	}
	if out, _, code = run(t, "remote", "status", "--json"); code != 0 || !strings.Contains(out, `"persistPort":"5555"`) {
		t.Fatalf("json: exit %d stdout %q", code, out)
	}
	h.g.phone.setFail("shell getprop persist.adb.tcp.port", true)
	if out, _, code = run(t, "remote", "off"); code != 0 || !strings.Contains(out, "could not be read") || strings.Contains(out, "is closed") {
		t.Fatalf("unreadable: exit %d stdout %q", code, out)
	}
}

// VB-41: off without serve reads persist.adb.tcp.port before `adb usb` and
// says it.
func TestRemoteOffWithoutServeSaysThePersistPort(t *testing.T) {
	until := time.Now().Add(5 * time.Hour).UTC().Format(time.RFC3339)
	e := newSetupEnv(t, []testutil.FakeRule{
		{Match: "^start-server$"},
		{Match: "^devices$", Stdout: phoneOnline},
		{Match: `^shell getprop ro\.serialno$`, Stdout: "AAAA1111\n"},
		{Match: `^shell getprop persist\.adb\.tcp\.port$`, Stdout: "5555\n"},
		{Match: "^usb$", Stdout: "restarting in USB mode\n"},
	})
	saveRemoteCmdConfig(t, e, until, phoneSerial)
	out, errOut, code := run(t, "remote", "off")
	if code != 0 || !strings.Contains(out, "persist.adb.tcp.port is 5555") || strings.Contains(out, "is closed") {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
	calls := strings.Join(loggedCalls(t, e.log), "|")
	if i, j := strings.Index(calls, "getprop persist.adb.tcp.port"), strings.Index(calls, "usb"); i < 0 || j < i {
		t.Fatalf("calls %q: persist must be read before adb usb", calls)
	}
}
```

Edit en `host/internal/cli/doctor_lang_test.go`:

**Reemplazar:**
```go
		{"adb server down", doctorRules(noServer), []string{
```

**por:**
```go
		{"persist port", doctorRules(testutil.FakeRule{Match: `^shell getprop service\.adb\.tcp\.port$`, Stdout: "0\n"}, testutil.FakeRule{Match: `^shell getprop persist\.adb\.tcp\.port$`, Stdout: "5555\n"}), []string{
			"⚠ puerto de red: cerrado por ahora, pero persist.adb.tcp.port vale 5555", "    Qué hacer: ADBuddy no la fijó"}, nil},
		{"adb server down", doctorRules(noServer), []string{
```

Revisión I1, M1 y M5: el móvil que vuelve con `serve` en marcha, doctor con `serve` en marcha y `remote off` sin serve con la lectura de persist colgada (para esta, `remotecmd_test.go` importa `device`):

Añadir al final de `host/internal/cli/phone_watch_test.go`:

```go

// VB-41 (review I1): a phone that comes back may have restarted and taken
// persist.adb.tcp.port. The watcher reads the port again (it never closes it
// mid-session) and says once what changed: a port open with the remote mode
// off, and the persist port.
func TestWatchPhoneRereadsThePortWhenThePhoneComesBack(t *testing.T) {
	back := func(t *testing.T, r *rig) {
		r.set(t, devicesRule(phoneSerial), testutil.FakeRule{Match: tcpPortRule, Stdout: "\n"},
			testutil.FakeRule{Match: `^shell getprop persist\.adb\.tcp\.port$`, Stdout: "5555\n"})
	}
	t.Run("remote mode off", func(t *testing.T) {
		r := newRig(t, devicesRule(), failRule(tsAddr))
		w, _, log := watcherFor(t, r, tsConfig(), false, phoneSerial)
		steps(w, 2)
		back(t, r)
		steps(w, 2)
		if strings.Count(log.String(), "adb listens on network port 5555 on the phone again") != 1 || strings.Count(log.String(), "persist.adb.tcp.port is 5555") != 1 {
			t.Fatalf("log %q", log.String())
		}
		if r.count(t, "usb") != 0 || r.count(t, "tcpip") != 0 {
			t.Fatalf("the watcher closed or opened a port: %q", r.calls(t))
		}
		// Lost and back again with the same port: nothing new to say.
		r.set(t, devicesRule(), failRule(tsAddr))
		steps(w, 2)
		back(t, r)
		steps(w, 1)
		if n := strings.Count(log.String(), "persist.adb.tcp.port is 5555"); n != 1 {
			t.Fatalf("said %d times: %q", n, log.String())
		}
	})
	t.Run("remote mode on", func(t *testing.T) {
		r := newRig(t, devicesRule(), failRule(tsAddr), failRule(tsHome))
		w, _, log := watcherFor(t, r, tsConfig(), true, phoneSerial)
		steps(w, 2)
		back(t, r)
		steps(w, 1)
		if strings.Contains(log.String(), "adb listens on network port") || strings.Count(log.String(), "persist.adb.tcp.port is 5555") != 1 {
			t.Fatalf("log %q", log.String())
		}
	})
}
```

Añadir al final de `host/internal/cli/doctor_test.go`:

```go

// VB-41 (review I1): with serve running, an open port is not closed until
// serve starts again (when the phone reconnects, serve only warns): doctor
// does not say "nothing to do" then.
func TestDoctorNetworkPortOpenWithServeRunning(t *testing.T) {
	e := doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^shell getprop service\.adb\.tcp\.port$`, Stdout: "5555\n"}))
	doctorServe(t, e)
	out, _, _ := runWithInput(t, "", "doctor", "--json")
	c := statusOf(doctorJSON(t, out), "network port")
	if c.Status != checkWarn || strings.Contains(c.Guidance, "Nothing to do") || !strings.Contains(c.Guidance, "adbuddy remote off") {
		t.Fatalf("%+v", c)
	}
}
```

Añadir al final de `host/internal/cli/remotecmd_test.go`:

```go

// VB-41 (review M1, M5): off without serve with a persist read that hangs:
// the read has its own short deadline, `adb usb` still runs, and the answer
// says persist could not be read (never "closed").
func TestRemoteOffWithoutServePersistUnreadable(t *testing.T) {
	setVar(t, &device.ReadTCPWait, 300*time.Millisecond)
	until := time.Now().Add(5 * time.Hour).UTC().Format(time.RFC3339)
	e := newSetupEnv(t, []testutil.FakeRule{
		{Match: "^start-server$"},
		{Match: "^devices$", Stdout: phoneOnline},
		{Match: `^shell getprop ro\.serialno$`, Stdout: "AAAA1111\n"},
		{Match: `^shell getprop persist\.adb\.tcp\.port$`, Stdout: "5555\n", SleepMs: 20000},
		{Match: "^usb$", Stdout: "restarting in USB mode\n"},
	})
	saveRemoteCmdConfig(t, e, until, phoneSerial)
	start := time.Now()
	out, errOut, code := run(t, "remote", "off")
	if code != 0 || !strings.Contains(out, "could not be read") || strings.Contains(out, "is closed") || time.Since(start) > 15*time.Second {
		t.Fatalf("exit %d stdout %q stderr %q after %s", code, out, errOut, time.Since(start))
	}
	if !strings.Contains(strings.Join(loggedCalls(t, e.log), "|"), "usb") {
		t.Fatalf("adb usb did not run: %q", loggedCalls(t, e.log))
	}
}
```

Edit en `host/internal/cli/remotecmd_test.go`:

**Reemplazar:**
```go
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
```

**por:**
```go
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/device"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
```

- [ ] **Step 2: comprobar el RED.**

Run: `cd host && gofmt -l internal/cli && go vet ./internal/cli/ && go test -count=1 -timeout 10m -run 'TestApplyInvariantWarnsAboutAPersistPort|TestDoctorNetworkPortPersist|TestRemoteSetOffSaysThePersistPort|TestWatchRemoteSaysThePersistPortWhenItExpires|TestRemoteOffThroughServeSaysThePersistPort|TestRemoteOffWithoutServeSaysThePersistPort|TestDoctorInSpanish|TestWatchPhoneRereadsThePortWhenThePhoneComesBack|TestDoctorNetworkPortOpenWithServeRunning|TestRemoteOffWithoutServePersistUnreadable' ./internal/cli/`
Expected: `FAIL`, sin errores de compilación ni `panic`; 19 líneas `--- FAIL` (10 tests y 9 subtests), todas por el aviso o la lectura que faltan:
- `TestDoctorInSpanish/persist_port`: el informe dice `✓ puerto de red: cerrado (sin modo adb tcpip)`.
- `TestDoctorNetworkPortPersist`: `service_at_0` → `Status:ok Detail:closed (no adb tcpip mode)`; `after_a_reboot` → `Guidance:Nothing to do: serve closes it (remote mode is off).`; `unreadable` → `Detail:could not read service.adb.tcp.port: adb shell getprop persist.adb.tcp.port: exit 1: error: closed`.
- `TestApplyInvariantWarnsAboutAPersistPort`: `service_at_0` → `stderr ""`; `after_a_reboot` → solo `Closed adb's network port on the phone (adb usb)…`; `still_open_after_adb_usb` → fix `Restart the phone (that closes a port opened with \`adb tcpip\`)…`.
- `TestRemoteSetOffSaysThePersistPort` → `log ""`; `TestWatchRemoteSaysThePersistPortWhenItExpires` → `timed out waiting for the warning` (5 s).
- `TestRemoteOffThroughServeSaysThePersistPort` y `TestRemoteOffWithoutServeSaysThePersistPort` → `stdout "Remote mode is off: port 5555 is closed.\n"`.
- `TestWatchPhoneRereadsThePortWhenThePhoneComesBack` (los dos subtests) → el log solo dice `The phone is not reachable … The phone is reachable again.`
- `TestDoctorNetworkPortOpenWithServeRunning` → `Guidance:Nothing to do: serve closes it (remote mode is off).`
- `TestRemoteOffWithoutServePersistUnreadable` → `stdout "Remote mode is off: port 5555 is closed.\n"` (hoy no lee persist).

- [ ] **Step 3: los textos (en y es).**

Edit en `host/internal/i18n/catalog/en.toml`:

**Reemplazar:**
```toml
"err.invariant.device_port_open.fix" = "Restart the phone (that closes a port opened with `adb tcpip`), turn wireless debugging on again and run `%[1]s` again."
```

**por:**
```toml
"err.invariant.device_port_open.fix" = "Restart the phone (that closes a port opened with `adb tcpip`), turn wireless debugging on again and run `%[1]s` again."
"err.invariant.device_port_open.persist_fix" = "persist.adb.tcp.port is %[2]s on the phone and keeps the port open; ADBuddy did not set it and cannot clear it without root. Turn off whatever set it (a ROM's 'ADB over network' setting, a root app or a command run earlier), or empty it as root (`setprop persist.adb.tcp.port ''`), restart the phone and run `%[1]s` again."
```

Edit en `host/internal/i18n/catalog/en.toml`:

**Reemplazar:**
```toml
"doctor.port.read_failed" = "could not read service.adb.tcp.port: %[1]s"
```

**por:**
```toml
"doctor.port.read_failed" = "could not read adb's network port (service.adb.tcp.port and persist.adb.tcp.port): %[1]s"
```

Edit en `host/internal/i18n/catalog/en.toml`:

**Reemplazar:**
```toml
"doctor.port.nothing_to_do" = "Nothing to do: serve closes it (remote mode is off)."
```

**por:**
```toml
"doctor.port.nothing_to_do" = "Nothing to do: serve closes it when it starts (remote mode is off)."
"doctor.port.serve_running" = "serve closes it only when it starts; when the phone reconnects while serve runs, it only warns. Run `adbuddy remote off` to close it now."
"doctor.port.persist" = "closed for now, but persist.adb.tcp.port is %[1]s: adb listens on port %[1]s again, on any network, every time the phone restarts"
"doctor.port.persist_fix" = "ADBuddy did not set it and cannot clear it without root: `adb usb` (serve runs it when it starts, and `adbuddy remote off` runs it) only closes the port until the next restart, and when the phone restarts while serve runs, serve only warns. Turn off whatever set it (a ROM's 'ADB over network' setting, a root app or a command run earlier), or empty it as root (`setprop persist.adb.tcp.port ''`) and restart the phone."
```

Edit en `host/internal/i18n/catalog/en.toml`:

**Reemplazar:**
```toml
"remote.text.no_serve_rest" = "Nothing closes it while serve is stopped: run 'adbuddy remote off'."
```

**por:**
```toml
"remote.text.no_serve_rest" = "Nothing closes it while serve is stopped: run 'adbuddy remote off'."
"remote.text.off_mode" = "Remote mode is off."
"remote.text.persist" = "But adb's network port is not closed for good: persist.adb.tcp.port is %[1]s (ADBuddy did not set it), and adb listens on that port again every time the phone restarts. `adbuddy doctor` says how to remove it."
"remote.text.persist_unknown" = "But persist.adb.tcp.port could not be read: adb's network port may open again when the phone restarts. Run `adbuddy doctor` to check."
"remote.persist.log" = "⚠ persist.adb.tcp.port is %[1]s on the phone (ADBuddy did not set it): adb listens on port %[1]s again, on any network, every time the phone restarts. `adbuddy doctor` says how to remove it."
"remote.persist.unknown_log" = "⚠ Could not read persist.adb.tcp.port: adb's network port may open again when the phone restarts. Run `adbuddy doctor` to check."
```

Edit en `host/internal/i18n/catalog/es.toml`:

**Reemplazar:**
```toml
"err.invariant.device_port_open.fix" = "Reinicia el móvil (eso cierra un puerto abierto con `adb tcpip`), vuelve a activar la depuración inalámbrica y ejecuta de nuevo `%[1]s`."
```

**por:**
```toml
"err.invariant.device_port_open.fix" = "Reinicia el móvil (eso cierra un puerto abierto con `adb tcpip`), vuelve a activar la depuración inalámbrica y ejecuta de nuevo `%[1]s`."
"err.invariant.device_port_open.persist_fix" = "persist.adb.tcp.port vale %[2]s en el móvil y mantiene el puerto abierto; ADBuddy no la fijó y no puede borrarla sin root. Desactiva lo que la fijó (el ajuste «ADB por red» de una ROM, una app con root o una orden de antes), o vacíala como root (`setprop persist.adb.tcp.port ''`), reinicia el móvil y vuelve a ejecutar `%[1]s`."
```

Edit en `host/internal/i18n/catalog/es.toml`:

**Reemplazar:**
```toml
"doctor.port.read_failed" = "no se pudo leer service.adb.tcp.port: %[1]s"
```

**por:**
```toml
"doctor.port.read_failed" = "no se pudo leer el puerto de red de adb (service.adb.tcp.port y persist.adb.tcp.port): %[1]s"
```

Edit en `host/internal/i18n/catalog/es.toml`:

**Reemplazar:**
```toml
"doctor.port.nothing_to_do" = "No hay nada que hacer: serve lo cierra (el modo remoto está desactivado)."
```

**por:**
```toml
"doctor.port.nothing_to_do" = "No hay nada que hacer: serve lo cierra al arrancar (el modo remoto está desactivado)."
"doctor.port.serve_running" = "serve solo lo cierra al arrancar; si el móvil se reconecta con serve en marcha, solo avisa. Ejecuta `adbuddy remote off` para cerrarlo ya."
"doctor.port.persist" = "cerrado por ahora, pero persist.adb.tcp.port vale %[1]s: adb vuelve a escuchar en el puerto %[1]s, en cualquier red, cada vez que el móvil se reinicia"
"doctor.port.persist_fix" = "ADBuddy no la fijó y no puede borrarla sin root: `adb usb` (serve lo ejecuta al arrancar, y también `adbuddy remote off`) solo cierra el puerto hasta el siguiente reinicio, y si el móvil se reinicia con serve en marcha, serve solo avisa. Desactiva lo que la fijó (el ajuste «ADB por red» de una ROM, una app con root o una orden de antes), o vacíala como root (`setprop persist.adb.tcp.port ''`) y reinicia el móvil."
```

Edit en `host/internal/i18n/catalog/es.toml`:

**Reemplazar:**
```toml
"remote.text.no_serve_rest" = "Nada lo cierra mientras serve esté detenido: ejecuta 'adbuddy remote off'."
```

**por:**
```toml
"remote.text.no_serve_rest" = "Nada lo cierra mientras serve esté detenido: ejecuta 'adbuddy remote off'."
"remote.text.off_mode" = "El modo remoto está desactivado."
"remote.text.persist" = "Pero el puerto de red de adb no queda cerrado del todo: persist.adb.tcp.port vale %[1]s (ADBuddy no la fijó), y adb vuelve a escuchar en ese puerto cada vez que el móvil se reinicia. `adbuddy doctor` explica cómo quitarla."
"remote.text.persist_unknown" = "Pero no se pudo leer persist.adb.tcp.port: el puerto de red de adb puede volver a abrirse cuando el móvil se reinicie. Ejecuta `adbuddy doctor` para comprobarlo."
"remote.persist.log" = "⚠ persist.adb.tcp.port vale %[1]s en el móvil (ADBuddy no la fijó): adb vuelve a escuchar en el puerto %[1]s, en cualquier red, cada vez que el móvil se reinicia. `adbuddy doctor` explica cómo quitarla."
"remote.persist.unknown_log" = "⚠ No se pudo leer persist.adb.tcp.port: el puerto de red de adb puede volver a abrirse cuando el móvil se reinicie. Ejecuta `adbuddy doctor` para comprobarlo."
```

Edit en `host/internal/i18n/catalog/en.toml`:

**Reemplazar:**
```toml
"phone_watch.the_phone_is_now_at" = "📱 The phone is now at %[1]s."
```

**por:**
```toml
"phone_watch.the_phone_is_now_at" = "📱 The phone is now at %[1]s."
"phone_watch.port_open" = "⚠ adb listens on network port %[1]s on the phone again and remote mode is off: serve does not close it while it runs. Run `adbuddy remote off` to close it now."
```

Edit en `host/internal/i18n/catalog/es.toml`:

**Reemplazar:**
```toml
"phone_watch.the_phone_is_now_at" = "📱 El móvil está ahora en %[1]s."
```

**por:**
```toml
"phone_watch.the_phone_is_now_at" = "📱 El móvil está ahora en %[1]s."
"phone_watch.port_open" = "⚠ adb vuelve a escuchar en el puerto de red %[1]s del móvil y el modo remoto está desactivado: serve no lo cierra mientras está en marcha. Ejecuta `adbuddy remote off` para cerrarlo ya."
```

- [ ] **Step 4: el código.**

Edit en `host/internal/cli/invariant.go`:

**Reemplazar:**
```go
// what the user runs again after a failure. When ctx ends, the error is whatever the step returned: callers turn it into
// Interrupted.
func applyInvariant(ctx context.Context, runner adb.Runner, wait time.Duration, log io.Writer, command, keep string) error {
	_, restarted, err := device.EnsureNoTCPPort(ctx, runner, keep)
	if err != nil {
		return NewErrorT("DEVICE_UNREADABLE", "error.cause", "err.invariant.device_unreadable.fix", err.Error())
	}
	if !restarted {
		return nil
	}
	tell(log, "invariant.closed_adb_s_network_port")
	if err := waitDropped(ctx, runner, usbDropWait); err != nil {
		return err
	}
	if _, err := runner.WaitSerial(ctx, runner.Serial, wait); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return TransientErrorT("DEVICE_NOT_FOUND", "err.invariant.device_not_found", "err.invariant.device_not_found.fix", command)
	}
	// Only read the port now: another `adb usb` would restart adbd again.
	t, err := device.ReadTCP(ctx, runner)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return NewErrorT("DEVICE_UNREADABLE", "error.cause", "err.invariant.device_unreadable.fix_2", err.Error())
	}
	if t.Now != "" && t.Now != keep {
		return NewErrorT("DEVICE_PORT_OPEN", "err.invariant.device_port_open", "err.invariant.device_port_open.fix", command)
	}
	return nil
}
```

**por:**
```go
// what the user runs again after a failure. When ctx ends, the error is whatever the step returned: callers turn it into
// Interrupted. A persist.adb.tcp.port that names a port (VB-41) is said once
// in log, from the last read: ADBuddy cannot clear it, and adbd listens on it
// again when the phone restarts.
func applyInvariant(ctx context.Context, runner adb.Runner, wait time.Duration, log io.Writer, command, keep string) error {
	_, err := invariantTCP(ctx, runner, wait, log, command, keep)
	return err
}

// invariantTCP is applyInvariant that also returns the last read of the port:
// serve gives it to the remote controller (Note) and to the watcher of the
// connection, so both know what persist.adb.tcp.port holds from the start.
func invariantTCP(ctx context.Context, runner adb.Runner, wait time.Duration, log io.Writer, command, keep string) (device.TCP, error) {
	t, restarted, err := device.EnsureNoTCPPort(ctx, runner, keep)
	if err != nil {
		return t, NewErrorT("DEVICE_UNREADABLE", "error.cause", "err.invariant.device_unreadable.fix", err.Error())
	}
	if !restarted {
		tellPersist(log, t.Persist, false)
		return t, nil
	}
	tell(log, "invariant.closed_adb_s_network_port")
	if err := waitDropped(ctx, runner, usbDropWait); err != nil {
		return t, err
	}
	if _, err := runner.WaitSerial(ctx, runner.Serial, wait); err != nil {
		if ctx.Err() != nil {
			return t, ctx.Err()
		}
		return t, TransientErrorT("DEVICE_NOT_FOUND", "err.invariant.device_not_found", "err.invariant.device_not_found.fix", command)
	}
	// Only read the port now: another `adb usb` would restart adbd again.
	t, err = device.ReadTCP(ctx, runner)
	if err != nil {
		if ctx.Err() != nil {
			return t, ctx.Err()
		}
		return t, NewErrorT("DEVICE_UNREADABLE", "error.cause", "err.invariant.device_unreadable.fix_2", err.Error())
	}
	if t.Now != "" && t.Now != keep {
		// Restarting the phone closes a port of `adb tcpip`, but opens
		// persist's again: that fix only without persist.
		if t.Persist != "" {
			return t, NewErrorT("DEVICE_PORT_OPEN", "err.invariant.device_port_open", "err.invariant.device_port_open.persist_fix", command, t.Persist)
		}
		return t, NewErrorT("DEVICE_PORT_OPEN", "err.invariant.device_port_open", "err.invariant.device_port_open.fix", command)
	}
	tellPersist(log, t.Persist, false)
	return t, nil
}

// tellPersist says in log that persist.adb.tcp.port names a port (VB-41:
// ADBuddy did not set it, cannot clear it, and adbd listens on it again when
// the phone restarts), or that it could not be read (unknown). Nothing when
// neither.
func tellPersist(log io.Writer, persist string, unknown bool) {
	switch {
	case persist != "":
		tell(log, "remote.persist.log", persist)
	case unknown:
		tell(log, "remote.persist.unknown_log")
	}
}
```

Edit en `host/internal/cli/doctor.go`:

**Reemplazar:**
```go
// says until when; with serve stopped nothing closes it, even after that.
func checkRemotePort(
```

**por:**
```go
// says until when; with serve stopped nothing closes it, even after that.
// persist.adb.tcp.port is read too (device.ReadTCP, VB-41): a port it names
// is never "closed", and a read that fails is "could not read".
func checkRemotePort(
```

Edit en `host/internal/cli/doctor.go`:

**Reemplazar:**
```go
	case port == "":
		return mk("network port", checkOK, i18n.M("doctor.port.closed"))
	}
```

**por:**
```go
	case port == "" && t.Persist == "":
		return mk("network port", checkOK, i18n.M("doctor.port.closed"))
	case port == "":
		// VB-41: closed now, but adbd takes persist again when the phone restarts.
		return mk("network port", checkWarn, i18n.M("doctor.port.persist", t.Persist)).withGuide(i18n.M("doctor.port.persist_fix"))
	}
```

Edit en `host/internal/cli/doctor.go`:

**Reemplazar:**
```go
	return mk("network port", checkWarn, i18n.M("doctor.port.open", port)).withGuide(i18n.M("doctor.port.nothing_to_do"))
}
```

**por:**
```go
	c := mk("network port", checkWarn, i18n.M("doctor.port.open", port))
	switch {
	case t.Persist != "":
		// serve's adb usb closes it, but only until the phone restarts.
		return c.withGuide(i18n.M("doctor.port.persist_fix"))
	case serveRunning:
		// serve closes it only when it starts; on a reconnection it only warns.
		return c.withGuide(i18n.M("doctor.port.serve_running"))
	}
	return c.withGuide(i18n.M("doctor.port.nothing_to_do"))
}
```

(Con el modo remoto encendido y persist fijado, doctor sigue con su guía del modo, que es lo urgente; el aviso de persist llega al apagarlo: ruling 8. Queda dicho en "Riesgos abiertos".)

Edit en `host/internal/cli/remotecmd.go`:

**Reemplazar:**
```go
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
```

**por:**
```go
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/device"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
```

Edit en `host/internal/cli/remotecmd.go`:

**Reemplazar:**
```go
// still enabled, never "off". Serve says whether `adbuddy serve` answered
// (otherwise it was read from config.toml).
type remoteInfo struct {
	Enabled   bool       `json:"enabled"`
	Closing   bool       `json:"closing,omitempty"`
	Address   string     `json:"address,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	Serve     bool       `json:"serve"`
}

func infoOf(s remote.State, serve bool) remoteInfo {
	i := remoteInfo{Enabled: s.Enabled, Closing: s.Closing, Address: s.Address, Serve: serve}
```

**por:**
```go
// still enabled, never "off". Serve says whether `adbuddy serve` answered
// (otherwise it was read from config.toml). PersistPort is
// persist.adb.tcp.port when the last read found it naming a port (VB-41: adbd
// listens on it again when the phone restarts), and PersistUnknown says that
// read failed: an "off" with either is never a plain "closed".
type remoteInfo struct {
	Enabled        bool       `json:"enabled"`
	Closing        bool       `json:"closing,omitempty"`
	Address        string     `json:"address,omitempty"`
	ExpiresAt      *time.Time `json:"expiresAt,omitempty"`
	Serve          bool       `json:"serve"`
	PersistPort    string     `json:"persistPort,omitempty"`
	PersistUnknown bool       `json:"persistUnknown,omitempty"`
}

func infoOf(s remote.State, serve bool) remoteInfo {
	i := remoteInfo{Enabled: s.Enabled, Closing: s.Closing, Address: s.Address, Serve: serve, PersistPort: s.Persist, PersistUnknown: s.PersistUnknown}
```

Edit en `host/internal/cli/remotecmd.go`:

**Reemplazar:**
```go
	var until string
	if i.ExpiresAt != nil {
		until = i.ExpiresAt.Local().Format("15:04")
	}
	switch {
	case !i.Enabled && op == "off":
		return lang.T("remote.text.off_closed")
	case !i.Enabled:
		return sentence(lang.T("remote.text.off"))
```

**por:**
```go
	var until string
	if i.ExpiresAt != nil {
		until = i.ExpiresAt.Local().Format("15:04")
	}
	// VB-41: off with a persist port, or with persist unread, is not closed
	// for good.
	var persist string
	switch {
	case i.PersistPort != "":
		persist = lang.T("remote.text.persist", i.PersistPort)
	case i.PersistUnknown:
		persist = lang.T("remote.text.persist_unknown")
	}
	switch {
	case !i.Enabled && op == "off" && persist != "":
		return lang.T("remote.text.off_mode") + " " + persist
	case !i.Enabled && op == "off":
		return lang.T("remote.text.off_closed")
	case !i.Enabled && persist != "":
		return sentence(lang.T("remote.text.off")) + ". " + persist
	case !i.Enabled:
		return sentence(lang.T("remote.text.off"))
```

Edit en `host/internal/cli/remotecmd.go`:

**Reemplazar:**
```go
// remoteOffWithoutServe closes the port directly: `adb usb` on the phone, and
// only when it worked the expiry is cleared (if it fails the mode stays on).
// There is no serve to hold config.toml, so this is its one writer now.
```

**por:**
```go
// remoteOffWithoutServe closes the port directly: `adb usb` on the phone, and
// only when it worked the expiry is cleared (if it fails the mode stays on).
// There is no serve to hold config.toml, so this is its one writer now. It
// reads persist.adb.tcp.port first (VB-41: `adb usb` does not clear it, and
// right after it adbd restarts) and says what it holds.
```

Edit en `host/internal/cli/remotecmd.go`:

**Reemplazar:**
```go
	octx, cancel := context.WithTimeout(ctx, remoteOpTimeout)
	defer cancel()
	var out, errb strings.Builder
	code, err := runner.Run(octx, "", []string{"usb"}, nil, &out, &errb)
```

**por:**
```go
	octx, cancel := context.WithTimeout(ctx, remoteOpTimeout)
	defer cancel()
	// Its own short deadline: a hung read must not eat the budget of adb usb.
	rctx, rcancel := context.WithTimeout(ctx, device.ReadTCPWait)
	t, rerr := device.ReadTCP(rctx, runner)
	rcancel()
	var out, errb strings.Builder
	code, err := runner.Run(octx, "", []string{"usb"}, nil, &out, &errb)
```

Edit en `host/internal/cli/remotecmd.go`:

**Reemplazar:**
```go
		return remoteInfo{}, NewErrorT("CONFIG_UNWRITABLE", "err.remotecmd.config_unwritable", "err.remotecmd.config_unwritable.fix", err.Error(), e.dirs.ConfigFile())
	}
	return remoteInfo{}, nil
}
```

**por:**
```go
		return remoteInfo{}, NewErrorT("CONFIG_UNWRITABLE", "err.remotecmd.config_unwritable", "err.remotecmd.config_unwritable.fix", err.Error(), e.dirs.ConfigFile())
	}
	return remoteInfo{PersistPort: t.Persist, PersistUnknown: rerr != nil}, nil
}
```

Edit en `host/internal/cli/phone_remote.go`:

**Reemplazar:**
```go
	if j.on {
		st, err = r.ctl.Enable(ctx)
	} else {
		st, err = r.ctl.Disable(ctx)
	}
	now := r.refresh()
```

**por:**
```go
	if j.on {
		st, err = r.ctl.Enable(ctx)
	} else {
		st, err = r.ctl.Disable(ctx)
		if err == nil {
			// VB-41: an off that leaves a persist port says so in serve's log
			// (the app only shows the mode).
			tellPersist(r.log, st.Persist, st.PersistUnknown)
		}
	}
	now := r.refresh()
```

Edit en `host/internal/cli/phone_remote.go`:

**Reemplazar:**
```go
			if changed {
				tell(log, "phone_remote.remote_mode_expired_port_5555")
				link.refresh()
```

**por:**
```go
			if changed {
				tell(log, "phone_remote.remote_mode_expired_port_5555")
				tellPersist(log, s.Persist, s.PersistUnknown) // once: the mode is off now
				link.refresh()
```

`serve` le pasa la lectura del invariante al controlador (revisión M2: `remote status` y su `--json` dicen lo de persist desde el arranque) y al vigilante del móvil, que relee el puerto cuando el móvil vuelve (revisión I1).

Edit en `host/internal/cli/serve.go`:

**Reemplazar:**
```go
			// Host invariant (spec §3.3): no adb network port while remote mode is off.
			if err := applyInvariant(ctx, active.Runner(), deviceWait, app.Log(), "adbuddy serve", keep); err != nil {
				if ctx.Err() != nil {
					return Interrupted()
				}
				return err
			}
```

**por:**
```go
			// Host invariant (spec §3.3): no adb network port while remote mode is off.
			tcp, err := invariantTCP(ctx, active.Runner(), deviceWait, app.Log(), "adbuddy serve", keep)
			if err != nil {
				if ctx.Err() != nil {
					return Interrupted()
				}
				return err
			}
			// VB-41: `adbuddy remote status` says what persist.adb.tcp.port
			// holds from the start, not only after the first "off".
			ctl.Note(tcp)
```

Edit en `host/internal/cli/serve.go`:

**Reemplazar:**
```go
				Cfg: store.Config, RemoteOn: func() bool { return !store.Until().IsZero() }, Log: app.Log(),
				Every: watchEvery, Misses: watchMisses, TryWait: watchTryWait,
			})
```

**por:**
```go
				Cfg: store.Config, RemoteOn: func() bool { return !store.Until().IsZero() }, Log: app.Log(),
				Every: watchEvery, Misses: watchMisses, TryWait: watchTryWait,
				Port: tcp, Seen: ctl.Note, ReadWait: device.ReadTCPWait,
			})
```

Edit en `host/internal/cli/serve.go`:

**Reemplazar:**
```go
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
```

**por:**
```go
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/device"
```

Edit en `host/internal/cli/phone_watch.go`:

**Reemplazar:**
```go
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
```

**por:**
```go
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/device"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
```

Edit en `host/internal/cli/phone_watch.go`:

**Reemplazar:**
```go
	Every   time.Duration
	Misses  int
	TryWait time.Duration
}
```

**por:**
```go
	Every   time.Duration
	Misses  int
	TryWait time.Duration

	// Port is the last read of adbd's TCP port already said (serve's
	// invariant at its start) and Seen gets each new read (the remote
	// controller's Note); both optional. ReadWait bounds each read (zero:
	// device.ReadTCPWait, read when the watcher is made).
	Port     device.TCP
	Seen     func(device.TCP)
	ReadWait time.Duration
}
```

Edit en `host/internal/cli/phone_watch.go`:

**Reemplazar:**
```go
// its guid). It never opens or closes a port (no applyInvariant): that is the
// remote mode's job. It ends with ctx.
```

**por:**
```go
// its guid). It never opens or closes a port (no applyInvariant): that is the
// remote mode's job. When the phone comes back it reads the port again
// (recheckPort: the phone may have restarted and taken persist.adb.tcp.port,
// VB-41) and warns. It ends with ctx.
```

Edit en `host/internal/cli/phone_watch.go`:

**Reemplazar:**
```go
	warned   bool            // "not reachable" was said and the phone has not come back
	wrong    map[string]bool // addresses said to be another phone since the phone was last listed
}
```

**por:**
```go
	warned   bool            // "not reachable" was said and the phone has not come back
	wrong    map[string]bool // addresses said to be another phone since the phone was last listed
	readWait time.Duration
	said     device.TCP // the last read of the port that was said
	unknown  bool       // "could not read persist" was said and no read worked since
}
```

Edit en `host/internal/cli/phone_watch.go`:

**Reemplazar:**
```go
	wt := &watcher{phoneWatch: w, every: w.Every, misses: w.Misses, tryWait: w.TryWait, wrong: map[string]bool{}}
```

**por:**
```go
	wt := &watcher{phoneWatch: w, every: w.Every, misses: w.Misses, tryWait: w.TryWait, wrong: map[string]bool{}, readWait: w.ReadWait, said: w.Port}
	if wt.readWait <= 0 {
		wt.readWait = device.ReadTCPWait
	}
```

Edit en `host/internal/cli/phone_watch.go`:

**Reemplazar:**
```go
	if hasSerial(online, w.Active.Serial()) {
		if w.warned {
			tell(w.Log, "phone_watch.the_phone_is_reachable_again")
		}
		w.count, w.warned = 0, false
		clear(w.wrong)
		return
	}
```

**por:**
```go
	if hasSerial(online, w.Active.Serial()) {
		back := w.count > 0 || w.warned // it was missing at least once
		if w.warned {
			tell(w.Log, "phone_watch.the_phone_is_reachable_again")
		}
		w.count, w.warned = 0, false
		clear(w.wrong)
		if back {
			w.recheckPort(ctx)
		}
		return
	}
```

Edit en `host/internal/cli/phone_watch.go`:

**Reemplazar:**
```go
		if err == nil && verified {
			w.switchTo(d.Serial, d.Serial)
			return
		}
```

**por:**
```go
		if err == nil && verified {
			w.switchTo(ctx, d.Serial, d.Serial)
			return
		}
```

Edit en `host/internal/cli/phone_watch.go`:

**Reemplazar:**
```go
		if err == nil && verified {
			w.switchTo(active, addr)
			return
		}
```

**por:**
```go
		if err == nil && verified {
			w.switchTo(ctx, active, addr)
			return
		}
```

Edit en `host/internal/cli/phone_watch.go`:

**Reemplazar:**
```go
// switchTo makes serial the active phone. Only called with a verified one.
func (w *watcher) switchTo(serial, where string) {
	w.Active.Switch(serial)
	w.count, w.warned = 0, false
	clear(w.wrong)
	tell(w.Log, "phone_watch.the_phone_is_now_at", where)
}
```

**por:**
```go
// switchTo makes serial the active phone. Only called with a verified one.
func (w *watcher) switchTo(ctx context.Context, serial, where string) {
	w.Active.Switch(serial)
	w.count, w.warned = 0, false
	clear(w.wrong)
	tell(w.Log, "phone_watch.the_phone_is_now_at", where)
	w.recheckPort(ctx)
}

// recheckPort reads adbd's TCP port again once the phone is back (VB-41: it
// may have restarted and taken persist.adb.tcp.port, which serve's invariant
// only reads when serve starts). It never opens or closes a port: `adb usb`
// in the middle of the session would cut the phone again and race with the
// remote mode. It says what changed since the last time it was said, once: a
// port open with the remote mode off (run `adbuddy remote off`), the persist
// port, or a read that failed.
func (w *watcher) recheckPort(ctx context.Context) {
	rctx, cancel := context.WithTimeout(ctx, w.readWait)
	t, err := device.ReadTCP(rctx, w.Active.Runner())
	cancel()
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		if !w.unknown {
			w.unknown = true
			tellPersist(w.Log, "", true)
		}
		return
	}
	w.unknown = false
	if w.Seen != nil {
		w.Seen(t)
	}
	if t == w.said {
		return
	}
	w.said = t
	// RemoteOn is read after the port: Enable saves the expiry before
	// `adb tcpip`, so a 5555 it opened is seen with the mode on.
	if t.Now != "" && !(t.Now == remote.Port && w.RemoteOn()) {
		tell(w.Log, "phone_watch.port_open", t.Now)
	}
	tellPersist(w.Log, t.Persist, false)
}
```

El test que vigila que el vigilante no toca el puerto admite ahora leerlo (solo `getprop`), nunca abrirlo ni cerrarlo:

Edit en `host/internal/cli/phone_watch_test.go`:

**Reemplazar:**
```go
// The watcher never opens or closes a port.
func TestWatchPhoneNeverTouchesThePort(t *testing.T) {
	r := newRig(t, devicesRule(tsAddr), connectRule(tsAddr), serialnoIs("AAAA1111"))
	w, _, _ := watcherFor(t, r, tsConfig(), true, phoneSerial)
	steps(w, 6)
	for _, c := range r.calls(t) {
		if strings.HasPrefix(c, "tcpip") || strings.Contains(c, "adb.tcp.port") || strings.Contains(c, "setprop") {
			t.Fatalf("the watcher touched the port: %q", c)
		}
	}
}
```

**por:**
```go
// The watcher never opens or closes a port: it only reads it (getprop) when
// the phone comes back (VB-41).
func TestWatchPhoneNeverTouchesThePort(t *testing.T) {
	r := newRig(t, devicesRule(tsAddr), connectRule(tsAddr), serialnoIs("AAAA1111"))
	w, _, _ := watcherFor(t, r, tsConfig(), true, phoneSerial)
	steps(w, 6)
	read := 0
	for _, c := range r.calls(t) {
		if strings.HasPrefix(c, "tcpip") || c == "usb" || strings.Contains(c, "setprop") {
			t.Fatalf("the watcher touched the port: %q", c)
		}
		if strings.HasPrefix(c, "shell getprop") && strings.Contains(c, "adb.tcp.port") {
			read++
		}
	}
	if read == 0 {
		t.Fatalf("the port was not read after the switch: %q", r.calls(t))
	}
}
```

Run: `cd host && gofmt -l . && go vet ./internal/cli/ && go test -count=1 -timeout 10m -run 'TestApplyInvariantWarnsAboutAPersistPort|TestDoctorNetworkPortPersist|TestRemoteSetOffSaysThePersistPort|TestWatchRemoteSaysThePersistPortWhenItExpires|TestRemoteOffThroughServeSaysThePersistPort|TestRemoteOffWithoutServeSaysThePersistPort|TestDoctorInSpanish|TestWatchPhoneRereadsThePortWhenThePhoneComesBack|TestDoctorNetworkPortOpenWithServeRunning|TestRemoteOffWithoutServePersistUnreadable|TestWatchPhoneNeverTouchesThePort' ./internal/cli/`
Expected: `ok`. Mutación comprobada: sin la línea `tellPersist(log, s.Persist, s.PersistUnknown)` de `watchRemote`, `TestWatchRemoteSaysThePersistPortWhenItExpires` falla; restaurada, `ok`.

- [ ] **Step 5: el español de `adbuddy remote` con persist.**

Edit en `host/internal/cli/messages_lang_test.go`:

**Reemplazar:**
```go
		"off without serve": {Serve: false},
	}
```

**por:**
```go
		"off without serve": {Serve: false},
		"off with persist":  {Serve: true, PersistPort: "5555"},
		"persist unknown":   {Serve: true, PersistUnknown: true},
	}
```

Edit en `host/internal/cli/messages_lang_test.go`:

**Reemplazar:**
```go
	if got := remoteTextIn(i18n.ES, "off", infos["off"]); got != "El modo remoto está desactivado: el puerto 5555 está cerrado." {
		t.Errorf("off: %q", got)
	}
```

**por:**
```go
	if got := remoteTextIn(i18n.ES, "off", infos["off"]); got != "El modo remoto está desactivado: el puerto 5555 está cerrado." {
		t.Errorf("off: %q", got)
	}
	// VB-41: off with a persist port never says "cerrado" alone.
	if got := remoteTextIn(i18n.ES, "off", infos["off with persist"]); got != "El modo remoto está desactivado. Pero el puerto de red de adb no queda cerrado del todo: persist.adb.tcp.port vale 5555 (ADBuddy no la fijó), y adb vuelve a escuchar en ese puerto cada vez que el móvil se reinicia. `adbuddy doctor` explica cómo quitarla." {
		t.Errorf("off with persist: %q", got)
	}
	if got := remoteTextIn(i18n.ES, "status", infos["persist unknown"]); !strings.HasPrefix(got, "Modo remoto: desactivado. Pero no se pudo leer persist.adb.tcp.port") {
		t.Errorf("status, persist unknown: %q", got)
	}
```

(Los dos casos nuevos entran también en el bucle de `TestRemoteTextInSpanish`: sin inglés en el español, mayúscula inicial con serve, y "5555" y "adbuddy remote off" en los dos idiomas o en ninguno.)

Revisión M5, el inglés exacto:

Añadir al final de `host/internal/cli/remotecmd_test.go`:

```go

// VB-41 (review M5): the English of off and status with a persist port, and
// with persist unread.
func TestRemoteTextWithPersist(t *testing.T) {
	persist := remoteInfo{Serve: true, PersistPort: "5555"}
	const warning = "But adb's network port is not closed for good: persist.adb.tcp.port is 5555 (ADBuddy did not set it), and adb listens on that port again every time the phone restarts. `adbuddy doctor` says how to remove it."
	if got := remoteText("off", persist); got != "Remote mode is off. "+warning {
		t.Errorf("off: %q", got)
	}
	if got := remoteText("status", persist); got != "Remote mode: off. "+warning {
		t.Errorf("status: %q", got)
	}
	if got := remoteText("status", remoteInfo{PersistPort: "5555"}); got != "serve is not running; remote mode: off. "+warning {
		t.Errorf("status without serve: %q", got)
	}
	if got := remoteText("off", remoteInfo{Serve: true, PersistUnknown: true}); got != "Remote mode is off. But persist.adb.tcp.port could not be read: adb's network port may open again when the phone restarts. Run `adbuddy doctor` to check." {
		t.Errorf("off, unknown: %q", got)
	}
}
```

- [ ] **Step 6: suite completa y commit.**

Run: `cd host && go build ./... && go vet ./... && gofmt -l . && go test -count=1 -timeout 15m ./...`
Expected: sin salida de `gofmt`; todos los paquetes con tests en `ok` (entre ellos `internal/i18n`: paridad y claves usadas; `internal/cli`: `TestNoTextLeftInTheCode`, `TestDoctorInSpanish`, `TestRemoteTextInSpanish`).

```bash
git add host/internal/cli host/internal/i18n/catalog
git commit -m "feat(cli): avisar de persist.adb.tcp.port en doctor, serve y adbuddy remote (VB-41)

Un puerto de persist nunca sale como cerrado: doctor lo marca con qué
propiedad es, que no la fijó ADBuddy y cómo quitarla; serve lo dice una vez
al arrancar, en cada off, al caducar y cuando el móvil vuelve (el vigilante
relee el puerto, sin adb usb); remote off/status lo dice desde el arranque
(Controller.Note) y --json lleva persistPort/persistUnknown. DEVICE_PORT_OPEN
por persist no manda reiniciar el móvil.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: la spec, troubleshooting, los pendientes y el inventario dicen lo que hace el host con persist

**Files:** los cinco de `docs/` del mapa. Todos en CRLF en el árbol de trabajo: anclas de una sola línea.

- [ ] **Step 1: spec v1 §3.3.**

Edit en `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md`:

**Reemplazar:**
```markdown
  - **Al desactivarlo,** el host ejecuta `adb usb`: el puerto se cierra (`service.adb.tcp.port=0`) y se conserva la depuración inalámbrica.
```

**por:**
```markdown
  - **Al desactivarlo,** el host ejecuta `adb usb`: el puerto se cierra (`service.adb.tcp.port=0`) y se conserva la depuración inalámbrica. `adb usb` no borra `persist.adb.tcp.port` (medido en la T8, VB-41): si otra herramienta la fijó, adbd vuelve a escuchar en ese puerto tras reiniciar el móvil. El host la lee siempre junto a `service.adb.tcp.port`, nunca da ese puerto por cerrado y lo avisa (doctor, terminal de `serve` al arrancar, en cada apagado y cada vez que el móvil vuelve tras perderse, y `adbuddy remote off|status`); sin root no puede borrarla y no usa `su` (plan `2026-10-04-adbuddy-vb41-persist-port`).
```

Edit en `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md`:

**Reemplazar:**
```markdown
  - **Invariante del host:** al conectar, si `getprop service.adb.tcp.port` no está vacío ni es `0` y el modo remoto no está activado, el host ejecuta `adb usb`, que reinicia adbd y corta la conexión un instante (ver "Reactivar" en §4.6).
```

**por:**
```markdown
  - **Invariante del host:** al conectar, si el puerto en el que escucha adbd (`service.adb.tcp.port`, o `persist.adb.tcp.port` cuando aquel está vacío, como tras un reinicio: así arranca adbd en AOSP) no está vacío ni es `0` y el modo remoto no está activado, el host ejecuta `adb usb`, que reinicia adbd y corta la conexión un instante (ver "Reactivar" en §4.6). Con `service.adb.tcp.port` a `0` y persist fijado no hay nada que cerrar hasta el siguiente reinicio (suposición sin medir del todo, ver el plan): solo se avisa (así no hay bucle de `adb usb`). Una lectura que falla es «desconocido», nunca «cerrado». Si el móvil se reinicia con `serve` en marcha, al volver `serve` relee el puerto y avisa, pero no lo cierra: lo cierra `adbuddy remote off` o el siguiente arranque de `serve`.
```

- [ ] **Step 2: spec M5 §7.2.**

Edit en `docs/superpowers/specs/2026-10-01-adbuddy-m5-ayudas-remoto-design.md`:

**Reemplazar:**
```markdown
solo lo borra si `getprop service.adb.tcp.port` confirma que el puerto no está abierto
```

**por:**
```markdown
solo lo borra si la lectura del puerto (`service.adb.tcp.port`, o `persist.adb.tcp.port` si aquel está vacío: VB-41) confirma que el 5555 no está abierto
```

Edit en `docs/superpowers/specs/2026-10-01-adbuddy-m5-ayudas-remoto-design.md`:

**Reemplazar:**
```markdown
- **`remote.set {enabled: false}`,** al caducar `remote_until` o con `adbuddy remote off`: el host ejecuta `adb usb` y borra `remote_until`.
```

**por:**
```markdown
- **`remote.set {enabled: false}`,** al caducar `remote_until` o con `adbuddy remote off`: el host ejecuta `adb usb` y borra `remote_until`. Antes lee el puerto, `persist.adb.tcp.port` incluida (VB-41, plan `2026-10-04-adbuddy-vb41-persist-port`): `adb usb` no la borra, así que si nombra un puerto el estado lo dice (`adbuddy remote off|status` y su `--json`, `persistPort`; si no se pudo leer, `persistUnknown`) y la terminal de `serve` lo avisa una vez; nunca sale un «cerrado» a secas. Con el modo apagado y el puerto cerrado ahora (también con persist fijado y `service.adb.tcp.port` a `0`) no se ejecuta `adb usb`. `Reconcile` toma por abierto el 5555 de persist tras un reinicio: el modo sigue encendido hasta caducar. La tarjeta de la app no lleva este aviso (`remote.state` no cambia; pendiente).
```

Edit en `docs/superpowers/specs/2026-10-01-adbuddy-m5-ayudas-remoto-design.md`:

**Reemplazar:**
```markdown
- **Invariante** (`invariant.go`): con `remote_until` en el futuro, el puerto abierto es el esperado y `applyInvariant` no ejecuta `adb usb`. Sin `remote_until`, o ya pasado, se comporta como hoy.
```

**por:**
```markdown
- **Invariante** (`invariant.go`): con `remote_until` en el futuro, el puerto abierto es el esperado y `applyInvariant` no ejecuta `adb usb`. Sin `remote_until`, o ya pasado, se comporta como hoy. El puerto es el que lee `device.ReadTCP` (service, o persist si aquel está vacío); persist fijado se avisa en la terminal de `serve`, y si el puerto sigue abierto tras `adb usb` por persist, `DEVICE_PORT_OPEN` no manda reiniciar el móvil (eso lo volvería a abrir). El invariante solo corre al arrancar `serve`: cuando el móvil vuelve a mitad de sesión, el vigilante relee el puerto y avisa (puerto abierto con el modo apagado, persist), sin `adb usb`.
```

- [ ] **Step 3: `docs/troubleshooting.md`.**

Edit en `docs/troubleshooting.md`:

**Reemplazar:**
```markdown
7. **network port**: adb's network port on the phone (the `tcpip` mode) is closed, or says why it is open.
```

**por:**
```markdown
7. **network port**: adb's network port on the phone (the `tcpip` mode) is closed, or says why it is open. It reads `persist.adb.tcp.port` too: when that property names a port, the port is not closed for good (see [Doctor says `persist.adb.tcp.port` is set](#doctor-says-persistadbtcpport-is-set)).
```

Edit en `docs/troubleshooting.md`:

**Reemplazar:**
```markdown
| `DEVICE_PORT_OPEN` | 1 | adb's network port on the phone is still open after `adb usb` (`adbuddy serve` and `adbuddy setup`). | Restart the phone (that closes a port opened with `adb tcpip`), turn Wireless debugging on again and run the command again. |
```

**por:**
```markdown
| `DEVICE_PORT_OPEN` | 1 | adb's network port on the phone is still open after `adb usb` (`adbuddy serve` and `adbuddy setup`). | Restart the phone (that closes a port opened with `adb tcpip`), turn Wireless debugging on again and run the command again. When the message names `persist.adb.tcp.port`, a restart opens it again: see [Doctor says `persist.adb.tcp.port` is set](#doctor-says-persistadbtcpport-is-set). |
```

Edit en `docs/troubleshooting.md`:

**Reemplazar:**
```markdown
### "Wrong password" and `protocol fault` when pairing
```

**por:**
```markdown
### Doctor says `persist.adb.tcp.port` is set

Some ROMs (a setting often called "ADB over network"), root apps or a command run before ADBuddy set the property `persist.adb.tcp.port`. While it names a port, adb listens on that port, on every network, each time the phone restarts. ADBuddy never sets it and cannot clear it without root: `adb usb`, which `adbuddy serve` runs when it starts and `adbuddy remote off` runs, only closes the port until the next restart. If the phone restarts while serve runs, serve only warns when the phone comes back ("adb listens on network port … again"): run `adbuddy remote off` to close the port. ADBuddy says so in `adbuddy doctor`, in the serve terminal and in `adbuddy remote off` and `adbuddy remote status` (`persistPort` in `--json`), and never calls that port closed.

To remove it, turn off the setting or app that set it. With root, empty it as root (`setprop persist.adb.tcp.port ''`) and restart the phone. Then run `adbuddy doctor` again.

### "Wrong password" and `protocol fault` when pairing
```

- [ ] **Step 4: pendientes.**

Edit en `docs/superpowers/plans/pendientes-m3-m5.md`:

**Reemplazar:**
```markdown
  - T2: `Disable` solo mira `service.adb.tcp.port`, no `persist.adb.tcp.port`
```

**por:**
```markdown
  - **Resuelto (VB-41, rama `fix/vb41-persist-tcp-port`, plan `2026-10-04-adbuddy-vb41-persist-port`): el host lee también `persist.adb.tcp.port` y avisa; ver la sección "VB-41" al final.** T2: `Disable` solo mira `service.adb.tcp.port`, no `persist.adb.tcp.port`
```

Edit en `docs/superpowers/plans/pendientes-m3-m5.md`:

**Reemplazar:**
```markdown
- Validaciones en dispositivo 44 y 45: en "Validaciones manuales pendientes" del plan.
```

**por:**
```markdown
- Validaciones en dispositivo 44 y 45: en "Validaciones manuales pendientes" del plan.

## VB-41 · `persist.adb.tcp.port` (plan `docs/superpowers/plans/2026-10-04-adbuddy-vb41-persist-port.md`)

- **HECHO:** `device.ReadTCP` lee `service.adb.tcp.port` y `persist.adb.tcp.port`; el puerto de adbd es service, o persist si aquel está vacío; una lectura fallida es desconocido. El invariante, `Disable`, `Reconcile`, `Enable` y `Expire` deciden con él; el host avisa en doctor, en la terminal de `serve` (al arrancar, en cada apagado, al caducar y cuando el móvil vuelve: el vigilante relee el puerto, sin `adb usb`) y en `adbuddy remote off|status` (`persistPort`/`persistUnknown` en `--json`, desde el arranque de `serve`). No ejecuta `su` ni intenta borrar persist.
- **Medido en el emulador (ya no bloquea la integración):** `shell` lee persist sin `su` (5555, código 0) y `TestOffClosesAPersistTCPPort` y `TestPersistTCPPortAgainstUSBAndRestarts` pasan con `ADBUDDY_EMULATOR_REBOOT=1`. Pendiente, con permiso del usuario y solo lectura: `adb shell getprop persist.adb.tcp.port` en el Pixel. `su 0 setprop service.adb.tcp.port 0` sin reiniciar NO reinicia adbd: el ruling 3 solo vale con el `0` de `adb usb`.
- **Diferido (protocolo, ok de adbuddy-e5):** la tarjeta del modo remoto no dice nada de persist; `remote.state` podría llevar `persistPort` para que la app lo avise.
- **Diferido:** `adbuddy remote off` sin serve y con el modo ya apagado no lee el puerto (ruling 7 del plan: no afirma «cerrado» y leer costaría arrancar el servidor y esperar al móvil).
- **Diferido:** con el modo remoto encendido y persist fijado, doctor da la guía del modo (lo urgente) y no la de persist; el aviso llega al apagarlo.
- **Límite conocido:** el host lee propiedades, no sockets. No mira `service.adb.listen_addrs` (AOSP adbd también escucha donde diga) ni el 5555 que adbd abre por defecto en un dispositivo sin USB; un adbd que no siga AOSP (escuchar en persist con service a `0`) quedaría abierto, avisado pero sin cerrar. Con `serve` en marcha, un puerto que reaparece al volver el móvil se avisa pero no se cierra (ruling 9 del plan).
```

- [ ] **Step 5: inventario, fila VB-41.**

Edit en `docs/superpowers/plans/2026-10-03-validaciones-manuales.md`:

**Reemplazar:**
```markdown
| Hallazgo confirmado solo en el estado de las propiedades (persist no se limpia; `service.adb.tcp.port` vacío tras el reinicio); que adbd escuche en el puerto de `persist` tras el reinicio sale de leer AOSP, no se midió. Fijar `persist` como shell falla (exit 1, medido): el riesgo real son móviles con root o con `persist` ya fijado. El arreglo va en rama aparte, con verificación y aviso en doctor/estado, pendiente de plan. | medio | T8 (medida); hallazgo pendiente de arreglar |
```

**por:**
```markdown
| Hallazgo confirmado solo en el estado de las propiedades (persist no se limpia; `service.adb.tcp.port` vacío tras el reinicio); que adbd escuche en el puerto de `persist` tras el reinicio sale de leer AOSP, no se midió. Fijar `persist` como shell falla (exit 1, medido): el riesgo real son móviles con root o con `persist` ya fijado. **Arreglo:** rama `fix/vb41-persist-tcp-port` (plan `2026-10-04-adbuddy-vb41-persist-port`): el host lee persist, nunca lo da por cerrado y avisa (doctor, `serve`, `remote off|status`); `TestOffClosesAPersistTCPPort` redefinido (ruling 4: persist reportado, no vaciado). **Medido en el emulador (2026-10-03):** pasan `TestOffClosesAPersistTCPPort` y `TestPersistTCPPortAgainstUSBAndRestarts` con `ADBUDDY_EMULATOR_REBOOT=1`; `shell` lee persist sin `su` (5555, código 0); `su 0 setprop service.adb.tcp.port 0` sin reiniciar no reinicia adbd (el «cerrado por ahora» solo vale con el `0` de `adb usb`). Pendiente en el Pixel: lectura de solo lectura `adb shell getprop persist.adb.tcp.port`, con permiso (la política SELinux puede ser otra). | medio | T8 (medida); arreglo en rama, medido en emulador; lectura en el Pixel pendiente (no bloquea) |
```

- [ ] **Step 6: comprobar y commit.**

Run (desde la raíz del worktree): `git diff --check -- docs/` y `file docs/troubleshooting.md docs/superpowers/plans/pendientes-m3-m5.md docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md docs/superpowers/specs/2026-10-01-adbuddy-m5-ayudas-remoto-design.md docs/superpowers/plans/2026-10-03-validaciones-manuales.md`; después, en `host/`, `go test -count=1 -timeout 10m -run Doc ./internal/cli/` (`TestDocsAreGuarded` revisa enlaces, anclas, órdenes y privacidad de `docs/troubleshooting.md`).
Expected: `git diff --check` sin salida; los cinco ficheros `with CRLF line terminators`; `ok`.

```bash
git add docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md docs/superpowers/specs/2026-10-01-adbuddy-m5-ayudas-remoto-design.md docs/troubleshooting.md docs/superpowers/plans/pendientes-m3-m5.md docs/superpowers/plans/2026-10-03-validaciones-manuales.md
git commit -m "docs: persist.adb.tcp.port en la spec, troubleshooting, pendientes e inventario (VB-41)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

## Validaciones pendientes (emulador y móvil)

1. **Emulador (MEDIDO el 2026-10-03, API 34: (a) `shell` lee persist sin `su`, 5555 con código 0; (b)-(d) pasan con `ADBUDDY_EMULATOR_REBOOT=1`; (e) `su 0 setprop service.adb.tcp.port 0` sin reiniciar NO reinicia adbd (mismo pid 6 s después), así que el «cerrado por ahora» solo vale con el `0` de `adb usb`; nunca en el móvil del usuario):** `TestOffClosesAPersistTCPPort` y `TestPersistTCPPortAgainstUSBAndRestarts` con `ADBUDDY_EMULATOR_SERIAL`, `ADBUDDY_EMULATOR_ADBD=1` y, para el paso de reinicio, `ADBUDDY_EMULATOR_REBOOT=1` (arranque y parada con `scripts/emulator.sh`, ver `host/README.md`). Miden: (a) que el host lee persist **como `shell`** (`State{Persist: "5555"}`, `TCP{Persist: "5555"}`); si saliera vacío, el diseño no sirve tal cual (ruling 4); (b) que tras el "off" adbd queda con service `0` y otro pid; (c) que una segunda pasada no reinicia adbd; (d) con el reinicio, que `Disable` cierra el 5555 de persist; (e) a mano en el emulador, `adb shell su 0 setprop service.adb.tcp.port 0` sin `ctl.restart`: ¿cambia el pid de adbd? (suposición del ruling 3; si no cambia, un service a `0` puesto así no prueba que adbd no escuche en persist, y el texto «cerrado por ahora» debería suavizarse). **(a) ya no bloquea la integración** (revisión I2); si en el Pixel la lectura diera vacío con código 0 se reabriría.
2. **Móvil del usuario, solo lectura y con su permiso explícito (pendiente, no bloquea):** `adb shell getprop -Z service.adb.tcp.port` y `adb shell getprop -Z persist.adb.tcp.port` (contexto SELinux de cada propiedad) y `adb shell getprop persist.adb.tcp.port`. Si los dos contextos son el mismo, `shell` lee persist como lee service (medido en S1); un valor vacío por sí solo no distingue "sin fijar" de "vetado".

## Réplica acumulada tarea a tarea

**v2 (tras la revisión opus), desde el texto del plan** con un script que interpreta las cabeceras "Edit en `…`" (con o sin `replace_all` y su número de apariciones) y "Añadir al final de `…`", aplica cada bloque en orden y aborta si un ancla no aparece exactamente las veces dichas (los ficheros CRLF se editan normalizando y se guardan en CRLF). Sobre `git archive b5a32a6` completo (la cabeza de la rama: código de 8f881bf más el plan v1; los tests de `cli` y `bridge` leen `protocol/` y `docs/`), sin emulador, por etapas:

- **Tarea 1, paso 1 (9 ediciones, RED):** `vet` limpio; `device` → `FAIL` con 2 tests (`TestEnsureNoTCPPortReadsPersist`, 5 casos; `TestEnsureNoTCPPortPersistUnreadable`, 3 casos), `remote` → `FAIL` con 5 tests, los citados en el paso 2; ninguno por compilación ni `panic`.
- **Resto de la Tarea 1 (49 ediciones, GREEN):** `go build ./... && go vet ./... && gofmt -l .` limpios; suite completa: los 34 paquetes con tests en `ok` (`cli 196.030s`, máquina cargada), `EXIT 0`. Mutaciones del paso 4 (en la implementación a mano, restauradas): sin la línea de persist en `ReadTCP`, 8 `--- FAIL`; con `c.persist = ""` fijo, 3; sin el plazo propio de `readTCP`, `TestReadTCPHasItsOwnWait` falla a los 10 s.
- **Tarea 2, paso 1 (12 ediciones, RED):** 19 `--- FAIL` (10 tests y 9 subtests), los citados en el paso 2, sin errores de compilación ni `panic`.
- **Resto de la Tarea 2 (38 ediciones, GREEN):** limpios; el `-run` del paso 4 más `TestRemoteTextWithPersist` y `TestRemoteTextInSpanish` → `ok`; suite completa: 34 en `ok` (`cli 129.220s`), `EXIT 0`. Mutaciones: sin `recheckPort` al volver el móvil, `TestWatchPhoneRereadsThePortWhenThePhoneComesBack` falla (los dos subtests); sin `tellPersist` en `watchRemote`, `TestWatchRemoteSaysThePersistPortWhenItExpires` falla (v1).
- **Tarea 3 (11 ediciones):** los cinco ficheros siguen en CRLF; suite completa final: 34 en `ok` (`cli 114.159s`), `EXIT 0` (incluye `TestDocsAreGuarded`: enlaces, anclas, órdenes y privacidad de `docs/troubleshooting.md`).

**v1 (antes de la revisión):** implementada a mano sobre `git archive 8f881bf` y reaplicada desde el texto; suite en `ok` en las tres tareas, salvo un fallo suelto de `internal/ipc` (`TestDisconnectCancelsHandlerThatIgnoresStdin`, paquete que el plan no toca; 3 de 3 `ok` al repetirlo solo: flake con la máquina cargada). Línea base de 8f881bf: 34 paquetes en `ok`.

## Autorrevisión del plan

### Cobertura frente al encargo

- Criterio 1 (nunca «cerrado» sin leer): tabla "Revisión de sitios", con cada sitio y su cambio (también el móvil que vuelve con `serve` en marcha, revisión I1, y la lectura del invariante que llega al controlador, revisión M2); los dos que no cambian, con su ruling (6 y 7).
- Criterio 2 (tras off, leer persist, `adb usb` si hay algo que cerrar, avisar en doctor y en estado/log, sin `su`): Tarea 1 (`ReadTCP`, `EnsureNoTCPPort`, `Disable`), Tarea 2 (doctor, log, `remote`). Desviación explicada en el ruling 3: con service a `0`, persist no lanza `adb usb` (solo aviso).
- Criterio 3 (sin bucles, `adb usb` solo con algo que cerrar, avisar una vez): rulings 3 y 8; `TestPersistDoesNotRestartAdbdAgain`, `TestEnsureNoTCPPortReadsPersist` ("service at 0"), `TestApplyInvariantWarnsAboutAPersistPort` (aviso una vez), `TestWatchRemoteSaysThePersistPortWhenItExpires` (una vez con más ticks), y en emulador la segunda pasada de `EnsureNoTCPPort`.
- Criterio 4 (aceptación redefinida, RED con adb falso en `remote` y `device`): ruling 4; RED reales en los pasos 1-2 de las dos tareas. Casos adversos: persist raro (`abc`), enorme, negativo, `0`, 5555, otro puerto, lectura que falla (service y persist), `adb usb` que falla (`TestUsbFailsWithAPersistPort`, `TestEnsureNoTCPPortEdgeCases`), persist que sigue tras usb (`still open after adb usb`, `TestPersistDoesNotRestartAdbdAgain`), shell sin root (el host no escribe persist: la aceptación lo exige), lecturas contradictorias (service vacío y persist fijado; primera lectura abierta y segunda cerrada en "after a reboot"; lectura nueva que reemplaza a la vieja en `TestStateKeepsTheLastReadOfPersist`).
- Criterio 5 (tareas pequeñas que compilan solas): Tarea 1 incluye los ajustes mecánicos de `cli` y `emutest` por la firma nueva (sin textos); Tarea 2 los textos; Tarea 3 los docs.

### Placeholders y tipos

Sin placeholders. `remote.ADB` y `device.ADB` tienen el mismo método `Run`: `device.ReadTCP(ctx, c.adb)` compila sin conversión. `device.TCP` es comparable (dos `string`): los tests lo comparan con `!=`. `remote.State` sigue comparable (añade un `string` y un `bool`). `tellPersist` vive en `invariant.go` y la usan `invariant.go`, `phone_remote.go` y `phone_watch.go` (mismo paquete). `ctl.Note` tiene la firma de `phoneWatch.Seen` (`func(device.TCP)`).

### Riesgos abiertos para el revisor

- **Premisa (medida en el emulador; en el Pixel pendiente de lectura):** que `shell` pueda leer `persist.adb.tcp.port` se midió en el emulador API 34 (ruling 4). En AOSP las dos propiedades comparten contexto SELinux (`adbd_config_prop`, leído de memoria, no verificado aquí) y `service.adb.tcp.port` sí se lee como `shell` (S1). Si no, un `getprop` vetado da vacío con código 0 y el host diría "sin persist": el fallo sería el de hoy, no uno nuevo. Validaciones 1 y 2.
- **Lectura antes de `adb usb` en el controlador** (ruling 5): una carrera entre la lectura y el `adb usb` daría un valor viejo hasta la siguiente lectura.
- **Un `getprop` más por decisión:** `Disable`, `Expire` y `Reconcile` hacen dos `adb shell` donde antes hacían uno o ninguno (unos 100 ms cada uno; con su propio plazo, `device.ReadTCPWait`, 10 s, dentro de `remoteOpTimeout`). El invariante, dos más al arrancar `serve`; el vigilante, dos cada vez que el móvil vuelve.
- **Doctor con el modo encendido y persist fijado:** se queda con la guía del modo (pendientes).
- **El vigilante avisa, no cierra** (ruling 9): con `serve` en marcha, un puerto que reaparece al volver el móvil queda abierto hasta `adbuddy remote off` o el siguiente arranque de `serve`. El vigilante solo relee cuando el móvil faltó al menos una vez (10 s) o cambió de serial; un reinicio del móvil siempre lo hace.
- **Ruling 3 descansa en suposiciones** (revisión I2): la validación 1 (e) midió que un `0` puesto a mano sin reiniciar no reinicia adbd; solo vale el `0` de `adb usb` (límite en pendientes).
- **Texto "adb listens on port X again":** sale de leer AOSP (el emulador no deja ver el listener real); el texto lo afirma como consecuencia esperada, igual que la spec.

### Decisiones y dudas para el controlador

- El encargo cita la §7.1 de la spec M5; el plan edita la §7.2, que es donde están `adb usb`, la caducidad y el invariante (la §7.1 trata el puerto de la depuración inalámbrica).
- Revisión opus (v2): I1 arreglado aquí (ruling 9, sin `adb usb` a mitad de sesión: solo relectura y aviso); I2 como suposición con validación y límite; M1-M5 aplicados. La lectura de persist como `shell` se midió en el emulador (ya no bloquea); queda la lectura de solo lectura en el Pixel.
- `docs/usage.md` y `docs/safety.md` no se tocan: troubleshooting tiene la sección nueva y doctor/`remote` remiten a ella; si el controlador quiere una línea en `usage.md` ("Remote mode"), es una edición de una línea.
