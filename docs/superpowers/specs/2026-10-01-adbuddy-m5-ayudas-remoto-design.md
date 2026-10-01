# ADBuddy · M5 · Ayudas al agente y modo remoto · Diseño

- **Fecha:** 2026-10-01
- **Estado:** escrito por delegación del usuario; las decisiones están en §13, y las que necesitan el ok del supervisor (adbuddy-e5) están marcadas
- **Spec de la v1:** [2026-09-29-adbuddy-v1-design.md](2026-09-29-adbuddy-v1-design.md) (§3.3, §4.6, §5.7 y §6)
- **Protocolo:** [2026-10-01-adbuddy-protocolo-m3-m5-design.md](2026-10-01-adbuddy-protocolo-m3-m5-design.md) (§8 y §10). Esta spec fija los `args` y los `result` de cada operación de `ui.*`, que aquella deja a M5
- **Pendientes que recoge:** `docs/superpowers/plans/pendientes-m3-m5.md`, sección M5
- **Rama:** `m5`, en el worktree `nautilus`, sale de `main` (0ec286f)

## 1. Qué y por qué

Hoy el agente solo ve el móvil con `uiautomator dump` (de 2 a 4 s y, además, desconecta el servicio de accesibilidad) y solo toca con `input tap` e `input text` (solo ASCII). Además, nada impide que actúe sobre una app de banca abierta. M5 trae:

1. **Ayudas al agente:** `adbuddy screen`, `tap`, `type`, `foreground` y `screenshot`. Las resuelve el servicio de accesibilidad de la app, unas 10 veces más rápido (S2), y con Unicode completo. Si la app no está, el host usa adb como respaldo.
2. **Protecciones en la interfaz:** apps protegidas y la propia ADBuddy (§4.6 de la v1 y pendientes de M5). Lo que lee o toca la pantalla se bloquea con ellas en primer plano, venga de una ayuda o de un `adb shell input`.
3. **Superposición** (§5.7 de la v1): marco y aviso "Agente usando la pantalla · Pausar", con una marca donde toca.
4. **Notificaciones** (opcional): `adbuddy notifications`.
5. **Modo remoto** (§3.3 de la v1): `device.info`, `remote.set` y `remote.state`.

### 1.1 Tres sub-hitos

Se reparte en tres planes, por dependencias:

| Sub-hito | Contenido | Depende de |
|---|---|---|
| **M5a** | Ayudas 1-3: servicio de accesibilidad, operaciones `dump`, `click`, `setText` y `foreground`, CLI, respaldo por adb, captura, apps protegidas (lista por heurística, aún sin pantalla para editarla) y superposición | `m3-bridge` en `main` (arreglos de `Bridge.kt` y `reset(s)`). Las líneas compartidas de `daemon.go`, `serve.go` y `root.go`, al final y rebasadas sobre M3 |
| **M5b** | Modo remoto completo, con su interruptor en la app | Que M3 haya cerrado `serve.go` (bucle de reconexión) y la base de diseño de la app |
| **M5c** | Notificaciones; pantallas de apps protegidas y de acceso a notificaciones (onboarding y Ajustes) | La base de diseño de M3 |

Esta spec cubre los tres. El plan que se escribe primero es el de M5a.

### 1.2 Criterios de éxito

1. `adbuddy screen` devuelve la lista de elementos en menos de 500 ms con la app conectada en la misma Wi-Fi (S2: 31-129 ms en el móvil, más el transporte).
2. `adbuddy type "¿Qué tal? 👋"` escribe el texto tal cual en el campo con foco.
3. Ninguna ayuda ni ningún `input` actúa mientras una pregunta 🔴 espera respuesta o con la sesión en pausa. Tampoco con ADBuddy o una app protegida en primer plano (tests del daemon).
4. Sin la app, las mismas órdenes funcionan por adb, con los límites de §5.
5. Ningún texto de un campo de contraseña sale del móvil.

## 2. Arquitectura de una ayuda

```
agente ──► adbuddy tap --text OK          (CLI, en la sesión del agente)
            │ IPC, kind "ui"
            ▼
         daemon ── pausa, pregunta 🔴, admisión en vuelo, guarda de apps protegidas, diario
            │ Options.UI (el gancho que da serve)
            ▼
         phoneLink.uiArea ──► ui.request (flujo, con id y expiresAt) ──► app
            ▲                                                          │
            └──────────────── ui.response (id) ◄── UiExecutor ◄── AgentAccessibilityService
            │
            └─ si la app no está o responde unavailable: respaldo por adb (§5)
```

