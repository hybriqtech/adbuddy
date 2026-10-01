# SDD ledger — plan: docs/superpowers/plans/2026-10-01-adbuddy-m6b-skills.md

Plan: `docs/superpowers/plans/2026-10-01-adbuddy-m6b-skills.md` · Spec: `docs/superpowers/specs/2026-10-01-adbuddy-m6b-skills-design.md` · Rama: `m6a` (M6b va en el mismo PR).
Supervisor: adbuddy-e5 (2026-10-01 21:50: Sonnet para sesión e implementadores; opus solo en revisión del plan, seguridad y revisión final; un subagente a la vez; tras revisión final limpia, push y PR sin pedir ok; el merge lo hace el supervisor).

## Rulings

- Ruling: M6b = solo las nueve skills (+ instalación, Gemini, tests); i18n, README completo y `adbuddy update` quedan fuera — el handoff y la spec de M6a lo separan, y así cabe en un PR revisable — cuesta un hito más para la i18n.
- Ruling: skills en inglés, ejemplos anotados con el nivel del clasificador y comprobados por test — es la única forma de que "recetas probadas" sea verdad sin móvil — cuesta acoplar skills y clasificador (se rompe en CI, que es lo buscado).
- Ruling: la aprobación de la spec y del plan la da el supervisor por delegación del usuario (memoria `trabajar-autonomamente`) — el usuario fijó el alcance y no interviene — cuesta que el usuario vea la spec solo en el PR.
- Ruling: se aplican los 25 hallazgos de la revisión opus del plan (4 bloqueantes: YAML sin comillas, test existente de `.claude/`, `mv` a carpeta existente = 🔴, `mv -n`/`--help` = 🔴; los demás de niveles reales, `build-utility-apk` como vía alrededor del shim, privacidad, test de flags, T2/T3/T6 con revisor opus) — todos eran verificables en el código real — cuesta más texto en los briefs.

## Tareas

| T | Estado | Notas |
|---|---|---|
| T1 | pendiente | |
| T2 | pendiente | |
| T3 | pendiente | |
| T4 | pendiente | |
| T5 | pendiente | |
| T6 | pendiente | |
| T7 | pendiente | |
| T8 | pendiente | |
| T9 | pendiente | |
| Final | pendiente | revisión opus de la rama, push y PR |

## Pre-flight (la revisión opus del plan hizo la réplica acumulada; filas de pares)

| Par / tarea | Productor → consumidor | Resultado |
|---|---|---|
| T1 → T2 | `Skills()`, `SkillNames()`, `requiredSkills` | T2 consume `Skills()`; coincide |
| T1 → T3 | `Skills()` | coincide |
| T2 → T9 | `skillExamples`, `splitExample` | mismo paquete `cli`; coincide |
| T3 → T4 | `ensureRealDirs`, `Result.note`, `loadSettings` | T4 usa `ensureRealDirs(o.Dir, ".gemini")`; coincide |
| T1..T4 (cada una sola) | réplica opus | correcciones aplicadas (7ea5eb1): YAML con comillas, test `.claude/` |
| T5..T8 | `requiredSkills` en el mismo fichero | en serie |
| T5..T9 | niveles reales vs briefs | corregidos según la revisión; el test de T2 es la autoridad |

Rulings de proceso: implementadores `adbuddy-implementer` (Sonnet); revisores según el reparto del plan (opus en T2, T3, T5-T8 y final).

Task 1: complete (commits 7ea5eb1..981c5cd, review clean)
Task 1: minor (deferred): lint usa t.Fatalf en `frontmatter` (un SKILL.md roto esconde los demás; pasar a t.Errorf+continue o t.Run); `quotedDesc` busca en todo el fichero, no solo en el frontmatter; len(desc) cuenta bytes; sin caso negativo del lint.

Task 2: review opus (26958c5): spec ✅, calidad "Necesita arreglos" — 4 importantes (metacaracteres del shell del PC sin comillas, bloques esquivables, recetas prohibidas esquivables, sin pruebas negativas) + 3 menores
Task 2: fix round 1/5 (7 addressed por el implementador, re-revisión PENDIENTE; commits 26958c5..b82d05c)
Task 2: Ruling: el test prohíbe `rm` con `-r`/`--recursive` (más estricto que el plan) — el implementador lo eligió y es coherente con §2.3 — CONFLICTO con el brief de T6 (`storage-cleanup` vacía la papelera con `rm -r /sdcard/ADBuddy-papelera`, 🔴). Decidir en T6: o la guarda admite exactamente `rm -r /sdcard/ADBuddy-papelera` (lista blanca explícita en el test) o el brief cambia a `rm` de ficheros uno a uno (🔴 cada uno); recomiendo la lista blanca de esa única orden — cuesta una excepción en la guarda.
PAUSA 2026-10-01 (supervisor adbuddy-e5: ventana de 5 h al 21% en 45 min, proyección 140%): sin subagentes nuevos hasta que él reanude por SendMessage (proyección <70% o reinicio de las 02:50).

