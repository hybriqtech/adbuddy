# v1 · ola 2 H: diario, sesión y doctor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cerrar tres diferidos de seguridad que dejó la ola 1 y una decisión de producto:
- el diario sigue enlaces y se cuelga con una FIFO (journal);
- `doctor` desde la sesión del agente usa el adb real aunque haya pausa (V20);
- el agente puede teclear en su propia terminal declarándose "no agente" (V07);
- V27 (lista de variables permitidas) se descarta.

**Architecture:**
- **T1** saca a un paquete `safeio` el helper de lectura de la ola 1 (V14) y añade lo que el diario necesita: abrir la carpeta como `os.Root` solo si es una carpeta real (Lstat, `OpenRoot`, `SameFile`), y dentro de ella abrir solo ficheros regulares con un único nombre y sin bloquear. `journal` escribe, lee y crea las copias de seguridad por esa raíz. `doctor` mira con `Lstat`.
- **T2** añade `Paused` al ping del daemon; el `doctor` del agente no llama a adb en pausa y `serve` lo avisa.
- **T3** identifica en el SERVIDOR el proceso del otro extremo del socket (T3a: `ipc.Peer`), sabe si un PID es del árbol del agente (T3b: `pty` y `session`) y decide `attach`, `session new/stop` y `remote` por eso, no por `ADBUDDY_SOURCE` (T3c).
- **T4** (docs) cierra V27 como descartada en el triage, en `pendientes-m3-m5.md` y en la spec §5.1.

**Tech Stack:** Go 1.27 (host): `os.Root`, `golang.org/x/sys/{unix,windows}` (ya en `go.mod`). Nada nuevo en `go.mod`.

**Spec:** `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md` (§4.4 diario y undo, §4.6 lo que no se garantiza, §5 IPC y sesión). Triage: `.superpowers/sdd/2026-10-02-v1-diferidos/triage.md` (local, no versionado; filas V07, V20 y V27). El diferido "Diario" está en `docs/superpowers/plans/pendientes-m3-m5.md`, sección "v1 · Diferidos tras la ola 1".

**Verificación del plan:**
- Todo el código sale de un prototipo acumulado tarea a tarea sobre esta rama (19969dd), en Windows 11 con Go 1.27.0.
- En cada paso: `go vet ./...`, `GOOS=linux go vet ./...`, `GOOS=darwin go vet ./...`, `gofmt -l .` (vacío) y `go test ./... -count=1`. Todo pasa, salvo lo que depende de la raíz del repo y el prototipo no tenía (fixtures del protocolo y `docs/`).
- Las RED de cada tarea se comprobaron con el código anterior (salidas en cada paso).
- Los tests `//go:build unix` (FIFO) y los caminos Linux y macOS de T3 solo se compilaron aquí; los corre la CI.
- **Flake previo, ajeno al plan:** `session` `TestResumeFallsBackToANewConversation` falló una vez con la suite completa. Aislado pasa (`-count=5`). Mira `State` en cuanto ve el segundo arranque, antes de que `startLocked` ponga `idle`. Si sale, se repite y se apunta en el informe; no se arregla aquí.

**Revisión del plan:** `adbuddy-reviewer-opus` (aa6658eee7a9078d9) pidió enmiendas. Todas están incorporadas (ledger `.superpowers/sdd/2026-10-02-adbuddy-v1-ola2-h/progress.md`):
- **C1:** PID conocido pero no verificable ⇒ `AGENT_SESSION`, con el par capturado en `handle()` antes de la primera trama → T3a y T3c.
- **C2:** criterio Unix `getsid == raíz` o cadena de padres, con los residuos escritos → T3b y T3c.
- **I3:** la declaración solo añade bloqueo → T3c.
- **I4:** ficheros nuevos (`ipc/peer*.go`, `session/tree.go`, `pty/tree_*.go`), interfaz opcional `Contains`, `handleRemote` recibe `sess` (puede ser nil) → T3.
- **I5:** `job` con mutex y lista de PIDs del job; `job == 0` ⇒ desconocido → T3b.
- **I6:** `Lstat` + `OpenRoot` + `SameFile`, `Load` por la misma raíz y envoltorios finos en `workspace` → T1.
- **I7:** `Nlink > 1` → T1.
- **I8:** un mes no regular es un error de `Load` que nombra el fichero → T1.
- **I9:** `Ping.Paused`, sin llamadas a adb, aviso en `serve`, textos nuevos → T2.
- **I10:** la base → Global Constraints.
- **I11:** tests con el binario de test como ayudante → T3a y T3c.
- **M1:** `journalWritable` con `Lstat` → T1.
- **M2:** `MakeBackupDir` por la raíz → T1.
- **M3:** datos de plataforma → T3a.
- **M4:** T3 partida en 3a, 3b y 3c.
- **M5:** validaciones de personas → al final.
- **M6:** alternativas descartadas y modo estricto → "Fuera de esta ola".

## Global Constraints

- **Base (I10):** la de esta rama (`fix/v1-diferidos`, con la PR #20), no `origin/main`, mientras #20 no esté integrada.
  - Al escribir esto, `origin/main` y `origin/fix/v1-diferidos` apuntan los dos a 6797030 (un commit de `phone_files.go` sobre la base de esta rama). Si `git branch -r --contains origin/fix/v1-diferidos` lista `origin/main`, el controlador rebasa la rama sobre `origin/main` antes de T1.
  - **T4 exige ese rebase.**
  - Antes de cada tarea:
    - `git fetch`;
    - `git diff --stat $(git merge-base HEAD origin/main)..origin/main -- <ficheros de la tarea>`;
    - `git diff --stat HEAD...origin/fix/v1-diferidos-b -- <ficheros de la tarea>`.

    Si alguien cambió los mismos ficheros, se trabaja sobre su estado y se dice en el informe.
- **Cada asunto se reproduce antes de arreglarse, sobre la base de esta rama:** un test que falla sin el arreglo (RED con comando y salida). Si ya no falla, se dice y no se inventa trabajo.
- **Nada que necesite móvil, emulador, claves o red.** Los tests usan el adb falso.
  - En este PC Windows no corre `go test -race` (solo la CI): leer los globales de test antes de lanzar gorrutinas y no mutar globales con gorrutinas vivas.
  - Los tests `//go:build unix` no corren aquí; que compilen: `GOOS=linux go vet ./...` y `GOOS=darwin go vet ./...`.
  - T3 exige la CI: subir la rama como copia (sin `--force`, tras `git grep` de seriales e IPs) y mirar `host.yml` en Linux y macOS.
- **Textos de cara al usuario (M6c):** hay catálogo en/es (`host/internal/i18n/catalog/{en,es}.toml`) y controles (`TestKeysExist`, `TestCatalogParity`, `cli/textlint_test.go`).
  - En `internal/cli`, ningún literal nuevo de `NewError`/`Fprint*`/`say`/`warn`/`Short`/`Long`/`Detail`/`Guidance`.
  - Los textos nuevos de este plan son tres claves, con su en/es exacto en T1 y T2.
  - Los `Exit.Error` de IPC se quedan en inglés.
- **Archivos de otras sesiones:**
  - tern: `serve.go`, `setup.go`, `run.go`, `phone.go`, `helpers.go`, `guard.go`, `session/session.go`, `phone_chat.go`, `phone_term.go`, `rule.go` y `phone_agent.go`;
  - nautilus: `pcwrite.go`, `send.go`, `phone_files.go`, `ipc/server.go`, `ipc_test.go` y el `ipc.Lock` de `serve`.

  Este plan toca de ellos solo esto, y el informe lo dice:
  - `serve.go`: 1 línea (T3c, aviso a tern);
  - `ipc/server.go`: 4 líneas (T3a, aviso a nautilus).

  `session.go` no se toca: lo nuevo va en `session/tree.go`. Si nautilus añade un `TestMain` al paquete `ipc`, choca con el de `ipc/peer_test.go` (T3a): se juntan en uno.
- **Seguridad:** un arreglo no puede abrir un hueco que deje ejecutar 🔴 sin confirmación, escribir fuera del entorno, abrir el socket IPC a otros usuarios ni saltarse el shim. Ante la duda, falla cerrado y se dice en el informe.
- **Proceso (`CLAUDE.md`):**
  - cada tarea compila y pasa sus tests sola;
  - commits con la identidad local `hybriqtech` y el trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`;
  - nunca `--no-verify`; `git add` con rutas concretas (y `git rm` para lo que se borra);
  - nunca modelos Haiku; sin IPs privadas ni rutas personales en lo que se sube; ficheros de `host/` en LF;
  - `docs/superpowers/plans/pendientes-m3-m5.md` solo lo toca T4.
- Comandos desde `host/`: `go test ./... -count=1 -timeout 15m`, `gofmt -l .` (vacío), `go vet ./...`, `GOOS=linux go vet ./...`, `GOOS=darwin go vet ./...`.
- **Informe** de cada tarea en `.superpowers/sdd/2026-10-02-adbuddy-v1-ola2-h/task-N-report.md`: una fila por asunto (arreglado, ya no vigente o aplazado con motivo), el test que lo guarda, comandos y salidas RED/GREEN, textos nuevos y residuos.

## Reparto

| Tarea | Implementador | Revisor | Depende de |
|---|---|---|---|
| T1 diario (`safeio`, `journal`, backups, doctor) | `adbuddy-implementer` (Sonnet) | `adbuddy-reviewer-opus` | — |
| T2 V20 (`Ping.Paused`, doctor) | `adbuddy-implementer` (Sonnet) | `adbuddy-reviewer-opus` (daemon, ipc) | — |
| T3a V07: el par del socket (`procinfo`, `ipc`) | `adbuddy-implementer-opus` | `adbuddy-reviewer-opus` | — |
| T3b V07: el árbol del agente (`pty`, `session`) | `adbuddy-implementer-opus` | `adbuddy-reviewer-opus` | T3a (`procinfo`) |
| T3c V07: el criterio (`cli`) | `adbuddy-implementer-opus` | `adbuddy-reviewer-opus` | T3a, T3b |
| T4 docs V27 | `adbuddy-implementer` (Sonnet) | `adbuddy-reviewer` | rebase sobre `main` con #20 |

En serie, un implementador a la vez (casi todo es `host/`). Revisión final de la rama: `adbuddy-reviewer-opus`. Límites a los revisores: `go test -timeout 15m` y 45 minutos.

## Fuera de esta ola

- **De otras sesiones:**
  - tern: V08, V18, V31, V34-V36 y V40;
  - nautilus: V09, V19, V22 y el `ipc.Lock` de `serve`.
- **Límites del modelo:** `journal` firma con `ipc.secret`, ya documentado.
- **Mejoras posteriores:**
  - que `source` del diario salga del par del socket en vez de `ADBUDDY_SOURCE`;
  - registrar en el diario las lecturas de `doctor`: son fijas y de solo lectura; T2 bloquea en pausa.
- **Alternativas descartadas para V07 (M6):**
  - un token por sesión del agente: el agente no lo presenta;
  - exigir TTY: se falsea con `script`.
- **Opción para más adelante (M6):** un modo estricto que confirme cada `attach` en el móvil o en la terminal de `serve`, configurable y no por defecto.
- **Aparte (M2), para pendientes:** `adb push` lee cualquier fichero del PC.

---

### Task 1: El diario no sigue enlaces ni se cuelga con una FIFO

**Implementador:** Sonnet. **Revisor:** `adbuddy-reviewer-opus`.

**Files:**
- Create:
  - `host/internal/safeio/safeio.go`, `flags_unix.go`, `flags_windows.go`, `flags_other.go`;
  - tests `host/internal/safeio/safeio_test.go`, `safeio_unix_test.go`;
  - tests `host/internal/journal/links_test.go`, `links_unix_test.go`, `backups_test.go`;
  - test `host/internal/precheck/backupdir_test.go`.
- Modify:
  - `host/internal/workspace/readregular.go`;
  - `host/internal/journal/journal.go`;
  - `host/internal/precheck/precheck.go`;
  - `host/internal/daemon/daemon.go` (1 línea, :474);
  - `host/internal/cli/doctor.go` (`journalWritable`, :613-634);
  - `host/internal/cli/doctor_test.go`;
  - `host/internal/i18n/catalog/{en,es}.toml`.
- Delete: `host/internal/workspace/readregular_unix.go`, `readregular_other.go` (pasan a `safeio`).

**Interfaces:**
- Produces:
  - Errores:
    - `safeio.ErrNotRegular` ("not a regular file");
    - `safeio.ErrLinked`;
    - `safeio.ErrNotRealDir`.
  - Flags: `safeio.ReadFlags` y `safeio.AppendFlags` (con `O_NONBLOCK` en Unix).
  - Funciones:
    - `safeio.OpenRegular(path string) (*os.File, fs.FileInfo, error)`;
    - `safeio.OpenRoot(dir string) (*os.Root, error)`;
    - `safeio.OpenRegularIn(root *os.Root, name string) (*os.File, error)`;
    - `safeio.OpenAppendIn(root *os.Root, name string, perm os.FileMode) (*os.File, error)`;
    - `(*journal.Store).MakeBackupDir(id string) (string, error)`.
  - Campo `precheck.Env.MakeBackupDir func() (string, error)`.
- Consumes: nada de otras tareas.
- Decisión (como T9a de la ola 1): una carpeta `journal` que el usuario sustituyó por un enlace o una junction (la movió a otro disco) se rechaza. El diario falla cerrado (`Writable`) y `doctor` lo dice ("is not a folder").

- [ ] **Step 1: `safeio`, con sus tests (RED: el paquete no existe).**

`host/internal/safeio/safeio.go`:

```go
// Package safeio opens files in folders the agent can write to without being
// led elsewhere: never through a link or a junction in place of the folder,
// never into a FIFO or a device that would hang or flood the reader, never
// into a file that has a second name outside the folder (a hard link).
package safeio

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

var (
	// ErrNotRegular: what is there is not a regular file (a folder, a
	// FIFO, a device, a link…).
	ErrNotRegular = errors.New("not a regular file")
	// ErrLinked: the file has more than one name (a hard link), and the
	// other one may be outside the folder.
	ErrLinked = errors.New("has more than one name (a hard link)")
	// ErrNotRealDir: the folder is a link, a junction or not a folder.
	ErrNotRealDir = errors.New("not a real folder (a link, a junction or a file)")
)

// OpenRegular opens path for reading only if it is a regular file, and the
// same one Lstat saw. The agent can plant a FIFO, a device or a link to one
// (/dev/zero) where ADBuddy reads: that, or a file swapped in after the
// Lstat, is an error that wraps ErrNotRegular, never a read that hangs. On
// Unix the open itself does not block (ReadFlags), so a FIFO swapped in is
// not waited on.
func OpenRegular(path string) (*os.File, fs.FileInfo, error) {
	lst, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !lst.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
	}
	f, err := os.OpenFile(path, ReadFlags, 0)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	if !st.Mode().IsRegular() || !os.SameFile(lst, st) {
		f.Close()
		return nil, nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
	}
	return f, st, nil
}

