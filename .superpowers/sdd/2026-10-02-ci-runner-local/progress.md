# ci-runner-local — progress

Plan: docs/superpowers/plans/2026-10-02-ci-runner-local.md

- Ruling: runner Windows como proceso en primer plano con el usuario actual, en ~/actions-runner fuera del repo — lo pidio el supervisor — si me equivoco, hay que re-registrarlo como servicio.
- Ruling: la prueba de facturacion fue un workflow minimo en push de la rama (run 37006760269 success) — el bloqueo no afecta a los runners propios — si me equivoco, la CI propia no sirve.
- Ruling: macOS se salta localmente (CI_RUNNER_MACOS="none") — ningun runner propio cubre macOS y con la facturacion bloqueada tampoco corre en GitHub — si me equivoco, un fallo solo-macOS llega a main.
- Ruling: el job ubuntu NO se pone a "none" mientras no haya runner Linux — quedaria verde sin -race — si me equivoco, los PR salen rojos hasta tener WSL.
- Plan review (opus, medium): 2 criticos (matrix no vale en `if` de job; sdkmanager.bat en Windows) + I1-I6 + M1-M6, todos incorporados al plan. Veredicto del revisor tras corregir C1, C2, I2, I5: ejecutable.
- Ruling: host.yml usa una sola variable CI_HOST_MATRIX (JSON de celdas) en vez de una por SO — `exclude`/`if` con matrix no estan garantizados — si me equivoco, un JSON largo que editar a mano.
- Ruling: no se pone ninguna variable CI_* en el repo hasta revisar la auditoria de la tarea 1 (tests que tocan el home, puertos fijos o conmutadores de entorno reales) — el runner corre con el usuario del PC emparejado con el movil — si me equivoco, un test podria tocar adb/credenciales reales.
- T1-T5 hechas (60cae0e, f784986, 870e003, 3158c9a, e7a4406); revision por tarea sustituida por una revision opus de toda la rama al final — son YAML de ~15 lineas cada uno, con actionlint y CI real — si me equivoco, un hallazgo mas en la revision final.
- Auditoria T1 (tests vs estado del usuario): ningun test toca el home real ni puertos fijos (isolate(t)/TestMain los redirigen). Solo dos conmutadores pasan de falso a real: ADBUDDY_REAL_CLAUDE=1 (usa credenciales de claude, copiadas a un temporal) y ADBUDDY_MKSH_SERIAL/_ADB (adb real, exige serial emulator-*). Regla: el runner se arranca con entorno limpio (sin ADBUDDY_*, ANDROID_SERIAL, ANDROID_ADB_SERVER_PORT); lo hice con `env -i`.
- Hallazgo empirico: el emulador de Windows rechaza -partition-size 4096 (tope 2047 MB) -> emulator.sh pasa 2047 en Windows (eb3884b/54cda39, ruta de tern, avisado). El oracle paso en el runner Windows (14m48s) con el tamano por defecto.
- Revision opus final: sin criticos; I1 (guarda de forks solo con variable definida), I2 (checklist previa a publicar), I3 (caches en release-check), M1/M3/M4/M5 arreglados (7f610c8, d9f1686, 54cda39). Pendientes de validar: M2 (XDG_CONFIG_HOME en ubuntu-latest, cuando vuelva la facturacion) y emulator.sh en el runner Linux.
- Ruling: scripts/goreleaser/sign no se cubren en Windows (shellcheck, install.sh por uname, apksigner son Linux) y quedan en GitHub-hosted (rojo por facturacion) hasta el runner WSL — "none" daria verdes falsos — si me equivoco, PR rojos por jobs que no importan hasta tener WSL.
