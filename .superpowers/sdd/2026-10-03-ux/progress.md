# Ledger UX (rama ux/revision)

Encargo: revisión completa de UX (app Android + CLI `adbuddy`) y aplicar mejoras. Fase 1 = revisión solo lectura -> docs/superpowers/plans/2026-10-03-ux-revision.md.

## Estado
- Fase 1: en curso. 3 revisores de solo lectura lanzados (ux-rev-chat, ux-rev-onboarding, ux-rev-cli), areas disjuntas.

## Rulings
- Ruling: 3 revisores adbuddy-reviewer (Sonnet) en paralelo, uno por area (chat/confirm/notify/term/files; onboarding/ajustes/permisos/a11y; CLI host) — el encargo permite 2-3 y asi no inundo mi contexto — cuesta: hallazgos superficiales; mitigo verificando el top 10 yo mismo antes de enviarlo.

## Fase 1 terminada
- 3 revisiones recibidas (C1-C27, O1-O21, H1-H16). Informe: docs/superpowers/plans/2026-10-03-ux-revision.md. Top 10 re-verificado por mi (C2, O1, O5, O6 leidos en codigo).
- Ruling: C2 queda como "S, SEG": el diseño actual (la notificacion reabre el dialogo, MainActivity.kt:85) es intencionado; la mejora añade un banner que reabre la misma tarjeta, sin cambiar semantica — si me equivoco: un banner mal hecho podria dar un atajo; por eso plan con revisor opus.
- Ruling: C8/C9/C10/O9/O12/O21 fuera del top aunque baratos y agrupados en P5 (solo con opus) — tocan consentimiento o semantica visible de confirmacion — cuesta: mejoras de seguridad percibida tardan mas.
- Ruling: H14 (platformtools) no se toca sin avisar al dueño del paquete — fuera de mi arbol — cuesta: mensaje de descarga sigue en ingles.
- Esperando OK del lider a la lista antes de la fase 2 (plan con superpowers:writing-plans).

## Fase 2 - P1 (barra y a11y base)
- Lider aprueba P1-P6 en orden; O1 pasa a `seguridad`; P5 el ultimo con opus y regla "ningun cambio da al agente forma de aprobar una 🔴"; O7 fuera; H10/H14 solo si son textos/CLI sin daemon ni clasificador. ux/revision subida (05367ee).
- Gradle: JAVA_HOME jdk-17.0.20.1+1 y /c/Users/Jose/AppData/Local/Gradle/gradle-9.8.0/bin/gradle --no-daemon --offline (desde android/); android/local.properties creado (no versionado).
- Plan P1: docs/superpowers/plans/2026-10-03-ux-p1-barra-y-a11y.md. Revision opus (a7090bb): 2 bloqueos + 5 importantes, todos incorporados.
- Ruling: la altura de AdbuddyTopBar sale del contenido (2*lineHeight+8, min 64) y no de un umbral de fontScale — un umbral falla en moviles estrechos con letra 1,3 — si me equivoco: lineHeight sin especificar en el tema (el test lo caza).
- Ruling: assertTitleFitsTheTopBar mide la barra PINTADA (testTag) — una constante compartida con el codigo haria pasar un test aunque la barra no creciera — cuesta: un tag en produccion.
- Ruling: C23 (borrar de 18 dp en chips) aplazado a P3 — cambiar el chip pide diseno — cuesta: area tactil pequeña unas semanas mas.
- Ruling: SecondaryActionButton no se aplica a ConsentScreen aqui (Screens.kt lo toca seguridad para O1) — se completa en P5.
- Task 1 (P1): complete (commits 3853d58..11509e5, review clean; Sonnet implementer aa9c6774, reviewer a1cdd9a7).
- Task 1: minor (deferred, a Task 4/5 cleanup): imports sin uso en SettingsScreen.kt (semantics, heading), ActivityScreen.kt (Icons); @OptIn(ExperimentalMaterial3Api) probablemente sobrante en SettingsScreen/ActivityScreen; heading() en ConsentScreen (Screens.kt:105) se hace en P5; aserción de que la barra pintada mide >= height(twoLines).
- Ruling: hallazgo O3 era en parte falso positivo — el TopAppBar de M3 1.4.0 ya crecia con el titulo y asertTitleFitsTheTopBar medía contra una constante de 64 dp; el valor real de T1 es la barra medible (testTag), titulo de 2 lineas con elipsis vigilada, heading() y altura minima coherente. Las access_*_font_200_es no cambiaron (coherente). Se queda como esta — revertir expandedHeight solo ahorraria ~5 dp en titulos de 1 linea a 200%.
- Task 2 (P1): complete (commits 0a1c48d..cf80635, review clean; minor: assertEquals en vez de assertTrue en SecondaryActionButtonTest:53).
- Task 3 (P1): complete (commits 15f9001..0b7e897, review opus clean).
- Task 3: minor (deferred, limpieza en el dispatch de la Task 4): tests extra de RemoteCardSemanticsTest (remoto activo: tocar abre remote_disable_title y no envía; PENDING: la fila no es accionable); KDoc de RemoteCard.kt:48-49 dice que al apagar "does not ask" y el código siempre pregunta.
- Task 4 (P1): complete (commits dfbbf2d..49acc22, review clean; incluye limpieza de menores 1-3: OptIn/imports, tests de RemoteCard con remoto activo, KDoc exacto; test PENDING omitido: estado local no construible).
- Task 4: minor (se arregla en la Task 5): es usa "archivo" en file_action_download/open y "fichero" en el resto del fichero (3 vs 2): unificar a "fichero".
- P1: cerrado (revision final opus limpia; fragmento corregido M1/M2). Rango 3853d58..ac85310 + fix de fragmento.

