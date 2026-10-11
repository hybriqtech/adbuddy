# Latencia del agente en el dispositivo: ledger

Rama `perf/latencia`, worktree `<worktree>`.

## Fase 1: medición y diagnóstico

- Ruling: el arnés vive en tests con puerta `ADBUDDY_LATENCY=1` (`cmd/adbuddy/latency_e2e_test.go`, `internal/emutest/latency_test.go`, `internal/emutest/latency_agent_test.go`, `internal/testutil/latency.go`) — no toca producción y se repite con el mismo comando antes y después — cuesta: no mide el salto Wi-Fi real (el emulador está en el PC); el plan lo estima contando llamadas adb por orden.
- Ruling: el daemon se mide en proceso con un `Executor` y una función `UI` envueltos que cronometran cada llamada — da el tiempo por etapa sin cambiar producción; el resto (clasificador, diario, lógica de la guarda) sale por resta — cuesta: no separa clasificador de diario.
- Ruling: APK de la app copiado de `../apk/adbuddy-4395b50-debug.apk` (sin cambios en `android/` entre 4395b50 y la base 4a745ef) en vez de compilar con gradle — ahorra RAM y minutos — cuesta: nada si `git diff 4395b50 HEAD -- android` sigue vacío.
- Ruling: el banco de modelos usa `claude -p` real con `--setting-sources project,local --strict-mcp-config --permission-mode dontAsk`, sin los hooks de ADBuddy en `.claude/settings.json` — aísla del MCP/plugins del usuario y evita que cada hook sin servidor cueste un arranque de proceso — cuesta: la config real del usuario (plugins, MCP) añade una constante igual para todos los modelos que no se mide.
- Ruling: configuraciones del banco: opus:medium (lo que hereda hoy el agente del usuario: `model: opus`, `effortLevel: medium`) como base, sonnet:low y sonnet:medium; 5 tareas, una pasada, intercaladas por tarea — pedido del líder, coste acotado (`--max-budget-usd 1.5` por pasada) — cuesta: n=1 por celda, el ruido de la API no se promedia.
- Ruling: banco de modelos SIN emulador (orden del líder: el emulador va para largo) con un teléfono falso (`latbench.Phone`: Ajustes real grabado + pantallas escritas a su estilo) tras el daemon y el shim reales — mide lo que depende del modelo (turnos, tiempo por turno, tokens) — cuesta: no incluye la latencia real del dispositivo; se suma con el arnés de dispositivo.
- Ruling: permisos del banco `bypassPermissions` (con `dontAsk` el agente encadenaba `cat NOTES.md; adbuddy …` y se le denegaba) — no mide la espera por permisos, que en uso real es del móvil — cuesta: el agente real puede tener alguna pregunta más de permisos.
- Resultado fase 1 (modelo): 15 pasadas, los tres no se separan en tiempo (~1.3-2 s por turno); sonnet ~45 % más barato; sonnet:low más turnos en pantalla. Recomendación: sin cambio de modelo por latencia. Lo que mueve el tiempo son los turnos: P1 (encadenar), P2 (esperar a que la UI se asiente), P3 (@NOTES.md).
- Pendiente: medición de dispositivo en el emulador (turno), resumen al líder y espera de su OK antes de implementar.

## Fase 2: implementación (plan `docs/superpowers/plans/2026-10-03-latencia-impl.md`)

- Revisión del plan (opus, antes de la tarea 1): con reservas; 7 hallazgos y 9 menores, todos aplicados al plan: nota «la acción ya se hizo, no la repitas» (un tap bloqueado en la lectura no debe repetirse), bloqueo de la app apuntado como `blocked`, `settleMin` 400 ms para no dar por asentada la pantalla vieja, relojes en tiempo real y copiados al entrar, `screen --settle` con `blockUI`/`abandoned`, textos de runtime en inglés (solo la ayuda de banderas por el catálogo), encadenar solo con objetivos ya vistos.
- Ruling: la espera de `--screen` se hace por sondeo del `dump` en el host, no por eventos de accesibilidad de la app — el líder la proponía por eventos «o polling»: no cambia el protocolo ni la app (apps viejas siguen valiendo), detecta cambios de contenido que el servicio actual no recibe (solo escucha typeWindowStateChanged/WindowsChanged/Notification) y cuesta ~2 lecturas más — cuesta: unos 100-250 ms más que una espera por eventos.
- Ruling: `--screen` es una orden de dos entradas de diario (acción y lectura `adbuddy screen`) y la lectura pasa por `guardForeground` cada vez — mantiene «una lectura de pantalla = misma guarda» — cuesta: la spec M5 §4.1 y §11 se actualizan (T4).
- Ruling: orden de ejecución secuencial T1, T2, T3, T4 (mismo árbol; no dos implementadores a la vez).
- T1 4460b65, T2 ebc0d28 (+23db3bd), T3 7a1cf36 (+a14177d), T4 f3736cc (+9777407), texto final d361e86. Cada tarea revisada (T1/T2 Sonnet; T3/T4 opus de diff; rondas de arreglos y una re-revisión acotada, todo arreglado).
- Ruling: el aviso de «la acción ya se hizo, no la repitas» va primero en toda nota de pantalla no leída tras un tap/type, y el CLI sale 0 — un tap bloqueado en su lectura no debe repetirse (doble acción) — cuesta: un agente que quisiera repetir a propósito debe leer la nota.
- Ruling: `settleMin` 400 ms (dos lecturas iguales antes no cuentan) — una transición lenta enseña dos veces la pantalla vieja — cuesta ~0,5 s por `--screen` aunque la pantalla se asiente al instante; revisar con el dispositivo.
- Medidas antes/después en `2026-10-03-latencia.md`: turnos 6,4 → 3,8; total 11,8 → 10,0 s (teléfono falso; ruido ±3 s).
- Pendiente: dispositivo (P4 guarda en paralelo, P5 foreground ligero, ajuste de settle*) cuando haya cupo de emulador; fila para pendientes en `docs/superpowers/fragmentos/perf-latencia.md`.

