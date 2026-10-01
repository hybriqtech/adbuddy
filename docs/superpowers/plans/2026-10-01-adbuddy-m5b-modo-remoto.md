# ADBuddy M5b · Modo remoto · Plan de implementación

> **Para quien ejecute:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`. Los pasos usan casillas (`- [ ]`). Un subagente a la vez; implementadores y re-revisiones en Sonnet, Opus solo para la revisión de seguridad de las tareas marcadas 🔒 y la final. Nunca Haiku.

**Meta:** que el usuario llegue a su móvil fuera de casa (Wi-Fi + Tailscale, o solo datos con `adb tcpip 5555`), con un interruptor en la app, una caducidad de 12 h que cierra el host, y la reconexión automática de `serve` cuando cambia la dirección del móvil. Más lo que arrastra M5a: la decisión de spec sobre los 2FA visibles en `screen`/`screenshot`, `ignoreUnknownKeys=false` en `UiRequest` y menores del ledger.

**Arquitectura:** el host gana un paquete `remote` (controlador de `adb tcpip`/`adb usb` con caducidad), una zona `remote` del enlace con la app (`device.info`, `remote.set`, `remote.state`), un `activePhone` (el serial del móvil deja de estar fijado al arrancar `serve`) con un vigilante que reconecta, y el subcomando `adbuddy remote`. La app implementa `RemoteReducer` con estado de tres valores, manda `device.info` al cambiar la red y pinta el interruptor. **El protocolo no cambia** (todos los tipos y fixtures de `remote.*` y `device.info` ya existen en `main`): ninguna tarea toca `protocol/`.

**Stack:** Go 1.2x (`host/`), Kotlin + Compose (`android/`), TOML (BurntSushi), adb falso de `testutil`.

**Specs:** `docs/superpowers/specs/2026-10-01-adbuddy-m5-ayudas-remoto-design.md` (§7, §4.3, §12, §13) · `2026-09-29-adbuddy-v1-design.md` (§3.3, §4.6) · `2026-10-01-adbuddy-protocolo-m3-m5-design.md` (§8.2, §10) · `docs/superpowers/plans/pendientes-m3-m5.md` (M5, I7).

## Global Constraints

- Rama `m5`, ya sobre `main` (a11cbf2). Commits con identidad local hybriqtech y trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Sin push, PR ni cambios en `protocol/` sin el ok de adbuddy-e5. Nunca `--force`. `git grep` de seriales e IPs antes de subir.
- El móvil del usuario no se toca: tests con el adb falso de `testutil` o el emulador. Lo que no se pueda probar va a "Validaciones manuales pendientes" (al final).
- `-race` no corre en el PC Windows: leer los globales de test antes de lanzar gorrutinas, y que lo corra la CI.
- Cada tarea compila y pasa sus tests por sí sola; nada usa clases de tareas posteriores.
- Todo manejador de una petición con `rid` responde por todos sus caminos; los que guardan estado por conexión comprueban `Server.Current(m)`; ningún reductor lanza excepción con ningún payload (pendientes de M3).
- **Estado de tres valores** (ruling de e5): activo, apagado o desconocido. Tras `Reducer.reset` con sesión nueva o snapshot ilegible, la app muestra "desconocido", nunca "apagado".
- Un `adbuddy remote on|off` y `serve` no se pueden lanzar desde la sesión del agente (`AGENT_SESSION`); el servidor IPC lo comprueba también (`ADBUDDY_SOURCE=agent`), aun sabiendo que lo declara el cliente (pendientes de M3: la defensa real es la identidad del par del socket).
- Solo `go test` del paquete tocado más `go vet ./...` y `gofmt -l .` limpios; en Kotlin `./gradlew :app:testDebugUnitTest` (JVM). Con `-timeout 5m` en Go.

## Rulings de este plan (van al ledger tal cual)

1. **Ruling: `serve` no tiene bucle de reconexión (M3 no lo trajo) y M5b lo construye mínimo: un vigilante que cada 10 s mira si el serial activo sigue en `adb devices` y, tras dos fallos seguidos, prueba las direcciones candidatas.** — Sin él, `adb tcpip` (que reinicia adbd y cambia el serial) deja a `serve` sin móvil, y el modo remoto no sirve. — Si me equivoco: el vigilante se desactiva sin tocar nada más; el coste es un fichero (`phone_watch.go`).
2. **Ruling: el serial activo vive en `activePhone` (implementa `daemon.Executor`, `device.ADB`, `phoneapp.ADB`), no en `Options.Serial`.** — `daemon.Options.ADB` es una interfaz: no hay que tocar las 8 llamadas del daemon. Solo cambia la comprobación de "solo el móvil activo" (el agente conserva el `ANDROID_SERIAL` con el que arrancó) y el `Ping`. — Si me equivoco: volver al serial fijo es quitar el `Switch`.
3. **Ruling: `adb tcpip` y `adb usb` reinician adbd y cortan el túnel de la app: el `remote.state` de respuesta puede perderse; la app lo trata como "desconocido" hasta el snapshot de la reconexión, y el host republica el estado al `attach`.** — Es lo que hace adbd; esperar a que vuelva el túnel antes de contestar bloquearía la petición hasta 45 s. — Si me equivoco: la app muestra "Activando…" unos segundos más.
4. **Ruling: `Disable` ejecuta `adb usb` antes de borrar `remote_until`; si falla, el estado sigue "activo" y se reintenta en el siguiente tic.** — Un falso "apagado" con el 5555 abierto es lo que e5 quiere evitar. — Si me equivoco: el usuario ve "activo" con el puerto ya cerrado hasta el siguiente tic (30 s); el arranque lo reconcilia.
5. **Ruling: `remote_until` se guarda como texto RFC 3339 (`remote_until = "2026-10-02T09:00:00Z"`) y `tailscale_ip` solo se acepta dentro de `100.64.0.0/10`.** — El codificador TOML no omite bien un `time.Time` cero; y la app solo informa de esa franja (CGNAT de Tailscale). — Si me equivoco: un campo más en `Validate`.
6. **Ruling: `device.info` con `tailscaleIp` vacío o ausente significa "sin cambios".** — Al salir de casa la app deja de ver la VPN en la red del móvil, pero el host necesita la última IP conocida para alcanzarlo fuera. — Si me equivoco: la IP vieja queda guardada hasta que llegue otra.
7. **Ruling (I7, decisión de spec antes de v1): mientras la persiana de notificaciones o los ajustes rápidos están desplegados, la guarda los trata como "app protegida".** Es decir: bloquea `screen`, `screenshot`, `screencap`, los toques y `input` de la guarda, salvo `input keyevent` con BACK (4) o HOME (3) o `KEYCODE_BACK`/`KEYCODE_HOME` para poder cerrarla. Se detecta en el host con `dumpsys window` (la ventana con foco es `NotificationShade`). Falla cerrado si `dumpsys window` no se entiende. **Heads-up (ok de e5):** una notificación emergente es una ventana de `systemui` sobre cualquier app y no tiene el foco, así que `mCurrentFocus` no la ve. La app lo señala: el servicio de accesibilidad escucha `typeNotificationStateChanged` y, durante 8 s tras cualquier notificación que no sea de ADBuddy, `foreground` y `dump` dan `protected` (la guarda del host ya consulta `foreground`). Sin app, el host lo mira en `dumpsys window windows` (ventanas de `com.android.systemui` visibles que no estén en una lista corta de las fijas: barra de estado, navegación, decoración de pantalla) y, si no se entiende, falla cerrado. — Cierra el hueco de los códigos 2FA en `com.android.systemui` sin depender del paquete de cada notificación, que la accesibilidad no da. — Si me equivoco: el agente no puede usar los ajustes rápidos mientras están abiertos (puede usar `cmd`/`settings`); lo valida el usuario en el Pixel (validación 14).
8. **Ruling (I7, resto; corregido por e5): nada que lea la pantalla queda fuera de la guarda en v1.** `screenrecord` pasa a 🔴 especial (regla `screen.record`: graba mucho después de que la guarda mirase); `screencap` y `uiautomator` siguen pasando por la guarda; `dumpsys activity top` (y cualquier `dumpsys activity` con argumento fuera de una lista corta: `activities`, `a`, `services`, `providers`, `broadcasts`, `processes`) pasa por la guarda como lectura de pantalla, porque vuelca el árbol de vistas con su texto; `dumpsys notification --noredact` pasa a 🔴 (da el texto de todas las notificaciones, 2FA incluidos). `dumpsys window` se queda 🟢 (solo títulos de ventana, sin contenido). — Un `screenrecord` ya iniciado no se puede cortar a media grabación, pero solo arranca con confirmación del usuario. — Si me equivoco: el agente pide confirmación para grabar y para leer notificaciones completas; `dumpsys activity top` falla en primer plano protegido.
9. **Ruling: las capturas por defecto en modo remoto pasan a 0,5 en JPEG decidiéndolo el daemon (que sabe si el modo remoto está activo), no la CLI.** — La CLI corre en la sesión del agente y no conoce el estado. — Si me equivoco: el agente pide `--format png` y lo tiene.

## Mapa de ficheros

**Host (nuevo):**
- `host/internal/remote/remote.go`, `remote_test.go`: `Controller`, `State`, `Store`.
- `host/internal/cli/hostconfig.go`: `cfgStore` (config.toml compartido, con mutex).
- `host/internal/cli/phone_remote.go`: zona `remote` del enlace (`device.info`, `remote.set`).
- `host/internal/cli/activephone.go`: `activePhone`.
- `host/internal/cli/phone_watch.go`: vigilante y candidatas.
- `host/internal/cli/remotecmd.go`: `adbuddy remote` y `handleRemote` de `serve`.

**Host (modificado, además):** `classify/checks_read.go`, `classify/rules.go` (Task 10b) y `uidump/` (Task 10).

**Host (modificado):** `config/config.go` (3 campos), `device/device.go` (`EnsureNoTCPPort` con puerto que se conserva), `cli/invariant.go`, `cli/connect.go` (`findKnownPhone` con varias direcciones), `cli/serve.go`, `cli/setup.go`, `cli/phone.go` (registro de la zona), `ipc/ipc.go` (`KindRemote`), `daemon/daemon.go` (serials extra y `RemoteActive`), `daemon/ui.go` (valores por defecto de la captura), `daemon/guard.go` (persiana), `cli/helpers.go` (captura), `cli/root.go` (registrar el subcomando).

**App:** `bridge/BridgeState.kt` (`remote: RemoteState?`), `bridge/RemoteReducer.kt`, `bridge/Protocol.kt` (`Snapshot.remote` anulable), `a11y/UiHelpers.kt` (envoltura estricta), `net/TailscaleIp.kt` y `net/DeviceInfoSender.kt` (nuevos), `ui/RemoteUi.kt` (lógica pura) y `ui/Screens.kt` (tarjeta), `res/values/strings.xml` y `values-es/strings.xml`.

---

### Task 1: Spec y ledger

**Correcciones de la revisión opus (obligatorias; mandan sobre el texto de esta tarea):**
- Añadir también a la spec: §10 gana `typeNotificationStateChanged` en `accessibilityEventTypes`; el "Coste" de §4.3 suma un `dumpsys window` por orden guardada; §8 gana `REMOTE_UNAVAILABLE` (salida 1); §7.2 dice que el único candidato de red es `tailscale_ip:5555` (no se prueba la IP de casa con el 5555: sería probar a ciegas en cualquier red) y que `adbuddy remote on` por IPC pide confirmación al usuario (ver Task 8); §7.2 y §13 dicen que el modo remoto es una concesión que M6a debe pasar por `phoneapp.Verified`; §4.3 sobre heads-up dice que `adb bugreport` es 🔴 (Task 10b).
- Los ledgers de `.superpowers/` no se versionan (`git ls-files .superpowers` vacío): no hay `git add` del ledger.


**Files:**
- Modify: `docs/superpowers/specs/2026-10-01-adbuddy-m5-ayudas-remoto-design.md`
- Create: `.superpowers/sdd/2026-10-01-adbuddy-m5b-modo-remoto/progress.md`

**Interfaces:** ninguna.

- [ ] **Step 1: Ledger.** Crear `progress.md` con cabecera "M5b · modo remoto", el enlace a este plan y los 9 rulings de arriba, cada uno como `Ruling: qué — por qué — qué cuesta si me equivoco`.
- [ ] **Step 2: Spec §4.3.** Añadir, tras el punto 5 de "La guarda", el punto 6: "**Persiana.** Con la persiana de notificaciones o los ajustes rápidos desplegados (`dumpsys window`: la ventana con foco es `NotificationShade`), la guarda bloquea como con una app protegida, y falla cerrada si no entiende `dumpsys window`. Se exceptúan `input keyevent` con `3`, `4`, `KEYCODE_HOME` o `KEYCODE_BACK`, que sirven para cerrarla. Las notificaciones emergentes (heads-up) cuentan igual: la app las señala 8 s (rulings 7 de este plan). Así un código 2FA de una app protegida no sale en `screen` ni en `screenshot`." Y en "La guarda" y "Límite honesto": `screenrecord` es 🔴 especial; `dumpsys activity top` pasa por la guarda; `dumpsys notification --noredact` es 🔴; `dumpsys window` queda 🟢 (sin contenido de pantalla). Límite honesto restante: el texto que una app protegida ponga en pantalla en el instante entre la consulta de primer plano y la lectura.
- [ ] **Step 3: Spec §7.** Añadir a 7.2: la respuesta a `remote.set` puede perderse (adbd se reinicia y cae el túnel); la app lo trata como desconocido y el host republica `remote.state` en cada `attach`. Añadir a 7.1: `device.info` sin `tailscaleIp` no cambia la IP guardada. Añadir a §13 el ruling 9 (capturas en modo remoto decididas por el daemon).
- [ ] **Step 4: Commit.**

```bash
git add docs/superpowers/specs docs/superpowers/plans/2026-10-01-adbuddy-m5b-modo-remoto.md
git commit -m "docs(spec): decisiones de M5b: persiana como protegida, límite honesto y respuesta perdida de remote.set"
```
(El ledger vive en `.superpowers/` y se comitea con `git add -f` solo si el repo ya versiona los ledgers anteriores; comprobarlo con `git ls-files .superpowers | head`.)

---

### Task 2: Campos de config

**Files:**
- Modify: `host/internal/config/config.go`
- Test: `host/internal/config/config_test.go`

**Interfaces:**
- Produces: `Config.TailscaleIP string`, `Config.TailscalePort int`, `Config.RemoteUntil string`; `func (c Config) RemoteUntilTime() time.Time` (cero si vacío o ilegible), `func ValidTailscaleIP(s string) bool`.

- [ ] **Step 1: Tests que fallan.** En `config_test.go`:

```go
func TestRemoteFields(t *testing.T) {
	ok := Default()
	ok.TailscaleIP, ok.TailscalePort = "100.101.102.103", 41234
	ok.RemoteUntil = "2026-10-02T09:00:00Z"
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := ok.RemoteUntilTime(); !got.Equal(time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("until %v", got)
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, ok); err != nil {
		t.Fatal(err)
	}
	back, err := Load(path)
	if err != nil || back.TailscaleIP != ok.TailscaleIP || back.RemoteUntil != ok.RemoteUntil {
		t.Fatalf("%+v %v", back, err)
	}
	for name, mut := range map[string]func(*Config){
		"ip outside CGNAT":    func(c *Config) { c.TailscaleIP = "198.51.100.5" },
		"ip not an address":   func(c *Config) { c.TailscaleIP = "tailscale" },
		"port without ip":     func(c *Config) { c.TailscaleIP, c.TailscalePort = "", 4000 },
		"port out of range":   func(c *Config) { c.TailscalePort = 70000 },
		"until not a time":    func(c *Config) { c.RemoteUntil = "tomorrow" },
	} {
		c := ok
		mut(&c)
		if c.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if (Config{RemoteUntil: "x"}).RemoteUntilTime() != (time.Time{}) {
		t.Error("an unreadable until must read as none")
	}
}
```

- [ ] **Step 2:** `go test ./internal/config -run TestRemoteFields -timeout 5m` → FALLA (no compila).
- [ ] **Step 3: Implementar.** En `Config` añadir, tras `ChatPort`:

```go
	TailscaleIP    string `toml:"tailscale_ip"`    // the phone's Tailscale IPv4, as its app reported it; empty = unknown
	TailscalePort  int    `toml:"tailscale_port"`  // wireless debugging port read from the phone over the same connection; 0 = unknown
	RemoteUntil    string `toml:"remote_until"`    // RFC 3339: remote mode (adb tcpip 5555) is on until then; empty = off
```

En `Validate`, antes del `switch c.Language`:

```go
	if c.TailscaleIP != "" && !ValidTailscaleIP(c.TailscaleIP) {
		return fmt.Errorf("tailscale_ip must be an IPv4 address in 100.64.0.0/10, got %q", c.TailscaleIP)
	}
	if c.TailscalePort != 0 && (c.TailscaleIP == "" || c.TailscalePort < 1 || c.TailscalePort > 65535) {
		return fmt.Errorf("tailscale_port needs tailscale_ip and must be between 1 and 65535, got %d", c.TailscalePort)
	}
	if c.RemoteUntil != "" {
		if _, err := time.Parse(time.RFC3339, c.RemoteUntil); err != nil {
			return fmt.Errorf("remote_until must be an RFC 3339 time (2026-10-02T09:00:00Z), got %q", c.RemoteUntil)
		}
	}
```

Y al final del fichero:

```go
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// ValidTailscaleIP accepts only an IPv4 address in Tailscale's CGNAT range.
func ValidTailscaleIP(s string) bool {
	a, err := netip.ParseAddr(s)
	return err == nil && a.Is4() && cgnat.Contains(a)
}

// RemoteUntilTime is remote_until as a time; zero when it is empty or unreadable.
func (c Config) RemoteUntilTime() time.Time {
	t, err := time.Parse(time.RFC3339, c.RemoteUntil)
	if err != nil {
		return time.Time{}
	}
	return t
}
```
- [ ] **Step 4:** `go test ./internal/config -timeout 5m` → PASA; `gofmt -l internal/config` vacío.
- [ ] **Step 5: Commit** `feat(host): campos tailscale_ip, tailscale_port y remote_until en config.toml`.

---

### Task 3: Controlador del modo remoto e invariante 🔒

**Correcciones de la revisión opus (obligatorias; mandan sobre el texto de esta tarea):**
- **El código del controlador de más abajo ya incluye las correcciones C2 y C3 y M1/M9/M14** (escritura anticipada de `remote_until`; `State.Closing`; `Enable` idempotente; tope de 12 h; `ADB` solo con `Run`). `Store`, `State` y los tests son los de esa versión.
- `State.Enabled` es verdad mientras `remote_until` exista, también caducado (`Closing`): `serve`, el snapshot, `candidateAddrs` y `keep` del invariante usan `Enabled`, no "vigente". `applyInvariant(..., keep)` recibe `remote.Port` si `State().Enabled`.
- `device.ADB` no tiene `Shell`: `EnsureNoTCPPort` usa su `TCPPort`; no cambiar esa interfaz.


**Files:**
- Create: `host/internal/remote/remote.go`, `host/internal/remote/remote_test.go`
- Modify: `host/internal/device/device.go:232` (`EnsureNoTCPPort`), `host/internal/device/device_test.go`, `host/internal/cli/invariant.go`, `host/internal/cli/invariant_test.go`, `host/internal/cli/serve.go:135`, `host/internal/cli/setup.go:267`

**Interfaces:**
- Consumes: `config.Config.RemoteUntilTime`.
- Produces:
  - `remote.Port = "5555"`, `remote.Lifetime = 12 * time.Hour`.
  - `remote.State{Enabled, Closing bool; Address string; ExpiresAt time.Time}`.
  - `remote.Store` (`TailscaleIP() string`, `Until() time.Time`, `SetUntil(time.Time) error`; cero borra).
  - `remote.ADB` (solo `Run(ctx, dir, args, stdin, stdout, stderr) (int, error)`; el `getprop` va por `Run` con `shell`).
  - `remote.New(a ADB, s Store, now func() time.Time) *Controller`; `(*Controller).State() State`; `Enable(ctx) (State, error)`; `Disable(ctx) (State, error)`; `Expire(ctx) (State, bool, error)` (cierra si caducó; `bool` = cambió); `Reconcile(ctx) (State, error)` (arranque).
  - `var ErrNoTailscale`; `device.EnsureNoTCPPort(ctx, a ADB, keep string) (bool, error)`; `applyInvariant(ctx, runner, wait, log, command, keep string)`.

- [ ] **Step 1: Tests de `device`.** En `device_test.go` ampliar `TestEnsureNoTCPPort`: con `keep == "5555"` y el puerto `5555`, devuelve `false` y no ejecuta `usb`; con `keep == "5555"` y el puerto `37123` (depuración inalámbrica), ejecuta `usb`; con `keep == ""` todo como hoy. Actualizar las llamadas existentes a `EnsureNoTCPPort(ctx, f, "")`.
- [ ] **Step 2:** `go test ./internal/device -timeout 5m` → FALLA. Implementar:

```go
func EnsureNoTCPPort(ctx context.Context, a ADB, keep string) (bool, error) {
	port, err := TCPPort(ctx, a)
	if err != nil || port == "" || keep != "" && port == keep {
		return false, err
	}
	...resto igual
```
Con su comentario: "keep is a port that must stay open (remote mode's 5555); another open port is still closed." Pasa.

- [ ] **Step 3: Tests del controlador** (`remote_test.go`, con un `fakeADB` que registra los argumentos de `Run` y responde a `Shell("getprop service.adb.tcp.port")` con un valor configurable, y un `memStore`):

```go
func TestEnableSavesUntilThenRunsTcpip(t *testing.T)       // orden: SetUntil(now+12h) antes de `tcpip 5555`; State{Enabled, Address "100.64.0.5:5555"}
func TestEnableTwiceRunsTcpipOnce(t *testing.T)            // ya activo y vigente → ninguna orden y el mismo State
func TestEnableNeedsTailscaleIP(t *testing.T)              // store sin IP → ErrNoTailscale, ninguna orden, nada guardado
func TestEnableTcpipFailsPortClosedClearsUntil(t *testing.T) // tcpip falla y getprop da "" o "37123" → error y Until vuelve a cero
func TestEnableTcpipFailsPortOpenKeepsUntil(t *testing.T)    // tcpip falla pero getprop da "5555" → Until intacto (State activo)
func TestEnableTcpipFailsPortUnreadableKeepsUntil(t *testing.T) // tcpip y getprop fallan → Until intacto
func TestEnableSaveFailsRunsNothing(t *testing.T)          // SetUntil falla → error y NINGUNA orden de adb
func TestDisableRunsUsbThenClears(t *testing.T)            // orden: usb antes de SetUntil(zero)
func TestDisableUsbFailsKeepsEnabled(t *testing.T)         // usb falla → Until intacto, State.Enabled true, error
func TestExpire(t *testing.T)                              // antes de la hora: (State, false); después: usb, borra, true
func TestExpireUsbFailsStaysClosing(t *testing.T)          // usb falla tras caducar: State{Enabled:true, Closing:true}, nunca apagado, error
func TestReconcileClearsStaleUntil(t *testing.T)           // Until futuro y puerto "" o "37123" → se borra, nada que ejecutar salvo el getprop
func TestReconcileKeepsLiveUntil(t *testing.T)             // Until futuro y puerto "5555" → intacto
func TestReconcileClosesExpiredOpenPort(t *testing.T)      // Until pasado y puerto "5555" → usb y se borra
func TestReconcilePortUnreadableChangesNothing(t *testing.T) // getprop falla → error, Until intacto
func TestUntilIsCappedAtLifetime(t *testing.T)             // Until a 30 días en config.toml → ExpiresAt = now+12h
```
- [ ] **Step 4:** `go test ./internal/remote -timeout 5m` → FALLA (no existe el paquete).
- [ ] **Step 5: Implementar `remote.go`:**

```go
// Package remote is the remote mode (spec §3.3 and M5 §7.2): adb listens on
// port 5555 on every interface until it expires, so the host can reach the
// phone over Tailscale with only mobile data.
package remote

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	Port     = "5555"
	Lifetime = 12 * time.Hour
)

// ErrNoTailscale: the app has not reported a Tailscale address yet.
var ErrNoTailscale = errors.New("Turn on Tailscale on the phone first")

// ADB runs adb against the active phone.
type ADB interface {
	Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error)
}

