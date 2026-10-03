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
