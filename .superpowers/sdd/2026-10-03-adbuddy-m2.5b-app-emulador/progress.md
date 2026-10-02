# SDD ledger — plan: docs/superpowers/plans/2026-10-03-adbuddy-m2.5b-app-emulador.md

Branch feat/m2.5b-app-emulador (base main 2f3d650; plan commits f177f46, 4f9a715). Opus plan review done (4 important + 14 minor, all applied in 4f9a715).
Rule: emulators only via scripts/emulator.sh with ADBUDDY_EMU_AVD=adbuddy-dev ADBUDDY_EMU_PORT=5600 ADBUDDY_EMU_ADB_PORT=5060 (puffin's vars), never while a CI emulator/oracle job runs or a qemu process exists; verify no qemu left after stop; never emulator.exe by hand.
Ruling: execution of tasks that need the emulator waits for a CI-quiet window — CI queue busy at plan end (emulator run in_progress on main) — cost: time only.

# M2.5b · La app en el emulador — progress

Plan: docs/superpowers/plans/2026-10-03-adbuddy-m2.5b-app-emulador.md

## Spike (Tarea 1)

Emulador: 37.2.12 (emulator/source.properties), imagen android-37.0 google_apis x86_64 rev 6, AVD adbuddy-dev, emulator-5600, servidor adb 5060.

Nota: 5600 (y el 5590 de la CI) están fuera del rango que escanea el servidor adb (5555-5585): tras perder adbd, el emulador vuelve por los reintentos de adb (~60 × 1 s). reactivateAfter (15 s) debe quedar muy por debajo de 60 s, y estas medidas no valen para el puerto por defecto 5580.

Paso 0 (2026-10-02): rama contiene 2f3d650; HEAD..origin/main vacío; scripts/emulator.sh lee ADBUDDY_EMU_AVD/PORT/ADB_PORT (sin cambios de nombres); árbol limpio.
Desviación (ruling del supervisor): CI-LIBRE imprimía `1` y SIN-EMULADOR mostraba el emulador de la CI (emulator.exe/qemu `-avd adbuddy-ci-api37 -port 5590`). El supervisor autoriza coexistir con él: condición de arranque RAM libre >= 12 GB (había 16.95 GB) y ningún qemu con `-port 5600`; durante el spike, si la RAM libre baja de 6 GB se para el emulador de desarrollo. El de la CI no se toca.

### S1 (paso 2): el sistema no aguanta — BLOQUEADO

Arranque (`JAVA_HOME` al JDK 17 del plan; sin él, `avdmanager` falla con "JAVA_HOME is not set" y el script sale con 1 antes de crear el AVD): el script creó `adbuddy-dev` (pixel_6, android-37.0 google_apis x86_64) y llegó a `sys.boot_completed=1` en ~61 s (17:53:44 → 17:54:45). `ro.build.version.release=17`, `sdk=37`, `ro.hardware=ranchu`.

```
$ EMU; "$ADB" -s "$S" shell 'a=$(pidof system_server); f=$(pidof surfaceflinger); sleep 120; echo "system_server $a -> $(pidof system_server)"; echo "surfaceflinger $f -> $(pidof surfaceflinger)"; logcat -d -b crash | grep -c -i surfaceflinger'
system_server 7646 -> 24427
surfaceflinger 7563 -> 24356
120
```

Los dos pid cambian y hay 120 líneas de surfaceflinger en el buffer `crash`: no sirve. `logcat -d -b crash | tail -40` (resumen de la última caída):

```
F libc    : Fatal signal 6 (SIGABRT), code -1 (SI_QUEUE) in tid 23061 (RegionSampling), pid 23017 (surfaceflinger)
F DEBUG   : Build fingerprint: 'google/sdk_gphone64_x86_64/emu64xa:17/CE2A.260420.019/15611780:userdebug/dev-keys'
F DEBUG   : Kernel Release: '6.12.58-android16-6-gccafb60de224-ab14828483'
F DEBUG   : Process uptime: 10s
F DEBUG   : Executable: /system/bin/surfaceflinger
F DEBUG   : Abort message: 'Assertion failed: !rcEnc->featureInfo()->hasReadColorBufferDma'
F DEBUG   :   #03 pc ...5adf  /vendor/lib64/hw/mapper.ranchu.so ((anonymous namespace)::GoldfishMapper::readFromHost(cb_handle_t const&) const+543)
F DEBUG   :   #04 pc ...55b0  /vendor/lib64/hw/mapper.ranchu.so (GoldfishMapper::GoldfishMapper()::lambda::__invoke+704)
F DEBUG   :   #05 pc ...98cf  /system/lib64/libui.so (android::Gralloc5Mapper::lock(...)+63)
F DEBUG   :   #07 pc ...a751  /system/lib64/libui.so (android::GraphicBuffer::lockAsync(...)+545)
F DEBUG   :   #09 pc ...059b  /system/bin/surfaceflinger (android::RegionSamplingThread::threadMain()+2571)
```

Lectura: surfaceflinger aborta cada ~10 s en el hilo RegionSampling, al bloquear un buffer gráfico para leerlo desde la CPU: el mapper de goldfish (`mapper.ranchu.so`) afirma que el host no anuncia `hasReadColorBufferDma`, y con `-gpu swiftshader_indirect` (el del script) no lo anuncia. Al caer surfaceflinger, system_server se reinicia con él. La causa parece la combinación imagen android-37.0 rev 6 + emulador 37.2.12 + swiftshader_indirect, no la app (aún no instalada). Para el shell de mksh (oráculo) no importa: adbd sigue vivo; para la app, que necesita system_server estable, sí.

BLOQUEADO: la imagen android-37.0 google_apis reinicia el sistema (surfaceflinger); la app no puede correr en ella.

No se hicieron S2-S7 (el plan manda parar en S1). Cambiar de imagen (o de `-gpu`) lo decide el usuario y necesitaría que `scripts/emulator.sh` lo leyera (es de puffin).

Otros hallazgos:
- RAM: a las 17:57:30 la RAM libre bajó a 5.8 GB (con el emulador de la CI encendido y el mío en bucle de caídas); paré el mío al momento (`scripts/emulator.sh stop`, 17:57:38). Tras el `stop` solo quedó un `emulator.exe -kill <pid> -sleep 20` (el vigilante que lanza `emu kill`, salió solo en <20 s); el qemu de 5600 ya no existía. Después: solo el emulador de la CI (`-port 5590`), sin tocar; RAM libre 18 GB.
- **Seguridad:** el servidor adb del emulador (5060), recién arrancado por el script, listó también el móvil del usuario como `adb-<serial>-<id>._adb-tls-connect._tcp  device`: lo descubre por mDNS (depuración inalámbrica emparejada) y se conecta solo. No se ejecutó nada contra él (todas las órdenes llevan `-s emulator-5600`), pero cualquier orden sin `-s`, o un `adb devices` que cuente dispositivos, o Gradle `connected…` sin `ANDROID_SERIAL`, lo alcanzaría. Propuesta (decide el controlador): que `scripts/emulator.sh` exporte `ADB_MDNS_AUTO_CONNECT=0` (o `ADB_MDNS=0`) antes de arrancar su servidor, y que los preludios lo exporten también.

Estado antes del reinicio del PC (WSL2): T1 spike BLOQUEADO en S1 (surfaceflinger en bucle con swiftshader_indirect; ver S1 arriba). Esperar PR ci/emulator-no-mdns de puffin en main (mDNS apagado + ADBUDDY_EMU_GPU); luego integrar main y reintentar T1 con ADBUDDY_EMU_GPU=swangle_indirect (AVD adbuddy-dev, 5600/5060). Si sigue inestable: escalar a adbuddy-54 con el log. Plan con la regla del movil real en 931f9f3. T2-T6 sin empezar.

Task 2a (no-emulator part): complete — commits 0e00a8a, fbb4351 (guard moved to taskGraph.whenReady after controller found doFirst runs after install deps). Pending for emulator window: steps 2,3,4,6,7 of Tarea 2 (guard with real connected task must show no "Installing APK"; TokenProviderTest, mutations, AdbWifiObserverTest run). Ruling: no separate review of 2a; opus review of T2 after emulator steps.

Task 3a (no-emulator part): complete — commit 6e711dc. Pending for emulator window: steps 5,6 of Tarea 3.
Pending overall: T1 spike retry (after puffin PR #27 in main, ADBUDDY_EMU_GPU=swangle_indirect), T2 steps 2,3,4,6,7, T3 steps 5,6, T4 (variant by spike), T5 (emulator.yml after puffin changes), T6 docs.

## Spike (Tarea 1), reintento tras PR #27 de puffin (2026-10-02, implementador opus)

Base: rama en e7ab4f3 (contiene 2f3d650 y a154ec9: `scripts/emulator.sh` exporta ADB_MDNS=0/ADB_MDNS_AUTO_CONNECT=0, aborta si su servidor ve otro dispositivo, lee ADBUDDY_EMU_GPU con `swiftshader_indirect` por defecto). HEAD..origin/main vacío. Emulador 37.2.12, AVD adbuddy-dev (ya existía), emulator-5600, servidor adb 5060.
Condiciones de arranque: RAM libre 15.5 GB; qemu de la CI (`-avd adbuddy-ci-api37 -port 5590`, job en curso en main y otro en cola) presente y sin tocar (ruling del supervisor: coexistir); ningún qemu con `-port 5600`. Antes de cada orden, `adb -P 5060 devices` comprobado: solo `emulator-5600` (nunca apareció otro dispositivo).

### S1 con la GPU por defecto del script (swiftshader_indirect): mismo bucle

Arranque 19:10:21 → `sys.boot_completed=1` y env impreso 19:10:51 (30 s). `ro.build.version.release=17`, sdk 37, `ro.hardware=ranchu`, `ro.hardware.egl=emulation`.

```
$ EMU; "$ADB" -s "$S" shell 'a=$(pidof system_server); f=$(pidof surfaceflinger); sleep 120; echo "system_server $a -> $(pidof system_server)"; echo "surfaceflinger $f -> $(pidof surfaceflinger)"; logcat -d -b crash | grep -c -i surfaceflinger'
system_server 2470 -> 19243
surfaceflinger 2378 -> 19163
96
```

No sirve. Líneas de surfaceflinger del buffer `crash` (`logcat -d -b crash | grep -i -E "surfaceflinger|Abort message|mapper" | tail -30`, recortadas las rutas largas de los marcos #06-#10):

```
10-02 19:12:39.373 F DEBUG   : pid: 15588, ppid: 1, tid: 15635, name: RegionSampling  >>> /system/bin/surfaceflinger <<<
10-02 19:12:39.373 F DEBUG   : Abort message: 'Assertion failed: !rcEnc->featureInfo()->hasReadColorBufferDma'
10-02 19:12:39.377 F DEBUG   :       #03 pc 0000000000005adf  /vendor/lib64/hw/mapper.ranchu.so ((anonymous namespace)::GoldfishMapper::readFromHost(cb_handle_t const&) const+543)
10-02 19:12:39.377 F DEBUG   :       #04 pc 00000000000055b0  /vendor/lib64/hw/mapper.ranchu.so (GoldfishMapper::GoldfishMapper()::lambda::__invoke+704)
10-02 19:12:39.377 F DEBUG   :       #05 pc 00000000000498cf  /system/lib64/libui.so (android::Gralloc5Mapper::lock(...)+63)
10-02 19:12:39.377 F DEBUG   :       #06 pc 00000000000338d6  /system/lib64/libui.so (android::GraphicBufferMapper::lock(...)+198)
10-02 19:12:39.377 F DEBUG   :       #09 pc 000000000042059b  /system/bin/surfaceflinger (android::RegionSamplingThread::threadMain()+2571)
10-02 19:12:39.377 F DEBUG   :       #10 pc 00000000004c9727  /system/bin/surfaceflinger (std::__thread_proxy<... RegionSamplingThread ...>+39)
10-02 19:12:46.802 F libc    : Fatal signal 6 (SIGABRT), code -1 (SI_QUEUE) in tid 16896 (RegionSampling), pid 16870 (surfaceflinger)
10-02 19:12:47.116 F DEBUG   : Executable: /system/bin/surfaceflinger
10-02 19:12:47.116 F DEBUG   : Cmdline: /system/bin/surfaceflinger
10-02 19:12:47.116 F DEBUG   : pid: 16870, ppid: 1, tid: 16896, name: RegionSampling  >>> /system/bin/surfaceflinger <<<
10-02 19:12:47.116 F DEBUG   : Abort message: 'Assertion failed: !rcEnc->featureInfo()->hasReadColorBufferDma'
10-02 19:12:47.116 F DEBUG   :       #03 pc 0000000000005adf  /vendor/lib64/hw/mapper.ranchu.so (GoldfishMapper::readFromHost(cb_handle_t const&) const+543)
10-02 19:12:47.116 F DEBUG   :       #04 pc 00000000000055b0  /vendor/lib64/hw/mapper.ranchu.so (GoldfishMapper::GoldfishMapper()::lambda::__invoke+704)
10-02 19:12:47.116 F DEBUG   :       #05 pc 00000000000498cf  /system/lib64/libui.so (android::Gralloc5Mapper::lock(...)+63)
10-02 19:12:47.116 F DEBUG   :       #06 pc 00000000000338d6  /system/lib64/libui.so (android::GraphicBufferMapper::lock(...)+198)
10-02 19:12:47.116 F DEBUG   :       #09 pc 000000000042059b  /system/bin/surfaceflinger (android::RegionSamplingThread::threadMain()+2571)
10-02 19:12:47.116 F DEBUG   :       #10 pc 00000000004c9727  /system/bin/surfaceflinger (std::__thread_proxy<... RegionSamplingThread ...>+39)
10-02 19:12:54.790 F libc    : Fatal signal 6 (SIGABRT), code -1 (SI_QUEUE) in tid 17936 (RegionSampling), pid 17887 (surfaceflinger)
10-02 19:12:55.427 F DEBUG   : Executable: /system/bin/surfaceflinger
10-02 19:12:55.427 F DEBUG   : Cmdline: /system/bin/surfaceflinger
10-02 19:12:55.427 F DEBUG   : pid: 17887, ppid: 1, tid: 17936, name: RegionSampling  >>> /system/bin/surfaceflinger <<<
10-02 19:12:55.427 F DEBUG   : Abort message: 'Assertion failed: !rcEnc->featureInfo()->hasReadColorBufferDma'
10-02 19:12:55.427 F DEBUG   :       #03 pc 0000000000005adf  /vendor/lib64/hw/mapper.ranchu.so (GoldfishMapper::readFromHost(cb_handle_t const&) const+543)
10-02 19:12:55.427 F DEBUG   :       #04 pc 00000000000055b0  /vendor/lib64/hw/mapper.ranchu.so (GoldfishMapper::GoldfishMapper()::lambda::__invoke+704)
10-02 19:12:55.427 F DEBUG   :       #05 pc 00000000000498cf  /system/lib64/libui.so (android::Gralloc5Mapper::lock(...)+63)
10-02 19:12:55.427 F DEBUG   :       #06 pc 00000000000338d6  /system/lib64/libui.so (android::GraphicBufferMapper::lock(...)+198)
10-02 19:12:55.427 F DEBUG   :       #09 pc 000000000042059b  /system/bin/surfaceflinger (android::RegionSamplingThread::threadMain()+2571)
10-02 19:12:55.427 F DEBUG   :       #10 pc 00000000004c9727  /system/bin/surfaceflinger (std::__thread_proxy<... RegionSamplingThread ...>+39)
```

Mismo aborto que en el primer intento (cada ~8 s). La RAM libre bajó a 6.37 GB durante la medida (los dos emuladores, el mío en bucle): parado al momento, 19:13:06 (`scripts/emulator.sh stop`, rc 0). Tras el stop: ningún qemu/emulator con `-port 5600`; solo el vigilante `emulator -kill <pid> -sleep 20` (sale solo) y el emulador de la CI (`-port 5590`), sin tocar. RAM libre 16.82 GB.

Nota: que el emulador de la CI "aguante" con swiftshader_indirect solo se ve desde el shell (adbd y mksh sobreviven a las caídas de surfaceflinger); no lo he comprobado en su emulador (no se toca), pero con la misma imagen y GPU es probable que también reinicie surfaceflinger sin que el oráculo lo note.

Siguiente: reintento único con ADBUDDY_EMU_GPU=swangle_indirect.

### S1 con ADBUDDY_EMU_GPU=swangle_indirect, intento 1: interrumpido por RAM (sin veredicto)

RAM libre 16.96 GB, ningún qemu `-port 5600`. Vigilante de RAM en segundo plano (cada 5 s; por debajo de 6 GB hace `adb -P 5060 -s emulator-5600 emu kill`). Arranque 19:13:58 → boot y env 19:14:25 (27 s); qemu con `-avd adbuddy-dev -port 5600 ... -gpu swangle_indirect`; `ro.hardware.egl=emulation`. La medida de 120 s empezó a las 19:14:30 y no terminó: a las 19:15:33 la RAM libre llegó a 5.93 GB (bajada continua desde 16.9 GB a ~0.5 GB/5 s) y el vigilante mató el emulador (`OK: killing emulator, bye bye`). Tras el corte: ningún qemu `-port 5600` (solo el vigilante `emulator -kill <pid> -sleep 20`), RAM libre 10.67 GB, y `vmmemWSL` con 6.28 GB de working set (otra carga en WSL2, no mía) y el qemu de la CI con 3.0 GB. La bajada no era solo de mi emulador: la VM de WSL creció a la vez.

Siguiente: esperar a RAM libre >= 12 GB y repetir la medida con swangle_indirect (sigue siendo el único reintento con swangle: el anterior no llegó a medir), leyendo pids y recuento a los 30 s, 60 s y 120 s para que una interrupción deje datos.

### S1 con ADBUDDY_EMU_GPU=swangle_indirect, intento 2: mismo bucle — BLOQUEADO

RAM libre 14.37 GB (esperada con un bucle `until` en segundo plano), ningún qemu `-port 5600`, vigilante de RAM activo. Arranque 19:16:48 → boot y env 19:17:11 (23 s); `guard` (adb -P 5060 devices) solo `emulator-5600`.

```
$ EMU; "$ADB" -s "$S" shell 'a=$(pidof system_server); f=$(pidof surfaceflinger); echo "t0 ..."; (lecturas a 30, 60 y 120 s de pidof y de logcat -d -b crash | grep -c -i surfaceflinger)'
 19:17:16 up 0 min
t0 system_server=1606 surfaceflinger=1520
t30 system_server 1606 -> 6114; surfaceflinger 1520 -> 6040; crash=30
t60 system_server 1606 -> 9785; surfaceflinger 1520 -> 9716; crash=48
```

La lectura de 120 s no llegó: a las 19:19:09 la RAM libre bajó a 5.82 GB y el vigilante mató el emulador (`OK: killing emulator, bye bye`). Esta vez la VM de WSL no creció (vmmemWSL 2.0 GB): la bajada (~8 GB en 2 min) acompaña al bucle de reinicios de mi emulador. Las lecturas de 30 y 60 s bastan: los dos pid cambian y el buffer `crash` acumula líneas de surfaceflinger al mismo ritmo que con swiftshader_indirect. No sirve. No capturé el `Abort message` antes del corte.

Por qué swangle no cambia nada: el log del emulador (`/tmp/adbuddy-emulator.log`) con `-gpu swangle_indirect` dice `emuglConfig_init: vulkan_mode_selected:swiftshader gles_mode_selected:swiftshader` y `Graphics Adapter Android Emulator OpenGL ES Translator (Google SwiftShader)`, `OpenGL ES 3.0 SwiftShader 4.0.0.1` (backend gfxstream). En el emulador 37.2.12 de Windows, swangle_indirect acaba en el mismo traductor SwiftShader, que no anuncia `hasReadColorBufferDma`; por eso el aborto del mapper de goldfish en RegionSampling se repite.

Tras el corte: ningún qemu/emulator con `-port 5600` (solo el vigilante `emulator -kill <pid> -sleep 20`, que sale solo), el de la CI (`-port 5590`) sin tocar, RAM libre 12.49 GB.

BLOQUEADO: la imagen android-37.0 google_apis (rev 6) con el emulador 37.2.12 reinicia surfaceflinger y system_server cada ~8 s con swiftshader_indirect y con swangle_indirect (que aquí es el mismo SwiftShader); la app no puede correr en ella. Según el encargo, no se prueban otras imágenes ni GPUs. No se hicieron S2-S7 ni se creó el test temporal; ficheros temporales borrados (`/tmp/adbuddy-m25b-timeline.sh`, `/tmp/adbuddy-m25b-emu.env`).

Opciones para quien decide (no probadas): `-gpu host` (GPU real del PC; en la CI el runner es este mismo PC), otra imagen (android-36 google_apis, o una revisión posterior de android-37.0 cuyo mapper no aborte) o desactivar el muestreo de regiones (RegionSampling) si hubiera una propiedad para ello. Cualquiera necesita que `scripts/emulator.sh` cambie (es de puffin) o decisión del usuario sobre la imagen.

### S1 con ADBUDDY_EMU_GPU=host (tercer intento, ruling del supervisor): mismo bucle — BLOQUEADO

Base: rama en 799785e; origin/main tiene 5b0ff91 (solo `.github/workflows/emulator.yml`: el oráculo corre en PR y a mano), no integrado (no afecta al spike). CI-LIBRE imprimía `2` (un job en curso y otro en cola) sin emulador de la CI encendido al empezar; coexistencia autorizada por el supervisor. RAM libre 15.46 GB, ningún qemu `-port 5600`; vigilante de RAM en segundo plano (cada 5 s; por debajo de 6 GB, `adb -P 5060 -s emulator-5600 emu kill`; no llegó a dispararse). Variables: `ADBUDDY_EMU_AVD=adbuddy-dev ADBUDDY_EMU_PORT=5600 ADBUDDY_EMU_ADB_PORT=5060 ADBUDDY_EMU_GPU=host`, `ANDROID_SERIAL=emulator-5600`, `ANDROID_ADB_SERVER_PORT=5060`, `ADB_MDNS=0`, `ADB_MDNS_AUTO_CONNECT=0`, `JAVA_HOME` al JDK 17. `adb devices` (servidor 5060) antes de cada orden: solo `emulator-5600`.

Arranque 19:21:55 → `scripts/emulator.sh start` rc 0 a las 19:22:40 (45 s). El log del emulador confirma la GPU real:

```
emuglConfig_init: vulkan_mode_selected:host gles_mode_selected:host
emuglConfig_get_vulkan_hardware_gpu_support_info: Found physical GPU 'NVIDIA GeForce RTX 5080', type: VK_PHYSICAL_DEVICE_TYPE_DISCRETE_GPU, apiVersion: 1.4.325, driverVersion: 591.86
Graphics Adapter Android Emulator OpenGL ES Translator (NVIDIA GeForce RTX 5080/PCIe/SSE2)
Graphics API Version OpenGL ES 3.0 (4.5.0 NVIDIA 591.86)
```

Vigilante de estabilidad (dentro del emulador; pids y líneas del buffer `crash` a boot+5 s y hasta +120 s):

```
$ EMU; "$ADB" -s "$S" shell 'a=$(pidof system_server); f=$(pidof surfaceflinger); c0=$(logcat -d -b crash | wc -l); echo "t5 ..."; for t in 30 60 115; do sleep ...; echo "t$((t+5)) system_server $a -> $(pidof system_server); surfaceflinger $f -> $(pidof surfaceflinger); crashlines=...; dma=$(logcat -d -b crash | grep -c hasReadColorBufferDma)"; done'
t5 system_server=4466 surfaceflinger=4390 crashlines=217
t35 system_server 4466 -> 9431; surfaceflinger 4390 -> 9369; crashlines=433; dma=11
t65 system_server 4466 -> 14358; surfaceflinger 4390 -> 14289; crashlines=675; dma=17
t120 system_server 4466 -> 22800; surfaceflinger 4390 -> 22741; crashlines=1101; dma=28
(sys.boot_completed=1, ro.hardware.egl=emulation, ro.build.version.release=17)
```

No sirve: los dos pid cambian y hay 28 aserciones `hasReadColorBufferDma` en 2 min (una cada ~5 s, más rápido que con SwiftShader). 30 líneas del buffer `crash` (`logcat -d -b crash | grep -i -E "Fatal signal|Abort message|mapper|RegionSampling" | tail -30`, recortados los BuildId y las firmas largas de los marcos #04 y #10):

```
10-02 19:24:29.039 21162 21162 F DEBUG   :       #06 pc 00000000000338d6  /system/lib64/libui.so (android::GraphicBufferMapper::lock(...)+198)
10-02 19:24:29.039 21162 21162 F DEBUG   :       #09 pc 000000000042059b  /system/bin/surfaceflinger (android::RegionSamplingThread::threadMain()+2571)
10-02 19:24:29.039 21162 21162 F DEBUG   :       #10 pc 00000000004c9727  /system/bin/surfaceflinger (std::__thread_proxy<... RegionSamplingThread ...>+39)
10-02 19:24:33.556 21207 21258 F libc    : Fatal signal 6 (SIGABRT), code -1 (SI_QUEUE) in tid 21258 (RegionSampling), pid 21207 (surfaceflinger)
10-02 19:24:33.771 21881 21881 F DEBUG   : pid: 21207, ppid: 1, tid: 21258, name: RegionSampling  >>> /system/bin/surfaceflinger <<<
10-02 19:24:33.771 21881 21881 F DEBUG   : Abort message: 'Assertion failed: !rcEnc->featureInfo()->hasReadColorBufferDma'
10-02 19:24:33.771 21881 21881 F DEBUG   :       #03 pc 0000000000005adf  /vendor/lib64/hw/mapper.ranchu.so ((anonymous namespace)::GoldfishMapper::readFromHost(cb_handle_t const&) const+543)
10-02 19:24:33.771 21881 21881 F DEBUG   :       #04 pc 00000000000055b0  /vendor/lib64/hw/mapper.ranchu.so ((anonymous namespace)::GoldfishMapper::GoldfishMapper()::lambda::__invoke+704)
10-02 19:24:33.771 21881 21881 F DEBUG   :       #05 pc 00000000000498cf  /system/lib64/libui.so (android::Gralloc5Mapper::lock(...)+63)
10-02 19:24:33.771 21881 21881 F DEBUG   :       #06 pc 00000000000338d6  /system/lib64/libui.so (android::GraphicBufferMapper::lock(...)+198)
10-02 19:24:33.771 21881 21881 F DEBUG   :       #09 pc 000000000042059b  /system/bin/surfaceflinger (android::RegionSamplingThread::threadMain()+2571)
10-02 19:24:33.771 21881 21881 F DEBUG   :       #10 pc 00000000004c9727  /system/bin/surfaceflinger (std::__thread_proxy<... RegionSamplingThread ...>+39)
10-02 19:24:38.525 21966 22014 F libc    : Fatal signal 6 (SIGABRT), code -1 (SI_QUEUE) in tid 22014 (RegionSampling), pid 21966 (surfaceflinger)
10-02 19:24:38.871 22672 22672 F DEBUG   : pid: 21966, ppid: 1, tid: 22014, name: RegionSampling  >>> /system/bin/surfaceflinger <<<
10-02 19:24:38.871 22672 22672 F DEBUG   : Abort message: 'Assertion failed: !rcEnc->featureInfo()->hasReadColorBufferDma'
10-02 19:24:38.871 22672 22672 F DEBUG   :       #03 pc 0000000000005adf  /vendor/lib64/hw/mapper.ranchu.so ((anonymous namespace)::GoldfishMapper::readFromHost(cb_handle_t const&) const+543)
10-02 19:24:38.871 22672 22672 F DEBUG   :       #04 pc 00000000000055b0  /vendor/lib64/hw/mapper.ranchu.so ((anonymous namespace)::GoldfishMapper::GoldfishMapper()::lambda::__invoke+704)
10-02 19:24:38.871 22672 22672 F DEBUG   :       #05 pc 00000000000498cf  /system/lib64/libui.so (android::Gralloc5Mapper::lock(...)+63)
10-02 19:24:38.871 22672 22672 F DEBUG   :       #06 pc 00000000000338d6  /system/lib64/libui.so (android::GraphicBufferMapper::lock(...)+198)
10-02 19:24:38.871 22672 22672 F DEBUG   :       #09 pc 000000000042059b  /system/bin/surfaceflinger (android::RegionSamplingThread::threadMain()+2571)
10-02 19:24:38.871 22672 22672 F DEBUG   :       #10 pc 00000000004c9727  /system/bin/surfaceflinger (std::__thread_proxy<... RegionSamplingThread ...>+39)
10-02 19:24:44.181 22741 22790 F libc    : Fatal signal 6 (SIGABRT), code -1 (SI_QUEUE) in tid 22790 (RegionSampling), pid 22741 (surfaceflinger)
10-02 19:24:44.375 23507 23507 F DEBUG   : pid: 22741, ppid: 1, tid: 22790, name: RegionSampling  >>> /system/bin/surfaceflinger <<<
10-02 19:24:44.375 23507 23507 F DEBUG   : Abort message: 'Assertion failed: !rcEnc->featureInfo()->hasReadColorBufferDma'
10-02 19:24:44.375 23507 23507 F DEBUG   :       #03 pc 0000000000005adf  /vendor/lib64/hw/mapper.ranchu.so ((anonymous namespace)::GoldfishMapper::readFromHost(cb_handle_t const&) const+543)
10-02 19:24:44.375 23507 23507 F DEBUG   :       #04 pc 00000000000055b0  /vendor/lib64/hw/mapper.ranchu.so ((anonymous namespace)::GoldfishMapper::GoldfishMapper()::lambda::__invoke+704)
10-02 19:24:44.375 23507 23507 F DEBUG   :       #05 pc 00000000000498cf  /system/lib64/libui.so (android::Gralloc5Mapper::lock(...)+63)
10-02 19:24:44.375 23507 23507 F DEBUG   :       #06 pc 00000000000338d6  /system/lib64/libui.so (android::GraphicBufferMapper::lock(...)+198)
10-02 19:24:44.375 23507 23507 F DEBUG   :       #09 pc 000000000042059b  /system/bin/surfaceflinger (android::RegionSamplingThread::threadMain()+2571)
10-02 19:24:44.376 23507 23507 F DEBUG   :       #10 pc 00000000004c9727  /system/bin/surfaceflinger (std::__thread_proxy<... RegionSamplingThread ...>+39)
```

Parada 19:24:49 (`scripts/emulator.sh stop`, rc 0). Tras el stop (19:24:55): ningún qemu con `-port 5600`; solo el vigilante `emulator -kill <pid> -sleep 20` (sale solo) y el emulador de la CI (`-avd adbuddy-ci-api37 -port 5590`, arrancado por el job en curso mientras medía), sin tocar. RAM libre 10.87 GB (con el de la CI encendido).

Lectura: el aborto no depende del backend gráfico del host. Con la GPU real (NVIDIA, OpenGL 4.5 y Vulkan 1.4) el traductor de gfxstream tampoco anuncia `hasReadColorBufferDma`, y el `mapper.ranchu.so` de la imagen android-37.0 google_apis rev 6 lo exige en `readFromHost` cuando RegionSampling bloquea un buffer para la CPU. Es la combinación imagen rev 6 + emulador 37.2.12, no la GPU.

BLOQUEADO: la imagen android-37.0 google_apis reinicia el sistema (surfaceflinger) con swiftshader_indirect, swangle_indirect y host; la app no puede correr en ella. Según el ruling, no se prueban otras imágenes ni GPUs (el plan B, imagen android-34, lo despacha el supervisor). No se hicieron S2-S7 ni se creó el test temporal; temporales borrados.

## Spike (Tarea 1), cuarto intento: imagen android-34 google_apis (2026-10-02, implementador opus)

Base: rama en 9f7a58a (contiene 67d6e1e de puffin: `scripts/emulator.sh` lee ADBUDDY_EMU_IMAGE y ADBUDDY_EMU_STABLE_SECS). Emulador 37.2.12; imagen `system-images;android-34;google_apis;x86_64` rev 14 (ya instalada; no se instaló nada); AVD nuevo `adbuddy-dev34` (lo creó el script con avdmanager, pixel_6); emulator-5600, servidor adb 5060, GPU por defecto del script (swiftshader_indirect).
Variables en todas las órdenes: `JAVA_HOME` al JDK 17, `ADBUDDY_EMU_IMAGE='system-images;android-34;google_apis;x86_64' ADBUDDY_EMU_AVD=adbuddy-dev34 ADBUDDY_EMU_PORT=5600 ADBUDDY_EMU_ADB_PORT=5060 ADBUDDY_EMU_STABLE_SECS=120`, `ANDROID_SERIAL=emulator-5600 ANDROID_ADB_SERVER_PORT=5060 ADB_MDNS=0 ADB_MDNS_AUTO_CONNECT=0`; toda orden con `-s emulator-5600`. Antes de cada paso, `adb -P 5060 devices` comprobado: solo `emulator-5600` (se anota como `guard`).
Condiciones: CI-LIBRE imprimía `2` y el emulador de la CI (`-avd adbuddy-ci-api37 -port 5590`, con un `run.sh` del oráculo en marcha) estaba encendido: coexistencia autorizada por el supervisor, sin tocarlo. RAM libre 15.19 GB al comprobarla; al arrancar el vigilante de RAM (cada 5 s; por debajo de 6 GB, `adb -P 5060 -s emulator-5600 emu kill`) ya era 11.55 GB (la carga de la CI subió entre medias) — desviación menor sobre el umbral de 12 GB, anotada; el mínimo durante el arranque fue 6.46 GB (20:37:12) y a las 20:39:54 había 8.14 GB libres (dos qemu de 3.0 y 3.6 GB).

### S1 (paso 2): el sistema aguanta — SIRVE

`bash scripts/emulator.sh start` 20:36:45 → rc 0 a las 20:39:44 (incluye la creación del AVD y el vigilante de estabilidad de 120 s del script, que pasó: exit 0 e imprimió el env). Medida propia en paralelo (dentro del emulador, desde `sys.boot_completed=1`):

```
boot_completed 20:37:40
t5 20:37:45 system_server=567 surfaceflinger=430 crash_sf=0
t120 20:39:40 system_server 567 -> 567; surfaceflinger 430 -> 430; crash_sf=0
release=14 sdk=34 hw=ranchu egl=emulation fp=google/sdk_gphone64_x86_64/emu64xa:14/UE1A.230829.050/12077443:userdebug/dev-keys
```

Los dos pid no cambian y el buffer `crash` está vacío (`logcat -d -b crash | wc -l` = 0 a los ~2 min). Sirve.

### S2 (paso 3): línea base

```
$ guard: emulator-5600	device;
$ EMU; "$ADB" -s "$S" shell 'for k in adb_enabled adb_wifi_enabled; do echo "$k=$(settings get global $k)"; done; getprop init.svc.adbd; getprop service.adb.tcp.port; getprop persist.adb.tcp.port; netstat -ltn 2>/dev/null | grep -E ":5555 " || echo "nada en 5555"'
adb_enabled=1
adb_wifi_enabled=0
running
(vacío)
(vacío)
tcp6       0      0 [::]:5555               [::]:*                  LISTEN
$ ... shell 'su 0 netstat -ltnp | grep 5555; getprop | grep -iE "adb"'
tcp6       0      0 [::]:5555               [::]:*                  LISTEN      444/adbd
[persist.sys.usb.config]: [adb]  [sys.usb.config]: [adb]  [sys.usb.state]: [adb]  [ro.adb.secure]: [0]
```

Hallazgo: en el emulador, adbd escucha en 5555 **sin** modo tcpip (`service.adb.tcp.port` vacío): es el transporte del propio emulador (el puerto 5601 del PC se reenvía al 5555 del invitado). La nota de la regla de decisión "sin modo tcpip nada escucha en 5555" no vale en el emulador: `listening before the cut` será `true` también sin tcpip.

### S3, R1 (paso 5): corte desde el shell con restauración programada — no vuelve

```
$ guard: emulator-5600	device;
$ (20:40:34) EMU; "$ADB" -s "$S" shell 'setsid sh -c "sleep 20; settings put global adb_enabled 1" </dev/null >/dev/null 2>&1 & sleep 1; settings put global adb_enabled 0'
rc=255   (la orden pierde su adbd; 20:40:35)
registro 120 s (cambios de estado):
20:40:32 device
20:40:35 gone
20:40:36 offline
last: 20:42:41 offline
```

Sale de `device` en ~1-2 s (gone 20:40:35, offline 20:40:36) y no vuelve en 2 min. `adb -s emulator-5600 reconnect` (20:42:49, `reconnecting emulator-5600 [offline]`, rc 0), registro 60 s:

```
20:42:47 offline
20:42:49 gone
20:42:51 offline
last: 20:43:51 offline
```

Tampoco vuelve con `reconnect`. Según el paso 5, se hace aquí la segunda mitad del paso 8 (S6 y S7). Si la restauración programada (`sleep 20; settings put global adb_enabled 1`) llegó a correr se verá en S7 por el valor de `adb_enabled` al arrancar.

### S6 (paso 8, tras R1): `stop` con adb cortado — apaga

```
$ guard: emulator-5600	offline;
$ (20:44:06) EMU; scripts/emulator.sh stop
stop rc=0
$ (espera de SIN-EMULADOR, solo -port 5600) 20:44:11
emulator.exe -avd adbuddy-ci-api37 -port 5590 ...          (CI, sin tocar)
qemu-system-x86_64-headless.exe -avd adbuddy-ci-api37 -port 5590 ...   (CI, sin tocar)
emulator -kill 24616 -sleep 20                             (vigilante del propio `emu kill`, sale solo)
```

El qemu de 5600 desaparece en ≤5 s sin `Stop-Process`: (iv) se cumple (`emu kill` va por la consola del emulador, no por adbd).

### S7 (paso 8, tras R1): no arranca con adb — (v) no se cumple

RAM libre esperada hasta >= 12 GB (13.92 GB a las 20:47:22), ningún qemu `-port 5600`. `scripts/emulator.sh start` 20:47:33 con registro de 720 s en paralelo:

```
20:47:35 gone
20:47:42 offline
20:50:49 gone        (el vigilante de RAM lo mató: 20:50:47 RAM libre 5.75 GB < 6 GB, `emu kill` → "OK: killing emulator, bye bye")
start: 20:57:40 rc=124 "the emulator did not finish booting in 10 minutes"
log del emulador (/tmp/adbuddy-emulator.log): "INFO | Boot completed in 97416 ms"   (≈20:49:10)
```

El emulador terminó de arrancar (lo dice su propio log, por su canal y no por adb) y `emulator-5600` siguió `offline` en el servidor 5060 durante ~1.5 min más, hasta que lo paró el vigilante de RAM: adbd no acepta al servidor. Lectura: `adb_enabled=0` quedó en los datos del AVD y la restauración programada de R1 no llegó a correr (murió con el grupo de adbd, como prevé el paso 6). No se pudo leer el valor (sin adb); la recuperación borra los datos. La bajada de RAM (20:50) fue con mi emulador recién arrancado y el qemu de la CI de antes encendido; a las 20:53:21 la CI arrancó otro emulador (`-port 5590`, 7.6 GB de working set a las 20:58). No hay bucle de reinicios en mi lado (S1 lo midió estable).

Tras el corte: ningún qemu `-port 5600` (solo el de la CI, sin tocar). Siguiente: "Recuperación del AVD de desarrollo" sobre `adbuddy-dev34` (y ningún otro AVD), arranque y ADB-ON.

### Recuperación del AVD `adbuddy-dev34` y ADB-ON

Con el emulador parado: `rm -f ~/.android/avd/adbuddy-dev34.avd/userdata-qemu.img ~/.android/avd/adbuddy-dev34.avd/userdata-qemu.img.qcow2` (los dos existían; ningún otro AVD tocado). Espera hasta RAM libre >= 12 GB (13.13 GB a las 21:08:44; la CI tenía otro emulador `-port 5590` con 7.6 GB). `scripts/emulator.sh start` 21:08:55 → rc 0 a las 21:11:42 (vigilante de estabilidad de 120 s pasado). ADB-ON:

```
$ guard: emulator-5600	device;
$ EMU; "$ADB" -s "$S" shell 'settings put global adb_enabled 1; settings get global adb_enabled; settings get global adb_wifi_enabled; pidof system_server surfaceflinger; su 0 id'
1
0
422 558
uid=0(root) ... context=u:r:su:s0
```

### S3, R2 (paso 6): como root fuera del grupo de adbd — vuelve solo

```
$ guard: emulator-5600	device;
$ (21:11:56) EMU; "$ADB" -s "$S" shell 'su 0 setsid sh -c "echo \$\$ >/sys/fs/cgroup/cgroup.procs; sleep 20; settings put global adb_enabled 1" </dev/null >/dev/null 2>&1 & sleep 1; settings put global adb_enabled 0'
rc=255 (21:11:57)
registro 120 s:
21:11:54 device
21:11:58 gone
21:11:59 offline
21:12:17 device
last: 21:14:03 device
```

Sale de `device` en ~1-2 s y vuelve solo a los ~20 s del corte (justo cuando corre la restauración), sin `reconnect`. Después: `adb_enabled=1`, `adb_wifi_enabled=0`, pids 422/558 sin cambios. Lectura: lo que impidió volver en R1 fue que la restauración murió con el grupo de adbd; el servidor adb 5060 sí reengancha solo a 5600 cuando adbd vuelve (≤20 s aquí). Existe red de seguridad desde el shell (como root, fuera del cgroup de adbd).

### S4, R3 (paso 7): Reactivar desde la app — vuelve solo

Fichero temporal `CutAccessOnEmulatorTest.kt` con el contenido exacto del paso 7. `gradle --no-daemon -q :app:assembleDebug :app:assembleDebugAndroidTest` (21:14:24 → 21:14:41, rc 0). Instalación (`install -r --no-incremental`, las dos `Success`), `pm grant tech.hybriq.adbuddy android.permission.WRITE_SECURE_SETTINGS` (rc 0; `dumpsys package`: `WRITE_SECURE_SETTINGS: granted=true`, targetSdk=37 instalado en API 34 sin problema), `logcat -c`. CI-LIBRE `0`; RAM libre 15.08 GB.

```
$ guard: emulator-5600	device;
$ (21:14:59) EMU; "$ADB" -s "$S" shell 'am instrument -r -e class tech.hybriq.adbuddy.CutAccessOnEmulatorTest#cutThenReactivate -e adbuddyCut 1 -e reactivateAfterMs 15000 tech.hybriq.adbuddy.test/androidx.test.runner.AndroidJUnitRunner'
rc=0 (21:15:00)
registro 150 s:
21:14:57 device
21:15:01 gone
21:15:02 offline
21:15:16 device
last: 21:17:38 device
$ EMU; "$ADB" -s "$S" shell 'logcat -d -v time -s ADBuddyEmuCut:I; settings get global adb_enabled; settings get global adb_wifi_enabled; ...'
10-02 21:14:59.341 I/ADBuddyEmuCut( 6078): listening before the cut: true
10-02 21:14:59.355 I/ADBuddyEmuCut( 6078): cut=true isCut=true
10-02 21:14:59.357 I/ADBuddyEmuCut( 6078): listening after the cut: false
10-02 21:15:14.366 I/ADBuddyEmuCut( 6078): restored=true isCut=false
1
0
422 558
I/TestRunner( 6078): run finished: 1 tests, 0 failed, 0 ignored
```

Medidas: (i) sale de `device` ~2 s después del corte (21:14:59.355 → gone 21:15:01); (ii) vuelve a `device` solo a las 21:15:16, ~17 s desde el corte (~2 s después de `restored`), sin `reconnect`; (iii) `cut=true isCut=true`, `restored=true isCut=false`, `adb_enabled=1` (y `adb_wifi_enabled=0`, como antes). `listening before the cut: true` sin modo tcpip: es el transporte del emulador en 5555 (ver S2), no tcpip. system_server y surfaceflinger no cambian.

### S5 (paso 7): modo tcpip

```
$ guard: emulator-5600	device;
$ (21:18:03) EMU; "$ADB" -s "$S" tcpip 5555
rc=0
registro 90 s:
21:18:01 device
21:18:03 gone
21:18:05 device
last: 21:19:38 device
$ EMU; "$ADB" -s "$S" shell getprop service.adb.tcp.port   → 5555   (netstat: [::]:5555 LISTEN 6165/adbd)
```

Vuelve como `emulator-5600` en ~2 s (no aparece otro número de serie). R3 en modo tcpip (CI-LIBRE `0`, `logcat -c`, mismas órdenes):

```
$ (21:19:50) am instrument ... (sin -w)   rc=0
registro 150 s:
21:19:46 device
21:19:51 offline
21:20:07 device
last: 21:22:29 device
10-02 21:19:49.961 I/ADBuddyEmuCut( 6213): listening before the cut: true
10-02 21:19:49.970 I/ADBuddyEmuCut( 6213): cut=true isCut=true
10-02 21:19:50.473 I/ADBuddyEmuCut( 6213): listening after the cut: false
10-02 21:20:05.482 I/ADBuddyEmuCut( 6213): restored=true isCut=false
adb_enabled=1  adb_wifi_enabled=0  service.adb.tcp.port=5555  (5555 LISTEN 6267/adbd)  pids 422 558
I/TestRunner( 6213): run finished: 1 tests, 0 failed, 0 ignored
```

(i) sale ~1 s tras el corte; (ii) vuelve solo a ~17 s; (iii) cumple. Tras Reactivar el modo tcpip sigue (`service.adb.tcp.port=5555`), como dice la spec §4.6 (hasta `adb usb`). `adb -s emulator-5600 usb` (21:22:36, "restarting in USB mode"), registro 60 s: `21:22:34 device / 21:22:36 gone / 21:22:37 device / last 21:23:37 device`; después `service.adb.tcp.port=0`, `adb_enabled=1`, y 5555 sigue en LISTEN (adbd, transporte del emulador).

Salvedad: en el emulador, `listening before the cut: true` / `listening after the cut: false` salen igual con y sin modo tcpip (R3 sin tcpip dio lo mismo), porque el 5555 del invitado es también el transporte del emulador. Esas líneas prueban que el corte cierra adbd, no que cierre el modo tcpip en particular.