// Store is where the mode's persistent facts live (config.toml).
type Store interface {
	TailscaleIP() string
	Until() time.Time
	SetUntil(time.Time) error // the zero time clears it
}

// State is the mode as the host has it. Enabled stays true from the moment
// the expiry is saved until `adb usb` has really closed the port: an expiry
// that has passed but could not be closed is Enabled and Closing, never "off"
// (a false off would hide an open port 5555).
type State struct {
	Enabled   bool
	Closing   bool // the expiry passed and the port is not known to be closed yet
	Address   string // ip:port while enabled
	ExpiresAt time.Time
}

type Controller struct {
	mu  sync.Mutex // one change at a time: adbd restarts under each
	adb ADB
	st  Store
	now func() time.Time
}

func New(a ADB, s Store, now func() time.Time) *Controller {
	if now == nil {
		now = time.Now
	}
	return &Controller{adb: a, st: s, now: now}
}

// until is the saved expiry, never more than Lifetime ahead (a config.toml
// edited by hand cannot keep the port open longer).
func (c *Controller) until() time.Time {
	u := c.st.Until()
	if max := c.now().Add(Lifetime); u.After(max) {
		return max
	}
	return u
}

func (c *Controller) State() State {
	until := c.until()
	if until.IsZero() {
		return State{}
	}
	s := State{Enabled: true, Closing: !until.After(c.now()), ExpiresAt: until}
	if ip := c.st.TailscaleIP(); ip != "" {
		s.Address = net.JoinHostPort(ip, Port)
	}
	return s
}

func (c *Controller) adbCmd(ctx context.Context, args ...string) (string, error) {
	var out, errb bytes.Buffer
	code, err := c.adb.Run(ctx, "", args, nil, &out, &errb)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("adb %s: %s", args[0], strings.TrimSpace(errb.String()+" "+out.String()))
	}
	return strings.TrimSpace(strings.ReplaceAll(out.String(), "\r", "")), nil
}

