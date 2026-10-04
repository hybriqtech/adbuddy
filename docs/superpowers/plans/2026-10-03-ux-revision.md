# Revisión de UX (app Android y CLI `adbuddy`) — 2026-10-03

Fase 1 del encargo "UX de la aplicación y de la app de Android". Solo lectura: tres revisores (chat y confirmaciones; primer arranque, Ajustes y accesibilidad; CLI del PC) con código verificado, imágenes de referencia vistas y el binario `adbuddy` ejecutado sin móvil. El top 10 está comprobado de nuevo por el controlador.

Rutas de la app relativas a `android/app/src/main/kotlin/tech/hybriq/adbuddy/` salvo `res/` y `test/`; las del CLI, a `host/internal/`. Severidad: **bloquea** (hueco funcional o de seguridad), **molesta**, **pulido**. Coste: S (horas), M (un día), L. **SEG** = toca confirmaciones, consentimiento, emparejamiento o superficie de seguridad: no se hace sin revisión `adbuddy-reviewer-opus`.

Lo que ya está bien y no se rompe: tarjeta 🔴 con scroll obligatorio hasta el final antes de poder permitir y `filterTouchesWhenObscured`; el consentimiento de notificaciones distingue "permitido en el sistema" de "confirmado aquí"; estado "Comprobando…"; `StateCard` con icono y texto (nunca solo color); descartar cambios con confirmación; filas de apps protegidas con `toggleable`; errores del CLI con `Error: CODE — msg` y `Fix:`, `--json` con envelope, códigos de salida semánticos; `doctor` con el comando a ejecutar en cada línea; stdout/stderr bien separados.

## Top 10 (valor/coste, propuesto para aprobar)

| # | Hallazgos | Qué | Coste | Seguridad |
|---|---|---|---|---|
| 1 | C2, C3, O4 | Una 🔴 cerrada o sin avisos no tiene vuelta: no cerrar con toque fuera, banner fijo "N acciones esperando · Ver" que reabre la misma tarjeta, y banner/permiso con motivo si las notificaciones están bloqueadas | S-M | SEG (misma tarjeta, sin cambiar la semántica; revisión opus del plan) |
| 2 | C1 | La pregunta de permiso del agente no suena ni sale heads-up con la app en segundo plano: canal propio (importancia alta), texto genérico | M | no (el texto sigue sin tool ni comando) |
| 3 | O1 | `ConsentScreen` y los diálogos del modo remoto no filtran toques tapados (las demás pantallas de concesión sí) | S | SEG |
| 4 | O3, O13 | `AdbuddyTopBar` único con altura adaptable a la letra, `maxLines = 2` y `heading()`: cierra `KNOWN_UI_ISSUES` | S | no |
| 5 | O5, O10 | Paso 2 de 2: "Ahora no" y "Listo" hacen lo mismo; rechazar el consentimiento no da feedback ni dice cómo retomarlo | S | no (mínimo; no se mueve el botón de confirmar) |
| 6 | O6, C14 | Interruptor del modo remoto sin nombre para TalkBack y con área táctil de 52 dp: `toggleable(Role.Switch)` en la fila, el diálogo de confirmación se queda | S | no |
| 7 | C4, C20, O2 | Estado del chat: cabecera de una línea y banner con el texto accionable; estado vacío; "esperando al PC" falso antes de `setup` (`NOT_SET_UP`) y depuración inalámbrica apagada | M | no |
| 8 | C5 | Botón "Parar" (`agent.interrupt` ya existe en el protocolo y ninguna UI lo usa) | S | no (solo reduce lo que hace el agente) |
| 9 | C6, C7 | TalkBack en el chat (`liveRegion` en las tarjetas, `heading`, quién habla) y cuenta atrás en el diálogo 🔴 (hoy solo hora absoluta, al final del scroll) | S-M | C7 SEG leve (solo presentación) |
| 10 | H3, H1, H8, H4 (y H6) | CLI: backticks que rompen el `--help` de 5 flags, `Fix:` de uso que apunta a `--help` de la raíz, plurales ingleses ("1 phone(s)"), `help nope` con exit 0; después grupos de comandos y ejemplo raíz | S (H6 S-M) | no |