// OpenRoot opens dir as an os.Root only if dir itself is a real folder.
// os.OpenRoot follows a link or a junction in place of dir (only what is
// below it is kept inside); this does not: it checks dir with Lstat first
// and that the root it opened is that same folder.
func OpenRoot(dir string) (*os.Root, error) {
	lst, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if lst.Mode().Type() != fs.ModeDir {
		return nil, fmt.Errorf("%s: %w", dir, ErrNotRealDir)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	st, err := root.Stat(".")
	if err != nil {
		root.Close()
		return nil, err
	}
	if !os.SameFile(lst, st) {
		root.Close()
		return nil, fmt.Errorf("%s: %w", dir, ErrNotRealDir)
	}
	return root, nil
}

// OpenRegularIn opens name in root for reading only if it is a regular file
// with a single name. The open does not block (ReadFlags).
func OpenRegularIn(root *os.Root, name string) (*os.File, error) {
	lst, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !lst.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: %w", name, ErrNotRegular)
	}
	f, err := root.OpenFile(name, ReadFlags, 0)
	if err != nil {
		return nil, err
	}
	if err := checkOpened(f, name); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// OpenAppendIn opens name in root to read and append, creating it with perm
// when it is missing, only if it is a regular file with a single name. The
// open does not block (AppendFlags).
func OpenAppendIn(root *os.Root, name string, perm os.FileMode) (*os.File, error) {
	lst, err := root.Lstat(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	case !lst.Mode().IsRegular():
		return nil, fmt.Errorf("%s: %w", name, ErrNotRegular)
	}
	f, err := root.OpenFile(name, AppendFlags, perm)
	if err != nil {
		return nil, err
	}
	if err := checkOpened(f, name); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// checkOpened checks what was opened, whatever the Lstat before it saw: a
// regular file with a single name.
func checkOpened(f *os.File, name string) error {
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("%s: %w", name, ErrNotRegular)
	}
	n, err := linkCount(f, st)
	if err != nil {
		return err
	}
	if n > 1 {
		return fmt.Errorf("%s: %w", name, ErrLinked)
	}
	return nil
}
```

`host/internal/safeio/flags_unix.go`:

```go
//go:build unix

package safeio

import (
	"io/fs"
	"os"
	"syscall"
)

// ReadFlags and AppendFlags open without blocking: opening a FIFO otherwise
// waits for the other end.
const (
	ReadFlags   = os.O_RDONLY | syscall.O_NONBLOCK
	AppendFlags = os.O_RDWR | os.O_APPEND | os.O_CREATE | syscall.O_NONBLOCK
)

// linkCount is the number of names of the file (st_nlink).
func linkCount(_ *os.File, st fs.FileInfo) (uint64, error) {
	if s, ok := st.Sys().(*syscall.Stat_t); ok {
		return uint64(s.Nlink), nil
	}
	return 1, nil
}
```

`host/internal/safeio/flags_windows.go`:

```go
package safeio

import (
	"io/fs"
	"os"

	"golang.org/x/sys/windows"
)

// ReadFlags and AppendFlags: on Windows there are no FIFOs in the file
// system to block on.
const (
	ReadFlags   = os.O_RDONLY
	AppendFlags = os.O_RDWR | os.O_APPEND | os.O_CREATE
)

// linkCount is the number of names of the file (NumberOfLinks).
func linkCount(f *os.File, _ fs.FileInfo) (uint64, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info); err != nil {
		return 0, err
	}
	return uint64(info.NumberOfLinks), nil
}
```

`host/internal/safeio/flags_other.go`:

```go
//go:build !unix && !windows

package safeio

import (
	"io/fs"
	"os"
)

// ReadFlags and AppendFlags: no FIFOs to block on here.
const (
	ReadFlags   = os.O_RDONLY
	AppendFlags = os.O_RDWR | os.O_APPEND | os.O_CREATE
)

// linkCount: these systems do not say; one name.
func linkCount(*os.File, fs.FileInfo) (uint64, error) { return 1, nil }
```

`host/internal/safeio/safeio_test.go`:

```go
package safeio

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// linkDir makes link point to the folder target: a symlink, or a junction on
// Windows without the privilege for symlinks. It is removed with os.Remove,
// which never follows it, before t.TempDir's cleanup.
func linkDir(t *testing.T, link, target string) {
	t.Helper()
	err := os.Symlink(target, link)
	if err != nil && runtime.GOOS == "windows" {
		var out []byte
		if out, err = exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
			t.Skipf("no links here: mklink /J: %v %s", err, out)
		}
	} else if err != nil {
		t.Skipf("no links here: %v", err)
	}
	t.Cleanup(func() { os.Remove(link) })
}

// OpenRoot takes a real folder, never a link or a junction in its place,
// nor a file.
func TestOpenRootOnlyARealFolder(t *testing.T) {
	base := t.TempDir()
	folder := filepath.Join(base, "real")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	r, err := OpenRoot(folder)
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRoot(file); !errors.Is(err, ErrNotRealDir) {
		t.Fatalf("a file: %v, want ErrNotRealDir", err)
	}
	if _, err := OpenRoot(filepath.Join(base, "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a missing folder: %v", err)
	}
	link := filepath.Join(base, "link")
	linkDir(t, link, folder)
	if r, err := OpenRoot(link); !errors.Is(err, ErrNotRealDir) {
		if r != nil {
			r.Close()
		}
		t.Fatalf("a link: %v, want ErrNotRealDir", err)
	}
}

// OpenAppendIn creates and appends, OpenRegularIn reads; neither takes a
// folder, nor a file with a second name (outside the folder, maybe), which
// is left as it was.
func TestOpenInRootOnlyRegularFilesWithOneName(t *testing.T) {
	dir := t.TempDir()
	r, err := OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	f, err := OpenAppendIn(r, "a.jsonl", 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("x\n")); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if f, err = OpenRegularIn(r, "a.jsonl"); err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(f)
	f.Close()
	if err != nil || string(b) != "x\n" {
		t.Fatalf("%q %v", b, err)
	}
	if _, err := OpenRegularIn(r, "missing.jsonl"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "d.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRegularIn(r, "d.jsonl"); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("read a folder: %v, want ErrNotRegular", err)
	}
	if _, err := OpenAppendIn(r, "d.jsonl", 0o600); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("append to a folder: %v, want ErrNotRegular", err)
	}
	outside := filepath.Join(t.TempDir(), "theirs")
	if err := os.WriteFile(outside, []byte("theirs"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(outside, filepath.Join(dir, "h.jsonl")); err != nil {
		t.Skipf("no hard links here: %v", err)
	}
	if _, err := OpenAppendIn(r, "h.jsonl", 0o600); !errors.Is(err, ErrLinked) {
		t.Fatalf("append to a hard link: %v, want ErrLinked", err)
	}
	if _, err := OpenRegularIn(r, "h.jsonl"); !errors.Is(err, ErrLinked) {
		t.Fatalf("read a hard link: %v, want ErrLinked", err)
	}
	if b, _ := os.ReadFile(outside); string(b) != "theirs" {
		t.Fatalf("the file outside changed: %q", b)
	}
}
```

`host/internal/safeio/safeio_unix_test.go`:

```go
//go:build unix

package safeio

import (
	"errors"
	"fmt"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO in the folder is an error, never an open that waits for a writer:
// also when the open itself meets it (the Lstat before it can be raced).
func TestFifoInRootDoesNotHang(t *testing.T) {
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, "2026-10.jsonl"), 0o600); err != nil {
		t.Skipf("no fifo: %v", err)
	}
	r, err := OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	done := make(chan error, 1)
	go func() {
		for _, flag := range []int{ReadFlags, AppendFlags} {
			f, err := r.OpenFile("2026-10.jsonl", flag, 0o600)
			if err != nil {
				done <- fmt.Errorf("open %#x: %v", flag, err)
				return
			}
			err = checkOpened(f, "2026-10.jsonl")
			f.Close()
			if !errors.Is(err, ErrNotRegular) {
				done <- fmt.Errorf("open %#x: %v, want ErrNotRegular", flag, err)
				return
			}
		}
		if _, err := OpenRegularIn(r, "2026-10.jsonl"); !errors.Is(err, ErrNotRegular) {
			done <- fmt.Errorf("OpenRegularIn: %v", err)
			return
		}
		if _, err := OpenAppendIn(r, "2026-10.jsonl", 0o600); !errors.Is(err, ErrNotRegular) {
			done <- fmt.Errorf("OpenAppendIn: %v", err)
			return
		}
		done <- nil
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("hangs on a FIFO")
	}
}
```

Run: `go test ./internal/safeio/ -count=1 -v`. Expected: los dos tests en PASS. En Windows, sin `Skip`: el prototipo hizo la junction con `mklink /J` y el enlace duro con `os.Link`.

- [ ] **Step 2: `workspace` usa `safeio` (refactor: sus tests no cambian y siguen pasando).**

`git rm host/internal/workspace/readregular_unix.go host/internal/workspace/readregular_other.go`. `host/internal/workspace/readregular.go` queda entero así: conserva `maxRead`, `errNotRegular`, `errTooLarge` (el test busca "1 MiB"), `openFlags` (lo usa `readregular_unix_test.go`), `openRegular` y `readRegular`.

```go
package workspace

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/hybriqtech/adbuddy/host/internal/safeio"
)

// maxRead bounds what Init reads of a file the agent can write: a settings
// file, a file it compares before rewriting, a marker.
const maxRead = 1 << 20

var (
	errNotRegular = safeio.ErrNotRegular
	errTooLarge   = errors.New("larger than 1 MiB")
)

// openFlags opens without blocking on Unix (safeio.ReadFlags).
const openFlags = safeio.ReadFlags

// openRegular opens path for reading only if it is a regular file, and the
// same one Lstat saw (safeio.OpenRegular): a FIFO, a device or a link to one
// is an error that wraps errNotRegular, never a read that hangs.
func openRegular(path string) (*os.File, fs.FileInfo, error) { return safeio.OpenRegular(path) }

// readRegular reads the regular file at path (see openRegular) up to maxRead
// bytes. A longer one returns its first maxRead bytes and errTooLarge.
func readRegular(path string) ([]byte, error) {
	f, _, err := openRegular(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxRead+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRead {
		return data[:maxRead], fmt.Errorf("%s: %w", path, errTooLarge)
	}
	return data, nil
}
```

Run: `go test ./internal/workspace/ ./internal/safeio/ -count=1`, `GOOS=linux go vet ./internal/workspace/` y `GOOS=darwin go vet ./internal/workspace/`. Expected: ok y sin salida.

Commit: `refactor(safeio): el helper de lectura regular de workspace pasa a un paquete compartido` (con `git add host/internal/safeio host/internal/workspace/readregular.go`).

- [ ] **Step 3: Tests RED del diario (`openAppend` y `Load`).**

`host/internal/journal/links_test.go`:

```go
package journal

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/safeio"
)

// linkDir makes link point to the folder target: a symlink, or a junction on
// Windows without the privilege for symlinks. It is removed with os.Remove,
// which never follows it, before t.TempDir's cleanup.
func linkDir(t *testing.T, link, target string) {
	t.Helper()
	err := os.Symlink(target, link)
	if err != nil && runtime.GOOS == "windows" {
		var out []byte
		if out, err = exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
			t.Skipf("no links here: mklink /J: %v %s", err, out)
		}
	} else if err != nil {
		t.Skipf("no links here: %v", err)
	}
	t.Cleanup(func() { os.Remove(link) })
}

