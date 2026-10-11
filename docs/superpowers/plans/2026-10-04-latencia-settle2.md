# Latencia: la espera de `--screen` con pantalla de partida (settle2)

> Para quien ejecute: `superpowers:subagent-driven-development`. Rama `perf/latencia`; worktree `<worktree>` (el checkout principal `<checkout principal>` NO se toca: rutas absolutas y `cd <worktree> && …` en el mismo comando; `git -C <worktree> …`; sin worktrees nuevos; sin push). Commits con identidad hybriqtech y el trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. `go test -race` no corre en este PC (la CI sí): leer valores sustituibles por tests antes de lanzar gorrutinas; `settle` no lanza gorrutinas. Nunca modelos Haiku. Medidas que lo motivan: `2026-10-03-latencia.md`, «Transiciones reales». Revisión opus del plan hecha (no conforme a la primera: 2 críticos, 5 a corregir, 5 menores): este texto los incorpora.

## Por qué

Las transiciones reales (emulador, `TestLatencySettle`) enseñan dos defectos de `settle` (T3 de `2026-10-03-latencia-impl.md`):

1. `settleMin` = 400 ms: dos lecturas iguales de la pantalla **vieja** (a los 150 y 270 ms) cuentan como asentada si ya han pasado 400 ms, pero el primer cambio llega a 520-1100 ms (máx. 1,7 s). El agente vería `Settled` con la pantalla anterior, creería que el tap no hizo nada y podría repetirlo (acción doble).
2. Durante una transición la app dice «protegida» (una ventana sin contenido): tras Atrás en el 39 % de las lecturas. Una lectura de `--screen` justo después sale como bloqueo espurio.

## Diseño

**Pantalla de partida (`base`).** Antes de una acción con `Screen` (click, setText) se lee el árbol una vez, y esa lectura **no se devuelve, no se escribe y no se registra** (solo se compara). Para `screen --settle` la partida es la primera lectura. Con `base`, una pantalla solo se da por asentada si **cambió** respecto a ella (con una firma estable, ver `sameScreen`) y **se mantiene** `settleQuiet`; si nunca cambia, se espera `settleIdle` y se devuelve diciendo que no cambió. Sin `base` (la lectura de partida falló por tiempo o no hay app), **no se sabe** si cambió: solo `settleIdle` cierra la espera y la salida lleva `Changed` nulo (desconocido), nunca «no cambió».

**La lectura de partida no debilita las puertas de la acción.** Va con tope corto (`baselineWait`, 1 s; si vence, `base` nil, sin caer a `uiautomator`); tras ella, la pausa y el contexto se miran otra vez antes de actuar (si ya no, no hay acción); y si pasó más de `baselineSlack` (150 ms) desde que la guarda de la acción dijo sí, la guarda se pasa **de nuevo** antes de actuar (cualquier no la bloquea: la acción no ocurre). Si el dump de partida falla con `ExitDenied` (la app dice protegida: notificación o app protegida delante), se bloquea con ese motivo **antes** de la acción: no ocurrió, así que no hay riesgo de acción doble. Con cualquier otro fallo (tiempo, sin app, error) la acción sigue y `base` es nil.

**Reintento de las lecturas bloqueadas por la guarda.** Si `guardForeground` dice no durante la espera, no se lee y se vuelve a intentar tras `settleGap`, **hasta `settleBlockWait`** desde el inicio; después se devuelve el bloqueo. Lo mismo para un `UI_PROTECTED` (ExitDenied) del propio dump. Cada intento pasa la guarda completa y solo se devuelve una lectura hecha **después** de pasarla; si el último intento fue un bloqueo, no se devuelve nada leído antes. Pausa y pregunta 🔴 en espera **no** se reintentan. La primera guarda de `screen --settle` (la del propio helper en `ui()`) también reintenta: si bloquea por la guarda (no pausa ni pregunta) entra en `settle` sin lectura previa y **la primera lectura válida es la partida**.

**Qué cuenta como «cambió» (`sameScreen`).** Comparar paquete, actividad, `Locked` y los elementos por (id, texto, desc, hint, clase, límites, click, edit, scroll, checked, enabled, password, window), **sin `Focused`** y tratando `Elements` nil como vacío; así un foco, un `Checked` del elemento pulsado o una diferencia entre rutas (app y adb) no cuenta como la transición. (Un `Checked` que cambia tras un tap sí es un cambio real: está en la firma.)

