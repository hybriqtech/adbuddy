# Plan: el `Kill` libera el `Write` atascado del master en Linux (V08, fallo real de #23 en la CI de Linux)

> Hallazgo de la CI de Linux (runner WSL de puffin, go1.27.0, kernel 6.18, `-race`): `TestStopEndsWithARealAgentThatDoesNotRead` (`host/internal/session/writer_child_test.go:93`) falla con «the Kill did not free the writer's Write» (`go test -race -count=4 -run TestStopEndsWithARealAgentThatDoesNotRead ./internal/session` → 1 PASS, 3 FAIL; el escritor se atasca a ~35-38 KB y tras `Stop`+`Kill` su `Write` no vuelve en 10 s). Es justo lo que V08 debía garantizar: un agente que no lee no puede colgar el host.

## Causa (verificada en el código, no supuesta)

`pty/pty_unix.go` abre el pty con `creack/pty` (`cpty.StartWithSize`). En `creack/pty v1.1.24`, `ioctl(f, …)` llama a `f.Fd()` (`ioctl.go:9`: «Fall back to blocking io»; la variante no bloqueante `ioctlNonblock` está marcada «Unused»), y en Go `(*os.File).Fd()` **pone el fd en modo bloqueante**. Además `unixPTY.Resize` usa `cpty.Setsize(p.f, …)`, que repite `Fd()`.

