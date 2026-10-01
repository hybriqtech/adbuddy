# ADBuddy M3 · Chat desde el móvil con cualquier agente · Diseño

- **Fecha:** 2026-10-01
- **Estado:** escrito por delegación del usuario; las decisiones están en §12
- **Spec de la v1:** [2026-09-29-adbuddy-v1-design.md](2026-09-29-adbuddy-v1-design.md) (§5.1, §5.5-§5.7, §6, §13)
- **Protocolo:** [2026-10-01-adbuddy-protocolo-m3-m5-design.md](2026-10-01-adbuddy-protocolo-m3-m5-design.md). M3 implementa el comportamiento de sus tipos de §5-§7; el contrato no cambia salvo lo que dice §10 aquí.
- **Parte de:** `main` con M3.0, m3-bridge (`SendTo`, `Reducer.reset`, callbacks aislados) y el protocolo de M5. Líneas paralelas: M5 (nautilus-19) y M6a (skimmer-32), coordinadas por adbuddy-e5 (`COORDINACION.md`, fuera del repo).

## 1. Qué resuelve

Hoy (M2) el agente corre con `adbuddy run` en su propia terminal, y el móvil solo ve el diario y las confirmaciones. M3 hace que la conversación con el agente viva en el host y se pueda seguir y llevar desde el móvil:

- `adbuddy serve` lanza el agente en un **PTY** y es dueño de la sesión;
- el PC se engancha y desengancha (`adbuddy attach`) sin cortar al agente;
- el móvil ve la **terminal** del agente, le escribe (**chat**), le manda y recibe **ficheros** y una **captura**;
- una **notificación de conversación** con respuesta directa, que se abre como **burbuja**;
- una **base de diseño** de la app (tema, modo oscuro, insets), que arregla lo que el usuario vio el 2026-10-01 en su Pixel.

Sin adaptador: la vista es la terminal para cualquier agente. Las burbujas de chat con el texto del agente son M4 (Claude Code).

### 1.1 Criterios de éxito

1. **El agente sobrevive a sus terminales.** Con `serve` en marcha, `adbuddy attach` engancha la terminal del PC a la sesión y Ctrl+] la suelta; el agente sigue. Una segunda terminal que se engancha ve la pantalla actual, no la salida desde el principio.
2. **Chat desde el móvil.** Un mensaje escrito en el móvil llega al agente como un único mensaje (texto multilínea y Unicode) y aparece en el chat de los dos lados; reintentarlo no lo duplica.
3. **Terminal en el móvil.** La vista de terminal muestra la TUI del agente (colores, repintados) y la maneja con texto y teclas rápidas; al volver tras perder la conexión muestra la pantalla actual sin basura.
4. **Ficheros en los dos sentidos.** Una foto o un fichero del móvil (hasta 200 MiB) llega a `inbox/` con su hash comprobado y se puede reanudar; `adbuddy send` desde el agente llega al móvil y se abre o se comparte; una captura de la pantalla del móvil llega al borrador.
5. **Notificación y burbuja.** La notificación fija muestra el estado, permite responder sin abrir la app y abrirse como burbuja; al terminar una tarea avisa "Listo".
6. **Diseño.** Todas las pantallas respetan las barras del sistema, se leen en claro, oscuro y con letra grande, y usan los colores del tema.
7. **Nada de M2 se debilita.** Clasificador, diario, confirmaciones 🔴, pausa y Cortar acceso funcionan igual; el texto que llega del móvil no salta al shim.

## 2. Alcance y planes

Una spec y dos planes, en árboles disjuntos que pueden ir en paralelo:

| Plan | Rama | Árbol | Contenido |
|---|---|---|---|
| **M3a · host** | `m3` | `host/` | Sesión en PTY dentro de `serve`, `attach`/`new`/`stop`, terminal hacia el móvil (emulador VT), chat, adjuntos, descargas, captura, `adbuddy send` |
| **M3b · app** | `m3` | `android/` | Base de diseño, pantalla de chat y terminal, adjuntos y descargas, captura, notificación de conversación y burbuja, avisos |