Paquete aparte, **solo con revisión opus** (textos de consentimiento o semántica visible de confirmación): C8, C9, C10, O9, O12, O21 (detalle abajo).

## Hallazgos — chat, confirmaciones, notificaciones, ficheros, terminal (C)

| ID | Sev | Evidencia | Problema y propuesta | Coste | SEG |
|---|---|---|---|---|---|
| C1 | bloquea | `notify/ConversationView.kt:43,80`; `notify/ConversationNotifier.kt:72`; `AdbuddyApp.kt:112` | La pregunta del agente solo cambia el titular de la notificación ongoing (canal LOW, `setOnlyAlertOnce`) y `shouldAnnounceDone` suprime el "Done" mientras hay pregunta: con el móvil en el bolsillo caduca sin que nadie se entere. Canal `ask` de importancia alta con texto genérico que abre la app; sin botones de respuesta | M | no |
| C2 | bloquea | `ui/Screens.kt:161-162`; `ui/ConfirmHost.kt:37`; `MainActivity.kt:25-26,85-86` | El toque fuera o atrás cierra el diálogo 🔴 y solo la notificación lo reabre; el chat no muestra `state.pending`. `dismissOnClickOutside = false` y banner fijo "N acciones delicadas esperando · Ver" que quita el id de `closed` | S | SEG |
| C3 | bloquea | `MainActivity.kt:32-37` | Permiso de notificaciones pedido una vez sin explicación y sin banner si está denegado o el canal `confirm` en `IMPORTANCE_NONE`: las 🔴 solo se ven con la app abierta y el agente queda esperando. Banner en el chat con atajo a los ajustes; ver también O4 | S-M | SEG leve |
| C4 | molesta | `ui/ChatParts.kt:96-129`; `res/values/strings.xml:27` | Cabecera de alto fijo con dos líneas y `maxLines = 2`: `status_update_app` (con `adbuddy setup --apk`) y `CUT`/`WAITING` quedan cortados con elipsis. Cabecera de una línea y banner de cuerpo con el texto completo | S | no |
| C5 | molesta | `bridge/Protocol.kt:41`; `bridge/Registry.kt:44`; `ui/ChatScreen.kt:131` | Sin botón de parar con el agente en WORKING y el terminal oculto. "Parar" en lugar de "Enviar" cuando `SessionLabel.WORKING` | S | no |
| C6 | molesta | `ui/` y `confirm/` sin `semantics` (solo `SettingsScreen.kt:96` usa `heading()`); `ui/ChatParts.kt:201-230` | Las tarjetas 🔴 y del agente no se anuncian a TalkBack y las burbujas no dicen quién habla. `liveRegion` en el título de la tarjeta, `heading()`, prefijo o `contentDescription` "Tú:"/"Agente:", `stateDescription` en la cabecera | S-M | no |
| C7 | molesta | `ui/Screens.kt:195`; `res/values/strings.xml:43` | La caducidad del diálogo 🔴 es una hora absoluta al final del contenido con scroll; los botones desaparecen sin aviso (la tarjeta del agente sí tiene cuenta atrás). Línea fija sobre los botones "Caduca en 0:42", en rojo por debajo de 10 s, sin `liveRegion` por segundo | S | SEG leve |
| C8 | molesta | `ui/Screens.kt:198-220` | Deny, Allow y "Allow during the session" son `TextButton` iguales; los dos permisos, apilados; el aviso de scroll al lado de Deny. Deny tonal primero, Allow como `Button`, el de sesión separado y más discreto; el aviso de scroll al cuerpo | S | SEG (orden y énfasis de botones) |
| C9 | molesta | `res/values/strings.xml:33` | "Allow during the session" no dice el alcance (spec §4.5: la misma regla con los mismos argumentos). "Allow this same command during the session" / "Permitir este mismo comando durante la sesión" | S | SEG (texto) |
| C10 | molesta | `confirm/ConfirmNotifier.kt:97-108` | La notificación 🔴 enseña el comando en la pantalla bloqueada (sin `setVisibility`/`setPublicVersion`; la de conversación sí). `VISIBILITY_PRIVATE` con versión pública genérica; decidir si se pierde el botón Rechazar en el bloqueo | S | SEG |
| C11 | molesta | `ui/ChatRoot.kt:68,117`; `res/values/strings.xml:75` | Snackbars de 4 s para avisos largos (`cut_restored` avisa de que el puerto sigue abierto). `Long` o `Indefinite` en esos, y acción "Restaurar" tras `cut_done` | S | no |
| C12 | molesta | `ui/ChatScreen.kt:89,112-118`; `ChatBubbleActivity.kt` | Con terminal visible (por defecto), en horizontal, con teclado o en la burbuja, la lista queda en unas decenas de dp. Persistir la preferencia, terminal oculto por defecto en la burbuja, uno solo a la vez con altura menor de 480 dp | M | no |
| C13 | molesta | `ui/ChatScreen.kt:112`; `ui/RemoteCard.kt:79` | La tarjeta del modo remoto ocupa unos 88 dp fijos aunque esté apagado. Fila compacta en OFF | S | no |
| C14 | molesta | `ui/RemoteCard.kt:93-98` | Ver O6 (mismo defecto) | S | no |
| C15 | molesta | `ui/ChatParts.kt:268,279-283` | Tocar una descarga en curso la cancela sin avisar y la tarjeta no tiene rol ni `onClickLabel`. "Toca para cancelar" y rol `Button` | S | no |
| C16 | molesta | `files/Drafts.kt` (captura); spec M3 §6.3 | La captura desde el chat captura el propio chat. Mínimo: avisarlo en la etiqueta; caro (M): app a segundo plano y captura tras ~1 s | S/M | no |
| C17 | molesta | `files/Drafts.kt`; `ui/ChatParts.kt:314` | Sin feedback mientras se prepara un adjunto (`size == 0` no pinta progreso). Indicador indeterminado | S | no |
| C18 | molesta | `term/TerminalFeed.kt:109-113` | El terminal acepta teclas sin conexión e ignora el resultado de `channel.send`. Capa "Sin conexión" y chips deshabilitados | S | no |
| C19 | pulido | `assets/term/term.js:21-28`; `assets/term/index.html:5`; `term/QuickKeys.kt` | Terminal sin `screenReaderMode`, `fontSize` 12 fijo, `maximum-scale=1`, sin copiar ni zoom; faltan ←, →, Shift+Tab. `screenReaderMode` con TalkBack, tamaño según `fontScale`, A+/A-, copiar | M | no |
| C20 | molesta | `ui/ChatScreen.kt:116-130`; `ui/LinkStatus.kt`; `bridge/BridgeState.kt` | Sin estado vacío ni indicador de "pensando"; `WAITING` agrupa host caído, reconectando y sin red. Estado vacío con la pista de `adbuddy serve`, puntos de espera, diferenciar "reconectando" de "esperando al PC" | S-M | no |
| C21 | molesta | `ui/ChatScreen.kt:98` | `animateScrollToItem(rows-1)` en cada cambio devuelve al final a quien lee arriba. Autoscroll solo si ya estaba abajo; si no, chip "Mensajes nuevos" | S | no |
| C22 | molesta | `res/values/strings.xml:98,101,33`; `values-es:189` | Vocabulario incoherente: "Yes/No" frente a Allow/Deny, "denegado" en el diario, `perm_always` sin alcance, título fijo "Claude Code asks", "ADB access/debugging/access", "Working…" dos veces. "Permitir"/"Rechazar", título con `sessionState.agent` | S | no |
| C23 | pulido | `ui/ChatParts.kt:320-324` | Botón de borrar del chip de borrador de 18 dp. `IconButton` de 48 dp | S | no |
| C24 | pulido | `ui/PermissionCard.kt:127,129` | Estados offline/expirada/cuenta atrás en `labelSmall`, sin "Enviando…", `FlowRow` sin `verticalArrangement`; posible desborde al 200 % con teclado (por comprobar con captura). `bodySmall` en negrita, "Enviando…", scroll interno con tope | S | no |
| C25 | pulido | `ui/PermissionCard.kt:93-101`; `ui/theme/Palette.kt` | La tarjeta del agente usa `tertiary*`, que la paleta no define (sin color dinámico sale el rosa base de M3) y `PaletteContrastTest` no cubre. Definirlos y probarlos (ver O15) | S | no |
| C26 | pulido | `ui/ActivityScreen.kt:56-58,63,75` | Filtros solo con 🟢🟡🔴, vacío del filtro con el texto de "diario vacío", sin hora por fila, el inverso sin decir que se deshace con `adbuddy undo`. Etiqueta de nivel, hora, textos propios | S | no |
| C27 | pulido | `AndroidManifest.xml`; `confirm/ConfirmNotifier.kt:98,103`; `notify/ConversationNotifier.kt:56`; `AdbuddyApp.kt:112-114`; `ui/ChatScreen.kt:91` | Iconos del sistema, `CATEGORY_CALL` en la 🔴 (interfiere con No molestar), canales sin descripción, toggle del terminal con ↑/↓, `LinkDialog` no sobrevive a la rotación (`remember`) | S c/u | CATEGORY_CALL SEG leve |