Valores (variables de paquete, para los tests): `settleFirst` 150 ms (tras una acción, antes de la primera lectura), `settleGap` 120 ms, `settleQuiet` 600 ms (la pantalla igual durante al menos esto, tras haber cambiado; calibrado con el mayor hueco estable dentro de una transición: máx. 495-512 ms en el emulador, ver `2026-10-03-latencia.md`; era 350 ms), `settleIdle` 2 s (sin cambio alguno), `settleBlockWait` 2 s, `settleMax` 3 s, `settleReads` 40 (intentos: lecturas o bloqueos), `baselineWait` 1 s, `baselineSlack` 150 ms. Desaparece `settleMin`.

Código (`host/internal/daemon/settle.go`; sustituye `settle` y `ScreenSettled`):

```go
// ScreenSettled is dump's data when the client asked to wait for the screen
// to settle. Settled is false when the screen was still changing at the end
// of the wait (an animation, a video). Changed says whether the screen is not
// the one it started from: false with Settled means nothing happened on
// screen; nil means it is not known (the start could not be read).
type ScreenSettled struct {
	bridge.UIDump
	Settled bool  `json:"settled"`
	Changed *bool `json:"changed,omitempty"`
}

// sameScreen reports whether two reads show the same screen: everything but
// which element has the focus, with no elements and none being the same.
func sameScreen(a, b bridge.UIDump) bool {
	a.Elements, b.Elements = noFocus(a.Elements), noFocus(b.Elements)
	return reflect.DeepEqual(a, b)
}

// noFocus is es without the focus flag; nil and empty are the same.
func noFocus(es []bridge.UIElement) []bridge.UIElement {
	out := make([]bridge.UIElement, len(es))
	for i, e := range es {
		e.Focused = false
		out[i] = e
	}
	return out
}

// settleIn is what settle starts from.
type settleIn struct {
	args  json.RawMessage
	base  *bridge.UIDump // the screen the wait starts from; nil: unknown
	first *bridge.UIDump // a read already made (and guarded); nil: none
	via   string         // how first was read
	start time.Time
	adopt bool // base is nil: the first valid read becomes it
}

// settle reads the screen until it has changed from base and stayed the same
// for settleQuiet, or, if it never changes, until settleIdle has passed; at
// most until start+settleMax and settleReads attempts. ... (la documentación
// larga de arriba se conserva aquí: puertas, bloqueos, qué sale y qué no).
func (d *Daemon) settle(ctx context.Context, in settleIn) (screen *ScreenSettled, via, why string, f *uiFailure) {
	firstWait, gap, quiet, idle, blockWait, max, maxTries := settleFirst, settleGap, settleQuiet, settleIdle, settleBlockWait, settleMax, settleReads
	base, cur, via := in.base, in.first, in.via
	deadline := in.start.Add(max)
	changed := false
	lastMove := in.start // when the screen last moved (a read differed from the one before, or from base)
	blocked := ""
	known := func() *bool { // Changed for the result: nil when there is no base
		if base == nil {
			return nil
		}
		c := changed
		return &c
	}
	if in.first != nil && base != nil && !sameScreen(*base, *in.first) {
		changed = true
	}
	for tries := 0; ; tries++ {
		wait := gap
		if cur == nil && tries == 0 {
			wait = firstWait
		}
		// The cap: no new attempt starts past the deadline or the tries. If the
		// last attempt was a block nothing read before is returned; else what the
		// last read said (it passed the guard) is what comes out. With nothing
		// read and nothing blocked (a deadline shorter than the first wait), read once.
		if tries >= maxTries || time.Now().Add(wait).After(deadline) {
			switch {
			case blocked != "":
				return nil, "", blocked, nil
			case cur != nil:
				return &ScreenSettled{UIDump: *cur, Settled: false, Changed: known()}, via, "", nil
			}
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
			if time.Since(in.start) >= blockWait {
				return nil, "", blocked, nil // the block lasts: no screen
			}
			continue // maybe a transition: try again
		}
		blocked = ""
		if d.o.Paused() { // paused while the guard looked
			return nil, "", PausedMessage, nil
		}
		v, how, uf := d.runUI(ctx, bridge.UIOpDump, in.args, nil)
		if uf != nil {
			if ctx.Err() != nil { // the agent or serve went away during the read
				return nil, "", "", nil
			}
			if uf.exit == ExitDenied { // the app's own check: a transition or a protected screen
				blocked = uf.msg
				if time.Since(in.start) >= blockWait {
					return nil, "", blocked, nil
				}
				continue
			}
			return nil, "", "", uf
		}
		blocked = ""
		read, ok := asDump(v)
		if !ok {
			return nil, "", "", fail(1, "UI_FAILED", "the ADBuddy app answered something unreadable")
		}
		via = how
		now := time.Now()
		if base == nil && in.adopt {
			base = &read // the first valid read is where it started
			cur = &read
			lastMove = now
			continue
		}
		if cur != nil && !sameScreen(*cur, read) {
			lastMove = now
		}
		if base != nil && !sameScreen(*base, read) {
			if !changed {
				lastMove = now // the first change counts as a move
			}
			changed = true
		}
		cur = &read
		still := now.Sub(lastMove) >= quiet
		switch {
		case changed && still: // it moved and has been quiet
			return &ScreenSettled{UIDump: read, Settled: true, Changed: known()}, via, "", nil
		case still && now.Sub(in.start) >= idle: // it never moved (or the start is unknown): the idle wait is over
			return &ScreenSettled{UIDump: read, Settled: true, Changed: known()}, via, "", nil
		}
	}
}
```

