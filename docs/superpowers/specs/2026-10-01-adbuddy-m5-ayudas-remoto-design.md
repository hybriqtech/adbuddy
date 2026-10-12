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

- **CLI** (`host/internal/cli/helpers*.go`, nuevos): cada ayuda es un subcomando. Manda un `ipc.Request{Kind: "ui", UIOp, UIArgs, Argv}` al daemon (`Argv` es la orden tal como la escribió el agente, para el diario) y escribe la respuesta: texto compacto o `--json` con el sobre habitual de la CLI. Sin `adbuddy serve` en marcha, sale con `DAEMON_NOT_RUNNING` (salida 1, como `undo`): las ayudas, como el shim, solo existen con el host.
- **Daemon** (`host/internal/daemon/ui.go` y `guard.go`, nuevos): el kind `ui` pasa por las mismas puertas que una orden del shim (§4.1). Después ejecuta con `Options.UI` y, si hace falta, con el respaldo. Cada ayuda deja su entrada en el diario.
- **Enlace con la app** (`host/internal/cli/phone_ui.go`): `uiArea` envía `ui.request` con `Server.Send`, o con el envío atado a la conexión que traiga M3 (`SendTo`). Espera el `ui.response` de su `id` hasta `expiresAt` y descarta las respuestas de ids que no espera. Un `ui.response` solo vale de la conexión a la que se envió la petición (`Server.Current(m)`, §10 del protocolo).
- **App** (`android/.../a11y/`): `AgentAccessibilityService` guarda una referencia a sí mismo mientras está vinculado. `UiExecutor` escucha `ui.request` con `Bridge.listen` y ejecuta las operaciones de una en una, en un hilo propio, nunca en el principal. Responde con `Bridge.send(ui.response)`.

**Plazos.** El host pone `expiresAt` = ahora + 10 s, y la app descarta lo caducado (con `HostClock`, como en M2). Si en ese plazo no llega respuesta, el daemon da la operación por fallida. Para `dump`, `foreground` y `notifications`, que solo leen, prueba entonces el respaldo. Para `click` y `setText` no lo prueba: un toque que pudo llegar a ejecutarse no se repite por adb. El agente recibe `UI_TIMEOUT` y la entrada del diario queda en `error` (incierta), como una orden de adb cortada.

**Una operación a la vez.** El host serializa las operaciones de acción (`click` y `setText`) con la admisión en vuelo del daemon, como cualquier orden 🟡. La app las ejecuta en orden y no ejecuta dos veces el mismo `id`: guarda los últimos 64.

## 3. Operaciones

M3.0 dejó `args` y `result` como JSON libre (`json.RawMessage` y `JsonElement`). M5 los define aquí, con estructuras propias en ficheros nuevos (`bridge/ui_ops.go`, `ProtocolUiOps.kt`) y una fixture por operación (§12). `UIRequest` y `UIResponse` no cambian.