- **CLI** (`host/internal/cli/helpers*.go`, nuevos): cada ayuda es un subcomando. Manda un `ipc.Request{Kind: "ui", Argv: [op, …]}` al daemon y escribe la respuesta: texto compacto o `--json` con el sobre habitual de la CLI. Sin `adbuddy serve` en marcha, sale con `DAEMON_NOT_RUNNING`: las ayudas, como el shim, solo existen con el host.
- **Daemon** (`host/internal/daemon/ui.go` y `guard.go`, nuevos): el kind `ui` pasa por las mismas puertas que una orden del shim (§4.1). Después ejecuta con `Options.UI` y, si hace falta, con el respaldo. Cada ayuda deja su entrada en el diario.
- **Enlace con la app** (`host/internal/cli/phone_ui.go`): `uiArea` envía `ui.request` con `Server.Send`, o con el envío atado a la conexión que traiga M3 (`SendTo`). Espera el `ui.response` de su `id` hasta `expiresAt` y descarta las respuestas de ids que no espera. Un `ui.response` solo vale de la conexión a la que se envió la petición (`Server.Current(m)`, §10 del protocolo).
- **App** (`android/.../a11y/`): `AgentAccessibilityService` guarda una referencia a sí mismo mientras está vinculado. `UiExecutor` escucha `ui.request` con `Bridge.listen` y ejecuta las operaciones de una en una, en un hilo propio, nunca en el principal. Responde con `Bridge.send(ui.response)`.

**Plazos.** El host pone `expiresAt` = ahora + 10 s, y la app descarta lo caducado (con `HostClock`, como en M2). Si en ese plazo no llega respuesta, el daemon da la operación por fallida. Para `dump`, `foreground` y `notifications`, que solo leen, prueba entonces el respaldo. Para `click` y `setText` no lo prueba: un toque que pudo llegar a ejecutarse no se repite por adb. El agente recibe `UI_TIMEOUT` y la entrada del diario queda en `error` (incierta), como una orden de adb cortada.

**Una operación a la vez.** El host serializa las operaciones de acción (`click` y `setText`) con la admisión en vuelo del daemon, como cualquier orden 🟡. La app las ejecuta en orden y no ejecuta dos veces el mismo `id`: guarda los últimos 64.

## 3. Operaciones

M3.0 dejó `args` y `result` como JSON libre (`json.RawMessage` y `JsonElement`). M5 los define aquí, con estructuras propias en ficheros nuevos (`bridge/ui_ops.go`, `ProtocolUiOps.kt`) y una fixture por operación (§12). `UIRequest` y `UIResponse` no cambian.

### 3.1 `dump` → `adbuddy screen [--all] [--json]`

`args`: `{}`.

`result`:

```json
{
  "locked": false,
  "package": "com.android.settings",
  "activity": "com.android.settings.Settings",
  "width": 1344, "height": 2992, "rotation": 0,
  "truncated": false,
  "elements": [
    {"id": "android:id/title", "text": "Network & internet", "desc": "", "hint": "",
     "class": "TextView", "bounds": [42, 610, 980, 680],
     "click": true, "edit": false, "scroll": false, "focused": false,
     "checked": null, "enabled": true, "password": false, "window": "app"}
  ]
}
```

- **Qué ventanas:** todas las interactivas (`flagRetrieveInteractiveWindows`), ordenadas por capa de mayor a menor: la app, sus diálogos, el teclado y la interfaz del sistema. Es la idea de `HierarchyDumper` de ARTEMIS (Apache-2.0; la atribución va en `NOTICE` y en la cabecera del fichero si se copia código). Las ventanas de `tech.hybriq.adbuddy`, incluida la superposición, no salen nunca.
- **Qué nodos:** los visibles para el usuario, con los límites recortados a la pantalla, a su ventana y a sus antepasados desplazables (paridad con UIAutomator). Entre ellos, solo los informativos (texto, descripción o pista) o los que se pueden usar (pulsar, editar, desplazar o marcar). Las capas sin nada de eso desaparecen. `--all` (`args: {"all": true}`) incluye todos los nodos visibles.
- **Topes:** 2000 elementos, profundidad 75. Más que eso pone `truncated: true`. Si todo junto pasa de 512 KiB, se recortan los textos largos y, si aún no cabe, los últimos elementos.
- **Contraseñas:** de un nodo con `isPassword` sale `password: true`, y `text` y `hint` salen vacíos.
- **Bloqueado:** con el móvil bloqueado (`KeyguardManager.isKeyguardLocked`), `locked: true` y `elements` vacío. El agente no debe intentar desbloquear (la regla 14 de `AGENTS.md`).
- **App protegida en primer plano:** `error` `protected` (§4.3).
- **`id`:** el `viewIdResourceName` tal cual (`paquete:id/nombre`). La CLI lo muestra sin el paquete si es el de la app en primer plano.
- **`window`:** `app`, `dialog`, `ime`, `system` o `split`.