Propiedades: (1) cada lectura devuelta pasó antes `paused`, `admit` y `guardForeground`; (2) si el último intento fue un bloqueo (de la guarda o de la propia app), no se devuelve nada leído antes; (3) `Settled` con `Changed` falso significa «no cambió» (y el CLI lo dice); `Changed` nil = desconocido (no se dice «no cambió»); (4) sin `base` solo `settleIdle` cierra; (5) sin gorrutinas; acotado por `settleMax`/`settleReads`; el `continue` pasa siempre por `tries++` y `sleepCtx`, así que el reloj avanza.

### `ui()` (host/internal/daemon/ui.go)

- **Acción con `Screen` (click, setText)**, tras pasar la guarda y la pausa y **antes** de la entrada de diario `running` y de la acción:

```go
	var base *bridge.UIDump
	if req.Screen && (req.UIOp == bridge.UIOpClick || req.UIOp == bridge.UIOpSetText) {
		guarded := time.Now() // right after the guard said yes
		var denied *uiFailure
		base, denied = d.baselineScreen(ctx)
		if ctx.Err() != nil { // the agent or serve went away: no action
			... (el mismo bloque «abandoned» de ui())
		}
		if d.o.Paused() { // paused while the start was read: no action
			return d.blockUI(e, PausedMessage)
		}
		if denied != nil { // the app says there is something protected in front: no action
			return d.blockUI(e, denied.msg)
		}
		if time.Since(guarded) > baselineSlack { // the guard looked too long ago: look again before acting
			if why := d.guardForeground(ctx, nil); why != "" {
				if ctx.Err() != nil { ...abandoned... }
				return d.blockUI(e, why)
			}
			if d.o.Paused() {
				return d.blockUI(e, PausedMessage)
			}
		}
	}
```

con

```go
// baselineScreen reads the screen the wait of --screen starts from: the one
// before the act, with a short limit. nil when it cannot be read (then settle
// waits settleIdle and says nothing about a change); denied is set when the
// app itself says something protected is in front.
func (d *Daemon) baselineScreen(ctx context.Context) (*bridge.UIDump, *uiFailure) {
	bctx, cancel := context.WithTimeout(ctx, baselineWait)
	defer cancel()
	v, _, uf := d.runUI(bctx, bridge.UIOpDump, json.RawMessage("{}"), nil)
	if uf != nil {
		if uf.exit == ExitDenied {
			return nil, uf
		}
		return nil, nil
	}
	if dump, ok := asDump(v); ok {
		return &dump, nil
	}
	return nil, nil
}
```

(`runUI` con el contexto vencido no cae a `uiautomator` porque `fallsBack` mira el error de la app, no el contexto; el implementador comprueba que con `bctx` vencido `runUI` devuelve un fallo y no lanza `uiautomator`; si lanzara, `baselineScreen` debe pasar un flag para no caer a adb.)
- `withScreen(ctx, req, data, base)` llama a `d.settle(ctx, settleIn{args: "{}", base: base, start: time.Now()})`; la entrada de diario de la lectura dice `changed=` (`true`, `false` o `unknown`) además de `settled=`.
- **`screen --settle`** (flujo dump): con la primera lectura ya hecha: `d.settle(ctx, settleIn{args: req.UIArgs, base: &first, first: &first, via: via, start: time.Now()})`. Si en cambio la guarda de `ui()` bloquea por la guarda (no por pausa ni pregunta) y `req.Screen`: se entra en `settle` con `settleIn{args: req.UIArgs, start: time.Now(), adopt: true}` en vez de `blockUI`; los resultados (`why`, `f`, abandono, datos) se tratan como en el bloque actual de `ui()`; refactorizar ese bloque en una función que sirva a los dos caminos.
- Un `Settled` verdadero con `Changed` falso en `screen --settle` no se avisa (es lo normal de una pantalla quieta).

