# ADBuddy · Protocolo común de M3, M4 y M5 · Diseño

- **Fecha:** 2026-10-01
- **Estado:** escrito por delegación del usuario; las decisiones están en §11
- **Spec de la v1:** [2026-09-29-adbuddy-v1-design.md](2026-09-29-adbuddy-v1-design.md) (§5.2, §5.3, §5.6, §6 y §3.3). Esta spec **sustituye a la tabla de §5.3** y precisa §5.6
- **Parte de:** rama `protocolo`, apilada sobre `m2.5a` (ae2144f)

## 1. Por qué

M3 (chat), M4 (adaptador de Claude Code) y M5 (ayudas y modo remoto) amplían el mismo protocolo app ↔ host y tocan los mismos ficheros: `host/internal/bridge/protocol.go`, el `switch` de `host/internal/cli/phone.go`, `Protocol.kt` y el `when` de `BridgeState.kt`. Se van a hacer en dos líneas de trabajo en paralelo (memoria del 2026-09-30). Si cada hito ampliara el protocolo por su cuenta, chocarían en esos ficheros y M4 y M5 se escribirían contra un M3 a medio hacer.

Además, la tabla de §5.3 de la v1 deja huecos que hay que cerrar antes de escribir código:

- **La vista de terminal y el búfer de reanudación.** Cada trozo de salida del PTY sería un evento, y los 2000 eventos del búfer circular se llenarían en segundos con repintados de la TUI. Tras reconectar, los mensajes del chat y las preguntas ya no estarían en el búfer.
- **Peticiones repetidas al reconectar.** Un `ui.request` (pulsar en la pantalla) que se reenvía desde el búfer tras una reconexión ejecutaría un toque viejo.
- **Las respuestas no tienen correlación.** `capture.request` no tiene forma de casar su respuesta, y no hay tipo de error.
- **Los ficheros del agente se envían por empuje.** `file.incoming`, seguido de fragmentos, no se puede reanudar. Con el modo remoto a unos 84 KB/s, una foto de 10 MB tarda dos minutos.
- **Faltan tipos.** Al cerrarse un permiso en la terminal del PC, nada lo quita del móvil (las confirmaciones lo resuelven con el diario; los permisos no van al diario). Tampoco hay forma de activar el modo remoto ni de saber su estado.
- **El contrato está en cadenas escritas a mano.** Hoy lo sostienen `ProtocolTest.kt` y la app falsa del e2e (pendiente de M2).

## 2. Alcance

Una spec y un plan, **M3.0 · protocolo común**, en la rama `protocolo`, apilada sobre `m2.5a`. M3.0 deja hecho **todo el contrato** y ningún comportamiento nuevo:

- los tipos de mensaje de M3, M4 y M5, en Go y en Kotlin;
- las fixtures JSON compartidas y sus tests a los dos lados;
- las tres clases de mensaje (§3) en el servidor;
- el tipo `error` y la correlación por `rid`;
- el códec de las tramas binarias;
- los puntos de extensión que permiten a cada hito añadir sus manejadores en ficheros propios.

Lo que hace cada mensaje (el PTY, los hooks, la accesibilidad, el modo remoto) lo implementa su hito. Tras M3.0, el host responde `error {code: "unsupported"}` a lo que aún no maneja.

Después de M3.0 se abren las líneas acordadas:

| Línea | Hitos | Toca del protocolo |
|---|---|---|
| A | M3 → M4, y M5 en paralelo con M4 | Solo los manejadores de sus tipos (§10) |
| B | M6 sin protocolo (releases, skills) | Nada |

### 2.1 Criterios de éxito

1. **Un contrato ejecutable.** Cada tipo de §5-§8 tiene al menos una fixture en `protocol/fixtures/`. Go y Kotlin la decodifican en modo estricto (un campo que su estructura no conoce hace fallar el test) y, al volver a codificarla, sale el mismo JSON.
2. **Cobertura en los dos sentidos.** Un tipo declarado en Go o en Kotlin sin fixture, o una fixture sin tipo declarado a ese lado, hace fallar el test.
3. **Nada volátil se repite.** Un test del servidor comprueba que respuestas, flujos y `ui.request` no entran en el búfer ni se reenvían al reconectar.
4. **Compatibilidad con la app de M2.** Una app de M2 conectada a un host de M3.0 sigue funcionando: ignora los tipos nuevos y no recibe nada que entienda mal (test con las fixtures de M2 sin cambios).
5. **Sin conflictos entre líneas.** M3, M4 y M5 añaden manejadores sin tocar `protocol.go`, `Protocol.kt` ni un `switch` compartido (§10).

