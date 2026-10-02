# ci-runner-local — progress

Plan: docs/superpowers/plans/2026-10-02-ci-runner-local.md

- Ruling: runner Windows como proceso en primer plano con el usuario actual, en ~/actions-runner fuera del repo — lo pidio el supervisor — si me equivoco, hay que re-registrarlo como servicio.
- Ruling: la prueba de facturacion fue un workflow minimo en push de la rama (run 37006760269 success) — el bloqueo no afecta a los runners propios — si me equivoco, la CI propia no sirve.
- Ruling: macOS se salta localmente (CI_RUNNER_MACOS="none") — ningun runner propio cubre macOS y con la facturacion bloqueada tampoco corre en GitHub — si me equivoco, un fallo solo-macOS llega a main.
- Ruling: el job ubuntu NO se pone a "none" mientras no haya runner Linux — quedaria verde sin -race — si me equivoco, los PR salen rojos hasta tener WSL.
- Plan review (opus, medium): 2 criticos (matrix no vale en `if` de job; sdkmanager.bat en Windows) + I1-I6 + M1-M6, todos incorporados al plan. Veredicto del revisor tras corregir C1, C2, I2, I5: ejecutable.
- Ruling: host.yml usa una sola variable CI_HOST_MATRIX (JSON de celdas) en vez de una por SO — `exclude`/`if` con matrix no estan garantizados — si me equivoco, un JSON largo que editar a mano.
- Ruling: no se pone ninguna variable CI_* en el repo hasta revisar la auditoria de la tarea 1 (tests que tocan el home, puertos fijos o conmutadores de entorno reales) — el runner corre con el usuario del PC emparejado con el movil — si me equivoco, un test podria tocar adb/credenciales reales.