### CLI (host/internal/cli/helpers.go)

Decodificar `changed` como `*bool`. `tap --screen`/`type --screen`: si hay pantalla, `Settled` es true y `Changed` no es nil y es false: imprimir tras la pantalla `(the screen did not change in about 2 s after the action: run adbuddy screen before trying again)`. Nada si `Changed` es nil o true. `screen --settle`: solo el aviso de `Settled` falso, como hoy. `--json` conserva `changed` solo cuando el daemon lo mandó (reconstruir con el puntero, nunca inventar `false`).

## Tareas

### S1: daemon (settle.go, ui.go, settle_test.go) y CLI (helpers.go, helpers_test.go), spec y docs

- [ ] **Paso 1: tests.** Los de `settle_test.go` se adaptan (no se debilita ninguna aserción de seguridad: «ningún dump tras el bloqueo», «la primera lectura nunca sale» siguen). La lectura de partida desplaza la numeración de `dumpFn(n)` y los recuentos de dumps: los tests cuentan **sobre lo ocurrido después del click** (`afterClick()`), no sobre el total; los que bloquean de inmediato fijan `settleBlockWait` y `baselineWait` a milisegundos; el helper de tiempos cambia de firma. Se retocan al menos `TestSettleStopsWhenTwoReadsMatch`, `TestSettleCapStopsNewReads`, `TestSettleGuardBlocksMidwayInATap`, `TestSettleEveryReadIsGuarded`, `TestSettlePausedAndQuestionStopTheReads`, `TestSettleGivesUpAtTheCap`. `TestSettleMinimumTime` desaparece. Tests nuevos (valores reducidos a milisegundos con `t.Cleanup` que los restaura, leídos antes de lanzar gorrutinas):
  1. `TestSettleNeedsAChange`: partida A, lecturas A, A, A, … sin cambio: no se da por asentada hasta `settleIdle`; sale `Settled` true y `Changed` false. Partida A y lecturas A, A, B, B, B: sale B con `Changed` true, **nunca** A, aunque haya dos A seguidas y haya pasado más tiempo que `settleQuiet`.
  2. `TestSettleQuietAfterTheChange`: A, B, C, C, C: no se da por asentada en B ni en la primera C hasta que `settleQuiet` se cumple sobre C.
  3. `TestSettleIgnoresFocusOnly`: A, A' (solo cambia `Focused`), A', …, B: A' no cuenta como cambio; Elements nil frente a vacío tampoco.
  4. `TestSettleWithoutBase`: la lectura de partida falla por tiempo (la app no contesta): la acción ocurre una vez, dos lecturas iguales antes de `settleIdle` no cierran, después sí, y `Changed` es **nil** (con `--json` no aparece `changed`; el CLI no dice «no cambió»).
  5. `TestBaselineDeniedBlocksTheAction`: el dump de partida contesta `CodeProtected`: `UI_BLOCKED`, **ningún click**, diario `blocked`.
  6. `TestBaselineKeepsTheDoors`: pausa durante el dump de partida → no hay click y sale la pausa; contexto cancelado durante la partida → no hay click, diario `abandoned`; la guarda dice protegida en su segunda consulta (la partida tardó más de `baselineSlack`) → no hay click; si la partida fue rápida, la guarda no se repite.
  7. `TestSettleRetriesATransientBlock`: `foreground` devuelve protegida las dos primeras veces tras el click y luego normal: sale la pantalla sin nota, ningún `dump` durante el bloqueo; lo mismo con un `UI_PROTECTED` del propio dump. `screen --settle` con la guarda del helper bloqueando las dos primeras consultas: sale la pantalla (la primera lectura válida es la partida) y ningún dump durante el bloqueo.
  8. `TestSettleBlockThatLastsGivesNoScreen`: protegida siempre: tras `settleBlockWait` sale la nota (tap) o `UI_BLOCKED` sin datos (`screen --settle`), ningún dump tras el primer bloqueo; **con una lectura previa en `screen --settle` no se devuelve esa lectura si el último intento fue un bloqueo** (también al llegar `settleMax` o `settleReads`: un test de `settleReads` alcanzado solo con bloqueos).
  9. `TestSettlePausedAndQuestionAreNotRetried`: terminan al instante (sin esperar `settleBlockWait`).
  10. Siguen verdes `TestNoScreenWithoutTheFlag`, `TestAgentsTemplateQuotesNotDone`, la tabla de variantes de `TestSettleEveryReadIsGuarded` (ADBuddy delante, sombra, `Protected`, `CodeProtected` del dump) y los de `type --screen`.
  11. CLI (`helpers_test.go`): `tap --screen` con `settled` true y `changed` false imprime el aviso; con `changed` true no; con `changed` ausente (daemon antiguo o partida desconocida) no; `--json` conserva `changed` cuando viene y no lo inventa cuando falta (también en `screen --settle --json`).