## 3. Clases de mensaje

Se mantiene el sobre de M2. Se le añade un campo opcional, `rid`:

```json
{"v":1,"seq":7,"type":"chat.message","ts":"2026-10-01T10:00:00Z","rid":"r12","payload":{…}}
```

Hay tres clases de mensaje del host a la app:

| Clase | `seq` | Búfer y reenvío | Ejemplos |
|---|---|---|---|
| **Evento** | > 0 | Entra en el búfer de 2000 y se reenvía al reanudar. Lo esencial también va en `snapshot` | `session.state`, `chat.message`, `permission.request`, `journal.entry` |
| **Respuesta** | 0 | Solo a la conexión actual, nunca se reenvía | `error`, `attach.ready`, `attach.done`, `file.end`, `capture.ready` |
| **Flujo** | 0 | Solo a la conexión actual, nunca se reenvía | `terminal.*`, `ui.request`, tramas binarias |

- **`rid` (identificador de petición).** Lo pone la app en las peticiones que esperan resultado. El host lo copia en la respuesta o en el `error` que le corresponde, y también en el evento que la confirma (el `chat.message` que confirma un `chat.send`). Es una cadena opaca de hasta 64 caracteres, única por conexión.
- **Peticiones perdidas.** Si la conexión cae antes de la respuesta, la app da la petición por fallida y la repite si tiene sentido. Las subidas se reanudan (§6.1) y `chat.send` no se repite solo, para no duplicar mensajes: la app muestra "no enviado" con la opción de reintentar. Por eso las respuestas pueden ser volátiles.
- **Envío al host.** `Server.Publish` sigue publicando eventos. M3.0 añade `Server.Send(typ, rid, payload) error` para respuestas y flujos: falla si no hay app conectada y nunca guarda nada en el búfer.
- **Lo que no se entiende.** La app ignora los tipos desconocidos, como en M2. El host responde a un tipo desconocido o no manejado con `error {code: "unsupported"}` si lleva `rid`, y con el `notice` de M2 si no lo lleva (así sigue funcionando una app de M2).

## 4. Tipos comunes

### 4.1 `error` (respuesta)

| Campo | Tipo | Contenido |
|---|---|---|
| `code` | cadena | Uno de la tabla siguiente |
| `text` | cadena | Para mostrar a la persona, en inglés. Lo escribe el host, nunca el agente |

| `code` | Cuándo |
|---|---|
| `unsupported` | Tipo que este host no maneja |
| `invalid` | Payload mal formado o fuera de límites |
| `not_found` | Id de adjunto, fichero o pregunta desconocido |
| `no_agent` | No hay sesión del agente para recibirlo |
| `busy` | Ya hay una operación igual en curso |
| `too_large` | Supera un límite (§9) |
| `checksum` | El SHA-256 no coincide |
| `failed` | Ha fallado en el host (adb, disco…); `text` dice qué |

### 4.2 `Attachment` (estructura, no es un tipo de mensaje)

Describe un fichero que el host guarda: un adjunto subido desde el móvil, una captura o un fichero que envía el agente.

| Campo | Tipo | Contenido |
|---|---|---|
| `id` | cadena | 32 caracteres hexadecimales en minúsculas (16 bytes, el prefijo de las tramas binarias) |
| `name` | cadena | Nombre para mostrar, ya saneado por el host |
| `mime` | cadena | |
| `size` | entero | Bytes |
| `sha256` | cadena | Hexadecimal |
| `thumb` | cadena, opcional | Miniatura JPEG en base64, de 16 KiB como máximo, solo para imágenes |

Un adjunto se descarga con `file.get` (§6.2). La app nunca ve ni manda rutas del PC.

### 4.3 `snapshot` (evento; amplía el de M2)

| Campo | Desde | Contenido |
|---|---|---|
| `state` | M2 | `session.state` |
| `pending` | M2 | `confirm.request` pendientes |
| `journal` | M2 | Últimas entradas del diario |
| `chat` | M3 | Últimos 100 `chat.message`, de más antiguo a más reciente |
| `permissions` | M4 | `permission.request` pendientes |
| `remote` | M5 | `remote.state` |

