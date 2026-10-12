# Ledger: pantalla oculta y overlay en la captura (P1), 2026-10-12

Rama `fix/p1-pantalla-oculta` (desde main aa7f3698). Plan: `docs/superpowers/plans/2026-10-12-p1-pantalla-oculta.md`. Spec: `docs/superpowers/specs/2026-10-12-p1-pantalla-oculta-design.md`. Compañero `seguridad`.

## Entorno (sin rutas personales)

- Emulador: AVD `adbuddy-p1-36` (API 36, `system-images;android-36;google_apis;x86_64`), consola 5600, servidor adb 5070; `ADBUDDY_EMU_STABLE_SECS=40`. Lock: `emu-lock.sh acquire|release seguridad`. Android 17 (android-37.0) no arranca (surfaceflinger aborta con swiftshader y con `-gpu host`).
- App: Gradle 9.8.0 de `$LOCALAPPDATA/Gradle`, `ANDROID_HOME=$LOCALAPPDATA/Android/Sdk`, `--offline --no-daemon`; build debug ~1 min 13 s. No hay `gradlew`.
- emutest: `ADB_MDNS=0 ADB_MDNS_AUTO_CONNECT=0 ANDROID_ADB_SERVER_PORT=5070 ADBUDDY_MKSH_SERIAL=emulator-5600 ADBUDDY_EMULATOR_SERIAL=emulator-5600` (+ las variables `*_ADB` con el adb del SDK).

## Rulings

- R1. Retirar marco y píldora durante la captura de `adbuddy screenshot` (op `hideNotice`, 3 fotogramas, restauración automática) — por qué: medido en API 36, `screencap` incluye marco y píldora (91,7 % de la región de la píldora distinta, 6/6 puntos de borde) y quitar las vistas deja la captura limpia 18/18; `takeScreenshot` de accesibilidad excluye la píldora pero incluye el marco (40423 px de diferencia), no está documentado y añade una capacidad al servicio — qué cuesta si me equivoco: el aviso está ausente ~0,3 s (tope 2 s) durante la propia captura; en Android 17 no se ha medido.
- R2. Si no se puede retirar el aviso la captura sale igualmente, con `noticeInImage` y una nota por stderr — por qué: la persona vio el aviso subir antes (`showNotice`); bloquear la captura capa al agente sin ganar seguridad — qué cuesta: una captura con el aviso dentro cuando el velo falla (el agente lo sabe).
- R3. Dos cambios aditivos de protocolo (op `hideNotice`, campo `UiDump.unreadable`), cada uno en un solo commit — por qué: es la única forma de que app y host se entiendan — qué cuesta: revisar la spec del protocolo común; los necesita el visto bueno del líder (M5 §12).
- R4. Las lecturas en bruto del agente (`screencap`, `screenrecord`, `uiautomator`) no retiran el aviso — por qué: no se capa al agente, y la rama de lectura en bruto sigue exigiendo el aviso — qué cuesta: el agente que quiere una captura limpia debe usar `adbuddy screenshot`.
- R5. No declarar `isAccessibilityTool` ni buscar rodeos para leer lo que el sistema oculta — por qué: debilita una protección anti-malware y no cumple la política de la API; `uiautomator` desconectaría el servicio — qué cuesta: en Android 17 esas pantallas se quedan sin lectura (se explica y el agente puede mirar con `screenshot`).
- R6. La restauración automática por temporizador no se hace en pausa, y `show()`/`remove()` la cancelan — por qué: una pausa debe quitar el overlay y no volver a ponerlo — qué cuesta: en pausa no hay aviso (como hoy).
- R7. Android 17 no corre en emulador: se valida en API 36 y lo de Android 17 queda en «Validaciones manuales pendientes» — por qué: no hay otra opción sin tocar el móvil — qué cuesta: el comportamiento exacto en Android 17 no está probado.
- R8. `hideNotice` no es un helper del agente (test `TestTheAgentCannotAskToHideTheNotice`) — por qué: el agente no debe poder quitar el aviso y actuar; `tap`/`type` además lo vuelven a mostrar — qué cuesta: nada.
- R9. Causa de P1.1 no confirmable sin el móvil — por qué: el diagnóstico solo-lectura del Pixel (ventana `SubSettings` activa, sin FLAG_SECURE, mismos flags que en el emulador, servicio vinculado con `capabilities=33`, el gestor solo le enseña dos ventanas: barra de estado y app) indica que el servicio no recibe contenido de ninguna ventana, pero no el mecanismo — qué cuesta: no se arregla, se explica con una nota genérica y cierta, y una línea de log sin contenido (T7) para la siguiente prueba.
- R10. El aviso lleva al menos 400 ms en pantalla antes de retirarse (`MIN_SHOWN_MS`) — por qué: `Overlay.show` solo espera a `addView`, no a que se dibuje, y `hideNotice` llega unos 50 ms después; sin mínimo el aviso previo a una captura sería un parpadeo que la persona no llega a ver y la premisa «la persona vio el aviso subir» (R2) sería falsa — qué cuesta si me equivoco: de 0 a 0,4 s más de latencia por captura. Si el líder prefiere latencia cero, `MIN_SHOWN_MS = 0` y la spec dice que el aviso de `screenshot` es sobre todo posterior.
- R11. Durante el velo no hay píldora (se quitan las vistas; no se deja una ventana transparente y tocable) — por qué: una ventana más que medir, y con el mínimo de 400 ms más los 4 s posteriores la píldora está a mano casi siempre — qué cuesta: un toque de la persona en esa zona durante ~0,5 s cae en la app de debajo y no pausa.
- R12. Tras un velo lento (> `baselineSlack`) se repite la guarda de primer plano antes de `screencap`, y la pausa que informa la app es `UI_BLOCKED` «Blocked by ADBuddy: …» — por qué: con los 400 ms la guarda puede quedar vieja y una app protegida sin FLAG_SECURE saldría en la imagen; es la regla que ya seguían los actos (`baselineSlack`) — qué cuesta: un `dumpsys` extra en capturas lentas.
- R13. Límites conocidos, solo documentados: una lectura o un `tap` concurrentes pueden cruzarse con el velo (la captura puede llevar el aviso aunque `noticeInImage` sea false; una lectura en bruto puede hacerse sin aviso hasta 2 s) y si la app desaparece entre `notice` y `hideNotice` la nota puede faltar; `restoreNotice` usa su propio contexto y no tiene test propio (la garantía es el temporizador de la app) — por qué: serializar todos los comandos del agente es un cambio mayor que no arregla un riesgo de seguridad (nada se lee sin que el aviso suba antes o inmediatamente después) — qué cuesta: imprecisión de la nota en carreras raras.

