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