Los campos nuevos son listas vacías u objetos por defecto mientras su hito no los rellena. La terminal no va en el `snapshot`: tiene su propio flujo (§7).

## 5. Sesión y chat (M3, ampliado en M4)

### 5.1 Host → app

| Tipo | Clase | Payload |
|---|---|---|
| `session.state` | evento | `agent`, `state` (`idle`, `working`, `waiting` o `stopped`), `activity`, `paused` (M2) y, nuevos: `conversation` (id que cambia con cada `session.new` y en cada relanzamiento), `view` (`chat` si hay adaptador, `terminal` si no) y `pcAttached` (hay una terminal del PC conectada) |
| `chat.message` | evento | `id`, `role` (`user` o `assistant`), `text` (markdown), `attachments[]` (`Attachment`) y `conversation`. **Upsert:** un `chat.message` con un `id` ya visto sustituye al anterior, porque el transcript de Claude Code escribe un mensaje en varias líneas (M4). Si confirma un `chat.send`, lleva su `rid` |
| `tool.activity` | evento | `id`, `messageId` (opcional), `summary`, `status` (`running`, `done` o `error`). Upsert por `id` (M4) |
| `permission.request` | evento | `id`, `tool`, `summary`, `detail`, `expiresAt` y `allowAlways` (si se puede ofrecer "Sí, siempre") (M4) |
| `permission.resolved` | evento | `id`, `decision` (`allow`, `allow_always` o `deny`), `by` (`phone`, `terminal`, `timeout` o `agent`). Quita la tarjeta del móvil se responda donde se responda (M4) |

- **En M3, sin adaptador,** el host emite `chat.message` con `role: user` al inyectar un `chat.send` en el PTY, y `role: assistant` con adjuntos cuando el agente ejecuta `adbuddy send` (§5.6 de la v1). El texto de la respuesta del agente se ve en la terminal.
- **`adbuddy send`** produce un `chat.message` de `role: assistant` con `text` = el `--caption` y el fichero en `attachments`. Desaparece el tipo `file.incoming` de §5.3.
- **Markdown seguro.** La app pinta el markdown sin HTML y sin cargar imágenes remotas (una URL de imagen en la respuesta del agente filtraría datos al abrir el chat). Los enlaces solo se abren al pulsarlos, mostrando antes el dominio.

### 5.2 App → host

| Tipo | `rid` | Payload | Resultado |
|---|---|---|---|
| `chat.send` | sí | `text` y `attachments[]` (ids de adjuntos ya subidos o capturados) | El `chat.message` de `role: user` con el mismo `rid`, o `error` (`no_agent`, `not_found`, `too_large`) |
| `session.new` | sí | `agent` (opcional) | `session.state` con la `conversation` nueva, o `error` |
| `session.stop` | sí | – | `session.state` `stopped`, o `error` |
| `agent.interrupt` | sí | – | Envía Esc al PTY. Sin respuesta si va bien; `error` `no_agent` si no hay sesión |
| `permission.reply` | no | `id`, `decision` (`allow`, `allow_always` o `deny`) | `permission.resolved`, o el `notice` de respuesta tardía de M2 |
| `pause`, `resume`, `confirm.reply` | no | Igual que en M2 | Igual que en M2 |

## 6. Adjuntos y ficheros (M3)

### 6.1 Subida (app → host)

1. `attach.begin` (con `rid`): `id` (lo genera la app, `Attachment.id`), `name`, `mime`, `size` y `sha256`. La app calcula el hash antes de subir.
2. El host responde `attach.ready {id, offset}`, donde `offset` es lo que ya tiene de una subida anterior con el mismo `id` y el mismo `sha256`, o 0 si no tiene nada.
3. La app envía tramas binarias desde `offset`.
4. `attach.end` (con `rid`): `id`.
5. El host comprueba tamaño y hash, guarda el fichero en `inbox/` (con el nombre de §5.6 de la v1) y responde `attach.done {attachment}`, o `error` (`checksum`, `too_large`, `invalid`).

Si la conexión cae, la app vuelve a empezar por `attach.begin` con el mismo `id` y sigue desde el `offset` que le dé el host. El host descarta las subidas incompletas que lleven una hora sin avanzar.

### 6.2 Descarga (host → app)

Ningún fichero se empuja: el host anuncia adjuntos en los `chat.message` y la app los pide.