// A journal folder that is a link (a junction on Windows), planted after
// Init, is refused: no record and no backup folder land where it points,
// and Load says so instead of reading the other side.
func TestJournalThatIsALinkIsRefused(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, "journal")
	linkDir(t, dir, outside)
	s := Open(dir)
	if err := s.Append(Entry{ID: "j_1"}); !errors.Is(err, safeio.ErrNotRealDir) {
		t.Fatalf("Append: %v, want ErrNotRealDir", err)
	}
	if err := s.Writable(); !errors.Is(err, safeio.ErrNotRealDir) {
		t.Fatalf("Writable: %v, want ErrNotRealDir", err)
	}
	if _, _, err := s.Load(); !errors.Is(err, safeio.ErrNotRealDir) {
		t.Fatalf("Load: %v, want ErrNotRealDir", err)
	}
	if names, _ := os.ReadDir(outside); len(names) != 0 {
		t.Fatalf("written through the link: %v", names)
	}
}

// This month's file with a second name (a hard link to a file outside the
// journal) is not appended to, and the file outside is left as it was.
func TestMonthFileWithAnotherNameIsRefused(t *testing.T) {
	s := fixedStore(t, time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC))
	if err := s.Append(Entry{ID: "j_1"}); err != nil {
		t.Fatal(err)
	}
	month := filepath.Join(s.Dir(), "2026-10.jsonl")
	outside := filepath.Join(t.TempDir(), "theirs.txt")
	if err := os.WriteFile(outside, []byte("theirs\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(month); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(outside, month); err != nil {
		t.Skipf("no hard links here: %v", err)
	}
	if err := s.Append(Entry{ID: "j_2"}); !errors.Is(err, safeio.ErrLinked) {
		t.Fatalf("Append: %v, want ErrLinked", err)
	}
	if b, _ := os.ReadFile(outside); string(b) != "theirs\n" {
		t.Fatalf("the file outside changed: %q", b)
	}
}

// A month "file" that is not a regular file is an error of Load that names
// it, never lines skipped: undo and adbuddy journal fail where the person
// sees it.
func TestLoadNamesAMonthFileThatIsNotRegular(t *testing.T) {
	s := fixedStore(t, time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC))
	if err := s.Append(Entry{ID: "j_1"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(s.Dir(), "2026-09.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Load(); !errors.Is(err, safeio.ErrNotRegular) || !strings.Contains(err.Error(), "2026-09.jsonl") {
		t.Fatalf("Load: %v, want ErrNotRegular naming 2026-09.jsonl", err)
	}
}

// A journal that does not exist yet loads as empty, as before.
func TestLoadWithoutAJournal(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "journal"))
	if entries, skipped, err := s.Load(); err != nil || len(entries) != 0 || skipped != 0 {
		t.Fatalf("%v %d %v", entries, skipped, err)
	}
}
```

`host/internal/journal/links_unix_test.go`:

```go
//go:build unix

package journal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/safeio"
)

// A FIFO planted as this month's file hangs neither Load (adbuddy journal,
// undo) nor the next Append: both are errors that name it.
func TestFifoMonthFileDoesNotHang(t *testing.T) {
	s := fixedStore(t, time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC))
	if err := os.MkdirAll(s.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(s.Dir(), "2026-10.jsonl"), 0o600); err != nil {
		t.Skipf("no fifo: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		if _, _, err := s.Load(); !errors.Is(err, safeio.ErrNotRegular) || !strings.Contains(err.Error(), "2026-10.jsonl") {
			done <- fmt.Errorf("Load: %v, want ErrNotRegular naming the file", err)
			return
		}
		if err := s.Append(Entry{ID: "j_1"}); !errors.Is(err, safeio.ErrNotRegular) {
			done <- fmt.Errorf("Append: %v, want ErrNotRegular", err)
			return
		}
		done <- nil
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the journal hangs on a FIFO")
	}
}
```

Run: `go test ./internal/journal/ -count=1 -run 'Link|Another|NotRegular|Without'`. Salida del prototipo con el `journal.go` de la base, en Windows:

```
--- FAIL: TestJournalThatIsALinkIsRefused (0.02s)
    links_test.go:46: Append: <nil>, want ErrNotRealDir
--- FAIL: TestMonthFileWithAnotherNameIsRefused (0.01s)
    links_test.go:78: Append: <nil>, want ErrLinked
--- FAIL: TestLoadNamesAMonthFileThatIsNotRegular (0.01s)
    links_test.go:97: Load: read …\journal\2026-09.jsonl: Incorrect function., want ErrNotRegular naming 2026-09.jsonl
