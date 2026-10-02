# ci-runner-local — progress

Plan: docs/superpowers/plans/2026-10-02-ci-runner-local.md

- Ruling: runner Windows como proceso en primer plano con el usuario actual, en ~/actions-runner fuera del repo — lo pidio el supervisor — si me equivoco, hay que re-registrarlo como servicio.
- Ruling: la prueba de facturacion fue un workflow minimo en push de la rama (run 37006760269 success) — el bloqueo no afecta a los runners propios — si me equivoco, la CI propia no sirve.
- Ruling: macOS se salta localmente (CI_RUNNER_MACOS="none") — ningun runner propio cubre macOS y con la facturacion bloqueada tampoco corre en GitHub — si me equivoco, un fallo solo-macOS llega a main.
- Ruling: el job ubuntu NO se pone a "none" mientras no haya runner Linux — quedaria verde sin -race — si me equivoco, los PR salen rojos hasta tener WSL.
