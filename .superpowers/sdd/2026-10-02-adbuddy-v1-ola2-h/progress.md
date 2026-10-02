# SDD ledger — plan: docs/superpowers/plans/2026-10-02-adbuddy-v1-ola2-h.md

Rama fix/v1-ola2-h (desde fix/v1-diferidos con #20; rebasar sobre main cuando #20 este integrada).
Orden del supervisor: ola 2 H = journal (FIFO/enlaces), V07, V20; revision opus; hasta las 12:50 solo el plan.

| Tarea | Estado |
|---|---|
| T1 journal | pendiente |
| T2 V20 doctor | pendiente |
| T3 V07 peer PID | pendiente |
| Final | pendiente |

- Plan escrito; revision opus del plan en curso.

## Revisión opus del plan (aa6658eee7a9078d9): ⚠️ ejecutar tras aplicar ENMIENDAS (corte por límite de uso)
Pendiente: reescribir el plan con esto y commitear; luego ejecutar T1 (Sonnet), T2 (Sonnet), T3 partida en T3a/b/c (opus). Rama fix/v1-ola2-h desde fix/v1-diferidos; rebasar sobre main cuando #20 esté integrada.
- C1 (T3): PID del par conocido pero no verificable (muerto, zombi, starttime distinto, OpenProcess falla) => AGENT_SESSION; solo si la plataforma NO da PID se vuelve a la declaración. Capturar el par en handle() antes de leer la primera trama (Windows: OpenProcess QUERY_LIMITED|SYNCHRONIZE y guardar el handle; Unix: pid + starttime de /proc/<pid>/stat campo 22 o KinfoProc.P_starttime).
- C2 (T3 Unix): criterio "getsid(peer)==pid raíz del agente O cadena de padres llega al raíz" (creack/pty usa Setsid, pty_unix.go:116); residuos a escribir: setsid -f, systemd-run, tmux, at/cron, launchd; Windows: WMI, schtasks, explorer/COM. No es garantía (spec §4.6).
- I3: corregir contradicción del plan: declarado agente fuera del árbol SIGUE bloqueando (la declaración solo añade bloqueo).
- I4: ficheros nuevos para no chocar: ipc/peer.go + peer_{linux,darwin,windows,other}.go + peer_test.go (una sola línea en server.go handle(): c.peer = capturePeer(nc); nautilus toca server.go/ipc_test.go en origin/fix/v1-diferidos-b); session/tree.go (tern toca session.go); interfaz OPCIONAL Contains(pid)(bool,error) en pty (no ampliar pty.PTY: fakes con Pid()=1); handleRemote no recibe session.Manager: pasarlo desde serveHandler (serve.go:282-286, aviso a tern); manejar nil (remotecmd.go:291, remote_ask_test.go:97).
- I5 (Windows): proteger p.job con mutex (Kill lo cierra, pty_windows.go:296-324); pertenencia con QueryInformationJobObject(JobObjectBasicProcessIdList) (IsProcessInJob no está en x/sys); job==0 => desconocido => declaración + residuo.
- I6 (T1): os.OpenRoot sigue el enlace de su propia ruta: Lstat(s.dir)+rechazar enlace+OpenRoot+SameFile con root.Stat("."), o abrir la raíz del entorno y ws.OpenRoot("journal"); Load por la misma raíz (fs.Glob(root.FS()), root.Lstat, root.OpenFile O_RDONLY|nonblock); safeio.OpenRegularIn(root,name); workspace conserva envoltorios finos y sus centinelas (readRegular, errNotRegular, errTooLarge, maxRead; test busca "1 MiB").
- I7 (T1): rechazar Nlink>1 en openAppend (Stat_t.Nlink / NumberOfLinks).
- I8 (T1): un mes no regular NO se cuenta como skipped (son líneas dañadas): Load devuelve error que nombra el fichero; undo y journal fallan visibles.
- I9 (T2): Ping no lleva la pausa: añadir Paused bool `json:"paused,omitempty"` en ipc.go y rellenar en daemon.go:408 con d.o.Paused(); saltar todas las llamadas adb de doctor en pausa (version, devices, phone, identity, network port, phone app); verdict() ignora skip: añadir warn en la comprobación serve; textos nuevos doctor.skip.paused y doctor.serve.paused en en/es (el plan no debe exigir que ya exista); decir que T2 es coherencia, no frontera.
- I10: base: reproducir sobre la base de esta rama (no origin/main); conflictos con git diff <merge-base>..origin/main y origin/fix/v1-diferidos-b.
- I11: tests T3 Windows con el binario de test en modo ayudante (como startAgent/childOf): hijo del agente con SOURCE vacío => AGENT_SESSION; conector que sale al conectar => AGENT_SESSION; proceso de test fuera del árbol => pasa; declaración agent fuera del árbol => bloquea. Unix solo compila: exige CI (subir rama como copia).
- Menores: M1 doctor journalWritable usa os.Stat (doctor.go:615) => Lstat; M2 backups: Store.MakeBackupDir(id) por la raíz (precheck.go:344-349), residuo adb pull; aparte: adb push lee cualquier fichero del PC (pendientes); M3 datos de plataforma (SIO_AF_UNIX_GETPEERPID=0x58000100 definir local, windows.WSAIoctl vía SyscallConn().Control; Linux unix.GetsockoptUcred; macOS LOCAL_PEERPID + SysctlKinfoProc; comprobar getsid en Darwin en la CI); M4 partir T3 en 3a PeerPID, 3b árbol pty/session, 3c criterio+conexión; M5 validaciones de humanos: attach desde otra terminal/IDE/ssh/elevada/WSL1 (WSL2 no conecta AF_UNIX de Windows).
- Alternativas descartadas: token por sesión del agente (el agente no lo presenta), exigir TTY (se falsea con script); opción de modo estricto: confirmar cada attach en el móvil/terminal de serve (configurable, no por defecto).

Task 1: complete — commits 6b76bb7 e3806ef 723120a 1ea7ea6; opus review ✅ (0 critical/important). Ruling: minors 1 (doctor journalWritable no mira Nlink>1/backups-link), 2 (OneDrive cloud reparse tags => ModeIrregular, validar a mano), 3 (residuo adb pull: symlink `original` plantado en la carpeta ya creada) — van a pendientes-m3-m5.md en T4 — son seguros (fallan cerrado) — cuesta nada si me equivoco. Minor 4/5 ignorados.

Task 2: complete — commit 0e2276c; opus review ✅ (minors only, ignored).

Task 3a: complete — commit 30e7311 (implementer report OK). Ruling: no per-subtask review for 3a/3b; one opus review of T3 (3a+3b+3c) acts as T3 task review AND final branch review (supervisor asked for economy; T1/T2 already reviewed) — cost if wrong: one extra fix round. Concern to carry: Windows Peer copy keeps released handle value; Peer.Same valid only inside the synchronous handler — reviewer must check 3b/3c callers.

Task 3b: complete — commit b49b1b6. Carry: T3c must not pass pid<=0 to InAgentTree; Linux/macOS paths only verified in CI.

Task 3c: complete — commit 8657b15. Open: Windows Conn.peer copy keeps released handle (nobody calls Same() outside handler) -> pendientes.

Task 4: complete — 5d4588d. Opus T3+final review ✅ (0 critical/important); fix wave 1d52cc8 (peer unverifiable Known+Err; release on Conn copy) + 6c48727 (docs). Ready to push as PR.