// Enable opens port 5555 (`adb tcpip 5555`). The expiry is saved BEFORE the
// port opens: right after `adb tcpip` adbd restarts and the connection is
// gone, so an `adb usb` to undo a failed save would fail too. If `tcpip`
// fails the expiry is cleared again; if that also fails, an expiry with no
// open port is left, which is the safe side (Reconcile clears it).
// Already enabled: nothing is run. adbd restarts, so the caller does not
// wait for the connection.
func (c *Controller) Enable(ctx context.Context) (State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st.TailscaleIP() == "" {
		return c.State(), ErrNoTailscale
	}
	if s := c.State(); s.Enabled && !s.Closing {
		return s, nil
	}
	if err := c.st.SetUntil(c.now().Add(Lifetime)); err != nil {
		return c.State(), err
	}
	if _, err := c.adbCmd(ctx, "tcpip", Port); err != nil {
		// adbd may have got the order and the failure be the cut connection or
		// the timeout: the expiry is cleared only when the port is seen NOT to
		// be open. If it cannot be read, the expiry stays (a false "on" is
		// better than a false "off" with the port open); Reconcile clears it
		// at the next start if the port turns out closed.
		if port, perr := c.adbCmd(ctx, "shell", "getprop service.adb.tcp.port"); perr == nil && port != Port {
			_ = c.st.SetUntil(time.Time{})
		}
		return c.State(), err
	}
	return c.State(), nil
}

// Disable closes the port (`adb usb`) and only then clears the expiry: if
// adb fails the mode stays on (and says so) and is tried again.
func (c *Controller) Disable(ctx context.Context) (State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.disable(ctx)
}

func (c *Controller) disable(ctx context.Context) (State, error) {
	if _, err := c.adbCmd(ctx, "usb"); err != nil {
		return c.State(), err
	}
	if err := c.st.SetUntil(time.Time{}); err != nil {
		return c.State(), err
	}
	return State{}, nil
}

// Expire closes the mode when its time has passed. The bool says the state
// changed (it is off now).
func (c *Controller) Expire(ctx context.Context) (State, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	until := c.until()
	if until.IsZero() || until.After(c.now()) {
		return c.State(), false, nil
	}
	s, err := c.disable(ctx)
	return s, err == nil, err
}

// Reconcile is run once when serve starts, with the phone connected. No
// expiry: off. Port 5555 not open: the expiry is stale (the phone restarted)
// and is cleared. Port open with an expiry in the future: kept. Port open
// with the expiry passed: closed now. If the port cannot be read, nothing
// changes (the next start looks again).
func (c *Controller) Reconcile(ctx context.Context) (State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	until := c.until()
	if until.IsZero() {
		return State{}, nil
	}
	port, err := c.adbCmd(ctx, "shell", "getprop service.adb.tcp.port")
	if err != nil {
		return c.State(), err
	}
	switch {
	case port != Port:
		if err := c.st.SetUntil(time.Time{}); err != nil {
			return c.State(), err
		}
		return State{}, nil
	case until.After(c.now()):
		return c.State(), nil
	}
	return c.disable(ctx)
}
```
(`activePhone` y `adb.Runner` ya tienen `Run`; el `fakeADB` del test responde a `shell getprop service.adb.tcp.port` por `Run`.)

- [ ] **Step 6: `invariant.go`.** Cambiar la firma a `applyInvariant(ctx, runner, wait, log, command, keep string)`, pasar `keep` a `device.EnsureNoTCPPort(ctx, runner, keep)` y tras reiniciar el "puerto aún abierto" comprueba `port != "" && port != keep`. Actualizar los 6 usos de `invariant_test.go` con `""` y añadir un caso: `keep == "5555"` con puerto `5555` no ejecuta `usb`. En `serve.go` y `setup.go` pasar `""` por ahora (la tarea 5 pone el valor real en `serve`; `setup` siempre `""`: un `setup` cierra el modo remoto como hoy, y debe decirlo en stderr si había `remote_until` — añadir la línea "Remote mode was on: closing it." y borrar `remote_until` en el `config.Save` que `setup` ya hace).
- [ ] **Step 7:** `go build ./... && go test ./internal/remote ./internal/device ./internal/cli -timeout 5m` → PASA.
- [ ] **Step 8: Commit** `feat(host): controlador del modo remoto y puerto 5555 conservado por el invariante`.

---

### Task 4: `device.info` en el host

**Correcciones de la revisión opus (obligatorias; mandan sobre el texto de esta tarea):**
- **Un solo `cfgStore` (I2).** Se crea en `serve.go` una vez, antes de `findKnownPhone`: `store := newCfgStore(e.dirs.ConfigFile(), &e.cfg)`. `startPhoneLink` pasa a recibir `store *cfgStore` (parámetro nuevo; actualizar sus 4 llamadas de tests) y `chatPort` usa `store.Update` en vez de escribir `e.cfg`/`config.Save` (cubrir con un test: `chat_port` no pisa `tailscale_ip` guardada a la vez). `cfgStore` guarda su propia copia de `config.Config` (`cfg config.Config`, no un puntero compartido) y `serve` lee `e.cfg.Lang`/`ConfirmTimeout` antes de crear el enlace o por `store.Config()`: nunca el puntero mientras otra gorrutina escribe.
- **`remoteLink` se crea siempre** (como `l.ui`), también sin app; sus métodos admiten `srv == nil` (no publican). Se registra en `setAreas` solo con su manejador `TypeRemoteSet` (el de `TypeDeviceInfo` sigue siendo el de `uiLink` con el gancho; M13).
- `phoneapp.ADB` solo tiene `Run` (I3): el puerto se lee con `Run(ctx, "", []string{"shell", "getprop service.adb.tls.port"}, nil, &out, &errb)`, con tope de 5 s.
- Sin `useRemote`: el controlador llega por constructor (Task 5).


**Files:**
- Create: `host/internal/cli/hostconfig.go`, `host/internal/cli/phone_remote.go`, `host/internal/cli/phone_remote_test.go`
- Modify: `host/internal/cli/phone.go` (registrar la zona: `l.setAreas(..., l.remote.area())`; `l.remote` se crea siempre), `host/internal/cli/serve.go` (crear el `cfgStore` y pasarlo), `host/internal/cli/uilink.go:192` (`onDeviceInfo` deja de ser el único que lee `device.info`: ver paso 3)

**Interfaces:**
- Consumes: `config.Config`, `config.Save`, `config.ValidTailscaleIP` (Task 2), `bridge.DeviceInfo`.
- Produces: `cfgStore` con `newCfgStore(path string, cfg config.Config) *cfgStore` (guarda su propia copia), `Update(func(*config.Config)) error` (guarda en disco con el mutex tomado y deshace el cambio en memoria si falla), `TailscaleIP() string`, `Until() time.Time`, `SetUntil(time.Time) error` (implementa `remote.Store`), `Config() config.Config` (copia). `remoteLink` con `newRemoteLink(store *cfgStore, a phoneapp.ADB, log io.Writer) *remoteLink` (el controlador y el servidor se le dan en la Task 5) y `(*remoteLink).area() area`.

- [ ] **Step 1: Tests** (`phone_remote_test.go`; usar el servidor del `phone_ui_test.go` como patrón de cómo enviar `device.info` desde una app falsa y esperar):
  - `device.info` con `tailscaleIp: "100.101.102.103"` y el adb falso respondiendo `getprop service.adb.tls.port` → `41234`: `config.toml` queda con `tailscale_ip` y `tailscale_port = 41234`.
  - IP fuera de `100.64.0.0/10`: no se guarda y se avisa una vez en el log.
  - `tailscaleIp` vacío/ausente (solo `protectedApps`): no cambia nada (ruling 6).
  - `getprop` vacío y `wifiAdbPort: 40000`: usa 40000. Ambos vacíos: guarda la IP y deja el puerto como estaba.
  - Mensaje de una conexión reemplazada (`Server.Current(m)` falso): se ignora.
  - Mismo valor otra vez: no reescribe el fichero (comparar `ModTime` o contar guardados con un `save` inyectable).
  - Dos `device.info` seguidos no se pisan: el segundo espera al primero (`go test` con dos mensajes y comprobar el último).
- [ ] **Step 2:** `go test ./internal/cli -run 'DeviceInfo|CfgStore' -timeout 5m` → FALLA.
- [ ] **Step 3: Implementar.** `hostconfig.go`:

```go
type cfgStore struct {
	mu   sync.Mutex
	path string
	cfg  config.Config // its own copy: nobody else writes the env's config while serve runs
}

func newCfgStore(path string, cfg config.Config) *cfgStore { return &cfgStore{path: path, cfg: cfg} }

func (s *cfgStore) Config() config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// Update applies f and saves; if saving fails the change is undone.
func (s *cfgStore) Update(f func(*config.Config)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cfg
	f(&next)
	if err := config.Save(s.path, next); err != nil {
		return err
	}
	s.cfg = next
	return nil
}

