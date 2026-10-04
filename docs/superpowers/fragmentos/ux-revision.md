# Fragmento UX: revisión de la app (para pendientes y validaciones)

El controlador de integración copia lo que haga falta a `pendientes-m3-m5.md` y a `2026-10-03-validaciones-manuales.md`. Este fragmento no edita ninguno de los dos.

## P1 · barra y accesibilidad base

P1 aplicado en pendientes (2026-10-04): `pendientes-m3-m5.md`, «Revisión UX (P1)», y el inventario de validaciones (M5c-27). No volver a copiarlo.

### Qué cambió

- Cuatro pantallas usan el nuevo `AdbuddyTopBar`. `assertTitleFitsTheTopBar` ahora mide la barra pintada y `KNOWN_UI_ISSUES` queda vacío.
- `SecondaryActionButton` compartido para el "Ahora no" del paso 2 de 2 y de apps protegidas (ConsentScreen queda para P5, que toca ese fichero).
- La fila del modo remoto es accionable entera, y TalkBack la lee como un solo interruptor con nombre. El diálogo de confirmación sigue apareciendo antes de abrir o cerrar nada.
- Pistas de TalkBack en la descarga de ficheros ("Toca para cancelar") y en la tarjeta de pregunta del agente.
- Pistas de TalkBack en la descarga de ficheros ("Toca para cancelar", etiqueta de la acción según el estado) y en la tarjeta de pregunta del agente (anuncio del resumen, título como cabecera); `heading()` también en `StepHeader` y en los títulos de las barras.

### Imágenes regrabadas

- Task 1 (200 %): 17 PNG: `access_*_font_200` x7, `protected_apps_*` x6, `settings_*` x4.
- Task 2 (paso 2 de 2 del onboarding): 6 PNG `onboarding_2_notification_access_*`.
- Tasks 3 y 4: ninguna.

### Sigue necesitando el Pixel con TalkBack

Ninguna se pide al usuario ahora; el resto está cubierto por tests de semántica e imágenes en la JVM.

- Lectura real del interruptor del modo remoto como un solo interruptor con nombre ("Modo remoto, interruptor, desactivado").
- Anuncio de la tarjeta del agente al aparecer.
- Descarga que dice "Toca para cancelar".

### Notas de pendientes que se pueden quitar

- El título de Acceso a las notificaciones al 200 % en es ya cabe. Quitar la nota "Hallazgo de UI" de `pendientes-m3-m5.md` y el "Hallazgo" de la validación 27 de `docs/superpowers/plans/2026-10-02-adbuddy-m5c-notificaciones.md:5187`. Matiz: el TopAppBar de Material ya crecía solo; lo que cambia es que `assertTitleFitsTheTopBar` ahora mide la barra pintada y `KNOWN_UI_ISSUES` queda vacío.

### Aplazado

- C23 (borrar de 18 dp en los chips de borrador): cambiar el tamaño del chip pide diseño propio; va a P3 con el resto de adjuntos.

## P2 · onboarding y estados del chat

### Qué cambió

- Task 1, pie del onboarding: los botones del pie se apilan con letra grande en vez de comprimirse. El paso 2 ya no pinta un "Listo" falso con el acceso apagado: el recordatorio ("cómo retomarlo", `access_later_hint`) va en el cuerpo y "Listo" solo aparece con el acceso activo.
- Task 2, tarjeta de ajustes restringidos: la pantalla de acceso a las notificaciones explica el aviso de "ajustes restringidos" de Android (`access_restricted_hint`) cuando el acceso está apagado o sin determinar.
- Task 3, consentimiento: al responder (aceptar o rechazar) un aviso dice qué se hizo y qué queda (`consent_done_steps`, `consent_done`, `consent_declined`); tras rechazar la app sigue con el paso 1 de 2.
- Task 4, cabecera del chat: el subtítulo es de una línea (versión corta del estado) y el detalle completo va en un banner bajo la barra, anunciado una vez a TalkBack (live region cortés). Estado nuevo `NOT_SET_UP`: sin enlace del host ni canal, el banner dice que ejecutes `adbuddy setup`.
- Task 5, chat vacío: conectado y sin mensajes, sin tira de herramientas y sin mensaje saliente pendiente, el chat dice dónde escribir (`chat_empty_ready`).
- Task 6, scroll que no arrastra: el chat sigue lo que llega solo si el lector está al final; si no, muestra "Mensajes nuevos" y se queda donde está.

