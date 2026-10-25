# F1: tests de emulador inestables en runners de GitHub

Rama `fix/f1-emulador-inestable` (desde main aa7f3698). Compañero: ci.

## Evidencia de CI (primer fallo de cada log)

Runs de `emulator-app` del 2026-10-11 23:18Z (PRs), logs en el scratchpad.

| run | rama | resultado | primer fallo |
|---|---|---|---|
| 38183773545 | fix/m2-8-unlock-allow | failure | `TestTheListenerContent` (listener_test.go:650) |
| 38183773047 | perf/haiku-default | failure | `TestTheListenerContent`; además `TestScreenAfterOnTheEmulator/a_slow_transition...` |
| 38183772294 | perf/latencia | failure | `TestTheListenerContent` |
| 38183686997 | ci/setup-go-last | success | |
| 38183772339 | ux/p6b | success | |
| 38183773061 | chore/deps-agrupado | success | |

Los tres fallos de `TestTheListenerContent` son idénticos: tras `am force-stop com.android.shell`
y un `cmd notification list` inmediato siguen los cinco `t6fill1..5` de `com.android.shell`.
Duran 43.3-43.8 s en los tres. Ningún job cancelado ni cascada: el test falla solo.

## Hipótesis (por verificar en el emulador)

TestTheListenerContent: `am force-stop` devuelve antes de que NotificationManagerService
cancele las notificaciones del paquete (lo hace al recibir el broadcast de reinicio de
paquete, en su hilo de mensajes). El test lista justo al volver el comando y en un runner lento
la carrera se pierde.

Slow transition: ver más abajo.

## Causa raiz (verificada en el emulador local android-34, puerto 5604)

### TestTheListenerContent
`am force-stop com.android.shell` vuelve antes de que el servicio de notificaciones cancele las
del paquete (lo hace al tratar el broadcast de reinicio de paquete, en su propio hilo). Medido con
adb: filler -> force-stop -> `cmd notification list` seguido dentro del emulador (sin ida y vuelta
de adb) deja las 5 o 4 `t6fill` en 2 de 10 intentos; sondeando hasta 0 tarda 0,05-1,0 s (6 medidas:
1.01, 0.11, 0.31, 0.70, 0.05, 0.15 s). Con un `adb shell` aparte entre force-stop y list (lo que
hace el test) en Windows no se pierde nunca (0/25): el adb de Windows tarda mas que la carrera.
En Linux/GitHub el adb tarda unos pocos ms y el runner va mas cargado: se pierde (3 de 3 runs
fallidos en listener_test.go:650, y los tres identicos). Es del test, no del producto: el producto
no interviene en esta cancelacion. Arreglo: sondear hasta 30 s (`poll`) en vez de leer una vez.
Antes (bucle local x10 sin cambios): 10/10 verdes (no discrimina: la carrera no se pierde en Windows).

