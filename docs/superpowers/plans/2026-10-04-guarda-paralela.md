# Guarda de primer plano en paralelo (P4): plan

> Para quien ejecute: `superpowers:subagent-driven-development`. Rama `perf/guarda-paralela` (desde `perf/latencia`); worktree `C:/Users/Jose/orca/workspaces/adbuddy/latencia` (el checkout principal `C:/Users/Jose/Documents/projects/adbuddy` NO se toca: rutas absolutas y `cd <worktree> && …` en el mismo comando; `git -C <worktree> …`; sin worktrees nuevos). Commits con identidad hybriqtech y el trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. `go test -race` no corre en este PC. Nunca modelos Haiku. Diagnóstico: `2026-10-03-latencia.md`, propuesta P4. Revisión opus del plan hecha: este texto incorpora sus hallazgos (cota de edad, esperar a las gorrutinas, tests con ejecutor propio, medición).

**Objetivo:** que la guarda de primer plano haga a la vez las lecturas que no dependen una de otra, para que cada orden con pantalla (`screen`, `tap`, `type`, `input …`, `screencap`, cada lectura de `--screen`) tarde lo que la más lenta y no la suma. **Sin abrir ningún hueco**: las dos respuestas siguen siendo obligatorias, ante la duda se bloquea, con las mismas razones y en el mismo orden de decisión que hoy, y una lectura que haya envejecido más de `focusSlack` se vuelve a hacer.

## Qué hace hoy `guardForeground` (host/internal/daemon/guard.go)

En serie: `inFront` (pregunta a la app el primer plano; si no contesta en `foregroundWait` (3 s), `dumpsys activity activities` y quizá `dumpsys power`) → recorre los paquetes (ADBuddy delante, apps protegidas) → `appProtected` → `systemUIReason`, que lee `dumpsys window | grep mCurrentFocus` (la sombra de notificaciones abierta) y, solo si la app no dijo el primer plano (`!viaApp`), `dumpsys window windows` (ventanas de systemui no fijas) → `screenOff` solo si una razón se va a levantar con la tecla de despertar.

Las dos lecturas de `systemUIReason` no usan nada de lo que dice `inFront`: se pueden lanzar a la vez que `inFront`. Lo que sí depende de `inFront` es **cómo se interpretan** (`viaApp`, `closing`), y eso no cambia: se lee el resultado en el mismo punto y con el mismo orden de razones que ahora.

## Diseño

```go
// focusSlack is how old a read of the system UI (the shade, the system windows)
// may be when the guard uses it. The reads start together with inFront; if
// inFront took longer than this (a slow or silent app), they are made again,
// after it, as the guard did before it ran them in parallel. A variable for the
// tests.
var focusSlack = 150 * time.Millisecond

// readResult is the answer of one of the guard's own dumpsys reads, and when it came.
type readResult struct {
	out  string
	code int
	err  error
	done time.Time // when the read ended
}

// guardReads are the reads the guard started in the background. wait ends
// them all (cancelled or finished) before the guard answers, so nothing it
// started outlives it.
type guardReads struct {
	wg sync.WaitGroup
}

// start runs the shell command cmd on the phone in the background and returns
// where its answer will be (one answer; the buffer lets the goroutine end even
// if nobody reads it).
func (g *guardReads) start(ctx context.Context, adb Executor, cmd string) <-chan readResult {
	ch := make(chan readResult, 1)
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		out, code, err := adb.Shell(ctx, cmd)
		ch <- readResult{out, code, err, time.Now()}
	}()
	return ch
}
```

`guardForeground` (los comentarios largos de hoy se conservan; se añade lo siguiente):