M3b puede hacerse contra un host falso (los tests usan el protocolo y sus fixtures). La prueba extremo a extremo con el móvil va al final de los dos.

**Fuera de M3:** el adaptador de Claude Code (hooks, transcript, permisos con botones: M4); las ayudas de accesibilidad y el modo remoto (M5); arrancar el daemon con el sistema y `adbuddy` sin argumentos que lo levanta (M6, con los instaladores).

## 3. La sesión en el host (M3a)

### 3.1 Quién es dueño de qué

- **`adbuddy serve`** sigue siendo el host en primer plano de su terminal: diario en vivo y preguntas 🔴 cuando la app no está (M2). Además lanza al agente en un **PTY propio** y es el único dueño de la sesión.
- **PTY:** `github.com/UserExistsError/conpty` v0.1.4 en Windows (validado en S3: Unicode, bracketed paste, redimensionar, Esc, `/exit`) y `github.com/creack/pty` v1.1.24 en Linux y macOS. Las dos son MIT y Go puro (sin cgo: los binarios de M6a salen con `CGO_ENABLED=0`).
- Una sesión por host (v1 §5.1). `epoch` y `conversation` (protocolo §5.1 y §7) son aleatorios y cambian con cada proceso del agente.

### 3.2 Arranque y ciclo de vida

- `serve` arranca la sesión con el agente de la configuración (`agent` en `config.toml`) cuando el móvil está listo, después de abrir el canal con la app. Si el agente no está instalado, `serve` sigue sin sesión y lo dice (v1 §9).
- **Relanzar:** si el host se reinicia, el siguiente arranque usa `claude --continue` para Claude Code (v1 §5.1); para los demás agentes, una conversación nueva.
- **`session.new` / `adbuddy new [agente]`:** termina el proceso actual (cierra el PTY, espera 5 s, mata el árbol de procesos) y lanza otro, sin `--continue`. **`session.stop` / `adbuddy stop`:** lo termina y deja la sesión en `stopped`. Si el agente sale solo (`/exit`), también `stopped`.
- **Entorno del agente:** el de M1 (`agent.Env`): el del usuario sin las variables de la sesión que lanzó ADBuddy, con el shim primero en `PATH`, `ANDROID_SERIAL` y `ADBUDDY_SOURCE=agent`, más `TERM=xterm-256color` y `COLORTERM=truecolor`. (Ruling §12: se mantiene la lista negra; no se pasa a lista blanca.)
- **`adbuddy run`** queda como estaba (el agente en la terminal del PC, sin PTY ni móvil): sirve para usar el entorno sin la app. La forma normal pasa a ser `serve` + `attach`.

### 3.3 Confianza de la carpeta (Claude Code)

Claude Code pregunta si confía en una carpeta nueva, con "No, exit" por defecto (S3); un Enter a ciegas cierra la sesión. Comprobado en el PC del usuario el 2026-10-01 con Claude Code 2.1.286: la confianza se guarda en `~/.claude.json` (o en `$CLAUDE_CONFIG_DIR/.claude.json`), en `projects["<carpeta con barras />"].hasTrustDialogAccepted`, con la ruta de la carpeta de trabajo (el entorno no es un repo git).

- **Antes de lanzar Claude Code**, el host marca el entorno como confiado si aún no lo está: lee el fichero, añade solo esa clave (creando `projects[ruta]` si falta, sin tocar nada más) y lo reemplaza de forma atómica. Si el fichero no existe o no se puede leer como JSON, no lo toca.
- **Si aun así aparece el diálogo** (otra sesión de Claude Code reescribió el fichero entre medias): el host lo detecta en la pantalla del emulador VT (§4.2) y entonces `chat.send` responde `error` `busy` con "Answer the question in the agent's terminal first". La persona lo contesta en la vista de terminal o en el PC.
- Solo para Claude Code; los demás agentes no tienen ese diálogo o se contestan en la terminal.

