# ADBuddy v1 · Diferidos L (contención de escritura y envío: `pcwrite` con `pathguard`, FIFO en `send`, cuota de `outbox/` y cerrojo de `serve`) · Plan de implementación

> **Para quien ejecute:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`. Los pasos usan casillas (`- [ ]`). Implementadores y re-revisiones en Sonnet (`adbuddy-implementer`, `adbuddy-rereviewer`); `adbuddy-reviewer-opus` para las tareas marcadas 🔒 (daemon, IPC y `serve`) y para la revisión final; `adbuddy-reviewer` para las demás. Nunca Haiku. **Antes de la tarea 1, `adbuddy-reviewer-opus` revisa este plan completo, código incluido** (CLAUDE.md), y el plan se corrige con sus hallazgos. Todas las tareas son de `host/` y van en serie (un solo implementador en el árbol).

**Meta:** cerrar tres diferidos del triage de la v1 (V09, V19 y V22, fila L) y el diferido barato del cerrojo de #19: que la guarda de "no escribir fuera del entorno" del daemon sea la misma que la de `adbuddy send`, que una FIFO puesta a destiempo no cuelgue `adbuddy send` ni el cierre de `serve`, que el agente no pueda llenar el disco con copias en `outbox/` ni hacer crecer sin fin el registro de adjuntos, y que un segundo `serve` se pare antes de arrancar nada.

**Arquitectura:** cambios locales, sin piezas nuevas. V09 borra la copia de `pcwrite.go` y llama a `pathguard` (el código era idéntico línea a línea). V19 abre con `safeio.ReadFlags`, el `O_NONBLOCK` que trae #21. V22 añade `outboxQuota` (2 GiB, como `inbox/`), con los envíos por turnos en un `copyToOutbox` nuevo, y acota el registro de adjuntos a 2000 entradas, olvidando primero las más viejas. El cerrojo se exporta como `ipc.Lock` (tipo `ipc.ServeLock`) y `ipc.ListenLocked`; `ipc.Listen` mantiene su firma. **No cambian** el protocolo de cable (IPC ni canal con la app), la autenticación del IPC, `classify/` ni los textos del catálogo.

**Tecnologías:** Go 1.27 (`host/`). Nada nuevo en `go.mod`.

**Fuentes:** triage `C:\Users\Jose\orca\workspaces\adbuddy\skimmer\.superpowers\sdd\2026-10-02-adbuddy-v1-diferidos\triage.md` (filas V09, V19 y V22; reparto L) · encargo `.superpowers/sdd/2026-10-02-adbuddy-v1-diferidos-b/plan-brief-l.md` · spec M3 `docs/superpowers/specs/2026-10-01-adbuddy-m3-chat-design.md` (§5.2 `adbuddy send`, §6 límites de ficheros: 200 MiB por fichero y 2 GiB de `inbox/`) · spec M6b (§ skills: los informes se escriben en `outbox/` y se entregan con `adbuddy send`) · plan B `docs/superpowers/plans/2026-10-02-adbuddy-v1-diferidos-b.md` (Tarea 4: el cerrojo) · rama `origin/fix/v1-ola2-h` (#21: `safeio`, `TestMain` de `ipc`, captura del par en `handle()`).

**Precondición:** el plan se ejecuta sobre `main` con **#19** (`fix/v1-diferidos-b`: el cerrojo `adbuddy.lock` de `ipc`) y **#21** (`fix/v1-ola2-h`: `host/internal/safeio`) ya integradas. Antes de la tarea 1, comprobar `ls host/internal/safeio/flags_unix.go host/internal/ipc/lock.go host/internal/ipc/peer.go` (los tres existen) y `grep -n "func tryLock" host/internal/ipc/lock.go`. Si falta #21, la Tarea 2 no compila: parar y avisar al controlador.

**Verificación del plan (réplica acumulada):** ver "Réplica acumulada tarea a tarea" al final: un script aplicó este documento, en orden y tarea a tarea, sobre un `git archive` de `fix/v1-diferidos-b` (b4e528c) **fusionado con `origin/fix/v1-ola2-h` (51c2cf2)**, que es lo que tendrá `main`. Los ficheros nuevos son byte a byte los del plan; los cambios a ficheros existentes son pares "reemplazar / por" para la herramienta Edit (cada texto "reemplazar" es único en su fichero) o "añadir al final".

## Global Constraints

- Rama nueva desde `main` (con #19 y #21). Commits con identidad local hybriqtech (nunca `git config --global`) y trailer **exacto** `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` (también en fixups; comprobarlo con `git log -1 --format=%B`). Nunca `--no-verify`, nunca `--force`, sin `git stash`. Sin push ni PR salvo que lo pida el controlador; a `main` integra el usuario. `git grep` de seriales e IPs antes de cualquier push.
- **Sin IPs RFC 1918** en código, tests ni docs (`100.64.0.5`, `198.51.100.x`, `192.0.2.x` si hiciera falta alguna; este plan no usa ninguna). Sin seriales reales.
- **El móvil del usuario no se toca.** Los tests usan `bareLink` (chat sin app), el `env` del daemon con su adb falso, `isolate` y un adb que no existe. Lo que necesita un móvil o un emulador va a "Validaciones manuales pendientes" (desde la 42) y solo se hace con permiso explícito del usuario.
- **Las guardas fallan cerrado:** ningún cambio de este plan deja pasar algo que hoy se rechaza. Los tests existentes de `daemon/`, `pathguard/`, `cli/` (`TestSend*`, `TestDownload*`, `TestServe*`) e `ipc/` siguen en verde sin tocarlos (salvo el añadido de la Tarea 5 a `TestServeWaitsForRequestsInFlight`).
- **IPC:** ni el formato de las tramas, ni `Request`/`Exit`, ni la comprobación del secreto cambian. `ipc.Listen` conserva su firma (la usan los tests de `ipc`, de `cli` y el `serveOnce` de `peer_test.go` de #21). **No se declara otro `TestMain` en `ipc`** (#21 añade uno en `peer_test.go`). `handle()` y `Conn` no se tocan (#21 los cambia).
- `host/internal/classify/` no se toca; `testdata/habitual.jsonl` intacto.
- **No se tocan:** `host/internal/cli/doctor.go` (V20, otro agente), `host/internal/i18n/catalog/*.toml`, `docs/troubleshooting.md`, `host/internal/cli/remotecmd_test.go`. `docs/superpowers/plans/pendientes-m3-m5.md` solo recibe una sección **al final** (Tarea 6).
- **Textos:** los errores nuevos son para el agente (salida de `adbuddy send` y causas técnicas de `ipc`) y van en inglés sin catálogo, como los demás de `send.go` y de `ipc` (spec M6c §3). Ningún texto nuevo de cara a la persona. Lista en "Textos nuevos".
- **Cada tarea compila y pasa sus tests por sí sola;** nada usa símbolos de tareas posteriores.
- **Go:** `cd host && go build ./... && go vet ./... && gofmt -l . && go test -count=1 -timeout 5m ./<paquete>/...`; la suite completa (`-timeout 15m ./...`) en el último paso de cada tarea. Las tareas con código por sistema además: `GOOS=linux go vet ./<paquete> && GOOS=darwin go vet ./<paquete>`. Este PC no corre `-race`: la concurrencia se razona en cada tarea y la corre la CI. Los tests que solo existen en Unix (FIFO) se omiten aquí y los ejerce la CI de Linux y macOS.
- `docs/superpowers/plans/pendientes-m3-m5.md` está en **CRLF** (`core.autocrlf=true`); `host/**` es LF (`.gitattributes`).
- Pasar a los revisores límites de tiempo (`go test -timeout 5m`, tope de 30 minutos por revisión).
- Windows con Git Bash: sin `sleep` en primer plano; los comandos largos con timeout amplio o en segundo plano.

## Rulings de este plan (van al ledger tal cual)

1. **Ruling: `daemon/pcwrite.go` usa `pathguard.ResolveLinks`, `pathguard.Within` y `pathguard.ReparseBelow` y borra su copia; `hasDotDot` se queda en el daemon.** — Las dos copias eran idénticas línea a línea (comprobado: mismo cuerpo, mismos comentarios), así que no cambia ninguna decisión: es quitar la posibilidad de que diverjan. `hasDotDot` es propia del `cwd` del agente (`send` limpia la ruta con `filepath.Clean` antes), y `pathguard` no la tiene. Un test de sintaxis (`TestPCWriteUsesPathguard`) impide que vuelva una copia, y uno de Windows (`TestPullThroughAJunctionSaysSo`) fija el texto de la junction, que ningún test miraba. — Si me equivoco: un cambio futuro de `pathguard` afecta a la vez a `adb pull`/`bugreport` y a `adbuddy send` (es lo que se quiere); el comentario del paquete lo dice.
2. **Ruling: `adbuddy send` abre el fichero con `safeio.ReadFlags` (`O_RDONLY|O_NONBLOCK` en Unix, `O_RDONLY` en Windows) en vez de `os.Open`.** — Entre el `Lstat` que vio un fichero regular y el `open`, el agente puede poner una FIFO: `os.Open` esperaría un escritor para siempre, con el manejador del daemon y el cierre de `serve` detrás (`Close` espera a los manejadores). Con `O_NONBLOCK` la apertura vuelve al momento y el `Stat` de después (que ya existía, con `SameFile`) la rechaza como "changed while it was being opened". Para un fichero regular `O_NONBLOCK` no cambia nada al leer. Se reutiliza la constante de #21 (`safeio`), que hace lo mismo para el diario: una sola forma de abrir sin bloquear. **Windows:** no hay FIFOs en el sistema de ficheros; su residual, la ventana entre `ReparseBelow` y `Lstat` en la que una carpeta del camino puede volverse junction (la segunda resolución tampoco la ve, porque `EvalSymlinks` no sigue junctions), queda documentado en el código y en pendientes, sin cerrar: cerrarlo pide `GetFinalPathNameByHandle` sobre el handle abierto y la raíz resuelta del mismo modo. — Si me equivoco: en un Unix raro donde `O_NONBLOCK` cambie la lectura de un fichero regular, los tests de `send` de la CI (que leen y copian) fallarían a la vista.
3. **Ruling: `outbox/` tiene una cuota de 2 GiB (`outboxQuota`), que cuenta los ficheros regulares de su primer nivel, también los que escribe el propio agente; un envío que no cabe es `TOO_LARGE` con "the outbox folder is full", y los envíos van por turnos desde la comprobación hasta el final de la copia.** — 2 GiB es el mismo tope que la spec M3 §6 da a `inbox/` (diez copias del fichero más grande, 200 MiB) y el que ya usa `inboxQuota`, que se aplica igual: se mide la carpeta en el momento de escribir y se libera borrando ficheros (aquí, los que el móvil ya tiene). Cuenta lo del agente porque `outbox/` es también donde las skills (spec M6b) dejan sus informes antes de enviarlos, y la copia va al mismo disco; las subcarpetas no cuentan (las copias van siempre al primer nivel, y lo que el agente escribe en otras carpetas es suyo, como el resto del entorno). Se comprueba antes de copiar con el tamaño medido y después con lo copiado (un fichero que crece entre medias). El turno (`filesState.sendMu`) evita que dos envíos a la vez quepan los dos en un hueco de uno; no se tiene mientras se llama a la sesión ni al servidor del canal. Se reutiliza el código `TOO_LARGE` (el protocolo ya lo usa para `inbox/` lleno) para no inventar uno nuevo que `docs/troubleshooting.md` no conoce. — Si me equivoco: un agente que envía mucho y no limpia `outbox/` recibe `TOO_LARGE` con el motivo y el arreglo en el texto; subir el tope es cambiar una variable.
4. **Ruling: el registro de adjuntos guarda como mucho `maxRegistered = 2 × keptChat × maxChatAttachments = 2000` y olvida primero el más viejo; volver a poner un id lo cambia en su sitio.** — Hoy el registro solo crece (`put` y `get`, sin borrar nunca), con hasta 16 KiB de miniatura por entrada. El historial que la app ve son los últimos 100 mensajes con hasta 10 adjuntos cada uno (1000 ids); se guarda otro tanto para lo que espera su mensaje (subidas y capturas aún sin `chat.send`, envíos del agente con la app fuera). Un adjunto olvidado responde `not_found` a `file.get` y a `chat.send` (el mismo error que un id desconocido) y una subida repetida de uno olvidado se vuelve a subir; su fichero sigue en disco. No se vacía al cambiar de conversación: la app puede tener aún los mensajes anteriores y pedirlos. — Si me equivoco: con más de 1000 adjuntos pendientes a la vez, los más viejos fallan con `not_found` y la persona los vuelve a adjuntar.
5. **Ruling: `serve` toma el cerrojo de la carpeta de datos con `ipc.Lock` justo después de `serveRunning` y se lo da a `ipc.ListenLocked`; `ipc.Listen` sigue igual por fuera (toma el cerrojo y llama a `ListenLocked`).** — Hoy el cerrojo se toma dentro de `Listen`, cuando `serve` ya preparó el entorno, habló con adb y arrancó los vigilantes del móvil y del modo remoto (`serve.go`, `go watchRemote`/`go watchPhone` antes de `ipc.Listen`): dos `serve` a la vez llegaban los dos hasta ahí. Ahora el segundo sale con `ALREADY_RUNNING` antes de tocar nada. El `Server` suelta el cerrojo en `Close`, después de quitar el socket (como en #19); un `defer lock.Close()` en `serve` cubre todos los demás caminos de salida, y `Close` del cerrojo es idempotente. Si `ListenLocked` falla, el cerrojo sigue siendo de quien lo pasó; un cerrojo ya soltado se rechaza (nunca se sirve sin cerrojo). Un error al tomarlo que no sea "ya lo tiene otro" sale como `SOCKET_FAILED`, igual que cuando lo daba `Listen`. `serveRunning` se queda: un daemon de una versión anterior no toma el cerrojo pero contesta al ping. — Si me equivoco: con una carpeta de datos sin cerrojos de fichero (red rara), `serve` sale antes (ya salía en `Listen`).

## Verificación de los ítems del encargo (contra el código de b4e528c + #21)

| Ítem | ¿Real? | Qué hace este plan |
|---|---|---|
| V09 copia de `resolveLinks`/`within`/`reparseBelow` en `pcwrite.go` (`:81`, `:102`, `:117`) | Sí. Las tres funciones son idénticas a las de `pathguard` (comentarios incluidos); `pathguard` dice de sí misma que es "la lógica de pcwrite.go para el código que no vive en el daemon". Hoy no divergen | Tarea 1 |
| V19 FIFO entre `Lstat` y `os.Open` (`send.go:212`) | Sí. `openInEnvironment` ya rechaza una FIFO que estaba (Lstat) y algo cambiado tras abrir (`Stat` + `SameFile`), pero el `os.Open` se queda esperando un escritor si la FIFO aparece en medio. El residual de Windows (junction entre `ReparseBelow` y `Lstat`) existe y ya está en pendientes (M3a) | Tarea 2 (Windows: documentado, no cerrado) |
| V22 `outbox/` sin cuota | Sí. `sendFile` copia hasta 200 MiB cada vez sin mirar cuánto hay; solo `inbox/` tiene cuota (`inboxQuota`, 2 GiB, `phone_files.go:53`, aplicada en `beginUploadLocked` y en la captura) | Tarea 3 |
| V22 registro de adjuntos que no se vacía | Sí. `attachmentRegistry` (`phone_chat.go:37`) solo tiene `put` y `get` | Tarea 4 |
| Cerrojo tarde en `serve` (diferido de #19) | Sí. `ipc.Lock` **no existe** todavía en la rama: #19 dejó el cerrojo privado (`tryLock`, `lockPath`) dentro de `Listen` (`server.go:205`). `serve` arranca `watchRemote` y `watchPhone` (`serve.go:202-209`) antes de `ipc.Listen` (`:236`) | Tarea 5 |

## Fuera de alcance (con motivo)

- **Cerrar la ventana de junction de `send` en Windows** (V19, residual): necesita comparar la ruta final del handle abierto (`GetFinalPathNameByHandle`) con la raíz del entorno resuelta del mismo modo (si la carpeta del usuario pasa por una junction, las dos rutas tienen que salir igual o habría rechazos falsos). Es un cambio de diseño de la guarda con su propia revisión; el encargo pide documentarlo. Queda en pendientes (Tarea 6).
- **Usar `safeio.OpenRegular` entero en `openInEnvironment`:** haría lo mismo, pero junta en `ErrNotRegular` lo que hoy son dos respuestas distintas para el agente (`NOT_A_FILE` frente a "changed while it was being opened") y no deja poner el gancho de test entre `Lstat` y `open`. Se reutiliza solo la constante de apertura.
- **Borrar copias viejas de `outbox/` automáticamente:** el host no sabe qué ficheros ya tiene el móvil (la app los guarda en su caché a su ritmo), y el agente puede haber dejado ahí los suyos. La cuota avisa y el agente o la persona borran.
- **La fila `TOO_LARGE` de `docs/troubleshooting.md`** (solo menciona los 200 MiB) no se toca por el encargo: queda anotado en pendientes para el dueño de los docs.

## Textos nuevos

Ninguno de cara a la persona; nada entra en el catálogo. En inglés a propósito (salida para el agente y causas técnicas, spec M6c §3):

1. **Tarea 3**, `cli/send.go`, error de `adbuddy send` con código `TOO_LARGE`: `the outbox folder is full (at most 2048 MiB): delete from it the files the phone already has` (el número sale de `outboxQuota>>20`).
2. **Tarea 5**, `ipc/server.go`, causa técnica de `ListenLocked` con un cerrojo ya soltado: `the daemon lock is not held` (no sale en `serve`, que siempre pasa un cerrojo tomado; si saliera, iría dentro de `SOCKET_FAILED` como las demás causas de `Listen`).

## Mapa de ficheros

**Modificados:** `host/internal/daemon/pcwrite.go`, `host/internal/pathguard/pathguard.go` (T1) · `host/internal/cli/send.go` (T2, T3) · `host/internal/cli/phone_files.go` (T3) · `host/internal/cli/phone_chat.go` (T4) · `host/internal/ipc/lock.go`, `host/internal/ipc/server.go`, `host/internal/cli/serve.go`, `host/internal/cli/serve_test.go` (T5) · `docs/superpowers/plans/pendientes-m3-m5.md` (T6, solo al final).

**Nuevos (tests):** `host/internal/daemon/pcwrite_guard_test.go` (T1) · `host/internal/cli/send_swap_test.go`, `host/internal/cli/send_swap_unix_test.go`, `host/internal/cli/send_swap_other_test.go` (T2) · `host/internal/cli/send_outbox_test.go` (T3) · `host/internal/cli/phone_chat_registry_test.go` (T4) · `host/internal/ipc/servelock_test.go` (T5).

---

### Task 1: `pcwrite.go` usa `pathguard` y no guarda copia propia (V09) 🔒

**Revisor:** `adbuddy-reviewer-opus` (daemon: guarda de escritura en el PC).

**Files:**
- Create: `host/internal/daemon/pcwrite_guard_test.go`
- Modify: `host/internal/daemon/pcwrite.go` (imports, comentario de `checkPCWrites`, bucle de `checkPCWrite`, borrar `resolveLinks`/`within`/`reparseBelow`), `host/internal/pathguard/pathguard.go` (comentario del paquete)

**Interfaces:**
- Consumes: `pathguard.ResolveLinks(p string) (string, error)`, `pathguard.Within(root, p string) (string, bool)`, `pathguard.ReparseBelow(root, rel string) bool` (existentes).
- Produces: nada nuevo. Desaparecen las funciones privadas `resolveLinks`, `within` y `reparseBelow` del paquete `daemon` (nadie más las usa: `grep -rn "resolveLinks\|reparseBelow" host/internal/daemon` solo las encuentra en `pcwrite.go`).

- [ ] **Step 1: Tests.** **Crear `host/internal/daemon/pcwrite_guard_test.go`:**

```go
package daemon

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// V09: the daemon's check of adb pull and adbuddy send share one guard
// (pathguard). The daemon keeps no copy of it that could drift: no function of
// the package resolves links, compares roots or looks for reparse points on
// its own, and pcwrite.go imports pathguard.
func TestPCWriteUsesPathguard(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	copies := map[string]bool{"resolveLinks": true, "within": true, "reparseBelow": true}
	imports := false
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && copies[fn.Name.Name] {
				t.Errorf("%s declares %s: use pathguard instead", name, fn.Name.Name)
			}
		}
		if name != "pcwrite.go" {
			continue
		}
		for _, im := range f.Imports {
			if p, _ := strconv.Unquote(im.Path.Value); p == "github.com/hybriqtech/adbuddy/host/internal/pathguard" {
				imports = true
			}
		}
	}
	if !imports {
		t.Error("pcwrite.go does not use pathguard")
	}
}

// V09: a junction inside the environment (Windows: filepath.EvalSymlinks does
// not follow it) is refused and named, as a folder, as a file under it, as a
// path under it that does not exist yet and as the working directory.
func TestPullThroughAJunctionSaysSo(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junctions are a Windows matter")
	}
	e := newEnv(t)
	outside := t.TempDir()
	j := filepath.Join(e.ws, "j")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", j, outside).CombinedOutput(); err != nil {
		t.Skipf("mklink /J: %v %s", err, out)
	}
	const why = "through a junction or another link ADBuddy does not follow"
	for _, target := range []string{"j", "j/x.txt", "j/new/y.txt", "J/x.txt"} {
		if d := deniedPC(t, e, e.ws, "pull", "/sdcard/x", target); !strings.Contains(d, why) {
			t.Errorf("%s: detail %q", target, d)
		}
	}
	if d := deniedPC(t, e, j, "pull", "/sdcard/x"); !strings.Contains(d, why) {
		t.Errorf("working directory in the junction: %q", d)
	}
}
```

- [ ] **Step 2: Ver que falla el de sintaxis y pasa el de la junction** (caracterización: fija lo que ya hace).

```bash
cd host && go test -count=1 -timeout 5m ./internal/daemon -run 'TestPCWriteUsesPathguard|TestPullThroughAJunctionSaysSo' -v
```

Esperado (réplica, Windows; en Linux y macOS el de la junction sale en `SKIP`):

```
=== RUN   TestPCWriteUsesPathguard
    pcwrite_guard_test.go:37: pcwrite.go declares resolveLinks: use pathguard instead
    pcwrite_guard_test.go:37: pcwrite.go declares within: use pathguard instead
    pcwrite_guard_test.go:37: pcwrite.go declares reparseBelow: use pathguard instead
    pcwrite_guard_test.go:50: pcwrite.go does not use pathguard
--- FAIL: TestPCWriteUsesPathguard (0.00s)
=== RUN   TestPullThroughAJunctionSaysSo
--- PASS: TestPullThroughAJunctionSaysSo (0.06s)
FAIL
```

- [ ] **Step 3: Usar `pathguard` y borrar la copia.**

**Editar `host/internal/daemon/pcwrite.go`** (cambio 1 de 4): reemplazar

```go
import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
)
```

por

```go
import (
	"path/filepath"
	"strings"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/pathguard"
)
```

**Editar `host/internal/daemon/pcwrite.go`** (cambio 2 de 4): reemplazar

```go
// never trusted: it must itself be inside the environment. Paths are resolved
// through links where they exist, and compared without case on Windows. The
// answer is the reason of the 🔴 question, in both languages.
```

por

```go
// never trusted: it must itself be inside the environment. Paths are resolved
// through links where they exist, and compared without case on Windows, by
// pathguard, the guard adbuddy send uses too. The answer is the reason of the
// 🔴 question, in both languages.
```

**Editar `host/internal/daemon/pcwrite.go`** (cambio 3 de 4): reemplazar

```go
		real, err := resolveLinks(p)
		if err != nil {
			return cat(outside, same(p+" ("+err.Error()+")"))
		}
		rel, inside := within(root, real)
		switch {
		case !inside:
			return cat(outside, same(p))
		case reparseBelow(root, rel):
```

por

```go
		// The part that does not exist yet (the file adb will create) is
		// appended to its deepest existing folder, resolved; a link that
		// exists but points nowhere is an error: adb would create its target,
		// wherever that is.
		real, err := pathguard.ResolveLinks(p)
		if err != nil {
			return cat(outside, same(p+" ("+err.Error()+")"))
		}
		rel, inside := pathguard.Within(root, real)
		switch {
		case !inside:
			return cat(outside, same(p))
		case pathguard.ReparseBelow(root, rel):
```

**Editar `host/internal/daemon/pcwrite.go`** (cambio 4 de 4): reemplazar

```go
	return false
}

// resolveLinks resolves the links in p. The part that does not exist yet (the
// file adb will create) is appended to its deepest existing folder, resolved.
// A link that exists but points nowhere is an error: adb would create its
// target, wherever that is.
func resolveLinks(p string) (string, error) {
	var rest []string
	for {
		real, err := filepath.EvalSymlinks(p)
		if err == nil {
			return filepath.Join(append([]string{real}, rest...)...), nil
		}
		if _, lerr := os.Lstat(p); lerr == nil {
			return "", err // it exists, but its links do not resolve
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "", err
		}
		rest = append([]string{filepath.Base(p)}, rest...)
		p = parent
	}
}

// within reports whether p is root or inside it, and p relative to root.
// Windows paths are compared without case.
func within(root, p string) (string, bool) {
	if runtime.GOOS == "windows" {
		root, p = strings.ToLower(root), strings.ToLower(p)
	}
	rel, err := filepath.Rel(root, p)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// reparseBelow reports whether a folder or file on the way from root to rel
// is a link that filepath.EvalSymlinks left unresolved: on Windows it does
// not follow junctions (mount points), which anyone can create with
// `mklink /J` and which may point anywhere.
func reparseBelow(root, rel string) bool {
	if rel == "." {
		return false
	}
	p := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		p = filepath.Join(p, part)
		fi, err := os.Lstat(p)
		if err != nil {
			return false // the rest does not exist yet
		}
		if fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return true
		}
	}
	return false
}
```

por

```go
	return false
}
```

**Editar `host/internal/pathguard/pathguard.go`**: reemplazar

```go
// Package pathguard decides whether a path stays inside a folder once links
// are followed. It is the logic of the daemon's checks of adb pull (pcwrite.go),
// for the code that does not live in the daemon.
```

por

```go
// Package pathguard decides whether a path stays inside a folder once links
// are followed. It is the one guard of the daemon's checks of adb pull and
// adb bugreport (daemon/pcwrite.go) and of adbuddy send (cli/send.go): a
// change here changes both.
```

- [ ] **Step 4: Ver que pasan, y todo lo de la guarda que ya había.**

```bash
cd host && gofmt -l ./internal/daemon ./internal/pathguard && go vet ./internal/daemon ./internal/pathguard && GOOS=linux go vet ./internal/daemon && GOOS=darwin go vet ./internal/daemon && go test -count=1 -timeout 5m ./internal/daemon ./internal/pathguard
```

Esperado: sin salida de `gofmt` ni de los `vet`; `ok` en los dos paquetes. Siguen en verde, sin tocarlos, los de `pcwrite_test.go` (`TestPullFromOutsideTheEnvironmentIsRed`, `TestPullThroughALinkOutOfTheEnvironmentIsRed`, `TestPullOverADanglingLinkIsRed`, `TestPullComparesPathsWithoutCaseOnWindows`, `TestPullWithDotDotAfterALinkIsRed`, `TestPullOverTheEnvironmentsOwnFilesIsRed`…), `TestPCWriteReasonWithASystemErrorIsInBothLanguages` y los de `pathguard_test.go`.

- [ ] **Step 5: Suite completa.**

```bash
cd host && go build ./... && go vet ./... && gofmt -l . && go test -count=1 -timeout 15m ./...
```

Esperado: todo `ok`.

- [ ] **Step 6: Commit.**

```bash
git add host/internal/daemon/pcwrite.go host/internal/daemon/pcwrite_guard_test.go host/internal/pathguard/pathguard.go
git commit -m "refactor(host): pcwrite usa pathguard y deja de tener su propia copia de la guarda (V09)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Equivalencia:** el cuerpo de las tres funciones borradas y el de las de `pathguard` es el mismo carácter a carácter (compruébalo antes de borrar con `diff <(sed -n '/^func resolveLinks/,/^}/p' host/internal/daemon/pcwrite.go) <(sed -n '/^func ResolveLinks/,/^}/p' host/internal/pathguard/pathguard.go)`: solo cambia la mayúscula del nombre; lo mismo con `within`/`Within` y `reparseBelow`/`ReparseBelow`). El comentario que explicaba por qué un enlace roto es un error para `adb pull` pasa al bucle, que es donde importa.

---

### Task 2: `adbuddy send` no se cuelga con una FIFO puesta antes de abrir (V19)

**Revisor:** `adbuddy-reviewer` (cambia el modo de apertura de un fichero ya comprobado; la guarda de rutas no cambia).

**Files:**
- Create: `host/internal/cli/send_swap_test.go`, `host/internal/cli/send_swap_unix_test.go`, `host/internal/cli/send_swap_other_test.go`
- Modify: `host/internal/cli/send.go` (import, `openInEnvironment` y su comentario, gancho `beforeOpen`)

**Interfaces:**
- Consumes: `safeio.ReadFlags` (#21, `host/internal/safeio/flags_*.go`: `os.O_RDONLY|syscall.O_NONBLOCK` en Unix, `os.O_RDONLY` en los demás). Helpers de test existentes: `bareLink`, `writeIn`, `sendReq`, `wantSendError`, `outboxNames` (`send_test.go`), `setVar` (`setup_test.go`).
- Produces: `var beforeOpen = func() {}` (gancho de test en `send.go`); en los tests, `const fifosExist bool` y `func makeFIFO(p string) error` (por sistema).

- [ ] **Step 1: Test.** **Crear `host/internal/cli/send_swap_test.go`:**

```go
package cli

import (
	"os"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
)

// V19: a FIFO put in place of the file once Lstat found it regular (between
// Lstat and open) does not block open, and with it the handler and the
// shutdown of serve: the file is opened without waiting, and refused once
// open because it is not the regular file that was checked.
func TestSendFIFOSwappedInBeforeOpen(t *testing.T) {
	if !fifosExist {
		t.Skip("Windows has no FIFOs in the file system; its residual is the junction window between ReparseBelow and Lstat (see openInEnvironment)")
	}
	l, ws := bareLink(t)
	p := writeIn(t, ws, "a.txt", []byte("x"))
	swapped := false
	setVar(t, &beforeOpen, func() {
		if swapped {
			return
		}
		swapped = true
		if err := os.Remove(p); err != nil {
			t.Error(err)
			return
		}
		if err := makeFIFO(p); err != nil {
			t.Errorf("mkfifo: %v", err)
		}
	})
	done := make(chan ipc.Exit, 1)
	go func() {
		_, e := l.sendFile(ws, sendReq(p, ""))
		done <- e
	}()
	select {
	case e := <-done:
		wantSendError(t, e, "SEND_FAILED", "changed while it was being opened")
	case <-time.After(5 * time.Second):
		// A FIFO opened to read and write never waits: it lets the blocked
		// open go on, so that nothing is left running after the test.
		if w, err := os.OpenFile(p, os.O_RDWR, 0); err == nil {
			defer w.Close()
		}
		<-done
		t.Fatal("sendFile blocked on a FIFO put in place of the file")
	}
	if !swapped {
		t.Fatal("the file was never swapped")
	}
	if got := outboxNames(t, ws); len(got) != 0 {
		t.Fatalf("copied %v", got)
	}
}
```

**Crear `host/internal/cli/send_swap_unix_test.go`:**

```go
//go:build unix

package cli

import "syscall"

// fifosExist: Unix file systems have FIFOs.
const fifosExist = true

func makeFIFO(p string) error { return syscall.Mkfifo(p, 0o600) }
```

**Crear `host/internal/cli/send_swap_other_test.go`:**

```go
//go:build !unix

package cli

import "errors"

// fifosExist: no FIFOs in this file system (on Windows, named pipes live in
// \\.\pipe\, out of any environment).
const fifosExist = false

func makeFIFO(string) error { return errors.New("no FIFOs on this system") }
```

- [ ] **Step 2: Ver que no compila** (el gancho no existe).

```bash
cd host && go vet ./internal/cli
```

Esperado (réplica): `vet.exe: internal\cli\send_swap_test.go:22:13: undefined: beforeOpen` (en Linux y macOS, la misma línea con `/`).

- [ ] **Step 3: El gancho, todavía con `os.Open`.**

**Editar `host/internal/cli/send.go`** (cambio 1 de 5): reemplazar

```go
	if !lst.Mode().IsRegular() {
		return nil, nil, sendFail(1, "NOT_A_FILE", "%s is not a regular file: only files can be sent", classify.Printable(path))
	}
	f, err := os.Open(real)
```

por

```go
	if !lst.Mode().IsRegular() {
		return nil, nil, sendFail(1, "NOT_A_FILE", "%s is not a regular file: only files can be sent", classify.Printable(path))
	}
	beforeOpen()
	f, err := os.Open(real)
```

**Editar `host/internal/cli/send.go`** (cambio 2 de 5): reemplazar

```go
// afterOpen runs between opening the file and resolving its path again: a
// hook for the tests.
var afterOpen = func() {}
```

por

```go
// beforeOpen runs between the Lstat that found a regular file and its open:
// a hook for the tests.
var beforeOpen = func() {}

// afterOpen runs between opening the file and resolving its path again: a
// hook for the tests.
var afterOpen = func() {}
```

- [ ] **Step 4: Ver el RED** (solo en Unix: lo da la CI o un Linux/macOS; en Windows el test se omite).

```bash
cd host && go test -count=1 -timeout 5m ./internal/cli -run TestSendFIFOSwappedInBeforeOpen -v
```

Esperado en Linux o macOS: `send_swap_test.go:50: sendFile blocked on a FIFO put in place of the file` al cabo de 5 s (el `os.Open` espera un escritor; el test lo desbloquea abriendo la FIFO en lectura y escritura antes de fallar). En Windows (réplica): `--- SKIP: TestSendFIFOSwappedInBeforeOpen (0.00s)` con el motivo, y `ok`.

- [ ] **Step 5: Abrir sin esperar.**

**Editar `host/internal/cli/send.go`** (cambio 3 de 5): reemplazar

```go
	beforeOpen()
	f, err := os.Open(real)
```

por

```go
	beforeOpen()
	f, err := os.OpenFile(real, safeio.ReadFlags, 0) // O_NONBLOCK on Unix: a FIFO swapped in is not waited on
```

**Editar `host/internal/cli/send.go`** (cambio 4 de 5): reemplazar

```go
// (which EvalSymlinks does not follow) is refused. The file opened must be
// the one resolved, and it is never opened before it is known to be regular
// (a FIFO would block open, and with it the shutdown of serve).
```

por

```go
// (which EvalSymlinks does not follow) is refused. The file opened must be
// the one resolved, and it is never opened before it is known to be regular
// (a FIFO would block open, and with it the shutdown of serve). A FIFO put
// in its place after Lstat does not block either: the open does not wait
// for a writer (safeio.ReadFlags, O_NONBLOCK on Unix), and the Stat after it
// refuses what is not the regular file checked. Windows has no FIFOs in the
// file system; its residual is the window between ReparseBelow and Lstat,
// where a folder on the way can become a junction that the second
// resolution does not see either (EvalSymlinks does not follow junctions).
```

**Editar `host/internal/cli/send.go`** (cambio 5 de 5, el import): reemplazar

```go
	"github.com/hybriqtech/adbuddy/host/internal/pathguard"
	"github.com/spf13/cobra"
```

por

```go
	"github.com/hybriqtech/adbuddy/host/internal/pathguard"
	"github.com/hybriqtech/adbuddy/host/internal/safeio"
	"github.com/spf13/cobra"
```

- [ ] **Step 6: Ver que pasa, en los tres sistemas.**

```bash
cd host && gofmt -l ./internal/cli && go vet ./internal/cli && GOOS=linux go vet ./internal/cli && GOOS=darwin go vet ./internal/cli && go test -count=1 -timeout 5m ./internal/cli -run 'TestSend|TestHandleSend|TestDownload' -v
```

Esperado: sin salida de `gofmt` ni de los `vet`; en Windows (réplica) `TestSendFIFOSwappedInBeforeOpen` en `SKIP` y el resto de `TestSend*`, `TestHandleSendThroughIPC` y `TestDownloadFailsWhenOutboxBecomesALink` en `PASS` (o en `SKIP` los que ya se omitían aquí: symlinks sin privilegio); `ok`. En Linux y macOS (CI) `TestSendFIFOSwappedInBeforeOpen` y `TestSendFIFO` en `PASS`, y los `TestSend*` que copian y leen el fichero confirman que `O_NONBLOCK` no cambia la lectura de un fichero regular.

- [ ] **Step 7: Suite completa.**

```bash
cd host && go build ./... && go vet ./... && gofmt -l . && go test -count=1 -timeout 15m ./...
```

Esperado: todo `ok`.

- [ ] **Step 8: Commit.**

```bash
git add host/internal/cli/send.go host/internal/cli/send_swap_test.go host/internal/cli/send_swap_unix_test.go host/internal/cli/send_swap_other_test.go
git commit -m "fix(cli): adbuddy send abre sin esperar, y una FIFO puesta antes de abrir no lo cuelga (V19)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Concurrencia:** el gancho `beforeOpen` es una variable de paquete que solo cambian los tests con `setVar` (el patrón de `afterOpen` y `resolveAgain`); el test lo lee dentro de `sendFile`, en la gorrutina que lanza, después de cambiarlo. Si el test fallara por cuelgue, abre la FIFO en `O_RDWR` (nunca espera) para que la gorrutina termine antes de que `setVar` restaure el gancho.

---

### Task 3: Cuota de `outbox/` con los envíos por turnos (V22, parte 1)

**Revisor:** `adbuddy-reviewer`. No hace falta Opus: no toca el confinamiento de rutas de `inbox/` ni de `outbox/` (las comprobaciones `ensureOutbox`/`plainDir`, `createExclusive` con `O_EXCL` y la recomprobación tras la copia quedan igual, solo se mueven a una función); lo nuevo es una medida de tamaño, que solo lee (`os.ReadDir` y `Info`) y que, si la carpeta se cambiara por un enlace entre medias, solo contaría mal, sin escribir en ningún sitio nuevo.

**Files:**
- Create: `host/internal/cli/send_outbox_test.go`
- Modify: `host/internal/cli/send.go` (variables `outboxQuota` y `quotaPassed`; `sendFile` llama a `copyToOutbox`; nuevos `outboxCopy`, `copyToOutbox`, `outboxUsed`, `outboxFull`), `host/internal/cli/phone_files.go` (`filesState.sendMu` y su comentario)

**Interfaces:**
- Consumes: `ensureOutbox`, `createExclusive`, `statSize`, `maxSendSize`, `filesNow`, `sendFail` (existentes).
- Produces: `var outboxQuota int64 = 2 << 30`; `type outboxCopy struct{ name, path string; size int64; sha256 string }`; `func (l *phoneLink) copyToOutbox(ws, name, path string, src *os.File, fi os.FileInfo) (outboxCopy, ipc.Exit)`; `func outboxUsed(outbox string) int64`; `func outboxFull() ipc.Exit`; `var quotaPassed = func() {}` (gancho de test, como `afterOpen`; se declara en el paso 2 junto a la cuota); campo `filesState.sendMu sync.Mutex`.

- [ ] **Step 1: Tests.** **Crear `host/internal/cli/send_outbox_test.go`:**

```go
package cli

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
)

// V22: outbox/ has a quota, like inbox/. A send that would take it over is
// refused before anything is copied, and deleting files from outbox/ (what
// the phone already has) makes room again.
func TestSendOutboxQuota(t *testing.T) {
	setVar(t, &outboxQuota, int64(10))
	l, ws := bareLink(t)
	p := writeIn(t, ws, "a.bin", make([]byte, 6))
	if _, e := l.sendFile(ws, sendReq(p, "")); e.Code != 0 {
		t.Fatalf("first send: %+v", e)
	}
	_, e := l.sendFile(ws, sendReq(p, "")) // 6 + 6 > 10
	wantSendError(t, e, "TOO_LARGE", "the outbox folder is full")
	got := outboxNames(t, ws)
	if len(got) != 1 {
		t.Fatalf("outbox %v", got)
	}
	if err := os.Remove(filepath.Join(ws, "outbox", got[0])); err != nil {
		t.Fatal(err)
	}
	if _, e := l.sendFile(ws, sendReq(p, "")); e.Code != 0 {
		t.Fatalf("send after making room: %+v", e)
	}
}

// What the agent writes in outbox/ itself counts too (a report written there
// and then sent, as the skills do), and nothing is copied over the quota.
func TestSendOutboxQuotaCountsTheAgentsFiles(t *testing.T) {
	setVar(t, &outboxQuota, int64(10))
	l, ws := bareLink(t)
	p := writeIn(t, ws, "outbox/informe.md", make([]byte, 6))
	_, e := l.sendFile(ws, sendReq(p, "")) // 6 (the report) + 6 (its copy) > 10
	wantSendError(t, e, "TOO_LARGE", "the outbox folder is full")
	if got := outboxNames(t, ws); len(got) != 1 || got[0] != "informe.md" {
		t.Fatalf("outbox %v", got)
	}
}

// A file that grew after it was measured is counted by what was copied: the
// copy over the quota is removed.
func TestSendOutboxQuotaCountsWhatWasCopied(t *testing.T) {
	setVar(t, &outboxQuota, int64(10))
	setVar(t, &statSize, func(os.FileInfo) int64 { return 0 }) // measured empty, it is not
	l, ws := bareLink(t)
	p := writeIn(t, ws, "a.bin", make([]byte, 11))
	_, e := l.sendFile(ws, sendReq(p, ""))
	wantSendError(t, e, "TOO_LARGE", "the outbox folder is full")
	if got := outboxNames(t, ws); len(got) != 0 {
		t.Fatalf("left %v", got)
	}
}

// Sends take turns from the quota check to the end of the copy: several at
// once cannot all pass a check that only one of them fits in. The pause after
// the check, once every send has begun, would let them all pass it unless
// they take turns: then only the second check would stop them, and maybe
// none would be sent.
func TestSendOutboxQuotaHoldsUnderConcurrentSends(t *testing.T) {
	setVar(t, &outboxQuota, int64(10))
	l, ws := bareLink(t)
	p := writeIn(t, ws, "a.bin", make([]byte, 6))
	exits := make([]ipc.Exit, 8)
	var started, wg sync.WaitGroup
	var passed atomic.Int32
	started.Add(len(exits))
	setVar(t, &quotaPassed, func() {
		passed.Add(1)
		started.Wait() // every send has begun
		time.Sleep(20 * time.Millisecond)
	})
	for i := range exits {
		wg.Add(1)
		go func() {
			defer wg.Done()
			started.Done()
			_, exits[i] = l.sendFile(ws, sendReq(p, ""))
		}()
	}
	wg.Wait()
	sent := 0
	for _, e := range exits {
		if e.Code == 0 {
			sent++
			continue
		}
		wantSendError(t, e, "TOO_LARGE", "the outbox folder is full")
	}
	if got := outboxNames(t, ws); sent != 1 || len(got) != 1 || passed.Load() != 1 {
		t.Fatalf("%d sent, %d passed the first check, outbox %v", sent, passed.Load(), got)
	}
}
```

- [ ] **Step 2: Declarar la cuota y el gancho sin usarlos** (para que el RED sea de comportamiento y no de compilación).

**Editar `host/internal/cli/send.go`** (cambio 1 de 4): reemplazar

```go
	// statSize is the size a file was measured at, a variable so that a test
	// can tell the copy's own limit from the measurement's.
	statSize = func(fi os.FileInfo) int64 { return fi.Size() }
)
```

por

```go
	// statSize is the size a file was measured at, a variable so that a test
	// can tell the copy's own limit from the measurement's.
	statSize = func(fi os.FileInfo) int64 { return fi.Size() }
	// outboxQuota is what outbox/ may weigh (V22): the copies adbuddy send
	// leaves there and the files the agent writes there itself. The same
	// 2 GiB as inbox/ (spec M3 §6), ten copies of the biggest file.
	outboxQuota int64 = 2 << 30
	// quotaPassed runs in copyToOutbox once a send passed the quota check,
	// before its copy: a hook for the tests.
	quotaPassed = func() {}
)
```

- [ ] **Step 3: Ver que fallan.**

```bash
cd host && go test -count=1 -timeout 5m ./internal/cli -run TestSendOutbox
```

Esperado (réplica):

```
--- FAIL: TestSendOutboxQuota (0.02s)
    send_outbox_test.go:25: exit {Code:0 ErrorCode: Error:}, want error TOO_LARGE containing "the outbox folder is full"
--- FAIL: TestSendOutboxQuotaCountsTheAgentsFiles (0.00s)
    send_outbox_test.go:45: exit {Code:0 ErrorCode: Error:}, want error TOO_LARGE containing "the outbox folder is full"
--- FAIL: TestSendOutboxQuotaCountsWhatWasCopied (0.00s)
    send_outbox_test.go:59: exit {Code:0 ErrorCode: Error:}, want error TOO_LARGE containing "the outbox folder is full"
--- FAIL: TestSendOutboxQuotaHoldsUnderConcurrentSends (0.01s)
    send_outbox_test.go:101: 8 sent, 0 passed the first check, outbox [20261002-145018-a-2.bin 20261002-145018-a-3.bin 20261002-145018-a-4.bin 20261002-145018-a-5.bin 20261002-145018-a-6.bin 20261002-145018-a-7.bin 20261002-145018-a-8.bin 20261002-145018-a.bin]
FAIL
```

- [ ] **Step 4: Implementar.**

**Editar `host/internal/cli/send.go`** (cambio 2 de 4): reemplazar

```go
	name := safeName(filepath.Base(path))
	outbox, exit := ensureOutbox(ws)
	if exit.Code != 0 {
		return "", exit
	}
	ext := filepath.Ext(name)
	stem := filesNow().Format("20060102-150405") + "-" + strings.TrimSuffix(name, ext)
	dst, final, err := createExclusive(outbox, stem, ext)
	if err != nil {
		return "", sendFail(1, "SEND_FAILED", "could not save the file in outbox/")
	}
	dstPath := filepath.Join(outbox, final)
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(dst, h), io.LimitReader(src, maxSendSize+1))
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxSendSize {
		os.Remove(dstPath)
		return "", sendFail(1, "TOO_LARGE", "%s is too large: at most %d MiB", filepath.Base(path), maxSendSize>>20)
	}
	if err != nil {
		os.Remove(dstPath)
		return "", sendFail(1, "SEND_FAILED", "could not copy the file")
	}
```

por

```go
	name := safeName(filepath.Base(path))
	cp, exit := l.copyToOutbox(ws, name, path, src, fi)
	if exit.Code != 0 {
		return "", exit
	}
	final, dstPath, n := cp.name, cp.path, cp.size
```

**Editar `host/internal/cli/send.go`** (cambio 3 de 4): reemplazar

```go
		SHA256: hex.EncodeToString(h.Sum(nil)), Thumb: thumbnailOf(dstPath),
```

por

```go
		SHA256: cp.sha256, Thumb: thumbnailOf(dstPath),
```

**Editar `host/internal/cli/send.go`** (cambio 4 de 4): reemplazar

```go
// openInEnvironment opens the regular file at path if it is inside the
```

por

```go
// outboxCopy is the copy of a file that adbuddy send left in outbox/.
type outboxCopy struct {
	name   string // in outbox/
	path   string // outbox/name, absolute
	size   int64
	sha256 string // hex
}

// copyToOutbox copies src (the file at path, measured as fi) into outbox/ of
// ws under a new name made from name, within the quota of outbox/ (V22).
// Sends take turns here, from the check to the end of the copy (sendMu): two
// at once could both fit in the room that only one of them has. The turn is
// not held while the session or the bridge server are called.
func (l *phoneLink) copyToOutbox(ws, name, path string, src *os.File, fi os.FileInfo) (outboxCopy, ipc.Exit) {
	l.files.sendMu.Lock()
	defer l.files.sendMu.Unlock()
	outbox, exit := ensureOutbox(ws)
	if exit.Code != 0 {
		return outboxCopy{}, exit
	}
	if outboxUsed(outbox)+statSize(fi) > outboxQuota {
		return outboxCopy{}, outboxFull()
	}
	quotaPassed()
	ext := filepath.Ext(name)
	stem := filesNow().Format("20060102-150405") + "-" + strings.TrimSuffix(name, ext)
	dst, final, err := createExclusive(outbox, stem, ext)
	if err != nil {
		return outboxCopy{}, sendFail(1, "SEND_FAILED", "could not save the file in outbox/")
	}
	dstPath := filepath.Join(outbox, final)
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(dst, h), io.LimitReader(src, maxSendSize+1))
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	switch {
	case err != nil:
		os.Remove(dstPath)
		return outboxCopy{}, sendFail(1, "SEND_FAILED", "could not copy the file")
	case n > maxSendSize:
		os.Remove(dstPath)
		return outboxCopy{}, sendFail(1, "TOO_LARGE", "%s is too large: at most %d MiB", filepath.Base(path), maxSendSize>>20)
	case outboxUsed(outbox) > outboxQuota: // it grew after it was measured
		os.Remove(dstPath)
		return outboxCopy{}, outboxFull()
	}
	return outboxCopy{name: final, path: dstPath, size: n, sha256: hex.EncodeToString(h.Sum(nil))}, ipc.Exit{}
}

// outboxUsed is what outbox/ weighs for its quota: the regular files at its
// top, where adbuddy send leaves its copies. The agent's own files there
// count too; its subfolders are its own, like the rest of the environment.
func outboxUsed(outbox string) int64 {
	var total int64
	entries, _ := os.ReadDir(outbox)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
	}
	return total
}

// outboxFull is the answer to a send that does not fit in outbox/.
func outboxFull() ipc.Exit {
	return sendFail(1, "TOO_LARGE", "the outbox folder is full (at most %d MiB): delete from it the files the phone already has", outboxQuota>>20)
}

// openInEnvironment opens the regular file at path if it is inside the
```

**Editar `host/internal/cli/phone_files.go`** (cambio 1 de 2): reemplazar

```go
// filesState is what the files keep. Two mutexes, neither held while calling
// the Manager or the bridge server (the handlers answer after releasing it):
// upMu guards the uploads, and every frame of the app takes it too; dlMu
// guards the downloads and the capture flag.
```

por

```go
// filesState is what the files keep. Three mutexes, none held while calling
// the Manager or the bridge server (the handlers answer after releasing it):
// upMu guards the uploads, and every frame of the app takes it too; dlMu
// guards the downloads and the capture flag; sendMu gives adbuddy send its
// turns at outbox/ (send.go, copyToOutbox).
```

**Editar `host/internal/cli/phone_files.go`** (cambio 2 de 2): reemplazar

```go
	dlSlots   chan struct{} // one per download that sends: maxDownloads
	capturing bool
}
```

por

```go
	dlSlots   chan struct{} // one per download that sends: maxDownloads
	capturing bool

	sendMu sync.Mutex
}
```

- [ ] **Step 5: Ver que pasan, y el resto de `send`.**

```bash
cd host && gofmt -l ./internal/cli && go vet ./internal/cli && go test -count=1 -timeout 5m ./internal/cli -run 'TestSend|TestHandleSend|TestDownload'
```

Esperado: sin salida de `gofmt` ni de `vet`; `ok`. Siguen en verde sin tocarlos `TestSendCopyStopsAtTheLimit` (el `TOO_LARGE` de los 200 MiB va antes que la cuota), `TestSendTwiceTheSameNameInTheSameSecond`, `TestSendOutboxThatIsALink` y `TestSendReachesTheApp` (el `sha256` del adjunto sale ahora de `copyToOutbox`).

- [ ] **Step 6: Suite completa.**

```bash
cd host && go build ./... && go vet ./... && gofmt -l . && go test -count=1 -timeout 15m ./...
```

Esperado: todo `ok`.

- [ ] **Step 7: Commit.**

```bash
git add host/internal/cli/send.go host/internal/cli/phone_files.go host/internal/cli/send_outbox_test.go
git commit -m "fix(cli): outbox/ tiene cuota de 2 GiB, como inbox/, y los envios van por turnos (V22)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Concurrencia:** `sendMu` solo lo toma `copyToOutbox`, y dentro no se llama a nada que tome otro mutex del enlace (`ensureOutbox`, `createExclusive`, `io.Copy` y `os.ReadDir` son de disco). Se suelta antes de `thumbnailOf`, `l.chat.attachments.put`, `l.session()` y `l.addMessage`, así que el orden de mutex del resto del enlace no cambia. Un envío espera a que termine la copia del anterior (hasta 200 MiB de disco): `serve` ya esperaba a los manejadores al cerrar, y la copia de un fichero regular no se queda colgada (Tarea 2). En `TestSendOutboxQuotaHoldsUnderConcurrentSends` cada gorrutina escribe solo su `exits[i]` y se leen tras `wg.Wait()`; el gancho `quotaPassed` cuenta con un `atomic.Int32` y espera a que todas hayan empezado (`started`, que cada gorrutina marca antes de llamar a `sendFile`, así que no hay bloqueo con el turno tomado). **Mutación comprobada (réplica):** sin las dos líneas de `sendMu`, el test falla 8 de 8 veces con `1 sent, 8 passed the first check` (la segunda comprobación deja un solo envío, pero los ocho copiaron, y a veces ninguno se queda); con ellas pasa 20 de 20 (`-count=20`).

---

### Task 4: El registro de adjuntos tiene tope (V22, parte 2)

**Revisor:** `adbuddy-reviewer`.

**Files:**
- Create: `host/internal/cli/phone_chat_registry_test.go`
- Modify: `host/internal/cli/phone_chat.go` (`maxRegistered`, `attachmentRegistry` y `put`)

**Interfaces:**
- Consumes: `keptChat`, `maxChatAttachments` (`phone_chat.go`), `fileID` (`phone_files_test.go`).
- Produces: `const maxRegistered = 2 * keptChat * maxChatAttachments` (2000); campo `attachmentRegistry.order []string`. `put` y `get` mantienen su firma; el campo `m` sigue ahí (lo lee `phone_files_test.go:515`).

- [ ] **Step 1: Test.** **Crear `host/internal/cli/phone_chat_registry_test.go`:**

```go
package cli

import (
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
)

// V22: the registry of attachments forgets the oldest once it holds
// maxRegistered, which is more than the history can refer to. Putting an id
// again changes it where it is: it takes no second place.
func TestAttachmentRegistryIsBounded(t *testing.T) {
	if maxRegistered < keptChat*maxChatAttachments {
		t.Fatalf("maxRegistered %d is below what the history can refer to (%d)", maxRegistered, keptChat*maxChatAttachments)
	}
	var r attachmentRegistry
	for i := 0; i <= maxRegistered; i++ {
		r.put(bridge.Attachment{ID: fileID(i)}, "outbox/x")
	}
	if len(r.m) != maxRegistered {
		t.Fatalf("%d kept, want %d", len(r.m), maxRegistered)
	}
	if _, ok := r.get(fileID(0)); ok {
		t.Fatal("the oldest is still there")
	}
	for _, i := range []int{1, maxRegistered} {
		if _, ok := r.get(fileID(i)); !ok {
			t.Fatalf("attachment %d forgotten", i)
		}
	}
	r.put(bridge.Attachment{ID: fileID(1), Size: 7}, "inbox/y")
	if a, ok := r.get(fileID(1)); !ok || a.Size != 7 || a.Path != "inbox/y" || len(r.m) != maxRegistered {
		t.Fatalf("put again: %+v %v, %d kept", a, ok, len(r.m))
	}
	// It kept its place: it is still the oldest, and the next one forgets it.
	r.put(bridge.Attachment{ID: fileID(maxRegistered + 1)}, "outbox/z")
	if _, ok := r.get(fileID(1)); ok {
		t.Fatal("the oldest (put again) was kept")
	}
	if _, ok := r.get(fileID(2)); !ok || len(r.m) != maxRegistered {
		t.Fatalf("the second oldest forgotten, or %d kept", len(r.m))
	}
}
```

- [ ] **Step 2: Declarar el tope sin usarlo** (RED de comportamiento).

**Editar `host/internal/cli/phone_chat.go`** (cambio 1 de 2): reemplazar

```go
// attachmentRegistry knows the attachments that exist (task 8 fills it).
type attachmentRegistry struct {
	mu sync.Mutex
	m  map[string]storedAttachment
}
```

por

```go
// maxRegistered is how many attachments the registry keeps, the oldest
// forgotten first (V22): the history refers to at most keptChat messages of
// maxChatAttachments each, and as many again may wait for their message
// (uploaded or captured and not sent yet, or sent by the agent while the app
// is away). A forgotten one answers not_found to file.get and chat.send; its
// file stays on disk.
const maxRegistered = 2 * keptChat * maxChatAttachments

// attachmentRegistry knows the attachments that exist (task 8 fills it).
type attachmentRegistry struct {
	mu sync.Mutex
	m  map[string]storedAttachment
}
```

- [ ] **Step 3: Ver que falla.**

```bash
cd host && go test -count=1 -timeout 5m ./internal/cli -run TestAttachmentRegistryIsBounded
```

Esperado (réplica): `phone_chat_registry_test.go:21: 2001 kept, want 2000`.

- [ ] **Step 4: Implementar.**

**Editar `host/internal/cli/phone_chat.go`** (cambio 2 de 2): reemplazar

```go
// attachmentRegistry knows the attachments that exist (task 8 fills it).
type attachmentRegistry struct {
	mu sync.Mutex
	m  map[string]storedAttachment
}

func (r *attachmentRegistry) put(a bridge.Attachment, path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.m == nil {
		r.m = map[string]storedAttachment{}
	}
	r.m[a.ID] = storedAttachment{Attachment: a, Path: path}
}
```

por

```go
// attachmentRegistry knows the attachments that exist (task 8 fills it), up
// to maxRegistered.
type attachmentRegistry struct {
	mu    sync.Mutex
	m     map[string]storedAttachment
	order []string // the ids in m, oldest first
}

func (r *attachmentRegistry) put(a bridge.Attachment, path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.m == nil {
		r.m = map[string]storedAttachment{}
	}
	if _, ok := r.m[a.ID]; !ok {
		r.order = append(r.order, a.ID)
	}
	r.m[a.ID] = storedAttachment{Attachment: a, Path: path}
	for len(r.order) > maxRegistered {
		delete(r.m, r.order[0])
		r.order = r.order[1:]
	}
}
```

- [ ] **Step 5: Ver que pasa, y lo que usa el registro.**

```bash
cd host && gofmt -l ./internal/cli && go vet ./internal/cli && go test -count=1 -timeout 5m ./internal/cli -run 'TestAttachmentRegistry|TestSend|TestChat|TestUpload|TestAttach|TestFile|TestCapture|TestDownload'
```

Esperado: sin salida de `gofmt` ni de `vet`; `ok`.

- [ ] **Step 6: Suite completa.**

```bash
cd host && go build ./... && go vet ./... && gofmt -l . && go test -count=1 -timeout 15m ./...
```

Esperado: todo `ok`.

- [ ] **Step 7: Commit.**

```bash
git add host/internal/cli/phone_chat.go host/internal/cli/phone_chat_registry_test.go
git commit -m "fix(cli): el registro de adjuntos guarda como mucho 2000 y olvida primero los mas viejos (V22)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Memoria:** `r.order = r.order[1:]` deja el prefijo en el array de fondo hasta que `append` lo realoja; al realojar solo se copian los vivos, así que la capacidad se queda en el orden de `2 × maxRegistered` ids. Todo bajo `r.mu`, como antes.

---

### Task 5: `serve` toma el cerrojo antes de arrancar nada (diferido de #19) 🔒

**Revisor:** `adbuddy-reviewer-opus` (IPC y arranque de `serve`).

**Files:**
- Create: `host/internal/ipc/servelock_test.go`
- Modify: `host/internal/ipc/lock.go` (import; `ServeLock`, `Lock`, `Close`, `held` al final), `host/internal/ipc/server.go` (campo `Server.lock`; `Listen` se parte en `Listen`, `ListenLocked` y `checkListen`), `host/internal/cli/serve.go` (tomar el cerrojo tras `serveRunning`; `ListenLocked`), `host/internal/cli/serve_test.go` (dos tests al final y una comprobación en `TestServeWaitsForRequestsInFlight`)

**Interfaces:**
- Consumes: `tryLock`, `lockPath`, `errLocked` (`ipc/lock.go`, de #19); `ErrAlreadyRunning`, `maxSocketPath`, `SocketPath` (`ipc/ipc.go`); `maxPreauth` (`ipc/server.go`); `shortDir`, `EnsureSecret` en los tests de `ipc`; `isolate`, `run`, `exists`, `alreadyRunning` en los de `cli`.
- Produces: `type ServeLock struct{ … }` con `func (l *ServeLock) Close() error` (idempotente, también con `nil`) y `func (l *ServeLock) held() bool`; `func Lock(dataDir string) (*ServeLock, error)`; `func ListenLocked(lock *ServeLock, secret string, h Handler) (*Server, error)`; `func checkListen(dataDir, secret string) error`; campo `Server.lock *ServeLock`. `Listen(dataDir, secret string, h Handler) (*Server, error)` no cambia de firma.

**Solape con #21 (ya en `main`):** #21 cambia `Conn` (campo `peer`) y `handle()` (captura del par), y añade `TestMain` en `peer_test.go`. Este plan no toca ninguna de las tres cosas: en `server.go` solo cambia una línea del `struct Server` y la función `Listen` (que #21 no toca), y los tests nuevos van en un fichero propio sin `TestMain`. El `serveOnce` de `peer_test.go` usa `Listen`, que sigue igual por fuera. (#19 y #21 chocan entre sí en `handle()` al integrarse: eso lo resuelve quien integre, antes de este plan; la réplica lo resolvió dejando las dos cosas, la plaza de `preauth` y la captura del par.)

- [ ] **Step 1: Tests de `ipc`.** **Crear `host/internal/ipc/servelock_test.go`:**

```go
package ipc

import (
	"errors"
	"path/filepath"
	"testing"
)

// serve takes the data folder's lock before anything else (Lock) and hands it
// to the server (ListenLocked): while it is held, neither another Lock nor a
// Listen gets it, and the server's Close releases it. Closing it again does
// nothing.
func TestLockIsHeldUntilTheServerCloses(t *testing.T) {
	dir := shortDir(t)
	secret, err := EnsureSecret(dir)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if _, err := Lock(dir); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second Lock: %v", err)
	}
	if _, err := Listen(dir, secret, nil); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("Listen with the lock taken: %v", err)
	}
	s, err := ListenLocked(lock, secret, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Lock(dir); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("Lock while serving: %v", err)
	}
	s.Close()
	again, err := Lock(dir)
	if err != nil {
		t.Fatalf("Lock after Close: %v", err)
	}
	again.Close()
	if err := lock.Close(); err != nil {
		t.Fatalf("closing a released lock: %v", err)
	}
}

// A ListenLocked that fails leaves the lock with its caller, and a lock
// already released is refused instead of serving without one.
func TestListenLockedThatFailsKeepsTheLock(t *testing.T) {
	dir := shortDir(t)
	lock, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ListenLocked(lock, "not a secret", nil); err == nil {
		t.Fatal("ListenLocked accepted a bad secret")
	}
	if _, err := Lock(dir); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("the failed ListenLocked released the lock: %v", err)
	}
	lock.Close()
	secret, err := EnsureSecret(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s, err := ListenLocked(lock, secret, nil); err == nil {
		s.Close()
		t.Fatal("ListenLocked served with a released lock")
	}
}

// Lock creates the data folder, as Listen does.
func TestLockCreatesTheDataFolder(t *testing.T) {
	lock, err := Lock(filepath.Join(shortDir(t), "data"))
	if err != nil {
		t.Fatal(err)
	}
	lock.Close()
}
```

- [ ] **Step 2: Ver que no compila** (la API no existe).

```bash
cd host && go vet ./internal/ipc
```

Esperado (réplica): `vet.exe: internal\ipc\servelock_test.go:19:15: undefined: Lock`.

- [ ] **Step 3: El cerrojo exportado.**

**Editar `host/internal/ipc/lock.go`**: reemplazar

```go
import (
	"errors"
	"os"
	"path/filepath"
	"time"
)
```

por

```go
import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)
```

**Añadir al final de `host/internal/ipc/lock.go`:**

```go

// ServeLock is the data folder's daemon lock (lockPath), held from Lock to
// Close. serve takes it before anything else and hands it to ListenLocked.
type ServeLock struct {
	dir string
	mu  sync.Mutex
	f   *os.File // nil once released
}

// Lock takes the daemon lock of dataDir without waiting, creating the folder
// if needed: ErrAlreadyRunning when another daemon holds it (or another Lock
// of this process: the lock is per open file). serve takes it first, so that
// a second serve stops before it touches the workspace or starts watching
// the phone, instead of finding out at Listen.
func Lock(dataDir string) (*ServeLock, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	f, err := tryLock(lockPath(dataDir))
	if errors.Is(err, errLocked) {
		return nil, ErrAlreadyRunning
	}
	if err != nil {
		return nil, err
	}
	return &ServeLock{dir: dataDir, f: f}, nil
}

// Close releases the lock. Later calls, and a nil lock, do nothing: the
// Server that was given it closes it too.
func (l *ServeLock) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

// held reports whether the lock has not been released yet.
func (l *ServeLock) held() bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f != nil
}
```

**Editar `host/internal/ipc/server.go`** (cambio 1 de 2): reemplazar

```go
	lock    *os.File      // held until Close (lockPath)
```

por

```go
	lock    *ServeLock    // held until Close (lockPath)
```

**Editar `host/internal/ipc/server.go`** (cambio 2 de 2): reemplazar

```go
// Listen creates the socket in dataDir. A leftover socket from a dead daemon
// is removed; a live one yields ErrAlreadyRunning. secret must be the
// 64-hex-character installation secret.
func Listen(dataDir, secret string, h Handler) (*Server, error) {
	path := SocketPath(dataDir)
	if len(path) > maxSocketPath {
		return nil, fmt.Errorf("socket path %s is longer than %d characters; set ADBUDDY_DATA_DIR to a shorter folder", path, maxSocketPath)
	}
	if _, err := hex.DecodeString(secret); err != nil || len(secret) != 64 {
		return nil, errors.New("the IPC secret must be 64 hexadecimal characters")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	// One daemon per data directory, for as long as it serves: two serve that
	// start at once could otherwise both find a dead socket and the second
	// remove the first one's live socket (Stat, Dial and Remove are not one
	// step).
	lock, err := tryLock(lockPath(dataDir))
	if errors.Is(err, errLocked) {
		return nil, ErrAlreadyRunning
	}
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*Server, error) {
		lock.Close()
		return nil, err
	}
	if _, err := os.Stat(path); err == nil {
		// A daemon of an older version holds no lock, but it answers.
		if c, err := net.DialTimeout("unix", path, 500*time.Millisecond); err == nil {
			c.Close()
			return fail(ErrAlreadyRunning)
		}
		if err := os.Remove(path); err != nil {
			return fail(err)
		}
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return fail(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o600); err != nil {
			ln.Close()
			return fail(err)
		}
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	return &Server{ln: ln, lock: lock, preauth: make(chan struct{}, maxPreauth), secret: secret, handler: h, ctx: ctx, cancel: cancel}, nil
}
```

por

```go
// Listen creates the socket in dataDir, holding the data folder's lock
// (Lock) until Close. A leftover socket from a dead daemon is removed; a live
// one, or the lock held by another daemon, yields ErrAlreadyRunning. secret
// must be the 64-hex-character installation secret.
func Listen(dataDir, secret string, h Handler) (*Server, error) {
	if err := checkListen(dataDir, secret); err != nil {
		return nil, err
	}
	// One daemon per data directory, for as long as it serves: two serve that
	// start at once could otherwise both find a dead socket and the second
	// remove the first one's live socket (Stat, Dial and Remove are not one
	// step).
	lock, err := Lock(dataDir)
	if err != nil {
		return nil, err
	}
	s, err := ListenLocked(lock, secret, h)
	if err != nil {
		lock.Close()
	}
	return s, err
}

// ListenLocked is Listen with the data folder's lock already taken (serve
// takes it before it starts anything else). The Server releases it on Close;
// if ListenLocked fails, the lock stays with the caller. A lock already
// released is refused: a daemon never serves without it.
func ListenLocked(lock *ServeLock, secret string, h Handler) (*Server, error) {
	if !lock.held() {
		return nil, errors.New("the daemon lock is not held")
	}
	dataDir := lock.dir
	if err := checkListen(dataDir, secret); err != nil {
		return nil, err
	}
	path := SocketPath(dataDir)
	if _, err := os.Stat(path); err == nil {
		// A daemon of an older version holds no lock, but it answers.
		if c, err := net.DialTimeout("unix", path, 500*time.Millisecond); err == nil {
			c.Close()
			return nil, ErrAlreadyRunning
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o600); err != nil {
			ln.Close()
			return nil, err
		}
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	return &Server{ln: ln, lock: lock, preauth: make(chan struct{}, maxPreauth), secret: secret, handler: h, ctx: ctx, cancel: cancel}, nil
}

// checkListen refuses a socket path too long for the system and a secret
// that is not 64 hexadecimal characters.
func checkListen(dataDir, secret string) error {
	if path := SocketPath(dataDir); len(path) > maxSocketPath {
		return fmt.Errorf("socket path %s is longer than %d characters; set ADBUDDY_DATA_DIR to a shorter folder", path, maxSocketPath)
	}
	if _, err := hex.DecodeString(secret); err != nil || len(secret) != 64 {
		return errors.New("the IPC secret must be 64 hexadecimal characters")
	}
	return nil
}
```

(`Close` no cambia: `_ = s.lock.Close()` llama ahora a `ServeLock.Close`, después de `s.ln.Close()` y `s.wg.Wait()`, como en #19.)

- [ ] **Step 4: Ver que pasan, en los tres sistemas.**

```bash
cd host && gofmt -l ./internal/ipc && go vet ./internal/ipc && GOOS=linux go vet ./internal/ipc && GOOS=darwin go vet ./internal/ipc && go test -count=1 -timeout 5m ./internal/ipc
```

Esperado: sin salida de `gofmt` ni de los `vet`; `ok  …/internal/ipc`. Siguen en verde sin tocarlos `TestSecondServerRefusedAndStaleSocketReplaced`, `TestSecondServerRefusedEvenWithoutTheSocket` (#19) y los de `peer_test.go` (#21, que usan `Listen`).

- [ ] **Step 5: Tests de `serve`.** **Añadir al final de `host/internal/cli/serve_test.go`:**

```go

// One serve per data folder: a second one that finds the lock taken (a serve
// that has not reached its socket yet) stops with ALREADY_RUNNING before it
// touches the workspace, asks adb anything or starts its watchers.
func TestServeFindsTheLockBeforeStarting(t *testing.T) {
	data := isolate(t)
	held, err := ipc.Lock(data)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	ws := filepath.Join(t.TempDir(), "ws")
	_, errOut, code := run(t, "serve", "--adb", filepath.Join(t.TempDir(), "no-adb"), "--workspace", ws, "--confirm", "deny")
	if code != ExitFailure || !strings.Contains(errOut, "ALREADY_RUNNING") || exists(ws) {
		t.Fatalf("exit %d, workspace created %v, stderr %q", code, exists(ws), errOut)
	}
}

// serve releases the lock however it ends: here adb cannot even run.
func TestServeReleasesTheLockWhenItFails(t *testing.T) {
	data := isolate(t)
	ws := filepath.Join(t.TempDir(), "ws")
	_, errOut, code := run(t, "serve", "--adb", filepath.Join(t.TempDir(), "no-adb"), "--workspace", ws, "--confirm", "deny")
	if code == 0 || strings.Contains(errOut, "ALREADY_RUNNING") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	lock, err := ipc.Lock(data)
	if err != nil {
		t.Fatalf("the lock is still held after serve ended: %v", err)
	}
	lock.Close()
}
```

**Editar `host/internal/cli/serve_test.go`** (en `TestServeWaitsForRequestsInFlight`): reemplazar

```go
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop")
	}
	// Checked as soon as serve returns: nothing may still be in flight.
```

por

```go
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop")
	}
	// The lock went with the server's Close: the next serve can start.
	if lock, err := ipc.Lock(data); err != nil {
		t.Fatalf("the lock is still held after serve stopped: %v", err)
	} else {
		lock.Close()
	}
	// Checked as soon as serve returns: nothing may still be in flight.
```

- [ ] **Step 6: Ver que falla el primero** (los otros dos fijan lo que ya pasa: hoy `serve` no toma el cerrojo hasta `Listen`, y `Close` lo suelta).

```bash
cd host && go test -count=1 -timeout 5m ./internal/cli -run 'TestServeFindsTheLockBeforeStarting|TestServeReleasesTheLockWhenItFails|TestServeWaitsForRequestsInFlight'
```

Esperado (réplica):

```
--- PASS: TestServeWaitsForRequestsInFlight (0.64s)
--- FAIL: TestServeFindsTheLockBeforeStarting (0.06s)
    serve_test.go:429: exit 1, workspace created true, stderr "Waiting for the phone on ADBuddy's adb server (port 5038)…\nError: ADB_FAILED — exec: \"C:\\\\Users\\\\…\\\\no-adb\": executable file not found in %PATH%\nFix: Check the adb path (…).\n"
--- PASS: TestServeReleasesTheLockWhenItFails (0.07s)
FAIL
```

- [ ] **Step 7: Tomarlo en `serve`.**

**Editar `host/internal/cli/serve.go`** (cambio 1 de 2): reemplazar

```go
			} else if running {
				return alreadyRunning
			}
```

por

```go
			} else if running {
				return alreadyRunning
			}
			// One serve per data folder from here on (ipc.Lock), taken before
			// the workspace is touched and before the watchers of the phone
			// and of the remote mode start: a second serve stops here. The
			// server releases it on Close; the defer covers every other way
			// out.
			lock, err := ipc.Lock(e.dirs.Data)
			if errors.Is(err, ipc.ErrAlreadyRunning) {
				return alreadyRunning
			}
			if err != nil {
				return NewError("SOCKET_FAILED", err.Error(), "")
			}
			defer lock.Close()
```

**Editar `host/internal/cli/serve.go`** (cambio 2 de 2): reemplazar

```go
			srv, err := ipc.Listen(e.dirs.Data, secret, serveHandler(link, e.cfg, app.Getenv, ws, agentName, sess, d))
```

por

```go
			srv, err := ipc.ListenLocked(lock, secret, serveHandler(link, e.cfg, app.Getenv, ws, agentName, sess, d))
```

- [ ] **Step 8: Ver que pasan, y todo `serve`.**

```bash
cd host && gofmt -l ./internal/cli && go vet ./internal/cli && go test -count=1 -timeout 5m ./internal/cli -run 'TestServe|TestDoctorWithServeRunning|TestConnect|TestSetup'
```

Esperado: sin salida de `gofmt` ni de `vet`; `ok`. `TestServeAlreadyRunning` y `TestServeDaemonUnreachable` siguen igual (los resuelve `serveRunning`, antes del cerrojo); `TestServeInterruptedWhileStarting` también (Ctrl+C durante `serveRunning`).

- [ ] **Step 9: Suite completa con los tres sistemas.**

```bash
cd host && go build ./... && go vet ./... && gofmt -l . && GOOS=linux go vet ./... && GOOS=darwin go vet ./... && go test -count=1 -timeout 15m ./...
```

Esperado: todo `ok` (la CLI arranca `serve` muchas veces en carpetas de datos distintas; cada una suelta su cerrojo, y en Windows la limpieza de `t.TempDir` fallaría si el fichero quedara abierto).

- [ ] **Step 10: Commit.**

```bash
git add host/internal/ipc/lock.go host/internal/ipc/server.go host/internal/ipc/servelock_test.go host/internal/cli/serve.go host/internal/cli/serve_test.go
git commit -m "fix(host): serve toma el cerrojo de la carpeta de datos antes de arrancar los vigilantes

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Concurrencia y caminos de salida:** `ServeLock` protege `f` con su mutex, así que el `Close` del `Server` (desde la gorrutina de `serveUntil`) y el `defer lock.Close()` de `serve` pueden coincidir sin doble cierre. Caminos de `serve` tras tomar el cerrojo: todos los `return` (entorno, reglas, Ctrl+C, adb, invariante, secreto, `--confirm`, `startPhoneLink`, `ListenLocked`) pasan por el `defer`; el camino normal lo suelta antes, en `srv.Close()` dentro de `serveUntil`, después de quitar el socket y de esperar a los manejadores (como en #19), y el `defer` ya no hace nada. Un pánico también pasa por el `defer` (y el sistema suelta el cerrojo al morir el proceso, sea como sea). Orden de los `defer` de `serve`: `stop` (el primero, se ejecuta el último), `lock.Close`, `sess.Stop` (el primero en ejecutarse); igual que antes, el cerrojo se suelta en `Close` antes de que `sess.Stop` termine con el agente, lo que ya pasaba con #19 (ver "Riesgos").

---

### Task 6: Pendientes y verificación final

**Revisor:** el controlador; después, `adbuddy-reviewer-opus` hace la revisión final de la rama.

**Files:**
- Modify: `docs/superpowers/plans/pendientes-m3-m5.md` (CRLF; solo se añade una sección al final)

- [ ] **Step 1: Apuntar lo hecho y lo que sigue abierto.** **Añadir al final de `docs/superpowers/plans/pendientes-m3-m5.md`** (con CRLF, como el resto; tras la última sección que haya, sea la del plan B, la de la ola 2 H o la de otra rama integrada después):

```markdown

## v1 · Diferidos L (plan `docs/superpowers/plans/2026-10-02-adbuddy-v1-diferidos-l.md`)

- **HECHO:** `daemon/pcwrite.go` usa `internal/pathguard` y ya no tiene copia propia de `resolveLinks`, `within` y `reparseBelow` (V09; `TestPCWriteUsesPathguard` impide que vuelva). `adbuddy send` abre con `safeio.ReadFlags` (`O_NONBLOCK` en Unix): una FIFO puesta entre el `Lstat` y el `open` ya no cuelga el manejador ni el cierre de `serve` (V19). `outbox/` tiene una cuota de 2 GiB como `inbox/` (los ficheros regulares de su primer nivel, también los del agente; `TOO_LARGE` con "the outbox folder is full"; los envíos van por turnos) y el registro de adjuntos guarda como mucho 2000, olvidando primero los más viejos (V22). `serve` toma el cerrojo de la carpeta de datos (`ipc.Lock`) justo después de `serveRunning`, antes de tocar el entorno, hablar con adb o arrancar los vigilantes, y se lo da a `ipc.ListenLocked` (diferido de #19). Con esto quedan hechas las notas de M3a de más arriba sobre la FIFO de `adbuddy send` y sobre `pcwrite.go` y `pathguard`.
- **Sigue abierto:**
  - la ventana de junction de `adbuddy send` en Windows entre `ReparseBelow` y `Lstat` (nota de M3a de más arriba): cerrarla pide comparar la ruta final del handle abierto (`GetFinalPathNameByHandle`) con la raíz del entorno resuelta del mismo modo;
  - un adjunto olvidado del registro responde `not_found` a `file.get` y a `chat.send` aunque su fichero siga en disco (solo con más de 2000 adjuntos en la sesión);
  - `docs/troubleshooting.md`, fila `TOO_LARGE`: no dice todavía que también sale con `outbox/` lleno (2 GiB; se arregla borrando de `outbox/` lo que el móvil ya tiene). Para el dueño de los docs;
  - `serve` suelta el cerrojo en `srv.Close()`, antes de que `sess.Stop` termine con el agente: un `serve` nuevo puede arrancar mientras el agente del anterior aún se cierra (ya pasaba con #19).
  - Validaciones 42 y 43 de este plan: en su sección "Validaciones manuales pendientes", no se repiten aquí.
```

Comprobar con `file docs/superpowers/plans/pendientes-m3-m5.md` que sigue en CRLF y con `git diff` que solo se añaden líneas al final.

- [ ] **Step 2: Verificación final.**

```bash
cd host && go build ./... && go vet ./... && gofmt -l . && GOOS=linux go vet ./... && GOOS=darwin go vet ./... && go test -count=1 -timeout 15m ./...
git grep -nE '\b(10\.[0-9]+\.[0-9]+\.[0-9]+|192\.168\.[0-9]+\.[0-9]+|172\.(1[6-9]|2[0-9]|3[01])\.[0-9]+\.[0-9]+)\b' -- host docs/superpowers/plans/2026-10-02-adbuddy-v1-diferidos-l.md
git log --format='%b' origin/main..HEAD | grep -c 'Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>'
git rev-list --count origin/main..HEAD
git log --format='%an <%ae>' origin/main..HEAD | sort -u
```

Esperado: Go en verde; el `git grep` de IPs privadas sin salida nueva respecto a `origin/main`; el número de trailers igual al de commits de la rama y un solo autor, la identidad local hybriqtech.

- [ ] **Step 3: Commit.**

```bash
git add docs/superpowers/plans/pendientes-m3-m5.md
git commit -m "docs: pendientes al dia tras los diferidos L de la v1

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

## Validaciones manuales pendientes (v1 diferidos L)

Solo con permiso explícito del usuario; ninguna es bloqueante para integrar.

42. **Dos `serve` a la vez, sin móvil** (solo el PC: no toca el móvil): con el móvil apagado o fuera de la red, `adbuddy serve` en una terminal (espera al móvil hasta 45 s); mientras espera, `adbuddy serve` en otra: el segundo sale al momento con el mensaje de "ya está en marcha" (`ALREADY_RUNNING`), sin la línea de espera del móvil y sin crear el entorno si no existía. Cuando el primero termina (Ctrl+C o `DEVICE_NOT_FOUND`), otro `adbuddy serve` ya no lo dice.
43. **CI de Linux y macOS de la rama:** `TestSendFIFOSwappedInBeforeOpen` corre (no se omite) y pasa en menos de 5 s, `TestSendFIFO` sigue pasando, y los `TestSend*` que copian ficheros pasan (lectura con `O_NONBLOCK`). En este PC solo se compilan con `GOOS=linux`/`GOOS=darwin go vet`.

## Autorrevisión del plan

### Cobertura frente al encargo

- V09 → Tarea 1: la copia desaparece y `pcwrite` llama a `pathguard`, sin ablandar nada: el código era idéntico, y los casos del encargo (enlaces simbólicos, junctions, `..`, rutas que no existen todavía, mayúsculas en Windows) los cubren los tests de `pcwrite_test.go` que siguen sin tocar (`TestPullThroughALinkOutOfTheEnvironmentIsRed`, `TestPullOverADanglingLinkIsRed`, `TestPullWithDotDotAfterALinkIsRed`, `TestPullComparesPathsWithoutCaseOnWindows`, el destino nuevo `out/new/y`) más el nuevo de la junction (con un camino que no existe y otra caja) y el de sintaxis.
- V19 → Tarea 2: `O_NONBLOCK` vía `safeio.ReadFlags` (Unix) y el `Stat` tras abrir que ya había; test con `syscall.Mkfifo` en Unix y omitido con motivo en Windows; el residual de Windows, documentado en el código, en el ruling 2 y en pendientes.
- V22 → Tareas 3 y 4: cuota de `outbox/` análoga a `inboxQuota` (2 GiB, se mide al escribir y se libera borrando), con tamaño justificado en la spec M3 §6 y en las skills de M6b (ruling 3); el registro acotado (ruling 4). Revisor Sonnet justificado en la Tarea 3.
- Cerrojo → Tarea 5 (la última de código, como pide la adenda): `ipc.Lock` exportado, tomado tras `serveRunning`, pasado a `ListenLocked`, soltado en todos los caminos (`defer` + `Close` idempotente); tests de "el segundo sale antes de arrancar nada" (sin entorno creado: `workspace.Init` va antes de los vigilantes y de adb) y de "se suelta al salir" (fallo, parada normal y `Close` del servidor). Sin cambios de protocolo ni de autenticación.
- Pendientes → Tarea 6, solo una sección al final.

### Placeholders y tipos

Sin "TBD" ni pasos sin código. Nombres cruzados: `beforeOpen` (T2) solo en `send.go` y su test; `outboxQuota`, `quotaPassed`, `outboxCopy`, `copyToOutbox`, `outboxUsed`, `outboxFull`, `filesState.sendMu` solo en T3; `maxRegistered` y `attachmentRegistry.order` solo en T4; `ServeLock`, `Lock`, `ListenLocked`, `checkListen` solo en T5. T2 y T3 editan `send.go` en sitios distintos (T2: apertura, comentario de `openInEnvironment`, gancho e import; T3: bloque `var`, cuerpo de `sendFile` y lo nuevo antes de `openInEnvironment`): los textos "reemplazar" de T3 no incluyen nada que cambie T2. El `reemplazar` de T3 sobre `// openInEnvironment opens the regular file at path if it is inside the` es la primera línea del comentario, que T2 no toca.

### Réplica acumulada tarea a tarea

Árbol de la réplica: `git archive` de `fix/v1-diferidos-b` (b4e528c) fusionado con `origin/fix/v1-ola2-h` (51c2cf2) mediante `git merge-tree --write-tree` (lo que tendrá `main`). La fusión choca solo en `host/internal/ipc/server.go`, dentro de `handle()` (#19 añade la plaza de `preauth`, #21 la captura del par); se resolvió dejando las dos cosas, y el árbol fusionado compila y pasa `go vet ./internal/ipc`.

Un script aplicó este documento sobre ese árbol (bloques `Crear`, pares `reemplazar`/`por` y "Añadir al final", en orden; cada texto "reemplazar" apareció exactamente una vez), con un `git checkout`/`git clean` del árbol antes de cada parada:

- **Cada RED** de los pasos es la salida real de aplicar el plan hasta ese paso (T1 paso 2, T2 pasos 2 y 4, T3 paso 3, T4 paso 3, T5 pasos 2 y 6). El RED de Unix de T2 (cuelgue con la FIFO) no se pudo ver aquí (Windows, sin WSL): lo dará la CI.
- **Al final de cada tarea (1 a 6):** `go build ./...`, `go vet ./...`, `gofmt -l .` vacío, `GOOS=linux go vet ./...` y `GOOS=darwin go vet ./...` limpios, y `go test -count=1 -timeout 15m ./...` con los **33 paquetes en `ok`** en las seis paradas (34 operaciones en total).
- **Repeticiones:** `go test -count=20 -run TestSendOutbox ./internal/cli`, `-count=10` de los tests nuevos del registro y de `serve`, y `-count=10 -run Lock ./internal/ipc`, en verde.
- **Mutación de T3:** sin `sendMu`, `TestSendOutboxQuotaHoldsUnderConcurrentSends` falla 8 de 8 (ver la tarea).
- **Sin #21** (`git archive` de b4e528c solo, aplicando todo menos la Tarea 2, que necesita `safeio`): 26 operaciones, build, `vet` y `gofmt` limpios, y `ipc`, `daemon`, `pathguard` y `cli` en `ok`. Es decir, T1, T3, T4, T5 y T6 no dependen de #21; T2 sí.
- `docs/superpowers/plans/pendientes-m3-m5.md` sigue en CRLF tras el añadido de T6.

### Riesgos abiertos para el revisor Opus

1. **T1:** ninguno de comportamiento (código idéntico). El test de sintaxis mira las declaraciones de nivel superior de los `.go` del paquete `daemon` que no son de test: una copia con otro nombre no la vería (la revisión sí).
2. **T2:** `os.OpenFile` con `O_NONBLOCK` mete el descriptor en el *poller* de Go cuando el sistema lo admite (macOS: kqueue también con ficheros regulares; Linux: epoll no admite ficheros regulares y queda como fichero normal). Leer un fichero regular nunca devuelve `EAGAIN`, así que la copia es la misma; es lo que ya hace `safeio` de #21 para el diario. Lo confirman en la CI los `TestSend*` que copian. El residual de Windows (junction entre `ReparseBelow` y `Lstat`) sigue abierto y documentado.
3. **T3:** la cuota mide `outbox/` por nombre (`os.ReadDir` tras `ensureOutbox`); si el agente cambia `outbox/` por un enlace entre la comprobación y la medida, la medida cuenta otra carpeta (solo lectura) y la copia sigue protegida como antes (`createExclusive` en esa ruta, que ya existía, y la recomprobación `ensureOutbox` tras copiar, que rechaza y no publica). No es un empeoramiento: es el mismo patrón de antes. Los envíos se serializan durante la copia (hasta 200 MiB): un segundo `adbuddy send` espera al primero.
4. **T4:** un id olvidado responde `not_found`; con la app pidiendo miniaturas de un historial de 100 mensajes nunca se llega a olvidar uno que esté en él (1000 < 2000), salvo que haya más de 1000 adjuntos pendientes a la vez.
5. **T5:** el cerrojo se suelta en `srv.Close()` (después de quitar el socket y esperar a los manejadores) y `sess.Stop()` se ejecuta después, en un `defer`: un `serve` nuevo puede arrancar mientras el agente del anterior aún se cierra. Ya pasaba con #19 y no lo cambia este plan (cambiarlo sería soltar el cerrojo solo en el `defer`, después de `sess.Stop`, y que `Server.Close` no lo soltara: otro cambio de contrato de `ipc`). Apuntado en pendientes.
6. **T5 y #21:** #19 y #21 chocan entre sí en `handle()` (la plaza de `preauth` frente a la captura del par); quien integre la segunda lo resuelve dejando las dos. Este plan no toca `handle()`.

### Decisiones y dudas para el controlador

- **Ledger:** los cinco rulings de arriba van tal cual a `.superpowers/sdd/<este plan>/progress.md`.
- **Revisores:** Opus para T1 (daemon) y T5 (IPC y `serve`); Sonnet para T2, T3 y T4. T3 no toca confinamiento de rutas (ver su "Revisor"); si el controlador prefiere Opus para todo `send.go`, T3 es la candidata.
- **Riesgos de solape:** #21 (ola 2 H) ya integrada: T2 usa su `safeio`; T5 no toca `handle()`, `Conn` ni `TestMain`. `doctor.go` (V20) no se toca. `docs/troubleshooting.md` y el catálogo no se tocan (la fila `TOO_LARGE` queda en pendientes). `pendientes-m3-m5.md` solo recibe una sección al final (cualquier otro PR que también añada al final puede chocar en las últimas líneas: se resuelve dejando las dos secciones). `remotecmd_test.go` no se toca.
