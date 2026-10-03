# Tareas 2b y 3b: pasos con emulador (API 34, adbuddy-dev34, emulator-5600, servidor 5060)

Evidencia completa (órdenes y salidas) en progress.md, sección "Tareas 2b y 3b".

- T2 Paso 2: guarda OK con la tarea real: `not-an-emulator`, `emulator-5600,x` y vacío → exit 1 con el mensaje, ninguna tarea ejecutada, nada instalado.
- T2 Paso 3 por Gradle: DEFECTO. AGP 9.4.1 solo tiene UTP; su motor corre en un worker de Gradle con entorno vacío en Windows (`DefaultProcessWorkerSpec.sanitizeEnvironment`; en Unix solo LANG/LC_*), así que su `adb -s emulator-5600 …` va al servidor 5037 (el del móvil): "device 'emulator-5600' not found", y aun así BUILD SUCCESSFUL con 0 tests. Móvil no direccionado (todas las órdenes del motor llevan -s). No volví a lanzar `connected*`.
- T2 Pasos 3/4/6/7 por `adb -P 5060 install` + `am instrument`: 3 → fallo de test en getType (ContentResolver.getType se traga el SecurityException; logcat lo muestra) → test arreglado (a5055c1) → OK 12; 4 → fallan los 4 esperados; 6 → fallan los 2 del observador ("did not hear"); 7 → OK 15, sin saltados, adb_wifi_enabled 0 antes y después.
- T3 Paso 5: PASS los 4; hallazgo: la limpieza no desinstalaba (t.Context cancelado antes de Cleanup) → test de regresión RED/GREEN y arreglo (context.WithoutCancel).
- T3 Paso 6: FAIL esperado `port:0 … want port 47123 and a nonce`; revertido.
- Emulador parado, ningún qemu -port 5600, AVD con adb y sin la app.

Propuestas (decide el controlador): la guarda debería negar `connected*` mientras `ANDROID_ADB_SERVER_PORT` sea distinto de 5037 (o siempre), y los instrumentados deberían correr por `am instrument` desde un script o desde emutest; el README de T2 (paso 8) y el job de T5 dependen de esto. Hay una alternativa experimental, sin probar: `android.experimental.androidTest.builtin_test_platform=true` (el motor correría en una tarea Test, que hereda el entorno).
Manual en el Pixel (Android 17): lectura de adb_wifi_enabled por la app, validaciones 2, 3 y 10 en 17.