### 3.4 Terminal del PC: `adbuddy attach`

- `adbuddy attach` se conecta a `serve` por el IPC de M1 (socket local con secreto) con una petición nueva de tipo `attach`, pone la consola en modo crudo y pasa bytes en los dos sentidos. Al engancharse recibe la pantalla actual (§4.2) y después la salida en vivo.
- **Soltar:** Ctrl+] suelta la terminal y deja la sesión viva (el byte 0x1D nunca llega al agente). Ctrl+C va al agente.
- **Tamaño:** el de la terminal del PC enganchada manda (v1 §5.1; protocolo §7); `pcAttached` lo refleja en `session.state`. Con varias, la última que se enganchó. Sin ninguna, el del móvil si lo pidió, o 120×40.
- **Sin sesión** (agente no instalado o `stopped`): `attach` lo dice y propone `adbuddy new`.
- **Seguridad:** el IPC ya exige el secreto de M1 y rechaza al agente (`ADBUDDY_SOURCE=agent`): el agente no puede engancharse a su propia terminal ni escribirse.

### 3.5 Estado de la sesión

`session.state` (protocolo §5.1) se publica al cambiar:

| `state` | Cuándo |
|---|---|
| `working` | Hubo salida en el PTY en los últimos 4 s (configurable, v1 §5.5) |
| `waiting` | Tras `working`, 4 s sin salida |
| `idle` | Sesión recién lanzada sin actividad todavía |
| `stopped` | Sin proceso del agente |

`activity` lleva la última orden de adb del agente (la que ya ve el diario), y `view` es `terminal` (M4 pondrá `chat` cuando haya adaptador).

## 4. La terminal hacia el móvil (M3a)

### 4.1 Flujo

Cumple el protocolo §7: la salida se agrupa cada 50 ms en `terminal.output` (hasta 64 KiB), solo hacia una conexión suscrita (`terminal.watch`), con `SendTo` (m3-bridge) para que nunca llegue a otra conexión. `terminal.unwatch`, el cierre de la conexión o una conexión nueva (`area.attach`) terminan la suscripción.

### 4.2 Pantalla actual: emulador VT en el host

El host alimenta un emulador de terminal en memoria con toda la salida del PTY. Lo usan `terminal.snapshot`, el enganche del PC (§3.4) y la detección del diálogo de confianza (§3.3).

- **Biblioteca:** `github.com/charmbracelet/x/vt` (MIT, Go puro), fijada a un commit concreto porque el módulo no tiene versiones etiquetadas. Da `Write`, `Resize`, `Render()` (la pantalla con estilos como ANSI) y modo alternativo de pantalla.
- **Validado (spike S4, 2026-10-01, Claude Code 2.1.286 en ConPTY, commit de la biblioteca 7be62f4c612f):**
  - el emulador reconstruye bien la pantalla viva de Claude Code (cabecera, prompt, respuesta con Unicode y barra de estado), también tras redimensionar;
  - `Render()` separa las líneas con `
` sin ``, así que el host forma el `terminal.snapshot` como borrado (`ESC[2J ESC[H`), `Render()` con `
` cambiado a `
` y el cursor colocado en su posición (`CursorPosition`). Así redibujado en un emulador limpio sale idéntico;
  - Claude Code no hizo ninguna consulta a la terminal (posición del cursor, atributos). Las respuestas que genere el emulador del host se descartan: contestan las terminales de verdad (PC o xterm.js). Si un agente se queda esperando una respuesta sin ninguna terminal enganchada, se apunta como caso límite;
  - en una carpeta temporal nueva, Claude Code no mostró el diálogo de confianza, así que la detección (§3.3) queda como red de seguridad.
