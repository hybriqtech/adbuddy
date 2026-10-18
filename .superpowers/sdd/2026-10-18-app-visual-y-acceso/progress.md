# SDD ledger — plan: docs/superpowers/plans/2026-10-18-app-visual-y-acceso.md

Ledger de UX1 (aspecto visual de la app y estado del acceso), 2026-10-18. Compañero `ux`, rama `ux/visual` desde 01ce0672. Spec: `docs/superpowers/specs/2026-10-18-app-visual-y-acceso.md`.

## Entorno

- Un solo Gradle a la vez en este árbol; comandos Gradle con timeout <= 20 min; nada de emuladores ni adb contra el móvil del usuario (puerto 5038 intocable).

## Rulings

- R-ux1. Re-revisión del arreglo del plan (`adbuddy-rereviewer`, 01ce0672): nada bloquea; 5 hallazgos de documentación/tests corregidos en el plan y la spec antes de la tarea 1 (spec §7.2 con claves duplicadas y `restore_confirm_body` viejo; texto "te lleva a Opciones de desarrollador" que el código no hace → "te dice cómo hacerlo"; `LaunchedEffect(status)` después de `val status`; test nuevo `theActionCardButtonRefusesCoveredTouches` en `TouchGuardSourceTest`; `theRestoreDialogClosesWhenTheCutEnds` con segundo corte) — por qué: eran errores del plan que habrían dado `Unresolved reference` o un filtro de toques sin test — qué cuesta si me equivoco: nada que ejecutar; solo texto de plan.
- R-ux2. También aplicados los menores 6 (centrado del estado vacío con `Box` + `Column` con scroll) y 7 (`theTerminalStartsFolded` para R15, con mutación). No aplicado el 8 (acortar `status_waiting`) — por qué: la clave también alimenta el titular de la notificación (`LinkStatus.WAITING`, `ConversationViewTest`), recortarla cambiaría tests y notificación fuera del alcance — qué cuesta: la tarjeta WAITING repite "Buscando tu PC…" en el cuerpo (el título es otro).
- R-ux3. Observación abierta (no bloquea): con registro 0/0 y lecturas falsas, `restore()` devuelve false para siempre y el estado queda en CUT; lo cubre R3 (canal vivo gana) y V1. Se anota para el líder, no se cambia.

## Tareas

(se rellena al avanzar)

### T1 — 5dc01e77 — CONFORME (opus, 1 ronda)
Tests: AccessDiagTest 3/3, AccessSwitchTest 10/10, NoHardcodedTextTest 3/3, StringsParityTest 4/4. Mutaciones (take(16), saltos de línea, null->raw:0, "-" por "0", quitar línea de i18n-exceptions) en rojo; el revisor las repitió en copia.
- Ruling: no endurecer el valor leído a `[0-9-]` (hallazgo menor 1) ni añadir el test de "log nunca lanza" (menor 2) — por qué: quien puede escribir el ajuste con adb ya puede leer el logcat, y `log` solo es diagnóstico (V1) que se quita o se deja al cerrar — qué cuesta si me equivoco: un valor raro de hasta 16 caracteres en logcat; si alguien quita el try/catch de `log` ningún test lo ve (el try/catch está a la vista en el código).
- Entorno: `gradle` no está en el PATH; se usa `C:\Users\Jose\AppData\Local\Gradle\gradle-9.8.0\bin` y `ANDROID_HOME=C:\Users\Jose\AppData\Local\Android\Sdk` (sin local.properties).

### T2 — f164fb4b + 02066e4f — CONFORME (opus, 2 rondas)
Ronda 1: CONFORME con 2 menores; tests: control.* 59 (AccessSwitchTest 20, CutAccessTest 1, AccessDiagTest 3, ConversationViewTest 20, LinkStatusTest 8, NoHardcodedText 3, StringsParity 4). 6 mutaciones en rojo (el revisor repitió 3+ en copia), ningún test existente tocado salvo `FakeSettings`.
Ronda 2 (02066e4f): hallazgo real del revisor: un registro caducado sobrevivía a un segundo corte (`cut()` no llamaba a `forgetIfRestored()`; regresión pequeña frente al código anterior) -> `forgetIfRestored()` al principio de `cut()` + test `aStaleRecordIsForgottenBeforeTheNextCut` (mutación en rojo); quitado `val resolver` sin uso. control.* 25/25, assembleDebugAndroidTest compila.
- Ruling: el hueco del registro caducado se cierra en la tarea 2, no en la 8 como proponía el revisor — por qué: es la regla de cortar/reactivar y no se aparca — qué cuesta si me equivoco: una línea más en `cut()`.

