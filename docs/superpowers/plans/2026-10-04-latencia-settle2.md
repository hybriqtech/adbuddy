# Latencia: la espera de `--screen` con pantalla de partida (settle2)

> Para quien ejecute: `superpowers:subagent-driven-development`. Rama `perf/latencia`; worktree `C:/Users/Jose/orca/workspaces/adbuddy/latencia` (el checkout principal `C:/Users/Jose/Documents/projects/adbuddy` NO se toca: rutas absolutas y `cd <worktree> && …` en el mismo comando; `git -C <worktree> …`; sin worktrees nuevos; sin push). Commits con identidad hybriqtech y el trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. `go test -race` no corre en este PC (la CI sí): leer valores sustituibles por tests antes de lanzar gorrutinas; `settle` no lanza gorrutinas. Nunca modelos Haiku. Medidas que lo motivan: `2026-10-03-latencia.md`, «Transiciones reales».

## Por qué

Las transiciones reales (emulador, `TestLatencySettle`) enseñan dos defectos de `settle` (T3 de `2026-10-03-latencia-impl.md`):

1. `settleMin` = 400 ms: dos lecturas iguales de la pantalla **vieja** (a los 150 y 270 ms) cuentan como asentada si ya han pasado 400 ms, pero el primer cambio llega a 520-1100 ms (máx. 1,7 s). El agente vería `Settled` con la pantalla anterior, creería que el tap no hizo nada y podría repetirlo (acción doble).
2. Durante una transición la app dice «protegida» (una ventana sin contenido): tras Atrás en el 39 % de las lecturas. Una lectura de `--screen` justo después sale como bloqueo espurio.

## Diseño

**Pantalla de partida.** Antes de una acción con `Screen` (click, setText) se lee el árbol una vez (`base`): la lectura ya pasó la guarda de la propia acción. Para `screen --settle`, la partida es la primera lectura (`first`). Con `base`, una pantalla solo se da por asentada si **cambió** respecto a ella y **se mantiene** (`settleQuiet`); si nunca cambia, se espera `settleIdle` y se devuelve diciendo que no cambió. Sin `base` (la lectura de partida falló), se exige además `settleIdle` desde el inicio: nunca se da por buena una pantalla sin saber si es la vieja.

**Reintento de las lecturas bloqueadas por la guarda.** Si `guardForeground` dice no durante la espera, no se lee y se vuelve a intentar tras `settleGap`, **hasta `settleBlockWait`** desde el inicio; después, el bloqueo se devuelve como hoy (nota o `UI_BLOCKED`). Cada intento pasa por la guarda completa y solo se devuelve una lectura hecha **después** de pasarla. Pausa y pregunta 🔴 en espera **no** se reintentan: terminan al instante.

Valores (variables de paquete, para los tests): `settleFirst` 150 ms (tras una acción, antes de la primera lectura), `settleGap` 120 ms, `settleQuiet` 250 ms (la pantalla igual durante al menos esto), `settleIdle` 2 s (sin cambio alguno), `settleBlockWait` 2 s, `settleMax` 3 s, `settleReads` 40 (intentos, lecturas o bloqueos). Desaparece `settleMin`.

Código (`host/internal/daemon/settle.go`; sustituye `settle`, `ScreenSettled` gana `Changed`):