- **Respaldo** (si la biblioteca falla): cola de los últimos 256 KiB cortada en un salto de línea, precedida de un borrado de pantalla, y un cambio de tamaño de ida y vuelta para que la TUI se repinte (S3: Claude Code repinta la pantalla entera al redimensionar). No cambia el protocolo.
- **Reanudación:** con `terminal.watch {epoch, offset}`, el host manda lo que falta si `epoch` es el actual y los bytes siguen en su cola de 256 KiB; si no, `terminal.snapshot`.

### 4.3 Presión

M3a añade a `bridge.Server` la consulta de presión de la cola de la conexión (protocolo §6.4 y §7). Si pasa de la mitad, la terminal deja de mandar `terminal.output` y marca la pantalla como pendiente; cuando baja, manda un `terminal.snapshot`. La terminal nunca provoca que se expulse a la app. Las descargas (§6.2) usan la misma consulta para esperar hueco.

## 5. Chat (M3a + M3b)

### 5.1 Del móvil al agente

`chat.send {id, text, attachments[]}` (protocolo §5.2):

1. Si el `id` ya existe en el historial, se vuelve a confirmar con el mensaje existente y no se inyecta.
2. Si no hay sesión: `error` `no_agent`. Si la pantalla muestra el diálogo de confianza (§3.3): `error` `busy`.
3. Texto final: `text`, y si hay adjuntos, `\n\nAdjuntos:\n- inbox/<nombre>` por cada uno (v1 §5.6), con la ruta relativa al entorno.
4. **Inyección:** si el agente activó bracketed paste (el host vigila `ESC[?2004h`/`ESC[?2004l` en la salida), `ESC[200~ … ESC[201~` seguido de `\r`; si no, el texto con los saltos de línea cambiados a espacios, seguido de `\r` (un salto de línea a mitad enviaría el mensaje por trozos).
5. `chat.message {id, role: user, text, attachments}` con el `rid` de la petición (`PublishRID`).

Límites del protocolo §9: 64 KiB de texto, 10 adjuntos.

### 5.2 Del agente al móvil: `adbuddy send`

`adbuddy send <fichero> [--caption "…"]`, ejecutado por el agente en su entorno (ya está permitido en `.claude/settings.json`: `Bash(adbuddy:*)`):

- solo ficheros dentro del entorno del agente (como `adb pull`, v1 §4.6): fuera de él, error;
- copia el fichero a `outbox/<fecha>-<nombre>` y publica `chat.message {role: assistant, text: caption, attachments: [Attachment]}`;
- un fichero de más de 200 MiB, un directorio o un enlace simbólico que sale del entorno: error.

Cuando el móvil no está conectado, el mensaje queda en el búfer de eventos y en el `snapshot` (protocolo §4.3), y el fichero en `outbox/`.

### 5.3 Historial

El host guarda en memoria los últimos 100 `chat.message` de la sesión (los que van en el `snapshot`); no hay historial persistente en la v1 (v1 §14).

## 6. Ficheros (M3a + M3b)

### 6.1 Subidas del móvil

Cumple el protocolo §6.1:

- `inbox/<YYYYMMDD-HHMMSS>-<nombre saneado>` (v1 §5.6). Saneado: solo el nombre base, sin separadores, sin caracteres de control, sin nombres reservados de Windows, como mucho 100 caracteres; si se repite, sufijo `-2`, `-3`…
- Mientras se sube, en `inbox/.partial/<id>`; al comprobar tamaño y SHA-256 se renombra. Las incompletas sin avanzar durante una hora se borran.
- Como mucho 4 subidas a la vez por conexión (`busy`), 200 MiB por fichero (`too_large`), y 2 GiB en total de `inbox/` (`too_large` con texto que pide liberar espacio).
- El área de tramas protege su estado con su propio mutex, comprueba el offset (protocolo §6.4) y descarta tramas de ids no aceptados o de la conexión anterior (m3-bridge y pendientes de M3.0).

### 6.2 Descargas al móvil

`file.get {id, offset}` → tramas desde `offset` y `file.end`, por el mismo escritor (protocolo §6.2), con `SendTo` y esperando hueco (§4.3). Solo ids de adjuntos que el host anunció en esta sesión (subidas, capturas y `adbuddy send`); nunca una ruta.