Un `os.File` en modo bloqueante no pasa por el poller: `Write` es un `write(2)` bloqueado en el kernel y **`Close()` no lo interrumpe** (sin poller no hay `evict`; el cierre real del fd espera a que vuelva el `Write`). El `Write` solo vuelve si el kernel lo despierta (EIO al cerrarse el último fd del esclavo), y la repro de puffin demuestra que en Linux no ocurre a tiempo (el atasco se queda tras `SIGKILL` al grupo y `closeTerm`). En Windows no aplica (ConPTY; `Kill` ya cancela la E/S, #23).

## Diseño de la corrección

**Ruling:** devolver el master a un `os.File` **pollable y no bloqueante** justo después de `cpty.StartWithSize` y no volver a llamar a `Fd()` sobre él: así `Close()` (`Kill`/`watch`) hace `evict` del poller y un `Write` (o `Read`) bloqueado vuelve de inmediato con `os.ErrClosed` — por qué: es el mecanismo que ya asume `Kill` («closes the terminal») y no depende de cómo el kernel despierte a un `write(2)`; sin dependencias nuevas (`x/sys` ya está en `go.mod`) — qué cuesta si me equivoco: en un SO donde el poller no admita el pty (macOS/kqueue en alguna versión) el fd nuevo no sería pollable; **por eso hay guarda y respaldo** (abajo) y el comportamiento previo queda como fallback.

1. `pty_unix.go`, nueva función `pollableMaster(f *os.File) *os.File`, llamada en `Start` tras `cpty.StartWithSize` (el hijo ya heredó el esclavo; el master solo lo usa el host):
   - `fd, err := syscall.Dup(int(f.Fd()))` (el `Fd()` aquí es el último, sobre el fichero que se va a cerrar), `syscall.CloseOnExec(fd)`, `syscall.SetNonblock(fd, true)` (la bandera `O_NONBLOCK` es de la descripción de fichero abierta, compartida entre `f` y el dup), `nf := os.NewFile(uintptr(fd), f.Name())`.
   - **Guarda:** comprobar que `nf` quedó en el poller (`nf.SetWriteDeadline(time.Time{})` devuelve `os.ErrNoDeadline` si no es pollable). Si no lo es, o si `Dup`/`NewFile` fallan: se cierra `nf`/el dup (sin tocar `f`), se **devuelve el `f` original** y se deja el comportamiento anterior (el `SetNonblock(false)` sobre la descripción compartida se restaura para `f`). Nunca un master no bloqueante sin poller (daría `EAGAIN` en `Read`).
   - Si todo va bien: `f.Close()` (cierra solo el fd viejo; la descripción sigue abierta por el dup) y se devuelve `nf`.
2. `unixPTY.Resize`: sin `Fd()`. `nf.SyscallConn().Control(func(fd uintptr){ unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, &unix.Winsize{Col, Row}) })` (`golang.org/x/sys/unix`, que cubre Linux y macOS; no se usa `cpty.Setsize`, que llama a `Fd()`). Si el `SyscallConn` falla se devuelve el error.
3. `Kill`/`watch`/`closeTerm` no cambian: `closeTerm` ya hace `p.f.Close()`; ahora un `Write`/`Read` bloqueado vuelve con `os.ErrClosed`. `Read` ya traduce `ErrClosed` a `io.EOF`. `Write` devuelve el error; el escritor (K1) ya trata cualquier error de `Write` terminando el trabajo.
4. `session/writer.go`: comprobar (sin cambiar el diseño) que un `Write` que vuelve con error tras `Kill` termina el trabajo y que `wdone` se cierra; si el escritor deja pasar el error sin salir, arreglarlo.

## Tests (deterministas; deben fallar ANTES del arreglo en Linux)

- **`pty/pty_unix_test.go` (nuevo, `//go:build !windows`; en Linux corre el de verdad):** `TestKillFreesAWriteBlockedOnADeafChild`: arranca por `pty.Start` un hijo que no lee su entrada (el propio binario de test re-ejecutado con una variable de entorno que solo duerme, como `writer_child_test.go`), escribe líneas en una goroutine hasta que el `Write` deja de avanzar (sin progreso 1 s, medido con un contador atómico de bytes), llama a `p.Kill()` y exige que la goroutine **vuelva en ≤ 5 s** con error. Si el terminal se lo traga todo (más de 4 MiB sin atascarse, p. ej. algún macOS), `t.Skip` con la razón. Sin el arreglo (master bloqueante) la goroutine no vuelve: fallo. `-count=20` en la CI de Linux/`-race`.
- **`pty/pty_unix_test.go`:** `TestMasterIsPollable`: tras `Start`, `p.(*unixPTY).f.SetWriteDeadline(time.Now().Add(time.Hour))` no da `ErrNoDeadline` (garantiza el modo pollable; si el fallback se activó en esa plataforma, el test lo dice con `t.Skip`) y `Resize` no vuelve a poner el fd en modo bloqueante (tras `Resize`, repetir la comprobación).
- `session/writer_child_test.go`: el test existente sigue como prueba de integración (ya espera ≤ 10 s tras `Stop`); no se cambia salvo que haga falta (mensaje del fallo con el último error de `Write`).
- Locales (Windows): `go vet`, `gofmt`, `go test ./...` y `GOOS=linux go vet ./internal/pty/ ./internal/session/` + `GOOS=darwin go build ./...`; el test nuevo solo corre en Unix: **se compila para Linux** (`GOOS=linux go test -c ./internal/pty`) y se entrega el binario a puffin para `-count=20` si lo pide el supervisor; nadie accede al runner Linux salvo puffin.

## Orden y límites

Una tarea (Sonnet, `adbuddy-implementer`; si BLOCKED o ronda 4-5, `adbuddy-implementer-opus`) con dos o tres commits (pty, test, escritor si hace falta) y revisión `adbuddy-reviewer-opus` medium (concurrencia/pty). Push sin `--force` y PR; la CI de Linux (runner WSL, `go test -race`) lo verifica. Validación manual: ninguna sin móvil; la de K1 (carga real del PTY) pasa a verificada en Linux cuando el test corra.

**Criterios de aceptación.** El test nuevo falla sin el arreglo y pasa con él en Linux (`-race -count=20`); `TestStopEndsWithARealAgentThatDoesNotRead` pasa `-count=20` en Linux; Windows no cambia; ningún camino de lectura del pty cambia (`Read` sigue acabando con `io.EOF` al cerrar el último fd del esclavo).
