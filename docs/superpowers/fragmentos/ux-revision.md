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
- Task 3, consentimiento: al responder (aceptar o rechazar) un aviso dice qué se hizo y qué queda (`consent_done_steps`, `consent_done`, `consent_declined`); tras rechazar la app vuelve al paso 1 de 2.
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
