# Latencia del agente en el dispositivo: ledger

Rama `perf/latencia`, worktree `C:/Users/Jose/orca/workspaces/adbuddy/latencia`.

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