### Imágenes regrabadas y creadas

- Task 1: 16 regrabadas; Task 1-fix: 6 más.
- Task 2: las de Acceso OFF/UNKNOWN y las 6 del paso 2 (`onboarding_2_notification_access_*`), más la nueva `access_not_allowed_below_font_200`.
- Task 4: 20 `chat_*` nuevas.
- Task 5: 4 `chat_connected_empty_*` nuevas.
- Task 7 (limpieza de menores): ninguna.

### Aplazado de P2 con motivo

C13 (tarjeta compacta del modo remoto apagado: pide diseño propio); indicador de "pensando" y distinguir "reconectando" de "esperando al PC" de C20 (piden estados nuevos en el chat); aviso de depuración inalámbrica apagada y enlace a `troubleshooting.md` de O2 (piden leer `Settings.Global.adb_wifi_enabled` y decidir qué mostrar); pista de "cómo retomarlo" en el paso 1 de O10 (el paso 2 la lleva en `access_later_hint`); pie de `ConsentScreen` apilado con letra grande y `SecondaryActionButton` allí (P5, que toca `Screens.kt`).

### Notas honestas

- Criterio del scroll: el chat estaba "al final" si `último índice visible >= filas vistas - 1`. Coste: quien lee solo el inicio de una última burbuja muy alta es llevado al final cuando llega un mensaje; el crecimiento en sitio de un mensaje (p. ej. streaming) no genera aviso con el lector arriba; al volver de Ajustes el chat salta al final, como ya hacía.
- Los tests de "cambios seguidos" (dos llegadas mientras corre la animación) no tienen RED demostrable: son de humo.

### Validaciones manuales pendientes (Pixel)

Ninguna se pide al usuario ahora: el banner del chat con TalkBack (se anuncia una vez al aparecer), los avisos `consent_done_steps` y `consent_declined` en el flujo real de `adbuddy setup`, y que `BridgePrefs.link` se rellena al terminar `setup` y el estado "sin configurar" desaparece con la app abierta. Lo demás está cubierto por tests de semántica y por las imágenes en la JVM.

## P3 · chat, adjuntos, terminal y diario

### Qué cambió

- Task 1, "Parar" (C5): con el agente trabajando y la barra vacía (sin texto y sin ningún adjunto, aunque se esté subiendo o haya fallado), el botón de enviar pasa a ser "Parar" y envía `agent.interrupt`. Con algo escrito o cualquier adjunto sigue siendo "Enviar" (deshabilitado mientras `canSend` sea falso), para que quien redacta nunca vea "Parar" por error. Tras pulsarlo queda deshabilitado 3 s y se rehabilita antes si `working` cambia.
- Task 2, chips de adjuntos (C17, C23): un adjunto que se está preparando muestra una barra de progreso (indeterminada si aún no se sabe el tamaño); el botón de quitar tiene tamaño táctil de 48 dp y el reintento de un adjunto fallido está solo en la etiqueta, nunca en la X.
- Task 3, terminal sin conexión (C18): el terminal avisa de que no hay conexión con el PC en vez de quedarse mudo, y las teclas rápidas se desactivan.
- Task 4, avisos largos (C11): `notify` elige la duración por el texto: los largos duran más y los que llevan acción no desaparecen solos (botón de cerrar).
- Task 5, diario (C26): los filtros dicen el nivel con palabras (`level_chip`: emoji y nombre; las filas siguen mostrando solo el emoji, ver la nota de accesibilidad abajo), cada entrada lleva su hora (solo la hora si es de hoy, fecha y hora si no) y hay dos vacíos distintos (sin entradas / ningún aviso de ese nivel).
- Task 6, burbuja con el terminal plegado (C12): la burbuja arranca con el terminal plegado (es de ~600 dp de alto); el chat a pantalla completa lo sigue desplegando por defecto (blindado por un test del flujo).
- Task 7, limpieza de menores: orden de imports, zona táctil del reintento sin cambiar el aspecto, tests de reposo/reinicio de "Parar", aviso con acción que sigue tras 60 s, aviso de deshacer solo con estado `ok` e id válido, frontera de día por zona.

### Imágenes nuevas

`chat_working_*`, `draft_chips_*`, `activity_entries_*` y `activity_empty_*` (claro, oscuro, 200 % y 200 % en español). Ninguna imagen existente cambia.

### Aplazado con motivo