**Salida de texto de `adbuddy screen`** (una línea por elemento, pensada para el agente):

```
Settings (com.android.settings) · 1344×2992
[title] "Network & internet" TextView (42,610)-(980,680) click
[search_action_bar] desc="Search settings" (42,180)-(1302,300) click
- "Wi-Fi" Switch (980,700)-(1302,780) click checked=false
```

`[id]` sale si hay `id`, y si no, un `-`. Los textos van entre comillas, recortados a 120 caracteres y escapados con `classify.Printable`: un texto de la pantalla no puede meter secuencias de control en la terminal del agente.

### 3.2 `click` → `adbuddy tap "<texto>" | --id <id> | --xy x,y`

`args`: `{"text": "..."}`, `{"id": "..."}` o `{"x": 10, "y": 20}`, solo uno.

`result`: `{"clicked": true, "bounds": [l, t, r, b], "how": "action" | "gesture"}`.

- **Por `id` o por texto:** se busca entre los nodos que `dump` mostraría, por este orden: `id` exacto; texto o descripción exactos; y los mismos sin distinguir mayúsculas. Del nodo que coincide se sube al antepasado pulsable más cercano, y se pulsa con `ACTION_CLICK` (`how: action`). Si coinciden varios pulsables distintos, el error es `ambiguous` (§12), con el número de coincidencias y los límites de las 5 primeras en `text`. El agente repite entonces con `--id` o `--xy`. Si no coincide ninguno, `not_found`.
- **Por coordenadas:** un toque con `dispatchGesture` (`how: gesture`), 50 ms. Antes de tocar se mira qué ventana tiene encima ese punto. Si es de ADBuddy o de una app protegida, el error es `protected`.
- La superposición marca el punto (§6).

### 3.3 `setText` → `adbuddy type "<texto>"`

`args`: `{"text": "..."}`, hasta 4096 caracteres.

`result`: `{"set": true}`.

- Va al nodo con el foco de entrada (`findFocus(FOCUS_INPUT)`), con `ACTION_SET_TEXT`. Reemplaza el texto del campo: así es la acción de Android. Para añadir, el agente lee el campo con `screen` y manda el texto completo.
- Sin campo con foco: `not_found` ("Tap a text field first").
- Campo de contraseña: `protected` ("ADBuddy does not type into password fields: ask the user to type it").
- Bloqueado: `protected` ("The phone is locked: ask the user to unlock it").

### 3.4 `foreground` → `adbuddy foreground`

`args`: `{}`. `result`: `{"package": "...", "activity": "...", "protected": false}`.

La app lo sabe por los `TYPE_WINDOW_STATE_CHANGED` de la ventana de aplicación activa. `activity` puede salir vacío si la ventana no es una actividad. El daemon también lo usa para la guarda (§4.3).

### 3.5 `notifications` → `adbuddy notifications` (M5c)

`args`: `{}`. `result`: `{"notifications": [{"key": "...", "package": "...", "title": "...", "text": "...", "postedAt": "RFC3339", "ongoing": false}]}`.

- Necesita `AgentNotificationListener` con acceso concedido por el usuario. Si no lo tiene, el error es `unavailable` y el texto dice cómo concederlo.
- De las apps protegidas y de ADBuddy sale el paquete, y `title` y `text` vacíos: un código 2FA de una app protegida no sale del móvil.
- No hay respaldo por adb: `dumpsys notification --noredact` daría todo el texto, también el de las apps protegidas.

### 3.6 `adbuddy screenshot [--scale 0.5] [--format png|jpeg] [-o fichero]`

La hace el host, nunca la app: `exec-out screencap -p` por el adb del host. Antes pasa por la guarda (§4.3).