```go
func (d *Daemon) guardForeground(ctx context.Context, words []string) string {
	// Nothing is started for a context that is already over (as before: the reads would fail).
	if err := ctx.Err(); err != nil {
		return "could not tell which app is in the foreground (" + err.Error() + "); ADBuddy does not touch or read the screen without knowing"
	}
	// The reads of the system UI do not depend on what is in front: they start
	// together with it. Both answers are still required (systemUIReason waits
	// for them and makes them again if they are older than focusSlack); what
	// is still running when the guard answers is stopped and waited for.
	rctx, stop := context.WithCancel(ctx)
	var reads guardReads
	defer func() { stop(); reads.wg.Wait() }()
	focusCh := reads.start(rctx, d.o.ADB, focusCmd)
	var windowsCh <-chan readResult
	if d.o.UI == nil { // without the app viaApp is always false: that read is always needed
		windowsCh = reads.start(rctx, d.o.ADB, windowsCmd)
	}
	f, err := d.inFront(ctx)
	if err != nil {
		return "could not tell which app is in the foreground (" + err.Error() + "); ADBuddy does not touch or read the screen without knowing"
	}
	... (el resto, sin cambios, hasta la llamada)
	if why := d.systemUIReason(ctx, f.viaApp, closesShade(words), focusCh, windowsCh); why != "" && !(wakesScreen(words) && d.screenOff(ctx)) {
		return why
	}
	return ""
}
```

`systemUIReason` recibe los canales (se llaman `focusCh` y `windowsCh`; los nombres del cuerpo actual, `focus`, `windows`, `out`, `ok`, no cambian) y lee de ellos donde antes llamaba a `d.o.ADB.Shell`:

```go
func (d *Daemon) systemUIReason(ctx context.Context, viaApp, closing bool, focusCh, windowsCh <-chan readResult) string {
	const unknown = "; ADBuddy does not touch or read the screen without knowing"
	r := d.freshRead(ctx, focusCh, focusCmd)
	focus, ok := uidump.ParseFocus(r.out)
	if r.err != nil || r.code != 0 || !ok {
		return "could not tell whether the notification shade is open" + unknown
	}
	... (igual: closing, IsShade, viaApp)
	r = d.freshRead(ctx, windowsCh, windowsCmd)
	windows, ok := uidump.ParseSystemUIWindows(r.out)
	if r.err != nil || r.code != 0 || !ok {
		return "could not tell whether a system window is on screen" + unknown
	}
	... (igual)
}

// freshRead is the answer of a read started in the background (ch; nil: none
// was started), or a new read made now when it is older than focusSlack: the
// guard never decides on a read that is older than it would have been before
// the reads ran in parallel by more than that.
func (d *Daemon) freshRead(ctx context.Context, ch <-chan readResult, cmd string) readResult {
	slack := focusSlack
	if ch != nil {
		r := <-ch
		if time.Since(r.done) <= slack {
			return r
		}
	}
	out, code, err := d.o.ADB.Shell(ctx, cmd)
	return readResult{out, code, err, time.Now()}
}
```

Propiedades que el diseño conserva (y que los tests comprueban):
1. **Las dos respuestas son obligatorias.** Los dos `<-ch` bloquean hasta que la lectura termina (con `Shell` cancelable por el contexto). Si `inFront` falla, se devuelve la razón de siempre. Si `inFront` va bien y la lectura de `focus` falla o no se entiende, bloquea con «could not tell whether the notification shade is open», igual que hoy. Sin app, `windows` con error o ilegible bloquea. Nada se da por bueno por no haber llegado.
2. **Mismo orden de decisión.** Los paquetes protegidos, ADBuddy delante y `appProtected` se deciden antes de mirar el resultado de `focus`, como hoy. Una razón de protegida gana a una de «no se pudo leer la sombra». `closing`, `wakesScreen && screenOff` y `leaving` no cambian.
3. **Edad acotada.** Una lectura de la sombra o de las ventanas que tenga más de `focusSlack` (150 ms) cuando la guarda la va a usar se **vuelve a hacer**, en serie, como hoy. El caso de la app que no contesta (3 s de `foregroundWait`) o de adb lento queda igual que hoy más `focusSlack`; en el caso normal la lectura más lenta absorbe a la otra y la edad extra es cero.
4. **Llamadas a adb.** `focus` se lee ahora en todos los caminos con el contexto vivo (también cuando bloquea una protegida, ADBuddy delante o `appProtected`, o cuando `inFront` falla), y sin app `windows` se lee siempre: son lecturas de solo lectura, sin efectos, que se cancelan si sobran. Con el contexto ya cancelado no se lanza ninguna.
5. **Nada sobrevive a la guarda.** Las lecturas llevan `rctx`; el `defer` cancela y espera (`wg.Wait`) a que todas las gorrutinas terminen, de modo que ninguna llamada a adb ocurre después de que la guarda vuelva (los tests y el arnés no ven llamadas tardías). `Shell` vuelve al cancelarse (`adb.Runner` mata el proceso). El ejecutor se usa ya desde varias gorrutinas (las órdenes concurrentes de `serve`): `adb.Runner`, `activePhone`, `timedADB` y los falsos de los tests tienen candado.
6. **Una cancelación de `ctx`** (el agente se fue, `serve` para) deja las lecturas con error: bloquea con las mismas razones; `refuse`/`ui()` ya distinguen `ctx.Err()` y lo apuntan como abandono.