FAIL
```

(`TestFifoMonthFileDoesNotHang` es RED en la CI: `Load` se cuelga 20 s.)

- [ ] **Step 4: `journal` escribe y lee por la raíz.**

En `host/internal/journal/journal.go`:

(a) Imports: añadir `"io/fs"` (tras `"fmt"`) y, en un bloque aparte tras los de la biblioteca estándar, `"github.com/hybriqtech/adbuddy/host/internal/safeio"`.

(b) Sustituir `file()` (:121-123):

```go
func (s *Store) file() string {
	return filepath.Join(s.dir, s.now().UTC().Format("2006-01")+".jsonl")
}
```

por:

```go
// monthFile is the name of this month's file in the journal folder.
func (s *Store) monthFile() string { return s.now().UTC().Format("2006-01") + ".jsonl" }
```

(c) Sustituir `openAppend` (:136-141) por:

```go
// openAppend opens this month's file to append, through the journal folder
// itself (safeio.OpenRoot): a link or a junction planted in place of the
// folder after Init, a month file that is not a regular file (a FIFO, a
// link) or one with a second name (a hard link to a file elsewhere) is an
// error, never a record written outside the environment. Without the journal
// nothing runs (Writable).
func (s *Store) openAppend() (*os.File, error) {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return nil, err
	}
	root, err := safeio.OpenRoot(s.dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return safeio.OpenAppendIn(root, s.monthFile(), 0o600)
}
```

(d) En `Load`, sustituir el comentario final y el principio (:186-200, de "// and counted." hasta el `return nil, 0, err` de `os.Open`):

```go
// and counted.
func (s *Store) Load() (entries []Entry, skipped int, err error) {
	files, err := filepath.Glob(filepath.Join(s.dir, "*.jsonl"))
	if err != nil {
		return nil, 0, err
	}
	sort.Strings(files)
	byID := map[string]int{}
	for _, name := range files {
		f, err := os.Open(name)
		if err != nil {
			return nil, 0, err
		}
```

por:

```go
// and counted. It reads through the journal folder itself, as openAppend
// writes: a folder that is a link or a junction, or a month file that is not
// a regular file with a single name (a FIFO would hang the read), is an
// error that names it, never lines skipped.
func (s *Store) Load() (entries []Entry, skipped int, err error) {
	root, err := safeio.OpenRoot(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, nil // nothing journaled yet
	}
	if err != nil {
		return nil, 0, err
	}
	defer root.Close()
	files, err := fs.Glob(root.FS(), "*.jsonl")
	if err != nil {
		return nil, 0, err
	}
	sort.Strings(files)
	byID := map[string]int{}
	for _, name := range files {
		f, err := safeio.OpenRegularIn(root, name)
		if err != nil {
			return nil, 0, fmt.Errorf("%s: %w", s.dir, err)
		}
```

El resto de `Load` no cambia: escáner con tope de 16 MiB, firma y orden.

Run: `go test ./internal/journal/ -count=1 -v`, `GOOS=linux go vet ./internal/journal/` y `GOOS=darwin go vet ./internal/journal/`. Expected: todos en PASS (los cuatro nuevos y los de antes) y vet sin salida.

Commit: `fix(journal): el diario escribe y lee por su carpeta real, sin enlaces, FIFOs ni enlaces duros`.

- [ ] **Step 5: Copias de seguridad por la raíz (M2). Test RED.**

`host/internal/journal/backups_test.go`:

```go
package journal

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/safeio"
)

// The backup folder of an entry is made inside the journal, through it: a
// link in place of journal or of journal/backups is refused and nothing is
// made where it points.
func TestMakeBackupDir(t *testing.T) {
	now := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	s := fixedStore(t, now)
	got, err := s.MakeBackupDir("j_1")
	if err != nil || got != s.BackupDir("j_1") {
		t.Fatalf("%q %v, want %q", got, err, s.BackupDir("j_1"))
	}
	if st, err := os.Lstat(got); err != nil || !st.IsDir() {
		t.Fatalf("not made: %v %v", st, err)
	}

	s = fixedStore(t, now)
	if err := os.MkdirAll(s.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	linkDir(t, filepath.Join(s.Dir(), "backups"), outside)
	if _, err := s.MakeBackupDir("j_1"); err == nil {
		t.Fatal("made through a link in place of journal/backups")
	}
	if names, _ := os.ReadDir(outside); len(names) != 0 {
		t.Fatalf("made where the link points: %v", names)
	}

	base := t.TempDir()
	linkDir(t, filepath.Join(base, "journal"), outside)
	if _, err := Open(filepath.Join(base, "journal")).MakeBackupDir("j_1"); !errors.Is(err, safeio.ErrNotRealDir) {
		t.Fatalf("journal is a link: %v, want ErrNotRealDir", err)
	}
	if names, _ := os.ReadDir(outside); len(names) != 0 {
		t.Fatalf("made where the link points: %v", names)
	}
}
```

`host/internal/precheck/backupdir_test.go`:

```go
package precheck

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
)

// The backup of an overwritten file goes to the folder Env.MakeBackupDir
// makes (the journal makes it through its own folder), not to a folder made
// by path; if it cannot be made, there is no inverse and the change is 🔴.
func TestPushBackupDirFromTheJournal(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "notes.txt"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	made := filepath.Join(t.TempDir(), "made")
	dev := &fakeDevice{
		answers: map[string]string{kindCmd("/sdcard/notes.txt"): "file\n"},
		pulled:  map[string]string{"/sdcard/notes.txt": "old"},
	}
	env := Env{Cwd: cwd, BackupDir: filepath.Join(t.TempDir(), "by-path"),
		MakeBackupDir: func() (string, error) { return made, os.MkdirAll(made, 0o700) }}
	push := op("adb-push", "local", "notes.txt", "remote", "/sdcard/notes.txt")
	p := Prepare(context.Background(), dev, push, classify.Yellow, env)
	if want := filepath.Join(made, "original"); p.Level != classify.Yellow || len(p.Backups) != 1 || p.Backups[0] != want {
		t.Fatalf("%+v, want the backup in %s", p, want)
	}
	if _, err := os.Stat(env.BackupDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a folder was made by path: %v", err)
	}
	env.MakeBackupDir = func() (string, error) { return "", errors.New("journal/backups is a link") }
	if p := Prepare(context.Background(), dev, push, classify.Yellow, env); p.Level != classify.Red || p.Inverse != nil {
		t.Fatalf("%+v, want 🔴 without an inverse", p)
	}
}
```

Run: `go test ./internal/journal/ ./internal/precheck/ -count=1`. Expected: RED de compilación (`s.MakeBackupDir undefined`, `unknown field MakeBackupDir in struct literal of type Env`).

- [ ] **Step 6: `MakeBackupDir`, `precheck.Env` y el daemon.**

En `host/internal/journal/journal.go`, justo después de `openAppend`:

```go
// MakeBackupDir creates the backup folder of entry id (BackupDir) through
// the journal folder itself, and returns its path: a link or a junction in
// place of journal or of journal/backups is an error, never a folder made
// elsewhere. What then writes into it (adb pull) goes by path: a link
// swapped in after this call is not caught.
func (s *Store) MakeBackupDir(id string) (string, error) {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return "", err
	}
	root, err := safeio.OpenRoot(s.dir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	rel := filepath.Join("backups", id)
	if err := root.MkdirAll(rel, 0o700); err != nil {
		return "", err
	}
	for _, d := range []string{"backups", rel} {
		st, err := root.Lstat(d)
		if err != nil {
			return "", err
		}
		if st.Mode().Type() != fs.ModeDir {
			return "", fmt.Errorf("%s: %w", filepath.Join(s.dir, d), safeio.ErrNotRealDir)
		}
	}
	return s.BackupDir(id), nil
}
```

En `host/internal/precheck/precheck.go`, sustituir el final de `Env` (:32-33):

```go
	BackupDir string // where this entry's backups go (journal/backups/<id>)
}
```

por:

```go
	BackupDir string // where this entry's backups go (journal/backups/<id>)
	// MakeBackupDir, when set, creates BackupDir through the journal folder
	// (journal.Store.MakeBackupDir) and returns it; without it the folder is
	// made by path.
	MakeBackupDir func() (string, error)
}

// backupDir creates the entry's backup folder and returns it.
func (e Env) backupDir() (string, error) {
	if e.MakeBackupDir != nil {
		return e.MakeBackupDir()
	}
	return e.BackupDir, os.MkdirAll(e.BackupDir, 0o700)
}
```

En `push` (:344-349), sustituir:

```go
		if err := os.MkdirAll(env.BackupDir, 0o700); err != nil {
			return err
		}
		// Fixed name: the remote name is chosen on the phone and must never
		// pick where the backup lands (one backup per journal entry).
		backup := filepath.Join(env.BackupDir, "original")
```

por:

```go
		dir, err := env.backupDir()
		if err != nil {
			return err
		}
		// Fixed name: the remote name is chosen on the phone and must never
		// pick where the backup lands (one backup per journal entry).
		backup := filepath.Join(dir, "original")
```

En `host/internal/daemon/daemon.go:474`, sustituir:

```go
		prep = precheck.Prepare(ctx, d.o.ADB, res.Op, level, precheck.Env{Cwd: req.Cwd, BackupDir: d.o.Journal.BackupDir(e.ID)})
```

por:

```go
		prep = precheck.Prepare(ctx, d.o.ADB, res.Op, level, precheck.Env{Cwd: req.Cwd, BackupDir: d.o.Journal.BackupDir(e.ID),
			MakeBackupDir: func() (string, error) { return d.o.Journal.MakeBackupDir(e.ID) }})
```

Run: `go test ./internal/journal/ ./internal/precheck/ ./internal/daemon/ -count=1`. Expected: ok en los tres.

Commit: `fix(journal): las copias de seguridad se crean por la carpeta real del diario`.

- [ ] **Step 7: `doctor` mira el diario con `Lstat` (M1). Test RED.**

En `host/internal/cli/doctor_test.go`:
- imports: añadir `"os/exec"` (tras `"os"`) y `"runtime"` (tras `"path/filepath"`);
- en `TestDoctorJournalNotWritable`, añadir dos casos al mapa `cases`, tras `"not a folder"`:

```go
		// A link (a junction on Windows) in place of the folder: the
		// journal refuses it.
		"a link": func(t *testing.T, jdir string) {
			if err := os.Remove(jdir); err != nil {
				t.Fatal(err)
			}
			target := t.TempDir()
			if err := os.Symlink(target, jdir); err != nil {
				if runtime.GOOS != "windows" {
					t.Fatal(err)
				}
				// A junction, with System32's cmd: the PATH of these tests
				// is the fake adb's.
				cmd := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
				if out, err := exec.Command(cmd, "/c", "mklink", "/J", jdir, target).CombinedOutput(); err != nil {
					t.Skipf("mklink /J: %v %s", err, out)
				}
			}
		},
		// This month's file is not a regular file: the journal refuses it.
		"month file not regular": func(t *testing.T, jdir string) {
			if err := os.Mkdir(filepath.Join(jdir, time.Now().UTC().Format("2006-01")+".jsonl"), 0o700); err != nil {
				t.Fatal(err)
			}
		},
```

(El `linkDir` de `cli/send_test.go` no sirve aquí: llama a `cmd` por el `PATH`, que en estos tests es el del adb falso.)

Run: `go test ./internal/cli/ -count=1 -run TestDoctorJournalNotWritable -v`. Expected: RED en `a_link` y en `month_file_not_regular` (código 0 y `environment` en `ok`: `os.Stat` sigue la junction y no mira si el fichero es regular).

- [ ] **Step 8: Arreglar `journalWritable` y añadir el texto.**

En `host/internal/cli/doctor.go`, sustituir desde la segunda línea del comentario de `journalWritable` hasta el final de la función (:614-634) por:

```go
// folder (not a link or a junction, which the journal refuses), and this
// month's file, when there, a regular file that is not read-only. A missing
// folder is fine: the journal creates it on first use.
func journalWritable(dir string) (firstUse bool, err error) {
	fi, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return true, nil
	case err != nil:
		return false, err
	case fi.Mode().Type() != fs.ModeDir:
		return false, failf("doctor.err.not_folder", dir)
	case runtime.GOOS != "windows" && fi.Mode().Perm()&0o200 == 0:
		// On Windows the read-only attribute of a folder does not stop
		// new files in it.
		return false, failf("doctor.err.read_only", dir)
	}
	// The file journal.Store appends to this month.
	f := filepath.Join(dir, time.Now().UTC().Format("2006-01")+".jsonl")
	switch fi, err := os.Lstat(f); {
	case err != nil:
		// Missing: the journal creates it.
	case !fi.Mode().IsRegular():
		return false, failf("doctor.err.not_regular", f)
	case fi.Mode().Perm()&0o200 == 0:
		return false, failf("doctor.err.read_only", f)
	}
	return false, nil
}
```

La primera línea del comentario ("// journalWritable checks, without creating or writing anything, that the") y la segunda ("// journal could be appended to: its folder, when there, must be a writable") se quedan.

Catálogo, justo después de `"doctor.err.read_only"`:
- `host/internal/i18n/catalog/en.toml`: `"doctor.err.not_regular" = "%[1]s is not a regular file"`
- `host/internal/i18n/catalog/es.toml`: `"doctor.err.not_regular" = "%[1]s no es un fichero normal"`

Run: `go test ./internal/cli/ -count=1 -run 'TestDoctorJournal|TestDoctorDoesNotCreateTheJournal' -v` y `go test ./internal/i18n/ -count=1`. Expected: PASS, con los cuatro subcasos de `TestDoctorJournalNotWritable` (en el prototipo, `a_link` con junction) y el catálogo ok.

- [ ] **Step 9: Comprobar y hacer commit.**

Run: `go test ./... -count=1 -timeout 15m`, `gofmt -l .`, `go vet ./...`, `GOOS=linux go vet ./...` y `GOOS=darwin go vet ./...`. Expected: todo ok, sin salida de gofmt ni de vet.

Commit: `fix(doctor): el diario se comprueba sin seguir enlaces y exige un fichero regular`.

Informe (`task-1-report.md`): las filas `safeio`, `Load`, `openAppend`, `Nlink`, backups (con el residuo: `adb pull` escribe por ruta) y `doctor`; el texto nuevo `doctor.err.not_regular`; la decisión sobre la carpeta `journal` movida con una junction.

### Task 2: `doctor` desde la sesión del agente respeta la pausa (V20)

**Implementador:** Sonnet. **Revisor:** `adbuddy-reviewer-opus` (toca daemon e ipc).

**Files:**
- Modify:
  - `host/internal/ipc/ipc.go` (`Ping`, :94-102);
  - `host/internal/daemon/daemon.go:408`;
  - `host/internal/cli/doctor.go` (`runDoctor` :281 y :329-335, `checkServe` :414-422);
  - `host/internal/i18n/catalog/{en,es}.toml`.
- Test:
  - `host/internal/daemon/pause_test.go` (añadir al final);
  - `host/internal/cli/doctor_paused_test.go` (nuevo).

**Interfaces:**
- Produces: `ipc.Ping.Paused bool` (`json:"paused,omitempty"`), relleno por el daemon con `d.o.Paused()`. Nada que use T3.
- Consumes: `daemon.Options.Paused` (ya existe; `New` lo pone a `false` si es nil, `daemon.go:354`).
- Alcance:
  - Es coherencia, no frontera: un agente que vacía `ADBUDDY_SOURCE` sigue ejecutando `doctor` con el adb real. Las lecturas de `doctor` son fijas y de solo lectura.
  - En pausa con `ADBUDDY_SOURCE=agent`, no se llama a adb en ningún punto (`version`, `devices`, móvil, identidad, puerto de red, app).
  - `verdict()` ignora `skip`, así que el aviso va en la comprobación `serve`: `warn` con su guía, también para la persona.
  - Si `serve` no contesta (`perr`), el comportamiento no cambia: ya avisa.

- [ ] **Step 1: `Ping.Paused` en el daemon. Test RED.**

Al final de `host/internal/daemon/pause_test.go`:

```go
// V20: ping says whether the person paused the agent from the phone, so the
// agent's doctor can leave adb alone meanwhile.
func TestPingSaysWhetherPaused(t *testing.T) {
	e := newEnv(t)
	paused := false
	e.d.o.Paused = func() bool { return paused }
	for _, want := range []bool{false, true} {
		paused = want
		s := &stream{}
		if exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindPing}, s); exit.Code != 0 || len(s.data) != 1 {
			t.Fatalf("%+v", exit)
		}
		var p ipc.Ping
		if err := json.Unmarshal(s.data[0], &p); err != nil {
			t.Fatal(err)
		}
		if p.Paused != want {
			t.Fatalf("paused %v, want %v", p.Paused, want)
		}
	}
}
```

En `host/internal/ipc/ipc.go`, sustituir el final de `Ping`:

```go
	Identity string `json:"identity,omitempty"`
	Journal  string `json:"journal"`
}
```

por:

```go
	Identity string `json:"identity,omitempty"`
	Journal  string `json:"journal"`
	// Paused: the person paused the agent from the phone (V20); false from
	// a host that predates it.
	Paused bool `json:"paused,omitempty"`
}
```

Run: `go test ./internal/daemon/ -count=1 -run TestPingSaysWhetherPaused`. Expected RED (prototipo):

```
--- FAIL: TestPingSaysWhetherPaused (0.00s)
    pause_test.go:285: paused false, want true
```

En `host/internal/daemon/daemon.go:408`, sustituir `Journal: d.o.Journal.Dir()})` por `Journal: d.o.Journal.Dir(), Paused: d.o.Paused()})`.

Run: lo mismo. Expected: ok.

- [ ] **Step 2: Test RED de `doctor`.**

`host/internal/cli/doctor_paused_test.go`:

```go
package cli

import (
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
)

// V20: paused from the phone, the agent's doctor calls no adb (it would run
// outside the shim) and says why each check was not run; the person's doctor
// still checks the phone; both show the pause as a warning of serve.
func TestDoctorPausedAgentCallsNoAdb(t *testing.T) {
	e := doctorEnv(t, doctorRules())
	data := shortDir(t)
	t.Setenv("ADBUDDY_DATA_DIR", data)
	fakeDaemon(t, data, ipc.Ping{Version: Version, Session: "s_1", Serial: doctorSerial, Journal: workspace.JournalDir(e.ws), Paused: true}, ipc.Exit{})

	t.Setenv("ADBUDDY_SOURCE", "agent")
	out, errOut, code := runWithInput(t, "", "doctor", "--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	if calls := adbCalls(t, e); calls != "" {
		t.Fatalf("the paused agent's doctor called adb:\n%s", calls)
	}
	rep := doctorJSON(t, out)
	for _, name := range []string{"adb", "adb server", "phone", "identity", "network port", "phone app"} {
		if c := statusOf(rep, name); c.Status != checkSkip || !strings.Contains(c.Detail, "paused from the phone") {
			t.Errorf("%s: %+v", name, c)
		}
	}
	if c := statusOf(rep, "serve"); c.Status != checkWarn || !strings.Contains(c.Guidance, "Paused from the phone") {
		t.Errorf("serve: %+v", c)
	}
	if c := statusOf(rep, "environment"); c.Status != checkOK {
		t.Errorf("environment: %+v", c)
	}
	if rep.Verdict != "degraded" {
		t.Fatalf("verdict %s", rep.Verdict)
	}

	// The person: the phone is checked; the pause is still a warning.
	t.Setenv("ADBUDDY_SOURCE", "")
	out, _, _ = runWithInput(t, "", "doctor", "--json")
	rep = doctorJSON(t, out)
	if c := statusOf(rep, "phone"); c.Status != checkOK {
		t.Errorf("person, phone: %+v", c)
	}
	if c := statusOf(rep, "serve"); c.Status != checkWarn {
		t.Errorf("person, serve: %+v", c)
	}
	if adbCalls(t, e) == "" {
		t.Fatal("the person's doctor did not check the phone")
	}
}

// Without the pause the agent's doctor checks the phone as before.
func TestDoctorAgentWithoutPauseChecksThePhone(t *testing.T) {
	e := doctorEnv(t, doctorRules())
	doctorServe(t, e)
	t.Setenv("ADBUDDY_SOURCE", "agent")
	out, _, _ := runWithInput(t, "", "doctor", "--json")
	rep := doctorJSON(t, out)
	for _, name := range []string{"adb", "adb server", "serve", "phone", "identity", "network port"} {
		if c := statusOf(rep, name); c.Status != checkOK {
			t.Errorf("%s: %+v", name, c)
		}
	}
}
```

Run: `go test ./internal/cli/ -count=1 -run 'TestDoctorPaused|TestDoctorAgentWithoutPause'`. Expected RED (prototipo):

```
--- FAIL: TestDoctorPausedAgentCallsNoAdb (0.55s)
    doctor_paused_test.go:26: the paused agent's doctor called adb:
        version
        devices
        shell getprop ro.serialno
        …
```

- [ ] **Step 3: Arreglar `runDoctor` y `checkServe`, y añadir los textos.**

En `host/internal/cli/doctor.go`, justo después de `running, ping, perr := servePing(ctx, e.dirs.Data)` (:281), insertar:

```go
	// V20: paused from the phone, the agent's doctor calls no adb either: it
	// would run outside the shim, which refuses everything meanwhile. This is
	// coherence, not a boundary (an agent that clears ADBUDDY_SOURCE still
	// runs it); the pause shows as a warning of serve.
	if running && ping.Paused && app.Getenv("ADBUDDY_SOURCE") == "agent" {
		paused := i18n.M("doctor.skip.paused")
		skip(paused, "adb", "adb server")
		add(checkServe(running, ping, perr, e.dirs.Data))
		skip(paused, "phone", "identity", "network port", "phone app")
		addPCChecks(add, e, running, ping)
		return rep
	}
