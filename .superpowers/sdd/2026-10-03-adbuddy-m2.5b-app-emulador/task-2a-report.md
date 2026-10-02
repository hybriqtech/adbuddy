# Tarea 2A (sin emulador): guarda, AdbWifiObserverTest, README

Hecho: Pasos 1, 5, 8, 9, 10. NO hechos (arrancan el emulador): Pasos 2, 3, 4, 6, 7.

## Hallazgo: --dry-run no activa la guarda
`ANDROID_SERIAL` sin definir + `gradle -m :app:connectedDebugAndroidTest` -> BUILD SUCCESSFUL (exit 0): la guarda es un
`doFirst` y la ejecucion en seco no ejecuta acciones. Verificacion alternativa y segura: init script en el scratchpad
(fuera del repo) que registra una tarea no-op `connectedProbe` en :app; al empezar por `connected`, le aplica la guarda
real de build.gradle.kts (`tasks.configureEach`). Ningun task `connected*` real se ha lanzado, ni adb (servidor 5999).

| ANDROID_SERIAL | resultado |
|---|---|
| sin definir | rechazado, exit 1, `... not ""` |
| `emulator-5600` | permitido, PROBE-RAN, exit 0 |
| `emulator-5600,xyz` | rechazado, exit 1 |
| `xyz` | rechazado, exit 1 |
| ` emulator-5600` | rechazado, exit 1 |
| `emulator-5600 ` | rechazado, exit 1 |

## Paso 9
`gradle -q :app:testDebugUnitTest :app:assembleDebug :app:lintDebug :app:assembleDebugAndroidTest` (sin ANDROID_SERIAL) -> exit 0;
AdbWifiObserverTest.class compilado en debugAndroidTest.

## Pendiente para el emulador
Pasos 2 (RED guarda con la tarea real; instalar nada), 3, 4, 6, 7, y parar el emulador.
Nota: en el Paso 2 la guarda por `doFirst` solo se comprueba de verdad con la tarea real; la logica ya esta probada con la sonda.

## Correccion (aviso del coordinador): guarda en el grafo de tareas
El `doFirst` llegaba tarde (las tareas install de las que depende connected* corren antes). Ahora la guarda es
`gradle.taskGraph.whenReady { if (allTasks.any { it.name.startsWith("connected") }) check(Regex("emulator-[0-9]+").matches(serial)) }`.
Verificado con la sonda `connectedProbe` Y con `gradle -m :app:connectedDebugAndroidTest` (puerto adb 5999; nunca la tarea real):
sin definir -> exit 1; `emulator-5600` -> exit 0 (la sonda ejecuta PROBE-RAN; el dry-run pasa); `emulator-5600,xyz`, `xyz`,
` emulator-5600`, `emulator-5600 ` -> exit 1, en ambos metodos. `-m :app:installDebug` y `-m :app:assembleDebugAndroidTest`
sin ANDROID_SERIAL: BUILD SUCCESSFUL. Paso 9 completo: exit 0.