## Revisión del plan (`adbuddy-reviewer-opus`, 2026-10-12, ~9 min)

Veredicto: con reservas. Hallazgos y resolución:

1. BLOQUEANTE, `TestScreenshotAsksTheAppForTheNoticeFirst` se rompe (ahora hay dos `notice`): corregido en T3 (el test espera dos y el segundo tras `screencap`).
2. IMPORTANTE, la guarda queda vieja tras un velo lento: corregido (R12, test `TestScreenshotLooksAtTheGuardAgainAfterASlowVeil`, mutación apuntada).
3. IMPORTANTE, el aviso previo apenas se vería: corregido con R10 (`MIN_SHOWN_MS`, test de emulador del mínimo).
4. IMPORTANTE, la píldora desaparece bajo el dedo: ruling explícito R11, sin cambio de diseño.
5. IMPORTANTE, `plain` con notificación emergente: T4 reordenado (primero una lectura que espera la notificación).
6. IMPORTANTE, carrera de la prueba de pausa: T4 rehecho (`hide(5000)`, sondeo de `PROTECTED`, comprobación a los 6 s).
7. MENOR, `waitOverlay` duro tras el comando: el test nuevo mira las ventanas justo tras `g.read`; el viejo solo lo registra.
8. MENOR, `NoticeInImage` falso si la app desaparece entre las dos ops: R13 (documentado).
9. MENOR, código de pausa: `UI_BLOCKED` con `PausedMessage` (R12).
10. MENOR, concurrencia: R13 y spec §3.
11. MENOR, `removeCallbacks(restorer)` y mínimo de 50 ms con pantallas de 120 Hz: aplicado (`MIN_GONE_MS`, `takeOff`).
12. MENOR, dedup del log de T7: se reinicia cuando todo vuelve a tener contenido.
13. MENOR, texto de la nota (`adbuddy tap` en vez de «ADBuddy refuses to tap»): aplicado.
14. MENOR, T8: R1-R13, lista de operaciones de la spec del protocolo, grep de seriales, fila de FLAG_SECURE corregida: aplicado.
15. MENOR, test del contexto cancelado de `restoreNotice`: no se añade (`fakeApp` ignora el contexto, el test sería una tautología); R13.
16. Nota: `unreadable` solo llega al agente cuando la ventana de app activa no tiene contenido (el caso del Pixel) o no hay ventanas; un diálogo ilegible sobre una app legible lo para antes la guarda del host (`guardedOnScreen`). La nota es cierta en ambos casos.

## Tareas

| Tarea | Estado | Commit | Notas |
|---|---|---|---|
| T1 protocolo `hideNotice` | pendiente (espera OK del líder) | | |
| T2 app `hideNotice` | pendiente | | |
| T3 host `screenshot` con velo | pendiente | | |
| T4 emulador | pendiente | | |
| T5 protocolo `unreadable` | pendiente | | |
| T6 nota de `screen` | pendiente | | |
| T7 log de diagnóstico | pendiente | | |
| T8 docs | pendiente | | |

## Mutaciones

(se apuntan al ejecutar cada tarea)