- **Escala:** por defecto 0,5; `--scale 1` deja el tamaño original. El reescalado es un filtro de caja con la biblioteca estándar de Go (`image`, `image/png`, `image/jpeg`), sin dependencias nuevas.
- **Formato:** PNG por defecto, o JPEG con calidad 80.
- **Fichero:** `-o` relativo a la carpeta del agente. Por defecto, `screenshots/<AAAAMMDD-hhmmss>.png` dentro del entorno. Lo escribe la CLI, en el proceso del agente: el daemon le manda los bytes por el flujo de datos del IPC, en tramas de 1 MiB.
- Con el modo remoto activo, el valor por defecto pasa a 0,5 en JPEG (M5b, §3.3 de la v1).

## 4. Seguridad

### 4.1 Las mismas puertas que el shim

Una ayuda recorre en el daemon el mismo camino que `exec`:

| Puerta | `screen`, `foreground`, `notifications`, `screenshot` | `tap`, `type` |
|---|---|---|
| Nivel | 🟢 | 🟡 sin inverso, como `input` |
| En pausa | Bloqueada (`Blocked by ADBuddy: paused …`) | Bloqueada |
| Con una pregunta 🔴 pendiente | Pasa (es 🟢, no inyecta nada) | Bloqueada: un toque podría pulsar Permitir |
| Admisión en vuelo | No cuenta | Cuenta: retrasa la pregunta siguiente, como un `input` en marcha |
| Guarda de §4.3 | Sí | Sí |
| Diario | Una entrada con `argv` `["adbuddy", "<op>", …]`, `source` del agente y la descripción de la regla | Igual |

Las descripciones salen de reglas propias de las ayudas (`helper.screen`, `helper.tap`…), con título en inglés y en español como las demás. No pasan por el clasificador de shell: su `argv` no es una orden de adb.

### 4.2 ADBuddy no actúa sobre sí misma

- En la app: `dump` omite las ventanas de `tech.hybriq.adbuddy`. `click` sobre un nodo de ese paquete, o `--xy` sobre una de sus ventanas, da `protected`.
- **Con una pregunta 🔴 en el móvil,** la app también rechaza `click` sobre la interfaz del sistema (`com.android.systemui`): la notificación de la pregunta tiene el botón Permitir. El daemon ya bloquea los toques mientras hay pregunta (§4.1); esto es la segunda capa, por si un toque llegara por otra vía.
- En el host: la guarda (§4.3) trata a `tech.hybriq.adbuddy` como una app protegida más, y así también bloquea `input` con ADBuddy en primer plano.

### 4.3 Apps protegidas

**La lista.** Vive en la app, que es la fuente de verdad:

- **M5a:** al arrancar, la app la rellena por heurística con las apps que tienen icono en el lanzador. Para verlas usa `<queries>` con `MAIN`/`LAUNCHER`, sin `QUERY_ALL_PACKAGES`. Protege las que tienen en el paquete o en la etiqueta `bank`, `banco`, `pay`, `wallet`, `auth`, `password`, `bitwarden`, `1password`, `keepass`, `authenticator`, `2fa`, `otp`, `crypto` o `bizum`. La lista se marca como "propuesta".
- **M5c:** la pantalla del onboarding y la de Ajustes. El usuario confirma o edita la lista, y desde entonces la heurística ya no la toca.

La app manda la lista al host en `device.info` (campo nuevo `protectedApps`, §12), al conectar y cada vez que cambia. El host guarda la última en su carpeta de datos (`protected.json`), fuera del entorno del agente. Así sirve aunque la app se desconecte después.

**La guarda.** El daemon la aplica antes de admitir:

- las órdenes con un hallazgo de interfaz (`Finding.UI`: `input`, `am start` y `monkey`);
- las de leer la pantalla (`screencap` y `uiautomator dump`);
- las ayudas, salvo `foreground` y `notifications`.

Con eso:

1. Pide el primer plano, a la app (`foreground`) o, sin app, por adb: `dumpsys activity activities`, de donde lee `topResumedActivity` o `mResumedActivity`.
2. Si el paquete en primer plano está en la lista o es ADBuddy, bloquea: `Blocked by ADBuddy: <app> is a protected app; ask the user to do this part`. El código de salida es 126.
3. Con `am start -n <paquete>/…`, `am start -p <paquete>` o `monkey -p <paquete>` sobre un paquete protegido, bloquea igual. Un intent implícito (`am start -a VIEW -d https://…`) puede abrir una app protegida: entonces se bloquea lo siguiente que intente leer o tocar.
4. **Si no puede saber el primer plano** (la app no responde y `dumpsys` falla), bloquea: falla cerrada, tanto para tocar como para leer la pantalla (ruling del supervisor).
5. **Sin lista** (el host nunca ha recibido una), solo protege a ADBuddy. Lo dice una vez por sesión en la terminal de `serve`.

**Coste.** Cada orden guardada añade una consulta de primer plano: unos 120-165 ms con la app (el transporte medido en S1) y unos 300 ms por `dumpsys`. Solo la pagan las órdenes de interfaz y de captura.

**Límite honesto.** La guarda mira el primer plano un instante antes de ejecutar. Una app que pase a primer plano entre esa consulta y el toque (por ejemplo, abierta por una notificación) no se detecta. En la app, `click` vuelve a comprobar el paquete del nodo o de la ventana justo antes de actuar, y eso cierra el hueco para las ayudas. Para `input`, el hueco queda documentado.

### 4.4 Texto que viene del móvil

Todo lo que devuelve `dump` o `notifications` son datos de pantalla. La CLI lo escapa (§3.1). `AGENTS.md` ya dice que es contenido, no instrucciones (la regla 6). Su sección de órdenes gana las ayudas en M5a, y ese cambio va en `host/internal/workspace/templates/AGENTS.md`, que es de skimmer-32 en M6b: se le pasa el texto y lo integra él, o lo cambio yo avisando (§13).

## 5. Respaldo por adb (sin app o con el servicio parado)

El respaldo entra en dos casos: la app no está conectada, o responde `unavailable` porque el servicio de accesibilidad no está vinculado.

| Ayuda | Respaldo | Límite |
|---|---|---|
| `screen` | `exec-out uiautomator dump /dev/tty`, pasado al mismo formato con `encoding/xml` | Antes, el host comprueba con `dumpsys accessibility` que el servicio de ADBuddy **no** está vinculado: si lo está, `uiautomator dump` lo desconectaría (§6 de la v1, pendientes de M5). Entonces el error es `UI_UNAVAILABLE`, con el mensaje de los pendientes: `ADBuddy's accessibility service is running but the app is not connected: open the ADBuddy app and try again`. Sin contraseñas: de un nodo `password="true"` no se copia el texto |
| `tap --xy` | `shell input tap x y` | – |
| `tap` por texto o `--id` | `uiautomator dump`, y después `input tap` en el centro del elemento | Igual que `screen`, y el mismo orden de búsqueda |
| `type` | `shell input text <texto>` | Solo ASCII imprimible: los espacios se pasan como `%s` y el resto va entre comillas simples de mksh (paquete `shq`). Con otros caracteres, el error es `UI_UNAVAILABLE`, con el aviso "Unicode needs the ADBuddy app". Sin comprobar contraseñas, porque sin la app no se puede saber |
| `foreground` | `dumpsys activity activities` | – |
| `notifications` | Ninguno | `UI_UNAVAILABLE` |
| `screenshot` | Siempre por adb | – |

Las órdenes del respaldo las ejecuta el daemon con el adb del host, ya admitidas por las puertas de la ayuda. No llevan entrada propia en el diario: la entrada de la ayuda dice `"how": "adb"` en su campo `Reason`.

## 6. Superposición (§5.7 de la v1)

`AgentAccessibilityService` dibuja una ventana `TYPE_ACCESSIBILITY_OVERLAY`, que no necesita `SYSTEM_ALERT_WINDOW`:

- **Cuándo:** al empezar cualquier `ui.request` de acción (`click` y `setText`), y también con `dump`. Se oculta 4 s después de la última operación. Con la sesión en pausa no se muestra nunca.
- **Qué:** un marco fino del color de acento del tema y una píldora arriba: "Agent using the screen · Pause" / "Agente usando la pantalla · Pausar". Tocar la píldora envía `pause` (el tipo de M2) con `Bridge.send`.
- **Marca del toque:** un círculo de 48 dp que se desvanece en 600 ms, centrado en el punto o en el centro de los límites del nodo.
- **Toques:** toda la ventana es `FLAG_NOT_TOUCHABLE` salvo la píldora, que es una ventana aparte, pequeña y tocable. La superposición nunca tapa la interfaz: no intercepta el toque de `dispatchGesture` ni los del usuario.
- Sus ventanas son de ADBuddy, así que no salen en `dump` y no se pueden pulsar (§4.2).
- Colores del tema, claro y oscuro; textos en inglés y en español (`values/strings.xml`, `values-es/strings.xml`).

