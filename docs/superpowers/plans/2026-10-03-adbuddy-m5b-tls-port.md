# ADBuddy M5b · El puerto de la depuración inalámbrica sale de la conexión, no de `getprop` (M5b-11/12) · Plan de implementación

> **Para quien ejecute:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`. Los pasos usan casillas (`- [ ]`). Implementador y re-revisiones en Sonnet (`adbuddy-implementer`, `adbuddy-rereviewer`; si BLOCKED o ronda 4-5, `adbuddy-implementer-opus`); la Tarea 1 es del modo remoto (🔒): la revisa `adbuddy-reviewer-opus`, y la Tarea 2 `adbuddy-reviewer`. Nunca Haiku. **Antes de la tarea 1, `adbuddy-reviewer-opus` revisa este plan completo, código incluido** (CLAUDE.md), y el plan se corrige con sus hallazgos.

**Meta:** que `tailscale_port` se guarde de verdad. Hoy `remoteLink.readTLSPort` (`host/internal/cli/phone_remote.go:442`) ejecuta `adb shell getprop service.adb.tls.port`, y el usuario `shell` no puede leer esa propiedad: SELinux deniega la lectura (`adbd_prop`) y `getprop` devuelve una cadena vacía con código 0 (medido en la API 34, hallazgo T1-12 de `docs/superpowers/plans/2026-10-03-validaciones-manuales.md`, en `origin/main` 2bb199a; con `su 0` da el puerto, pero un Pixel de producción no tiene `su`). `readTLSPort` da 0, la app no manda `wifiAdbPort` (no hay API pública), y `tailscale_port` no se guarda nunca. Efecto: `candidateAddrs` (`phone_watch.go:33`) y `findKnownPhone` no prueban `tailscale_ip:tailscale_port`, y la reconexión por Tailscale solo funciona por el 5555 del modo remoto.

**Arquitectura:** el host ya conoce el puerto por la conexión que tiene con el móvil. Si adb lista el móvil como `ip:puerto` (conectado por dirección), ese es el puerto. Si lo lista por mDNS (`<instancia>._adb-tls-connect._tcp`, el caso normal en casa, ver `docs/spikes/s1-modo-remoto.md`), el puerto es el del servicio `_adb-tls-connect._tcp` con esa misma instancia en `adb mdns services` (reutiliza `pairing.Services`, `pairing.ConnectType` y el formato que ya usan `setup` y `pairing.Connect`). El `getprop` se quita. Para leer el serial vigente y lanzar órdenes de host, `remoteLink` recibe un `phoneConn` (`Serial()` y `Host()`), que es el `activePhone` de `serve`: `startPhoneLink` lo saca de su `phoneapp.ADB` con `connOf(a)`, así que su firma y la de sus 7 llamadas en tests no cambian. `save` sigue en `drain` (fuera del bucle de mensajes, un informe a la vez) y con el tope `tlsPortTimeout` sobre el contexto del enlace. **No cambian:** el protocolo con la app, `candidateAddrs`, `findKnownPhone`, el vigilante, el controlador del modo remoto, la comprobación de identidad (`verifyAt`) ni ningún texto de cara al usuario.

**Tecnologías:** Go 1.27 (`host/`). Nada nuevo en `go.mod`.

**Fuentes:** encargo `.superpowers/sdd/2026-10-03-adbuddy-m5b-tls-port/plan-brief.md` · spec M5 `docs/superpowers/specs/2026-10-01-adbuddy-m5-ayudas-remoto-design.md` §7.1 (dice que el host lee el puerto con `getprop`: la Tarea 2 la corrige) y §7.2 (reconexión, el 5555 solo como candidato con el modo encendido) · plan de M5b `docs/superpowers/plans/2026-10-01-adbuddy-m5b-modo-remoto.md` (Task 4, validaciones 11 y 12) · `docs/superpowers/plans/pendientes-m3-m5.md` ("QA automático de M5b") · inventario `docs/superpowers/plans/2026-10-03-validaciones-manuales.md` (en `origin/main`: fila M5b-11 y "Hallazgos de la T1") · spike S1 (`service.adb.tcp.port` sí se lee como `shell`: 5555, 0; solo `service.adb.tls.port` está vetada).

**Verificación del plan (réplica acumulada):** el código se escribió paso a paso sobre una copia (`git archive`) de `origin/main` y después **se volvió a aplicar desde el texto de este plan** con un script (cada par "Reemplazar / por" de cada paso, en orden, exigiendo que el ancla sea única) sobre un `git archive` limpio de 7c4613c, la base de la rama (`origin/main` está ahora en 2bb199a, con el mismo `host/`: entre las dos solo cambian documentos de `validaciones-manuales`). Las dos copias quedaron byte a byte iguales en los cinco ficheros, y las salidas citadas en los pasos son las reales. Al final: `go build ./... && go vet ./... && gofmt -l .` limpios y `go test -count=1 -timeout 15m ./...` con todos los paquetes en `ok` (detalle en "Réplica acumulada"). Los cambios a ficheros existentes son pares "Reemplazar / por" para la herramienta Edit (cada texto "Reemplazar" es único en su fichero en el momento de aplicarlo) y, una vez, un borrado por script (dicho en su paso).

## Global Constraints

- Rama `fix/m5b-tls-port`, desde `origin/main` 7c4613c. Commits con identidad local hybriqtech (nunca `git config --global`) y trailer **exacto** `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` (comprobarlo con `git log -1 --format=%B`). Nunca `--no-verify` ni `--force`, sin `git stash`. Sin push ni PR salvo que lo pida el controlador; a `main` integra el usuario. `git grep` de seriales e IPs antes de cualquier push.
- **Sin IPs RFC 1918** en código, tests ni docs: `100.64.0.0/10` (Tailscale, p. ej. `100.101.102.103`), `192.0.2.x`, `198.51.100.x`, `2001:db8::/32`. Sin seriales reales: los tests usan `phoneSerial` (`adb-AAAA1111-bbb._adb-tls-connect._tcp`, de `invariant_test.go`) y `AAAA1111`.
- **El móvil del usuario no se toca.** Los tests usan adb falsos en memoria (`connADB`, `ctlADB`). Lo que necesita un móvil va a "Validaciones manuales pendientes" y solo se hace con su permiso explícito.
- **Rulings de M5b que no se rompen:** el 5555 solo es candidato con el modo remoto encendido (spec §7.2: nunca la IP de casa con el 5555, ni el 5555 con el modo apagado); el vigilante solo cambia a un móvil verificado (`verifyAt`); `tailscale_ip` es la que manda la app (validada con `config.ValidTailscaleIP`); un `device.info` sin dirección no cambia nada; un informe de una conexión reemplazada no guarda.
- No se tocan `doctor.go`, los catálogos i18n (`host/internal/i18n/catalog/*`), el emulador (`scripts/emulator.sh`, tests de emulador) ni `host/internal/classify/`. **Ningún texto nuevo de cara al usuario** (ver "Textos nuevos").
- **Cada tarea compila y pasa sus tests por sí sola.**
- **Go:** `cd host && go build ./... && go vet ./... && gofmt -l . && go test -count=1 -timeout 5m ./internal/cli/...`; la suite completa (`go test -count=1 -timeout 15m ./...`) en el último paso de la Tarea 1. Este PC no corre `-race`: la concurrencia se razona en la tarea y la corre la CI.
- `docs/**` está en **CRLF** en el árbol de trabajo (`core.autocrlf=true`): sus ediciones usan anclas de **una sola línea**. `host/**` es LF (`.gitattributes`).
- Pasar a los revisores límites de tiempo (`go test -timeout 5m`, tope de 30 minutos por revisión).
- Windows con Git Bash: sin `sleep` en primer plano; los comandos largos con timeout amplio o en segundo plano.

## Rulings de este plan (van al ledger tal cual)

1. **Ruling: el puerto sale del serial activo (`ip:puerto`) o, con el móvil conectado por mDNS, del servicio `_adb-tls-connect._tcp` de su misma instancia; nunca de `getprop` ni de `cfg.Address`.** — El serial activo es la conexión viva y verificada (`serve` y el vigilante solo pasan a un móvil verificado), y su forma decide la fuente: las dos fuentes son excluyentes (un serial `ip:puerto` no tiene instancia mDNS y un serial mDNS no trae puerto), así que no hay orden que elegir entre ellas. `cfg.Address` no es una fuente aparte: cuando funciona, `findKnownPhone` (y el vigilante) la dejan como serial activo, y ya entra por la primera regla; cuando no es el serial activo es una dirección de otro momento (el puerto cambia cada vez que se reactiva la depuración inalámbrica) y sustituiría un puerto bueno por uno caducado (además puede ser un serial y no una dirección). El `wifiAdbPort` de la app, si algún día llega, solo cuenta cuando la conexión no da puerto. — Si me equivoco: conectado por USB o por el 5555 no hay puerto nuevo y se queda el guardado; la reconexión queda como hoy, nunca peor.
2. **Ruling: en mDNS cuenta el nombre exacto de la instancia, y si sus anuncios no coinciden en el puerto no se cree a ninguno.** — Cualquiera en la red puede anunciar ese nombre, y el sufijo ` (2)` de mDNS es un choque de nombres. Se busca la instancia exacta con la que adb lista la conexión viva (no `matchName` sobre el guid, que juntaría `adb-X` y `adb-X (2)`). Varios anuncios con el mismo puerto (una IPv4 y una IPv6) valen. Un puerto falso solo produciría un candidato que el vigilante descarta al verificar la identidad, nunca un cambio a otro móvil. — Si me equivoco: con un anuncio viejo en la caché de adb tras reactivar la depuración, no se guarda nada hasta que caduca; el siguiente `device.info` lo vuelve a intentar.
3. **Ruling: nunca se guarda el 5555 (del serial, de mDNS ni de la app) ni un puerto fuera de 1-65535, y sin puerto conocido se conserva el guardado.** — El 5555 es el `adb tcpip` del modo remoto (el móvil puede estar conectado por él, también desde la LAN), y ya tiene su candidato propio, `tailscale_ip:5555`, solo con el modo encendido; guardado como `tailscale_port` sería candidato con el modo apagado (spec §7.2). La IP de la conexión no se guarda nunca (puede ser la de casa): el puerto de la depuración inalámbrica es el mismo en todas las interfaces mientras no se apague, así que el de la LAN sirve para la dirección de Tailscale (validación 1). — Si me equivoco: un móvil cuyo puerto aleatorio fuera justo el 5555 no guarda puerto.
4. **Ruling: el `getprop service.adb.tls.port` se quita, no queda como último recurso.** — En un móvil de producción da siempre vacío con código 0 (no es un fallo intermitente: es la política de SELinux para `shell`); solo responde con root, que ADBuddy no usa. Dejarlo costaría un `adb shell` y un `avc: denied` en el registro del móvil en cada `device.info`. — Si me equivoco: en un móvil cuya política sí deje leerlo se pierde esa fuente, pero la conexión da el mismo puerto.
5. **Ruling: las órdenes de host salen del propio `activePhone` (`Host()`: su runner sin serial), y `startPhoneLink` lo recupera con `connOf(a)`.** — `serve` ya pasa `active` como `phoneapp.ADB`; así no cambia la firma de `startPhoneLink` ni sus llamadas, el serial se lee al buscar el puerto (cambia con `Switch`) y no hace falta otro runner (`hostRunner` de `serve` es el mismo con `Serial` vacío). `var _ phoneConn = (*activePhone)(nil)` lo fija al compilar y `TestConnOfIsTheActivePhone` prueba el cableado. Los adb falsos de otros tests no son `phoneConn`: con ellos el enlace solo usa el puerto de la app. — Si me equivoco: si `serve` pasara otro tipo, `connOf` daría nil y se volvería a no guardar puerto (lo de hoy); el test lo vigila para `activePhone`.

## Textos nuevos de cara al usuario

Ninguno. No hay claves nuevas ni cambiadas en `catalog/{en,es}.toml`; solo cambian comentarios de código (en inglés, como el resto) y documentos de diseño.

## Mapa de ficheros

| Fichero | Tarea | Cambio |
|---|---|---|
| `host/internal/cli/phone_remote.go` | 1 | `phoneConn`, `connOf`, campo `conn`, `tlsPort`/`tlsPortOf`/`addrPort`/`mdnsPort`/`wirelessPort`; fuera `readTLSPort` y el import `bytes` |
| `host/internal/cli/activephone.go` | 1 | `Host()` |
| `host/internal/cli/phone.go` | 1 | línea 131: `connOf(a)` |
| `host/internal/config/config.go` | 1 | comentario de `TailscalePort` |
| `host/internal/cli/phone_remote_test.go` | 1 | adb falso `connADB` (en lugar de `tlsADB`), tests nuevos, fuera `TestReadTLSPortParses` |
| `docs/superpowers/specs/2026-10-01-adbuddy-m5-ayudas-remoto-design.md` | 2 | §7.1 |
| `docs/superpowers/plans/pendientes-m3-m5.md` | 2 | QA automático de M5b, menor T4 |
| `docs/superpowers/plans/2026-10-01-adbuddy-m5b-modo-remoto.md` | 2 | validación 11 |
| `docs/superpowers/plans/2026-10-03-validaciones-manuales.md` | 2 | fila M5b-11 y hallazgo T1-12 (solo si la rama ya lo tiene) |

---

### Task 1: el puerto de la depuración inalámbrica sale de la conexión del host (M5b-11) 🔒

**Files:** los cinco de `host/` del mapa.

- Produces: `type phoneConn interface { Serial() string; Host() pairing.ADB }`; `func connOf(a phoneapp.ADB) phoneConn`; `func newRemoteLink(ctx context.Context, store *cfgStore, ctl *remote.Controller, conn phoneConn, log io.Writer) *remoteLink` (antes `a phoneapp.ADB`); `func (p *activePhone) Host() pairing.ADB`; `func tlsPortOf(ctx context.Context, host pairing.ADB, serial string) int`; `func addrPort(addr string) int`; `func mdnsPort(ctx context.Context, host pairing.ADB, inst string) int`; `func wirelessPort(p int) bool`.
- Desaparece: `func (r *remoteLink) readTLSPort() int`.

**Concurrencia (razonada; la CI corre `-race`):** `save` corre solo en la gorrutina de `drain` (una a la vez, `running` bajo `mu`). `tlsPort` lee `conn.Serial()` (`activePhone` toma su mutex) y `conn.Host()` (una copia del runner), y lanza `adb mdns services` con `context.WithTimeout(r.ctx, tlsPortTimeout)`: termina con `serve` o a los 5 s, como el `getprop` de antes. No toca estado compartido nuevo. En el adb falso, `serial`, `mdns` y `calls` van bajo su mutex (`set`, `Serial`, `Run`); `gate` y `block` se ponen antes de lanzar el informe, como en el `tlsADB` de hoy.

- [ ] **Step 1: tests que fallan con el código de hoy.** Todos compilan con el código actual: el adb falso nuevo sirve a la vez como `phoneapp.ADB` (hoy) y como `phoneConn` (después), y los tests nuevos pasan por `onDeviceInfo`, no por funciones nuevas.

Edit (paso 1) en `host/internal/cli/phone_remote_test.go`:

**Reemplazar:**
```go
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/remote"
```
**por:**
```go
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/pairing"
	"github.com/hybriqtech/adbuddy/host/internal/remote"
```

Edit (paso 1) en `host/internal/cli/phone_remote_test.go`:

**Reemplazar:**
```go
// tlsADB answers `getprop service.adb.tls.port`. gate, when set, makes each
// call wait for it to close before answering; block makes each call wait for
// its context to end.
type tlsADB struct {
	mu    sync.Mutex
	port  string
	calls []string
	gate  chan struct{}
	block bool
}

func (a *tlsADB) Run(ctx context.Context, _ string, args []string, _ io.Reader, stdout, _ io.Writer) (int, error) {
	if a.block {
		<-ctx.Done()
		return -1, ctx.Err()
	}
	if a.gate != nil {
		<-a.gate
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, strings.Join(args, " "))
	if a.port == "fail" {
		return 1, errors.New("adb down")
	}
	io.WriteString(stdout, a.port+"\n")
	return 0, nil
}

func (a *tlsADB) called() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string{}, a.calls...)
}
```
**por:**
```go
// rigInstance is the mDNS instance of phoneSerial's connect service.
const rigInstance = "adb-AAAA1111-bbb"

// mdnsLine is one line of `adb mdns services` for a connect service.
func mdnsLine(name, addr string) string {
	return name + "\t" + pairing.ConnectType + "\t" + addr + "\n"
}

// connADB is the phone as the remote link sees it: the serial adb lists it
// with, and adb's commands. `mdns services` lists mdns; `shell getprop
// service.adb.tls.port` answers empty with code 0, as on a real phone (SELinux
// keeps the shell from reading it, validation M5b-11). fail makes every
// command fail; gate, when set, makes each one wait for it to close before
// answering; block makes each one wait for its context to end.
type connADB struct {
	mu     sync.Mutex
	serial string
	mdns   string
	fail   bool
	calls  []string
	gate   chan struct{}
	block  bool
}

// newConnADB is the rig's phone, connected by mDNS: port is the one its
// connect service announces ("" announces none, "fail" makes adb fail).
func newConnADB(port string) *connADB {
	a := &connADB{serial: phoneSerial}
	switch port {
	case "":
	case "fail":
		a.fail = true
	default:
		a.mdns = mdnsLine(rigInstance, "192.0.2.10:"+port)
	}
	return a
}

func (a *connADB) Serial() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.serial
}

func (a *connADB) Host() pairing.ADB { return a }

func (a *connADB) set(serial, mdns string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.serial, a.mdns = serial, mdns
}

func (a *connADB) Run(ctx context.Context, _ string, args []string, _ io.Reader, stdout, _ io.Writer) (int, error) {
	if a.block {
		<-ctx.Done()
		return -1, ctx.Err()
	}
	if a.gate != nil {
		<-a.gate
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	cmd := strings.Join(args, " ")
	a.calls = append(a.calls, cmd)
	if a.fail {
		return 1, errors.New("adb down")
	}
	switch cmd {
	case "mdns services":
		io.WriteString(stdout, "List of discovered mdns services\n"+a.mdns)
	case "shell getprop service.adb.tls.port":
		io.WriteString(stdout, "\n")
	}
	return 0, nil
}

func (a *connADB) called() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string{}, a.calls...)
}
```

Edit (paso 1) en `host/internal/cli/phone_remote_test.go`:

**Reemplazar:**
```go
	adb    *tlsADB
```
**por:**
```go
	adb    *connADB
```

Edit (paso 1) en `host/internal/cli/phone_remote_test.go`:

**Reemplazar:**
```go
	rig := &remoteRig{adb: &tlsADB{port: port}, srv:
```
**por:**
```go
	rig := &remoteRig{adb: newConnADB(port), srv:
```

Edit (paso 1) en `host/internal/cli/phone_remote_test.go`:

**Reemplazar:**
```go
	if calls := g.adb.called(); len(calls) != 1 || calls[0] != "shell getprop service.adb.tls.port" {
```
**por:**
```go
	if calls := g.adb.called(); len(calls) != 1 || calls[0] != "mdns services" {
```

Borrar `TestReadTLSPortParses` entera (desde su comentario `// Validation 11: what \`getprop service.adb.tls.port\` says becomes a port only` hasta la línea en blanco tras su `}` de cierre, 26 líneas). Uno de sus casos lleva un CR literal dentro de la cadena (`"41234\r"`), así que no se puede anclar entera con Edit; se borra con este script (Git Bash, desde `host/`):

```bash
f=internal/cli/phone_remote_test.go
s=$(grep -n '^// Validation 11: what `getprop' "$f" | cut -d: -f1)
e=$(awk -v s="$s" 'NR>s && /^}$/ {print NR; exit}' "$f")
sed -i "${s},$((e+1))d" "$f"
```

Comprobar después que `grep -n "TestReadTLSPortParses\|readTLSPort()" internal/cli/phone_remote_test.go` no da nada y que `TestDeviceInfoPortFallbacks` sigue entera.

Edit (paso 1) en `host/internal/cli/phone_remote_test.go`:

**Reemplazar:**
```go
func TestDeviceInfoFromAReplacedConnectionIsIgnored(t *testing.T) {
```
**por:**
```go
// Validation M5b-11: the phone's shell cannot read service.adb.tls.port
// (connADB answers it empty with code 0, like a real phone), so the port comes
// from the connection the host already has: the serial adb lists the phone
// with (ip:port), or the phone's connect service in `adb mdns services`. Only
// a wireless debugging port is saved (1-65535, never the remote mode's 5555),
// the connection's IP never becomes tailscale_ip, and a port that is not known
// keeps the saved one (40000).
func TestDeviceInfoTakesThePortFromTheConnection(t *testing.T) {
	const other = "adb-CCCC2222-ddd"
	for _, c := range []struct {
		name, serial, mdns string
		app                int // the port the app reports (wifiAdbPort)
		want               int // 0: the saved port stays
	}{
		{name: "connected by a LAN address", serial: "192.0.2.10:41234", want: 41234},
		{name: "connected over Tailscale", serial: "100.101.102.103:41234", want: 41234},
		{name: "connected by IPv6", serial: "[2001:db8::10]:41234", want: 41234},
		{name: "the address wins over the app", serial: "192.0.2.10:41234", app: 41000, want: 41234},
		{name: "through the remote mode's 5555", serial: "100.101.102.103:5555"},
		{name: "5555 from the app is not wireless debugging", serial: "100.101.102.103:5555", app: 5555},
		{name: "the app's port when the connection has none", serial: "100.101.102.103:5555", app: 41000, want: 41000},
		{name: "by mDNS", serial: phoneSerial, mdns: mdnsLine(rigInstance, "192.0.2.10:41234"), want: 41234},
		{name: "renamed by mDNS: only its own instance", serial: rigInstance + " (2)." + pairing.ConnectType,
			mdns: mdnsLine(rigInstance, "192.0.2.10:39999") + mdnsLine(rigInstance+" (2)", "192.0.2.10:41234"), want: 41234},
		{name: "one port on two interfaces", serial: phoneSerial,
			mdns: mdnsLine(rigInstance, "192.0.2.10:41234") + mdnsLine(rigInstance, "[2001:db8::10]:41234"), want: 41234},
		{name: "mDNS announces two ports", serial: phoneSerial,
			mdns: mdnsLine(rigInstance, "192.0.2.10:41234") + mdnsLine(rigInstance, "192.0.2.66:41235")},
		{name: "mDNS announces 5555", serial: phoneSerial, mdns: mdnsLine(rigInstance, "192.0.2.10:5555")},
		{name: "another phone's service", serial: phoneSerial, mdns: mdnsLine(other, "192.0.2.20:41234")},
		{name: "the pairing service", serial: phoneSerial, mdns: rigInstance + "\t" + pairing.PairingType + "\t192.0.2.10:41234\n"},
		{name: "mDNS does not list it", serial: phoneSerial},
		{name: "by USB", serial: "AAAA1111"},
		{name: "port out of range", serial: "192.0.2.10:70000"},
		{name: "port 0", serial: "192.0.2.10:0"},
		{name: "not an IP", serial: "phone.example:41234"},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := newRemoteRig(t, withTailscale(), "")
			g.adb.set(c.serial, c.mdns)
			g.r.onDeviceInfo(bridge.Message{}, bridge.DeviceInfo{TailscaleIP: "100.101.102.104", WifiADBPort: c.app})
			g.idle(t)
			want := c.want
			if want == 0 {
				want = 40000
			}
			if s := g.saved(t); s.TailscaleIP != "100.101.102.104" || s.TailscalePort != want {
				t.Errorf("saved %s:%d, want 100.101.102.104:%d", s.TailscaleIP, s.TailscalePort, want)
			}
			for _, call := range g.adb.called() {
				if strings.Contains(call, "getprop") {
					t.Errorf("asked the phone's shell: %q", call)
				}
			}
		})
	}
}

// The defect of M5b-11 seen from the watcher: once the port is saved, the
// phone's Tailscale address at its wireless debugging port is a candidate.
func TestTheSavedPortIsACandidateAddress(t *testing.T) {
	g := newRemoteRig(t, config.Default(), "")
	g.adb.set("192.0.2.10:41234", "")
	g.r.onDeviceInfo(bridge.Message{}, bridge.DeviceInfo{TailscaleIP: "100.101.102.103"})
	g.idle(t)
	if got := candidateAddrs(g.store.Config(), false); !slices.Contains(got, "100.101.102.103:41234") {
		t.Fatalf("candidates %v", got)
	}
}

func TestDeviceInfoFromAReplacedConnectionIsIgnored(t *testing.T) {
```

Edit (paso 1) en `host/internal/cli/phone_remote_test.go`:

**Reemplazar:**
```go
	g.adb.mu.Lock()
	g.adb.port = "41235"
	g.adb.mu.Unlock()
```
**por:**
```go
	g.adb.set(phoneSerial, mdnsLine(rigInstance, "192.0.2.10:41235"))
```

Edit (paso 1) en `host/internal/cli/phone_remote_test.go`:

**Reemplazar:**
```go
func TestReadTLSPortFollowsTheLinkContext(t *testing.T) {
```
**por:**
```go
func TestTLSPortFollowsTheLinkContext(t *testing.T) {
```

Los demás tests del fichero no cambian: los que pasan `"41234"`, `""` o `"fail"` a `newRemoteRig` tienen ahora un móvil conectado por mDNS cuyo servicio anuncia ese puerto (o ninguno, o adb falla), y `gate`/`block` siguen deteniendo la orden de adb que haya (hoy el `getprop`, después `mdns services`). Las llamadas `newRemoteLink(g.ctx, g.store, g.ctl, g.adb, g.log)` y `newRemoteLink(g.ctx, g.store, nil, g.adb, g.log)` compilan antes y después (`*connADB` es `phoneapp.ADB` y `phoneConn`).

- [ ] **Step 2: RED.** `cd host && go vet ./internal/cli/ && go test -count=1 -timeout 5m -run 'TestDeviceInfo|TestTheSavedPort|TestTLSPort|TestChatPort' ./internal/cli/`. Falla por el motivo esperado (el puerto no se guarda y se pregunta al shell); salida real de la réplica, recortada:

```
--- FAIL: TestDeviceInfoSavesTheAddressAndThePort (0.02s)
    phone_remote_test.go:314: saved "100.101.102.103" 0
--- FAIL: TestDeviceInfoTakesThePortFromTheConnection (0.23s)
    --- FAIL: TestDeviceInfoTakesThePortFromTheConnection/connected_by_a_LAN_address (0.01s)
        phone_remote_test.go:412: saved 100.101.102.104:40000, want 100.101.102.104:41234
        phone_remote_test.go:416: asked the phone's shell: "shell getprop service.adb.tls.port"
    --- FAIL: TestDeviceInfoTakesThePortFromTheConnection/the_address_wins_over_the_app (0.01s)
        phone_remote_test.go:412: saved 100.101.102.104:41000, want 100.101.102.104:41234
    --- FAIL: TestDeviceInfoTakesThePortFromTheConnection/5555_from_the_app_is_not_wireless_debugging (0.01s)
        phone_remote_test.go:412: saved 100.101.102.104:5555, want 100.101.102.104:40000
    --- FAIL: TestDeviceInfoTakesThePortFromTheConnection/by_mDNS (0.01s)
        phone_remote_test.go:412: saved 100.101.102.104:40000, want 100.101.102.104:41234
    (… los 19 subtests fallan: los que esperan puerto, por el puerto; todos, por "asked the phone's shell")
--- FAIL: TestTheSavedPortIsACandidateAddress (0.01s)
    phone_remote_test.go:431: candidates []
--- FAIL: TestDeviceInfoWithTheSameValueDoesNotRewrite (0.03s)
    phone_remote_test.go:466: saves 1 port 0
--- FAIL: TestChatPortDoesNotOverwriteTheTailscaleAddress (0.02s)
    phone_remote_test.go:495: saved {… TailscaleIP:100.101.102.103 TailscalePort:0 …}
FAIL	github.com/hybriqtech/adbuddy/host/internal/cli	0.481s
```

Si algún test falla por otra cosa (no compila, `panic`), parar y avisar.

- [ ] **Step 3: el código.**

Edit (paso 3) en `host/internal/cli/phone_remote.go`:

**Reemplazar:**
```go
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
	"github.com/hybriqtech/adbuddy/host/internal/remote"
)
```
**por:**
```go
import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/pairing"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
	"github.com/hybriqtech/adbuddy/host/internal/remote"
)
```

Edit (paso 3) en `host/internal/cli/phone_remote.go`:

**Reemplazar:**
```go
// tlsPortTimeout bounds reading the wireless debugging port from the phone.
const tlsPortTimeout = 5 * time.Second
```
**por:**
```go
// tlsPortTimeout bounds looking up the wireless debugging port (adb mdns
// services).
const tlsPortTimeout = 5 * time.Second

// phoneConn is where the remote link finds the phone's wireless debugging
// port. The phone's shell cannot read service.adb.tls.port (SELinux: getprop
// answers empty, with code 0), so the port comes from the connection the host
// already has: the serial adb lists the phone with now, and adb's host
// commands (adb mdns services). serve's activePhone is one.
type phoneConn interface {
	Serial() string
	Host() pairing.ADB
}

var _ phoneConn = (*activePhone)(nil)

// connOf is the phoneConn behind a, nil when a has none (the fake phones of
// tests): only the port the app reports is used then.
func connOf(a phoneapp.ADB) phoneConn {
	c, _ := a.(phoneConn)
	return c
}
```

Edit (paso 3) en `host/internal/cli/phone_remote.go`:

**Reemplazar:**
```go
	ctl   *remote.Controller // nil only in tests: remote.set is then unavailable
	adb   phoneapp.ADB
	log   io.Writer
```
**por:**
```go
	ctl   *remote.Controller // nil only in tests: remote.set is then unavailable
	conn  phoneConn          // nil without one: only the app's port is used
	log   io.Writer
```

Edit (paso 3) en `host/internal/cli/phone_remote.go`:

**Reemplazar:**
```go
func newRemoteLink(ctx context.Context, store *cfgStore, ctl *remote.Controller, a phoneapp.ADB, log io.Writer) *remoteLink {
	r := &remoteLink{ctx: ctx, store: store, ctl: ctl, adb: a, log: log, warned: map[string]bool{}}
```
**por:**
```go
func newRemoteLink(ctx context.Context, store *cfgStore, ctl *remote.Controller, conn phoneConn, log io.Writer) *remoteLink {
	r := &remoteLink{ctx: ctx, store: store, ctl: ctl, conn: conn, log: log, warned: map[string]bool{}}
```

Edit (paso 3) en `host/internal/cli/phone_remote.go`:

**Reemplazar:**
```go
// onDeviceInfo keeps the Tailscale address the app reports, and the port of
// wireless debugging read from the phone (or the one the app reports). No
// address means no change (the message may carry only the protected apps).
```
**por:**
```go
// onDeviceInfo keeps the Tailscale address the app reports, and the port of
// wireless debugging the host's connection shows (see tlsPortOf), or else the
// one the app reports. No address means no change (the message may carry
// only the protected apps).
```

Edit (paso 3) en `host/internal/cli/phone_remote.go`:

**Reemplazar:**
```go
	// Reading the port runs adb: off the app's message loop, one report at a
```
**por:**
```go
	// Finding the port may run adb: off the app's message loop, one report at a
```

Edit (paso 3) en `host/internal/cli/phone_remote.go`:

**Reemplazar:**
```go
	port := r.readTLSPort()
	if port == 0 && info.WifiADBPort >= 1 && info.WifiADBPort <= 65535 {
		port = info.WifiADBPort
	}
```
**por:**
```go
	port := r.tlsPort()
	if port == 0 && wirelessPort(info.WifiADBPort) {
		port = info.WifiADBPort
	}
```

Edit (paso 3) en `host/internal/cli/phone_remote.go` (es la última función del fichero; la sustituyen cinco):

**Reemplazar:**
```go
// readTLSPort asks the phone for its wireless debugging port; 0 when it
// does not say.
func (r *remoteLink) readTLSPort() int {
	ctx, cancel := context.WithTimeout(r.ctx, tlsPortTimeout)
	defer cancel()
	var out, errb bytes.Buffer
	code, err := r.adb.Run(ctx, "", []string{"shell", "getprop service.adb.tls.port"}, nil, &out, &errb)
	if err != nil || code != 0 {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(out.String()))
	if err != nil || n < 1 || n > 65535 {
		return 0
	}
	return n
}
```
**por:**
```go
// tlsPort is the phone's wireless debugging port as the host's connection
// shows it (see tlsPortOf); 0 when it does not, and then the saved port stays.
func (r *remoteLink) tlsPort() int {
	if r.conn == nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(r.ctx, tlsPortTimeout)
	defer cancel()
	return tlsPortOf(ctx, r.conn.Host(), r.conn.Serial())
}

// tlsPortOf finds the wireless debugging port of the phone adb lists as
// serial, without the phone's shell:
//   - serial ip:port (connected by address, at home or over Tailscale): its
//     port. Wireless debugging listens on every interface at the same port, so
//     a LAN address tells the port of the Tailscale one too. Its IP is not
//     used: tailscale_ip is only what the app reports;
//   - serial "<instance>._adb-tls-connect._tcp" (connected by mDNS): the port
//     of the connect service with exactly that instance name in `adb mdns
//     services` (host is an ADB for host commands, with no serial);
//   - anything else (USB, an emulator): 0.
//
// A port that cannot be wireless debugging's (see wirelessPort) is 0 too.
func tlsPortOf(ctx context.Context, host pairing.ADB, serial string) int {
	if inst, ok := strings.CutSuffix(serial, "."+pairing.ConnectType); ok {
		return mdnsPort(ctx, host, inst)
	}
	return addrPort(serial)
}

// addrPort is the port of addr when addr is ip:port and the port can be
// wireless debugging's; 0 otherwise.
func addrPort(addr string) int {
	h, p, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	if _, err := netip.ParseAddr(h); err != nil {
		return 0
	}
	n, err := strconv.ParseUint(p, 10, 16)
	if err != nil || !wirelessPort(int(n)) {
		return 0
	}
	return int(n)
}

// mdnsPort is the port of the connect service named inst; 0 when adb fails,
// lists none, or lists them with different ports (anything on the network can
// announce the name: then none is believed).
func mdnsPort(ctx context.Context, host pairing.ADB, inst string) int {
	ss, err := pairing.Services(ctx, host)
	if err != nil {
		return 0
	}
	port := 0
	for _, s := range ss {
		if s.Type != pairing.ConnectType || s.Name != inst {
			continue
		}
		n := addrPort(s.Addr)
		if n == 0 || port != 0 && n != port {
			return 0
		}
		port = n
	}
	return port
}

// wirelessPort reports whether p can be the wireless debugging port: from 1
// to 65535, and not the remote mode's 5555 (that is `adb tcpip`, and the phone
// may be connected through it).
func wirelessPort(p int) bool {
	return p >= 1 && p <= 65535 && strconv.Itoa(p) != remote.Port
}
```

Edit (paso 3) en `host/internal/cli/activephone.go`:

**Reemplazar:**
```go
	"github.com/hybriqtech/adbuddy/host/internal/adb"
)
```
**por:**
```go
	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/pairing"
)
```

Edit (paso 3) en `host/internal/cli/activephone.go`:

**Reemplazar:**
```go
func (p *activePhone) Serial() string { return p.Runner().Serial }
```
**por:**
```go
func (p *activePhone) Serial() string { return p.Runner().Serial }

// Host runs adb's host commands (mdns services, devices): the current runner
// without the phone's serial.
func (p *activePhone) Host() pairing.ADB {
	r := p.Runner()
	r.Serial = ""
	return r
}
```

Edit (paso 3) en `host/internal/cli/phone.go`:

**Reemplazar:**
```go
	l.remote = newRemoteLink(ctx, store, ctl, a, app.Log())
```
**por:**
```go
	l.remote = newRemoteLink(ctx, store, ctl, connOf(a), app.Log())
```

- [ ] **Step 4: GREEN.** `cd host && go build ./... && go vet ./internal/cli/ && gofmt -l . && go test -count=1 -timeout 5m -run 'TestDeviceInfo|TestTheSavedPort|TestTLSPort|TestChatPort' ./internal/cli/` → `gofmt` sin salida y `ok  	github.com/hybriqtech/adbuddy/host/internal/cli	0.463s` (réplica).

- [ ] **Step 5: test del cableado.** Prueba una función nueva (`connOf`), así que no tiene RED propio con el código de antes (no compilaría); su RED es una mutación comprobada: con `connOf` devolviendo siempre `nil` (`_ = c; return nil`) falla con `phone_remote_test.go:443: conn <nil>`.

Edit (paso 5) en `host/internal/cli/phone_remote_test.go`:

**Reemplazar:**
```go
	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/pairing"
```
**por:**
```go
	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/pairing"
```

Edit (paso 5) en `host/internal/cli/phone_remote_test.go`:

**Reemplazar:**
```go
func TestDeviceInfoFromAReplacedConnectionIsIgnored(t *testing.T) {
```
**por:**
```go
// serve hands startPhoneLink its activePhone, and the link finds the port
// through it (the serial it has now, adb without -s for mdns services). The
// fake phones of other tests have no connection: only the app's port counts.
func TestConnOfIsTheActivePhone(t *testing.T) {
	p := newActivePhone(adb.Runner{Path: "adb", ServerPort: 5038, Serial: phoneSerial})
	c := connOf(p)
	if c == nil || c.Serial() != phoneSerial {
		t.Fatalf("conn %v", c)
	}
	p.Switch("100.101.102.103:41234")
	if c.Serial() != "100.101.102.103:41234" {
		t.Fatalf("serial after a switch %q", c.Serial())
	}
	if h, ok := c.Host().(adb.Runner); !ok || h != (adb.Runner{Path: "adb", ServerPort: 5038}) || p.Serial() != "100.101.102.103:41234" {
		t.Fatalf("host %#v, phone %q", c.Host(), p.Serial())
	}
	if c := connOf(&appPhone{}); c != nil {
		t.Fatalf("a fake phone has a connection: %v", c)
	}
	g := newRemoteRig(t, config.Default(), "")
	g.r = newRemoteLink(g.ctx, g.store, g.ctl, nil, g.log)
	g.r.setServer(g.srv)
	g.r.onDeviceInfo(bridge.Message{}, bridge.DeviceInfo{TailscaleIP: "100.101.102.103", WifiADBPort: 41000})
	g.idle(t)
	if s := g.saved(t); s.TailscaleIP != "100.101.102.103" || s.TailscalePort != 41000 {
		t.Fatalf("without a connection saved %s:%d", s.TailscaleIP, s.TailscalePort)
	}
}

func TestDeviceInfoFromAReplacedConnectionIsIgnored(t *testing.T) {
```

`cd host && go vet ./internal/cli/ && go test -count=1 -timeout 5m -run 'TestConnOf|TestDeviceInfo|TestTheSavedPort|TestTLSPort|TestChatPort' ./internal/cli/` → `ok` (réplica: 1.351s).

- [ ] **Step 6: el comentario de `config.toml`.**

Edit (paso 6) en `host/internal/config/config.go`:

**Reemplazar:**
```go
	TailscalePort  int      `toml:"tailscale_port"`  // wireless debugging port read from the phone over the same connection; 0 = unknown
```
**por:**
```go
	TailscalePort  int      `toml:"tailscale_port"`  // wireless debugging port, from the host's connection to the phone (its ip:port or its mDNS service); 0 = unknown
```

(`gofmt` deja la línea así: el comentario va detrás de la etiqueta y no realinea las demás.)

- [ ] **Step 7: verificación de la tarea.** `cd host && go build ./... && go vet ./... && gofmt -l .` (sin salida), `go test -count=1 -timeout 5m ./internal/cli/... ./internal/config/...` (réplica: `ok … cli 94.961s`, `ok … config 0.235s`) y la suite completa `go test -count=1 -timeout 15m ./...` en segundo plano (réplica: todos los paquetes `ok`, ver "Réplica acumulada"). `git grep -n "readTLSPort\|tls\.port" -- host/` solo debe dar comentarios y el adb falso (`phone_remote.go`: el comentario de `phoneConn`; `phone_remote_test.go`: `connADB` y el comentario del test). Ninguna IP RFC 1918 en las líneas añadidas: `git diff -U0 -- host/ | grep '^+' | grep -E '(^|[^0-9.])(10|172\.(1[6-9]|2[0-9]|3[01])|192\.168)\.[0-9]+\.[0-9]+'` sin salida (el `"10.0.0."` de `TestDeviceInfoRemembersFewWarnedAddresses` es anterior y a propósito: una dirección que no es de Tailscale).

- [ ] **Step 8: commit.** `git add host/internal/cli/phone_remote.go host/internal/cli/phone_remote_test.go host/internal/cli/activephone.go host/internal/cli/phone.go host/internal/config/config.go` y `git commit` con el mensaje:

```
fix(host): el puerto de la depuración inalámbrica sale de la conexión, no de getprop (M5b-11)

El usuario shell no puede leer service.adb.tls.port (SELinux: getprop da
vacío con código 0), así que tailscale_port no se guardaba nunca. Ahora sale
del serial activo (ip:puerto) o, conectado por mDNS, del servicio
_adb-tls-connect._tcp de la misma instancia en adb mdns services; nunca el
5555 ni un puerto fuera de 1-65535, y sin puerto se conserva el guardado.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
```

---

### Task 2: la spec y los pendientes dicen de dónde sale el puerto

**Files:** los cuatro de `docs/` del mapa. Todos en CRLF en el árbol de trabajo: anclas de una línea.

- [ ] **Step 1: spec M5 §7.1.**

Edit (paso T2) en `docs/superpowers/specs/2026-10-01-adbuddy-m5-ayudas-remoto-design.md`:

**Reemplazar:**
```markdown
- **El host,** con el móvil conectado, lee el puerto él mismo (`getprop service.adb.tls.port`). Guarda `tailscale_ip` y `tailscale_port` en `config.toml`, como `address`. `findKnownPhone` prueba `tailscale_ip:tailscale_port` después de `address` y antes de esperar por mDNS. La comprobación de identidad (guid contra `ro.serialno`) no cambia.
```
**por:**
```markdown
- **El host,** con el móvil conectado, toma el puerto de la conexión que ya tiene, sin el shell del móvil: el usuario `shell` no puede leer `service.adb.tls.port` (SELinux, `adbd_prop`: `getprop` devuelve vacío con código 0; medido en la API 34, T1-12 del inventario de validaciones manuales; corregido en `fix/m5b-tls-port`). Si adb lista el móvil como `ip:puerto`, ese puerto; si lo lista por mDNS (`<instancia>._adb-tls-connect._tcp`), el del servicio `_adb-tls-connect._tcp` con esa misma instancia en `adb mdns services`, solo si todos sus anuncios dan el mismo puerto; por USB, o conectado por el 5555 del modo remoto, ninguno. Nunca guarda el 5555 ni un puerto fuera de 1-65535, y sin puerto conocido se queda el guardado. La IP de esa conexión no se guarda (puede ser la de casa): `tailscale_ip` es siempre la que manda la app, y el puerto de la depuración inalámbrica es el mismo en todas las interfaces mientras no se apague. El `wifiAdbPort` de la app solo cuenta si la conexión no da puerto, con las mismas reglas. Guarda `tailscale_ip` y `tailscale_port` en `config.toml`, como `address`. `findKnownPhone` prueba `tailscale_ip:tailscale_port` después de `address` y antes de esperar por mDNS. La comprobación de identidad (guid contra `ro.serialno`) no cambia.
```

- [ ] **Step 2: pendientes.**

Edit (paso T2) en `docs/superpowers/plans/pendientes-m3-m5.md`:

**Reemplazar:**
```markdown
      - T4: `readTLSPort` usa `context.Background` (pasar el ctx del enlace); `warned` sin tope.
```
**por:**
```markdown
      - T4 (**HECHO**: el puerto se busca con el ctx del enlace, `TestTLSPortFollowsTheLinkContext`, y `warned` tiene tope, `TestDeviceInfoRemembersFewWarnedAddresses`; `readTLSPort` ya no existe desde `fix/m5b-tls-port`): `readTLSPort` usaba `context.Background`; `warned` sin tope.
```

Edit (paso T2) en `docs/superpowers/plans/pendientes-m3-m5.md`:

**Reemplazar:**
```markdown
      - **Automatizado:** 11 `TestReadTLSPortParses`;
```
**por:**
```markdown
      - **Automatizado:** 11 `TestDeviceInfoTakesThePortFromTheConnection` y `TestTheSavedPortIsACandidateAddress` (desde `fix/m5b-tls-port`, plan `2026-10-03-adbuddy-m5b-tls-port.md`: el usuario shell no puede leer `service.adb.tls.port`, así que el puerto sale de la conexión del host, `ip:puerto` o mDNS; antes `TestReadTLSPortParses` probaba un `getprop` que en un móvil real da vacío);
```

Edit (paso T2) en `docs/superpowers/plans/pendientes-m3-m5.md`:

**Reemplazar:**
```markdown
      - **No automatizable (D, móvil o red reales):** valores reales de 11;
```
**por:**
```markdown
      - **No automatizable (D, móvil o red reales):** en 11, que `tailscale_port` sea el puerto de Ajustes y que `adb connect <ip-tailscale>:<tailscale_port>` conecte (validación 1 del plan `2026-10-03-adbuddy-m5b-tls-port.md`);
```

- [ ] **Step 3: validación 11 del plan de M5b.**

Edit (paso T2) en `docs/superpowers/plans/2026-10-01-adbuddy-m5b-modo-remoto.md`:

**Reemplazar:**
```markdown
11. **Puerto de depuración inalámbrica:** `getprop service.adb.tls.port` desde el host da el puerto vigente
```
**por:**
```markdown
11. **Puerto de depuración inalámbrica:** (**corregido en `fix/m5b-tls-port`:** el usuario shell no puede leer `service.adb.tls.port`, así que el host ya no usa `getprop` y toma el puerto de su conexión, `ip:puerto` o el servicio mDNS; lo que queda está en la validación 1 de `2026-10-03-adbuddy-m5b-tls-port.md`.) `getprop service.adb.tls.port` desde el host da el puerto vigente
```

- [ ] **Step 4: inventario de validaciones (solo si la rama lo tiene).** Si `git ls-files docs/superpowers/plans/2026-10-03-validaciones-manuales.md` no da nada (la rama sale de 7c4613c y ese fichero entró en `origin/main` después, 7519c6f), no se toca y el informe lo dice para quien integre. Si la rama ya lo tiene (el controlador hizo merge de `origin/main`):

Edit (paso T2c) en `docs/superpowers/plans/2026-10-03-validaciones-manuales.md`:

**Reemplazar:**
```markdown
  - Afecta a M5b-11 y, por arrastre, a M5b-12. En un Pixel de producción no hay `su`. Falta confirmarlo en Android 17.
```
**por:**
```markdown
  - Afecta a M5b-11 y, por arrastre, a M5b-12. En un Pixel de producción no hay `su`. Falta confirmarlo en Android 17.
  - **Corregido en `fix/m5b-tls-port`** (plan `2026-10-03-adbuddy-m5b-tls-port.md`): el host ya no usa `getprop`; toma el puerto del serial activo (`ip:puerto`) o del servicio mDNS `_adb-tls-connect._tcp` de la misma instancia.
```

Edit (paso T2c) en `docs/superpowers/plans/2026-10-03-validaciones-manuales.md`:

**Reemplazar:**
```markdown
| Defecto que hay que adjudicar; después, el valor real en Android 17 (sin `su`). Paso: la pasada de solo lectura. |
```
**por:**
```markdown
| Defecto corregido en `fix/m5b-tls-port` (el puerto sale de la conexión del host, no de `getprop`); queda comprobar en el Pixel que `tailscale_port` es el de Ajustes y que `adb connect <ip-tailscale>:<tailscale_port>` conecta. Paso: la pasada de solo lectura. |
```

- [ ] **Step 5: comprobar y commit.** Cada texto "Reemplazar" aparece una sola vez antes de editar (`grep -c`); después, `git diff --stat` solo lista esos ficheros y `git diff` no cambia finales de línea fuera de las líneas editadas (`git diff --ignore-cr-at-eol --stat` igual). `git commit` con el mensaje:

```
docs(m5b): el puerto de la depuración inalámbrica sale de la conexión del host (spec §7.1, pendientes)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
```

---

## Validaciones manuales pendientes (M5b-11/12, puerto de la depuración inalámbrica)

Solo con el móvil del usuario y su permiso explícito. Ninguna cambia nada en el móvil.

1. **El puerto guardado es el de Ajustes y llega por Tailscale.** En casa, con la depuración inalámbrica encendida (serial mDNS) y la app con Tailscale: `adbuddy serve`; `config.toml` pasa a tener `tailscale_port` igual al puerto de la pantalla "Depuración inalámbrica". Después, desde el PC, `adb connect <ip-tailscale>:<tailscale_port>` contra el servidor adb de ADBuddy conecta (comprueba la premisa del ruling 3: el puerto TLS escucha también en la interfaz de Tailscale) y `adb disconnect` lo deja como estaba. _Automatizado: la lógica, `TestDeviceInfoTakesThePortFromTheConnection` y `TestTheSavedPortIsACandidateAddress`; resto: Android 17 real y Tailscale, D (Pixel). No sirve el emulador: su serial es `emulator-*` (sin puerto ni mDNS) y su red está detrás del NAT del emulador._
2. **`adb mdns services` del adb real** lista el servicio `_adb-tls-connect._tcp` con la misma instancia que el serial del móvil (el formato ya lo usan `setup` y `pairing.Connect`, probados en el Pixel en M1b; confirmarlo junto con la 1). _Automatizado: el formato en `TestSetupSavesTheStableSerial` y en este plan; resto: D (Pixel)._

## Réplica acumulada tarea a tarea

Réplica a mano sobre `git archive origin/main host` (2bb199a; `host/` idéntico a 7c4613c) en el scratchpad:

- **Tarea 1, paso 1-2 (RED):** `go vet ./internal/cli/` limpio; `go test -count=1 -timeout 5m -run 'TestDeviceInfo|TestTheSavedPort|TestTLSPort|TestChatPort' ./internal/cli/` → `FAIL` con los fallos citados en el Step 2 (todos por el puerto no guardado o por la llamada a `getprop`; ninguno por compilación ni `panic`).
- **Paso 3-4 (GREEN):** `go build ./...`, `go vet ./internal/cli/`, `gofmt -l .` limpios; el mismo `-run` → `ok … 0.463s`.
- **Paso 5:** `ok … 1.351s`; mutación `connOf` → `nil`: `FAIL`, `phone_remote_test.go:443: conn <nil>`; restaurado: `ok`.
- **Paso 6-7:** `go build ./... && go vet ./... && gofmt -l .` limpios; `go test -count=1 -timeout 5m ./internal/cli/... ./internal/config/...` → `ok … cli 94.961s`, `ok … config 0.235s`; suite completa `go test -count=1 -timeout 15m ./...` → los 34 paquetes con tests en `ok` (`cli 95.482s`), `EXIT 0`.
- **Réplica desde el texto del plan:** sobre `git archive HEAD` completo (7c4613c; los tests de `bridge` y `cli` leen `protocol/` y `docs/`, así que no vale archivar solo `host/`), el script aplica los pasos 1, 3, 5 y 6 en orden: 24 `--- FAIL` tras el paso 1 (los del Step 2), `ok` tras el 3, `go build`/`go vet`/`gofmt -l` limpios tras el 6, y los cinco ficheros quedan byte a byte iguales a los de la réplica a mano. La suite completa citada arriba es la de este árbol.
- **Tarea 2:** cada ancla comprobada única en el árbol de trabajo (CRLF) de la rama (`grep -c` = 1 en la spec, en `pendientes-m3-m5.md` y en el plan de M5b) y, para el paso 4, en `origin/main:docs/superpowers/plans/2026-10-03-validaciones-manuales.md`. No toca código: la suite no cambia.

## Autorrevisión del plan

### Cobertura frente al encargo

- Puerto de la conexión activa (`ip:puerto` del serial) → Tarea 1, `addrPort`; `cfg.Address` → descartado como fuente aparte con motivo (ruling 1); mDNS filtrado por la identidad del móvil y reutilizando `pairing.Services`/`ConnectType` → `mdnsPort` (instancia exacta del serial vivo, ruling 2). Orden decidido y justificado (ruling 1: la forma del serial elige la fuente; la app, después).
- Casos adversos: 5555 del modo remoto (serial, mDNS y app), validación 1-65535 (70000, 0), serial mDNS sin puerto (va por mDNS), IP de la LAN no se guarda como `tailscale_ip` (todos los casos comprueban `100.101.102.104`, la de la app), nunca sobrescribir un puerto bueno con 0 ni con 5555 (los casos sin puerto exigen el 40000 guardado), choque de nombres mDNS, otro móvil, servicio de emparejamiento, USB, no-IP, adb que falla, contexto del enlace.
- Hilo y runner: `save` sigue en `drain`; el runner de host sale de `activePhone.Host()`, sin tocar la firma de `startPhoneLink` ni romper `phone.go:131` ni los tests (ruling 5).
- `getprop` quitado, con motivo (ruling 4). Spec §7.1 actualizada (Tarea 2).
- RED real con adb falso (`getprop` vacío rc 0, conexión `ip:puerto`, mDNS) en `phone_remote_test.go`.

### Placeholders y tipos

Sin placeholders. `phoneConn` lo implementan `*activePhone` (comprobado al compilar) y `*connADB` (tests). `pairing.ADB` y `phoneapp.ADB` tienen el mismo método `Run`: `adb.Runner` y `connADB` valen para los dos. `remote.Port` es la cadena `"5555"`: `wirelessPort` compara con `strconv.Itoa(p)`.

### Riesgos abiertos para el revisor Opus

- **Premisa de la interfaz:** que el puerto TLS escuche también por Tailscale (ruling 3) no está medido: S1 solo probó el 5555 por Tailscale. Si no escuchara, `tailscale_ip:tailscale_port` sería un candidato que falla (lo de hoy, sin peor efecto) y la validación 1 lo diría.
- **Caché de mDNS:** tras reactivar la depuración, adb puede listar un rato el anuncio viejo junto al nuevo con la misma instancia; con puertos distintos no se guarda nada (ruling 2) hasta que caduque. Coste: un `device.info` más tarde.
- **`connOf` por aserción de tipo:** explícito en el código y vigilado por `var _ phoneConn = (*activePhone)(nil)` y `TestConnOfIsTheActivePhone`; la alternativa (un parámetro más en `startPhoneLink`) cambiaría 8 llamadas sin ganar seguridad.
- `service.adb.tcp.port` (el del modo remoto y el invariante) **sí** se lee como `shell` (S1 en el Pixel: `5555`, `0`): este defecto no afecta al controlador del modo remoto.

### Decisiones y dudas para el controlador

- El inventario `2026-10-03-validaciones-manuales.md` está en `origin/main` (2bb199a) pero no en esta rama (7c4613c): la Tarea 2 lo actualiza solo si la rama lo tiene; si no, lo hace quien integre (los dos pares están en el paso 4).
- La app sigue sin mandar `wifiAdbPort` (no hay API pública): no se toca `android/`.