## Hallazgos — primer arranque, Ajustes, consentimientos, accesibilidad (O)

El emparejamiento (QR, código, Tailscale) vive entero en el terminal del PC: la app no tiene pantalla de emparejar, y "QR no válido" o "token caducado" solo se ven como `WAITING`.

| ID | Sev | Evidencia | Problema y propuesta | Coste | SEG |
|---|---|---|---|---|---|
| O1 | bloquea | `ui/Screens.kt:78-124`; `ui/RemoteCard.kt:103-115`; `filterTouchesWhenObscured` solo en `NotificationAccessScreen.kt:76`, `ProtectedAppsScreen.kt:88`, `Screens.kt:174` | "Aceptar" del consentimiento concede secure settings, exención de batería y accesibilidad, y abrir el puerto 5555 en cualquier red: ambas sin protección contra tapjacking. El mismo `DisposableEffect` más un test que lo compruebe | S | SEG |
| O2 | molesta | `ui/LinkStatus.kt:28-36`; `res/values/strings.xml:25`; `MainActivity.kt:49` | Antes de `adbuddy setup` dice "run adbuddy serve", que no sirve; no distingue la depuración inalámbrica apagada. Estado `NOT_SET_UP` (sin `link`), aviso con `action_developer_options` si `adb_wifi_enabled == 0`, enlace a `troubleshooting.md` | M | no |
| O3 | molesta | `test/.../screens/ScreenTestSupport.kt:77`; `ui/NotificationAccessScreen.kt:134`; `access_not_allowed_font_200_es.png` | Título de dos líneas fuera de la barra de 64 dp (cuatro copias del mismo `TopAppBar`). `AdbuddyTopBar(title, onBack)` con `expandedHeight` mayor si `fontScale > 1.3`, `maxLines = 2`, `heading()`; quitar la entrada de `KNOWN_UI_ISSUES` y regrabar | S | no |
| O4 | molesta | `MainActivity.kt:34-37`; `notify/ConversationNotifier.kt:99`; `confirm/ConfirmNotifier.kt` | `POST_NOTIFICATIONS` se pide en frío encima del consentimiento y, si se rechaza, nada lo avisa. Pedirlo tras el onboarding con una línea de motivo; fila de aviso en Ajustes y en el chat con atajo | M | SEG leve |
| O5 | molesta | `ui/NotificationAccessScreen.kt:150-151`; `ui/ChatRoot.kt:136` | Paso 2 de 2: con el acceso sin activar, "Ahora no" y "Listo" hacen lo mismo y "Listo" es el relleno. Solo "Ahora no" si `effective != ON`; "Listo" con `ON`; pista "Puedes activarlo después en Ajustes" | S | no |
| O6 | molesta | `ui/RemoteCard.kt:79-101` | `Switch` sin etiqueta para TalkBack y solo el interruptor es táctil. `toggleable(role = Role.Switch)` en la fila, `Switch(onCheckedChange = null)`; el diálogo de confirmación se mantiene | S | no |
| O7 | molesta | `a11y/Overlay.kt:83-86,101,103-104`; `HIDE_AFTER_MS` | Pastilla del overlay: texto blanco siempre sobre `accent1_200` en oscuro (contraste bajo, estimado), botón Pausar de ~30 dp, sin rol, se oculta a los 4 s. Color según luminancia, `minHeight` 48 dp, delegado de accesibilidad. Cambiar el tiempo de ocultación es SEG | S | tiempo SEG |
| O8 | molesta | `ui/ChatParts.kt:143-157`; `docs/usage.md:206` | "Cortar acceso" ("the real kill switch") está en el menú ⋮ sin énfasis; cortado, solo una línea roja y "Reactivar" escondido. Icono y `error` en la entrada, banner con "Reactivar" si `cut`, fila en Ajustes | S/M | no |
| O9 | molesta | `res/values/strings.xml:65,5` | `consent_a11y` infravalora lo que concede (el agente lee la pantalla y toca); la nota de privacidad es el texto más pequeño. Ampliarlo y subir `consent_note` a `bodyMedium` sin cambiar lo que se concede | S | SEG (texto) |
| O10 | molesta | `MainActivity.kt:55-59`; `ui/ProtectedAppsScreen.kt:211`; `ui/NotificationAccessScreen.kt:150` | Rechazar el consentimiento no da feedback; aceptar muestra "Puedes volver al PC" y a continuación empieza el paso 1. Aviso al rechazar ("Ejecuta `adbuddy setup` en el PC para repetirlo"), "Faltan 2 pasos aquí; después vuelve al PC", cada paso dice cómo retomarlo | S | no |
| O11 | molesta | `ui/SettingsScreen.kt:116-155` | Nada dice si el servicio de accesibilidad está activo (Android lo apaga tras una actualización o un fallo; solo lo ve `adbuddy doctor`). Fila "Ayudante del agente: Activo/Apagado" con atajo a `ACTION_ACCESSIBILITY_SETTINGS`; solo lectura | M | no |
| O12 | molesta | `ui/ProtectedAppsScreen.kt:124-131`; `res/values/strings.xml:165` | El aviso de que lo recién protegido aún no lo está en el PC es un Toast de ~3 s que desaparece al navegar. Estado persistente "Pendiente de enviar al PC" en la fila de Ajustes hasta el siguiente `welcome` | M | SEG (estado de las protecciones) |
| O13 | pulido | `ui/Screens.kt:105`; `ui/ProtectedAppsScreen.kt:292` | Sin `heading()` en el consentimiento, `StepHeader` ni los títulos de Acceso y Apps protegidas (se resuelve con O3 y en `StepHeader`) | S | no |
| O14 | pulido | `ui/Screens.kt:86-89`; `ui/ProtectedAppsScreen.kt:211`; `ui/NotificationAccessScreen.kt:150` | "Ahora no" distinto en cada pantalla, con borde casi invisible. `SecondaryActionButton` compartido | S | no |
| O15 | pulido | `ui/theme/Palette.kt:10-35`; `settings_proposed_light.png` | Faltan `surfaceContainer*`, `outlineVariant`, `tertiary*`: la tarjeta "Remote mode" sale lila sobre fondo verdoso (Android 11 y goldens; con color dinámico no pasa). Definirlas y cubrirlas en `PaletteContrastTest` | S | no |
| O16 | pulido | `ui/LanguageSettings.kt:11-29`; `res/values/strings.xml:195-196` | `settings_language` no se usa; falta "Acerca de" (versión). Fila "Idioma" (Android 13+) y "Acerca de" | S | no |
| O17 | pulido | `AndroidManifest.xml:30` | Icono de lanzador del sistema (`sym_def_app_icon`). Icono adaptativo con capa monocromática | S/M | no |
| O18 | pulido | `res/values-es/strings.xml:130-131`; `res/values/strings.xml:64`; `ui/SettingsScreen.kt:125,136` | "Apagado/Activado" (debería ser "Desactivado"), "optimisation" en inglés británico, Toast frente a Snackbar, `error` para tareas pendientes. Pase de textos | S | no |
| O19 | pulido | `ui/ProtectedAppsScreen.kt:210-215`; `onboarding_1_protected_apps_font_200_es.png` | Botones del pie en `Row` aunque el comentario promete `Column`; "Confirmar la lista" en dos líneas al 200 %. Apilar a ancho completo si `fontScale > 1.3` | S | no |
| O20 | pulido | `ui/NotificationAccessScreen.kt:171-177`; `onboarding_2_notification_access_light.png` | La pista de "ajustes restringidos" (la más importante del paso) es `bodySmall` gris bajo el botón. Tarjeta con icono si el estado es `OFF` con interruptor gris | S | no |
| O21 | pulido | `res/values/strings.xml:170` | "Allowed in the system, not confirmed here": jerga y "aquí" ambiguo. "Android lo permite; falta tu confirmación" | S | SEG (texto) |