func (s *cfgStore) TailscaleIP() string  { return s.Config().TailscaleIP }
func (s *cfgStore) Until() time.Time     { return s.Config().RemoteUntilTime() }
func (s *cfgStore) SetUntil(t time.Time) error {
	return s.Update(func(c *config.Config) {
		if t.IsZero() {
			c.RemoteUntil = ""
		} else {
			c.RemoteUntil = t.UTC().Format(time.RFC3339)
		}
	})
}
```

`phone_remote.go`: `remoteLink` guarda `store`, `adb`, `log`, `srv remoteServer` (interfaz con `Current(bridge.Message) bool`, `Reply`/`Publish` que usa la Task 5), un `mu` propio, y `warned map[string]bool`. `area()` devuelve los manejadores `bridge.TypeDeviceInfo` y (Task 5) `bridge.TypeRemoteSet`. **Conflicto:** `uilink.area()` ya registra `TypeDeviceInfo` y `setAreas` entra en pánico con tipos duplicados. Solución: `uiLink.onDeviceInfo` pasa a ser el único manejador de `device.info` y llama al final a un gancho `u.deviceInfo func(bridge.Message, bridge.DeviceInfo)` que `startPhoneLink` fija con `l.remote.onDeviceInfo`. Así `device.info` se decodifica una vez, y el `return` temprano de `ProtectedApps == nil` se mueve después de llamar al gancho. Test: un `device.info` con solo `tailscaleIp` llega al gancho y no toca la lista de protegidas, y viceversa.

`onDeviceInfo(m, info)`: si `!srv.Current(m)` sale. Si `info.TailscaleIP == ""` sale (ruling 6). Si `!config.ValidTailscaleIP` avisa una vez y sale. Lanza (en una gorrutina por mensaje, serializada con `mu`) `port := readTLSPort(ctx 5s)`: `a.Shell("getprop service.adb.tls.port")` → entero 1-65535, o `info.WifiADBPort`, o 0. Después `store.Update` solo si algo cambia (IP distinta, o puerto distinto y > 0).
- [ ] **Step 4:** `go test ./internal/cli -timeout 5m` → PASA (todo el paquete: el cambio de `onDeviceInfo` toca los tests de M5a).
- [ ] **Step 5: Commit** `feat(host): el host guarda la IP de Tailscale y el puerto que informa la app`.

---

### Task 5: `remote.set`, `remote.state` y caducidad en `serve` 🔒

**Correcciones de la revisión opus (obligatorias; mandan sobre el texto de esta tarea):**
- **Respuesta (I1):** `remote.state` es `ClassEvent`: `Reply` da `ErrClass`. Responder y publicar son UNA sola llamada, `PublishRID(bridge.TypeRemoteState, m.RID, state)`, como `phone_chat.go:283,313`; sin `Publish` aparte. Los errores se contestan con la forma de error que use `phone_chat.go` para su `rid`. La interfaz `remoteServer` lleva `Current`, `PublishRID` y `Publish`. Un `remote.set` **sin `rid` se descarta** con un aviso (M11): el protocolo lo define como petición.
- **Constructor, sin `useRemote` (I2, I5):** `startPhoneLink(ctx, app, e, store, ctl, a, session, agentName, fallback)`; `serve` crea `ctl := remote.New(runner, store, time.Now)` justo después de encontrar el móvil (con `runner` hasta la Task 6, luego `active`) y antes de `startPhoneLink`, así el primer snapshot y el `attach` ya llevan el estado real. `Reconcile` e invariante van antes de `startPhoneLink`, como hoy el invariante.
- **Snapshot sin locks (M12):** `remoteLink` guarda el último estado en un `atomic.Pointer[remote.State]` que se actualiza tras cada `Enable`/`Disable`/`Expire`/`Reconcile`; `snapshot` solo lo lee (no toca el `cfgStore` con el lock del servidor tomado).
- **Test que cambia (M1):** "dos `remote.set` seguidos" = el segundo no ejecuta otro `tcpip` (Enable idempotente), no "espera".
- **Una expiración que no consigue cerrar** (`Closing`): se avisa una sola vez en stderr ("remote mode expired but port 5555 could not be closed: …; trying again") y el snapshot dice `enabled:true` (la app muestra "Cerrando…"). `remoteTick` se lee antes de lanzar la gorrutina (M2).
- Al salir `serve` con el modo activo: aviso en stderr "Remote mode is still on (port 5555 stays open until HH:MM). Nothing closes it while serve is stopped: run `adbuddy remote off` (it closes the port directly) or start serve again." (M10).
- La función `warnOnce` no existe: usar un `bool` local de la gorrutina.


**Files:**
- Modify: `host/internal/cli/phone_remote.go`, `host/internal/cli/phone_remote_test.go`, `host/internal/cli/serve.go`, `host/internal/cli/phone.go`

**Interfaces:**
- Consumes: `remote.Controller` (Task 3), `cfgStore` (Task 4), `bridge.RemoteSet`, `bridge.RemoteState`, `bridge.TypeRemoteState`, `Server.PublishRID` (éxito) y `Server.Reply(m, bridge.TypeError, bridge.ErrorReply{...})` (errores), como `phone_chat.go:168,283,313`.
- Produces: `remoteLink.ctl *remote.Controller` (por constructor: `startPhoneLink(ctx, app, e, store, ctl, a, session, agentName, fallback)`); `(*remoteLink).publish(remote.State)`; `remoteTick` (variable, 30 s) y la gorrutina `watchRemote(ctx, c, link)` en `serve`; `bridge.RemoteState` en el snapshot.

- [ ] **Step 1: Tests** (con un `remote.Controller` sobre el adb falso):
  - `remote.set {enabled:true}` con IP de Tailscale conocida: responde con `rid` un `remote.state {enabled:true, address:"100.x:5555", expiresAt}` y publica el evento; el adb falso vio `tcpip 5555`.
  - Sin IP: responde `error` con código `unavailable` y el texto "Turn on Tailscale on the phone first"; ninguna orden.
  - `enabled:false`: `usb`, responde `enabled:false`.
  - `usb` falla: responde `error` (`failed`) y el siguiente `snapshot` sigue diciendo `enabled:true` (ruling 4).
  - Mensaje de una conexión reemplazada: se ignora (sin respuesta: no es la actual).
  - Sin `rid` (mensaje suelto): se descarta con un aviso y no se ejecuta nada.
  - El snapshot lleva `Remote` (activo, apagado) y `attach` publica `remote.state` (ruling 3).
  - `Expire` desde el tic: pasa a apagado, publica `remote.state{enabled:false}`.
  - Dos `remote.set {enabled:true}` seguidos: el segundo no ejecuta otro `tcpip` (`Enable` es idempotente) y contesta con el mismo estado.
  - **Todos los caminos responden** (tabla: payload ilegible → `error` `invalid`; sin controlador → `unavailable`; éxito; fallo).
- [ ] **Step 2:** `go test ./internal/cli -run Remote -timeout 5m` → FALLA.
- [ ] **Step 3: Implementar.** `remoteLink` recibe `ctl *remote.Controller` por constructor (nil solo en tests: entonces `unavailable`). `handleSet(m)`: decodifica `bridge.RemoteSet` (error → `Reply` de error `invalid`); sin `rid` → descarta; `Current(m)` falso → sale. Ejecuta en una gorrutina (la petición no debe bloquear el lector del bridge: pendientes de M3) con `ctx` de `serve` y un tope de 30 s: `Enable`/`Disable`; con `ErrNoTailscale` → `unavailable` con ese texto; otro error → `failed` con `classify.Printable(err.Error())`; éxito → `PublishRID(bridge.TypeRemoteState, m.RID, toBridge(state))` (una sola llamada: responde y publica). `snapshot`: `s.Remote = toBridge(ctl.State())` (el `State()` solo lee el store: no llama a `Publish` ni toma el lock del servidor; cumple el contrato de `area.snapshot`). `attach: func(){ publish(ctl.State()) }` en una gorrutina (el comentario de `area.attach` dice que corre con el lock del servidor: no publicar en línea). `toBridge` pone `ExpiresAt` solo si activo.

En `serve.go`, el orden es: `store` (Task 4) → `findKnownPhone` → `runner.Serial = activeSerial` → `ctl` → `Reconcile` → invariante → `startPhoneLink`. El controlador se crea **después** de `runner.Serial = ...` (`remote.New` copia el `adb.Runner`, que es un valor; en la Task 6 recibe `active`):

```go
ctl := remote.New(runner, store, time.Now)   // en la Task 6: remote.New(active, store, time.Now)
```
y justo **antes** de `applyInvariant` y de `startPhoneLink`: `state, err := ctl.Reconcile(ctx)` (un error solo avisa en stderr), y `keep := ""; if state.Enabled { keep = remote.Port }` que se pasa a `applyInvariant`. Si `state.Enabled`, `serve` imprime "Remote mode is on until HH:MM (port 5555 open)". La gorrutina:

```go
go func() {
	t := time.NewTicker(remoteTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if s, changed, err := ctl.Expire(ctx); err != nil {
			warnOnce(...)  // "could not close remote mode: …; trying again"
		} else if changed {
			fmt.Fprintln(app.Stderr, "Remote mode expired: port 5555 closed.")
			link.remote.publish(s)
		}
	}
}()
```
Orden en `serve.go` (ver arriba): `store` → `findKnownPhone` → `runner.Serial` → `ctl` → `Reconcile` → invariante → `startPhoneLink(…, store, ctl, …)`. Hoy `startPhoneLink` ya va después del invariante: solo se añaden `ctl` y `Reconcile` antes.
- [ ] **Step 4:** `go test ./internal/cli -timeout 5m` → PASA.
- [ ] **Step 5: Revisión 🔒 de la tarea** (opus, con `-timeout`): que `remote.set` no pueda llegar de otro origen que la app autenticada y actual, que el 5555 nunca quede abierto sin `remote_until` ni al revés más de un tic, y que ningún camino deje sin responder.
- [ ] **Step 6: Commit** `feat(host): remote.set, remote.state y caducidad del modo remoto en serve`.

---

### Task 6: `activePhone` y serials extra del daemon

**Files:**
- Create: `host/internal/cli/activephone.go`, `host/internal/cli/activephone_test.go`
- Modify: `host/internal/daemon/daemon.go:47,301,314,352`, `host/internal/daemon/daemon_test.go`, `host/internal/cli/serve.go`, `host/internal/cli/phone.go`

**Interfaces:**
- Consumes: `adb.Runner`.
- Produces: 
  - `newActivePhone(r adb.Runner) *activePhone` (con `r.Serial` ya puesto); `(*activePhone).Runner() adb.Runner`; `Serial() string`; `Switch(serial string)`; `Serials() []string` (todos los usados en la sesión, sin repetidos, el actual el primero); y los métodos de `Runner` que usan el daemon, `phoneapp.ADB` y `device.ADB`: `Run`, `Shell`, `Pull`, `Output`.
  - `daemon.Options.Serials func() []string` (serials válidos para `ANDROID_SERIAL`/`-s` del agente además de `Serial` y la identidad), y `Options.CurrentSerial func() string` (lo que cuenta `Ping`; nil: `Serial`).

- [ ] **Step 0: Avisar a tern** (dueño de `daemon.go` y `serve.go` en M3) con `SendMessage` (nombre con `ListAgents`) **antes de empezar**: qué se toca (`Options.Serials` y `Options.CurrentSerial` en `daemon.go`, y en `serve.go` la creación de `activePhone`, la zona remota y el vigilante), que es mínimo y aislado en ficheros propios, y que tern rebasa. Repetir el aviso al empezar la Task 7 y en la Task 5 (reordenación de `serve.go`).
- [ ] **Step 1: Tests de `activePhone`:** `Switch` cambia el serial con el que se ejecuta la orden siguiente (adb falso que registra `-s`); `Serials()` lista el viejo y el nuevo sin duplicados; concurrencia: 50 `Run` en paralelo con 10 `Switch` no provocan carrera ni un `-s` vacío (leer los globales de test antes de lanzar gorrutinas); una orden en curso termina con el serial con el que empezó.
- [ ] **Step 2: Tests del daemon:** con `Serials` que devuelve `["old","new"]` y `Serial: "old"`, un `exec` con `ANDROID_SERIAL=old` y otro con `new` pasan; con `other` se bloquea con "only the active phone can be used"; `Ping` devuelve `CurrentSerial()`.
- [ ] **Step 3: Implementar `activephone.go`:**

```go
// activePhone is the phone serve works with. Its serial changes when the
// phone is found again at another address (M5b spec §7.2), so nobody keeps
// the Runner: they hold the activePhone and every command asks it.
type activePhone struct {
	mu      sync.Mutex
	runner  adb.Runner
	serials []string // every serial used this session, current first
}

func newActivePhone(r adb.Runner) *activePhone {
	return &activePhone{runner: r, serials: []string{r.Serial}}
}

func (p *activePhone) Runner() adb.Runner {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.runner
}

func (p *activePhone) Serial() string { return p.Runner().Serial }

func (p *activePhone) Switch(serial string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if serial == p.runner.Serial {
		return
	}
	p.runner.Serial = serial
	rest := p.serials[:0:0]
	for _, s := range p.serials {
		if s != serial {
			rest = append(rest, s)
		}
	}
	p.serials = append([]string{serial}, rest...)
}

func (p *activePhone) Serials() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.serials...)
}

