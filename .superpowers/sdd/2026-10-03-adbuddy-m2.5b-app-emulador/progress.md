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