### T3 — 5415fe2b — CONFORME (opus, 1 ronda)
Tests: LinkStatusTest 9, ConversationViewTest 21, ChatScreensTest 72 (verify), NoHardcodedTextTest 3, StringsParityTest 4, AskNoticeTest 7. Mutaciones (cut primero; cut antes de pausa; solo pausa deja de ganar; cut tras sin configurar) en rojo, repetidas por el revisor. 4 capturas `chat_cut_*` miradas por el revisor (claro, oscuro, 200 %, 200 % es): texto entero, sin "Listo" junto al corte.
- Ruling: no añadir la aserción de notificación con canal vivo + pausa + corte (menor 1 del revisor) — por qué: la cubre `LinkStatusTest` y `ConversationView` delega en `LinkStatus.of` — qué cuesta si me equivoco: si alguien cambia `ConversationView` para no delegar, ese caso no lo ve ningún test.
- Para T8: la cabecera en pausa dice "Listo" (`ChatParts.kt:110-128`, bug 9) y con canal vivo + `cut=true` el menú ofrece reactivar mientras la pastilla dice "Conectado" (aceptado en R3; reactivar va por diálogo). El revisor de T8 debe comprobar ambos.

### T4 — bd8d0a58 — CONFORME (opus, 1 ronda)
Tests: OverlayColorsTest 2, OverlayTest 2, NoHardcodedTextTest 3, StringsParityTest 4 (11/11). Mutaciones: texto oscuro en blanco y `system_accent1_600` de vuelta en rojo; una propia (`setTextColor(0xFFFFFFFF)` en Overlay.kt) sobrevive. Contraste píldora 6,61:1 claro / 7,80:1 oscuro. Sin PNG. `GuardOnEmulatorTest` en API 30 no se ejecutó (sin emuladores): validación de emulador pendiente (fragmento ux-visual.md).
- Ruling: los menores 1 (test de fuente que exija `setTextColor(colors.text)` y `setColor(colors.background)` en Overlay.kt) y 2 (assert de que `OverlayColors.of(d)` == `Palette` primary/onPrimary) se aplican en la tarea 5, que toca `Palette` — por qué: la mutación "texto blanco fijo" (bug 6) sobrevive hoy y la paleta solo coincide tras T5 — qué cuesta si me equivoco: el bug 6 podría volver sin que ningún test lo vea.
- Aplazados sin acción: 3 (marco 7FD8C6 sobre blanco 1,67:1, no es texto y ya era así), 4 (⏸ sin selector de variación, lo pide la spec), 5 (nombre `Pair`).

### T5 — 86dd1a65 — CONFORME (adbuddy-reviewer, 1 ronda)
Tests: suite completa del implementador 1094/1094; revisor: PaletteContrastTest 5, OverlayColorsTest 3, NoHardcodedTextTest 3, StringsParityTest 4 y `screens.*` 189 (verify) en verde. 4 mutaciones en rojo (repetidas). 113 PNG regrabados (todos los que había); 20 miradas por el revisor (claro, oscuro, 200 %, 200 % es): sin lila, texto entero, sin regresiones. Incluye los tests (a) y (b) pedidos por la revisión de T4.
- Ruling: los menores 1-2 (contraste: `outline` ≥3:1 sobre las superficies; `primary` sobre `errorContainer`; `success`/`warning` sobre `surfaceContainer*`) se añaden en la tarea 7 — por qué: son los pares que usará `ActionCard` y T7 es la que los introduce en pantalla — qué cuesta si me equivoco: un cambio futuro de `outline` o de un contenedor no rompería ningún test hasta T7.
- Observación: `draft_chips_light.png` tiene el carril de progreso apenas distinguible; se revisa en T13 (chips).
- `BubbleShapes` existe pero se usa desde T9.