func (p *activePhone) Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	return p.Runner().Run(ctx, dir, args, stdin, stdout, stderr)
}
func (p *activePhone) Shell(ctx context.Context, c string) (string, int, error) { return p.Runner().Shell(ctx, c) }
func (p *activePhone) Pull(ctx context.Context, remote, local string) error     { return p.Runner().Pull(ctx, remote, local) }
func (p *activePhone) Output(ctx context.Context, args ...string) (string, string, int, error) {
	return p.Runner().Output(ctx, args...)
}
```
- [ ] **Step 4: daemon.** Añadir los dos campos a `Options` (con su comentario), en `exec` cambiar la comprobación del bucle de serials por una función `d.knownSerial(s string) bool` (`Serial`, identidad y `Serials()`), y `Ping` usa `d.currentSerial()`.
- [ ] **Step 5: `serve.go`/`phone.go`.** Crear `active := newActivePhone(runner)` donde hoy se fija `runner.Serial = active` (renombrar la variable local `active` string a `activeSerial` para evitar el choque), pasar `active` a `startPhoneLink` (su parámetro `a phoneapp.ADB` ya es una interfaz; comprobar que `phoneapp.Tunnel/Handshake/InstalledVersion` solo usan `Run`) y a `daemon.Options{ADB: active, Serials: active.Serials, CurrentSerial: active.Serial, Serial: activeSerial,...}`. `Runner` sin serial (host commands) lo sigue construyendo `serve` con `runner` original.
- [ ] **Step 6:** `go build ./... && go test ./internal/... -timeout 5m` → PASA.
- [ ] **Step 7: Commit** `refactor(host): el móvil activo de serve se puede cambiar (activePhone)`.

---

### Task 7: Direcciones candidatas y vigilante de reconexión 🔒

**Correcciones de la revisión opus (obligatorias; mandan sobre el texto de esta tarea):**
- **Identidad (C1, crítico):** el vigilante solo hace `Switch` con `verified == true` (de `connectAddr`/`verifyAt`), también en el camino de "mDNS lo trajo con otro nombre": ese camino llama a `verifyAt` antes de `Switch`. Si la sesión arrancó sin poder verificar (`serial` sin guid, o `pairing.GuidSerialNo` falla), el vigilante **no prueba candidatas**: solo registra que el móvil no está y espera a que vuelva el mismo serial. Tests: guid anunciado por otro móvil (`WRONG_PHONE`: no se cambia, aviso una vez) y guid sin número de serie (no se prueba nada).
- **Quitar el candidato `<IP de casa>:5555`:** `candidateAddrs` solo añade `tailscale_ip:5555` con el modo activo (`Enabled`, aunque `Closing`; C3). El orden de `candidateAddrs` y sus tests se ajustan a eso (el caso "remoto activo añade `ts:5555` y `lan:5555`" pasa a "añade solo `ts:5555`").
- `candidateAddrs(cfg, remoteOn)` y `RemoteOn` se calculan con `store.Until().IsZero() == false` (el controlador aún no existe cuando `serve` llama a `findKnownPhone`; se crea después, Task 5).
- Sin `OnSwitch` (M3): el vigilante solo escribe en el log. `hostRunner` es una copia de `runner` con `Serial = ""` (`serve.go:133` fija el serial en `runner`). `warnOnce` no existe: usar un mapa local. `watchEvery`, `watchMisses` y `watchTryWait` se leen antes de lanzar la gorrutina (M2).
- Antes de empezar: avisar a tern (Step 0 de la Task 6 ya lo hizo; repetir si pasó tiempo).


**Files:**
- Create: `host/internal/cli/phone_watch.go`, `host/internal/cli/phone_watch_test.go`
- Modify: `host/internal/cli/connect.go` (`findKnownPhone`), `host/internal/cli/serve.go`, `host/internal/cli/setup.go:191`, `host/internal/cli/*_test.go` que llaman a `findKnownPhone`

**Interfaces:**
- Consumes: `activePhone` (Task 6), `cfgStore.Config()` (Task 4), `remote.Controller.State()` (Task 3), `pairing.GuidOf`, `connectAddr`, `verifyAt`.
- Produces:
  - `candidateAddrs(cfg config.Config, remoteOn bool) []string` (puro): en orden, `cfg.Address` si no vacío; `tailscale_ip:tailscale_port` si ambos; con `remoteOn`: `tailscale_ip:5555`; sin duplicados.
  - `findKnownPhone(ctx, runner, serial string, addrs []string, wait time.Duration, log io.Writer) (knownPhone, error)` (antes recibía un `addr string`).
  - `watchPhone(ctx, w phoneWatch)` con `phoneWatch{Active *activePhone; Runner adb.Runner; Serial string; Cfg func() config.Config; RemoteOn func() bool; Log io.Writer}` (sin `OnSwitch`); variables `watchEvery = 10 * time.Second`, `watchMisses = 2`, `watchTryWait = 8 * time.Second`.

- [ ] **Step 1: Tests puros de `candidateAddrs`** (tabla): sin nada → vacío; solo `address`; `address` + tailscale; remoto activo añade solo `ts:5555`; duplicados (`address` = `ts:port`) se quitan; remoto apagado no añade 5555; un `address` ilegible no rompe.
- [ ] **Step 2: Tests de `findKnownPhone` con varias direcciones** (reutilizar `addrRules`/`remoteRules` de `remote_test.go`): la primera falla por `connect` y la segunda funciona → `Used` es la segunda; una dirección con otro móvil (`WRONG_PHONE`) se salta con aviso y se prueba la siguiente; ninguna sirve → cae a mDNS y `AddrErr` lleva el último error; `addrs` vacío = comportamiento de hoy. Actualizar los tests existentes a `[]string{addr}`.
- [ ] **Step 3: Tests del vigilante** (adb falso con `devices` programable por llamadas y reloj de espera corto):
  - el serial sigue en `adb devices`: nunca prueba nada;
  - un fallo aislado (1 < `watchMisses`): no hace nada;
  - 2 fallos seguidos y la dirección `ts:5555` responde con la identidad verificada: `Switch(ts:5555)` una vez y aviso en el log;
  - una candidata con otro móvil (`WRONG_PHONE`) no se usa y se avisa una vez;
  - ninguna candidata sirve: reintenta en el siguiente tic, sin avisar más de una vez hasta que vuelva;
  - `ctx` cancelado termina la gorrutina sin dejar `adb connect` colgado (el `adb connect` se hace con `context.WithTimeout(ctx, watchTryWait)`);
  - el móvil vuelve por el mismo serial (mDNS): no cambia nada y el contador de fallos se reinicia.
- [ ] **Step 4:** `go test ./internal/cli -run 'Candidate|KnownPhone|Watch' -timeout 5m` → FALLA.
- [ ] **Step 5: Implementar `findKnownPhone` con `addrs`.** Extraer los pasos 2-3 del cuerpo actual a `tryAddr(ctx, runner, online []adb.Device, addr, guid string, byGuid bool, log io.Writer) (kp knownPhone, ok bool, addrErr error)` y recorrer `addrs` con él; la `AddrErr` que devuelve es la del último intento fallido. `serve.go`: `addrs := candidateAddrs(store.Config(), !store.Until().IsZero())` si `serial == e.cfg.Serial` y `findKnownPhone(ctx, runner, serial, addrs, ...)`; `setup.go` pasa `known` como `[]string{known}` si no está vacío. Nota: `serve` solo necesita `store` (Task 4) antes de `findKnownPhone`; el controlador se crea después (Task 5), así que `candidateAddrs` usa `!store.Until().IsZero()` como "remoto activo" (aunque haya caducado: `Closing` cuenta como activo). El `Reconcile` sigue después de encontrar el móvil.

- [ ] **Step 6: Implementar `phone_watch.go`.**

```go
func candidateAddrs(cfg config.Config, remoteOn bool) []string {
	var out []string
	add := func(a string) {
		if a == "" || slices.Contains(out, a) {
			return
		}
		out = append(out, a)
	}
	add(cfg.Address)
	if cfg.TailscaleIP != "" && cfg.TailscalePort != 0 {
		add(net.JoinHostPort(cfg.TailscaleIP, strconv.Itoa(cfg.TailscalePort)))
	}
	if remoteOn {
		if cfg.TailscaleIP != "" {
			add(net.JoinHostPort(cfg.TailscaleIP, remote.Port))
		}
	}
	return out
}
```
`watchPhone`: ticker `watchEvery`; en cada tic `Runner.Online(ctx)` (host command: el `Runner` del watcher no lleva serial); si `hasSerial(online, Active.Serial())` → `misses = 0`, continuar; si no, `misses++`; con `misses >= watchMisses`: buscar por guid en `online` (`pairing.IsGuidSerial`: mDNS lo trajo con otro nombre) → `verifyAt` y solo con `verified == true`, `Switch`; si no, `for addr in candidateAddrs(Cfg(), RemoteOn())` → `connectAddr(tctx, Runner, addr, guid, watchTryWait)`; el primero que devuelva `verified == true` → `Switch(active)`, `misses = 0`, log "📱 The phone is now at <addr>."; un candidato con `verified == false` se desconecta (`adb disconnect`) y no se usa. `WRONG_PHONE` avisa una vez por dirección. Nunca llama a `applyInvariant` (no tocar el 5555 mientras se reconecta). Fallo de todas: `warned` una sola vez ("The phone is not reachable (tried …): waiting"), y un aviso de "reachable again" cuando vuelve.
`serve.go`: `go watchPhone(ctx, phoneWatch{Active: active, Runner: hostRunner, Serial: serial, Cfg: store.Config, RemoteOn: func() bool { return !store.Until().IsZero() }, Log: app.Stderr})` arrancado después del daemon.
- [ ] **Step 7:** `go build ./... && go test ./internal/cli -timeout 5m` → PASA.
- [ ] **Step 8: Revisión 🔒** (opus): identidad verificada antes de `Switch` en todos los caminos (nunca se cambia a un móvil sin comprobar su guid cuando hay guid), `Switch` no deja órdenes del agente ejecutándose contra otro móvil, y el vigilante no abre ni cierra ningún puerto.
- [ ] **Step 9: Commit** `feat(host): serve reconecta con el móvil por Tailscale o el 5555 si cambia su dirección`.

---

### Task 8: `adbuddy remote on|off|status`

**Correcciones de la revisión opus (obligatorias; mandan sobre el texto de esta tarea):**
- **`on` por IPC pide confirmación (I11):** abrir el 5555 12 h en redes públicas no puede decidirlo un agente que borre `ADBUDDY_SOURCE`. `handleRemote("on")` pregunta al usuario con el mismo `confirm.Confirmer` que usan las 🔴 (el del enlace: móvil o terminal de `serve`), con título propio "Open adb's network port 5555 for 12 hours" / "Abrir el puerto de red 5555 de adb durante 12 horas"; denegar o caducar da `REMOTE_DENIED` (salida 126) y no ejecuta nada. La petición al `confirm.Confirmer` (`confirm/confirm.go:58`, el del enlace: `link.Confirmer()`) lleva **exactamente**: `Key: ""` y `Special: true` (si no, la `Key` de regla+cwd+argv activaría "permitir durante la sesión", `bridge/confirm.go:57-62`, y un agente reabriría el 5555 sin preguntar después de que el usuario lo cierre); `ID: journal.NewID("r_", now)` (con `ID` vacío el confirmador deniega); un `context.WithTimeout(ctx, ConfirmTimeout)` con su `ExpiresAt`; el título en el idioma del usuario (`Lang`); y `l.Paused()` verdadero deniega sin preguntar. Test con un confirmador falso: acepta, deniega, caduca, en pausa, y que la segunda petición vuelve a preguntar aunque la primera se aceptase. `off` y `status` no preguntan. El interruptor de la app (`remote.set`) NO pasa por esta confirmación: es el propio usuario con la app autenticada, y el diálogo de la app ya lleva el aviso. Apuntar en `pendientes` que la defensa real es la identidad del par del socket.
- **`off` sin `serve`:** ejecuta `adb usb` directamente con el adb configurado y borra `remote_until` (si `usb` falla, lo dice y no borra). `on` sin `serve` sigue siendo `DAEMON_NOT_RUNNING`.
- El texto para `AGENTS.md` (Task 15) avisa de que una orden guardada va sola: `dumpsys activity top | grep …` se bloquea con "must run alone" (M16).


**Files:**
- Create: `host/internal/cli/remotecmd.go`, `host/internal/cli/remotecmd_test.go`
- Modify: `host/internal/ipc/ipc.go` (`KindRemote`), `host/internal/cli/serve.go` (enrutar el kind), `host/internal/cli/root.go`

**Interfaces:**
- Consumes: `remote.Controller`, `remoteLink.publish` (Task 5), `ipc.Call`.
- Produces: `ipc.KindRemote = "remote"`; `newRemoteCmd(app *App) *cobra.Command`; `(*phoneLink).handleRemote(ctx, req ipc.Request, c *ipc.Conn) ipc.Exit`.

- [ ] **Step 1: Tests:** desde la sesión del agente (`ADBUDDY_SOURCE=agent`) los tres subcomandos dan `AGENT_SESSION` sin llamar al socket; el servidor IPC rechaza `KindRemote` con `Env["ADBUDDY_SOURCE"]=="agent"` (`AGENT_SESSION`, salida 1); `on` sin `serve` → `DAEMON_NOT_RUNNING`; `status` sin `serve` lee `config.toml` y dice "serve is not running; remote mode: on until … / off"; con `serve` (servidor IPC de test): `on` imprime la dirección y la caducidad y publica `remote.state`; `off` cierra; `--json` usa el sobre habitual; `status` con modo caducado dice apagado; `on` sin Tailscale da `REMOTE_UNAVAILABLE` con "Turn on Tailscale on the phone first" (salida 1).
- [ ] **Step 2:** FALLA. 
- [ ] **Step 3: Implementar.** `remote` tiene `ExactArgs(1)` con `ValidArgs: on|off|status`, comprueba `ADBUDDY_SOURCE` antes de nada y manda `ipc.Request{Kind: ipc.KindRemote, Argv: []string{arg}, Env: {"ADBUDDY_SOURCE": getenv}}` por `ipc.Call` (copiar la forma de `undo.go`). `handleRemote` en `serve`: igual que `handleSend`/`handleSession`, `if req.Env["ADBUDDY_SOURCE"] == "agent" { return agentSessionExit }`, ejecuta `ctl.Enable/Disable/State`, escribe el `remote.State` con `c.Data(...)` y publica por el enlace si hay app. Enrutar en el `ipc.Listen` de `serve.go` junto a `KindSend`. Texto: `Remote mode is on until 21:14 (about 12 h). Reach the phone at 100.64.0.5:5555. It stays open on any network until then: turn it off with `adbuddy remote off` when you are home.`
- [ ] **Step 4:** `go test ./internal/cli ./internal/ipc -timeout 5m` → PASA.
- [ ] **Step 5: Commit** `feat(host): adbuddy remote on|off|status`.

---

### Task 9: Capturas por defecto en modo remoto

**Correcciones de la revisión opus (obligatorias; mandan sobre el texto de esta tarea):**
- **Diario (M4):** los valores por defecto (escala 0,5, formato PNG o JPEG) se resuelven en `ui` **antes** de construir `helperArgv` y la descripción de la entrada, para que el diario no apunte `--scale 0 --format ""`.


**Files:**
- Modify: `host/internal/daemon/ui.go:231-239` (+ la resolución antes de ejecutar), `host/internal/daemon/daemon.go` (`Options.RemoteActive func() bool`), `host/internal/cli/helpers.go:286-345`, `host/internal/cli/serve.go`, tests de ambos.

**Interfaces:**
- Consumes: `ctl.State().Enabled` (Task 3).
- Produces: `ScreenshotArgs{Scale float64; Format string}` donde `Scale == 0` y `Format == ""` significan "el valor por defecto"; `ScreenshotResult.Format` dice lo que se hizo.

- [ ] **Step 1: Tests del daemon:** `Scale 0, Format ""` sin modo remoto → 0,5 PNG; con `RemoteActive() == true` → 0,5 JPEG; `Format: "png"` explícito con modo remoto → PNG; `Scale: 1` explícito se respeta; escala negativa o >1 sigue siendo `UI_INVALID`; formato desconocido sigue siendo `UI_INVALID`. **Tests de la CLI:** sin `--scale` ni `--format` envía ceros; con `--scale 0.3` envía 0.3; el nombre por defecto usa la extensión del `ScreenshotResult.Format` que devuelve el daemon (`.jpg` si es JPEG), no la que se pidió; `-o x.png` sin `--format` sigue pidiendo PNG.
- [ ] **Step 2:** FALLA.
- [ ] **Step 3: Implementar.** En `checkUIArgs` (case `OpScreenshot`): válido si `(a.Scale == 0 || 0 < a.Scale && a.Scale <= 1) && (a.Format == "" || png || jpeg)`. Donde se ejecuta la captura: `if a.Scale == 0 { a.Scale = 0.5 }; if a.Format == "" { a.Format = screenshot.PNG; if d.o.RemoteActive != nil && d.o.RemoteActive() { a.Format = screenshot.JPEG } }`. CLI: `--scale` con valor por defecto `0` (la ayuda dice "default 0.5; 0.5 and JPEG in remote mode"); si `out == ""` la ruta se calcula **después** de la respuesta, con `r.Format`; si `-o` trae `.jpg`/`.jpeg` y `--format` está vacío, pide JPEG como hoy; si `-o` termina en `.png` y no hay `--format`, pide PNG. `serve.go`: `RemoteActive: func() bool { return ctl.State().Enabled }`.
- [ ] **Step 4:** `go test ./internal/daemon ./internal/cli -timeout 5m` → PASA.
- [ ] **Step 5: Commit** `feat(host): en modo remoto la captura por defecto es JPEG a media escala`.

---

### Task 10: La persiana desplegada cuenta como protegida 🔒

**Correcciones de la revisión opus (obligatorias; mandan sobre el texto de esta tarea):**
- **Pantalla apagada (I6):** en Android 12+ la pantalla de bloqueo vive en `NotificationShade`; con `mWakefulness=Asleep|Dozing` (`uidump.Asleep`, ya usado por la guarda) NO se aplica la regla de la persiana, para que `input keyevent 224`/`26` siga pasando. Test: foco `NotificationShade` + `Asleep` → 224 pasa; foco `NotificationShade` + pantalla encendida → 224 bloquea.
- **Varios focos (M5):** `uidump.ParseFocus` devuelve **todos** los `mCurrentFocus` (uno por pantalla) como `[]string`; basta que uno sea la persiana. `IsShade(focus)` compara el título (`NotificationShade`, `StatusBar`) sin exigir el paquete, que esa línea no trae.
- **Teclas exactas (M5):** la excepción es `input keyevent` con exactamente una tecla `3`, `4`, `KEYCODE_HOME` o `KEYCODE_BACK`, y ninguna opción (`--longpress`, `--duration`…): `input keyevent --longpress 3` abre el asistente y bloquea.


**Files:**
- Modify: `host/internal/daemon/guard.go`, `host/internal/daemon/guard_test.go`, `host/internal/uidump/` (parser de `dumpsys window`), `host/internal/testutil/` si hace falta una muestra, `docs/superpowers/specs/...-m5-ayudas-remoto-design.md` (solo si el comportamiento final difiere del texto de la Task 1)
- Create: `host/internal/uidump/window.go`, `host/internal/uidump/window_test.go`, `host/internal/uidump/testdata/window_shade.txt`, `window_app.txt`, `window_unreadable.txt`

**Interfaces:**
- Consumes: la guarda de §4.3 (`guard.go`: mirar cómo `foreground` y `screenReads` deciden), `d.o.ADB.Shell`.
- Produces: `uidump.ParseFocus(out string) (focus string, ok bool)` (lee `mCurrentFocus=Window{… u0 <paquete/actividad>}`; `ok` falso si no hay línea o no se entiende) y `uidump.IsShade(focus string) bool` (`NotificationShade` o `StatusBar`, con el paquete `com.android.systemui`).

- [ ] **Step 1: Muestras reales.** Las muestras de `testdata` salen de `dumpsys window | grep -E 'mCurrentFocus|mFocusedApp'` grabadas **solo con el emulador** (`scripts/emulator.sh`) y nunca del móvil del usuario: si el emulador no sirve (pendientes de M2.5b), el implementador escribe las muestras a mano con el formato conocido (`  mCurrentFocus=Window{4b0f3a2 u0 NotificationShade}` y `  mCurrentFocus=Window{1a2b3c u0 com.android.settings/com.android.settings.Settings}`) y lo apunta como validación 14 (formato en Android 17).
- [ ] **Step 2: Tests del parser** (muestras de arriba, más: sin línea → `ok` falso; `mCurrentFocus=null` → `ok` verdadero con foco vacío, que NO es persiana; basura → `ok` falso).
- [ ] **Step 3: Tests de la guarda:** con foco `NotificationShade`: bloquean `screencap`, `uiautomator dump`, `input tap`, `input swipe`, `am start -n com.x/.Y`, y las ayudas `screen`, `tap`, `type` y `screenshot` ("`Blocked by ADBuddy: the notification shade is open; …`", salida 126); **no** bloquean `input keyevent 4`, `input keyevent KEYCODE_BACK`, `input keyevent 3`, `input keyevent KEYCODE_HOME`; sí bloquean `input keyevent 66`; con `dumpsys window` ilegible (`ok` falso) bloquea (falla cerrado) salvo que la pantalla esté apagada (`mWakefulness=Asleep|Dozing`, mismo criterio que el primer plano); con foco de una app normal no cambia nada de lo que ya hace la guarda; el orden pausa → pregunta → guarda no cambia; y `dumpsys window` se consulta **una vez por orden** junto con el primer plano (contar las llamadas del adb falso).
- [ ] **Step 4:** FALLA. 
- [ ] **Step 5: Implementar.** En `guard.go`, tras decidir el primer plano y antes de dejar pasar, `shade, known := d.shadeOpen(ctx)` (`d.o.ADB.Shell(ctx, "dumpsys window | grep -E 'mCurrentFocus'")` — el `grep` va dentro del `shell` del móvil, es una orden fija nuestra, no del agente); si `!known` y la pantalla no está apagada → bloquear; si `shade` y la orden no es una de las teclas de cierre → bloquear. Reutilizar la función que ya valida "una sola tecla" de §4.3 punto 0 para reconocer `keyevent 3|4|KEYCODE_HOME|KEYCODE_BACK`. Para las ayudas, la guarda de la app (`foreground`) no cambia: la comprobación del host cubre `screen`, `tap`, `type`, `screenshot`.
- [ ] **Step 5b: Heads-up sin app (ok de e5).** Cuando la guarda no tiene app (respaldo por adb) lee además `dumpsys window windows` y busca ventanas de `com.android.systemui` visibles (`mViewVisibility=0x0`) cuyo título no esté en `fixedSystemUI` (`StatusBar`, `NavigationBar0`, `NavigationBar1`, `ScreenDecorOverlay`, `ScreenDecorOverlayBottom`); una ventana de `systemui` visible que no esté en la lista, o una salida que no se entiende (`uidump.ParseSystemUIWindows` devuelve `ok` falso), bloquea. Con app conectada la señal la da `foreground` (Task 10c) y esta consulta no se hace (no añadir otros 300 ms). Tests: muestra con `StatusBar` y `NavigationBar0` visibles (no bloquea), con una ventana desconocida de systemui visible (bloquea), basura (bloquea), sin systemui (no bloquea). Las muestras son a mano (validación 17).
- [ ] **Step 6:** `go test ./internal/daemon ./internal/uidump -timeout 5m` → PASA; el clasificador no cambia en esta tarea (lo hace la 10b).
- [ ] **Step 7: Revisión 🔒** (opus): que no haya forma de que la orden elegida esquive la excepción (`input keyevent 4 ; input tap …` ya es "must run alone"), que el `shell` fijo no incluya texto del agente, y que fallar cerrado no deje al agente sin poder encender la pantalla.
- [ ] **Step 8: Commit** `feat(host): la persiana desplegada cuenta como app protegida para la guarda`.

---

### Task 10b: `screenrecord`, `dumpsys activity` y `dumpsys notification` bajo la guarda 🔒

**Correcciones de la revisión opus (obligatorias; mandan sobre el texto de esta tarea):**
- **Opciones de `dumpsys` (I7):** `checkDumpsys` busca `--noredact` en **cualquier** posición de un `dumpsys` con `notification` (`dumpsys -t 10 notification --noredact` → 🔴). Para saber el servicio se saltan las opciones propias de `dumpsys` (`-t N`, `--pri`/`--priority X`, `--proto`, `--pid`, `--skip X`, `--help`). Si el primer argumento empieza por `-` y no se reconoce, `screenAction` lo trata como lectura guardada (lista blanca). Tests: `dumpsys -t 10 activity top` bloqueado en app protegida, `dumpsys -t 10 notification --noredact` 🔴, `dumpsys --proto activity top` bloqueado.
- **`adb bugreport` (I8):** 🔴 **especial** (en `special.go`, no en `adb.yaml`: una regla del yaml la puede bajar el usuario, `pcwrite_test.go:167`): su informe incluye `dumpsys activity` con árbol de vistas y texto, y `notification`. Sin verificarlo en el emulador: 🔴 por prudencia y la spec lo dice (Task 1). Su `PCWrite` se sigue rellenando.
- **`dumpsys` sin servicio o con `--skip`:** vuelca todos los servicios (también `activity top` y `notification`): `checkDumpsys` lo marca 🔴 si no hay servicio (`dumpsys` solo, o solo opciones) salvo `dumpsys -l`/`--help`; con `--skip X` y sin servicio, igual. Tests: `dumpsys` → 🔴; `dumpsys -l` → 🟢; `dumpsys --skip meminfo` → 🔴; `dumpsys battery` → 🟢. Comprobar que `habitual.jsonl` no tiene un `dumpsys` solo (sus líneas 5-12 no lo tienen).
- Si `testdata/habitual.jsonl` contiene `bugreport` o `dumpsys notification --noredact` entre lo que "no puede subir", parar y avisar a e5.


**Files:**
- Modify: `host/internal/classify/checks_read.go` (`checkDumpsys`), `host/internal/classify/special.go` y `rules.go` (reglas `screen.record` y `adb.bugreport`), `host/internal/daemon/guard.go` (`screenAction`), sus tests, y los que cambian de resultado: `classify/hardening_test.go:94` (`bugreport` pasa de 🟢 a 🔴) y `daemon/pcwrite_test.go:49-50,72,74,95` (los `bugreport` ya no se ejecutan sin preguntar: ajustar o pasar a un caso que no use `bugreport`, y cubrir `pull` por separado); `rules/adb.yaml:22` no se toca

**Interfaces:**
- Consumes: `classify.Finding`, `screenAction`.
- Produces: `screenrecord` → 🔴 especial con regla `screen.record` (título "Record the screen" / "Grabar la pantalla"); `dumpsys notification` con `--noredact` → 🔴; `screenAction` devuelve `reads = true` para `dumpsys activity <arg>` con `<arg>` fuera de `{activities, a, services, providers, broadcasts, processes}` (incluye `top`).

- [ ] **Step 1: Tests del clasificador** (en la tabla `orden → nivel` que ya existe): `screenrecord /sdcard/x.mp4` y `screenrecord --time-limit 3 /sdcard/x.mp4` → 🔴 especial; `dumpsys notification --noredact` → 🔴 y `dumpsys notification` → 🟢; `dumpsys window windows` → 🟢; `dumpsys activity top` sigue 🟢 como nivel (lo bloquea la guarda, no el nivel).
- [ ] **Step 2: Tests de la guarda:** con una app protegida en primer plano, `dumpsys activity top` y `dumpsys activity com.bank/.Main` se bloquean con el mensaje de app protegida; con una app normal pasan; `dumpsys activity activities` y `dumpsys activity services` pasan siempre; `dumpsys window` pasa.
- [ ] **Step 3:** `go test ./internal/classify ./internal/daemon -timeout 5m` → FALLA. Implementar: `screenrecord` como regla especial (copiar la forma de la regla especial más cercana de `classify/special.go`); en `checkDumpsys`, si `ws[1].Value == "notification"` y algún argumento posterior empieza por `--noredact`, `red("dumpsys notification text of every app")`; en `screenAction` (`daemon/guard.go`), `cmd == "dumpsys"` con `Words[1] == "activity"` y `Words[2]` fuera de la lista segura da `reads = true`. `screenrecord` se queda en `screenReads` por si una regla de usuario lo baja de nivel.
- [ ] **Step 4: Contrato del clasificador.** `testdata/habitual.jsonl` no puede subir de nivel con estos cambios: `grep -n 'screenrecord\|dumpsys' host/internal/classify/testdata/habitual.jsonl`; si hay algún caso afectado, parar y avisar a e5. `differential.jsonl` y `oracle.jsonl` solo cubren cómo se parsea el shell de mksh, no el nivel de una orden: no hay que regrabar el oráculo; si el generador lo pidiera por una orden nueva, no regrabar y avisar. Ejecutar `go test ./internal/classify -timeout 5m` entero.
- [ ] **Step 5:** `go test ./internal/classify ./internal/daemon -timeout 5m` → PASA.
- [ ] **Step 6: Revisión 🔒** (opus, junto con la de la Task 10): `screenrecord` por `sh -c`, `nohup`, `busybox` o en segundo plano sigue siendo 🔴 (ya lo son las envolturas y los trabajos en segundo plano: un test lo prueba), y `dumpsys activity` con un argumento nuevo de una versión futura de Android falla cerrado (la lista segura es una lista blanca).
- [ ] **Step 7: Commit** `feat(host): screenrecord y dumpsys notification --noredact piden confirmación; dumpsys activity top pasa por la guarda`.

---

### Task 10c: App · las notificaciones emergentes cuentan como protegidas 8 s 🔒

**Correcciones de la revisión opus (obligatorias; mandan sobre el texto de esta tarea):**
- **Ficheros (M6):** añadir `.../a11y/UiHelpers.kt` (donde se construye `UiExecutor`: pasar la `NotificationWindow` desde el servicio).
- **Toasts (M6):** los toasts también emiten `TYPE_NOTIFICATION_STATE_CHANGED`; `AgentAccessibilityService` ignora los eventos cuyo `className` contiene `Toast` y los que no traen `Notification` en `parcelableData` (test con un `onAccessibilityEvent` lógico extraído a `NotificationWindow.accept(className, pkg, isNotification)`).
- Con el ajuste de accesibilidad "tiempo para actuar" un heads-up dura más de 8 s: queda como validación 17 (medir) y, si hace falta, usar `AccessibilityManager.getRecommendedTimeoutMillis` en una versión posterior.


**Files:**
- Create: `android/app/src/main/kotlin/tech/hybriq/adbuddy/a11y/NotificationWindow.kt`, `android/app/src/test/kotlin/tech/hybriq/adbuddy/a11y/NotificationWindowTest.kt`
- Modify: `.../a11y/AgentAccessibilityService.kt` (recibir el evento), `.../a11y/UiExecutor.kt` (`foreground`, `dump`, `click`, `setText`), `android/app/src/main/res/xml/accessibility_service.xml` (`typeNotificationStateChanged`), `.../test/.../UiExecutorTest.kt`

**Interfaces:**
- Produces: `NotificationWindow(clock: () -> Long, windowMs: Long = 8_000)` con `onNotification(pkg: String)` (ignora `tech.hybriq.adbuddy` y paquetes vacíos) y `active(): Boolean`.

- [ ] **Step 1: Tests:** sin eventos → `active` falso; tras un evento de otro paquete → verdadero hasta `windowMs` y falso al llegar a `windowMs` (reloj falso); un evento de `tech.hybriq.adbuddy` o de paquete vacío no activa; un segundo evento reinicia la ventana. `UiExecutorTest`: con `active()` verdadero, `foreground` devuelve `protected: true` y `dump` devuelve el error `protected` ("A notification is on screen: ask the user to do this part"); con la ventana pasada, vuelve a lo normal; `click` y `setText` también dan `protected` mientras dure (misma segunda capa que la interfaz del sistema con pregunta pendiente).
- [ ] **Step 2:** `./gradlew :app:testDebugUnitTest --tests '*NotificationWindowTest*' --tests '*UiExecutorTest*'` → FALLA.
- [ ] **Step 3: Implementar.**

```kotlin
/** A notification just posted may be on screen as a heads-up over any app, with a 2FA code in it: for a few seconds the helpers treat the screen as protected (M5b plan, ruling 7). */
class NotificationWindow(private val clock: () -> Long, private val windowMs: Long = 8_000) {
    @Volatile private var until = 0L

