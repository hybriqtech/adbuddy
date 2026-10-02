# SDD ledger — plan: docs/superpowers/plans/2026-10-03-adbuddy-m2.5b-app-emulador.md

Branch feat/m2.5b-app-emulador (base main 2f3d650; plan commits f177f46, 4f9a715). Opus plan review done (4 important + 14 minor, all applied in 4f9a715).
Rule: emulators only via scripts/emulator.sh with ADBUDDY_EMU_AVD=adbuddy-dev ADBUDDY_EMU_PORT=5600 ADBUDDY_EMU_ADB_PORT=5060 (puffin's vars), never while a CI emulator/oracle job runs or a qemu process exists; verify no qemu left after stop; never emulator.exe by hand.
Ruling: execution of tasks that need the emulator waits for a CI-quiet window — CI queue busy at plan end (emulator run in_progress on main) — cost: time only.