### 6.3 Captura

`capture.request`: el host ejecuta `exec-out screencap -p` con el adb real, sin pasar por el shim (es una acción de la persona, no del agente), guarda `inbox/<fecha>-captura.png` y responde `capture.ready` con una miniatura JPEG de 16 KiB como máximo. Con "Cortar acceso" activo o sin móvil conectado: `error` `failed` con el motivo.

### 6.4 En la app

- **Elegir:** galería con el selector de fotos y ficheros con el del sistema, sin permisos (v1 §5.6).
- **Subir:** la app copia el fichero a su caché, calcula el SHA-256, sube con `attach.begin` → tramas (esperando mientras `Bridge.queuedBytes()` pase de 4 MiB) → `attach.end`, y reanuda tras una reconexión desde el offset que dé el host. El borrador muestra el progreso; enviar espera a que todos estén `attach.done`.
- **Descargar:** imágenes de hasta 10 MiB (5 MiB en modo remoto) se bajan solas; lo demás al pulsar. Se guardan en la caché de la app y se abren o comparten con un `FileProvider` (permiso de lectura temporal). La caché se limpia al pasar de 500 MiB, empezando por lo más antiguo.

## 7. La app (M3b)

### 7.1 Base de diseño

Antes de las pantallas nuevas, y aplicada también a las que ya existen (consentimiento, inicio, Actividad, confirmaciones):

- **Tema:** Material 3 con esquema claro y oscuro y color dinámico (Material You) en Android 12+; en Android 11, una paleta propia. Todos los colores salen de `MaterialTheme.colorScheme`; ninguno fijo.
- **Borde a borde:** cada pantalla respeta los insets (`Scaffold` o `windowInsetsPadding`): nada bajo la barra de estado ni la de navegación, y el campo de texto sube con el teclado (`imePadding`).
- **Tipografía y jerarquía:** escala de Material 3; pantallas con cabecera, contenido y acciones al pie. La de consentimiento pasa a: título, lista de lo que se concede con iconos, nota de privacidad aparte y botones al pie (la secundaria con contraste suficiente).
- **Comprobación:** cada pantalla con `@Preview` en claro, oscuro y letra al 200 % para revisarla en Android Studio, y una validación manual en el móvil del usuario (§11). Sin tests automáticos de capturas: necesitarían el emulador (bloqueado, M2.5b) o un plugin de capturas que hoy no compensa.
- Se usa la skill `frontend-design` al diseñar.

### 7.2 Pantallas

- **Chat (pantalla principal):**
  - cabecera: agente, estado (`session.state`), conexión (red local o Tailscale) y Pausar;
  - **terminal** arriba (vista por defecto sin adaptador), plegable;
  - debajo, la **conversación**: los mensajes de la persona y los ficheros del agente (`adbuddy send`), con tarjetas de fichero; las preguntas 🔴 en línea (M2);
  - **barra de entrada:** adjuntar (+), captura, campo de texto y enviar; con la terminal a la vista, una fila de **teclas rápidas** (Esc, Ctrl+C, ↑, ↓, Tab, Enter, 1, 2, 3) que van como `terminal.input`;
  - un mensaje se queda "no enviado" con Reintentar si su petición falla (protocolo §3).
- **Actividad** (el diario) y **Ajustes** como hoy, con el tema nuevo.
- Los `notice` y los `error` del host se muestran como avisos (snackbar), con su texto tal cual (lo escribe el host).

### 7.3 Vista de terminal