    fun onNotification(pkg: String) {
        if (pkg.isEmpty() || pkg == ADBUDDY) return
        until = clock() + windowMs
    }

    fun active(): Boolean = clock() < until

    private companion object { const val ADBUDDY = "tech.hybriq.adbuddy" }
}
```
`AgentAccessibilityService.onAccessibilityEvent`: si `event.eventType == AccessibilityEvent.TYPE_NOTIFICATION_STATE_CHANGED`, `notifications.onNotification(event.packageName?.toString().orEmpty())`; `accessibility_service.xml`: añadir `typeNotificationStateChanged` a `accessibilityEventTypes`; el reloj es `SystemClock.elapsedRealtime`. Con el servicio recién vinculado no hay ventana: no se supone nada (el host cubre el hueco con la persiana).
- [ ] **Step 4:** `./gradlew :app:testDebugUnitTest` → PASA.
- [ ] **Step 5: Commit** `feat(app): una notificación reciente cuenta como pantalla protegida 8 s`.

---

### Task 11: App · estado de tres valores y `RemoteReducer`

**Correcciones de la revisión opus (obligatorias; mandan sobre el texto de esta tarea):**
- **No cambiar `Snapshot.remote` (I10):** sigue siendo `JsonElement? = null` (protocolo §4.3, por áreas: una parte mal formada no deja ilegible el snapshot entero con sus preguntas 🔴). `RemoteReducer.snapshot` decodifica con `try/catch`: `snap.remote?.let { runCatching { Protocol.json.decodeFromJsonElement(RemoteState.serializer(), it) }.getOrNull() }` → `null` (desconocido) si falta o no se lee. Esta tarea NO toca `Protocol.kt` ni `protocol/`.
- Test añadido: un snapshot con `remote` mal formado (`"remote":"x"`) deja `remote = null` y conserva `pending`.


**Files:**
- Modify: `android/app/src/main/kotlin/tech/hybriq/adbuddy/bridge/BridgeState.kt:18`, `.../bridge/RemoteReducer.kt`, (`Protocol.kt` y `ProtocolUi.kt` NO se tocan)
- Test: `android/app/src/test/kotlin/tech/hybriq/adbuddy/bridge/BridgeStateTest.kt` (casos nuevos), `ProtocolTest.kt` si el cambio de `Snapshot` lo exige

**Interfaces:**
- Produces: `BridgeState.remote: RemoteState?` (`null` = desconocido). `Snapshot.remote` sigue siendo `JsonElement? = null`.

- [ ] **Step 1: Tests que fallan** (en `BridgeStateTest.kt`, con los helpers de mensajes que ya usa):
  - estado inicial: `remote == null`;
  - `remote.state {enabled:true,address:"100.64.0.5:5555",expiresAt:...}` → `remote` es ese estado (con `seq`);
  - un `remote.state` repetido (mismo `seq`) no cambia nada;
  - snapshot con `remote` → lo fija; snapshot sin `remote` (`null`) → **desconocido**;
  - `reset(NEW_SESSION)` y `reset(UNREADABLE_SNAPSHOT)` → `null` (ya no `RemoteState()`);
  - `DISCONNECTED` y `INCOMPATIBLE` conservan lo que había (desconocido sigue desconocido; activo sigue activo: la desconexión por el reinicio de adbd es lo esperado);
  - un `remote.state` con payload ilegible no lanza y deja `remote = null` (ruling: ningún reductor lanza).
- [ ] **Step 2:** `./gradlew :app:testDebugUnitTest --tests '*BridgeStateTest*'` → FALLA.
- [ ] **Step 3: Implementar.**

```kotlin
object RemoteReducer : Reducer {
    override val types = setOf(Protocol.REMOTE_STATE)