## 7. Modo remoto (M5b)

Precisa §3.3 de la v1 sobre el contrato de §8.2 del protocolo.

### 7.1 Llegar al móvil fuera de casa con depuración inalámbrica (Wi-Fi + Tailscale)

- **La app** manda `device.info` al conectar y cuando cambia la red (`ConnectivityManager.NetworkCallback`). Pone `tailscaleIp` si hay una red VPN con una IPv4 de `100.64.0.0/10`, la de CGNAT que usa Tailscale. `wifiAdbPort` va vacío salvo que pueda leerlo: no hay API pública para el puerto de la depuración inalámbrica (validación manual pendiente).
- **El host,** con el móvil conectado, lee el puerto él mismo (`getprop service.adb.tls.port`). Guarda `tailscale_ip` y `tailscale_port` en `config.toml`, como `address`. `findKnownPhone` prueba `tailscale_ip:tailscale_port` después de `address` y antes de esperar por mDNS. La comprobación de identidad (guid contra `ro.serialno`) no cambia.

### 7.2 Solo con datos móviles (`adb tcpip 5555`)

- **`remote.set {enabled: true}`** desde la app (con `rid`), y solo con el móvil conectado al host. El host:
  1. ejecuta `adb tcpip 5555` con su adb, fuera del shim (es el usuario quien lo pide, no el agente);
  2. guarda `remote_until` = ahora + 12 h en `config.toml`;
  3. responde con `remote.state {enabled, address: "<tailscaleIp>:5555", expiresAt}` y lo publica como evento.

  Sin `tailscaleIp` conocida, contesta `error` `unavailable` ("Turn on Tailscale on the phone first").
- **`remote.set {enabled: false}`,** al caducar `remote_until` o con `adbuddy remote off`: el host ejecuta `adb usb` y borra `remote_until`.
- **Invariante** (`invariant.go`): con `remote_until` en el futuro, el puerto abierto es el esperado y `applyInvariant` no ejecuta `adb usb`. Sin `remote_until`, o ya pasado, se comporta como hoy.
- **Reconexión:** si se pierde el móvil durante la sesión, `serve` prueba, en orden, el serial de siempre (mDNS), `tailscale_ip:tailscale_port` y, con el modo remoto activo, `tailscale_ip:5555`. Como el serial con el que adb lista el móvil cambia, el ejecutor del daemon tiene que tomar el serial activo de un sitio que se pueda cambiar, no de `Options.Serial`, fijado al arrancar. Eso toca `serve.go` y `daemon.go`, que son de M3: el plan de M5b se escribe sobre el `serve` de M3 y se acuerda con tern-51.
- **App:** interruptor en Ajustes con el aviso de §3.3 de la v1 (el puerto 5555 queda abierto en redes públicas) y la caducidad visible. Lo pinta `RemoteReducer`, que pasa a guardar `remote.state` en `BridgeState`.
- **`adbuddy remote [on|off|status]`** en el PC, para el usuario: desde la sesión del agente se rechaza (`AGENT_SESSION`, como `serve`).

## 8. Errores de la CLI

| Código | Salida | Cuándo |
|---|---|---|
| `DAEMON_NOT_RUNNING` | 75 | `adbuddy serve` no está en marcha |
| `UI_BLOCKED` | 126 | Pausa, pregunta pendiente o guarda. El mensaje empieza por `Blocked by ADBuddy:`, como en el shim |
| `UI_NOT_FOUND` | 1 | `not_found` |
| `UI_AMBIGUOUS` | 1 | `ambiguous`, con las coincidencias |
| `UI_PROTECTED` | 126 | `protected` de la app (contraseña, bloqueado, ADBuddy, app protegida) |
| `UI_UNAVAILABLE` | 75 | Sin app, sin servicio y sin respaldo posible |
| `UI_TIMEOUT` | 75 | Sin respuesta a tiempo (§2) |

## 9. Ficheros

**M5a, host** (rutas de nautilus-19 en `COORDINACION.md`):