1. `file.get` (con `rid`): `id` y `offset`.
2. El host envía tramas binarias desde `offset` y después `file.end {id, size}`, o `error` (`not_found`).
3. La app comprueba el `sha256` del `Attachment`. Si no coincide, borra lo descargado y lo pide desde 0 una vez.

La app descarga sola las imágenes de hasta 10 MiB (5 MiB en modo remoto). El resto se descarga al pulsar la tarjeta. `file.cancel {id}` (sin respuesta) para una descarga en curso.

### 6.3 Captura

`capture.request` (con `rid`, sin payload): el host hace `exec-out screencap -p` con el adb real y guarda la imagen en `inbox/`. Responde `capture.ready {attachment}`, con `thumb`, o `error` `failed`. La app la añade al borrador y la envía, como cualquier adjunto, con `chat.send`.

### 6.4 Tramas binarias

```
[16 bytes: id del adjunto][datos: de 1 B a 256 KiB]
```

- Solo transportan datos de una subida aceptada (`attach.ready`) o de una descarga pedida (`file.get`). Las demás se descartan.
- Se envían en orden. Varias transferencias pueden intercalarse porque cada trama lleva su id.
- **Ritmo:** el host no mete las tramas de una descarga en la cola de 256 mensajes, cuyo desbordamiento corta la conexión de una app lenta (`sendLocked` en M2). Las genera el escritor a medida que salen.
- M3.0 incluye el códec (`EncodeFrame`/`DecodeFrame` y su equivalente en Kotlin) con vectores de test compartidos en `protocol/fixtures/frames.json`.

## 7. Terminal (M3)

La salida del PTY es un flujo con posición propia, fuera del búfer de eventos:

| Tipo | Sentido | Clase | Payload |
|---|---|---|---|
| `terminal.watch` | app → host | – | `epoch` y `offset` de lo último que tiene la app (vacío y 0 si nada) |
| `terminal.unwatch` | app → host | – | – |
| `terminal.snapshot` | host → app | flujo | `epoch`, `offset`, `cols`, `rows` y `data` (base64): bytes que, escritos en una terminal limpia de `cols`×`rows`, reproducen la pantalla en ese `offset` |
| `terminal.output` | host → app | flujo | `epoch`, `offset` (posición del primer byte) y `data` (base64, hasta 64 KiB antes de codificar) |
| `terminal.size` | host → app | flujo | `epoch`, `offset`, `cols` y `rows`: el tamaño cambia a partir de ese byte |
| `terminal.input` | app → host | – | `data` (base64): bytes al PTY, tal cual (teclas rápidas incluidas) |
| `terminal.resize` | app → host | – | `cols` y `rows` que caben en la vista del móvil |

- **`epoch`** identifica un proceso del PTY. Cambia con `session.new` y con cada relanzamiento. **`offset`** cuenta los bytes de salida desde el inicio de ese `epoch`.
- **Suscripción.** El host solo envía la terminal a una app que la mira (`terminal.watch`), y deja de hacerlo con `terminal.unwatch` o al cortarse la conexión. La app se suscribe mientras la vista de terminal está en pantalla. Así no gasta batería ni datos en segundo plano: en modo remoto, a 84 KB/s, una TUI repintando saturaría el enlace.
- **Reanudación.** Al recibir `terminal.watch`, el host envía lo que falta desde el `offset` de la app si su `epoch` es el actual y los bytes siguen en su cola (al menos 256 KiB). Si no, envía `terminal.snapshot`. Cómo genera el host la pantalla (emulador VT sin interfaz o cola acotada) lo decide M3.
- **Presión.** El host agrupa la salida (por ejemplo, cada 50 ms). Si la cola de envío de la app pasa de la mitad, deja de mandar `terminal.output` y, cuando se vacía, manda un `terminal.snapshot`. La terminal nunca provoca que se corte la conexión de la app.
- **Tamaño.** El del PC manda: `terminal.resize` solo se aplica si `pcAttached` es falso; si no, se ignora. Sin terminal del PC ni petición de la app, 120×40 (§5.1 de la v1).
- **La vista** (xterm.js) desactiva OSC 52 (escritura en el portapapeles), y los enlaces OSC 8 se abren como los del chat (§5.1).

## 8. Ayudas y modo remoto (M5)

### 8.1 Ayudas al agente

| Tipo | Sentido | Clase | Payload |
|---|---|---|---|
| `ui.request` | host → app | flujo | `id`, `op`, `args` y `expiresAt` |
| `ui.response` | app → host | – | `id`, `ok` y `result` (si `ok`) o `error {code, text}` (si no) |