```go
// ScreenSettled is dump's data when the client asked to wait for the screen
// to settle. Settled is false when the screen was still changing at the end
// of the wait (an animation, a video). Changed says the screen is not the one
// it started from: with Settled and not Changed, nothing happened on screen.
type ScreenSettled struct {
	bridge.UIDump
	Settled bool `json:"settled"`
	Changed bool `json:"changed"`
}

// settle reads the screen until it has changed from base and stayed the same
// for settleQuiet, or, if it never changes, until settleIdle has passed; at
// most until start+settleMax and settleReads attempts. first is a read
// already made (nil after an act: the first read waits settleFirst). base is
// the screen the wait started from (nil: unknown, the read of it failed: then
// only settleIdle ends the wait, never a read that may be the old screen).
// Every read goes through the doors and the guard like any other read of the
// screen: paused, a 🔴 question waiting (they end the wait at once), and
// guardForeground, which may be showing a transition (a window without
// content counts as protected): a block is tried again after settleGap until
// settleBlockWait; if it lasts, there is no screen and why says it. why and f
// are both empty when the context ended (the agent or serve went away). It
// fails closed: only a read made after the guard said yes is returned. via
// says how the last read was made.
func (d *Daemon) settle(ctx context.Context, args json.RawMessage, base, first *bridge.UIDump, start time.Time) (screen *ScreenSettled, via, why string, f *uiFailure) {
	firstWait, gap, quiet, idle, blockWait, max, maxTries := settleFirst, settleGap, settleQuiet, settleIdle, settleBlockWait, settleMax, settleReads
	cur := first // the last read that passed the guard
	deadline := start.Add(max)
	changed := false
	lastMove := start // when a read last differed from the one before it (or from base)
	if first != nil && base != nil && !reflect.DeepEqual(*base, *first) {
		changed = true
	}
	blocked := ""
	for tries := 0; ; tries++ {
		wait := gap
		if cur == nil && tries == 0 {
			wait = firstWait
		}
		// The cap: no new attempt starts past the deadline or the tries; what
		// the last read said (it passed the guard) is what comes out.
		if tries >= maxTries || time.Now().Add(wait).After(deadline) {
			switch {
			case blocked != "": // the last attempt was a block: nothing read before is returned
				return nil, "", blocked, nil
			case cur != nil:
				return &ScreenSettled{UIDump: *cur, Settled: false, Changed: changed}, via, "", nil
			}
			// Nothing was read and nothing was blocked (a deadline shorter than
			// the first wait): read once.
		}
		if !sleepCtx(ctx, wait) {
			return nil, "", "", nil
		}
		if d.o.Paused() {
			return nil, "", PausedMessage, nil
		}
		release, other := d.admit("Read the screen", false)
		if other != "" {
			return nil, "", other, nil
		}
		release()
		if why := d.guardForeground(ctx, nil); why != "" {
			if ctx.Err() != nil {
				return nil, "", "", nil
			}
			blocked = why
			if time.Since(start) >= blockWait {
				return nil, "", blocked, nil // the block lasts: no screen
			}
			continue // maybe a transition: try again
		}
		blocked = ""
		if d.o.Paused() { // paused while the guard looked
			return nil, "", PausedMessage, nil
		}
		v, how, uf := d.runUI(ctx, bridge.UIOpDump, args, nil)
		if uf != nil {
			if ctx.Err() != nil { // the agent or serve went away during the read
				return nil, "", "", nil
			}
			return nil, "", "", uf
		}
		read, ok := asDump(v)
		if !ok {
			return nil, "", "", fail(1, "UI_FAILED", "the ADBuddy app answered something unreadable")
		}
		via = how
		now := time.Now()
		if cur != nil && !reflect.DeepEqual(*cur, read) {
			lastMove = now
		}
		if base != nil && !reflect.DeepEqual(*base, read) {
			if !changed {
				lastMove = now // the first change counts as a move
			}
			changed = true
		}
		cur = &read
		still := now.Sub(lastMove) >= quiet
		switch {
		case changed && still: // it moved and has been quiet
			return &ScreenSettled{UIDump: read, Settled: true, Changed: true}, via, "", nil
		case !changed && still && now.Sub(start) >= idle: // it never moved
			return &ScreenSettled{UIDump: read, Settled: true, Changed: false}, via, "", nil
		}
	}
}
```

Propiedades: (1) cada lectura devuelta pasó antes `paused`, `admit` y `guardForeground`; (2) un bloqueo largo nunca devuelve una lectura anterior al bloqueo como si fuese la pantalla (la salida es la **última** lectura hecha tras pasar la guarda; en `screen --settle`, si la guarda bloquea hasta `settleBlockWait`, la salida es el bloqueo y **ningún dato**: `cur` no se devuelve); (3) `Settled` con `Changed` falso significa «no cambió» y se dice; (4) sin `base` solo `settleIdle` cierra; (5) sin gorrutinas; acotado por `settleMax`/`settleReads`.

