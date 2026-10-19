[Read in English](README.md)

# ADBuddy

Chatea desde el móvil con un agente de código de tu PC y deja que gestione tu Android por adb Wi-Fi, con cada comando clasificado por riesgo, cada cambio anotado en un diario y los comandos que clasifica en rojo (irreversibles, peligrosos o no entendidos) confirmados antes.

## Qué hace

- **El agente gestiona tu Android por adb Wi-Fi.** El agente de código que ya usas en el PC (Claude Code, Codex, Gemini CLI u OpenCode) ejecuta comandos adb en tu móvil: sin cable USB y sin que tengas que saber qué es adb.
- **Chateas con él desde el móvil.** Una pequeña app de ADBuddy en el móvil tiene el chat, muestra lo que hace el agente y te pregunta antes de los comandos que clasifica en rojo.
- **Con protecciones por defecto.** Cada comando se clasifica en verde, amarillo o rojo, los cambios se anotan en un diario, con su forma de deshacerlos cuando la tienen, y los comandos rojos esperan tu respuesta. La app tiene un botón Pausar y otro **Cortar acceso**.

## Cómo funciona

```text
  Móvil (Android 11+)                      PC (Windows, macOS o Linux)
  +--------------------+                   +-----------------------------------------+
  | App de ADBuddy     |  canal local      | agente de código (en el entorno)        |
  |  chat, preguntas,  | <---------------> |      | ejecuta su "adb" = el shim       |
  |  Pausar, Cortar    |  (lo lleva adb)   |      v                                  |
  |  acceso            |                   | host: adbuddy serve                     |
  | Depuración         | <---------------- |  clasifica, anota, te pregunta,         |
  |  inalámbrica       |    adb por Wi-Fi  |  y luego ejecuta el adb real            |
  +--------------------+                   |  (las platform-tools propias)           |
                                           +-----------------------------------------+
```

- **En el PC:** el host (`adbuddy serve`), el agente de código y la copia propia de adb de ADBuddy. El agente trabaja en una carpeta suya, el *entorno* (`~/ADBuddy/<phone name>/`, con el nombre del móvil).
- **En el móvil:** la app de ADBuddy y la depuración inalámbrica de Android.
- **El agente no ejecuta el adb real.** Cuando ADBuddy arranca el agente, el `adb` que este encuentra primero es un *shim*, un programa que hace de sustituto y pasa cada comando al host. El host lo clasifica, lo apunta en el diario, te pregunta si hace falta y solo entonces ejecuta el adb real.

## Inicio rápido

Todavía no hay versión pública, así que hoy compilas el host desde el código. [Instalar ADBuddy](docs/install.md) (en inglés) tiene los detalles y los demás sistemas.