## Fase 2 - P2 (onboarding y estados del chat)
- P1 integrable: subida y avisado al lider (9e25b29). Plan P2: docs/superpowers/plans/2026-10-03-ux-p2-onboarding-y-estados.md (b9f90b2), revisado por adbuddy-reviewer (a1a1f08a): 3 bloqueos (flujo de test roto, sintaxis, firma posicional), 7 importantes e importantes de scroll, todos incorporados.
- Ruling: C13 (tarjeta modo remoto compacta), "pensando" y reconectando/esperando de C20, aviso de depuracion inalambrica y enlace a troubleshooting de O2, pista de retomar en paso 1 de O10 — aplazados — piden diseño o estados nuevos — cuesta: chat con 88 dp fijos de tarjeta y sin indicador de espera unas semanas mas.
- Ruling: ChatHeader toma altura del contenido (AdbuddyTopBarSpec) y TOP_BAR_TAG — el TopAppBar no recorta y con 200 % titulo+subtitulo pasan de 64 dp — cuesta: barra algo mas alta con letra grande.
- Ruling: Task 6 (scroll) calcula "estaba abajo" con layoutInfo y fila vista, no con canScrollForward — canScrollForward cambia al crecer el contenido y mezcla cresce con subio — cuesta: algo mas de logica; 6 tests lo vigilan.
- Ruling: chat_empty_ready sin promesa absoluta de seguridad ("Los comandos delicados piden tu confirmacion") — hay modos donde la pregunta sale en el PC — cuesta: texto menos reconfortante.
- Rama: P2 en ux/p2 (desde ux/revision, el PR de P1 no cambia: ux/revision reposicionada en 9e25b29). Cada paquete en su rama (ux/p3...). Un solo worktree: ux.
- P2 Task 1: complete (commits 77c5c9d..63ab8d8; fix round 1/5: recordatorio del pie al cuerpo con scroll — 1 addressed, 0 open). Implementer a2db977b, reviewer a19fde65, re-reviewer a53abd94.
- P2 Task 1: minor (deferred, limpieza final de P2): test que el recordatorio access_later_hint NO existe con Effective.ON (stepTwoWithTheAccessOnHasNoNotNow); la rama fila de ActionFooterTest no se prueba con etiqueta larga.
- P2 Task 2: complete (commits 6035929..f6991b2, review clean). minor (limpieza final de P2): OnboardingScreensTest.stepTwoNotificationAccess sin assertExists de access_restricted_hint; orden de imports en NotificationAccessScreensTest.
- P2 Task 3: complete (commits 3bee8dc..ba81a47, review clean; consentimiento intacto). minor (limpieza final de P2): ConsentFeedbackFlowTest: import assertIsDisplayed sin uso, comprobar "paso 1 de 2" tras rechazar, assertFalse en vez de assertEquals(false,..).
- P2 Task 4: complete (commits 23754fd..eb7b114, review clean; 20 PNG nuevos chat_*). minor (limpieza final de P2): ChatScreensTest: import getOrNull sin uso, lineCount==1 del subtitulo dentro del if(banner != null) (mover fuera), assertEquals final repetido en updateAppShowsTheWholeCommandInTheBanner.
- P2 Task 5: complete (commits 2e9eaa6..e376e95, review clean). minor (limpieza final de P2): ChatScreen.kt calcula LinkStatus.of dos veces (extraer val status); tests de "conectado sin mensajes con tools" y "con outgoing pendiente" sin estado vacio.
- P2 Task 6: complete (commits a49cdbb..a7722ba; fix rounds 2/5: ronda 1 (fix de tira/clave/onClick: 3 addressed), ronda 2 (quitar `following` por atasco: addressed)). Implementer a4d74991, reviewer a58b3869, re-reviewers a55255ff y a54eb382.
- Ruling: criterio de scroll sin estado `lastVisibleIndex >= seen-1` (desviado del brief `offset+size<=viewportEnd`, que fallaba con la burbuja que crece en sitio) — cuesta: quien lee solo el INICIO de una ultima burbuja muy alta es llevado al final al llegar un mensaje; y los tests de "cambios seguidos" no demuestran RED (humo util), aceptados.
- P2 Task 6: minor (limpieza final): twoTallArrivals sin shown() del ultimo mensaje; layoutInfo puede ir un fotograma desfasado (vigilado por el test de la tira que desaparece); documentar en el fragmento que el crecimiento en sitio de un mensaje (streaming) no genera aviso con el lector arriba.
- P2 Task 7: complete (limpieza de menores 5162abf, fragmento c8ce8a7). Revision final opus (a1f61f6d): 0 criticos, 1 importante (banner parpadea en CONNECTING) -> dc8d4d4; re-revision halla banner obsoleto (CUT/UPDATE/PAUSED) -> d5c56f1; re-revision final limpia. Suite 772 tests + lint verdes.
- Ruling: tras la revision final se hizo un segundo arreglo (d5c56f1) aunque la skill dice "una sola oleada" — era un fallo de mi propio arreglo, de dos lineas, y dejarlo mostraba "ADB access is cut" tras quitar el corte — cuesta: una revision mas.
- Ruling: M1/M3/M2 de la final (configured obsoleto sin cambio de conexion, ChatHeader recalcula LinkStatus, aviso de rechazo seguido de paso 1) — DEFER, notados en el fragmento/ledger — cuestan: edge cases raros.
- P2: listo para PR (ux/p2, d5c56f1).

## Fase 2 - P3 (chat, adjuntos, terminal, diario) en la rama ux/p3 (desde ux/p2)
- P2 subido (ux/p2 76c1407, avisado al lider). Plan P3: docs/superpowers/plans/2026-10-03-ux-p3-chat-adjuntos-terminal.md, revisado por adbuddy-reviewer (aec45a0f): 3 bloqueos de compilacion/test y 6 importantes, todos incorporados (QuickKeysRow(enabled primero), DraftStatus.DONE, DraftChips test apilado, FlowRow de filtros, Parar solo con barra vacia y con reposo anti doble toque, hint de undo con id validado, test obligatorio de la burbuja, fecha si no es de hoy, notify() con cierre propio).
- Ruling: C16, C19 (JS del terminal y teclas), persistencia de C12, "Restaurar" tras cut_done (C11) — aplazados — piden diseño o tocan el corta-accesos — cuesta: captura de la propia app, terminal sin zoom, restaurar solo por menu.
- Ruling: el botón Parar queda deshabilitado 3 s tras pulsarlo — agent.interrupt no tiene respuesta y un segundo toque mandaria un segundo ESC — cuesta: si el agente no para, hay que esperar 3 s para reintentar.
