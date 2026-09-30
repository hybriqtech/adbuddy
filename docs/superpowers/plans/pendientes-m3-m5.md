# Pendientes para planes futuros (M2.5, M3, M5, M6 y v1.1+)

Notas que hay que incorporar cuando se escriban esos planes (desde el 2026-09-29).

## M2.5 · Clasificador con lista blanca y mksh real

Motivo (M2, Tarea 3): seis rondas de arreglos cerrando, uno a uno, engaños de mksh que ejecutan texto sin analizar (`PS4`/`ENV`/`FPATH`, `trap`/`alias`/`builtin`, `find -exec`, trabajos en segundo plano, aritmética que reevalúa variables, operadores dinámicos de `test`, `typeset -L/-R/-Z` y su herencia a shells hijas). Además, varias conclusiones se tomaron "de memoria del código de mksh", sin comprobarlas.

- **Lista blanca para 🟢/🟡:** una orden solo baja de 🔴 si el parser la entiende entera: órdenes conocidas con palabras literales, o variables probadas (literales, números, bucles `for` sobre literales). Cualquier construcción no reconocida (builtins de shell no listados, expansiones no probadas en contextos que evalúan, asignaciones a variables especiales) es 🔴 especial. Empezar por inventariar lo que hoy es 🟢 en el corpus y asegurar que sigue igual.
- **Tests diferenciales contra mksh real:** compilar el mksh de AOSP para el PC (o usar un emulador) y ejecutar un corpus de engaños con una orden centinela (`touch /tmp/pwned`) en lugar de `reboot`: si la centinela aparece, el clasificador debe haber dado 🔴 especial. Mismo corpus para las órdenes habituales (no deben subir).
- **Emulador Android** (imagen de sistema sin permisos de administrador si WHPX/Hyper-V ya está activo) para `connectedDebugAndroidTest` (`TokenProviderTest` con el uid del shell) y para automatizar validaciones manuales de M2: `content write` por stdin, servicio en primer plano desde el proveedor, Cortar/Reactivar.
- Menores aparcados de M2 que caen aquí: `a=([X]=1)` con índice en el literal (M-3); `$(( $X = 1 ))` con nombre desconocido.

## M3 · Chat: daemon con PTY

- **Lista de variables de entorno permitidas** para lanzar el agente (§5.1). En M1a, `host/internal/agent.Env` solo quita variables (`CLAUDECODE*`, `CLAUDE_CODE_*`, `CLAUDE_EFFORT`, `CLAUDE_PID`, `ANDROID_SERIAL`, `ANDROID_ADB_SERVER_PORT`, `ADBUDDY_SOURCE`). M3 debe pasar a una lista de permitidas: las de sistema, autenticación y proxy del usuario (`PATH`, `HOME`/`USERPROFILE`, `APPDATA`, `ANTHROPIC_*`, `HTTP(S)_PROXY`/`NO_PROXY`, `CLAUDE_CONFIG_DIR`…), verificando la lista en Windows, macOS y Linux.
- **Confianza previa de la carpeta del entorno** en Claude Code (§5.1, S3): averiguar dónde y con qué clave guarda Claude Code la confianza de una carpeta y preconfiar el entorno antes de lanzar el agente en el PTY; si no es posible, detectar el diálogo y elegir "Yes" de forma explícita (un Enter a ciegas elige "No, exit").

- **La app muestra los `notice` del host** (revisión final de M2): hoy los ignora; el de respuesta tardía ("That question was already answered or moved to the PC terminal.") y el de tipo no soportado deberían verse en la pantalla de inicio.
- **Fixtures JSON compartidas del protocolo** en `testdata/` que lean los tests de Go y de Kotlin: hoy el contrato lo sostienen cadenas escritas a mano (`ProtocolTest.kt`) y la app falsa del e2e. Hacerlo antes de que el protocolo crezca con el chat.
- **`adbuddy doctor` desde la sesión del agente** usa el adb real fuera del shim (solo lecturas fijas, sin argumentos del agente) aunque haya pausa o una pregunta pendiente, y no lo apunta en el diario. Decidir si se registra o se bloquea en pausa.

## M5 · Ayudas al agente