C19 (accesibilidad, tamaño del terminal y teclas ←, →, Shift+Tab) pide ajustes en el JS del terminal y en las teclas rápidas; C16 (la captura que se hace de ADBuddy mismo) pide decidir el comportamiento; C12 en lo que falta (persistir la preferencia del terminal y uno solo a la vez con poca altura) pide diseño; la acción "Restaurar" tras `cut_done` de C11 toca el corta-accesos y va con P5.

### Notas honestas

- "Parar" se deshabilita 3 s tras pulsarse: `agent.interrupt` no tiene respuesta, y el reposo evita enviar un segundo Esc seguido.
- El aviso de deshacer del diario solo sale con estado `ok`, inverso presente y no deshecho, e id con la forma `j_<alfanumérico>`: `adbuddy undo <id>` (sin id, deshace lo último, no esa entrada).
- Cola de avisos: cada aviso va en su corrutina y una ráfaga de avisos largos se encola (aceptado).
- Las imágenes en español del diario muestran la hora en formato inglés porque la JVM de test usa `Locale.getDefault()` inglés; no es del producto.
- El aviso del terminal sin conexión y el cableado de `connected` en `ChatScreen` no se pintan en la JVM (el WebView no se pinta): solo se prueban `QuickKeysRow` y `TerminalOfflineNotice` por separado, y el cableado queda para la validación manual de abajo.

- Accesibilidad del diario (aplazado): las filas muestran solo el emoji del nivel (TalkBack leerá "círculo verde"); una `contentDescription` con `levelName` lo arreglaría. Tampoco se refresca "hoy" si la pantalla sigue abierta pasada la medianoche, ni "Parar" avisa si `bridge.send` falla justo en la transición de la conexión.

### Validaciones manuales pendientes (Pixel)

Ninguna se pide al usuario ahora: "Parar" contra un agente real trabajando (el host recibe `agent.interrupt` y la sesión lo refleja), el aviso de "sin conexión" sobre el terminal real (el WebView no se pinta en la JVM) y las teclas desactivadas, y las horas del diario con zonas horarias reales. Lo demás está cubierto por tests de semántica y por las imágenes en la JVM.

## P4 · notificaciones

### Qué cambió

- Task 1, aviso de la pregunta del agente: cuando el agente pide permiso (`ask`) y la app no está delante, sale un aviso propio, genérico y sin botones, con la descripción de los canales ajustada. `foreground` no parpadea al recrear la actividad por un cambio de configuración (`isChangingConfigurations`), así que girar el móvil no dispara ni cancela avisos.
- Task 2, banner (`notifications_banner_*`): el chat muestra un banner cuando las notificaciones están bloqueadas (desactivadas en la app o canal de confirmaciones/preguntas silenciado), con atajo a los ajustes de notificaciones de la app. Si el sistema no tiene esa pantalla, abre los ajustes generales; si tampoco hay actividad, no hace nada y no cierra la app.
- Task 3, permiso (`notifications_rationale_*`): el permiso de notificaciones se pide una sola vez, con un diálogo que explica el motivo y fuera de la pantalla de configuración. El motivo cede si llega una pregunta del agente con él abierto.
- Task 4, limpieza de menores: import de `VisibleForTesting` en `Bridge.kt` y segundo `try/catch (ActivityNotFoundException)` en el respaldo de `openNotificationSettings` (la Task 2 lo dejó con un solo `try`; el segundo llegó en la Task 4).

### Imágenes nuevas

`notifications_banner_*` y `notifications_rationale_*`. Ninguna imagen existente cambia.

### Qué cierra y qué deja desfasado

Este fragmento no edita esos ficheros; el controlador de integración debe actualizarlos.

- Cierra el pendiente de `docs/superpowers/plans/pendientes-m3-m5.md` (~línea 325: canal alto para el aviso).
- Deja desfasada la spec de M4, `docs/superpowers/specs/2026-10-01-adbuddy-m4-claude-code-design.md` (~líneas 125-127: «el aviso es silencioso en M4…»; y la línea ~126, «con el enlace en pausa o sin conexión no hay aviso de permiso»: P4 decide lo contrario para la pausa, el aviso SÍ sale), y la validación manual «Aviso de permiso silencioso» de `pendientes-m3-m5.md` (~línea 320), que queda desfasada: ahora el aviso suena y sale en pantalla.

### Aplazado con motivo