### 3.1 `dump` → `adbuddy screen [--all] [--settle] [--json]`

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
     "enabled": true, "password": false, "window": "app"}
  ]
}
```

- **Qué ventanas:** todas las interactivas (`flagRetrieveInteractiveWindows`), ordenadas por capa de mayor a menor: la app, sus diálogos, el teclado y la interfaz del sistema. Es la idea de `HierarchyDumper` de ARTEMIS (Apache-2.0; la atribución va en `NOTICE` y en la cabecera del fichero si se copia código). Las ventanas de `tech.hybriq.adbuddy`, incluida la superposición, no salen nunca.
- **Qué nodos:** los visibles para el usuario, con los límites recortados a la pantalla, a su ventana y a sus antepasados desplazables (paridad con UIAutomator). Entre ellos, solo los informativos (texto, descripción o pista) o los que se pueden usar (pulsar, editar, desplazar o marcar). Las capas sin nada de eso desaparecen. `--all` (`args: {"all": true}`) incluye todos los nodos visibles.
- **Topes:** 2000 elementos, profundidad 75. Más que eso pone `truncated: true`. Si el JSON del resultado pasa de 512 KiB (bytes UTF-8 ya codificados, `MaxUIResult`), se recortan los textos largos a 200 caracteres y, si aún no cabe, los últimos elementos.
- **`unreadable`:** `true` cuando alguna ventana de aplicación o diálogo no entrega contenido (o no hay ventanas); el host lo dice al agente con una nota que no promete la causa. Ausente si no (Go `omitempty`).
- **Contraseñas:** de un nodo con `isPassword` sale `password: true`, y `text` y `hint` salen vacíos.
- **Bloqueado:** con el móvil bloqueado (`KeyguardManager.isKeyguardLocked`), `locked: true` y `elements` vacío. El agente no debe intentar desbloquear (la regla 14 de `AGENTS.md`).
- **App protegida en primer plano:** `error` `protected` (§4.3).
- **`id`:** el `viewIdResourceName` tal cual (`paquete:id/nombre`), también en la salida de texto: `tap --id` lo compara entero.
- **`checked`:** ausente si el elemento no se puede marcar (Go `omitempty`, Kotlin `null` con `explicitNulls=false`).
- **`window`:** `app`, `dialog`, `ime`, `system` o `split`.

**Salida de texto de `adbuddy screen`** (una línea por elemento, pensada para el agente):

```
com.android.settings · 1344×2992
[android:id/title] "Network & internet" TextView (42,610)-(980,680) click
[com.android.settings:id/search_action_bar] desc="Search settings" Toolbar (42,180)-(1302,300) click
- "Wi-Fi" Switch (980,700)-(1302,780) click checked=false
```

`--settle` (latencia, 2026-10-03; pantalla de partida, 2026-10-04) pide al daemon que espere a que la pantalla se asiente antes de devolverla, cada lectura con la guarda: sirve tras una orden `adb` en bruto (`am start`, `input`). La primera lectura es la **partida** y no se devuelve, salvo si se llega al tope (`settleMax` o `settleReads`) sin más lecturas: entonces sale ella, con `settled` falso (pasó la guarda antes de leerse). La pantalla se da por asentada cuando ha **cambiado** respecto a la partida y sigue igual al menos `settleQuiet` (600 ms); si nunca cambia, a los `settleIdle` (2 s): `screen --settle` sobre una pantalla quieta tarda unos 2 s (hasta settle2, 2026-10-04, unos 0,4 s); como mucho `settleMax` (3 s) más lo que tarde una lectura que ya empezó. Una lectura que llega por otra vía que la partida (`uiautomator` porque la app dejó de contestar) no se compara con ella, porque los formatos difieren: no cuenta como cambio (por ella solo `settleIdle` cierra la espera) y, si sale, `changed` va ausente. Para comparar no cuenta qué elemento tiene el foco, y no tener elementos es lo mismo que una lista vacía; un interruptor que cambia (`checked`) sí cuenta. Si la guarda, o la propia app con `UI_PROTECTED`, bloquea una lectura, se reintenta tras `settleGap` hasta `settleBlockWait` (2 s) desde el inicio: durante una transición la app enseña un momento una ventana sin contenido y la da por protegida. Cada intento pasa la guarda completa y solo sale una lectura hecha después de pasarla; si el último intento fue un bloqueo, sale el bloqueo y nada leído antes. La guarda propia de `screen` también reintenta: si bloquea, o si la app contesta `UI_PROTECTED` a la primera lectura, se entra en la espera sin lectura y la primera lectura válida es la partida; un bloqueo que dura hasta `settleBlockWait` sale como bloqueo, sin datos. La pausa y una pregunta 🔴 pendiente no se reintentan. La respuesta añade `settled` y `changed`, que habla de la lectura que sale (`false`: es igual a la partida, porque no cambió o porque cambió y volvió; ausente: no se sabe); si `settled` es `false`, la CLI avisa de que la pantalla seguía cambiando o no enseñaba contenido; si la pantalla de partida tenía alguna ventana de aplicación (`app` o `dialog`), una lectura de la app sin ninguna (solo la barra de estado, o nada: una transición cuya ventana aún no tiene contenido) con el móvil desbloqueado nunca se da por asentada, aunque lleve quieta más de `settleQuiet`, y sale con `settled` falso si llega el tope (consecuencia: navegar desde una pantalla normal a una sin contenido accesible, como un juego o un visor de cámara, tarda los 3 s una vez; si la partida ya no tenía ventana de aplicación, o se desconoce, esa lectura se asienta como antes, sin penalización en cada toque); un `changed` falso en `screen --settle` no se avisa (es una pantalla quieta). Sin la bandera, la salida es la de siempre.

`[id]` sale si hay `id`, y si no, un `-`. Los textos van entre comillas, recortados a 120 caracteres y escapados con `classify.Printable`: un texto de la pantalla no puede meter secuencias de control en la terminal del agente.

### 3.2 `click` → `adbuddy tap "<texto>" | --id <id> | --xy x,y [--screen]`

`args`: `{"text": "..."}`, `{"id": "..."}` o `{"x": 10, "y": 20}`, solo uno.

`result`: `{"clicked": true, "bounds": [l, t, r, b], "how": "action" | "gesture"}`.

- **Por `id` o por texto:** se busca entre los nodos que `dump` mostraría, por este orden: `id` exacto; texto o descripción exactos; y los mismos sin distinguir mayúsculas. Del nodo que coincide se sube al antepasado pulsable más cercano, y se pulsa con `ACTION_CLICK` (`how: action`). Si coinciden varios pulsables distintos, el error es `ambiguous` (§12), con el número de coincidencias y los límites de las 5 primeras en `text`. El agente repite entonces con `--id` o `--xy`. Si no coincide ninguno, `not_found`.
- **Por coordenadas:** un toque con `dispatchGesture` (`how: gesture`), 50 ms. Antes de tocar se mira qué ventana tiene encima ese punto. Si es de ADBuddy o de una app protegida, el error es `protected`.
- La superposición marca el punto (§6).
- **`--screen`:** antes del toque, tras pasar la guarda y la pausa, el daemon lee una vez la pantalla de **partida**, solo desde la app y con un tope de `baselineWait` (1 s; sin app no cae a `uiautomator`, que tarda más). Esa lectura no se devuelve, no se escribe y no se registra: solo se compara. No debilita las puertas del toque: tras ella se miran otra vez el contexto y la pausa, y la guarda si pasaron más de `baselineSlack` (150 ms) desde que dijo sí; si alguna dice no, el toque no ocurre. Si la app contesta `UI_PROTECTED` a esa lectura, el toque tampoco ocurre (bloqueo, sin riesgo de acción doble). Con cualquier otro fallo (sin app, sin respuesta a tiempo) el toque sigue sin partida: entonces solo `settleIdle` cierra la espera y no se sabe si la pantalla cambió. Por esa lectura, el total de `tap --screen` puede ser hasta `baselineWait` (1 s) más unos 3 s de la espera. Tras el toque, el daemon lee la pantalla asentada (como `screen --settle`, desde esa partida) y la CLI la imprime a continuación del `Tapped …`; así el agente no necesita otra llamada. Si se asentó igual que la partida (sin cambiar, o cambió y volvió), la respuesta trae `changed: false` y la CLI añade `(the screen did not change in about 2 s after the action: run adbuddy screen before trying again)`; con `changed` ausente (partida desconocida, daemon antiguo) no dice nada. La entrada de diario de la lectura dice `settled=` y `changed=` (`true`, `false` o `unknown`). Si la guarda impide leerla (el toque abrió una app protegida, por ejemplo; tras reintentarlo hasta `settleBlockWait`), la respuesta trae `screenNote` en vez de `screen` (también si hay pausa, una pregunta 🔴 pendiente, un fallo de la app o «stopped», no solo por la guarda) y la CLI la imprime y sale con 0: el toque sí ocurrió y un código de error haría reintentarlo. Deja **dos entradas** en el diario: la del toque y una `adbuddy screen` de la lectura.

### 3.3 `setText` → `adbuddy type "<texto>" [--screen]`

`args`: `{"text": "..."}`, hasta 4096 caracteres.

`result`: `{"set": true}`.

- Va al nodo con el foco de entrada (`findFocus(FOCUS_INPUT)`), con `ACTION_SET_TEXT`. Reemplaza el texto del campo: así es la acción de Android. Para añadir, el agente lee el campo con `screen` y manda el texto completo. `--screen` funciona igual que en `tap` (§3.2).
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
- **Qué sale (M5c).** De ADBuddy, de las apps protegidas (la lista se consulta en cada llamada), de una notificación que la app marca `VISIBILITY_SECRET` y de un paquete que no es un nombre de paquete (su `package` sale vacío) salen `key`, `title` y `text` vacíos: la `key` lleva el tag de la app, que puede llevar texto, y se sustituye por `hidden-<n>`. Lo demás, con el texto cortado a 1024 puntos de código, las 50 más recientes (`truncated`) y el resultado dentro de `MaxUIResult` (se descartan las más antiguas hasta que quepa).
- **Dos capas.** El daemon repite el vaciado con su copia de `protected.json`, el recorte a 50 y 1024 y el paquete inválido a vacío: una lista desfasada o una app que no sea la verdadera no saca un código. No pasa por la guarda de primer plano (no lee la pantalla), sí por pausa, pregunta 🔴 y diario; el diario guarda la orden, nunca el resultado. Un tiempo agotado es `UI_TIMEOUT`, sin reintento.
- **El listener solo lee** (no descarta, pospone ni pulsa nada, no abre intents ni registra texto, y nada fuera de su paquete lo alcanza; `ListenerIsReadOnlyTest`). El acceso lo concede el usuario en los ajustes del sistema; la app lo muestra con tres valores (permitido, no permitido, desconocido) y el desconocido nunca se pinta como ninguno de los otros.
- **Confirmación del usuario.** Que el sistema diga "permitido" no basta para leer: la app exige una marca que solo pone el botón de confirmar de su propia pantalla de acceso (nunca algo automático) y que revocar el acceso invalida (el propio listener la borra al desconectarse y en cada nuevo enlace, sin esperar a que nadie la mire: el sistema puede volver a enlazarlo sin desconectarlo antes, M5c-32 b); sin ella, `unavailable` con "confírmalo en la app ADBuddy". El agente podría activar el interruptor del sistema con `adbuddy tap`, pero no pulsar el botón de ADBuddy (app protegida).
- **Secretas.** Salen vacías las notificaciones que la app marca `VISIBILITY_SECRET`, las de otro usuario o del perfil de trabajo, las que el usuario o el canal ocultan de la pantalla de bloqueo y las que no se pueden leer.
- **Límite honesto.** Una app que reenvía las notificaciones de otras con su propio paquete (acompañantes de reloj, historiales de notificaciones, KDE Connect) saca el contenido de una app protegida: el paquete que sale es el del reenviador, y no es de la lista. Se documenta; no se puede cerrar desde aquí.

### 3.6 `adbuddy screenshot [--scale 0.5] [--format png|jpeg] [-o fichero]`

La hace el host, nunca la app: `exec-out screencap -p` por el adb del host. Antes pasa por la guarda (§4.3) y pide el aviso de la superposición a la app (§6, «Cuándo»). Entre el aviso y `screencap` pide `hideNotice`; si no puede retirarlo, la captura sale con el aviso dentro y `ScreenshotResult.noticeInImage` lo dice.

- **Escala:** por defecto 0,5; `--scale 1` deja el tamaño original. El reescalado es un filtro de caja con la biblioteca estándar de Go (`image`, `image/png`, `image/jpeg`), sin dependencias nuevas.
- **Formato:** PNG por defecto, o JPEG con calidad 80.
- **Fichero:** `-o` relativo a la carpeta del agente. Por defecto, `screenshots/<AAAAMMDD-hhmmss>.png` dentro del entorno. Lo escribe la CLI, en el proceso del agente: el daemon le manda los bytes por el flujo de datos del IPC, en tramas de 1 MiB.
- Con el modo remoto activo, el valor por defecto pasa a 0,5 en JPEG (M5b, §3.3 de la v1; lo decide el daemon, §13).

### 3.7 `notice` (sin comando de CLI)

Pide a la app que muestre la superposición (§6) sin hacer nada más. La usa el host antes de `screenshot` y de las lecturas crudas por adb (§6, «Cuándo»). `args` y `result` son `{}` (`bridge.Empty`).

- Respuestas: `ok`; `failed` si la app no puede dibujar la superposición; `protected` con la sesión en pausa; `unavailable` con el servicio apagado; `unsupported` en una app anterior.
- El host solo deja leer sin aviso con `ErrNoApp`, `unavailable` (con el servicio no vinculado) y `unsupported`; cualquier otra respuesta bloquea.
- No lleva guarda de app protegida: no lee nada. El nombre es el mismo que el del tipo de mensaje `notice`, pero son espacios de nombres distintos (op de `ui.request`).

### 3.8 `hideNotice` (sin comando de CLI)

Pide a la app que retire la superposición (marco y píldora) un instante, para que `adbuddy screenshot` no la lleve en la imagen. `args` `{ms: 1..5000}` (`MaxHideNoticeMs`), `result` `{}`. La app quita las vistas, espera 3 fotogramas y se restaura sola a los `ms` salvo pausa (nunca la repone en pausa; `show()` y `remove()` cancelan el temporizador).

- Respuestas: `ok`; `invalid` si `ms` está fuera de 1..5000; `protected` con la sesión en pausa; `failed` si no puede retirarla.
- No es un helper del agente: solo la pide el host dentro de `screenshot`.

## 4. Seguridad

### 4.1 Las mismas puertas que el shim

Una ayuda recorre en el daemon el mismo camino que `exec`:

| Puerta | `screen`, `foreground`, `notifications`, `screenshot` | `tap`, `type` |
|---|---|---|
| Nivel | 🟢 | 🟡 sin inverso, como `input` |
| En pausa | Bloqueada (`Blocked by ADBuddy: paused …`) | Bloqueada |
| Con una pregunta 🔴 pendiente | Bloqueada, como toda orden de adb (`admit` del daemon) | Bloqueada: un toque podría pulsar Permitir |
| Admisión en vuelo | No cuenta | Cuenta: retrasa la pregunta siguiente, como un `input` en marcha |
| Guarda de §4.3 | Sí | Sí |
| Diario | Una entrada con `argv` `["adbuddy", "<op>", …]`, `source` del agente y la descripción de la regla | Igual; con `--screen`/`--settle` son dos: la de la acción y una `adbuddy screen` de la lectura, que puede quedar `blocked` mientras la acción queda `ok`. No se escribe el contenido de la pantalla, solo el estado |

El `argv` del diario lo construye el daemon a partir de la operación y sus `args` (el del cliente no cuenta), con el texto de `type` recortado a 60 caracteres. Las descripciones salen de reglas propias de las ayudas (`helper.screen`, `helper.tap`…), con título en inglés y en español como las demás. No pasan por el clasificador de shell: su `argv` no es una orden de adb.

### 4.2 ADBuddy no actúa sobre sí misma

- En la app: `dump` omite las ventanas de `tech.hybriq.adbuddy`. `click` sobre un nodo de ese paquete, o `--xy` sobre una de sus ventanas, da `protected`.
- **Con una pregunta 🔴 en el móvil,** la app también rechaza `click` sobre la interfaz del sistema (`com.android.systemui`): la notificación de la pregunta tiene el botón Permitir. El daemon ya bloquea los toques mientras hay pregunta (§4.1); esto es la segunda capa, por si un toque llegara por otra vía.
- En el host: la guarda (§4.3) trata a `tech.hybriq.adbuddy` como una app protegida más, y así también bloquea `input` con ADBuddy en primer plano. **Excepción (V44, ruling A, 2026-10-03):** con ADBuddy arriba pasan dos salidas que no pulsan ni leen nada suyo: `input keyevent 3` o `input keyevent KEYCODE_HOME`, exactos y solos; y, si el primer plano lo dice la app, `monkey -p <paquete> -c android.intent.category.LAUNCHER 1` exacto hacia un paquete que no es ADBuddy ni protegido. Todo lo demás sigue bloqueado, también `am start` en cualquier forma (con `-n` se elige un diálogo que dejaría ADBuddy a la vista), y el motivo le dice al agente cómo salir (plan `docs/superpowers/plans/2026-10-02-adbuddy-v44-v45.md`, ruling 2).

### 4.3 Apps protegidas

**La lista.** Vive en la app, que es la fuente de verdad:

- **M5a:** al arrancar, la app la rellena por heurística con las apps que tienen icono en el lanzador. Para verlas usa `<queries>` con `MAIN`/`LAUNCHER`, sin `QUERY_ALL_PACKAGES`. Protege las que tienen en el paquete o en la etiqueta `bank`, `banco`, `pay`, `wallet`, `auth`, `password`, `bitwarden`, `1password`, `keepass`, `authenticator`, `2fa`, `otp`, `crypto` o `bizum`. La lista se marca como "propuesta".
- **M5c:** la pantalla de apps protegidas, en el onboarding (paso 1 de 2) y en Ajustes. El usuario confirma o edita la lista, y desde entonces la heurística ya no la toca (confirmar la propuesta sin cambios también cuenta, y una lista vacía confirmada se manda como `[]`). La propuesta incluye desde el primer arranque (no solo en la pantalla), además de la heurística, las apps que reciben los códigos de las demás: Mensajes de Google y de Samsung, Mensajería de AOSP, Gmail y la app de SMS predeterminada, si están instaladas, y Google Play services; así llegan al host en `device.info` y protegen `screen` y `tap` sobre ellas mientras no haya confirmación. La pantalla abre con esa propuesta marcada. ADBuddy siempre está protegida y no sale en la lista.

La app manda la lista al host en `device.info` (campo nuevo `protectedApps`, §12), al conectar y cada vez que cambia. El host guarda la última en su carpeta de datos (`protected.json`), fuera del entorno del agente. Así sirve aunque la app se desconecte después.

**La guarda.** El daemon la aplica después de las puertas (pausa, pregunta 🔴, admisión) y justo antes de ejecutar; si llega una pausa mientras mira, la orden se bloquea igualmente. La aplica a:

- las órdenes con un hallazgo de interfaz (`Finding.UI`: `input`, `am start` y `monkey`) y `cmd input`;
- las de leer la pantalla (`screencap` y `uiautomator dump`) y `dumpsys activity top` (y cualquier `dumpsys activity` con argumento fuera de una lista corta: `activities`, `a`, `services`, `providers`, `broadcasts`, `processes`), porque vuelca el árbol de vistas con su texto;
- las ayudas, salvo `foreground` y `notifications`.

Con eso:

0. **Una orden guardada va sola** (revisión del plan, C1): un único hallazgo, sin `;`, `&`, `|`, comillas invertidas, `$(` ni saltos de línea en su texto, y `input keyevent`/`keycombination` con una sola tecla. Si no, se bloquea ("must run alone"). Así una orden no puede abrir una app protegida y tocarla después de que la guarda haya mirado. Además va sin envolturas (`watch`, `flock`, `sh -c`, `env`, `nice`, `timeout`, `nohup`, `busybox`, `VAR=x`…): la primera palabra del texto de shell es la propia orden guardada, porque el clasificador desenvuelve `watch` y `flock` y un bucle con `watch` actuaría después de que la guarda mirase. Los bucles con `;` o salto de línea quedan fuera por lo anterior.
1. Pide el primer plano, a la app (`foreground`) o, sin app, por adb: `dumpsys activity activities`, de donde lee **todas** las actividades reanudadas (`topResumedActivity`, `mResumedActivity` y `ResumedActivity`): con pantalla dividida, PiP o una segunda pantalla hay varias, y cuenta cualquiera. La app, por su parte, marca `protected` si cualquier ventana de aplicación en pantalla está protegida o no deja ver su contenido.
2. Si el paquete en primer plano está en la lista o es ADBuddy, bloquea: `Blocked by ADBuddy: <app> is a protected app; ask the user to do this part`. El código de salida es 126.
3. Con `am start -n <paquete>/…`, `am start -p <paquete>` o `monkey -p <paquete>` sobre un paquete protegido, bloquea igual. Un intent implícito (`am start -a VIEW -d https://…`) puede abrir una app protegida: entonces se bloquea lo siguiente que intente leer o tocar.
4. **Si no puede saber el primer plano** (la app no responde y `dumpsys` falla o no se puede leer), bloquea: falla cerrada, tanto para tocar como para leer la pantalla (ruling del supervisor). Sin actividad reanudada, solo cuenta como "nada delante" si `dumpsys power` dice `mWakefulness=Asleep` o `Dozing` (pantalla apagada): así el agente puede encender la pantalla (`input keyevent 224`). Con la pantalla encendida y sin actividad reanudada (la pantalla de bloqueo, o un `dumpsys` que no se entiende) bloquea, y eso también evita capturar las notificaciones de la pantalla de bloqueo.
5. **Sin lista** (el host nunca ha recibido una), solo protege a ADBuddy. Lo dice una vez por sesión en la terminal de `serve`.
6. **Persiana.** Con la persiana de notificaciones o los ajustes rápidos desplegados (`dumpsys window`: la ventana con foco es `NotificationShade`), la guarda bloquea como con una app protegida, y falla cerrada si no entiende `dumpsys window`. Se exceptúan `input keyevent` con `3`, `4`, `KEYCODE_HOME` o `KEYCODE_BACK`, que sirven para cerrarla. Las notificaciones emergentes (heads-up) cuentan igual: son una ventana de `systemui` sobre cualquier app y no tienen el foco, así que `mCurrentFocus` no las ve. La app las señala 8 s: el servicio de accesibilidad escucha `typeNotificationStateChanged` y, durante 8 s tras cualquier notificación que no sea de ADBuddy, `foreground` y `dump` dan `protected` (ruling 7 del plan de M5b). Sin app, el host las busca en `dumpsys window windows` (ventanas visibles de `com.android.systemui` que no estén en una lista corta de las fijas: barra de estado, navegación y decoración de pantalla) y, si no lo entiende, falla cerrado. Así un código 2FA de una app protegida no sale en `screen` ni en `screenshot`. Por lo mismo, `screenrecord` es 🔴 especial (regla `screen.record`: graba mucho después de que la guarda mirase), `dumpsys notification --noredact` es 🔴 (da el texto de todas las notificaciones), `adb bugreport` es 🔴 (vuelca notificaciones y pantalla) y `dumpsys window` queda 🟢 (solo títulos de ventana, sin contenido de pantalla).

**Coste.** Cada orden guardada añade una consulta de primer plano: unos 120-165 ms con la app (el transporte medido en S1) y unos 300 ms por `dumpsys`. Solo la pagan las órdenes de interfaz y de captura. Cada orden guardada suma además un `dumpsys window` (la persiana y las notificaciones emergentes), otros 300 ms.

**Lo que la app no enseña.** `dump` omite los nodos de las apps protegidas y `click` los rechaza, aunque estén en la otra mitad de una pantalla dividida; y el respaldo por adb hace lo mismo (`uidump.Parse`/`Find` con la lista).

**Límite honesto.** La guarda mira el primer plano un instante antes de ejecutar. Una app que pase a primer plano entre esa consulta y el toque (por ejemplo, abierta por una notificación) no se detecta. En la app, `click` vuelve a comprobar el paquete del nodo o de la ventana justo antes de actuar, y eso cierra el hueco para las ayudas. Para `input`, el hueco queda documentado. Queda también el texto que una app protegida ponga en pantalla en el instante entre la consulta de primer plano y la lectura. Un `screenrecord` ya iniciado no se puede cortar a media grabación, pero solo arranca con confirmación del usuario.

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

- **Cuándo:** al empezar cualquier `ui.request` de acción (`click` y `setText`), y también con `dump`, que falla como `click` y `setText` si la app no puede mostrarla. Se oculta 4 s después de la última operación. Con la sesión en pausa no se muestra nunca (la app lo comprueba también en `dump` y `notice`, por si la pausa llega tras las puertas del host). En `adbuddy screenshot` el aviso lleva en pantalla 400 ms como mínimo antes de retirarse y se retira unos 0,3 s durante la propia captura (`hideNotice`, con restauración automática a los 2 s como mucho y nunca en pausa) para que el marco y la píldora no tapen la imagen; vuelve justo después. Las lecturas en bruto del agente no lo retiran.
  - **Lecturas por adb:** antes de `screenshot` y de los `screencap`, `screenrecord`, `uiautomator` y `dumpsys` que vuelque la vista (crudos), el host pide el aviso a la app con la op `notice` (§3.7). Va después de la guarda y de la confirmación (si el comando es 🔴), y antes de la última comprobación de pausa: una pausa que llegue durante el aviso bloquea la lectura.
  - **Sin aviso posible:** sin app conectada o con el servicio de accesibilidad apagado no hay aviso, y la lectura sigue (el agente conserva su experiencia normal). Una app anterior que contesta `unsupported` también deja leer, sin consultar el servicio.
  - **Se bloquea** (`UI_BLOCKED`, `Blocked by ADBuddy: …`, sin ejecutar nada por adb) cuando la accesibilidad está activa y la app no está conectada o no contesta a tiempo, cuando no se puede comprobar si lo está (`dumpsys accessibility` falla), cuando la app no puede dibujar el aviso (`failed`) y cuando está en pausa (`protected`).
  - `screenrecord` solo avisa al empezar: el overlay se oculta a los 4 s aunque la grabación siga. Es 🔴 y la persona la confirma.
- **Qué:** un marco fino del color de acento del tema y una píldora arriba: "Agent using the screen · Pause" / "Agente usando la pantalla · Pausar". Tocar la píldora envía `pause` (el tipo de M2) con `Bridge.send`.
- **Marca del toque:** un círculo de 48 dp que se desvanece en 600 ms, centrado en el punto o en el centro de los límites del nodo.
- **Toques:** toda la ventana es `FLAG_NOT_TOUCHABLE` salvo la píldora, que es una ventana aparte, pequeña y tocable. La superposición nunca tapa la interfaz: no intercepta el toque de `dispatchGesture` ni los del usuario.
- Sus ventanas son de ADBuddy, así que no salen en `dump` y no se pueden pulsar (§4.2).
- Colores del tema, claro y oscuro; textos en inglés y en español (`values/strings.xml`, `values-es/strings.xml`).

## 7. Modo remoto (M5b)

Precisa §3.3 de la v1 sobre el contrato de §8.2 del protocolo.

### 7.1 Llegar al móvil fuera de casa con depuración inalámbrica (Wi-Fi + Tailscale)

- **La app** manda `device.info` al conectar y cuando cambia la red (`ConnectivityManager.NetworkCallback`). Pone `tailscaleIp` si hay una red VPN con una IPv4 de `100.64.0.0/10`, la de CGNAT que usa Tailscale. `wifiAdbPort` va vacío salvo que pueda leerlo: no hay API pública para el puerto de la depuración inalámbrica (validación manual pendiente).
- **`device.info` sin `tailscaleIp`** (vacío o ausente) no cambia la IP guardada: al salir de casa la app deja de ver la VPN, y el host necesita la última IP conocida para alcanzar el móvil.
- **El host,** con el móvil conectado, toma el puerto de la conexión que ya tiene, sin el shell del móvil: el usuario `shell` no puede leer `service.adb.tls.port` (SELinux, `adbd_prop`: `getprop` devuelve vacío con código 0; medido en la API 34, T1-12 del inventario de validaciones manuales; corregido en `fix/m5b-tls-port`). Si adb lista el móvil como `ip:puerto` en estado `device`, ese puerto; si lo lista por mDNS (`<instancia>._adb-tls-connect._tcp`), el del servicio `_adb-tls-connect._tcp` con esa misma instancia en `adb mdns services`, solo si todos sus anuncios dan el mismo puerto; por USB, conectado por el 5555 del modo remoto, o con la conexión caída, ninguno. Nunca guarda el 5555, ni un puerto fuera de 1-65535, ni el de una dirección de bucle local, sin especificar o de multidifusión (un túnel tiene su propio puerto), y sin puerto conocido se queda el guardado. La IP de esa conexión no se guarda (puede ser la de casa): `tailscale_ip` es siempre la que manda la app, y el puerto de la depuración inalámbrica es el mismo en todas las interfaces mientras adbd no se reinicie. adbd se reinicia, y cambia de puerto, con `adb tcpip 5555`, con `adb usb` (apagar el modo remoto, su caducidad y el invariante) y al reactivar la depuración: el puerto guardado vale hasta el siguiente `device.info`. El `wifiAdbPort` de la app solo cuenta si la conexión no da puerto, con las mismas reglas. Guarda `tailscale_ip` y `tailscale_port` en `config.toml`, como `address`. `findKnownPhone` prueba `tailscale_ip:tailscale_port` después de `address` y antes de esperar por mDNS. La comprobación de identidad (guid contra `ro.serialno`) no cambia.

### 7.2 Solo con datos móviles (`adb tcpip 5555`)

- **`remote.set {enabled: true}`** desde la app (con `rid`), y solo con el móvil conectado al host. El host:
  1. guarda `remote_until` = ahora + 12 h en `config.toml` ANTES de abrir el puerto (ruling de M5b T3): tras `adb tcpip` adbd se reinicia y cae la conexión, así que un `adb usb` para deshacer un guardado fallido fallaría también; si no se puede guardar, no se abre nada;
  2. ejecuta `adb tcpip 5555` con su adb, fuera del shim (es el usuario quien lo pide, no el agente). Si falla porque no hay móvil conectado (la orden no llegó a adbd), restaura el `remote_until` anterior: el modo queda como estaba (apagado si estaba apagado; "cerrando" si ya había caducado sin poder cerrar el puerto, nunca un falso apagado con el 5555 quizá abierto); con cualquier otro error (la orden pudo llegar) solo lo borra si la lectura del puerto (`service.adb.tcp.port`, o `persist.adb.tcp.port` si aquel está vacío: VB-41) confirma que el 5555 no está abierto, y si no, el modo sigue activo y se dice;
  3. responde con `remote.state {enabled, address: "<tailscaleIp>:5555", expiresAt}` y lo publica como evento.

  Sin `tailscaleIp` conocida, contesta `error` `unavailable` ("Turn on Tailscale on the phone first").
  La respuesta puede perderse: `adb tcpip` reinicia adbd y cae el túnel de la app. La app trata la falta de respuesta como "desconocido" y el host republica `remote.state` en cada `attach`.
  `adbuddy remote on` por IPC pide confirmación al usuario (plan de M5b, Task 8): el agente no puede abrir el 5555. La pregunta pasa por el daemon como las 🔴 (`Daemon.AskUser`): una sola pregunta a la vez y, mientras está abierta, todo comando del agente se rechaza (un `input tap` podría pulsar Permitir; revisión final de M5b, C1).
- **`remote.set {enabled: false}`,** al caducar `remote_until` o con `adbuddy remote off`: el host ejecuta `adb usb` y borra `remote_until`. Antes lee el puerto, `persist.adb.tcp.port` incluida (VB-41, plan `2026-10-04-adbuddy-vb41-persist-port`): `adb usb` no la borra, así que si nombra un puerto el estado lo dice (`adbuddy remote off|status` y su `--json`, `persistPort`; si no se pudo leer, `persistUnknown`) y la terminal de `serve` lo avisa una vez; nunca sale un «cerrado» a secas. Con el modo apagado y el puerto cerrado ahora (también con persist fijado y `service.adb.tcp.port` a `0`) no se ejecuta `adb usb`. `Reconcile` toma por abierto el 5555 de persist tras un reinicio: el modo sigue encendido hasta caducar. La tarjeta de la app no lleva este aviso (`remote.state` no cambia; pendiente).
- **Invariante** (`invariant.go`): con `remote_until` en el futuro, el puerto abierto es el esperado y `applyInvariant` no ejecuta `adb usb`. Sin `remote_until`, o ya pasado, se comporta como hoy. El puerto es el que lee `device.ReadTCP` (service, o persist si aquel está vacío); persist fijado se avisa en la terminal de `serve`, y si el puerto sigue abierto tras `adb usb` por persist, `DEVICE_PORT_OPEN` no manda reiniciar el móvil (eso lo volvería a abrir). El invariante solo corre al arrancar `serve`: cuando el móvil vuelve a mitad de sesión, el vigilante relee el puerto y avisa (puerto abierto con el modo apagado, persist), sin `adb usb`.
- **Reconexión:** si se pierde el móvil durante la sesión, `serve` prueba, en orden, `cfg.Address` (el primer candidato, la última IP:puerto que funcionó), el serial de siempre (mDNS), `tailscale_ip:tailscale_port` y, con el modo remoto activo, `tailscale_ip:5555`. Ese es el único candidato de red con el 5555: no se prueba la IP de casa con el 5555, sería probar a ciegas en cualquier red. Como el serial con el que adb lista el móvil cambia, el ejecutor del daemon tiene que tomar el serial activo de un sitio que se pueda cambiar, no de `Options.Serial`, fijado al arrancar. Eso toca `serve.go` y `daemon.go`, que son de M3: el plan de M5b se escribe sobre el `serve` de M3 y se acuerda con tern-51.
- **App:** interruptor en Ajustes con el aviso de §3.3 de la v1 (el puerto 5555 queda abierto en redes públicas) y la caducidad visible. Lo pinta `RemoteReducer`, que pasa a guardar `remote.state` en `BridgeState`.
- **Estado de tres valores (ruling de adbuddy-e5):** activo, apagado o desconocido. Tras un reset del estado de la app (`Reducer.reset` con una sesión nueva o un snapshot ilegible, de `m3-bridge`), el modo remoto es "desconocido", nunca `enabled=false`. Un falso "apagado" engañaría al usuario mientras el puerto 5555 sigue abierto. La app muestra "desconocido" como aviso, y el host vuelve a publicar `remote.state` al reconectar.
- **Concesiones solo a una app verificada (M6a):** toda concesión de M5 (accesibilidad, superposición, acceso a notificaciones, modo remoto) pasa por `phoneapp.Verified`, que solo da `phoneapp.VerifyInstalled` tras comprobar el certificado de la app instalada. No hay caminos alternativos. El modo remoto es una concesión más: M6a debe pasarlo por `phoneapp.Verified`.
- **`adbuddy remote [on|off|status]`** en el PC, para el usuario: desde la sesión del agente se rechaza (`AGENT_SESSION`, como `serve`).

## 8. Errores de la CLI

| Código | Salida | Cuándo |
|---|---|---|
| `DAEMON_NOT_RUNNING` | 1 | `adbuddy serve` no está en marcha (el mismo error que `undo`) |
| `USAGE` | 2 | Argumentos no válidos, en la CLI o en el daemon (`UI_INVALID`) |
| `UI_BLOCKED` | 126 | Pausa, pregunta pendiente o guarda. El mensaje empieza por `Blocked by ADBuddy:`, como en el shim |
| `UI_NOT_FOUND` | 1 | `not_found` |
| `UI_AMBIGUOUS` | 1 | `ambiguous`, con las coincidencias |
| `UI_PROTECTED` | 126 | `protected` de la app (contraseña, bloqueado, ADBuddy, app protegida) |
| `UI_UNAVAILABLE` | 75 | Sin app, sin servicio y sin respaldo posible |
| `UI_TIMEOUT` | 75 | Sin respuesta a tiempo (§2) |
| `REMOTE_UNAVAILABLE` | 1 | `adbuddy remote on` sin móvil conectado o sin IP de Tailscale conocida |

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

**M5c:** `listener/` (`AgentNotificationListener`, `NotificationReport`, `NotificationAccess`, `NotificationAccessStatus`), las pantallas de `ui/` (apps protegidas, acceso a notificaciones, Ajustes y los pasos del onboarding, con la navegación dentro de `ChatRoot`), el servicio en el manifest, y en el host `daemon/notifications.go` y `cli/helpers_notifications.go`.

## 10. Lo que el servicio de accesibilidad declara

```xml
android:accessibilityEventTypes="typeWindowStateChanged|typeWindowsChanged|typeNotificationStateChanged"
android:accessibilityFlags="flagDefault|flagRetrieveInteractiveWindows|flagReportViewIds"
android:canRetrieveWindowContent="true"
android:canPerformGestures="true"
android:notificationTimeout="100"
```

Sin `canTakeScreenshot`: la captura la hace el host (§3.6). La descripción del servicio (M2) ya promete que nunca lee contraseñas ni actúa en apps protegidas, y §3.1 y §4.3 lo cumplen.

## 11. Tests

- **Host:**
  - el daemon con un `UI` falso: cada puerta de §4.1, el orden pausa → pregunta → guarda → admisión, y una entrada del diario por ayuda (dos con `--screen`: la de la acción y la `adbuddy screen` de la lectura posterior);
  - la guarda: con una app protegida, ADBuddy, primer plano desconocido o sin lista, para `input`, `am start -n`, `monkey -p`, `screencap` y las ayudas;
  - el respaldo con el adb falso de `testutil`: `uiautomator` con el servicio vinculado y sin vincular, `input text` con espacios, comillas y Unicode, y `dumpsys` de Android 11 a 17 (muestras reales en `testdata`);
  - `uiArea` con una app falsa de `bridge`: respuesta tardía, respuesta de otra conexión, `id` desconocido y caducidad;
  - la CLI: la salida de texto escapada, `--json`, los códigos de salida, y `tap`/`type --screen` y `screen --settle` (pantalla, nota con salida 0, daemon antiguo, aviso de pantalla cambiante);
  - el reescalado: tamaño y formato.
- **App** (JVM, sin Android):
  - `Dumper`, `Matcher` y `ProtectedApps` sobre un modelo propio de nodo (`UiNode`), que el servicio construye a partir de `AccessibilityNodeInfo`, para que la lógica no dependa de clases de Android;
  - los casos: contraseña, bloqueado, ventanas de ADBuddy, la interfaz del sistema con pregunta pendiente, ambigüedad, recorte de límites, topes y el `id` repetido o caducado.
- **Fixtures** (`protocol/fixtures/`): las de §12, con ida y vuelta estricta a los dos lados (spec del protocolo, §12).
- **Emulador:** el servicio real con `connectedDebugAndroidTest` cuando M2.5b resuelva la imagen (pendientes, M2.5b). Hasta entonces, validaciones manuales.
- **Validaciones manuales pendientes** (en el plan): la latencia de `screen` en el Pixel; `type` con emoji; la superposición en claro y oscuro; la guarda con una app de banca real; la lectura del puerto de la depuración inalámbrica desde la app (M5b); `tcpip 5555` y la reconexión por Tailscale solo con datos (M5b).

## 12. Cambios al protocolo (necesitan el ok de adbuddy-e5)

Todos aditivos (§11 del protocolo: sigue siendo `v1`):

**Integrado en `main` (9524c49, PR #5),** con los arreglos de su revisión:

1. **`device.info.protectedApps`:** lista de paquetes, opcional.
   - Si falta (`null`), quiere decir "sin cambios": el host conserva la lista anterior. Si viene, aunque sea vacía, es la lista entera.
   - Go: `ProtectedApps []string \`json:"protectedApps,omitempty"\``. El host solo la decodifica: nil frente a vacía.
   - Kotlin: `val protectedApps: List<String>? = null`.
   - Fixtures: se amplía `app/device.info.json` y se añade `@protectednone`.
   - Una lista con más de `MaxProtectedApps` (1000) paquetes, o con uno que no cumpla `ValidPackage`, se rechaza entera.
2. **El código de error `ambiguous`,** para `ui.response`.
   - Go: `CodeAmbiguous`; Kotlin: la constante equivalente.
   - Fixture: `app/ui.response@ambiguous.json`.
3. **Las fixtures de las operaciones:**
   - `host/ui.request@settext.json`, `@clickid.json`, `@clickxy.json` y `@dumpall.json`;
   - `app/ui.response@dump.json`, `@foreground.json`, `@settext.json` y `@notifications.json`.

   Con sus tests a los dos lados: las estructuras de `args` y `result` de §3 se decodifican en modo estricto.
4. **`protocol/fixtures/ui.json`,** que leen los dos lados:
   - las operaciones (registro `UIOps` / `UiOp.ALL`) y qué operación responde cada fixture;
   - los límites: `MaxUIResult` 512 KiB, `MaxNotifications` 50, `MaxNotificationText` 1024, `MaxSetText` 4096 code points, `MaxProtectedApps` 1000 y `MaxPackage` 255;
   - los casos de `ValidPackage`.
   - `notifications` gana `truncated`.
5. **La op `notice` de `ui.request`** (screen-overlay-frame): `args` y `result` vacíos (`Empty`). Go: `UIOpNotice` en `UIOps`; Kotlin: `UiOp.NOTICE` en `UiOp.ALL`. Fixtures `host/ui.request@notice.json` y `app/ui.response@notice.json`, y entrada en `protocol/fixtures/ui.json`. Aditivo; un commit con Go, Kotlin y fixtures.
6. **La op `hideNotice` de `ui.request`** (p1-pantalla-oculta): `args {ms}`, `result` vacío; `MaxHideNoticeMs` 5000. Go: `UIOpHideNotice` en `UIOps` y `UIHideNoticeArgs`; Kotlin: `UiOp.HIDE_NOTICE` y `UiHideNoticeArgs`. Fixtures de petición y respuesta, y entrada en `protocol/fixtures/ui.json`. Aditivo; un commit con Go, Kotlin y fixtures.
7. **El campo `unreadable` de `UiDump`** (p1-pantalla-oculta): `bool` con `omitempty` en Go, `Boolean?` sin emitir `false` en Kotlin; fixture `app/ui.response@dumpunreadable.json`. Aditivo.

Cada cambio fue en su propio commit (Go, Kotlin y fixtures; los de los puntos 6 y 7 en dos commits, uno por cambio), como pide §10 del protocolo, y se avisa a tern-51 y a skimmer-32 al integrar.

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
- **Ruling (M5b): las capturas por defecto en modo remoto pasan a 0,5 en JPEG, decidido por el daemon (que sabe si el modo remoto está activo), no por la CLI.** **Por qué:** la CLI corre en la sesión del agente y no conoce el estado; con datos móviles ahorra transporte. **Coste si me equivoco:** el agente pide `--format png` y lo tiene.
- **Ruling (M5b): la persiana, los ajustes rápidos y las notificaciones emergentes cuentan como app protegida en la guarda; nada que lea la pantalla queda fuera de ella en v1.** **Por qué:** cierra el hueco de los códigos 2FA en `com.android.systemui` sin depender del paquete de cada notificación (§4.3, punto 6). **Coste si me equivoco:** el agente no puede usar los ajustes rápidos mientras están abiertos (puede usar `cmd`/`settings`); lo valida el usuario en el Pixel.
- **Ruling (M5c): el contenido de una notificación sale vacío en dos capas (app y host), y la clave también; el listener solo lee, nada fuera de su paquete lo alcanza y un test de fuente lo vigila.** **Por qué:** un código 2FA de una app protegida no puede salir del móvil, y el agente no debe poder descartar ni accionar notificaciones. **Coste si me equivoco:** el agente ve menos de lo debido, o un hueco que no se aparca.
- **Ruling (M5c): `notifications` no pasa por la guarda de primer plano (sí por pausa, pregunta 🔴 y diario) y no tiene respaldo por adb.** **Por qué:** no lee la pantalla y adb daría todo el texto. **Coste si me equivoco:** una condición en `daemon/ui.go`.
- **Ruling (M5c): el acceso a notificaciones tiene tres valores, y permitido exige ajuste y conexión.** **Por qué:** lo desconocido no se pinta como permitido ni como denegado (ruling de M5b). **Coste si me equivoco:** un "desconocido" de más.
- **Ruling (M5c): las apps que reciben los códigos (Mensajes, Gmail, la app de SMS, Play services) están protegidas desde el primer arranque, por la propuesta.** **Por qué:** un OTP por SMS o correo es tan secreto como el de un banco y no puede esperar a la confirmación. **Coste si me equivoco:** el agente no lee esas apps hasta que el usuario las desmarque.
- **Ruling (M5c): `notifications` exige la confirmación del usuario en la app, además del acceso del sistema.** **Por qué:** el agente podría activar el interruptor del sistema con `adbuddy tap`, pero no pulsar el botón de ADBuddy. **Coste si me equivoco:** un toque de más del usuario tras cada reinicio, actualización o muerte de la app (la marca se borra en cada nuevo enlace del listener, M5c-32 b). El host no concede el acceso (el `setup` de M6a decide si lo ofrece).