- `cli/helpers.go` y `cli/helpers_screen.go`: subcomandos, salida y errores;
- `cli/phone_ui.go`: `uiArea`, la espera de respuestas por `id` y el manejador de `device.info` (solo `protectedApps` en M5a);
- `daemon/ui.go` y `daemon/guard.go`: el kind `ui`, las puertas, el respaldo y la guarda;
- `uidump/`, paquete nuevo: el formato común de elementos, el parser de `uiautomator dump` y el de `dumpsys activity`;
- `bridge/ui_ops.go`: los tipos de `args` y `result`;
- `ipc`: la constante `KindUI`;
- `screenshot/`, paquete nuevo: el reescalado.

**M5a, compartidos de una línea**, al final del plan y avisando:

- `daemon.go`: campos `UI` y `Foreground` en `Options`, un `case` en `Handle` y la llamada a la guarda en `exec`;
- `serve.go`: pasar `link.UI` y `link.Foreground` al daemon;
- `root.go`: registrar los subcomandos.

**M5a, app:**

- `a11y/`: el servicio, `UiExecutor`, `Dumper`, `Matcher`, `Overlay` y `ProtectedApps`;
- `bridge/ProtocolUiOps.kt`;
- `res/xml/accessibility_service.xml`: capacidades y flags;
- strings EN/ES;
- `AndroidManifest.xml`: `<queries>`.

**M5b:** `cli/remote.go`, `connect.go`, `invariant.go`, `config` (campos nuevos), `RemoteReducer.kt` y la pantalla del interruptor. Más el enganche en `serve.go` y `daemon.go`, acordado con tern-51.

**M5c:** `a11y/AgentNotificationListener.kt` (o `notify/`), las pantallas y el manifest.

## 10. Lo que el servicio de accesibilidad declara

```xml
android:accessibilityEventTypes="typeWindowStateChanged|typeWindowsChanged"
android:accessibilityFlags="flagDefault|flagRetrieveInteractiveWindows|flagReportViewIds"
android:canRetrieveWindowContent="true"
android:canPerformGestures="true"
android:notificationTimeout="100"
```

Sin `canTakeScreenshot`: la captura la hace el host (§3.6). La descripción del servicio (M2) ya promete que nunca lee contraseñas ni actúa en apps protegidas, y §3.1 y §4.3 lo cumplen.

## 11. Tests

- **Host:**
  - el daemon con un `UI` falso: cada puerta de §4.1, el orden pausa → pregunta → guarda → admisión, y una entrada del diario por ayuda;
  - la guarda: con una app protegida, ADBuddy, primer plano desconocido o sin lista, para `input`, `am start -n`, `monkey -p`, `screencap` y las ayudas;
  - el respaldo con el adb falso de `testutil`: `uiautomator` con el servicio vinculado y sin vincular, `input text` con espacios, comillas y Unicode, y `dumpsys` de Android 11 a 17 (muestras reales en `testdata`);
  - `uiArea` con una app falsa de `bridge`: respuesta tardía, respuesta de otra conexión, `id` desconocido y caducidad;
  - la CLI: la salida de texto escapada, `--json` y los códigos de salida;
  - el reescalado: tamaño y formato.
- **App** (JVM, sin Android):
  - `Dumper`, `Matcher` y `ProtectedApps` sobre un modelo propio de nodo (`UiNode`), que el servicio construye a partir de `AccessibilityNodeInfo`, para que la lógica no dependa de clases de Android;
  - los casos: contraseña, bloqueado, ventanas de ADBuddy, la interfaz del sistema con pregunta pendiente, ambigüedad, recorte de límites, topes y el `id` repetido o caducado.
- **Fixtures** (`protocol/fixtures/`): las de §12, con ida y vuelta estricta a los dos lados (spec del protocolo, §12).
- **Emulador:** el servicio real con `connectedDebugAndroidTest` cuando M2.5b resuelva la imagen (pendientes, M2.5b). Hasta entonces, validaciones manuales.
- **Validaciones manuales pendientes** (en el plan): la latencia de `screen` en el Pixel; `type` con emoji; la superposición en claro y oscuro; la guarda con una app de banca real; la lectura del puerto de la depuración inalámbrica desde la app (M5b); `tcpip 5555` y la reconexión por Tailscale solo con datos (M5b).

## 12. Cambios al protocolo (necesitan el ok de adbuddy-e5)

Todos aditivos (§11 del protocolo: sigue siendo `v1`):

