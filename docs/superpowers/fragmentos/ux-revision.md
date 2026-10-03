# Fragmento UX: revisión de la app (para pendientes y validaciones)

El controlador de integración copia lo que haga falta a `pendientes-m3-m5.md` y a `2026-10-03-validaciones-manuales.md`. Este fragmento no edita ninguno de los dos.

## P1 · barra y accesibilidad base

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