    // A payload this app cannot read is "unknown", never "off": a false off would
    // hide an open port 5555.
    override fun event(s: BridgeState, m: Message): BridgeState =
        s.copy(remote = Protocol.payload<RemoteState>(m))

    // Per area (protocol §4.3): a malformed remote part must not make the whole snapshot unreadable.
    override fun snapshot(s: BridgeState, snap: Snapshot): BridgeState = s.copy(
        remote = snap.remote?.let {
            runCatching { Protocol.json.decodeFromJsonElement(RemoteState.serializer(), it) }.getOrNull()
        },
    )

    override fun reset(s: BridgeState, why: Reset): BridgeState = when (why) {
        Reset.NEW_SESSION, Reset.UNREADABLE_SNAPSHOT -> s.copy(remote = null)
        Reset.DISCONNECTED, Reset.INCOMPATIBLE -> s
    }
}
```
`BridgeState`: `val remote: RemoteState? = null` (cambiar el comentario: "null = unknown"). Buscar todos los usos de `state.remote` (`grep -rn "\.remote" android/app/src`) y adaptarlos.
- [ ] **Step 4:** `./gradlew :app:testDebugUnitTest` → PASA.
- [ ] **Step 5: Commit** `feat(app): RemoteReducer con estado de tres valores`.

---

### Task 12: App · IP de Tailscale y `device.info` al cambiar la red

**Correcciones de la revisión opus (obligatorias; mandan sobre el texto de esta tarea):**
- **VPN (I9):** `NetworkRequest.Builder()` trae `NET_CAPABILITY_NOT_VPN` por defecto y con `addTransportType(TRANSPORT_VPN)` no coincidiría con ninguna red: `NetworkRequest.Builder().addTransportType(NetworkCapabilities.TRANSPORT_VPN).removeCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN).build()`. La validación 16 lo comprueba en el Pixel.


**Files:**
- Create: `android/app/src/main/kotlin/tech/hybriq/adbuddy/net/TailscaleIp.kt`, `.../net/DeviceInfoSender.kt`, `android/app/src/test/kotlin/tech/hybriq/adbuddy/net/TailscaleIpTest.kt`
- Modify: `.../a11y/UiHelpers.kt` (para no pisar `protectedApps` con otro `DeviceInfo`: ver abajo), `.../AdbuddyApp.kt` (arrancar el emisor), `AndroidManifest.xml` si falta `ACCESS_NETWORK_STATE`

**Interfaces:**
- Produces: `TailscaleIp.pick(addresses: List<String>): String` (la primera IPv4 de `100.64.0.0/10`, o `""`), `DeviceInfoSender(app: AdbuddyApp)` con `start()`/`stop()`.

- [ ] **Step 1: Tests de `TailscaleIp.pick`:** `["198.51.100.4","100.101.102.103"]` → la segunda; `["fd7a:115c:a1e0::1"]` → `""` (solo IPv4); `["100.63.255.255","100.128.0.0"]` → `""` (fuera de la franja); `["100.64.0.0","100.127.255.255"]` → la primera (límites incluidos); `["100.101.102.103","100.100.100.100"]` → la primera de la lista; entradas no válidas (`"abc"`, `""`, `"100.64.0"`, `"100.64.0.256"`) se ignoran sin lanzar.
- [ ] **Step 2:** FALLA. 
- [ ] **Step 3: Implementar `pick`** con aritmética de enteros sin `InetAddress` (testeable en JVM):

```kotlin
object TailscaleIp {
    /** First IPv4 address in 100.64.0.0/10 (Tailscale's CGNAT range), or "". */
    fun pick(addresses: List<String>): String = addresses.firstOrNull { inRange(it) } ?: ""