- **xterm.js** (MIT) con `addon-fit`, en un `WebView` que carga solo los ficheros incluidos en la app (`WebViewAssetLoader`), versión fijada en el plan.
- **El WebView no sale a la red:** `shouldInterceptRequest` niega todo lo que no sea de los assets; sin acceso a ficheros ni a contenido; JavaScript solo para xterm.js. El puente JS ↔ app tiene tres métodos: datos del teclado, nuevo tamaño y abrir enlace.
- **Enlaces** (OSC 8 y URLs detectadas): se abren con el navegador del sistema tras un diálogo que muestra el dominio. **OSC 52** (escribir en el portapapeles) no está activado.
- Suscripción (`terminal.watch`) solo mientras la vista está en pantalla; tamaño con `terminal.resize` al cambiar el hueco o la rotación.
- La app guarda la última pantalla (`epoch`, `offset` y lo escrito) en memoria para reanudar sin pedir un `snapshot` si no hace falta.

### 7.4 Notificación de conversación y burbuja

- La notificación fija de M2 (`BridgeService`) pasa a ser de **conversación** (`MessagingStyle`, con un acceso directo de conversación): muestra el estado ("Trabajando: …") y el último mensaje.
- **Respuesta directa** con `RemoteInput`: lo escrito se envía como `chat.send` con un `id` nuevo.
- **Acciones:** Enviar captura, Pausar/Reanudar y Cortar acceso.
- **Burbuja** (Bubbles API, Android 11+): la notificación se puede abrir como burbuja con la pantalla de chat. Si el usuario no permite burbujas, la notificación funciona igual.
- **Aviso de "Listo"** al pasar de `working` a `waiting` o `idle` tras al menos 10 s trabajando, con la primera línea del último mensaje del agente si la hay o "El agente terminó", en un canal propio que el usuario puede silenciar.
- Solo notificaciones locales (v1 §5.7).

## 8. Cambios en el protocolo

Ninguno en los tipos. M3 añade al host (no al protocolo):

- en `bridge.Server`, la consulta de presión de la cola (§4.3);
- en `cli`, el área de M3 (`chatArea`): manejadores de `chat.send`, `session.*`, `agent.interrupt`, `attach.*`, `file.*`, `capture.request` y `terminal.*`, su parte del `snapshot` (los últimos 100 mensajes) y su `attach`/`gone`/`frame`.

Si en el camino hace falta un campo nuevo, va en un solo commit en Go, Kotlin y fixture (protocolo §10), con el ok del supervisor.

### 8.1 Lo que M3 no cambia (para M5)

- La forma de `daemon.Handle` y de `daemon.exec` no cambia: M5 cuelga su guarda en `exec`, antes de ejecutar y después de las puertas de pausa, pregunta 🔴 y admisión (acordado con el supervisor).
- `serve.go` gana el arranque de la sesión y el registro del área de chat; M5 añade su línea al final, sobre lo de M3.

## 9. Fallos y casos límite

| Situación | Comportamiento |
|---|---|
| El agente sale (`/exit`, error) | `session.state` `stopped`; la terminal muestra su última pantalla; Nueva conversación lo relanza |
| `serve` se cierra con la sesión viva | El agente, hijo del PTY, termina con él (v1 §5.1); al volver, `--continue` |
| El móvil se desconecta a mitad de una subida | Se reanuda desde el offset del host al volver |
| La app se va a mitad de una descarga | La descarga se para; al volver, `file.get` desde lo que tenga |
| Dos PCs se enganchan | Las dos ven la sesión; manda el tamaño de la última |
| El diálogo de confianza aparece | `chat.send` responde `busy` y la persona contesta en la terminal (§3.3) |
| El agente escribe sin parar | La terminal se agrupa y, si la cola se llena, pasa a `snapshot` (§4.3) |
| Pausa activa | El chat y la terminal siguen (son de la persona); las órdenes de adb del agente siguen bloqueadas (M2) |

## 10. Seguridad

- El texto del chat y `terminal.input` llegan solo al PTY del agente: nunca a una pregunta de ADBuddy en el PC ni al shim. Lo que el agente haga con ellos pasa por el clasificador como siempre.
- `attach` exige el secreto del IPC y no lo puede usar el agente.
- Subidas: nombres saneados, carpeta fija, límites de tamaño y número; las rutas del PC nunca salen hacia la app.
- Descargas: solo ids anunciados; `adbuddy send` solo dentro del entorno.
- WebView sin red, sin ficheros y con un puente mínimo (§7.3).
- Ni el texto del chat ni el de la terminal se escriben en ningún log.

