# Pendientes para planes futuros (M3, M5, M6 y v1.1+)

Notas que hay que incorporar cuando se escriban esos planes (desde el 2026-09-29).

## M3 · Chat: daemon con PTY

- **Lista de variables de entorno permitidas** para lanzar el agente (§5.1). En M1a, `host/internal/agent.Env` solo quita variables (`CLAUDECODE*`, `CLAUDE_CODE_*`, `CLAUDE_EFFORT`, `CLAUDE_PID`, `ANDROID_SERIAL`, `ANDROID_ADB_SERVER_PORT`, `ADBUDDY_SOURCE`). M3 debe pasar a una lista de permitidas: las de sistema, autenticación y proxy del usuario (`PATH`, `HOME`/`USERPROFILE`, `APPDATA`, `ANTHROPIC_*`, `HTTP(S)_PROXY`/`NO_PROXY`, `CLAUDE_CONFIG_DIR`…), verificando la lista en Windows, macOS y Linux.
- **Confianza previa de la carpeta del entorno** en Claude Code (§5.1, S3): averiguar dónde y con qué clave guarda Claude Code la confianza de una carpeta y preconfiar el entorno antes de lanzar el agente en el PTY; si no es posible, detectar el diálogo y elegir "Yes" de forma explícita (un Enter a ciegas elige "No, exit").

## M5 · Ayudas al agente

- **`adbuddy screen` con el servicio de accesibilidad vinculado y la app desconectada:** devolver un error claro en lugar de usar `uiautomator dump`, que desconectaría el servicio (§6). Mensaje propuesto: `ADBuddy's accessibility service is running but the app is not connected: open the ADBuddy app and try again`.
- **Revisar ARTEMIS de Google antes de escribir las ayudas** (`github.com/google/artemis`, Apache-2.0; revisado en el commit 351ca84 del 2026-09-28). Su app auxiliar (`packages/artemis-accessibility-helper`) resuelve lo mismo que `AgentAccessibilityService` y se puede reutilizar con atribución. Ideas concretas:
  - `HierarchyDumper`: sin `waitForIdle` (no se cuelga), todas las ventanas (app, diálogos, System UI, teclado, pantalla partida), nodos invisibles fuera y límites recortados a la pantalla, a la ventana y a los ancestros con scroll (paridad con UIAutomator), campos semánticos (`error`, `is_heading`, `editable`, `pane_title`, `hint`), captura de pantalla y árbol en el mismo instante (API 30+), IPC por lotes en API 33+ y saneado estricto de XML.
  - `GestureController`: toques, pulsación larga, deslizar y escribir con `dispatchGesture`.
  - Localizar "primero dinámico, coordenadas de respaldo": id, texto o descripción antes que posición.
  - Detectar quién tiene UiAutomation (uiautomator2, Appium) antes de leer la pantalla, porque desactiva los demás servicios de accesibilidad (lo vimos en S2 con `uiautomator dump`).
  - Versión mínima del protocolo entre host y app (`MIN_PROTOCOL_VERSION`), para detectar una app desfasada.
  - Receptor del token protegido con `android:permission="android.permission.WRITE_SECURE_SETTINGS"`: solo el usuario shell o el sistema pueden enviarlo. Útil como alternativa al proveedor, pero **sin** pasar el token en la línea de comandos como hace ARTEMIS (`am broadcast --es token …`): lo mantenemos por stdin (plan de M2).

## M6 · Publicación

- **README, ventaja de coste frente a ARTEMIS:** ADBuddy usa el agente de código que el usuario ya tiene; ARTEMIS necesita su propio LLM y sus claves de API, porque el agente le delega tareas enteras. Decirlo en una línea al comparar.

## Más adelante (v1.1+)

- **Proxy del protocolo del servidor adb.** Herramientas como ARTEMIS (`adbutils`, `uiautomator2`) hablan con el servidor adb por su protocolo, sin el binario `adb`, así que se saltan el shim. Un proxy que atienda ese protocolo y clasifique las peticiones `host:transport` / `shell:` las pondría bajo el clasificador, el diario y las confirmaciones. Es un trabajo grande: evaluarlo cuando haya demanda.