## settle2 (perf/latencia): la espera de `--screen` con pantalla de partida

- Motivo (medido en emulador, `TestLatencySettle`): primer cambio de pantalla a 520-1100 ms (máx. 1,7 s) y la app dice «protegida» en el 39 % de las lecturas tras Atrás. `settleMin` 400 ms daba por asentada la pantalla vieja (riesgo de tap repetido) y los bloqueos transitorios salían como bloqueo.
- Revisión opus del plan: no conforme a la primera (2 críticos: la lectura de partida debilitaba pausa y guarda inmediata de la acción; `Changed` falso con partida desconocida invitaba a repetir), todo incorporado. Diff d259ca3: conforme con 5 menores; arreglos 26e36dd; re-revisión acotada conforme.
- Ruling: la lectura de partida va solo por la app (`callApp` + `checkResult`), no por `runUI` — con `ErrNoApp` o timeout `runUI` caía a `dumpsys accessibility` + `uiautomator` (~2 s, más que el tope de 1 s) dejando un dump en marcha en el móvil — cuesta: sin app no hay comprobación de «protegida» en la partida; la guarda de la acción sigue cubriéndolo y con `Changed` nil (desconocido) la espera cierra solo por `settleIdle`.
- Ruling: `Changed` nil = desconocido, nunca «no cambió»; el CLI avisa «no cambió» solo con falso — un aviso falso invitaría a repetir el tap — cuesta: una partida que falla alarga la espera a `settleIdle` (2 s).
- Ruling: los bloqueos de la guarda y los `UI_PROTECTED` del dump se reintentan hasta `settleBlockWait` (2 s); pausa y pregunta 🔴 no — es lo que cura el bloqueo espurio tras una transición — cuesta: un bloqueo real avisa a los ~2 s en vez de al instante; cada intento pasa la guarda completa y no se devuelve nada leído antes de un bloqueo.
- Pendiente: S2 en el emulador (prueba escrita: `screen_after_test.go`, 804d788d) y el hueco estable para calibrar `settleQuiet` (350 ms por defecto).

## settle2: validación en emulador tras integrar main (2026-10-11)

- Merge de origin/main (777fe578) en perf/latencia: conflictos solo en `serve_test.go`, catálogos en/es y ayudas `serve`/`tap` (se conservan `--model`/`--effort` y `--screen` junto a los textos nuevos de main); `go vet` y `go test ./...` verdes.
- Ruling: validar en API 34 con AVD propio (`adbuddy-latencia34`, puertos 5602/5072) — la imagen API 37 hace caer surfaceflinger en bucle con swiftshader y swangle en este PC — cuesta: la sincronía de animaciones de API 37 no se mide; el APK es `adbuddy-4395b50-debug` (los cambios de android/ de main son textos de consentimiento/estado y no tocan el dump).
- Ruling: añadir al test S2 una subprueba con animaciones ×8 — a velocidad normal la prueba pasaba con cualquier `settleQuiet` (vacua) — cuesta: +25 s de test en emulador. Mutaciones: `settleQuiet`=100 y `case still` la hacen fallar; 600 pasa.

## P5: foreground ligero (plan 2026-10-11-latencia-p5-foreground-ligero.md)

- Tarea 1 (ac08f40f): mediana de `foreground` en la app con ~2000 vistas 33 ms (>= 20 ms: se sigue).
- Ruling: `foreground` decide con `Screen.windowHeads()` (tipo y paquete de la raíz, sin copiar el árbol) y `typeOf` pasa a una función pura única usada por `windows()` y `windowHeads()` — mismo veredicto, menos idas y vueltas binder — cuesta: si `windowHeads()` divergiera de `windows()` dejaría pasar una ventana protegida (lo cubre el test de equivalencia en emulador y el valor por defecto de la interfaz).
- Ruling: único cambio de comportamiento: si `windows()` lanzaba al recorrer el árbol, `foreground` daba `FAILED` y ahora da un veredicto; igual de cerrado (raíz ilegible = paquete vacío = protegida).
- Mutación a mano (T2e): `it.pkg.isEmpty()` -> `false` en `guardedOnScreen` hace fallar los tests; salida:
  ```
  > Task :app:testDebugUnitTest FAILED
  ForegroundHeadsTest > headsDecideWithoutReadingTheTree FAILED
      java.lang.AssertionError at ForegroundHeadsTest.kt:46
  UiExecutorTest > aWindowItCannotSee FAILED
  39 tests completed, 2 failed
  ```
  Restaurado después (diff sin `false ||`). `:app:testDebugUnitTest` + `lintDebug` verdes: 1017 tests, 0 fallos.