1. **`device.info.protectedApps`:** lista de paquetes, opcional.
   - Go: `ProtectedApps []string \`json:"protectedApps,omitempty"\``.
   - Kotlin: `val protectedApps: List<String> = emptyList()`.
   - Fixtures: se amplía `app/device.info.json`.
2. **El código de error `ambiguous`,** para `ui.response`.
   - Go: `CodeAmbiguous`; Kotlin: la constante equivalente.
   - Fixture: `app/ui.response@ambiguous.json`.
3. **Las fixtures de las operaciones:**
   - `host/ui.request@settext.json`, `@clickid.json`, `@clickxy.json` y `@dumpall.json`;
   - `app/ui.response@dump.json`, `@foreground.json`, `@settext.json` y `@notifications.json`.

   Con sus tests a los dos lados: las estructuras de `args` y `result` de §3 se decodifican en modo estricto.

Van en un solo commit (Go, Kotlin y fixtures), como pide §10 del protocolo, y se avisa a tern-51 y a skimmer-32 al integrar.

## 13. Decisiones

- **Ruling: las ayudas pasan por el daemon con un kind IPC propio, no por el shim de adb.** **Por qué:** sus puertas (pausa, pregunta pendiente, admisión en vuelo) son privadas del daemon, y `adbuddy tap --text` no es una orden de adb que el clasificador pueda leer. **Coste si me equivoco:** un fichero nuevo en `daemon` y una línea en `Handle`.
- **Ruling: `tap` y `type` son 🟡 sin inverso, como `input`.** **Por qué:** hacen lo mismo que `input`, y pedir confirmación en cada toque haría inservible `home-screen`. Las protecciones están en la guarda y en las puertas. **Coste si me equivoco:** subirlos a 🔴 es cambiar una regla.
- **Ruling: la lista de apps protegidas vive en la app y el host guarda una copia.** **Por qué:** la app conoce las etiquetas y tiene la pantalla para editarla; la copia del host la mantiene aunque la app se desconecte. **Coste si me equivoco:** un campo en `device.info` (aditivo).
- **Ruling (adbuddy-e5): la guarda va en `exec` después de las puertas de pausa, pregunta 🔴 y admisión, y falla cerrada.** Si no se sabe qué app está en primer plano, no se ejecuta nada de lo que guarda, ni tocar ni leer. **Por qué:** seguridad; sin saber el primer plano no hay forma de saber si es una app protegida. **Coste si me equivoco:** algún bloqueo de más con `dumpsys` caído, que se afina después.
- **Ruling (adbuddy-e5): el protocolo de M5 va en una rama corta `m5-proto`, que sale de `main` y entra pronto.** **Por qué:** M3 rebasa sobre el protocolo final y nadie choca en `protocol/fixtures`. **Coste si me equivoco:** una rama más.
- **Ruling: la heurística protege por defecto ya en M5a, antes de que exista la pantalla.** **Por qué:** sin pantalla, la alternativa sería no proteger nada. **Coste si me equivoco:** una app mal detectada queda protegida hasta M5c; el usuario la usa él mismo.
- **Ruling: la captura la hace siempre el host.** **Por qué:** `screencap` ya funciona, `takeScreenshot` de accesibilidad tiene un límite de frecuencia y no ahorra transporte, y así la captura no depende de la app. **Coste si me equivoco:** la captura y el árbol no salen del mismo instante (ARTEMIS lo consigue en API 30+); se puede añadir como operación más adelante.
- **Ruling: en la app, `dump` y `click` sobre un modelo propio de nodo.** **Por qué:** tests en la JVM sin emulador, que hoy no sirve para la app (pendientes, M2.5b). **Coste si me equivoco:** una copia del árbol por operación; con el tope de 2000 nodos es despreciable.
- **Ruling: M5 en tres sub-hitos, empezando por M5a.** **Por qué:** M5a solo necesita `m3-bridge`; el modo remoto toca `serve.go` y `daemon.go` (de M3) y las pantallas necesitan la base de diseño. **Coste si me equivoco:** tres planes en vez de uno.
- **Ruling: el texto de las ayudas en `AGENTS.md` lo integra el dueño de la plantilla.** **Por qué:** `workspace/templates` es de skimmer-32 en M6b (`COORDINACION.md`). **Coste si me equivoco:** si M6b se retrasa, el agente no conoce las ayudas aunque existan; entonces lo cambio yo, avisando al supervisor.