```

Al final de `runDoctor` (:329-336), sustituir:

```go
	ws := e.workspace("")
	if w := serveWorkspace(ping); running && w != "" {
		ws = w
	}
	add(checkWorkspace(ws))
	add(checkClaudeHooks())
	return rep
}
```

por:

```go
	addPCChecks(add, e, running, ping)
	return rep
}

// addPCChecks adds the checks that only read the PC: the environment
// (serve's, when it runs) and Claude Code's global hooks.
func addPCChecks(add func(check), e env, running bool, ping ipc.Ping) {
	ws := e.workspace("")
	if w := serveWorkspace(ping); running && w != "" {
		ws = w
	}
	add(checkWorkspace(ws))
	add(checkClaudeHooks())
}
```

En `checkServe`, antes de `case running:`, insertar:

```go
	case running && ping.Paused:
		return mk("serve", checkWarn, i18n.M("doctor.serve.running", ping.Session, ping.Serial)).withGuide(i18n.M("doctor.serve.paused"))
```

Catálogo:
- `en.toml`:
  - tras `"doctor.skip.not_connected"`: `"doctor.skip.paused" = "paused from the phone"`;
  - tras `"doctor.serve.running"`: `"doctor.serve.paused" = "Paused from the phone: the agent's adbuddy doctor does not check the phone until you resume from the ADBuddy app."`.
- `es.toml`:
  - tras `"doctor.skip.not_connected"`: `"doctor.skip.paused" = "en pausa desde el móvil"`;
  - tras `"doctor.serve.running"`: `"doctor.serve.paused" = "En pausa desde el móvil: el adbuddy doctor del agente no comprueba el móvil hasta que reanudes desde la app de ADBuddy."`.

Run: `go test ./internal/cli/ -count=1 -run TestDoctor`, `go test ./internal/i18n/ ./internal/daemon/ -count=1`. Expected: ok.

- [ ] **Step 4: Comprobar y hacer commit.**

Run: `go test ./... -count=1 -timeout 15m`, `gofmt -l .`, `go vet ./...`. Expected: ok.

Commit: `fix(doctor): en pausa, el doctor del agente no llama a adb y serve lo avisa (V20)`.

Informe (`task-2-report.md`): las filas `Ping.Paused`, `doctor` y textos (`doctor.skip.paused` y `doctor.serve.paused`). Como límite: es coherencia, no frontera (`ADBUDDY_SOURCE` vaciada), y registrar en el diario queda fuera.

### Task 3a: El servidor sabe qué proceso hay al otro lado del socket (V07)

**Implementador:** opus. **Revisor:** `adbuddy-reviewer-opus`.

**Files:**
- Create:
  - `host/internal/procinfo/procinfo.go`, `procinfo_linux.go`, `procinfo_darwin.go`, `procinfo_other.go`;
  - tests `host/internal/procinfo/procinfo_test.go`, `procinfo_linux_test.go`;
  - `host/internal/ipc/peer.go`, `peer_unix.go`, `peer_linux.go`, `peer_darwin.go`, `peer_windows.go`, `peer_other.go`;
  - test `host/internal/ipc/peer_test.go`, con un `TestMain`: el paquete no tiene ninguno.
- Modify: `host/internal/ipc/server.go` (cuatro líneas: campo `peer` de `Conn`, la captura en `handle()` y el literal de `Conn`).

**Interfaces:**
- Produces:
  - `procinfo.Info{PPID int; Start string; Zombie bool}` y `procinfo.Get(pid int) (Info, error)`. Fuera de Linux y macOS, `errors.ErrUnsupported`.
  - `ipc.Peer{PID int; Known bool; Err error}`, con `(ipc.Peer).Same() bool` y `(*ipc.Conn).Peer() ipc.Peer`. Un `*Conn` nil da `Peer{}`.
- Consumes: nada.
- Semántica (C1):
  - El par se captura en `handle()` antes de leer la primera trama.
  - `Known == false` solo cuando el sistema no da el PID: la plataforma, un Windows sin el ioctl o una conexión que no es un socket.
  - Con el PID dado pero sin poder fijarlo (proceso ya acabado o zombi, `OpenProcess` que falla), `Known` y `Err != nil`: quien lo use no debe fiarse.
  - `Same()` dice si sigue siendo el mismo proceso:
    - Unix: mismo `starttime` (`/proc/<pid>/stat` campo 22; macOS `KinfoProc.Proc.P_starttime`) y no zombi;
    - Windows: el handle abierto con `PROCESS_QUERY_LIMITED_INFORMATION|SYNCHRONIZE` no está señalado. Mientras está abierto, Windows no da su PID a otro proceso.
- Datos de plataforma (M3):
  - Linux: `unix.GetsockoptUcred(fd, SOL_SOCKET, SO_PEERCRED)`;
  - macOS: `unix.GetsockoptInt(fd, SOL_LOCAL, LOCAL_PEERPID)`;
  - Windows: `windows.WSAIoctl` con `SIO_AF_UNIX_GETPEERPID = 0x58000100`, definido aquí, por `SyscallConn().Control`. Comprobado en este PC: da el PID del cliente.

- [ ] **Step 1: `procinfo`.**

`host/internal/procinfo/procinfo.go`:

```go
// Package procinfo reads what serve needs of another process to tell
// whether it belongs to the agent (V07): its parent, and its start time,
// which a later process that reuses its PID does not share.
package procinfo

// Info is a process as the system describes it now.
type Info struct {
	PPID   int    // its parent's PID
	Start  string // when it started, in the system's own units: only compared
	Zombie bool   // it has ended and its parent has not collected it yet
}
```

`host/internal/procinfo/procinfo_linux.go`:

```go
package procinfo

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

var errBadStat = errors.New("unexpected /proc/<pid>/stat")

// Get reads /proc/<pid>/stat.
func Get(pid int) (Info, error) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return Info{}, err
	}
	return parseStat(string(b))
}

// parseStat reads the fields of a /proc/<pid>/stat line that matter here.
// The name (field 2) is in parentheses and may hold spaces and ")": the
// fields are counted from the last ")".
func parseStat(s string) (Info, error) {
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return Info{}, errBadStat
	}
	f := strings.Fields(s[i+1:])
	// f[0] is field 3 (state), f[1] field 4 (ppid), f[19] field 22 (starttime).
	if len(f) < 20 {
		return Info{}, errBadStat
	}
	ppid, err := strconv.Atoi(f[1])
	if err != nil {
		return Info{}, errBadStat
	}
	return Info{PPID: ppid, Start: f[19], Zombie: f[0] == "Z" || f[0] == "X"}, nil
}
```

`host/internal/procinfo/procinfo_darwin.go`:

```go
package procinfo

import (
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

// szomb is SZOMB of <sys/proc.h>: ended, not collected by its parent.
const szomb = 5

// Get asks sysctl kern.proc.pid.<pid>.
func Get(pid int) (Info, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return Info{}, err // no such process: EIO from x/sys (an empty answer)
	}
	if int(kp.Proc.P_pid) != pid {
		return Info{}, syscall.ESRCH
	}
	t := kp.Proc.P_starttime
	return Info{
		PPID:   int(kp.Eproc.Ppid),
		Start:  strconv.FormatInt(int64(t.Sec), 10) + "." + strconv.FormatInt(int64(t.Usec), 10),
		Zombie: kp.Proc.P_stat == szomb,
	}, nil
}
```

`host/internal/procinfo/procinfo_other.go`:

```go
//go:build !linux && !darwin

package procinfo

import "errors"

// Get: this system is not read here (Windows asks its job object instead).
func Get(int) (Info, error) { return Info{}, errors.ErrUnsupported }
```

`host/internal/procinfo/procinfo_test.go`:

```go
package procinfo

import (
	"errors"
	"os"
	"runtime"
	"testing"
)

// This process: its parent and a start time that stays the same.
func TestGetThisProcess(t *testing.T) {
	info, err := Get(os.Getpid())
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		if !errors.Is(err, errors.ErrUnsupported) {
			t.Fatalf("%+v %v, want ErrUnsupported", info, err)
		}
		return
	}
	if err != nil || info.PPID != os.Getppid() || info.Start == "" || info.Zombie {
		t.Fatalf("%+v %v; want parent %d", info, err, os.Getppid())
	}
	if again, err := Get(os.Getpid()); err != nil || again.Start != info.Start {
		t.Fatalf("start %q then %q (%v)", info.Start, again.Start, err)
	}
}
```

`host/internal/procinfo/procinfo_linux_test.go`:

```go
package procinfo

import "testing"

// A name with spaces and ")" does not shift the fields.
func TestParseStat(t *testing.T) {
	line := "4242 (a) b (c)) S 77 4242 4242 0 -1 4194560 100 0 0 0 1 2 0 0 20 0 1 0 98765 1000 100\n"
	info, err := parseStat(line)
	if err != nil || info != (Info{PPID: 77, Start: "98765"}) {
		t.Fatalf("%+v %v", info, err)
	}
	if info, err := parseStat("4242 (z) Z 1 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 5\n"); err != nil || !info.Zombie {
		t.Fatalf("zombie: %+v %v", info, err)
	}
	if _, err := parseStat("4242 (short) S 1"); err == nil {
		t.Fatal("a short line was read")
	}
}
```

Run: `go test ./internal/procinfo/ -count=1`, `GOOS=linux go vet ./internal/procinfo/` y `GOOS=darwin go vet ./internal/procinfo/`. Expected: ok y sin salida.

- [ ] **Step 2: Tests RED de `ipc.Peer`.**

`host/internal/ipc/peer_test.go`:

```go
package ipc

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// TestMain lets the test binary be a client in a process of its own: with
// ADBUDDY_IPC_DIAL set to a data folder it sends a ping, reads one frame of
// the answer and ends without waiting for the rest.
func TestMain(m *testing.M) {
	if data := os.Getenv("ADBUDDY_IPC_DIAL"); data != "" {
		os.Exit(dialAndQuit(data))
	}
	os.Exit(m.Run())
}

func dialAndQuit(data string) int {
	secret, err := ReadSecret(data)
	if err != nil {
		return 1
	}
	nc, err := net.Dial("unix", SocketPath(data))
	if err != nil {
		return 1
	}
	b, _ := json.Marshal(Request{Secret: secret, Kind: KindPing})
	if WriteFrame(nc, FrameRequest, b) != nil {
		return 1
	}
	if _, _, err := ReadFrame(nc); err != nil {
		return 1
	}
	return 0 // the system closes the connection
}

// namesPeers: the systems whose sockets name the process at the other end.
var namesPeers = runtime.GOOS == "linux" || runtime.GOOS == "darwin" || runtime.GOOS == "windows"

// serveOnce listens on a short data folder with h.
func serveOnce(t *testing.T, h Handler) string {
	t.Helper()
	data := shortDir(t)
	secret, err := EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := Listen(data, secret, h)
	if err != nil {
		t.Skipf("no socket: %v", err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })
	return data
}

