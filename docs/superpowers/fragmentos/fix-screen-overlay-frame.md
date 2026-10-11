# Fragmento para el inventario de validaciones manuales: aviso del overlay al leer la pantalla

Rama `fix/screen-overlay-frame`. Plan: `docs/superpowers/plans/2026-10-11-screen-overlay-frame.md`.

## Rulings

- Recomendar A+B (dump falla cerrado + aviso previo en lecturas por adb) — por qué: la persona debe saber cuándo el agente ve su pantalla y la condición de fallo (no poder dibujar el overlay) ya impide tocar — qué cuesta si me equivoco: dump de diagnóstico no disponible si el overlay falla; una op más en el protocolo común.
- Sin app, sin accesibilidad, app antigua o sin respuesta a tiempo con el servicio no vinculado: la lectura por adb sigue sin aviso — por qué: no hay a quién avisar y no se debe capar al agente — qué cuesta: lecturas sin aviso con el servicio apagado por la propia persona.
- `foreground` y `notifications` quedan fuera — por qué: `foreground` solo lee el nombre de la app; `notifications` tiene su propia puerta (confirmación del usuario) — qué cuesta: texto de notificaciones sin marco.
- La pausa se mira también en la app para `dump` y `notice` — por qué: `acting()` devuelve true sin dibujar en pausa, y la pausa puede llegar tras las puertas del host — qué cuesta si me equivoco: un `dump` en pausa devuelve `PROTECTED` en vez de `ok`.
- `showNotice` es lista blanca (nil, sin app, `unavailable`, `unsupported` con servicio no vinculado) — por qué: fallar abierto ante cualquier otro error tomaría un `PROTECTED` por pausa como permiso — qué cuesta: una lectura rechazada si la app contesta algo inesperado.
- El aviso se exige también al leer (líder, 2026-10-11) — por qué: M5 §6 y v1 §5.7; no capa al agente — qué cuesta: ida y vuelta por lectura y lecturas bloqueadas con la app atascada (diagnosticables por el prefijo `Blocked by ADBuddy:`).
- La op se llama `notice` aunque exista el tipo de mensaje `notice` — por qué: son espacios de nombres distintos (op de `ui.request`) y renombrar toca más ficheros — qué cuesta: confusión de lectura.
- `unsupported` deja leer siempre, sin consultar el servicio — por qué: una app antigua que responde no está atascada y el orden de merge host/app queda indiferente — qué cuesta si me equivoco: lectura sin aviso con una app antigua y accesibilidad activa.
- `screenrecord` solo avisa al empezar — por qué: el overlay se oculta a los 4 s (`HIDE_AFTER_MS`); es 🔴 y la persona lo confirma — qué cuesta: grabación larga sin marco visible.
- El cambio de protocolo (op `notice`) lo aprueba el líder, sin e5 — por qué: añade una op al protocolo común de `ui.request` sin tocar la seguridad — qué cuesta si me equivoco: revisar la spec del protocolo.

## Validaciones manuales pendientes

| Validación | Qué comprobar | Estado |
|---|---|---|
| Overlay visible durante `adbuddy screenshot` | En emulador, con la accesibilidad de ADBuddy activa: `adbuddy screenshot` y `dumpsys window` muestra la ventana `TYPE_ACCESSIBILITY_OVERLAY` de ADBuddy. Automatizable en `emutest`; la mutación de T2 (quitar la llamada al aviso) la pone en rojo. | HECHA (2026-10-11): `TestTheNoticeWhenTheHostReadsTheScreen`. |
| `dump` con la pantalla bloqueada | Con el keyguard puesto: el overlay se dibuja sobre él, o `dump` devuelve `FAILED` («won't read»). Nunca debe leer sin aviso. | HECHA (2026-10-11): `TestADumpOfTheLockedScreen`. Resultado: el overlay se dibuja sobre el keyguard y el dump se lee. |