### Ruling: la edad de la lectura de la sombra

Hoy `mCurrentFocus` se lee **después** de `inFront`. Con el paralelo se lee a la vez, es decir, hasta `inFront` antes en el peor caso. La ventana entre «la guarda lee» y «la acción actúa» crece para este solo dato (la app comprueba por su cuenta, atómicamente con la acción, lo demás; la sombra, solo el host, y un `dump` la expondría). **Ruling:** paralelo con cota — ahorra el menor de los dos tiempos por orden con pantalla y la cota `focusSlack` devuelve el peor caso al de hoy más 150 ms — cuesta: una ventana de a lo sumo `focusSlack` más de edad para la sombra; si el revisor no la acepta, se baja `focusSlack` a 0 (siempre releer: sin ahorro) o no se integra P4.

## Tareas

### T1: guarda en paralelo (host/internal/daemon/guard.go, guard_parallel_test.go nuevo, waiting_test.go, spec M5 §4.3)

- [ ] **Paso 1: tests que fallan** (`guard_parallel_test.go`). `fakeADB` retiene `f.mu` durante toda la llamada e ignora el contexto, así que estos tests usan un **ejecutor propio** (`gateADB`): las llamadas se apuntan con candado, el punto de encuentro (canales) está **fuera de cualquier candado**, y cada `Shell` observa `ctx.Done()`. Debe ser limpio con `-race` (la CI lo ejecuta): sin variables compartidas sin candado.
  1. `TestGuardReadsRunTogether`: con app, el `foreground` de la UI falsa espera (tope 2 s) a que la lectura de `mCurrentFocus` haya empezado, y esa lectura espera a que haya empezado el `foreground`: solo terminan si corren a la vez (en serie el test falla con «ran in series»). Sin app: `dumpsys activity activities`, `mCurrentFocus` y `dumpsys window windows` empiezan a la vez.
  2. `TestGuardBothAnswersAreRequired`: tabla, con la app en primer plano no protegido: (a) la lectura de focus da error → bloquea con «could not tell whether the notification shade is open»; (b) código de salida ≠ 0; (c) salida que no se entiende; (d) focus bien y la app falla (cae a dumpsys) → pasa como hoy; (e) focus bien y `inFront` falla del todo → «could not tell which app is in the foreground»; (f) los dos mal → bloquea con la razón de `inFront`, como hoy. Sin app: `windows` con error, con código ≠ 0 y no entendida → «could not tell whether a system window is on screen».
  3. `TestGuardOrderOfReasons`: app protegida delante **y** lectura de focus rota → la razón es la de la app protegida; ADBuddy delante y la sombra abierta → la de ADBuddy; la sombra abierta y nada más → la de la sombra; `appProtected` y focus roto → la de pantalla protegida. Con app y protegida delante, `ran()` contiene exactamente la lectura de `focus` (propiedad 4).
  4. `TestGuardStopsAndWaitsForItsReads`: cuando la guarda vuelve sin haber esperado una lectura (app protegida delante, `inFront` falla, contexto cancelado a mitad), el `Shell` falso vio `ctx.Done()` **antes de que `guardForeground` vuelva** (la guarda no vuelve hasta que la gorrutina termina), y después de volver no se apunta ninguna llamada más. Con el contexto ya cancelado al entrar no se lanza ninguna lectura.
  5. `TestGuardRereadsStaleReads` (la cota): la `UI` falsa tarda más que `focusSlack` (variable reducida a milisegundos en el test con `t.Cleanup` que la restaura, leída antes de lanzar gorrutinas); la primera lectura de focus dice «sin sombra» y la segunda `NotificationShade`: bloquea por la sombra y hubo dos lecturas de focus. Y el caso rápido (`inFront` más corto que `focusSlack`): una sola lectura. Lo mismo para `windows` sin app (heads-up que aparece mientras `inFront` tarda).
  6. Sin app, `closing` (`input keyevent 4`) con la lectura de `windows` en error **pasa**, como hoy (esa lectura ni se mira); sin app, con la pantalla apagada y la tecla de despertar y `windows` en error, **pasa**, como hoy.
  Los estados grabados de `uidump/testdata/api34` (`TestTheGuardOnAPI34Screens` y compañeros de `guard_api34_test.go`) deben seguir verdes **sin tocar sus expectativas**: es la prueba de que las decisiones no cambian (no se copia su tabla).
  `waiting_test.go:155` (`TestCommandsAreBlockedWhileAQuestionWaits`) compara la lista ordenada de llamadas y falla de forma intermitente con el paralelo: se mantiene ordenado todo lo que no es de la guarda y el tramo de lecturas de la guarda entre `shell reboot` y `shell input tap 1 1` se compara como **multiconjunto** (lista ordenada alfabéticamente), exigiendo que estén todas antes del tap; ninguna otra expectativa cambia. Si algún otro test (`shade_test.go`, `ui_test.go`) mira `len(ran())` o la última llamada justo tras volver la guarda, debe seguir verde sin cambios (la espera de las gorrutinas lo garantiza); si no, adaptar solo lo que dependa del orden de las lecturas.