- [ ] **Paso 2: implementar** `settle.go` (con `settleIn`, `sameScreen`, `Changed *bool`), `baselineScreen`, el cambio de `ui()` (partida con sus puertas, `screen --settle` con reintento de la primera guarda) y de `withScreen` (pasa `base`; diario con `changed=`), y el CLI. El `via` de un tope sin lecturas nuevas viene de `settleIn.via`.
- [ ] **Paso 3: docs y spec.** `docs/usage.md` y la ayuda de `--screen`/`--settle` (en y es: «hasta unos 3 s» en vez de «unos 2 s»; un `tap` sin efecto tarda ~2 s y un bloqueo real avisa a los ~2 s en vez de al instante; regenerar goldens con `go test ./internal/cli -run Help -update-help`); spec M5 §3: la espera, la partida (una lectura interna que no se devuelve, no se escribe y no se registra), el reintento de bloqueos transitorios, `changed`; `AGENTS.md`: una frase («`(the screen did not change in about 2 s after the action …)` means the tap did nothing visible: look before you try again»; sin tocar las demás) y el test de plantillas.
- [ ] **Paso 4:** `cd <worktree>/host && go vet ./... && go test ./internal/daemon ./internal/cli ./internal/workspace ./internal/i18n -timeout 15m -count=1`; `go test ./internal/daemon -run 'Settle|Screen|Baseline' -count=10`; `go test ./... -timeout 15m -count=1`. `git diff --stat` antes de commitear (LF).
- [ ] **Paso 5:** commit `fix(daemon): --screen espera a que la pantalla cambie desde la de partida y reintenta los bloqueos transitorios de la guarda`.

### S2: validar en el emulador (no es código del producto)

Reglas del emulador propio: cupo de `emu-lock.sh`, consola 5602, servidor adb 5072, uno a la vez, apagarlo al terminar con `scripts/emulator.sh stop` y comprobar con `Get-CimInstance` (nunca matar por nombre). Prueba de `emutest` (con emulador, sin `ADBUDDY_LATENCY`; no cuenta como medidora): el daemon con la app real y `Options.Protected` a la medida: (a) `tap --screen` a «Network & internet» devuelve la pantalla nueva con `Changed` true (no la de Ajustes), **un solo `click` en el diario**; (b) Atrás (`input keyevent 4` y luego `screen --settle`), **20 repeticiones**: ninguna sale bloqueada; (c) abrir desde el launcher una app que el host marca protegida: la nota de bloqueo y el tap una sola vez; (d) un tap sin efecto devuelve `Changed` false tras `settleIdle`. Y `TestLatencySettle` se amplía para medir el **mayor hueco estable** dentro de cada transición (entre el primer y el último cambio) y calibrar `settleQuiet`. Apuntar tiempos reales (de la acción a la salida) en `2026-10-03-latencia.md`.

## Revisión de seguridad que se pide a opus (diff)

1. Cada lectura devuelta pasó antes `paused`, `admit` y `guardForeground`; el reintento de un bloqueo no devuelve nunca una lectura anterior al bloqueo cuando el último intento fue un bloqueo; pausa y pregunta no se reintentan.
2. Una partida mala o ausente nunca produce «asentada» con la pantalla vieja ni «no cambió»; `Changed` nil es desconocido.
3. La lectura de partida: no se devuelve ni se escribe; la acción conserva paused, contexto y guarda (repetida si pasó `baselineSlack`); `ExitDenied` en la partida bloquea antes de actuar.
4. El reintento de bloqueos (hasta `settleBlockWait`): ¿hueco?; ¿lo que ve el agente cambia?; coste de un bloqueo real (+2 s).
5. `sameScreen` no oculta un cambio real; tiempo acotado y cancelación; sin gorrutinas; compatibilidad con daemon o CLI antiguos.