    private fun inRange(s: String): Boolean {
        val parts = s.split('.')
        if (parts.size != 4) return false
        val n = parts.map { it.toIntOrNull()?.takeIf { v -> v in 0..255 } ?: return false }
        return n[0] == 100 && n[1] in 64..127
    }
}
```
- [ ] **Step 4: `DeviceInfoSender`.** Registra un `ConnectivityManager.NetworkCallback` (`registerDefaultNetworkCallback` no sirve: la VPN no es la red por defecto de datos; usar `registerNetworkCallback(NetworkRequest.Builder().addTransportType(TRANSPORT_VPN).build(), cb)`), y en `onAvailable`/`onLinkPropertiesChanged` lee `linkProperties.linkAddresses`, llama a `TailscaleIp.pick` y, si cambió o la app acaba de conectar (`bridge.listen(Protocol.WELCOME)`), envía `Bridge.send(DEVICE_INFO, DeviceInfo(tailscaleIp = ip))`. Con `ip == ""` no envía (ruling 6). `wifiAdbPort` queda en 0 (no hay API pública; la lee el host). **No pisa `protectedApps`**: `DeviceInfo` con `protectedApps = null` es "sin cambios" en el host, así que ambos emisores pueden enviar mensajes distintos. Comprobar que `DeviceInfo.protectedApps` por defecto `null` se serializa **ausente** (el `Json` de `Protocol` usa `explicitNulls=false`; añadir un test de `ProtocolTest` si no existe). Se arranca desde `AdbuddyApp.onCreate` y se para en `onTerminate`; capturar `SecurityException` y `RuntimeException` (sin permiso o Android sin VPN): nunca romper el arranque.
- [ ] **Step 5:** `./gradlew :app:testDebugUnitTest :app:assembleDebug` → PASA (`assembleDebug` solo si el JDK/SDK del PC lo permite; si no, dejarlo a la CI y decirlo en el ledger).
- [ ] **Step 6: Commit** `feat(app): la app informa de su IP de Tailscale al host`.

---

### Task 13: App · interruptor del modo remoto

**Correcciones de la revisión opus (obligatorias; mandan sobre el texto de esta tarea):**
- **`Closing` y desconocido (C3, M8, M15):** `RemoteState` del protocolo no cambia, así que "cerrando" se deduce igual que el host: `enabled=true` con `expiresAt` ya pasado (reloj del host) → `Kind.CLOSING` ("Closing… the PC is trying to close port 5555" / "Cerrando… el PC intenta cerrar el puerto 5555"), nunca `OFF`. Tras una respuesta `null` a `remote.set` (el túnel cayó por el reinicio de adbd) la tarjeta muestra `UNKNOWN` hasta el siguiente `remote.state` o snapshot, aunque el reductor conserve el valor anterior (`DISCONNECTED`): `RemoteUi.view` recibe `unknownSince: Boolean`. Añadir `CLOSING` al `enum Kind` y a los tests.
- El diálogo de **desactivar** avisa: "Away from home, turning it off cuts access until you are back on your Wi-Fi" / "Fuera de casa, apagarlo corta el acceso hasta que vuelvas a tu Wi-Fi" (M15).


**Files:**
- Create: `android/app/src/main/kotlin/tech/hybriq/adbuddy/ui/RemoteUi.kt`, `android/app/src/test/kotlin/tech/hybriq/adbuddy/ui/RemoteUiTest.kt`
- Modify: `.../ui/Screens.kt` (tarjeta en `HomeScreen`, tras Pause/Cortar acceso), `android/app/src/main/res/values/strings.xml`, `values-es/strings.xml`

**Interfaces:**
- Consumes: `BridgeState.remote: RemoteState?` (Task 11), `Bridge.request(type, payload, onResult)`, `HostClock`.
- Produces: `RemoteUi.view(state: BridgeState, pending: Boolean, unknownSince: Boolean, now: Instant): RemoteView` con `RemoteView(kind: Kind, address: String, expiresAt: Instant?, canToggle: Boolean)` y `enum Kind { UNKNOWN, OFF, ON, CLOSING, PENDING, DISCONNECTED }`; `RemoteUi.request(enabled: Boolean): JsonElement`.

- [ ] **Step 1: Tests de `RemoteUi.view`:** desconectado del host (`connection != CONNECTED`) → `DISCONNECTED`, `canToggle = false` (el interruptor solo se usa con el móvil conectado al host); conectado y `remote == null` → `UNKNOWN` (con aviso) y `canToggle = false`; `enabled=false` → `OFF`, `canToggle = true`; `enabled=true` y `expiresAt` futuro → `ON` con dirección y caducidad; `enabled=true` con `expiresAt` ya pasado (usar `HostClock`, no el del móvil) → `CLOSING` (nunca `OFF`); `unknownSince = true` → `UNKNOWN` aunque `remote` tenga valor; `pending = true` → `PENDING` y `canToggle = false`; `expiresAt` ilegible → `ON` sin caducidad mostrada, no lanza. `request(true)` es `{"enabled":true}`.
- [ ] **Step 2:** FALLA.
- [ ] **Step 3: Implementar** `RemoteUi` (puro) y la tarjeta: título "Remote mode" / "Modo remoto"; un `Switch` (`checked = kind in ON/PENDING`, `enabled = canToggle`); al activar, **antes** de enviar, un `AlertDialog` con el aviso de §3.3 de la v1 ("Port 5555 stays open on any network, including public ones, for up to 12 hours. Your PC's key is still required. Turn it off when you are back home." / "El puerto 5555 queda abierto en cualquier red, también en las públicas, hasta 12 horas. Sigue haciendo falta la clave de tu PC. Desactívalo al volver a casa."); al confirmar, `pending = true` y `bridge.request(Protocol.REMOTE_SET, RemoteUi.request(true)) { reply -> pending = false }` (una respuesta `error` muestra su texto, p. ej. "Turn on Tailscale on the phone first"; un `null` —el túnel cayó por el reinicio de adbd, ruling 3— deja `pending = false` y el estado lo da el snapshot de la reconexión). Desactivar no pide diálogo. La caducidad se muestra como hora local ("Until 21:14"/"Hasta las 21:14"); `UNKNOWN` muestra "Unknown: the host will tell when it reconnects" / "Desconocido: el host lo dirá al reconectar"; el estado `ON` muestra también la dirección. Todos los textos en `strings.xml` EN y ES. Colores del tema, nada fijo (pendientes: base de diseño de M3); si M3 ya cambió `HomeScreen`, adaptar a su estructura.
- [ ] **Step 4:** `./gradlew :app:testDebugUnitTest` → PASA.
- [ ] **Step 5: Commit** `feat(app): interruptor del modo remoto con su aviso y la caducidad`.

---

### Task 14: App · envoltura estricta y menores aplazados de M5a

**Files:**
- Modify: `android/app/src/main/kotlin/tech/hybriq/adbuddy/a11y/UiHelpers.kt` (y donde se decodifique `UiRequest`), `.../a11y/Dumper.kt`, `.../a11y/Matcher.kt`, `.../a11y/Overlay.kt`
- Test: los de `a11y/` ya existentes más los nuevos de abajo

**Interfaces:** ninguna nueva.

- [ ] **Step 1: Tests que fallan:**
  - `UiHelpers` decodifica `UiRequest` con un campo extra desconocido (`{"id":"u_1","op":"dump","args":{},"expiresAt":"…","future":1}`): hoy da `UI_TIMEOUT` por no responder (ignoreUnknownKeys=false); el test espera que la app **conteste** `ui.response` con error `invalid` y el `id` de la petición (para que el host no espere 10 s). Decisión: se decodifica con un `Json` propio `strictEnvelope` (`ignoreUnknownKeys = false`) y, **si falla, se rescata el `id` con un parseo mínimo** (`JsonObject["id"]`) y se responde `invalid`; sin `id` legible no se contesta (no hay a quién).
  - `Dumper`/`Matcher`: el parámetro `hidden` (hoy con valor por defecto `{ false }`) pasa a obligatorio; los tests que no lo pasaban pasan `{ false }` explícito.
  - `UiHelpers` WELCOME con `try/catch` (una excepción del manejador no corta el oyente): test con un manejador que lanza.
  - `Overlay`: `@Volatile` en los campos que lee otro hilo; si falla el segundo `addView`, el marco huérfano se quita (test con un `WindowManager` falso que lanza en la segunda llamada: la primera vista se elimina).
  - `ProtectedAppsTest`: añadir `propose` tras `confirm` (no pisa lo confirmado), el retorno de `propose` y `isProtected(ADBUDDY)`.
- [ ] **Step 2:** FALLA. 
- [ ] **Step 3: Implementar** lo anterior. Los menores que **no** se hacen (y por qué) se apuntan en el ledger: "desc de campo password sin vaciar" (el texto sí sale vacío; la `desc` de un campo de contraseña puede llevar el nombre del campo, no su contenido) y "dump con móvil bloqueado da el nombre de la app protegida" (arreglado en I1 de la ronda final de M5a: comprobar y, si ya está, cerrar sin cambios). Comprobar los dos con un test y dejar el resultado en el ledger.
- [ ] **Step 4:** `./gradlew :app:testDebugUnitTest` → PASA.
- [ ] **Step 5: Commit** `fix(app): envoltura UiRequest estricta con respuesta invalid y menores de M5a`.

---

### Task 15: Documentación, pendientes y revisión final

**Files:**
- Modify: `docs/superpowers/plans/pendientes-m3-m5.md`, este plan (sección de validaciones), `host/README.md` si describe `serve` o los subcomandos, `docs/superpowers/plans/2026-10-01-adbuddy-m5a-ayudas.md` (solo la línea de "Después de M5a")

**Interfaces:** ninguna.

- [ ] **Step 1:** En `pendientes-m3-m5.md`, sección M5: marcar I7 como decidido (rulings 7 y 8) y apuntar las pendientes que quedan: `screenrecord` y `dumpsys activity top`/`dumpsys window` (v1.1), Mensajes y Gmail en la pantalla de M5c, y "texto de las ayudas y de `adbuddy remote` en `AGENTS.md`" para el dueño de la plantilla (e5/skimmer-32: pasarles este texto: "`adbuddy remote on|off|status` lo usa el usuario, no el agente; desde la sesión del agente se rechaza").
- [ ] **Step 2:** Rellenar "Validaciones manuales pendientes" (abajo) y copiar a `pendientes` solo las que dependan de decisiones futuras.
- [ ] **Step 3:** `git grep -nE '([0-9]{1,3}\.){3}[0-9]{1,3}' -- ':!*.json' ':!*testdata*' | head` y revisar que ninguna IP real ni serial se haya colado (las `100.64.0.5` y `192.0.2.x` de los tests son de documentación).
- [ ] **Step 4: Revisión final** de toda la rama con `adbuddy-reviewer-opus` (con `-timeout` y tope de minutos), solo después del ok de e5 para lanzarla.
- [ ] **Step 5: Commit** `docs(plan): pendientes y validaciones manuales de M5b`. Avisar a e5; push y PR solo con su ok.

---

## Validaciones manuales pendientes (M5b)

Solo con el móvil del usuario y su permiso explícito.

11. **Puerto de depuración inalámbrica:** `getprop service.adb.tls.port` desde el host da el puerto vigente con Wi-Fi + depuración inalámbrica (Android 17), y `tailscale_port` se guarda.
12. **`tcpip 5555` con Tailscale y solo datos:** `adbuddy remote on` desde la app; el interruptor pasa a "Activando…" y, tras la reconexión, a "Activo hasta HH:MM"; `serve` reconecta por `<ip-tailscale>:5555` con el Wi-Fi apagado; `adbuddy remote off` y la caducidad (con `remote_until` forzado a un minuto) cierran el 5555 (`getprop service.adb.tcp.port` → `0` o vacío).
13. **Respuesta perdida de `remote.set`:** comprobar en el Pixel cuántos segundos tarda la app en volver a ver el estado tras `adb tcpip` (ruling 3) y que "desconocido" nunca se muestra como "apagado".
14. **Persiana y pantalla de bloqueo:** con la persiana y con los ajustes rápidos desplegados, `dumpsys window` da `NotificationShade` como ventana con foco (formato en Android 17) y `adbuddy screen` / `screenshot` responden `Blocked by ADBuddy`; `input keyevent 4` la cierra. Además: (a) con la pantalla de bloqueo, la guarda bloquea todo salvo Back/Home y la tecla de despertar exacta (T10 M3: está documentado, comprobar que no estorba de más); (b) persianas de otros fabricantes (MIUI/HyperOS, One UI, ColorOS) si hay un móvil a mano: ¿el título de la ventana con foco sigue siendo `NotificationShade` o falla cerrado de más/de menos? (T10 M5).
15. **Cambio de dirección en sesión:** con `serve` en marcha, pasar el móvil de la Wi-Fi de casa a datos (modo remoto activo): el vigilante pasa a `<ip-tailscale>:5555` en menos de ~30 s y el agente sigue funcionando con su `ANDROID_SERIAL` anterior.
16. **`NetworkCallback` de VPN:** la app informa de la IP de Tailscale al encender la VPN y al reconectar al host (no al apagarla).
17. **Heads-up (BLOQUEANTE para el respaldo por adb de los heads-up).** No hay muestras reales de `dumpsys window windows` (la imagen del emulador no sirve para la app, M2.5b); sin esta validación el respaldo por adb puede bloquear de más (ruling de T10). Con el servicio de accesibilidad vinculado, una notificación emergente de otra app (mensaje de prueba entre apps propias, nunca un código real) hace que `adbuddy screen`/`foreground` respondan protegido unos 8 s; sin la app conectada, `dumpsys window windows` muestra la ventana emergente de `systemui` y el respaldo la bloquea, y sin notificación NO bloquea (ventanas fijas de systemui: añadir sus títulos a `fixedSystemUI` si falta alguno). Medido en T10c, a comprobar en Android 17: (a) una notificación de prueba con aspecto de OTP (texto "Your code is 123456", de una app propia) genera el evento aunque no traiga `Notification` en `parcelableData`; (b) una notificación que se ACTUALIZA con un código nuevo (mismo id): ¿emite evento? Si no, el código nuevo queda sin los 8 s de protección (T10c I2); (c) ajuste "tiempo para actuar" de Accesibilidad: ¿el heads-up dura más de 8 s? Mirar `getRecommendedTimeoutMillis` y, si pasa, alargar la ventana (T10c M5).
18. **`dumpsys notification --reveal` / `--noredact`** contra el emulador: confirmar que `--reveal` es sinónimo de `--noredact` (T10b I1) y que `dumpsys --noredact` sin servicio reenvía la opción a todos los servicios; grabarlos en `oracle.jsonl`/`differential.jsonl` si procede. Ambos son 🔴 especiales (T10b M2). Además (revisión final de M5b, sin verificar): según AOSP, el `dumpsys notification` redactado (🟢 hoy) imprime los 8 primeros caracteres de `tickerText` cuando mide más de 16, así que un SMS "G-123456 is your…" dejaría ver el código entero. Comprobarlo en el emulador con una notificación de prueba de una app propia (nunca un código real); si se confirma, pasar `dumpsys notification` a 🔴 (`habitual.jsonl` no lo usa). Ese cambio de la lista blanca lleva su caso en `differential.jsonl` o en el generador y regrabar el oráculo si procede: no se hace en M5b.
19. **`Overlay`/`WindowManager` real** en emulador o Pixel: marco fino y píldora de Pausa visibles al actuar, la píldora envía `pause`, y el marco no tapa ni intercepta toques. Si no se pueden mostrar (permiso de superposición retirado, `addView` falla), `click` y `setText` responden FAILED ("ADBuddy can't show the on-screen notice") y el agente no actúa (ruling de T14). `dump` puede esperar hasta 2 s al overlay (T14 menor).
20. **`remote.state` al `attach`:** el host republica `remote.state` cuando la app hace `attach`. En UNKNOWN con canal abierto la tarjeta no tiene interruptor hasta que el host hable (T13): comprobar que el host habla siempre y que la tarjeta no se queda sin interruptor.
21. **`unknownSince` y CLOSING en la tarjeta,** en emulador o Pixel: tras pasar la caducidad con el puerto sin cerrar la tarjeta pasa sola a "Cerrando…" (tick atado a la caducidad, T13 I1) y vuelve a "apagado" al cerrarse; "desconocido" nunca se muestra como "apagado" y con DISCONNECTED manda DISCONNECTED.

## Autorrevisión del plan

- **Cobertura de la spec §7:** 7.1 → T4 (`device.info`, puerto por `getprop`, `findKnownPhone` con `tailscale_ip:tailscale_port` tras `address`, antes de mDNS: T7); 7.2 → T3 (`tcpip`/`usb`, 12 h, invariante), T5 (`remote.set`, `remote.state` de tres valores), T6-T7 (serial activo y reconexión con `ts:5555`), T8 (`adbuddy remote`, `AGENT_SESSION`), T11-T13 (reductor, aviso, caducidad visible); "concesiones solo a una app verificada" (`phoneapp.Verified`) no cambia: M5b no añade concesiones; §3.6 (captura en modo remoto) → T9. Arrastres de M5a → T10 (I7), T14 (`ignoreUnknownKeys`, menores).
- **Tipos coherentes:** `remote.Port` (T3) lo usan T5, T7; `cfgStore` (T4) lo usan T5, T7; `activePhone` (T6) lo usan T5 (tras T6), T7; `RemoteState?` (T11) lo usa T13; `ScreenshotArgs` con ceros (T9). `applyInvariant` con `keep` (T3) se llama desde `serve` (T3 con `""`, T5 con el valor real).
- **Orden de tareas:** T3 deja `serve` pasando `""`; T5 crea `remote.New(runner, ...)` y T6 lo cambia por `active` (dicho en su Step 5); T4 crea `store` antes de `findKnownPhone`; el controlador se crea después de `runner.Serial = ...` (T5), y `candidateAddrs` usa `store.Until()` para saber si el modo está activo (T7).
- **Riesgos abiertos para el revisor:** (a) formato de `dumpsys window` en Android 17 no verificado (validación 14, falla cerrado); (b) `adb tcpip` desde una conexión inalámbrica depende de S1 (validado con matices); (c) `ANDROID_SERIAL` del agente queda viejo tras un `Switch` (cubierto por `Serials`); (d) el `device.info` de la app y el de `UiHelpers` comparten manejador en el host (T4).