## Hallazgos — CLI `adbuddy` en el PC (H)

| ID | Sev | Evidencia | Problema y propuesta | Coste | SEG |
|---|---|---|---|---|---|
| H1 | molesta | `cli/errors.go:57,107-111`; `adbuddy tap` → `Fix: Run adbuddy --help…` | Todo error de uso manda al `--help` raíz. Usar `cmd.CommandPath()` del comando que falló (`root.ExecuteC()`) | S | no |
| H2 | molesta | `cli/errors.go:121-127`; `cli/help.go:16-19`; `adbuddy serv` con `ADBUDDY_LANG=es` | Errores de cobra en inglés con el `Fix:` en español y sugerencias pobres (`env`, `send` para `serv`). Traducir 5-6 patrones en `AsError` y sugerencia propia (prefijo o distancia ≤ 2, 1-2 candidatos); el JSON sigue en inglés | M | no |
| H3 | molesta | pflag toma lo de entre backticks como nombre del valor; `i18n/catalog/en.toml:107,144,170,193,226` y las de `es.toml` | `--no-agent adbuddy run   do not start…`, `--session undo session`: línea ilegible. Quitar los backticks y un test que falle si un `help.*.flag.*` los lleva | S | no |
| H4 | molesta | `adbuddy help nope` → exit 0, en inglés | Sin error normal. Sobrescribir `help`: tema inexistente → `UsageErrorT` con sugerencia | S | no |
| H5 | molesta | `cli/doctor.go:172,205`; `doctorTotalWait` 60 s | `doctor` no escribe nada hasta terminar (hasta 60 s). Callback `onCheck` que imprime cada línea (modo humano); `--json` igual; línea de progreso si stderr es TTY | M | no |
| H6 | molesta | `adbuddy --help` | 21 comandos en orden alfabético, `setup`/`serve`/`doctor` perdidos entre los del agente, sin `Examples:`. `AddGroup` "Primeros pasos / Día a día / Para el agente", `Example` raíz `setup` → `serve`, ejemplos que faltan en `attach`, `new`, `send`, `stop`, `env`, `version`; actualizar goldens en y es | S-M | no |
| H7 | molesta | 10 de 21 `Short` en > 80 columnas en inglés y 12 en español (hasta ~118); `Long` de es hasta 98 | Se rompe a 80 columnas. `Short` ≤ 60, `Long` a ≤ 80 y test de ancho | S-M | no |
| H8 | molesta | `en.toml`: `doctor.server.ok.one` ("1 phone(s)"), `journal.damaged_journal_lines_were_skipped.one`, `envcmd.skills_created.one` y `skills_updated.one` ("created 1 skills") | El `.one` en inglés repite el `.other`. Corregir las 4 y un test que compare | S | no |
| H9 | molesta | `cli/journal.go:67-69,98`; `cli/common.go:100` | `journal` corta a 50 sin avisar y sin fecha. Aviso a stderr "últimas N de M (usa `--limit 0`)", `total`/`truncated` aditivos en JSON, fecha si no es de hoy | S | no |
| H10 | molesta | `cli/serve.go:192-196,262` | En modo `auto` sin TTY se usa `DenyAll` sin avisar al arrancar. Una línea: "🔴 confirmaciones: en esta terminal / en el móvil / denegadas siempre (`--confirm terminal`)". Solo texto | S | SEG leve (que lo vea seguridad) |
| H11 | pulido | `cli/serve.go:118`; `en.toml:369` | "adb server (port 5038)" es jerga y no dice cuánto espera (45 s) ni qué mirar; el error de `setup --code` sin TTY lleva el `Fix` genérico. "Esperando al móvil (hasta 45 s). ¿Depuración inalámbrica activada y en la misma Wi-Fi?" y fix propio | S | no |
| H12 | pulido | `cli/doctor.go:326-332` | Sin nombrar `setup` cuando el móvil configurado no conecta (`serve` sí lo hace). Añadir "Si lo desemparejaste o es otro PC: `adbuddy setup`" | S | no |
| H13 | pulido | `cli/doctor.go:218`; `cli/setup.go:376-378`; `pairing/qr.go:23-25`; `cli/attach_windows.go:19` | `✓ ⚠ ✗` sin degradación; `colorOK` no mira que stderr sea TTY (escapes en un fichero con `setup 2>log`); con `NO_COLOR` el QR sale invertido y los lectores fallan; VT solo se activa en `attach` (conhost antiguo por comprobar). `colorOK` con `isTerminal(app.Stderr)`, QR siempre con fondo blanco, VT en Windows antes de escribirlo, fallback ASCII con `TERM=dumb`. Solo presentación del QR | S-M | no |
| H14 | pulido | `platformtools/platformtools.go:253,281,297`; `cli/setup.go:158` | Texto de descarga de platform-tools fijo en inglés y sin progreso. Callbacks `OnStart`/`OnDone` que el CLI traduce; cambia la interfaz de `platformtools` (fuera de mi árbol: lo decide su dueño) | M | no |
| H15 | pulido | `cli/serve.go:247-248`; `en.toml`/`es.toml` (`setup.no_phone_hint`, `err.connect.wrong_phone.fix`, `help.serve.flag.confirm`) | Prefijos `⚠ `/`⏳ ` fijos, primera persona ("No veo ningún móvil…"), `.fix` sin mayúscula ni punto, `(default "auto")` sin traducir. Unificar | S | no |
| H16 | pulido | `adbuddy attach --json` → `USAGE` | `--json` global listado en comandos que no lo soportan (`attach`, `new`, `stop`, `env`). Ocultarlo o documentarlo por comando | S | no |