### `ui()` (host/internal/daemon/ui.go)

- Para click y setText con `req.Screen`, **tras** pasar la guarda y la pausa y **antes** de la entrada de diario `running` y de la acción: `base := d.baselineScreen(ctx)`:

```go
// baselineScreen reads the screen the wait of --screen starts from: the one
// before the act. nil when it cannot be read (then settle waits settleIdle).
func (d *Daemon) baselineScreen(ctx context.Context) *bridge.UIDump {
	v, _, uf := d.runUI(ctx, bridge.UIOpDump, json.RawMessage("{}"), nil)
	if uf != nil {
		return nil
	}
	if dump, ok := asDump(v); ok {
		return &dump
	}
	return nil
}
```

- `withScreen(ctx, req, data, base)` pasa `base` a `settle(ctx, "{}", base, nil, time.Now())`; la entrada de diario de la lectura dice `changed=` además de `settled=`.
- `screen --settle` (flujo dump): `d.settle(ctx, req.UIArgs, &first, &first, time.Now())` (la partida es la primera lectura).

### CLI (host/internal/cli/helpers.go)

`tap --screen`/`type --screen`: si hay pantalla y `Settled` es true y `Changed` es false (decodificado como `*bool`: ausente = daemon antiguo = no decir nada), imprimir tras la pantalla `(the screen did not change after the action)`. `screen --settle`: solo el aviso de `Settled` falso, como hoy. `--json` incluye `changed`.

## Tareas

### S1: daemon (settle.go, ui.go, settle_test.go) y CLI (helpers.go, helpers_test.go), spec y docs

- [ ] **Paso 1: tests** (los de `settle_test.go` se adaptan a `settleQuiet`/`settleIdle`/`settleBlockWait`, sin `settleMin`; el fake de la app que devuelve una lista de dumps y la secuencia de operaciones se conserva; valores reducidos a milisegundos con `t.Cleanup` que los restaura, leídos antes de lanzar gorrutinas):
  1. `TestSettleNeedsAChange`: partida A, lecturas A, A, A, … (sin cambio): no se da por asentada hasta `settleIdle`; sale `Settled` true, `Changed` false. Con partida A y lecturas A, A, B, B, B: sale B, `Changed` true, en la tercera lectura igual (no en la primera pareja A, A): **la pantalla vieja nunca sale como asentada cuando hay cambio posterior dentro de `settleIdle`**.
  2. `TestSettleQuietAfterTheChange`: A, B, C, C, C: no se da por asentada en B ni en la primera C hasta que `settleQuiet` se cumple sobre C.
  3. `TestSettleWithoutBaseWaitsIdle`: la lectura de partida falla (`base` nil): dos lecturas iguales antes de `settleIdle` no cierran; después de `settleIdle` sí.
  4. `TestSettleRetriesATransientBlock`: `foreground` devuelve protegida las dos primeras veces y luego normal: sale la pantalla sin nota, no hubo ningún `dump` durante el bloqueo, y el orden es `foreground (bloqueado), foreground (bloqueado), foreground, dump …`; `settleBlockWait` largo.
  5. `TestSettleBlockThatLastsGivesNoScreen`: `foreground` protegida siempre: tras `settleBlockWait` sale la nota «The action was done; do not repeat it… Blocked by ADBuddy: …» (tap) o `UI_BLOCKED` sin datos (`screen --settle`), y **ningún dump** tras el primer bloqueo; con una lectura previa en `screen --settle` (la primera) **no** se devuelve esa lectura al llegar el tope si el último intento fue un bloqueo.
  6. `TestSettlePausedAndQuestionAreNotRetried`: pausa y pregunta 🔴 terminan al instante (sin esperar `settleBlockWait`).
  7. Los tests existentes de T3 (guarda por lectura, pausa, abandono, tipo `type --screen`, sin app, cap) siguen válidos con la nueva firma; `TestSettleMinimumTime` desaparece, sustituido por el 1; `TestNoScreenWithoutTheFlag` y `TestAgentsTemplateQuotesNotDone` siguen verdes.
  8. `baselineScreen`: con `Screen`, el daemon hace **una lectura de partida antes de la acción** y la acción ocurre una vez (la secuencia `foreground, dump (partida), click, …`); si el dump de partida falla (la app dice `CodeProtected`, timeout), la acción **sigue** su curso y `base` es nil (la acción ya pasó su guarda).
  9. CLI: `tap --screen` con `settled`+`changed:false` imprime `(the screen did not change after the action)`; con `changed:true` no; con `changed` ausente (daemon antiguo) no; `--json` conserva `changed`.