### T6 — f0ce49b1 — CONFORME (adbuddy-reviewer, 1 ronda)
Tests: AppIconsTest 3, ConfirmNotificationTest 8, BubbleFlowTest 8, ConversationViewTest 21, NoHardcodedTextTest 3, StringsParityTest 4 (47/47); assembleDebug y lintDebug verdes. 2 mutaciones en rojo (repetidas). Evidencia visual: render del implementador y del revisor (burbuja con `>_` dentro de la zona segura de 66 dp, monocroma con `>_` hueco, escudo relleno y tachado reconocibles a 24 dp). Sin PNG del repo cambiados.
- Ruling: no se añade el test de "🔴 usa ic_stat_shield / el resto ic_stat_adbuddy / setColor / roundIcon" (menor 2) ni se endurece `theNotificationIconsExist` (menor 1) — por qué: fuera del plan y los notifiers tienen 7 builders a la vista; el comportamiento (acciones, pantalla bloqueada) está cubierto por ConfirmNotificationTest/ConversationViewTest — qué cuesta si me equivoco: cambiar el icono de una notificación por otro propio no lo vería ningún test.
- Pendiente de validación (fragmento ux-visual.md): V3 icono monocromo en el lanzador de Pixel; icono en lanzador de emulador.

### T7 — 3f25531e + ae2c4c5d — CONFORME (opus, 2 rondas)
Ronda 1: CONFORME con 2 huecos importantes de test (mutaciones propias del revisor sobrevivían: `pendingQuestions > 1` y `notificationsBlocked && banner != null && banner != CONNECTED`). Tests (252/252 verify): HomeNoticeTest 5, TouchGuardSourceTest 7, TouchGuardBehaviorTest 3, PaletteContrastTest 5, NoHardcodedText 3, StringsParity 4, screens.* 225 (ActionCardScreensTest 36). 3 mutaciones declaradas en rojo, M1b (filtro movido al Surface) también rojo. 36 PNG `action_card_*` nuevos + 8 regrabados; 19 miradas por el revisor: texto entero, contraste legible.
Ronda 2 (ae2c4c5d, solo tests): `oneQuestionIsEnough`, `blockedNotificationsShowWhateverTheLink`; comentarios de PaletteContrastTest corregidos. HomeNoticeTest 7, PaletteContrastTest 5; las dos mutaciones en rojo.
- Ruling: la ronda 2 (solo añade tests, 13 líneas) la reviso yo leyendo el diff, sin nuevo re-revisor — por qué: no hay código de producción, el implementador comprobó las dos mutaciones y el diff es legible de un vistazo — qué cuesta si me equivoco: un test mal escrito que pase sin proteger; lo vería el revisor final.
- Ruling: QUESTIONS mantiene el icono de las demás tarjetas rojas (menor 4: se parece a UPDATE_APP/UPDATE_HOST) — por qué: el plan no pide icono propio y se distinguen por texto y botón — qué cuesta: menos distinción a primera vista; decidirlo el líder si quiere (candidato a T14).
- Para T8: PAUSED + notificaciones bloqueadas no ofrece [Reanudar] en la tarjeta (la tarjeta de notificaciones gana): comprobar que se puede reanudar desde la cabecera/menú. `setup_guide_url` aún no se usa: T8 abre la guía (comprobar apertura segura). `lastBanner` en CONNECTING queda para T8.

### T8 — 15201243 + 72cc71ff — CONFORME (opus, 2 rondas)
Ronda 1: CONFORME con 1 importante. Suite completa del implementador 1201/1201 (verify), assembleDebug/lintDebug/assembleDebugAndroidTest verdes; revisor: 221 tests concretos y `screens.*` 285/285; 9 mutaciones (escudo `if (cut)`, ⋮ que restaura, restaurar sin diálogo, sin LaunchedEffect, sin ObscuredTouchGuard del diálogo, sin ítem de pausa, sin `takeIf CONNECTED`, WAITING sin intent, HomeNotice sin preguntas) todas en rojo. Tests protegidos adaptados sin quitar aserciones (ConfirmReopenTest, ChatScrollTest, QuestionsCardTest, ChatPendingQuestionsTest). 44 PNG regrabados, 12 nuevos, 8 borrados.
Desviaciones aceptadas por el revisor: (a) `ChatHeaderBar` propia en vez de `TopAppBar` (al 200 % fallaban 16 tests por texto cortado) con la pastilla `RoundedCornerShape(16.dp)` y hasta 3 líneas; la única aserción relajada es "la pastilla ocupa una línea" solo en 200 % (a 1x se mantiene); `AdbuddyTopBar*` intactos; (b) `stringResource` en vez de `context.getString` (lint); (c) `agent_fallback` no añadida aún (la añade T10); (d) sin test unitario del `ActivityNotFoundException` (el código lo captura); (e) estado vacío al 200 % es con tarjeta se corta: lo arregla el scroll de T9.
Ronda 2 (72cc71ff): hallazgo importante: nada probaba el cableado de ChatRoot (cut/restore intercambiados no lo veía ningún test) -> `ChatRootCutWiringTest` (escudo corta ambos ajustes y sale `cut_done`; registro 0/0 pregunta, dice `restore_manual` y no escribe; registro propio 1/0 se restaura tal cual y sale `cut_restored`); `@OptIn` sobrante e imports ordenados. Diff de producción: solo el `@OptIn` quitado; revisado por mí.
- Ruling: la ronda 2 (test + limpieza) la reviso yo leyendo el diff, sin re-revisor — por qué: el diff de producción es una línea y el test es legible — qué cuesta si me equivoco: un test que no proteja el cableado; el revisor final lo ve.
- Ruling: icono propio por tipo de tarjeta (hallazgo menor 2 del revisor: PAUSED/QUESTIONS/UPDATE comparten el escudo relleno del botón Cortar) -> T14 (vectores propios: pausa, pregunta, actualizar), junto a la duda de T7 — por qué: no es seguridad, hay que dibujar 3 vectores y revisarlos a ojo — qué cuesta si me equivoco: hasta T14 el escudo relleno de la cabecera y el de las tarjetas se parecen.
- Para T10: añadir `agent_fallback` ("The agent"/"El agente") a su lista de cadenas.
- Para la spec (T14): anotar que la pastilla es `RoundedCornerShape(16.dp)` hasta 3 líneas y la cabecera propia, no una cápsula de una línea al 200 %.