1. **Instala el host.** Necesitas [Git](https://git-scm.com/) y [Go](https://go.dev/dl/) 1.27 o posterior. En Windows, compila `adbuddy.exe` y ponlo en una carpeta de tu `PATH`.

   ```sh
   git clone https://github.com/hybriqtech/adbuddy.git
   cd adbuddy/host
   go build -o adbuddy ./cmd/adbuddy
   mkdir -p ~/.local/bin
   cp adbuddy ~/.local/bin/
   ```

   Cuando haya una versión pública, un solo comando `curl … | sh` la instalará en Linux y macOS (en Windows será un archivo descargado): consulta [Instalar ADBuddy](docs/install.md) (en inglés).

2. **Prepara el móvil.** Activa las opciones de desarrollador y la depuración inalámbrica. [Prepara tu móvil](docs/setup/es/index.md) tiene los pasos (capturas próximamente; la guía en inglés está en [Set up your phone](https://hybriqtech.github.io/adbuddy/setup/)).

3. **Ejecuta setup.** Descarga las platform-tools de Android, empareja el móvil con un código QR, instala la app y prepara el entorno del agente. Hoy no descarga la app: compila el APK como dice [Instalar ADBuddy](docs/install.md#build-from-source) (en inglés) y pásalo.

   ```sh
   adbuddy setup --apk <file>
   ```

4. **Arranca el host** en una terminal y déjala abierta. Por defecto, él arranca el agente.

   ```sh
   adbuddy serve
   ```

5. **Habla con el agente.** Abre la app de ADBuddy en el móvil y escríbele. O, desde una segunda terminal del PC:

   ```sh
   adbuddy attach
   ```

   Si prefieres ejecutar el agente en una terminal tuya, arranca el host con `adbuddy serve --no-agent` y ejecuta `adbuddy run` en otra terminal; pierdes el chat en el móvil. Consulta [Usar ADBuddy](docs/usage.md) (en inglés).

## Agentes compatibles

Claude Code (recomendado), Codex, Gemini CLI y OpenCode. ADBuddy no instala agentes: arranca el que ya tienes y, si falta, `adbuddy run` muestra el comando que lo instala. Con Claude Code el móvil muestra las respuestas del agente como burbujas de chat; con los demás, muestra la terminal del agente.

## Seguridad

[Seguridad](docs/safety.md) (en inglés) lo explica entero. En resumen:

- Todo comando `adb` que ejecuta el agente pasa por el shim y por el host, que lo clasifica: verde (solo lectura), amarillo (reversible o inofensivo) o rojo (irreversible, peligroso o no entendido). Un comando que el host no entiende es rojo.
- Los comandos verdes y amarillos se ejecutan al instante y quedan en el diario; los amarillos con su forma de deshacerlos cuando la tienen (`adbuddy journal`, `adbuddy undo`). Los toques y la escritura son amarillos y no se pueden deshacer. Un comando amarillo pasa a rojo cuando ADBuddy no puede leer el estado que necesita o el cambio sobrescribiría algo.
- Los comandos rojos esperan tu respuesta, en el móvil (para permitir, debe estar desbloqueado) o en la terminal del host. Una pregunta sin respuesta cuenta como un no (a los 2 minutos por defecto).
- Con la app instalada, las apps de banca, pagos, contraseñas y 2FA están protegidas mientras están en primer plano (una propuesta a partir de sus nombres que confirmas o editas en la app). Sin la app, solo está protegido ADBuddy.
- ADBuddy no tiene telemetría ni servidor propio. Lo que el agente lee en tu móvil va al proveedor del propio agente.
- **El modo YOLO es la excepción.** Si lo activas en la app, el agente ejecuta incluso comandos destructivos sin preguntar, durante 15 minutos, 1 hora o hasta que acabe `adbuddy serve`. Solo lo activa el móvil, con huella o cara de clase fuerte o con el bloqueo de pantalla. Pausar lo apaga. Véase [Modo YOLO](docs/safety.md#yolo-mode) (en inglés).
- **El shim protege contra errores e inyecciones de prompts. No protege contra un agente empeñado en buscar otro adb.** La garantía real es **Cortar acceso** en la app, que apaga la depuración desde el móvil: con ella apagada, el PC no puede alcanzar el móvil.

ADBuddy reduce el riesgo de dejar que un agente maneje tu móvil; no lo elimina. Usa un móvil y unas cuentas que puedas permitirte reparar, y lee cada pregunta antes de permitir.

## Por qué ADBuddy

ADBuddy no tiene modelo propio y no necesita claves de API. A septiembre de 2026, ARTEMIS, la herramienta de código abierto de Google, necesita su propio LLM y claves de API; ADBuddy usa el agente y la suscripción que ya tienes.

## Requisitos

- **Móvil:** Android 11 o posterior, con depuración inalámbrica.
- **PC:** Windows, macOS o Linux, en la misma red Wi-Fi que el móvil (o Tailscale).
- **Un agente de código** instalado en el PC: Claude Code, Codex, Gemini CLI u OpenCode.
- **adb** 37 o posterior: `adbuddy setup` descarga por ti las platform-tools de Google, así que no lo instalas tú.

## Comandos

Son los comandos de `adbuddy --help` (el programa los describe en inglés; aquí van traducidos); `adbuddy <command> --help` muestra las opciones de cada uno.

| Comando | Qué hace |
|---|---|
| `adbuddy setup` | Configura ADBuddy: adb, emparejado y entorno del agente |
| `adbuddy serve` | Ejecuta ADBuddy en el PC: diario en vivo y confirmaciones |
| `adbuddy run` | Arranca un agente de código en el entorno de ADBuddy |
| `adbuddy attach` | Conecta esta terminal al agente (Ctrl+] desconecta) |
| `adbuddy new` | Empieza una conversación nueva, con otro agente si se indica |
| `adbuddy stop` | Para el agente de `adbuddy serve` (serve sigue en marcha) |
| `adbuddy journal` | Muestra el diario de comandos adb y de sus inversos |
| `adbuddy undo` | Deshace los cambios del diario ejecutando sus inversos |
| `adbuddy connect <ip:port>` | Conecta con el móvil por dirección IP y puerto |
| `adbuddy remote` | Abre o cierra el puerto de red 5555 de adb (modo remoto) |
| `adbuddy yolo` | Apaga el modo YOLO del agente, o mira si está activado |
| `adbuddy doctor` | Comprueba lo que ADBuddy necesita y explica cómo arreglarlo |
| `adbuddy env` | Gestiona el entorno del agente |
| `adbuddy screen` | Lista lo que hay en la pantalla del móvil y dónde tocar |
| `adbuddy tap` | Toca un elemento de la pantalla por texto, id o punto |
| `adbuddy type "<text>"` | Escribe texto en el campo con foco del móvil |
| `adbuddy foreground` | Enseña la app y la actividad en primer plano del móvil |
| `adbuddy notifications` | Lista las notificaciones actuales del móvil |
| `adbuddy screenshot` | Guarda una captura del móvil, reducida |
| `adbuddy send <file>` | Envía un fichero al móvil del usuario (para el agente) |
| `adbuddy version` | Muestra la versión de ADBuddy |

## Documentación

Las guías están en inglés, salvo la de preparación del móvil.

- [Instalar ADBuddy](docs/install.md) (en inglés): compilar desde el código, actualizar y desinstalar.
- [Prepara tu móvil](docs/setup/es/index.md): opciones de desarrollador, depuración inalámbrica y emparejamiento. La versión en inglés (capturas próximamente): [Set up your phone](https://hybriqtech.github.io/adbuddy/setup/).
- [Usar ADBuddy](docs/usage.md) (en inglés): arrancar el host, hablar con el agente, responder preguntas, deshacer, modo remoto.
- [Seguridad](docs/safety.md) (en inglés): lo que ADBuddy protege y lo que no garantiza.
- [Preguntas frecuentes](docs/faq.md) (en inglés): respuestas cortas.
- [Resolución de problemas](docs/troubleshooting.md) (en inglés): `adbuddy doctor` y los códigos de error.
- Las mismas guías como sitio web: https://hybriqtech.github.io/adbuddy/
- Para desarrolladores: [host/README.md](host/README.md) (en inglés).

<!-- Enable when the GitHub Sponsors and Ko-fi pages exist (docs/pre-public-checklist.md):

## Apoya el proyecto

ADBuddy es libre y de código abierto; si te ahorra tiempo, puedes apoyar su desarrollo.

[GitHub Sponsors](https://github.com/sponsors/hybriqtech) · [Ko-fi](https://ko-fi.com/<handle>)
-->

## Licencia

Apache-2.0. Consulta [LICENSE](LICENSE).

## Estado

Todavía no hay versión pública: la [página de Releases](https://github.com/hybriqtech/adbuddy/releases) es donde aparecerá. ADBuddy está en desarrollo activo y hoy se compila desde el código. Homebrew, winget y los binarios de Windows firmados todavía no están disponibles.
