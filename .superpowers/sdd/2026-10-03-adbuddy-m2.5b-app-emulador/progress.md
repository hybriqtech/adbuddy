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