### TestScreenAfterOnTheEmulator/a_slow_transition...
A 8x la transicion dura ~2,4 s y el tope de `settle` (settleMax) es 3 s. Medido en local: en 17 de 18
taps la espera acaba en el tope (3,2-3,9 s) con `settled=false`, el contrato documentado ("aun
cambiaba"), y la lectura que lleva es el fotograma de la transicion en que cayo el tope. En local ya
contiene la pantalla nueva, pero en un runner lento la actividad arranca tarde y ese fotograma puede
ser la pantalla vieja o no tener ventana de app aun (el volcado fallido de CI: solo barra de estado
del sistema, sin ventana de aplicacion). Prueba de que existe: una mutacion de `settle` que devuelve
a los 400 ms da volcados con 0 elementos en ventana de app (settled=true). Es del test: exigia
la pantalla nueva tambien en un resultado `settled=false`, que el contrato permite. No es seguridad:
`settle` sigue pasando por la guarda en cada lectura y el resultado era honesto (settled=false).
Arreglo: solo una lectura `settled=true` debe ser la pantalla nueva con Changed true (lo que el test
quiere impedir: dar por asentada la pantalla vieja). El test se registra `settled/changed/elementos
en ventana de app` en cada tap. Comprobado con la mutacion (settle devuelve a los 400 ms): el test
nuevo falla (3/3 taps), asi que sigue discriminando.

Ruling: asercion del test lento solo exigible a settled=true — el contrato del producto permite devolver settled=false en el tope con cualquier fotograma — si me equivoco, un fallo real de settle que devuelva settled=false con pantalla vieja pasaria; lo cubren los demas subtests (tap normal, Back x20) y la mutacion probada.
Ruling: sondear 30 s en el test del listener — la cancelacion es asincrona y de duracion no acotada por contrato — si me equivoco el test tarda hasta 30 s mas antes de fallar de verdad.

## Segunda vuelta de F1 (5 runs sobre 68b2565b: listener 5/5, subtest lento 3/5)

Evidencia (logs de CI de los runs 38215977776 y 38215980474): a 8x la transicion dura tanto como el tope (settleMax 3 s) o mas en los runners de GitHub. Fallo A: settle dio settled=true con 6 elementos, ninguno en ventana de app (solo barra de estado), porque la ventana de destino no tenia contenido mas de settleQuiet (600 ms, calibrado a 1x). Fallo B: la guarda bloqueo la ventana de la transicion mas de settleBlockWait (2 s): exit 0, sin pantalla y la nota «The action was done; do not repeat it...»; desenlace documentado, pero el test exigia pantalla. Aparte, TestAllowFromTheLockedShadeAsksToUnlock fallo una vez en `unlock`: la sombra seguia abierta sobre la pantalla de bloqueo y el primer deslizamiento solo la cierra; flake independiente, no contamino a los siguientes.

Ruling: una lectura de la app sin ventana app/dialog con el movil desbloqueado nunca se da por asentada (`bareApp`, 2fc7774e) — es un defecto de producto: el agente recibia un fotograma sin aplicacion como pantalla final — si me equivoco, apps sin contenido accesible (juegos, camara) dan settled=false a los 3 s en vez de a los 2 s y la CLI avisa de «still changing» sin que sea cierto (revisor opus: no es riesgo de seguridad, solo etiqueta de menos; la pantalla se devuelve igual).
Ruling: el subtest lento acepta «sin pantalla» solo con la nota de tap hecho y «Blocked by ADBuddy», y exige que un settled=true sea la pantalla nueva — el bloqueo largo de la guarda es un desenlace documentado — si me equivoco, un fallo de la app con esa nota pasaria; lo cubre que cada tap siga siendo un clic en el diario (3).
Ruling: `unlock` repite el deslizamiento cada 4 s hasta ver el bouncer (66b946a1) — el primer deslizamiento solo cierra la sombra — si me equivoco, un deslizamiento de mas sobre el bouncer no pulsa ninguna tecla (revisor opus), asi que el coste es nulo.
Ruling (revision opus, ronda 1): se aceptan sus dos importantes — el aviso de la CLI, el comentario de ScreenSettled, la spec M5 y docs/usage.md dicen ahora que settled=false tambien es «sin contenido de app»; el subtest a velocidad normal exige Settled=true (si bareApp fallara con una lectura real, el emulador lo notaria) — y los dos menores (test de la guarda `how != viaApp`, nota con «Blocked by ADBuddy») — si me equivoco, nada de seguridad cambia.

SHA: 1ba53b84 (listener, en origin), 2fc7774e (settle), 66b946a1 (tests), mas el commit de la ronda 1 de revision.

Ruling (lider, ronda 2): bareApp solo retiene la espera si la lectura de partida tenia ventana de app (`protectBare` en settle) — el volcado no distingue una ventana de app con arbol vacio (juego, camara) del fotograma de la transicion (CI 38215977776: 6 elementos, todos Window:system; Dumper.kt solo emite nodos usables o informativos), asi que sin un campo nuevo de protocolo solo se puede condicionar a la partida — cuesta una espera de 3 s por cada navegacion desde una pantalla normal a una sin contenido accesible, y con partida desconocida (adopt) el comportamiento vuelve al anterior; si me equivoco, se acota con un campo de protocolo mas adelante. Tests: TestSettleABareStartSettlesOnBareReads y TestSettleAnAdoptedBareStartSettlesOnBareReads; mutacion `protectBare := true` falla los dos nuevos, `:= false` falla los dos anteriores.
Ruling (cli): `go test ./internal/cli` no se cuelga — tarda ~124 s en este PC; mis cuelgues eran topes propios de 90-100 s — si me equivoco, un test alcanzaria un adb real; el unico que se nombro no llega a adb.

## Tanda 2 de CI (5 runs sobre 7be2094d): objetivos 5/5; run 38224233038 con dos fallos nuevos

Ruling: se quita la aserción Settled=true del subtest a 1x — settled=false con la pantalla correcta y changed=true es el contrato del tope de 3 s y ya ocurría a 1x antes de bareApp (taps de «Back 20 times» con settled=false en los 5 runs de 68b2565b: 4, 2, 11, 6, 4 de 20; en 38224233038, 4 de 20), asi que la aserción que añadí por la revisión opus era una flake mía — si me equivoco, un fallo de bareApp con una lectura real a 1x solo lo vería el tope (settled=false) y lo cubren los tests unitarios y el subtest lento.
Ruling (lider): `shadeUntil` toca Expand solo si el botón más cercano al título dice «Expand» y no hubo otro toque en 6 s; nunca si dice «Collapse» — fallo de TestAnAllowedTapOnTheLockedShadeNeedsTheUnlockOfItsOwn en la fase (ii) (confirm_test.go:891): la notificación seguía colapsada 15 s; hipótesis (no confirmada) de que el segundo toque a los 3 s plegaba lo que el primero abrió; no relacionado con `unlock` (no se llama en la fase (ii)) — cuesta unos segundos más en la cortina, y si la causa era otra el flake sigue (1 de 11).