### T9 — 261a2abb — CONFORME (adbuddy-reviewer, 1 ronda)
Tests (217/217 revisor; suite completa implementador 1217/1217): ChatScreensTest 152, ChatRootCutWiringTest 4, ChatScrollTest 12, InputBarTest 8, TouchGuard* 10, StringsParity 4, NoHardcodedText 3, flows verdes. 4 mutaciones (sugerencia que envía, terminal abierta por defecto, sin `|| PAUSED`, sin `verticalScroll`) en rojo, repetidas. 20 PNG modificados + 8 nuevos; 16 vistos por el revisor: texto entero, chips, burbujas 18/18/18/4. Test adaptado `OnboardingFlowTest.notNowTwice...` (terminal plegada, R15) sin quitar aserciones.
- Menor 1 (KDoc de `terminalOpenByDefault` en ChatScreen.kt:102 desactualizado) se arregla en el commit de T10. Menor 3: no hay captura del estado vacío con tarjeta en 200 % que muestre logo y texto (la aserción de alcanzabilidad lo cubre). Menor 2 (texto de pausa repetido en tarjeta y estado vacío) lo pide la spec §5.

### T10 — 1173963b — CONFORME (opus, 1 ronda)
Tests (revisor 230/230; suite completa implementador 1223/1223): ChatScreensTest 152, ConversationViewTest 21, PermissionCardScrollTest 12, TouchGuard* 10, Obscured* 15, PermissionCardScreensTest 4, PermissionCardSemanticsTest 3, ConfirmTextDirectionTest 3, OnboardingFlowTest 3, StringsParity 4, NoHardcodedText 3. Diff de PermissionCard.kt solo en firma, título y estilo del detalle (scroll vigilado, `enabled`, orden, peso y filtros intactos). 5 mutaciones en rojo (título sin agente, detalle sin Ltr, conceder sin leer, sin ObscuredTouchGuard; "No" al principio solo lo ve la captura). 4 PNG nuevos vistos por el revisor.
- Ruling: no sanear `SessionState.agent` (menor 1: bidi/longitud sin límite en título y cabecera) — por qué: lo pone el host con la misma confianza que el resumen y el detalle que el agente ya controla, el título queda fuera del scroll vigilado y no contiene el comando, y la cabecera ya lo mostraba así — qué cuesta si me equivoco: un nombre engañoso reordena "asks" o agranda la tarjeta; seguimiento posible (sanear una vez al recibir `SessionState.agent`: quitar control/bidi, 40 caracteres, maxLines=1). Se lo señalo al líder.
- Se aplican en T11: quitar `scrolled = true` de `PermissionCardScreensTest.kt:47` (menor 2) y añadir un test semántico del orden de los botones de la tarjeta (menor 3: hoy solo lo ve la captura, que en la CI Linux no falla).
- `AskNoticeTest` no se ejecutó en la revisión; la suite completa del implementador (1223) lo incluye.