## 11. Tests

- **Host:**
  - PTY con un agente falso (un programa de test en Go que hace eco, activa bracketed paste y escribe colores) en Windows, Linux y macOS (CI de host);
  - emulador VT: `Render()` de salidas grabadas (incluida la del spike) frente a la pantalla esperada;
  - `attach`: enganchar, soltar con Ctrl+], dos terminales, tamaño;
  - chat: inyección con y sin bracketed paste, `id` repetido, diálogo de confianza;
  - subidas y descargas con hash, reanudación, límites, tramas de ids no aceptados (pendiente de M3.0) y de la conexión anterior;
  - captura con el adb falso;
  - extremo a extremo (`cmd/adbuddy`) con la app falsa: `chat.send` llega al agente falso y vuelve como `chat.message`.
- **App:** tests JVM de los reductores (chat, subidas, descargas), del puente de la terminal (sin WebView), de la notificación (constructores) y del saneado; los instrumentados siguen bloqueados por la imagen del emulador (M2.5b).
- **Validaciones manuales** (en el Pixel del usuario, al final): chatear con Claude Code desde el móvil, la vista de terminal con teclas rápidas, una foto y un fichero de ida y vuelta, una captura, responder desde la notificación, la burbuja, reconexión con la app en segundo plano, y las pantallas en claro, oscuro y letra grande.

## 12. Decisiones

- **Ruling: `serve` es el dueño de la sesión y `attach` la comparte.** Por qué: el agente sobrevive a las terminales, el móvil y el PC ven lo mismo, y no hace falta todavía un daemon en segundo plano (llega con los instaladores, M6). Coste si me equivoco: `serve` ocupa una terminal; se resuelve con el arranque automático de M6.
- **Ruling: emulador VT en el host (`charmbracelet/x/vt` fijado a un commit), validado con un spike al principio, con respaldo de cola + repintado.** Por qué: una pantalla exacta para el móvil, el PC y la detección del diálogo de confianza. Coste: una dependencia sin versión etiquetada; el respaldo no cambia el protocolo.
- **Ruling: se mantiene la lista negra de variables de entorno** (v1 §5.1 pedía lista blanca "a concretar en M3"). Por qué: una lista blanca rompe con facilidad la autenticación y las herramientas del usuario en tres sistemas, y la lista negra ya quita lo que filtró S3. Coste: una variable nueva de otra sesión de agente se colaría hasta añadirla.
- **Ruling: preconfiar la carpeta en `~/.claude.json`, añadiendo solo esa clave, y si aun así sale el diálogo, bloquear el chat hasta que se conteste.** Por qué: el diálogo con "No, exit" por defecto mataría la sesión con el primer mensaje del móvil. Coste: escribir en la configuración de Claude Code; si otra sesión la reescribe, la persona contesta una vez en la terminal.
- **Ruling: el chat de M3 muestra texto plano con enlaces; el markdown llega con M4.** Por qué: sin adaptador, del agente solo llegan los pies de `adbuddy send`. Coste: ninguno.
- **Ruling: la captura usa el adb real sin el shim ni el diario.** Por qué: la pide la persona desde el móvil y solo lee la pantalla. Coste: no aparece en Actividad.
- **Ruling: `adbuddy run` se queda como modo sin móvil.** Por qué: no rompe a quien lo usa y es el modo de usar el entorno sin la app. Coste: dos maneras de lanzar al agente; la documentación recomienda `serve` + `attach`.
- **Ruling: dos planes (M3a host, M3b app) en paralelo dentro de esta línea.** Por qué: árboles disjuntos y CLAUDE.md lo permite. Coste: la prueba extremo a extremo espera a los dos.
