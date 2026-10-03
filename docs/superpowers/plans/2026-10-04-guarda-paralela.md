# Guarda de primer plano en paralelo (P4): plan

> Para quien ejecute: `superpowers:subagent-driven-development`. Rama `perf/guarda-paralela` (desde `perf/latencia`); worktree `C:/Users/Jose/orca/workspaces/adbuddy/latencia` (el checkout principal `C:/Users/Jose/Documents/projects/adbuddy` NO se toca: rutas absolutas y `cd <worktree> && …` en el mismo comando; `git -C <worktree> …`; sin worktrees nuevos). Commits con identidad hybriqtech y el trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. `go test -race` no corre en este PC. Nunca modelos Haiku. Diagnóstico: `2026-10-03-latencia.md`, propuesta P4.

**Objetivo:** que la guarda de primer plano haga a la vez las lecturas que no dependen una de otra, para que cada orden con pantalla (`screen`, `tap`, `type`, `input …`, `screencap`, cada lectura de `--screen`) tarde lo que la más lenta y no la suma. **Sin abrir ningún hueco**: las dos respuestas siguen siendo obligatorias y ante la duda se bloquea, con las mismas razones y en el mismo orden de decisión que hoy.

## Qué hace hoy `guardForeground` (host/internal/daemon/guard.go)

En serie: `inFront` (pregunta a la app el primer plano; si no contesta, `dumpsys activity activities` y quizá `dumpsys power`) → recorre los paquetes (ADBuddy delante, apps protegidas) → `appProtected` → `systemUIReason`, que lee `dumpsys window | grep mCurrentFocus` (la sombra de notificaciones abierta) y, solo si la app no dijo el primer plano (`!viaApp`), `dumpsys window windows` (ventanas de systemui no fijas) → `screenOff` solo si una razón se va a levantar con la tecla de despertar.

Las dos lecturas de `systemUIReason` no usan nada de lo que dice `inFront`: se pueden lanzar a la vez que `inFront`. Lo que sí depende de `inFront` es **cómo se interpretan** (`viaApp`, `closing`), y eso no cambia: se lee el resultado en el mismo punto y con el mismo orden de razones que ahora.

## Diseño

```go
// readResult is the answer of one of the guard's own dumpsys reads.
type readResult struct {
	out  string
	code int
	err  error
}

// startRead runs the shell command cmd on the phone in the background and
// returns where its answer will be (one answer, never closed: the buffer
// lets the goroutine end even if nobody reads it).
func (d *Daemon) startRead(ctx context.Context, cmd string) <-chan readResult {
	ch := make(chan readResult, 1)
	go func() {
		out, code, err := d.o.ADB.Shell(ctx, cmd)
		ch <- readResult{out, code, err}
	}()
	return ch
}
```

`guardForeground`:

```go
func (d *Daemon) guardForeground(ctx context.Context, words []string) string {
	// The reads of the system UI do not depend on what is in front: they start
	// together with it. Both answers are still required (systemUIReason waits
	// for them); a read still running when the guard answers is stopped.
	rctx, stop := context.WithCancel(ctx)
	defer stop()
	focus := d.startRead(rctx, focusCmd)
	var windows <-chan readResult
	if d.o.UI == nil { // without the app viaApp is always false: that read is always needed
		windows = d.startRead(rctx, windowsCmd)
	}
	f, err := d.inFront(ctx)
	if err != nil {
		return "could not tell which app is in the foreground (" + err.Error() + "); ADBuddy does not touch or read the screen without knowing"
	}
	... (el resto, sin cambios, hasta la llamada)
	if why := d.systemUIReason(ctx, f.viaApp, closesShade(words), focus, windows); why != "" && !(wakesScreen(words) && d.screenOff(ctx)) {
		return why
	}
	return ""
}
```

`systemUIReason` recibe los canales y lee de ellos donde antes llamaba a `d.o.ADB.Shell`:

```go
func (d *Daemon) systemUIReason(ctx context.Context, viaApp, closing bool, focus, windows <-chan readResult) string {
	const unknown = "; ADBuddy does not touch or read the screen without knowing"
	r := <-focus
	focusTitles, ok := uidump.ParseFocus(r.out)
	if r.err != nil || r.code != 0 || !ok {
		return "could not tell whether the notification shade is open" + unknown
	}
	if closing {
		return ""
	}
	for _, title := range focusTitles {
		if uidump.IsShade(title) {
			return "the notification shade is open; close it with Back (input keyevent 4) or ask the user to do this part"
		}
	}
	if viaApp {
		return "" // the app's foreground says if a notification is on screen
	}
	if windows == nil { // the app was there but did not answer: read it now, as before
		windows = d.startRead(ctx, windowsCmd)
	}
	w := <-windows
	list, ok := uidump.ParseSystemUIWindows(w.out)
	if w.err != nil || w.code != 0 || !ok {
		return "could not tell whether a system window is on screen" + unknown
	}
	... (igual: si hay ventanas, la razón de siempre)
}
```

(Mantener los nombres de variables de `focus` que usa hoy el código; aquí se renombra a `focusTitles` solo porque el canal ya se llama `focus`.)

Propiedades que el diseño conserva (y que los tests comprueban):
1. **Las dos respuestas son obligatorias.** Si `inFront` falla, se devuelve la razón de siempre (la lectura de `focus` en curso se cancela con `stop`). Si `inFront` va bien y la lectura de `focus` falla, no devuelve el error ni sale vacía: bloquea con «could not tell whether the notification shade is open», igual que hoy. Si falta `windows` o no se entiende, bloquea. Nada se da por bueno por no haber llegado.
2. **Mismo orden de decisión.** Los paquetes protegidos, ADBuddy delante y `appProtected` se deciden antes de mirar el resultado de `focus`, como hoy (`focus` se espera dentro de `systemUIReason`, que se llama después). Una razón de protegida gana a una de «no se pudo leer la sombra», como hoy.
3. **Las mismas llamadas a adb que hoy**, salvo una más, sin efecto, en un caso: sin app (`d.o.UI == nil`) `windows` se lee siempre, aunque la guarda acabe antes por una app protegida (una lectura de solo lectura, sin efectos; se cancela si sobra). Con app, nada cambia en el número de llamadas.
4. **Sin fugas.** Los canales tienen buffer 1 y las lecturas llevan `rctx`: al volver la guarda (`defer stop()`) se cancelan y la gorrutina termina cuando `Shell` vuelve (`adb.Runner` mata el proceso al cancelarse el contexto). El ejecutor (`Executor`) se usa ya desde varias gorrutinas (las órdenes concurrentes de `serve`): `adb.Runner` y `activePhone` no tienen estado compartido sin candado.
5. **Una cancelación de `ctx`** (el agente se fue, `serve` para) deja las lecturas con error: bloquea con las mismas razones; `refuse`/`ui()` ya distinguen `ctx.Err()` y lo apuntan como abandono.

### Lo que sí cambia, y se acepta: la edad de la lectura de la sombra

Hoy `mCurrentFocus` se lee **después** de `inFront`. Con el paralelo, se lee a la vez, es decir, hasta lo que tarde `inFront` (una ida y vuelta a la app, decenas de ms, o 2-3 llamadas a adb sin app) **antes** de lo que se leía. La ventana entre «la guarda lee» y «la acción actúa» crece en ese tiempo para este solo dato. Esto no es un hueco nuevo en la naturaleza (ya existía la ventana entre la guarda y la acción: el usuario puede abrir la sombra después de cualquier lectura) y la app comprueba por su cuenta, atómicamente con la acción, lo demás (protegida, notificación, bloqueo, pausa); solo la sombra la comprueba únicamente el host. Pero es un coste real y se declara: **Ruling:** aceptar el paralelo de `focus` con `inFront` — ahorra una ida y vuelta de la app (o varias llamadas a adb sin app) por orden con pantalla — cuesta: una ventana de TOCTOU de la sombra mayor en lo que dura `inFront`; si el revisor de seguridad no la acepta, la alternativa es paralelizar solo sin app (donde el ahorro es mayor) o solo `windows`, o no hacer P4.

## Tareas

### T1: guarda en paralelo (host/internal/daemon/guard.go, guard_parallel_test.go nuevo, y los tests existentes que miran el orden de las llamadas)

