# Pendientes para los planes de M3 y M5

Notas que hay que incorporar cuando se escriban esos planes (pedidas el 2026-09-29).

## M3 · Chat: daemon con PTY

- **Lista de variables de entorno permitidas** para lanzar el agente (§5.1). En M1a, `host/internal/agent.Env` solo quita variables (`CLAUDECODE*`, `CLAUDE_CODE_*`, `CLAUDE_EFFORT`, `CLAUDE_PID`, `ANDROID_SERIAL`, `ANDROID_ADB_SERVER_PORT`, `ADBUDDY_SOURCE`). M3 debe pasar a una lista de permitidas: las de sistema, autenticación y proxy del usuario (`PATH`, `HOME`/`USERPROFILE`, `APPDATA`, `ANTHROPIC_*`, `HTTP(S)_PROXY`/`NO_PROXY`, `CLAUDE_CONFIG_DIR`…), verificando la lista en Windows, macOS y Linux.
- **Confianza previa de la carpeta del entorno** en Claude Code (§5.1, S3): averiguar dónde y con qué clave guarda Claude Code la confianza de una carpeta y preconfiar el entorno antes de lanzar el agente en el PTY; si no es posible, detectar el diálogo y elegir "Yes" de forma explícita (un Enter a ciegas elige "No, exit").

## M5 · Ayudas al agente

- **`adbuddy screen` con el servicio de accesibilidad vinculado y la app desconectada:** devolver un error claro en lugar de usar `uiautomator dump`, que desconectaría el servicio (§6). Mensaje propuesto: `ADBuddy's accessibility service is running but the app is not connected: open the ADBuddy app and try again`.