// V07: the handler knows the process at the other end, without asking the
// client: here this test's own process, alive and the same.
func TestPeerIsTheConnectingProcess(t *testing.T) {
	type seen struct {
		p    Peer
		same bool
	}
	got := make(chan seen, 1)
	data := serveOnce(t, func(ctx context.Context, req Request, c *Conn) Exit {
		p := c.Peer()
		got <- seen{p, p.Same()}
		return Exit{}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := Call(ctx, data, Request{Kind: KindPing}, nil, io.Discard, io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	s := <-got
	if !namesPeers {
		if s.p.Known {
			t.Fatalf("%+v: this system was not expected to name the peer", s.p)
		}
		return
	}
	if !s.p.Known || s.p.Err != nil || s.p.PID != os.Getpid() || !s.same {
		t.Fatalf("peer %+v, same %v; want this process (%d), alive", s.p, s.same, os.Getpid())
	}
}

// V07 (C1): a peer that ended after connecting is no longer the same,
// whatever process gets its PID later.
func TestPeerThatEndedIsNotTheSame(t *testing.T) {
	if !namesPeers {
		t.Skip("this system does not name the peer")
	}
	type seen struct {
		p             Peer
		before, after bool
	}
	ended := make(chan struct{})
	got := make(chan seen, 1)
	data := serveOnce(t, func(ctx context.Context, req Request, c *Conn) Exit {
		p := c.Peer()
		before := p.Same()
		_, _ = c.Stdout().Write([]byte("go")) // the client ends once it reads this
		<-ended
		got <- seen{p, before, p.Same()}
		return Exit{}
	})
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^$")
	cmd.Env = append(os.Environ(), "ADBUDDY_IPC_DIAL="+data)
	runErr := cmd.Run()
	close(ended)
	if runErr != nil {
		t.Fatalf("client: %v", runErr)
	}
	select {
	case s := <-got:
		if !s.p.Known || s.p.PID != cmd.Process.Pid || !s.before || s.after {
			t.Fatalf("peer %+v, same before %v and after %v; want the client (%d), the same only while it ran", s.p, s.before, s.after, cmd.Process.Pid)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the request never reached the handler")
	}
}

// A handler called without a connection (the tests of other packages) has
// an unknown peer.
func TestPeerWithoutAConnectionIsUnknown(t *testing.T) {
	var c *Conn
	if p := c.Peer(); p.Known || p.Same() {
		t.Fatalf("%+v", p)
	}
}
```

Run: `go test ./internal/ipc/ -count=1 -run Peer`. Expected: RED de compilación (`c.Peer undefined`).

- [ ] **Step 3: `ipc.Peer` por plataforma.**

`host/internal/ipc/peer.go`:

```go
package ipc

import (
	"net"
	"syscall"
)

// Peer is the process at the other end of a request's connection, as the
// system names it when the connection is accepted (V07): serve can tell a
// request of the agent's process tree without trusting what the client
// declares. It is captured before the first frame is read.
type Peer struct {
	// PID is the process the system names; 0 when Known is false.
	PID int
	// Known is false when the system does not name the peer (this
	// platform, an old Windows, a connection that is not a local socket):
	// then only what the client declares counts, as before V07.
	Known bool
	// Err says why a named process could not be pinned when the connection
	// was accepted (it had already ended, it could not be opened): such a
	// peer cannot be checked, and callers must not trust it.
	Err error

	id peerID // what tells this process from a later one with its PID
}

// Peer is the process at the other end of the request. A nil Conn (a
// handler called by a test) has an unknown peer.
func (c *Conn) Peer() Peer {
	if c == nil {
		return Peer{}
	}
	return c.peer
}

// Same reports whether the process that connected still runs and still has
// PID: false once it ended, even if another process got its PID, and false
// for a peer that is not Known or has Err.
func (p Peer) Same() bool {
	if !p.Known || p.Err != nil {
		return false
	}
	return p.id.same(p.PID)
}

// withFD runs f with the connection's socket. It reports false when nc has
// none (a net.Pipe) or it cannot be reached.
func withFD(nc net.Conn, f func(fd uintptr)) bool {
	sc, ok := nc.(syscall.Conn)
	if !ok {
		return false
	}
	raw, err := sc.SyscallConn()
	if err != nil {
		return false
	}
	return raw.Control(f) == nil
}
```

`host/internal/ipc/peer_unix.go`:

```go
//go:build linux || darwin

package ipc

import (
	"errors"

	"github.com/hybriqtech/adbuddy/host/internal/procinfo"
)

// peerID is the process's start time: a later process with the same PID
// does not share it.
type peerID struct{ start string }

// pinPeer names the process pid by its start time too.
func pinPeer(pid int) Peer {
	p := Peer{PID: pid, Known: true}
	if pid <= 0 {
		p.Err = errors.New("the system did not name the peer's process")
		return p
	}
	info, err := procinfo.Get(pid)
	switch {
	case err != nil:
		p.Err = err
	case info.Zombie:
		p.Err = errors.New("the peer's process has ended")
	default:
		p.id.start = info.Start
	}
	return p
}

func (id peerID) same(pid int) bool {
	info, err := procinfo.Get(pid)
	return err == nil && !info.Zombie && info.Start == id.start
}

func (p *Peer) release() {}
```

`host/internal/ipc/peer_linux.go`:

```go
package ipc

import (
	"net"

	"golang.org/x/sys/unix"
)

// capturePeer asks the socket who connected (SO_PEERCRED).
func capturePeer(nc net.Conn) Peer {
	var cred *unix.Ucred
	var err error
	if !withFD(nc, func(fd uintptr) { cred, err = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED) }) || err != nil {
		return Peer{}
	}
	return pinPeer(int(cred.Pid))
}
```

`host/internal/ipc/peer_darwin.go`:

```go
package ipc

import (
	"net"

	"golang.org/x/sys/unix"
)

// capturePeer asks the socket who connected (LOCAL_PEERPID).
func capturePeer(nc net.Conn) Peer {
	var pid int
	var err error
	if !withFD(nc, func(fd uintptr) { pid, err = unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID) }) || err != nil {
		return Peer{}
	}
	return pinPeer(pid)
}
```

`host/internal/ipc/peer_windows.go`:

```go
package ipc

import (
	"errors"
	"net"
	"unsafe"

	"golang.org/x/sys/windows"
)

// sioAFUnixGetPeerPID is SIO_AF_UNIX_GETPEERPID of afunix.h
// (_WSAIOR(IOC_VENDOR, 256)): the PID of the process that connected. It is
// not in x/sys/windows.
const sioAFUnixGetPeerPID = 0x58000100

// peerID holds the process open: while it is, Windows gives its PID to no
// other process, and the handle says whether it has ended.
type peerID struct{ h windows.Handle }

// capturePeer asks the socket who connected and opens that process.
func capturePeer(nc net.Conn) Peer {
	var pid uint32
	var err error
	if !withFD(nc, func(fd uintptr) {
		var n uint32
		err = windows.WSAIoctl(windows.Handle(fd), sioAFUnixGetPeerPID, nil, 0,
			(*byte)(unsafe.Pointer(&pid)), uint32(unsafe.Sizeof(pid)), &n, nil, 0)
	}) || err != nil {
		return Peer{} // a Windows without the ioctl does not say who
	}
	p := Peer{PID: int(pid), Known: true}
	if pid == 0 {
		p.Err = errors.New("the system did not name the peer's process")
		return p
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		p.Err = err
		return p
	}
	p.id.h = h
	return p
}

func (id peerID) same(int) bool {
	ev, err := windows.WaitForSingleObject(id.h, 0)
	return err == nil && ev == uint32(windows.WAIT_TIMEOUT)
}

// release closes the process handle; the server calls it when the request
// is done.
func (p *Peer) release() {
	if p.id.h != 0 {
		_ = windows.CloseHandle(p.id.h)
		p.id.h = 0
	}
}
```

`host/internal/ipc/peer_other.go`:

```go
//go:build !linux && !darwin && !windows

package ipc

import "net"

// peerID: these systems do not name the peer.
type peerID struct{}

func capturePeer(net.Conn) Peer { return Peer{} }

func (peerID) same(int) bool { return false }

func (p *Peer) release() {}
```

En `host/internal/ipc/server.go`:

(a) Campo de `Conn` (:93-98): tras `nc     net.Conn`, añadir `peer   Peer // the process at the other end (peer.go)`.

(b) En `handle` (:251-252), sustituir:

```go
func (s *Server) handle(nc net.Conn) {
	defer nc.Close()
```

por:

```go
func (s *Server) handle(nc net.Conn) {
	defer nc.Close()
	// Who connected, before anything is read (V07, peer.go).
	peer := capturePeer(nc)
	defer peer.release()
```

(c) En :262, sustituir `c := &Conn{nc: nc, resize: make(chan [2]int, 4)}` por `c := &Conn{nc: nc, resize: make(chan [2]int, 4), peer: peer}`.

Run: `go test ./internal/ipc/ ./internal/procinfo/ -count=1 -v -run 'Peer|Get|Parse'`. Expected en Windows (prototipo):

```
--- PASS: TestPeerIsTheConnectingProcess (0.01s)
--- PASS: TestPeerThatEndedIsNotTheSame (0.11s)
--- PASS: TestPeerWithoutAConnectionIsUnknown (0.00s)
ok  	github.com/hybriqtech/adbuddy/host/internal/ipc
--- PASS: TestGetThisProcess (0.00s)
ok  	github.com/hybriqtech/adbuddy/host/internal/procinfo
```

- [ ] **Step 4: Comprobar y hacer commit.**

Run: `go test ./... -count=1 -timeout 15m`, `gofmt -l .`, `go vet ./...`, `GOOS=linux go vet ./...` y `GOOS=darwin go vet ./...`. Expected: todo ok. `GOOS=darwin` compila `SOL_LOCAL`, `LOCAL_PEERPID` y `KinfoProc`.

Commit: `feat(ipc): el servidor sabe qué proceso hay al otro lado del socket (V07)`.

### Task 3b: El PTY y la sesión dicen si un PID es del árbol del agente (V07)

**Implementador:** opus. **Revisor:** `adbuddy-reviewer-opus`.

**Files:**
- Create:
  - `host/internal/pty/tree_unix.go`, `tree_windows.go`;
  - tests `host/internal/pty/tree_test.go`, `tree_windows_test.go`;
  - `host/internal/session/tree.go` y su test `host/internal/session/tree_test.go`.
- Modify:
  - `host/internal/pty/pty_windows.go`: campo `jmu` y cierre del job en `Kill` bajo el mutex (I5);
  - `host/internal/pty/job_windows_test.go`: una comprobación más en `TestWithoutJobItWarnsAndGoesOn`.

**Interfaces:**
- Produces:
  - Los PTY de `pty.Start` implementan `Contains(pid int) (bool, error)` (interfaz OPCIONAL, I4). `pty.PTY` no cambia, así que los falsos de los tests (`Pid()` = 1) siguen valiendo.
  - `(*session.Manager).InAgentTree(pid int) (in, known bool)`.
- Consumes: `procinfo.Get` (T3a).
- Criterio:
  - **Unix (C2):** `pid == raíz`, o `getsid(pid) == raíz` (creack/pty arranca el agente con `Setsid`, `pty_unix.go:116`), o la cadena de padres llega a la raíz.
    - `ESRCH` (el proceso ya no existe) es un error.
    - `EPERM` de `getsid` (macOS puede negarlo; comprobarlo en la CI) deja decidir a la cadena de padres.
  - **Windows (I5):** pertenencia al job (`QueryInformationJobObject` con `JobObjectBasicProcessIdList`; `IsProcessInJob` no está en x/sys).
    - Sin job: `errors.ErrUnsupported`.
  - **`InAgentTree`:**
    - sin agente ⇒ `(false, true)`;
    - un PTY sin `Contains`, o `ErrUnsupported` ⇒ `(false, false)` (desconocido);
    - cualquier otro error ⇒ `(true, true)`: se rechaza lo que no se puede comprobar.

- [ ] **Step 1: Tests RED del PTY.**

`host/internal/pty/tree_test.go`:

```go
package pty

import (
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// V07: the PTY tells the program and what it starts (in its job on
// Windows, in its session on Unix) from any other process.
func TestContainsTheProgramAndItsChildren(t *testing.T) {
	p, out := startAgent(t)
	out.waitFor(t, "ready")
	p.Write([]byte("spawn\r"))
	re := regexp.MustCompile(`child:(\d+);`)
	deadline := time.Now().Add(30 * time.Second)
	var m []string
	for m = re.FindStringSubmatch(out.String()); m == nil; m = re.FindStringSubmatch(out.String()) {
		if time.Now().After(deadline) {
			t.Fatalf("no child pid; output:\n%q", out.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	child, _ := strconv.Atoi(m[1])
	t.Cleanup(func() {
		if pr, err := os.FindProcess(child); err == nil && alive(child) {
			pr.Kill()
		}
	})
	tree, ok := p.(interface{ Contains(pid int) (bool, error) })
	if !ok {
		t.Fatal("the PTY cannot tell its process tree")
	}
	for _, c := range []struct {
		what string
		pid  int
		want bool
	}{
		{"the program", p.Pid(), true},
		{"its child", child, true},
		{"this test", os.Getpid(), false},
	} {
		if in, err := tree.Contains(c.pid); err != nil || in != c.want {
			t.Errorf("%s (%d): %v %v, want %v", c.what, c.pid, in, err, c.want)
		}
	}
}
```

`host/internal/pty/tree_windows_test.go`:

```go
package pty

import (
	"errors"
	"testing"
)

// V07: Contains and Kill at once (Kill closes the job): no race, and once
// the job is closed the tree is not known.
func TestContainsWhileKilled(t *testing.T) {
	p, out := startAgent(t)
	out.waitFor(t, "ready")
	w := p.(*winPTY)
	pid := w.Pid()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			_, _ = w.Contains(pid)
		}
	}()
	if err := p.Kill(); err != nil {
		t.Fatal(err)
	}
	<-done
	if _, err := w.Contains(pid); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("after Kill: %v, want ErrUnsupported", err)
	}
}
```

En `host/internal/pty/job_windows_test.go`, `TestWithoutJobItWarnsAndGoesOn`, justo después de:

```go
			if p.(*winPTY).job != 0 {
				t.Fatal("a job despite the failure")
			}
```

añadir:

```go
			// V07: without a job the agent's tree is not known.
			if _, err := p.(*winPTY).Contains(p.Pid()); !errors.Is(err, errors.ErrUnsupported) {
				t.Fatalf("Contains without a job: %v, want ErrUnsupported", err)
			}
```

Run: `go test ./internal/pty/ -count=1 -run 'Contains|WithoutJob'`. Expected: RED de compilación en Windows (`w.Contains undefined`). En Unix, `TestContainsTheProgramAndItsChildren` falla con "the PTY cannot tell its process tree".

- [ ] **Step 2: `Contains` en Unix y en Windows.**

`host/internal/pty/tree_unix.go`:

```go
//go:build !windows

package pty

import (
	"errors"

	"github.com/hybriqtech/adbuddy/host/internal/procinfo"
	"golang.org/x/sys/unix"
)

// Contains reports whether pid is the program or one of its descendants
// (V07): in the program's session (creack/pty starts it with Setsid, so the
// session id is the program's pid) or with the program up its chain of
// parents. A descendant that left both (setsid, then its parent ended) is
// not found. A process that is gone is an error.
func (p *unixPTY) Contains(pid int) (bool, error) {
	root := p.cmd.Process.Pid
	if pid == root {
		return true, nil
	}
	sid, err := unix.Getsid(pid)
	switch {
	case err == nil && sid == root:
		return true, nil
	case errors.Is(err, unix.ESRCH):
		return false, err
	}
	// Another session, or one this system does not show (EPERM): the
	// parents decide.
	for i := 0; i < 4096 && pid > 1; i++ {
		info, err := procinfo.Get(pid)
		if err != nil {
			return false, err
		}
		if info.PPID == root {
			return true, nil
		}
		pid = info.PPID
	}
	return false, nil
}
```

`host/internal/pty/tree_windows.go`:

```go
package pty

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// maxJobPIDs bounds the processes Contains lists in the job: an agent with
// more is an error, never a "not in the tree".
const maxJobPIDs = 4096

// jobPIDList is JOBOBJECT_BASIC_PROCESS_ID_LIST with room for maxJobPIDs.
type jobPIDList struct {
	assigned, listed uint32
	ids              [maxJobPIDs]uintptr
}

// Contains reports whether pid is the program or one of its descendants
// (V07): a process in its job object, which nothing the program starts can
// leave (V25). Without a job (it could not be made, or Kill closed it) the
// tree is not known: errors.ErrUnsupported.
func (p *winPTY) Contains(pid int) (bool, error) {
	p.jmu.Lock()
	defer p.jmu.Unlock()
	if p.job == 0 {
		return false, errors.ErrUnsupported
	}
	l := new(jobPIDList)
	if err := windows.QueryInformationJobObject(p.job, windows.JobObjectBasicProcessIdList,
		uintptr(unsafe.Pointer(l)), uint32(unsafe.Sizeof(*l)), nil); err != nil {
		return false, err
	}
	if l.listed < l.assigned {
		return false, errors.New("the job holds more processes than were listed")
	}
	for _, id := range l.ids[:l.listed] {
		if int(id) == pid {
			return true, nil
		}
	}
	return false, nil
}
```

En `host/internal/pty/pty_windows.go`:

(a) `winPTY` (:61-62): sustituir

```go
type winPTY struct {
	job    windows.Handle // 0 if there is none
```

por

```go
type winPTY struct {
	jmu    sync.Mutex     // job: Kill closes it while Contains may read it
	job    windows.Handle // 0 if there is none
```

(b) Al final de `Kill` (:320-324), sustituir

```go
		closeAll(p.thread, p.proc)
		if p.job != 0 {
			_ = windows.CloseHandle(p.job)
		}
	})
```

por

```go
		closeAll(p.thread, p.proc)
		p.jmu.Lock()
		if p.job != 0 {
			_ = windows.CloseHandle(p.job)
			p.job = 0
		}
		p.jmu.Unlock()
	})
```

Las lecturas de `p.job` al principio de `Kill` no cambian: solo `Kill` lo escribe, y lo hace bajo `jmu`.

Run: `go test ./internal/pty/ -count=1 -v -run 'Contains|WithoutJob'`. Expected en Windows (prototipo):

```
--- PASS: TestWithoutJobItWarnsAndGoesOn (0.29s)
--- PASS: TestContainsTheProgramAndItsChildren (0.05s)
--- PASS: TestContainsWhileKilled (0.03s)
```

- [ ] **Step 3: `session.InAgentTree`. Test RED y código.**

`host/internal/session/tree_test.go`:

```go
package session

import (
	"errors"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/pty"
)

// treeFake is a fake agent that can tell its process tree.
type treeFake struct {
	*fakePTY
	contains func(pid int) (bool, error)
}

func (f treeFake) Contains(pid int) (bool, error) { return f.contains(pid) }

// V07: InAgentTree asks the agent's PTY. Without an agent nothing is in its
// tree; a PTY that cannot tell, or has no record of its tree, is unknown; any
// other error counts as in the tree.
func TestInAgentTree(t *testing.T) {
	var contains func(int) (bool, error) // read by Start, on this goroutine
	m := New(Options{
		Dir:  t.TempDir(),
		Look: func(name string) (string, error) { return "/bin/" + name, nil },
		Env:  func(string) []string { return nil },
		Start: func(pty.Options) (pty.PTY, error) {
			if contains == nil {
				return newFake(), nil
			}
			return treeFake{newFake(), contains}, nil
		},
	})
	t.Cleanup(m.Stop)
	if in, known := m.InAgentTree(42); in || !known {
		t.Fatalf("no agent: %v %v, want not in it, known", in, known)
	}
	for _, c := range []struct {
		name      string
		contains  func(int) (bool, error)
		in, known bool
	}{
		{"cannot tell", nil, false, false},
		{"in", func(pid int) (bool, error) { return pid == 42, nil }, true, true},
		{"out", func(int) (bool, error) { return false, nil }, false, true},
		{"no record", func(int) (bool, error) { return false, errors.ErrUnsupported }, false, false},
		{"error", func(int) (bool, error) { return false, errors.New("injected") }, true, true},
	} {
		contains = c.contains
		if err := m.Start("claude", false); err != nil {
			t.Fatal(err)
		}
		if in, known := m.InAgentTree(42); in != c.in || known != c.known {
			t.Errorf("%s: %v %v, want %v %v", c.name, in, known, c.in, c.known)
		}
	}
}
```

Run: `go test ./internal/session/ -count=1 -run TestInAgentTree`. Expected: RED de compilación (`m.InAgentTree undefined`).

`host/internal/session/tree.go` (fichero nuevo: `session.go` es de tern y no se toca):

```go
package session

import "errors"

// treeTeller is a PTY that can tell whether a process is the program or one
// of its descendants: pty.Start's PTYs; the fakes of tests need not be.
type treeTeller interface {
	Contains(pid int) (bool, error)
}

// InAgentTree reports whether the process pid is the agent or one of its
// descendants (V07). known is false when that cannot be told: a PTY that
// cannot say, or one without a record of its tree (errors.ErrUnsupported:
// a Windows agent without a job object, a system whose processes are not
// read). Any other error counts as in the tree: what cannot be checked is
// refused. Without an agent running nothing is in its tree.
func (m *Manager) InAgentTree(pid int) (in, known bool) {
	pr := m.current()
	if pr == nil {
		return false, true
	}
	t, ok := pr.p.(treeTeller)
	if !ok {
		return false, false
	}
	in, err := t.Contains(pid)
	switch {
	case errors.Is(err, errors.ErrUnsupported):
		return false, false
	case err != nil:
		return true, true
	}
	return in, true
}
```

Run: `go test ./internal/session/ ./internal/pty/ -count=1`. Expected: ok.

- [ ] **Step 4: Comprobar y hacer commit.**

Run: `go test ./... -count=1 -timeout 15m`, `gofmt -l .`, `go vet ./...`, `GOOS=linux go vet ./...` y `GOOS=darwin go vet ./...`. Expected: todo ok.

Commit: `feat(pty,session): el PTY dice si un proceso es del árbol del agente (V07)`.

### Task 3c: `attach`, `new`, `stop` y `remote` se deciden por el árbol del agente (V07)

**Implementador:** opus. **Revisor:** `adbuddy-reviewer-opus`.

**Files:**
- Create: `host/internal/cli/agentpeer.go` y su test `host/internal/cli/agentpeer_test.go`.
- Modify:
  - `host/internal/cli/serve_session.go` (:111-121);
  - `host/internal/cli/remotecmd.go` (:271-281 e imports);
  - `host/internal/cli/serve.go:283` (una línea, aviso a tern);
  - `host/internal/cli/remotecmd_test.go` (:85 y :718);
  - `host/internal/cli/main_test.go` (`TestMain`, tres líneas).

**Interfaces:**
- Produces: `fromAgent(sess *session.Manager, req ipc.Request, c *ipc.Conn) bool`, del paquete `cli`. `handleRemote` gana el parámetro `sess *session.Manager`, que puede ser nil: `remote_ask_test.go:97` llama a `serveHandler` con nil.
- Consumes: `(*ipc.Conn).Peer`, `ipc.Peer.Same` (T3a) y `(*session.Manager).InAgentTree` (T3b).
- Criterio (C1, I3):
  1. `ADBUDDY_SOURCE=agent` declarado ⇒ agente: la declaración solo añade bloqueo, y un declarado fuera del árbol sigue bloqueado.
  2. Par no `Known` ⇒ no agente: decide solo la declaración, como antes.
  3. Par `Known` con `Err` ⇒ agente.
  4. Si no, `InAgentTree`. Después, `Same()`: si el par ya no es el mismo (acabó, o su PID pasó a otro proceso) ⇒ agente.
  5. Agente si `known && in`. Desconocido (job nulo, PTY falso) ⇒ decide la declaración (I5).

  No es garantía (spec §4.6). Residuos:
  - Unix: `setsid -f`, `systemd-run`, `tmux`, `at`/`cron`, `launchd`;
  - Windows: WMI, `schtasks`, explorer/COM.

- [ ] **Step 1: Test RED de integración (I11).**

El binario de test hace de agente en un PTY real y de sus hijos, con `ADBUDDY_SOURCE` vacía:
- el agente pregunta él mismo;
- un hijo pregunta y espera;
- otro hijo envía la petición y sale sin esperar (C1).

El propio test, fuera del árbol, es atendido salvo si se declara agente.

En `host/internal/cli/main_test.go`, al principio de `TestMain` (:26), insertar:

```go
	// A process of the fake agent's tree (agentpeer_test.go).
	if mode := os.Getenv("ADBUDDY_PEER_MODE"); mode != "" {
		os.Exit(peerHelper(mode))
	}
```

`host/internal/cli/agentpeer_test.go`:

```go
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/pty"
	"github.com/hybriqtech/adbuddy/host/internal/session"
)

// peerHelper is what the test binary does as a process of the agent's tree
// (ADBUDDY_PEER_MODE, see TestMain), always with ADBUDDY_SOURCE empty:
// "agent" is the agent in its PTY, which asks serve itself and then through
// two children; "child" asks serve and waits for the answers; "quitter"
// sends its request and ends without waiting for the answer.
func peerHelper(mode string) int {
	data := os.Getenv("ADBUDDY_DATA_DIR")
	ask := func(kind, arg string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		e, err := ipc.Call(ctx, data, ipc.Request{Kind: kind, Argv: []string{arg}}, nil, io.Discard, io.Discard, nil)
		if err != nil {
			return err.Error()
		}
		return e.ErrorCode
	}
	switch mode {
	case "agent":
		fmt.Printf("self:%s;\r\n", ask(ipc.KindSession, "self"))
		exe, _ := os.Executable()
		for _, m := range []string{"child", "quitter"} {
			c := exec.Command(exe)
			c.Env = append(os.Environ(), "ADBUDDY_PEER_MODE="+m)
			out, err := c.CombinedOutput()
			fmt.Printf("%s:%s %v;\r\n", m, strings.TrimSpace(string(out)), err)
		}
		time.Sleep(time.Hour) // until serve stops it
	case "child":
		fmt.Println(ask(ipc.KindSession, "child"), ask(ipc.KindRemote, "status"))
	case "quitter":
		secret, err := ipc.ReadSecret(data)
		if err != nil {
			return 1
		}
		nc, err := net.Dial("unix", ipc.SocketPath(data))
		if err != nil {
			return 1
		}
		b, _ := json.Marshal(ipc.Request{Secret: secret, Kind: ipc.KindSession, Argv: []string{"quitter"}})
		if ipc.WriteFrame(nc, ipc.FrameRequest, b) != nil {
			return 1
		}
	default:
		return 2
	}
	return 0
}

// peerEnv is the fake agent's environment: this one without
// ADBUDDY_SOURCE, as an agent that cleared it.
func peerEnv(data string) []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(k) {
		case "ADBUDDY_SOURCE", "ADBUDDY_PEER_MODE", "ADBUDDY_DATA_DIR":
			continue
		}
		env = append(env, kv)
	}
	return append(env, "ADBUDDY_PEER_MODE=agent", "ADBUDDY_DATA_DIR="+data)
}

// V07: serve tells the agent's processes by the other end of the socket, not
// by what they declare. The agent and its children are refused with
// ADBUDDY_SOURCE empty, also one that ends right after sending its request;
// a process outside the agent's tree (this test) is served, unless it
// declares itself the agent.
func TestServeRefusesTheAgentsTreeWhateverItDeclares(t *testing.T) {
	data := isolate(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(session.Options{
		Dir:   t.TempDir(),
		Look:  func(string) (string, error) { return exe, nil },
		Env:   func(string) []string { return peerEnv(data) },
		Start: pty.Start,
	})
	t.Cleanup(sess.Stop)
	type answer struct{ probe, code string }
	answers := make(chan answer, 16)
	secret, err := ipc.EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := ipc.Listen(data, secret, func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit {
		var e ipc.Exit
		probe := "remote"
		if req.Kind == ipc.KindRemote {
			e = (&phoneLink{}).handleRemote(ctx, req, c, nil, sess)
		} else {
			e = handleSessionIn(i18n.EN, ctx, sess, "claude", req, c)
			probe = strings.Join(req.Argv, " ")
		}
		answers <- answer{probe, e.ErrorCode}
		return e
	})
	if err != nil {
		t.Skipf("no socket: %v", err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })

	if err := sess.Start("claude", false); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	deadline := time.After(60 * time.Second)
	for len(got) < 4 {
		select {
		case a := <-answers:
			got[a.probe] = a.code
		case <-deadline:
			screen := ""
			if s := sess.Screen(); s != nil {
				screen = s.Text()
			}
			t.Fatalf("answers %v; the agent's screen:\n%s", got, screen)
		}
	}
	for _, probe := range []string{"self", "child", "quitter", "remote"} {
		if got[probe] != "AGENT_SESSION" {
			t.Errorf("%s: %q, want AGENT_SESSION", probe, got[probe])
		}
	}

	// This test is outside the agent's tree: served, unless it declares
	// itself the agent (the declaration only ever adds a refusal).
	call := func(req ipc.Request) string {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		e, err := ipc.Call(ctx, data, req, nil, io.Discard, io.Discard, nil)
		if err != nil {
			t.Fatal(err)
		}
		return e.ErrorCode
	}
	if code := call(ipc.Request{Kind: ipc.KindSession, Argv: []string{"outside"}}); code != "USAGE" {
		t.Errorf("outside: %q, want USAGE (served)", code)
	}
	if code := call(ipc.Request{Kind: ipc.KindRemote, Argv: []string{"status"}}); code != "REMOTE_UNAVAILABLE" {
		t.Errorf("remote outside: %q, want REMOTE_UNAVAILABLE (served)", code)
	}
	agentEnv := map[string]string{"ADBUDDY_SOURCE": "agent"}
	if code := call(ipc.Request{Kind: ipc.KindSession, Argv: []string{"declared"}, Env: agentEnv}); code != "AGENT_SESSION" {
		t.Errorf("declared: %q, want AGENT_SESSION", code)
	}
}
```

En `host/internal/cli/remotecmd_test.go`:
- :85: `return l.handleRemote(ctx, req, c, d)` → `return l.handleRemote(ctx, req, c, d, nil)`;
- :718: `…Argv: []string{"on"}}, nil, d)` → `…Argv: []string{"on"}}, nil, d, nil)`.

En `host/internal/cli/remotecmd.go:279`, cambiar la firma a `func (l *phoneLink) handleRemote(ctx context.Context, req ipc.Request, c *ipc.Conn, d *daemon.Daemon, sess *session.Manager) ipc.Exit` y añadir el import `"github.com/hybriqtech/adbuddy/host/internal/session"`, tras `.../remote`. En `host/internal/cli/serve.go:283`: `return link.handleRemote(ctx, req, c, d)` → `return link.handleRemote(ctx, req, c, d, sess)`. La comprobación de `:280` aún no cambia.

Run: `go test ./internal/cli/ -count=1 -run TestServeRefusesTheAgentsTree`. Expected RED (prototipo, Windows):

```
--- FAIL: TestServeRefusesTheAgentsTreeWhateverItDeclares (2.09s)
    agentpeer_test.go:145: self: "USAGE", want AGENT_SESSION
    agentpeer_test.go:145: child: "USAGE", want AGENT_SESSION
    agentpeer_test.go:145: quitter: "USAGE", want AGENT_SESSION
    agentpeer_test.go:145: remote: "REMOTE_UNAVAILABLE", want AGENT_SESSION
```

- [ ] **Step 2: `fromAgent` y los dos manejadores.**

`host/internal/cli/agentpeer.go`:

```go
package cli

import (
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/session"
)

// fromAgent reports whether a request to serve comes from the agent's
// session, so that attach, new, stop and remote refuse it (V07). The agent's
// own declaration (ADBUDDY_SOURCE=agent) only ever adds a refusal. What
// decides is the process at the other end of the socket: in the agent's
// process tree, or named by the system but no longer checkable (it ended,
// its PID went to another process, it could not be opened), it is the
// agent. Only when the system does not name it, or the agent's tree is not
// known (no job object on Windows), does the declaration alone decide.
//
// Not a guarantee (spec §4.6): a process the agent detaches from its tree
// is not seen (setsid -f, systemd-run, tmux, at/cron, launchd; WMI,
// schtasks or explorer/COM on Windows).
func fromAgent(sess *session.Manager, req ipc.Request, c *ipc.Conn) bool {
	if req.Env["ADBUDDY_SOURCE"] == "agent" {
		return true
	}
	p := c.Peer()
	switch {
	case !p.Known:
		return false
	case p.Err != nil:
		return true
	}
	in, known := false, true
	if sess != nil {
		in, known = sess.InAgentTree(p.PID)
	}
	if !p.Same() {
		return true // it ended, or its PID went to another process, meanwhile
	}
	return known && in
}
```

En `host/internal/cli/serve_session.go`:
- el comentario de :111-112 queda:

  ```go
  // sessionRequest answers the IPC requests of the session: attach, new, stop.
  // The agent may not use them: it would type into its own terminal. Who is
  // the agent is fromAgent's to say (V07).
  ```
- en :119, `if req.Env["ADBUDDY_SOURCE"] == "agent" {` → `if fromAgent(sess, req, c) {`.

En `host/internal/cli/remotecmd.go`:
- las tres primeras líneas del comentario de `handleRemote` (:271-273) quedan:

  ```go
  // handleRemote answers `adbuddy remote`. The agent is refused (fromAgent: its
  // process tree, as serve sees it, or its declaration; sess may be nil), but a
  // process it detaches from its tree is not seen, so the real defence of `on`
  // is the question: the user answers it (app or terminal), every time, and
  // only for this request.
  ```
- en :280, `if req.Env["ADBUDDY_SOURCE"] == "agent" {` → `if fromAgent(sess, req, c) {`.

Run: `go test ./internal/cli/ -count=1 -run 'TestServeRefusesTheAgentsTree|TestHandleSession|TestNewAndStop|TestSessionRequest|Remote'`. Expected: ok (`TestServeRefusesTheAgentsTreeWhateverItDeclares`, unos 2 s en Windows).

- [ ] **Step 3: Comprobar, hacer commit y pasar por la CI.**

Run: `go test ./... -count=1 -timeout 15m`, `gofmt -l .`, `go vet ./...`, `GOOS=linux go vet ./...` y `GOOS=darwin go vet ./...`. Expected: todo ok.

Commit: `fix(cli): attach, new, stop y remote se deciden por el árbol del agente, no por su declaración (V07)`.

CI:
- Subir la rama como copia, tras el `git grep` de seriales e IPs que pide `CLAUDE.md` y sin `--force`, y mirar `host.yml` en Linux y macOS.
- Si macOS falla en `getsid` o en `LOCAL_PEERPID`, se arregla en esta tarea: es su criterio.

Informe (`task-3-report.md`):
- tabla por plataforma: qué corrió aquí (Windows) y qué en la CI;
- residuos:
  - lo que se separa del árbol (Unix y Windows);
  - un Windows sin `SIO_AF_UNIX_GETPEERPID` (decide la declaración);
  - agente sin job (decide la declaración);
  - PIDs intermedios reciclados durante la cadena de padres;
- textos nuevos: ninguno (`AGENT_SESSION` ya existe).

### Task 4: V27 descartada por decisión de producto (docs)

**Implementador:** Sonnet. **Revisor:** `adbuddy-reviewer`.

**Se ejecuta DESPUÉS de rebasar la rama sobre `main` con la PR #20 integrada:** #20 también edita `pendientes-m3-m5.md`. Si #20 no está en `origin/main`, T4 espera y se dice en el ledger.

**Decisión (del usuario, 2026-10-02):** el agente hereda el entorno sin filtrar, salvo lo que ADBuddy tiene que quitar; `agent.Env` se queda como está. No se toca código.

**Files:**
- Modify:
  - `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md` (§5.1);
  - `docs/superpowers/plans/pendientes-m3-m5.md`;
  - `.superpowers/sdd/2026-10-02-v1-diferidos/triage.md` (local y en `.gitignore`: se edita y no se versiona).

- [ ] **Step 1: Rebase.** `git fetch origin`. Si `git branch -r --contains origin/fix/v1-diferidos` lista `origin/main` y la rama aún no está rebasada, `git rebase origin/main`. Los conflictos se resuelven a favor de lo de `main` y se dice en el informe. Después, `go test ./... -count=1 -timeout 15m` desde `host/`.

- [ ] **Step 2: Spec §5.1.** Sustituir el punto entero que empieza por `- **Entorno explícito.** El host lanza el agente con una lista de variables permitidas.` por:

```markdown
- **Entorno del agente.** El agente hereda, sin filtrar, el entorno de quien lanza el host: así respeta la configuración global del usuario (autenticación, proxy, `CLAUDE_CONFIG_DIR`…) en los tres sistemas. ADBuddy solo quita o fija lo que necesita:
  - quita las variables de la sesión que lanzó el host (`CLAUDECODE*`, `CLAUDE_CODE_*`, `CLAUDE_EFFORT`, `CLAUDE_PID` y `AI_AGENT`) para no arrastrarla; en S3, lanzado desde otra sesión de Claude Code, el agente usó el esfuerzo de la sesión padre;
  - quita `ANDROID_SERIAL`, `ANDROID_ADB_SERVER_PORT` y `ADBUDDY_SOURCE`, y fija `ANDROID_SERIAL` y `ADBUDDY_SOURCE=agent`; pone el shim el primero del `PATH`;
  - en la sesión de `serve`, cambia las variables de la terminal que lo lanzó (`TERM`, `COLORTERM`…) por las del PTY.

  Una lista de variables permitidas se descartó por decisión de producto (V27, 2026-10-02).
```

- [ ] **Step 3: `pendientes-m3-m5.md`.**

(a) En "M3 · Chat: daemon con PTY", sustituir el punto entero que empieza por `- **Lista de variables de entorno permitidas** para lanzar el agente (§5.1).` por:

```markdown
- **Lista de variables de entorno permitidas** para lanzar el agente (§5.1): **DESCARTADA por decisión de producto (V27, 2026-10-02).** El agente hereda el entorno de quien lanza el host sin filtrar, salvo lo que ADBuddy tiene que quitar o fijar. `host/internal/agent.Env` se queda como está: quita `CLAUDECODE*`, `CLAUDE_CODE_*`, `CLAUDE_EFFORT`, `CLAUDE_PID`, `AI_AGENT`, `ANDROID_SERIAL`, `ANDROID_ADB_SERVER_PORT` y `ADBUDDY_SOURCE`. La spec §5.1 lo dice desde la ola 2 H.
```

(b) En "v1 · Diferidos tras la ola 1", en la frase "No son de esta rama y siguen donde estaban: …", sustituir `V27 (decisión de producto)` por `V27 (descartada por decisión de producto en la ola 2 H; ver M3)`.

- [ ] **Step 4: Triage (local).** En `.superpowers/sdd/2026-10-02-v1-diferidos/triage.md`:
- en la fila `| V27 |`, anteponer a la descripción `**DESCARTADO (decisión de producto, 2026-10-02):** el agente hereda el entorno sin filtrar salvo lo que ADBuddy quita; \`agent.Env\` se queda. `;
- en la fila del grupo `F. Entorno del agente`, cambiar `V27 (3 SO: la CI cubre Linux y macOS)` por `V27 (descartado)`.

- [ ] **Step 5: Commit.** Run: `git diff --stat`. Expected: solo la spec y `pendientes-m3-m5.md`; el triage no aparece porque está en `.gitignore`.

Commit (`git add` de las dos rutas): `docs(v1): V27 descartada: el agente hereda el entorno salvo lo que ADBuddy quita`.

---

## Validaciones manuales pendientes

Nada de esto se prueba sin el PC del usuario o su permiso (M5). Van a `pendientes-m3-m5.md` en la reconciliación final.
- **`attach` desde fuera del árbol del agente,** que tiene que funcionar: otra terminal, la terminal integrada de un IDE, una sesión ssh, una consola elevada (administrador) y WSL1. WSL2 no conecta con el AF_UNIX de Windows: no es un caso.
- **`SIO_AF_UNIX_GETPEERPID` en las versiones de Windows 10 y 11 que se soporten.** Sin él, T3 deja decidir a la declaración, como antes.
- **Un agente real que vacía `ADBUDDY_SOURCE` e intenta `adbuddy attach`, `new`, `stop` y `remote on`:** debe recibir `AGENT_SESSION`.
- **Un agente en pausa desde el móvil que ejecuta `adbuddy doctor`:** no llama a adb y `serve` sale como aviso.
- **La carpeta `journal` movida por el usuario a otro disco con una junction:** ahora se rechaza, y `doctor` dice "is not a folder".
- **macOS:** `getsid` sobre un proceso de otra sesión, cubierto por la CI (`host.yml`). Si da `EPERM`, decide la cadena de padres.

## Residuos (a `pendientes-m3-m5.md` en la reconciliación final)

- **V07:**
  - no es garantía (spec §4.6): lo que el agente separa de su árbol no se ve (Unix: `setsid -f`, `systemd-run`, `tmux`, `at`/`cron`, `launchd`; Windows: WMI, `schtasks`, explorer/COM);
  - un Windows sin el ioctl o un agente sin job vuelven a la declaración;
  - en Unix, un PID intermedio reciclado mientras se recorre la cadena de padres.
- **Backups:** `adb pull` escribe en `journal/backups/<id>/original` por ruta, después de `MakeBackupDir`.
- **`adb push`:** lee cualquier fichero del PC (aparte, M2).
- **V20:** es coherencia, no frontera.