- [ ] **Paso 2: implementar** `settle.go` (con la corrección obligatoria del cap), `baselineScreen`, el cambio de `ui()` y de `withScreen`, y el CLI.
- [ ] **Paso 3: docs y spec.** `docs/usage.md` y la ayuda de `--screen`/`--settle` (en y es: «unos 3 s» en vez de «unos 2 s»; regenerar goldens con `go test ./internal/cli -run Help -update-help`); spec M5 §3: la espera, la partida, el reintento de bloqueos transitorios y `changed`; `AGENTS.md`: una frase («`(the screen did not change after the action)` means the tap did nothing visible: look before you try again»; sin tocar las demás) y el test de plantillas.
- [ ] **Paso 4:** `cd <worktree>/host && go vet ./... && go test ./internal/daemon ./internal/cli ./internal/workspace ./internal/i18n -timeout 15m -count=1`; `go test ./internal/daemon -run 'Settle|Screen' -count=10`; `go test ./... -timeout 15m -count=1`. `git diff --stat` antes de commitear (LF).
- [ ] **Paso 5:** commit `fix(daemon): --screen espera a que la pantalla cambie desde la de partida y reintenta los bloqueos transitorios de la guarda`.

### S2: validar en el emulador (no es código)

`TestLatencySettle` ya mide la app; añadir a `emutest` una prueba (no medidora, con emulador, sin `ADBUDDY_LATENCY`) del daemon con la app real: (a) `tap --screen` a «Network & internet» devuelve la pantalla nueva con `Changed` true (no la de Ajustes); (b) Atrás (`input keyevent 4` y luego `screen --settle`) no sale bloqueado; (c) abrir desde el launcher una app que el host marca protegida (`Options.Protected`): la nota de bloqueo y el tap una sola vez; (d) un tap sin efecto devuelve `Changed` false tras `settleIdle`. Apuntar los tiempos reales (de la acción a la salida) en `2026-10-03-latencia.md`.

## Revisión de seguridad que se pide a opus

1. Cada lectura devuelta pasó antes `paused`, `admit` y `guardForeground`; el reintento de un bloqueo no devuelve nunca una lectura anterior al bloqueo cuando el último intento fue un bloqueo; pausa y pregunta no se reintentan.
2. Una pantalla de partida mala o ausente nunca produce «asentada» con la pantalla vieja (`base` nil → solo `settleIdle`).
3. El reintento de bloqueos (hasta `settleBlockWait`): ¿abre algún hueco (leer pantalla protegida)? ¿Cambia lo que ve el agente o el tiempo de un bloqueo real (apps protegidas, notificación 8 s)?
4. La lectura de partida: ¿pasó la guarda (la de la acción)? ¿Se lee algo que la guarda de la acción no cubriese (otra ventana, ya cambiada)? ¿La acción puede ocurrir sin que la lectura de partida pase por `paused`/`admit` (no es una lectura devuelta: solo se compara)? El contenido de partida nunca se devuelve ni se escribe.
5. Tiempo acotado y cancelación; sin gorrutinas; compatibilidad con daemon o CLI antiguos.