Task 2: re-revisión ronda 1 (rereviewer Sonnet): 7/7 ADDRESSED, sin rotura Critical/Important
Task 2: complete (commits 981c5cd..b82d05c, 1 ronda de arreglos)
Task 2: minor (deferred): `uncheckedCmdRe` lleva (?i): una frase de prosa que empiece por "ADBuddy " se marca (reescribir la frase); `rm` dentro de un script con `;` pegado (`ls;rm -rf x`) no lo ve `forbiddenRecipe` (el clasificador real sí clasifica el script); info string `adb{.x}`/`adb,title` se trata como otro lenguaje; el mensaje de error de `#` dice "quote the script".
RESUELTO 2026-10-01 23:2x: supervisor reanuda M6b (nautilus liberó presupuesto).

Task 3: review opus (adae7a6): spec ✅, calidad "Needs fixes" — 1 Importante (writeFile lee el destino sin límite con Force: FIFO/enlace a /dev/zero; defecto previo que afecta también a AGENTS.md/CLAUDE.md) + menores
Task 3: fix round 1/5 en curso (hallazgos 1-5; aplazados: 6 todo-o-nada con pasada previa, 7 mensaje de run.go:78, 8 TOCTOU/os.Root)
Task 3: minor (deferred): todo-o-nada (pasada previa de ensureRealDirs); `run.go:78` sugiere "Close other agents" ante errores de enlace; TOCTOU aceptable en v1 (os.Root más adelante).

Task 3: re-revisión ronda 1 (rereviewer Sonnet): 4/4 ADDRESSED, sin rotura nueva
Task 3: complete (commits 96df481..29ae59d, 1 ronda de arreglos)
Task 3: nota para la CI: los tests de symlink y el del FIFO (unix) no se ejecutan en el PC Windows; solo los corre host.yml en Linux/macOS. Mirar en la CI del PR que pasan.

Task 4: complete (commits 29ae59d..498a1a8, review Sonnet limpia, 0 rondas)
Task 4: minor (deferred): el mensaje de `ensureRealDirs` dice "install its skills there" también para `.gemini`; "created 1 skills" con N=1; faltan tests (settings.json de Gemini inválido en Init, `.gemini` fichero normal, segundo Init sin Updated); con `.gemini` mal Init ya escribió AGENTS.md y skills (mismo patrón que el resto). Test de `.gemini` enlace: solo en la CI.

Task 5: review opus (329189f): spec ✅, calidad "Needs fixes" — 2 Importantes de redacción en debloat (el ejemplo enseña "sin icono = prescindible"; el camino de fallo termina en un undo que no puede correr y sigue con el lote) + 7 menores
Task 5: fix round 1/5 en curso (los 9)
Ruling: `dumpsys package <paquete>` es 🟢 en el clasificador real (el brief del plan decía 🔴 por la revisión del plan) — el test de ejemplos es la autoridad y lo confirmó el revisor de T5 — se corrigió el plan (restricciones globales y brief de T7) y skills-common.md; cuesta nada.

Task 5: re-revisión ronda 1 (rereviewer Sonnet): 9/9 ADDRESSED, sin rotura nueva
Task 5: complete (commits 498a1a8..b7d2c4a, 1 ronda de arreglos)
Task 5: minor (deferred): debloat sub-promete `undo` en la desinstalación de apps de sistema (`--user 0` sí deja `install-existing`); es conservador.

Rebase 2026-10-02: rama `m6b` creada y rebasada sobre origin/main f9f4422 (14 commits; sin conflictos; identidad hybriqtech y trailer exacto en los 14; `go build/vet/test ./...` en verde). `m6a` queda como copia histórica sin tocar; el PR final va desde `m6b` (supervisor). Los SHA de este ledger anteriores a esta línea son los de antes del rebase.

Task 6: review opus (04caa1a): spec cumple en lo esencial, calidad "Needs fixes" — 4 Importantes (excepción de `rm -r` sin casos negativos pese a lo afirmado; nombres con espacios/metacaracteres en `mv`; `pull` de ocultos/`.trashed` y prosa "green" falsa; manifiesto roto al mover la carpeta) + 11 menores
Task 6: fix round 1/5 en curso (todo)
Task 6: Ruling: la guarda de T2 gana una excepción exacta (`shell rm -r /sdcard/ADBuddy-papelera`) — el brief de T6 exige vaciar la papelera con una sola orden 🔴 y el test prohibía todo `rm -r` — limitada a storage-cleanup (hallazgo 8) y con casos negativos; cuesta una excepción en una guarda de seguridad (revisada por opus).
Pendiente final: nota de §2.5 (el agente adapta el entrecomillado a su shell) en device-profile, debloat y battery (T6 la añade a sus tres skills).

