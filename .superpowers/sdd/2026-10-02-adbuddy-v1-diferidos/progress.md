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
- T4a (V24) despachada (implementador opus affa7fd1f622da366, BASE bceb578). T4b (V25 PTY Job object, Sonnet) despues.
- T4a (V24): d1fe7df; revision opus ✅✅ sin Critical/Important; arreglo de comentario en curso. Validacion manual pendiente: usuario con .claude como junction y transcript_path ya resuelto (falla cerrado, igual que antes). Para el PR: cambia API ValidTranscript->Transcript, NewTailer lo recibe.
- Task 4a: complete (d1fe7df, 9cccd65).
- Rebase sobre main 0aaa593 (M6c): conflictos reales en classify/undo (motivos pasan a Text con catalogo). Abortado a mano; delegado a implementador opus aef07c12743a09829 (backup branch backup/v1-diferidos-pre-m6c; rebase + conversion de textos T1-T3 a catalogo EN/ES + Long de cli/undo.go y UNDO_DENIED). Despues: review (sonnet) del rebase y T4b.
- Rebase M6c: HEAD bf81b28, revision sonnet aprobada. Ruling: dejar los 3 commits intermedios de T2 con TestReasonsBilingual rojo (el castellano llega en bf81b28) — reescribir historia es mas arriesgado que util; coste: bisect incomodo en 3 commits. Pendiente menor para el final: m2 (matar->detener en reasons_es.go:129,134), m3 (TestUndoDeniedHasAFix debe exigir exit 1). Cuenta: textos EN/ES de T1-T3 ya en catalogo; undo item.Error (denied/refused) sigue en ingles como M6c deja el resto.
- Task 4b (V25): 8d04d43; revision sonnet aprobada; arreglo menor test (w.job=0) en curso. Para el final/T10: flake preexistente internal/ipc TestClientDisconnectIsNotErrClosed (ipc_test.go:319-336, Sleep(100ms) bajo carga) -> V37; OJO ipc/ es de nautilus (T8). Aviso sin job object en pty usa log.Printf en ingles (no catalogo): apuntar a tern/pendientes.
- Task 4: complete (V24 d1fe7df+9cccd65; V25 8d04d43+53b8263). T5 se parte en 5a (V14+V26, opus) y 5b (V15+V32, sonnet).
- T5a (V14 b35bc0f, V26 6f8f5ab): revision opus ✅✅; ronda de tests menores en curso. DIFERIDO NUEVO (decirlo al supervisor): journal/journal.go:137-140 openAppend sigue enlaces en runtime y :197 Load sin acotar (FIFO cuelga journal/undo/doctor). Tests Unix de V14/V26 solo se ven pasar en CI.
- Task 5a: complete (b35bc0f, 6f8f5ab, b1fcf17). Siguiente 5b (V15+V32, sonnet).
- T5b (V15 40504ce, V32 4953403): revision opus ✅✅; minors en curso.
- Task 5: complete (5a b35bc0f 6f8f5ab b1fcf17; 5b 40504ce 4953403 16a34f7). T6 despachada.
- Task 6: complete (ca85a26 V16, d5e34cc V17, 1a78e17 V30; revision sonnet ✅✅ con SHAs verificados). Minor: comentario de chooseAPK (MaxAPK/FIFO) obsoleto, aplazado.
- Task 7: complete (8cedd68 V10, 349e466 V12, 1123b69 V13; revision sonnet ✅✅). Minors aplazados: tests de bounds invertidos/id; mensaje UI_FAILED menos accionable que UI_AMBIGUOUS; bounds negativos no tocados.
- T9a (V28, opus afa8a861289251911, BASE 1123b69) despachada; 9b (V23+V41) despues.
- Task 9a (V28 c937176): revision opus ✅✅. Minors aplazados: lecturas por ruta tras openRoot (cuota, e.Info ModTime, miniatura), symlink relativo Unix dentro del entorno, orden de limpieza en tests. Texto nuevo para catalogo: files.too_many_downloads ("Too many downloads at once: wait for one to finish.").
- T9b (V41b 512d179, V23+V41a 3245459): revision sonnet ✅✅; 2 minors de tests en curso. Pendiente UX: invalid en reintento (app FAILED con Reintentar).
- Task 9: complete (9a c937176; 9b 512d179 3245459 4f86efd). T10 despachada (sonnet).
- T10 (V37 1d72cab, V33 f2df7a6, V39 4269146): revision sonnet ✅✅; ronda de menores + m2/m3 del rebase en curso. Pendiente para nautilus: flake internal/ipc TestClientDisconnectIsNotErrClosed (Sleep(100ms)).
- Tras T10: revision final opus de toda la rama (git diff origin/main..HEAD), una sola tanda de arreglos, git grep IPs/seriales, push fix/v1-diferidos y PR (hybriqtech).
- Task 10: complete (1d72cab f2df7a6 4269146 + 4c86592 28f4529 097565a). Siguiente: reconciliar pendientes-m3-m5.md (doc) y revision final opus.
- Doc pendientes reconciliado (f2e2703). Siguiente: revision final opus.
- Revision final opus: ⚠️ sin Critical/Important. Tanda unica de arreglos (a97830f25b1c9c5e9): flake childOf 30s, nivel real del undo en el diario, tailer O_NONBLOCK (Unix), docs pendientes. Para la descripcion del PR: 3 commits intermedios de T2 con TestReasonsBilingual en rojo (2e530e6, 63afc0d, 4217c17) -> integrar con squash o avisar.
- Re-revision de la tanda final: sin problemas; arreglada puntuacion en pendientes. Listo para push y PR.