## Propuesta de agrupación para la fase 2

Un subagente de escritura a la vez; cada paquete con su test de UI y su captura en la JVM (claro, oscuro, 200 % y 200 % es) cuando cambie algo visible:

1. **P1 — Barra superior y accesibilidad base** (O3, O13, O14, O6/C14, C6 sin tarjeta 🔴, C23, C15): `AdbuddyTopBar`, `SecondaryActionButton`, `toggleable`, semántica. Sin seguridad.
2. **P2 — Onboarding y estados de enlace** (O5, O10, O2, O19, O20, C4, C20, C13, C21): reglas de pantalla y textos en y es. Sin seguridad.
3. **P3 — Chat: parar, adjuntos, terminal** (C5, C17, C18, C12, C11, C19 parcial, C26): sin seguridad.
4. **P4 — Notificaciones** (C1, C3/O4, C27 parcial): canal propio para la pregunta del agente y permisos con motivo. Plan con `adbuddy-reviewer` (la 🔴 no se toca) y revisión opus por C3/O4 y `CATEGORY_CALL`.
5. **P5 — Confirmaciones y consentimiento (opus)** (O1, C2, C7, C8, C9, C10, O9, O12, O21): plan y cada tarea con `adbuddy-reviewer-opus`.
6. **P6 — CLI** (H1, H3, H4, H8, H6, H7, H9, H11, H12, H15, H16, luego H2, H5, H13): árbol `host/internal/cli` e `i18n`, goldens de ayuda en y es. H10 con revisión de seguridad. H14 requiere avisar al dueño de `platformtools`.

Fuera de la lista y por decidir: O7 y O11 (accesibilidad), O8, O15 a O18 (paleta, idioma, icono). Entran en P1 o P2 si el líder aprueba.