- `CATEGORY_CALL`, la visibilidad de la 🔴 en pantalla bloqueada y el botón "Rechazar" en el bloqueo: van con P5 (opus), porque tocan la seguridad de las confirmaciones.
- El armado del diálogo 🔴 no se hace aquí: es de `seguridad`, en su rama `fix/confirm-arming`. Hueco preexistente anotado por el revisor: en `ConfirmDialog` "Permitir" no tiene armado; lo cubre `seguridad`.
- La fila de "notificaciones" en Ajustes que pedía O4: el banner del chat ya cubre el aviso permanente y la fila pediría un estado más en Ajustes.
- Los iconos del sistema y de lanzador, el `LinkDialog` que no sobrevive a la rotación y el botón del terminal con ↑/↓ (C27, pulido sin motivo de seguridad): quedan SIN destino en P5 (P5 es de confirmaciones y consentimiento); el controlador de integración debería apuntarlos en `pendientes-m3-m5.md` para que no se pierdan.

### Notas honestas

- El aviso de la pregunta del agente es genérico y sin botones.
- Con la sesión en pausa o el acceso cortado el aviso SÍ sale, porque la pregunta caduca igual.
- Una segunda pregunta mientras sigue la primera avisa otra vez, y una reconexión con la pregunta aún pendiente también.
- El aviso no se repinta al cambiar de idioma.
- Con las notificaciones desactivadas, `ask` no publica, pero el id queda como avisado hasta que la app pase a primer plano y vuelva.
- `foreground` no baja en un cambio de configuración (`isChangingConfigurations`).
- El permiso se pide una sola vez; el banner es el recordatorio permanente.
- No hay diálogo del motivo en la burbuja ni con una pregunta pendiente (el motivo cede si la pregunta llega con él abierto).
- `Bridge.setStateForTest` es `internal` y `@VisibleForTesting`.
- `foreground` podría quedarse en `true` si una actividad se para con `isChangingConfigurations` siendo la última y la nueva instancia nunca arranca (multiventana/PiP sin comprobar): validar en el emulador; arreglo barato si hiciera falta: recalcular en `onActivityDestroyed`.
- Con la app delante en Ajustes, Actividad o un paso de bienvenida, la pregunta del agente no tiene tarjeta (solo se pinta en el chat) ni aviso (`foreground` lo retira): ya pasaba antes; solución futura: contar como "delante" solo la ruta del chat, o un banner en las otras rutas.
- El aviso vuelve a sonar cada vez que se pliega la burbuja con la pregunta pendiente (coherente con la regla "vuelve a avisar al pasar a segundo plano"); si molesta, recordar los ids que ya sonaron y publicar sin alerta.
- En Android 13+ el banner de "notificaciones bloqueadas" sale a la vez que el diálogo del motivo, antes de pedir el permiso (cierto pero redundante).
- `BridgeService` no tiene test de servicio: las llamadas `cancelAsk` de `onCreate`/`onDestroy` y el colector solo se comprobaron leyendo.

### Validaciones manuales pendientes (Pixel)

Ninguna se pide al usuario ahora: el aviso de la pregunta del agente con la app en segundo plano y la pantalla apagada (suena y sale en pantalla, sin mostrar nada de la pregunta), el diálogo del sistema tras el motivo (Android 13+), y el banner al desactivar las notificaciones en los ajustes y volver. Lo demás está cubierto por tests de semántica, de la notificación construida y por las imágenes en la JVM.

## P6a · CLI: textos, ayuda y errores de uso

### Qué cambió

- Task 1, textos rotos y plurales (H3, H8): los textos de flags ya no llevan acentos graves que rompían la ayuda, los plurales ingleses y españoles salen bien (`1 phone`, `1 skill`, `1 línea dañada`) y los `.fix` de los errores tienen un estilo comprobado por test (`fix_style_test.go`).
- Task 2, errores de uso (H1, H4, H16): el error de uso apunta a la ayuda del comando que falló (`Run adbuddy tap --help to see the usage.`), `adbuddy help <tema que no existe>` es un error de uso (exit 2) y `--json` se describe como salida para programas.
- Task 3, ayuda raíz (H6 en parte, H7 en parte): comandos agrupados en tres bloques, con un ejemplo y descripciones cortas que caben en 80 columnas (`TestHelpShortsFit`, máximo 60 caracteres por `Short`).
- Task 4, `journal` y esperas (H9, H11, H12): `journal` avisa cuando recorta («Showing the last 50 of 60 entries») y pone la fecha cuando la entrada no es de hoy; los mensajes de espera de `serve` hablan del móvil y no del servidor adb; `doctor` es más accionable.
- Task 5, `serve` dice dónde saldrán las confirmaciones 🔴 (H10): una línea al arrancar, justo antes de «ADBuddy is serving», según `--confirm` y si la entrada es una terminal. Solo texto: no cambia qué confirmador se usa.
- Task 6, limpieza de menores de la revisión: `help.serve.flag.confirm` precisa que `auto` usa la terminal solo si la entrada es interactiva y que `terminal` lee la entrada estándar aunque sea una tubería; mejores `Short` de `doctor`, `setup` y `serve` en español (y «live»/«en vivo» en `serve`); el aviso de desconexión de la app dice «hasta que se vuelva a conectar»; tests más precisos (plurales españoles afirmados a mano, marcador de tres palabras, envelope `--json` completo, recorte seguro, comprobación de la llamada de espera por AST).

