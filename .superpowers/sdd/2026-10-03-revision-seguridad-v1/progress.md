# SDD ledger — revisión de seguridad independiente de main (5dd5e1a)

Encargo (supervisor adbuddy-f3): segunda mirada, solo lectura, sobre lo integrado hoy: #19 (guarda %XX, modo remoto, IPC lock/cuota), #21 (diario safeio, peer PID/árbol del agente V07, doctor), #23 (escritor del PTY, tarjetas/bidi app, V08 cola acotada, V40), #22/#24 (workflows de CI y runners). Rama docs/revision-seguridad-v1 (desde origin/main 5dd5e1a). Un subagente opus (medium) por área, uno a la vez; hallazgos con severidad (CRÍTICO = ejecuta 🔴 sin confirmación o toca el móvil sin confirmar; ALTO; MEDIO; BAJO). No se arregla nada sin OK del supervisor salvo críticos claros (avisar antes).

Áreas: A1 clasificador/guard/shim · A2 IPC/peer/daemon · A3 diario/undo/doctor · A4 confirmaciones, tarjetas, PTY, canal del móvil · A5 CI/runners · A6 app Android (permisos, tokens).
Informes por área: area-N-informe.md en esta carpeta (git-ignorada; el consolidado se versiona en docs/revision-seguridad-v1.md).

Ruling (supervisor, matiz): revisión enfocada, sin trabajo por trabajar: áreas con riesgo real = A1 clasificador/guard, A2 IPC/peer/V07, A4 confirmaciones y tarjetas, A5 runners/CI. A3 (diario) y A6 (Android) se omiten (revisadas por opus por tarea). PARAR en cuanto un área no dé hallazgos; si en 1 h no hay nada relevante, informe corto. M2.5b vuelve a ser prioridad cuando #27 esté en main.
