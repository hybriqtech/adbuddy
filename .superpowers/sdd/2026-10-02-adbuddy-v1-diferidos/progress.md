# SDD ledger — plan: docs/superpowers/plans/2026-10-02-adbuddy-v1-diferidos.md

Plan: `docs/superpowers/plans/2026-10-02-adbuddy-v1-diferidos.md` · Triage: `triage.md` (esta carpeta) · Rama: `fix/v1-diferidos` (desde main 83b7019).
Supervisor: adbuddy-e5 (Sonnet en sesión e implementadores; opus solo plan, seguridad y final; un subagente a la vez; evitar M6c (tern: cadenas/i18n) y M5c (nautilus: notificaciones/pantallas app); lo que solape, al final y avisar).

## Rulings
- Ruling: ola 1 = 7 tareas sin solape con M6c/M5c (V24 V25 / V14 V32 V26 V15 V16 V17 / V10 V12 V13 / V29 / V23 V28 V41 / V37 V33 V39 / app JVM V21 V42 V43); la ola 2 (clasificador, undo/journal, tarjetas, lector del bridge) va tras integrar M6c — un motivo o mensaje nuevo choca con su catálogo i18n — cuesta que lo de mayor impacto de seguridad (V01-V08) espere.
- Ruling: se aparta V27 (lista de variables de entorno permitidas para el agente) — cambia el comportamiento y puede romper agentes reales (AWS_*, NODE_*, claves de otros proveedores) sin poder probarlos aquí; decisión de producto — y V30 (fijar acciones de CI por SHA: hace falta red).

## Tareas
| T | Estado |
|---|---|
| T1..T7 | pendientes |
| Final | pendiente |

## Órdenes del supervisor (2026-10-02, tras el triage)
1. Regla del proyecto: un hueco que deja ejecutar 🔴 sin confirmación nunca se aparca. Comprobar V01 (undo con inversos forjados), V02/V03/V06 (clasificador) y V08 (lector del bridge); lo que lo permita, o sea un salto de confirmación, va a la ola 1 PRIMERO, con revisión opus, aunque choque con la i18n de M6c: mensajes/motivos en texto plano con el patrón actual y AVISAR A TERN de las cadenas nuevas para su catálogo. Mi lectura del triage: V01 (undo ejecuta `uninstall <pkg>`/`rm -f <ruta>` de entradas forjadas sin confirmar), V06 (`dumpsys` 🟢 con subcomandos mutantes), V04/V05 (tarjeta que no muestra `dangerouslyDisableSandbox`/`run_in_background`; Enter ciego sobre un diálogo no visto) son huecos de confirmación → tareas nuevas 1-3 (opus); V02/V03 (🔴 normal, solo una regla del usuario los baja) van con V06 (misma zona: classify/special.go); V08 falla seguro (el 🔴 caduca y se deniega): al final.
2. V30 (SHA de acciones de CI): hay red con el gh de hybriqtech (`GH_CONFIG_DIR="$HOME/.config/gh-hybriqtech" gh api repos/<owner>/<action>/git/ref/tags/<tag>`; etiquetas anotadas: deref con git/tags/<sha>); hacerlo en la ola 1 si es rápido; el pre-push y la CI lo validan.
3. V27 apartado (decisión de producto; lo apunta el supervisor en "Para el usuario").

## Estado al cortar (07:5x)
- Rama `fix/v1-diferidos` desde 83b7019 con el plan de la ola 1 (b076052; 7 tareas) y el triage local. Revisión opus del plan EN CURSO (agente a489aeb0c1f25e803, avisará al terminar).
- Siguiente: aplicar los hallazgos de esa revisión al plan + las órdenes de arriba (insertar 3 tareas de seguridad opus delante: T1 undo/V01 con el diseño "el inverso pasa por el mismo clasificador y confirmación que cualquier orden"; T2 clasificador V06+V02+V03; T3 tarjetas y atención V04+V05; renumerar el resto; V30 en la tarea de setup/CI), commitear el plan y ejecutar con subagentes de uno en uno (implementador Sonnet, revisor según plan).

## Corte (límite de uso)
Plan reescrito con los hallazgos de la revisión opus del plan y las órdenes del supervisor: 11 tareas (T1 undo/V01, T2 clasificador V06/V02/V03, T3 tarjetas y atención V04/V05, T4 transcript V24 + PTY V25, T5 workspace V14/V26/V15/V32, T6 instalación y CI V16/V17/V30, T7 guarda de pantalla, T8 IPC V29, T9 chat y ficheros, T10 tests, T11 JVM). Pendiente: commitear (hecho al cortar), generar briefs con `task-brief` y ejecutar T1 (implementador Sonnet, revisor opus) de uno en uno; recordar avisar a tern de los textos nuevos de T1-T3 y reconciliar `pendientes-m3-m5.md` al final. Ningún subagente en vuelo.

## Ejecución
- T1 despachada (implementador a0448e97e886b22e6, BASE abe6b37). Siguiente: review-package + revisor opus.
- T1 ronda 0: commit 74ed01b; revision opus (a6bf78baa3d46459b): 2 Important (titulo forjable §4.5, undo fuera de inFlight tras preguntar) + minors. Ronda 1 enviada al implementador. Aplazados: M4 registro denegacion, M6 comparar Op.Args, M9 Detail ingles. Para tern: Long de cli/undo.go y undoFixes sin UNDO_DENIED.
- Pendiente tras T1: quitar T8 (IPC V29) y T11 (JVM V21/V42/V43) del plan (nautilus los toma), y V11/V38 (guard.go); rebasar sobre main 2fcf8e2.
- Task 1: complete (74ed01b + 002b19c; re-revision sonnet sin hallazgos importantes). Para tern: textos nuevos en task-1-report.md; Long de cli/undo.go y undoFixes UNDO_DENIED.
- Rebase sobre main 2fcf8e2 hecho (T8/T11 quitadas, V11/V38 a nautilus). T2 despachada (implementador opus a7d9c165d5a6c26e5, BASE 6ce00cf).
- T2 ronda 0: 58f489c; revision opus: 1 Important (--checkin consume estado) + am restart/pkill adbd (ampliacion V03). Ronda 1 enviada. Validacion manual pendiente: repasar lista de lectura de dumpsys (activity/window/procstats) en el emulador.
- T2 ronda 1: 3e52c0b; re-revision opus: 4 bypasses en analisis de patron pkill/killall. Ruling: todo pkill/killall 🔴 especial sin analizar args — el analisis es fragil y el contrato no lo exige — cuesta algo de friccion si alguien usa pkill de una app (sigue siendo confirmable). Ronda 2 enviada.
- Task 2: complete (58f489c, 3e52c0b, ffdd4ff). Textos nuevos para tern en task-2-report.md (classify.special.amRestart, amUser, etc.). Propuesta futura: am unlock-user / pm remove-user 🔴 normal.
- T3 despachada (implementador opus ac9a016bfeb2cff63, BASE ffdd4ff).
- T3 ronda 0: 717e83d (V04), ca34d84 (V05); revision opus ✅✅ con 2 Important. Ruling: arreglar ambos en T3 (hook no procesado => held; Edit/Write a .claude/.git/.mcp.json => terminal) — cuesta que esas ediciones se aprueben siempre en terminal. Aplazados: bajar held al ver Request interrupted; elicitation_dialog; TOCTOU preexistente phone_chat.go:265. Ronda 1 enviada.
- Task 3: complete (717e83d, ca34d84, ade4c86, e814735; re-revision sin hallazgos). Sin textos nuevos para tern en T3.