### Goldens de ayuda regrabados

- `testdata/help/en/`: `journal`, `run`, `serve`, `tap`, `undo` (Task 1); todos los `*.txt` (Task 2, la línea del pie de ayuda con `--json`); `root` y `env` (Task 3); `root` y `serve` (Task 6).
- `testdata/help/es/`: `root` (Tasks 3 y 6). `doctor` no cambia.

### `--json`

Solo gana campos: `journal` añade `total` y `truncated`, y el `fix` de un error de uso ahora nombra el comando (también cambia el `fix` del error de `setup` sin terminal, que es otro error de uso con arreglo propio). Ningún campo existente cambia ni desaparece; la salida de máquinas sigue en inglés.

### Aplazado con motivo

Fuera de este plan (P6b, después): H2 (errores de cobra traducidos y sugerencias propias), H5 (`doctor` que imprime según avanza), H13 (colores y QR con `NO_COLOR`/redirección/Windows), H14 (`platformtools`: fuera de nuestro árbol), `Example` de `attach`/`new`/`send`/`stop`/`env`/`version` (H6), `Long` a ≤ 80 columnas con su test de ancho (H7) y los prefijos fijos `⚠ `/`⏳ ` de `serve.go:247-248` (H15).

También aplazado: H4 sin «sugerencia de comandos parecidos» (va con H2, P6b) y H15: el `(default "auto")` de `--confirm` sigue sin traducir (lo añade pflag con texto fijo, fuera del catálogo).

### Notas honestas

- El orden de los comandos dentro de cada grupo de la ayuda raíz es alfabético (doctor, serve, setup), no el del flujo, y el ejemplo de la raíz lleva la descripción en la misma línea (no copiable tal cual): mejorables en P6b.
- H10 es solo texto y sigue lo que el código hace de verdad: `startPhoneLink` envuelve siempre el confirmador en el de la app, así que con la app conectada la 🔴 se pregunta en la app sea cual sea `--confirm`.
- La fecha del `journal` es ISO, no localizada.
- El `Short` de ciertos comandos perdió detalle para caber en 60 caracteres; ese detalle sube al `Long`.
- `adbuddy help <Tab>` conserva su autocompletado.
- El test que lee `serve.go` para comprobar que se pasan los segundos de `deviceWait` mira el árbol sintáctico, no el texto; no depende del formato del fichero.

### PARA EL CONTROLADOR DE INTEGRACIÓN Y `seguridad` (no se tocan aquí)

- `docs/usage.md:~107` dice «`--confirm deny` refuses every red command without asking» y `docs/safety.md:~84` va en la misma línea. Es falso con el móvil conectado: `--confirm deny` SÍ pregunta en la app (solo deniega cuando la app no está conectada). Las dos frases hay que corregirlas.
- Los cinco avisos `phone.*` que decían «confirmations happen here» ya están corregidos (`phone.confirmations_happen_here_without_the`, `phone.could_not_check_whether_the`, `phone.the_adbuddy_app_is_not`, `phone.the_phone_app_disconnected_confirmations`, `phone.could_not_reach_the_phone`): ahora dicen que las confirmaciones no llegarán al móvil. Solo el aviso de desconexión de la app (`phone.the_phone_app_disconnected_confirmations`) promete reconexión («hasta que se vuelva a conectar»), y `phone.could_not_reach_the_phone` dice «hasta que se conecte», cierto donde se emite; los otros tres (sin app, no se pudo comprobar, canal no disponible) no prometen nada, porque la app no se reconecta hasta reiniciar `serve`.

### Validaciones manuales pendientes

Ninguna: todo está cubierto por tests de ayuda dorada y de ejecución del CLI.