- **`adbuddy screen` con el servicio de accesibilidad vinculado y la app desconectada:** devolver un error claro en lugar de usar `uiautomator dump`, que desconectaría el servicio (§6). Mensaje propuesto: `ADBuddy's accessibility service is running but the app is not connected: open the ADBuddy app and try again`.
- **Revisar ARTEMIS de Google antes de escribir las ayudas** (`github.com/google/artemis`, Apache-2.0; revisado en el commit 351ca84 del 2026-09-28). Su app auxiliar (`packages/artemis-accessibility-helper`) resuelve lo mismo que `AgentAccessibilityService` y se puede reutilizar con atribución. Ideas concretas:
  - `HierarchyDumper`: sin `waitForIdle` (no se cuelga), todas las ventanas (app, diálogos, System UI, teclado, pantalla partida), nodos invisibles fuera y límites recortados a la pantalla, a la ventana y a los ancestros con scroll (paridad con UIAutomator), campos semánticos (`error`, `is_heading`, `editable`, `pane_title`, `hint`), captura de pantalla y árbol en el mismo instante (API 30+), IPC por lotes en API 33+ y saneado estricto de XML.
  - `GestureController`: toques, pulsación larga, deslizar y escribir con `dispatchGesture`.
  - Localizar "primero dinámico, coordenadas de respaldo": id, texto o descripción antes que posición.
  - Detectar quién tiene UiAutomation (uiautomator2, Appium) antes de leer la pantalla, porque desactiva los demás servicios de accesibilidad (lo vimos en S2 con `uiautomator dump`).
  - Versión mínima del protocolo entre host y app (`MIN_PROTOCOL_VERSION`), para detectar una app desfasada.
  - Receptor del token protegido con `android:permission="android.permission.WRITE_SECURE_SETTINGS"`: solo el usuario shell o el sistema pueden enviarlo. Útil como alternativa al proveedor, pero **sin** pasar el token en la línea de comandos como hace ARTEMIS (`am broadcast --es token …`): lo mantenemos por stdin (plan de M2).

- **ADBuddy es app protegida frente a sus propias ayudas** (revisión de M2): las acciones de accesibilidad (tocar, leer la pantalla) nunca actúan sobre `tech.hybriq.adbuddy`, para que el agente no pueda pulsar Permitir en su diálogo.
- **Si aparece otra vía de inyección de toques fuera del shim**, pedir autenticación (BiometricPrompt o credencial) al pulsar Permitir en la app. En M2 bastan el bloqueo de órdenes mientras hay una pregunta 🔴 pendiente y que los trabajos en segundo plano sean 🔴 especiales.
- **`TokenProvider` y `BridgeService` no sobrescriben `dump()`**: `dumpsys activity provider|service` es 🟢 para el agente.

## M6 · Publicación

- **README, ventaja de coste frente a ARTEMIS:** ADBuddy usa el agente de código que el usuario ya tiene y funciona con su suscripción (por ejemplo, un plan de Claude Code), sin claves de API. ARTEMIS necesita su propio LLM y sus claves de API, porque el agente le delega tareas enteras. Decirlo en una línea al comparar, y en la portada entre las ventajas.

- **`ADBUDDY_RELEASE_URL`** cambia a la vez el origen del APK y el de su checksum: es un gancho de test que vive en producción. Documentarlo o limitarlo junto con el certificado de firma.
- **Fijar en el host el SHA-256 del certificado de firma del APK** (revisión de la Tarea 4 de M2): el `.sha256` de la release solo protege la integridad, no una release comprometida en la primera instalación.

## Más adelante (v1.1+)

- **Proxy del protocolo del servidor adb.** Herramientas como ARTEMIS (`adbutils`, `uiautomator2`) hablan con el servidor adb por su protocolo, sin el binario `adb`, así que se saltan el shim. Un proxy que atienda ese protocolo y clasifique las peticiones `host:transport` / `shell:` las pondría bajo el clasificador, el diario y las confirmaciones. Es un trabajo grande: evaluarlo cuando haya demanda.

- **Protocolo v2: el host se autentica ante la app** (revisión de la Tarea 9 de M2): hoy una app que ocupe el puerto mientras no hay túnel puede mostrar preguntas falsas (sus respuestas no llegan al host). Un HMAC del token sobre un nonce de la app dentro del `welcome` lo cerraría.