Task 6: re-revisión ronda 1 (rereviewer Sonnet): 18/18 ADDRESSED, sin rotura Critical/Important
Task 6: complete (commits a4c29a4..f754605, 1 ronda de arreglos)
Task 6: minor (deferred): backup-to-pc: `find` recursivo vs `stat` de un nivel (decir "primer nivel"); un fichero normal dentro de una carpeta oculta (`.thumbnails/`) sale 🔴 por tramo oculto: decir "o dentro de una carpeta que empieza por punto"; `NOTES.md` "where they keep the copies" roza con "siempre dentro del entorno"; mensaje del commit dice que quitó casos de `bad` y solo movió uno.

Task 7: Ruling: el filtro `wipe` de la guarda pasa de subcadena a `(^|[^a-z])wipe` (no casa `swipe`) y `input swipe` vuelve a bloques ```adb (🟡, sin inverso en el diario) — el filtro ancho dejaba los ejemplos de arrastre fuera de la comprobación — cuesta que `datawipe`/`rewipe` (con letra delante) ya no se rechacen (no existen comandos así).
Task 7: Ruling: `input swipe` probablemente no arrastra en muchos lanzadores (no tiene la pausa de una pulsación larga); se mantiene la skill con "tras UN intento sin movimiento = fallo; dos ⇒ parar" y se apunta en validaciones manuales (alternativas `input draganddrop` y `input motionevent DOWN/MOVE/UP`) — decidirlo con el Pixel — cuesta que home-screen falle en algunos lanzadores (para y avisa).
Task 7: review opus (810e547): guarda correcta; 2 Importantes de texto (menú del icono tras la pulsación larga; accesos especiales no revocables con pm revoke) + minores; ronda 1 (4fc005f): re-revisión (Sonnet) 13/13 ADDRESSED.
Task 7: complete (commits 0178bf0..4fc005f, 1 ronda de arreglos)
Task 7: minor (deferred): home-screen: línea suelta con espacio tras el bloque `tap "Cancel"`; "Do not insist after two failed tries" vs "one try = fallo"; permissions-audit línea 19 "Revoking is red" sin la excepción yellow de appops; regla 3 del paso 4 "or that is on the screen" vaga; sin bloque `screen` tras Atrás.
Task 7: nota: `TestRemoteOnClientGoneWhileAsking` (internal/cli) falló una vez por temporización en la máquina Windows y pasó a la segunda; no es de M6b — mirar en la CI.

Task 8: review opus (38fcc74): reglas del encargo ✅; 5 Importantes de seguridad no previstos por el brief (wrapper de Gradle sin origen fijado/ejecutar código sin verificar; cambios persistentes de PATH/perfil que ponen otro adb delante del shim; instalar sin mirar y sin camino si falla; permisos reales del APK; licencias del SDK imposibles de aceptar por el agente) + 6 menores. Ronda 1 (0183a44): re-revisión 11/11 ADDRESSED, 4 menores de redacción (frase de repositorios ambigua, cabecera inexacta, orden pasos 3-4, orden del wrapper). Ronda 2 (3be575b): 4/4 ADDRESSED, sin rotura.
Task 8: complete (commits 1ee4223..3be575b, 2 rondas de arreglos)
Task 8: minor (deferred): "official Gradle you installed in step 2" (el paso 2 solo anuncia); `.\gradlew` en PowerShell/cmd; la versión de Gradle del ejemplo (8.7) caducará; `where.exe`/Get-Command. Las nueve skills existen (requiredSkills completo).
Pendiente para la revisión final: nota §2.5 (entrecomillado por shell) falta en device-profile, debloat, battery; ver también los minors diferidos de T1-T7.

Task 9: review (Sonnet, 69f0eeb): Approved con 1 Importante (el test del emulador descartaba error y contexto: un timeout o un adb que no arranca contaba como verde) + 4 menores; ronda 1 (504b46f): re-revisión 3/3 ADDRESSED, sin rotura.
Task 9: complete (commits d0f071b..504b46f, 1 ronda de arreglos)
Task 9: minor (deferred): el nombre del job de emulator.yml ("oracle against the emulator's mksh") ya no describe todo (renombrarlo rompería un check requerido); el test del emulador no se ha ejecutado en emulador real (solo en la CI).
TODAS LAS TAREAS (T1-T9) COMPLETAS. Siguiente: rebase sobre origin/main 0a22518 (supervisor), go test, revisión final opus de la rama, push y PR.