- [ ] **Paso 2: implementar** `focusSlack`, `readResult`, `guardReads`, `freshRead`, el cambio de `guardForeground` y de `systemUIReason` tal como arriba (solo `guard.go`). Comentarios de `guardForeground` y `systemUIReason`: las lecturas empiezan antes, se esperan las dos, la cota de edad.
- [ ] **Paso 3: spec.** Spec M5 §4.3: «Coste» (la guarda ya no suma el `dumpsys window` en serie: ahora la más lenta de las dos) y «Límite honesto» (la lectura de la sombra puede tener hasta `focusSlack` más de edad que antes).
- [ ] **Paso 4:** `cd <worktree>/host && go vet ./... && go test ./internal/daemon -timeout 10m -count=1` (varias veces: `-count=5` sobre `Guard|Shade|Waiting|Settle`) y `go test ./... -timeout 15m -count=1`. `git diff --stat` antes de commitear (LF, sin reescribir ficheros).
- [ ] **Paso 5:** commit `perf(daemon): la guarda lee la sombra a la vez que el primer plano, con las dos respuestas obligatorias y la edad acotada`.

### T2: medir (no es código; necesita emulador)

Con el arnés de dispositivo (`TestLatencyDevice`, `internal/emutest/latency_test.go`): antes (`perf/latencia`) y después (esta rama), mismo comando, 25 pasadas en cada una, una rama tras otra. Con las etapas solapadas, la fila «daemon itself» (total menos suma de etapas) deja de significar nada: **se comparan solo los totales** (mediana y rango intercuartil). Se fija antes de medir qué es «más que ruido»: diferencia de medianas mayor que el mayor de los dos rangos intercuartiles; si no, no se integra. El emulador no tiene el salto Wi-Fi y sin app `dumpsys activity` y `dumpsys window` pueden competir por el mismo candado de system_server, así que el ahorro sin app puede ser casi nulo. Además se ejecutan en el emulador `TestTheGuardOnRealScreens`, `TestTheGuardWithAHeadsUp`, `TestTheGuardWithTheCollapsedBubble`, `TestTheGuardWithTheExpandedBubble` y `TestTheFallbackWithoutTheApp` (en `guard_test.go`): son la única prueba de las decisiones con adb real concurrente. Resultado en `2026-10-03-latencia.md`.

## Revisión de seguridad que se pide a opus (diff)

1. ¿Las dos respuestas siguen siendo obligatorias en todos los caminos (incluidos `windows` y `screenOff`)? ¿Hay algún camino en que una lectura que no llegó, llegó tarde, estaba vieja o se canceló se dé por buena?
2. ¿El orden de decisión y las razones son idénticos a los de hoy?
3. La cota de edad (`focusSlack`): ¿bien aplicada a `focus` y a `windows`? ¿Algún otro dato cuya edad crezca?
4. Gorrutinas: sin fugas, la guarda no vuelve con lecturas vivas, sin condiciones de carrera (sin `-race` local: leer los valores sustituibles por tests antes de lanzar gorrutinas), ejecutores seguros.
5. `settle()` y el resto de llamadores de `guardForeground` no cambian de comportamiento.