- [ ] **Paso 1: tests que fallan** (`guard_parallel_test.go`), con el `fakeADB` y la `UI` falsa de los tests del daemon (`daemon_test.go`, `ui_test.go`, `guard_api34_test.go`, `dumpsys_guard_test.go`):
  1. `TestGuardReadsRunTogether`: la `UI` falsa del `foreground` espera (con tope de 2 s) a que la lectura de `mCurrentFocus` haya empezado, y esa lectura espera a que haya empezado el `foreground`: sólo termina si corren a la vez (en serie, uno espera al otro hasta el tope y el test falla con «ran in series»). Lo mismo sin app: `dumpsys activity activities` y `mCurrentFocus` empiezan a la vez.
  2. `TestGuardBothAnswersAreRequired`: tabla, con la app en primer plano no protegido y Settings delante: (a) la lectura de focus da error → bloquea con «could not tell whether the notification shade is open»; (b) código de salida ≠ 0; (c) salida que no se entiende; (d) focus bien y la app falla (cae a dumpsys) → pasa como hoy; (e) focus bien y `inFront` falla del todo → «could not tell which app is in the foreground»; (f) los dos mal → bloquea (la razón de `inFront` primero, como hoy). Sin app: `windows` con error, con código ≠ 0 y no entendida → «could not tell whether a system window is on screen».
  3. `TestGuardOrderOfReasons`: con una app protegida delante **y** la lectura de focus rota, la razón es la de la app protegida (no la de la sombra); con ADBuddy delante y la sombra abierta, la de ADBuddy; con la sombra abierta y nada más, la de la sombra; con `appProtected` y focus roto, la de pantalla protegida.
  4. `TestGuardStopsItsReads`: cuando la guarda vuelve sin haber esperado una lectura (app protegida delante, o `inFront` falla), el contexto de esa lectura queda cancelado y su gorrutina termina (el `Shell` falso observa `ctx.Done()` y avisa por un canal; el test lo espera con tope).
  5. `TestGuardSameDecisionsOnRecordedStates`: la tabla de estados grabados de `uidump/testdata/api34` (los de `guard_api34_test.go`) da las mismas decisiones que antes: se reutiliza esa tabla tal cual sin tocar sus expectativas.
  Los tests existentes que comparan la **lista ordenada** de llamadas de adb (`ran()`) de la guarda pasan a comparar el **conjunto** (orden indiferente) cuando el orden era de la guarda; no se cambia ninguna otra expectativa. Todos deben seguir verdes: es la prueba de que las decisiones no cambian.
- [ ] **Paso 2: implementar** `readResult`, `startRead`, el cambio de `guardForeground` y de `systemUIReason` tal como arriba (sólo `guard.go`).
- [ ] **Paso 3:** `cd <worktree>/host && go vet ./... && go test ./internal/daemon -timeout 10m -count=1` y `go test ./... -timeout 15m -count=1`. `git diff --stat` antes de commitear (LF, sin reescribir ficheros).
- [ ] **Paso 4:** commit `perf(daemon): la guarda lee la sombra a la vez que el primer plano, con las dos respuestas obligatorias`.

### T2: medir (no es código)

Con el arnés de dispositivo (`TestLatencyDevice`, `internal/emutest/latency_test.go`) en el emulador: antes (`perf/latencia`) y después (esta rama), mismo comando, mismas pasadas; tabla por etapa (`app foreground`, `adb shell dumpsys window | grep`, total) para `adbuddy screen`, `tap`, `adb shell input keyevent`, y sin app. Resultado en `2026-10-03-latencia.md`. Si el ahorro no es claro (> ruido), no se integra.

## Revisión de seguridad que se pide a opus (plan y diff)

1. ¿Las dos respuestas siguen siendo obligatorias en todos los caminos (incluidos `windows` y `screenOff`)? ¿Hay algún camino en que una lectura que no llegó, llegó tarde o se canceló se dé por buena?
2. ¿El orden de decisión y las razones son idénticos a los de hoy (tabla de estados grabados)?
3. La ventana de TOCTOU de la sombra (sección «Lo que sí cambia»): ¿aceptable, o hay que paralelizar solo sin app / no hacerlo? ¿Algún otro dato cuya edad crezca (`focus` frente a `viaApp`/`closing`, `windows` sin app)?
4. Gorrutinas: fugas, `Shell` de un ejecutor que no sea seguro para uso concurrente (`activePhone`, `timedADB` del arnés, los falsos de los tests), cancelación al volver, sin condiciones de carrera (sin `-race` local: leer los valores antes de lanzar gorrutinas).
5. Que `settle()` (una guarda por lectura) y el resto de llamadores de `guardForeground` no cambian de comportamiento.