- **Es un flujo, no un evento:** una petición de la pantalla nunca se reenvía tras reconectar. La app descarta las que ya han caducado (`expiresAt`, juzgado con el reloj del host como en M2) y no ejecuta dos veces el mismo `id`.
- **Operaciones de partida:** `dump`, `click`, `setText`, `foreground` y `notifications` (§6 de la v1). Esta spec fija solo el sobre de cada una:
  - `click` admite `args` `{id}`, `{text}` o `{x, y}`;
  - `setText` admite `{text}`;
  - las demás van sin `args`;
  - `dump` devuelve `{locked, package, elements[]}`.

  Los campos de cada elemento, los gestos y las operaciones nuevas los fija la spec de M5. M5 puede ampliar `ui.*` (operaciones, `args`, `result`) añadiendo sus fixtures, sin tocar el resto del protocolo.
- **Códigos de error propios:**
  - `unavailable`: el servicio de accesibilidad no está activo;
  - `protected`: la acción caería sobre una app protegida o sobre la propia ADBuddy, `tech.hybriq.adbuddy` (pendientes de M5);
  - `expired`;
  - `not_found`: no hay ningún elemento con ese id o texto.

### 8.2 Información del móvil y modo remoto

| Tipo | Sentido | Clase | Payload |
|---|---|---|---|
| `device.info` | app → host | – | `tailscaleIp` (opcional) y `wifiAdbPort` (opcional): para reconectar fuera de casa (§3.3 de la v1) |
| `remote.set` | app → host | con `rid` | `enabled`. El host ejecuta `adb tcpip 5555` o `adb usb` (§3.3) |
| `remote.state` | host → app | evento | `enabled`, `address` (`ip:puerto`, si está activo) y `expiresAt`. También va en el `snapshot` |

`remote.set` responde con el `remote.state` nuevo (con su `rid`) o con `error`.

## 9. Límites

| Qué | Límite | Si se supera |
|---|---|---|
| Mensaje de texto | 1 MiB (como en M2) | El host lo descarta (M2) |
| `chat.send.text` | 64 KiB | `error` `too_large` |
| Adjuntos por `chat.send` | 10 | `error` `too_large` |
| Trama binaria | 16 B + 256 KiB | Se descarta |
| Adjunto | 200 MiB (§5.3 de la v1) | `error` `too_large` en `attach.begin` |
| Subidas en curso | 4 por conexión | `error` `busy` |
| `terminal.input.data` | 64 KiB | `error` `too_large` si lleva `rid`; si no, se descarta |
| `rid` | 64 caracteres | El mensaje se descarta |

## 10. Puntos de extensión (para trabajar en paralelo)

M3.0 deja el contrato completo y estos puntos, para que cada hito añada **solo ficheros suyos**:

- **Host:** `cli/phone.go` deja el `switch` y pasa a un registro `map[tipo]manejador`. Los tipos de M2 quedan registrados. Cada hito registra los suyos en su propio fichero (`phone_chat.go`, `phone_agent.go`, `phone_ui.go`) desde una función que llama `newPhoneLink`; esa función la deja M3.0 vacía con un comentario por hito. El `snapshot` se compone igual: cada parte aporta su función.
- **App:** `BridgeState.apply` pasa a delegar en reductores por área (`ConfirmReducer`, de M2; `ChatReducer`, de M3/M4; `RemoteReducer`, de M5). M3.0 crea los vacíos. `Bridge` gana `listen(tipo)` y `listenFrames` (respuestas, flujos y tramas, que el estado no guarda), `send`, `request` (pone un `rid` nuevo y lo devuelve) y `sendFrame`, para que ningún hito tenga que tocar el cliente WebSocket.
- **Tipos y fixtures:** todos los de §4-§8 quedan escritos en M3.0. Si un hito necesita un campo nuevo, lo añade al tipo, a las dos implementaciones y a la fixture en el mismo commit. Es el único cambio permitido en `protocol.go` y `Protocol.kt`, y la línea que lo haga se lo dice a la otra al integrar.

| Hito | Tipos de los que implementa el comportamiento |
|---|---|
| M3 | `session.*` (y los campos nuevos de `session.state`), `chat.send`, `chat.message` (`user` y `adbuddy send`), `agent.interrupt`, `attach.*`, `file.*`, `capture.*`, `terminal.*`, mostrar los `notice` y los `error` |
| M4 | `chat.message` (`assistant`), `tool.activity` y `permission.*` |
| M5 | `ui.*`, `device.info` y `remote.*` |

## 11. Decisiones

- **Ruling: el protocolo sigue siendo `v1` y `protocol: 1` hasta la primera release pública.** Todo lo nuevo es aditivo: la app ignora tipos y campos desconocidos, y el host contesta `unsupported`. Desde la primera release, cada cambio que no sea aditivo sube `protocol` y se ajustan los mínimos (`MinAppProtocol`, `MIN_HOST_PROTOCOL`). **Por qué:** no hay ninguna app publicada, y el host instala la app con `adb install` (M2). **Coste si me equivoco:** en desarrollo, una app y un host de ramas distintas pueden entenderse a medias; se arregla recompilando los dos.
- **Ruling: tres clases de mensaje y `rid` en el sobre.** **Por qué:** cierra la repetición de `ui.request`, evita que la terminal vacíe el búfer y da una correlación uniforme sin un tipo de respuesta por cada petición. **Coste si me equivoco:** si una respuesta volátil se pierde, la app tiene que repetir la petición. Está asumido en §3.
- **Ruling: descarga por petición (`file.get`) en lugar del `file.incoming` empujado.** **Por qué:** se puede reanudar, la app decide qué descargar en modo remoto y los adjuntos se unifican en `chat.message`. **Coste si me equivoco:** un viaje de ida y vuelta más por fichero.
- **Ruling: la terminal es un flujo con `epoch` y `offset`, solo mientras la app la mira.** **Por qué:** reanuda sin pasar por el búfer de eventos y no gasta en segundo plano. **Coste si me equivoco:** si M3 no consigue generar `terminal.snapshot` fiable para la TUI de Claude Code, la reanudación degrada a limpiar y repintar desde la cola. Se acota en el plan de M3, no cambia el protocolo.
- **Ruling: M3.0 escribe todos los tipos, incluidos los de M4 y M5.** **Por qué:** las dos líneas no chocan en `protocol.go` ni en `Protocol.kt`, y M4/M5 no esperan a M3 para conocer su contrato. **Coste si me equivoco:** M5 puede necesitar otros campos en `ui.*`; los añade en su spec (§8.1 lo prevé).
- **Ruling: fixtures en `protocol/fixtures/`, en la raíz.** **Por qué:** no son de `host/` ni de `android/`, y los dos árboles las leen: Go con una ruta relativa al paquete y Gradle con una propiedad del sistema que apunta a la carpeta. **Coste si me equivoco:** mover una carpeta.
- **Ruling: el `notice` de M2 se queda para lo que no lleva `rid`.** **Por qué:** una app de M2 no conoce `error` y tiene que seguir funcionando (criterio 4). **Coste si me equivoco:** ninguno.

## 12. Tests

- **Fixtures** (`protocol/fixtures/<sentido>/<tipo>[.<variante>].json`, con `<sentido>` igual a `host` o `app`):
  - el fichero base de cada tipo lleva todos los campos y se comprueba con ida y vuelta estricta a los dos lados;
  - las variantes (campos opcionales ausentes, valores límite) solo se decodifican en modo estricto;
  - `frames.json` guarda los vectores de las tramas binarias.
- **Cobertura:** Go y Kotlin tienen un registro `tipo → estructura` y un test que lo cruza con los ficheros de las fixtures.
- **Servidor:**
  - `Send` no guarda en el búfer y falla sin app;
  - un tipo desconocido con `rid` da `error` `unsupported`, y sin `rid` da `notice`;
  - un `rid` de más de 64 caracteres se descarta;
  - las tramas binarias de ids no aceptados se descartan.
- **Compatibilidad con M2:** las variantes `@m2` (`host/session.state@m2.json`, `host/snapshot@m2.json` y `app/hello@m2.json`) son lo que envían un host y una app de M2, y la app falsa del e2e sigue pasando sin cambios. `ProtocolTest.kt` se queda como está (prueba campos desconocidos y anidamiento absurdo, no el contrato).
- **Registro completo:** en Kotlin, un test cruza las constantes de tipo de `Protocol` con el registro, para que una constante nueva sin registrar no se quede sin fixture.
- **Fuera de M3.0:** el comportamiento de cada tipo se prueba en su hito, con las mismas fixtures como entrada.
