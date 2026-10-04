# Latencia del agente: plan de implementación

> Para quien ejecute: `superpowers:subagent-driven-development`. Diagnóstico y datos: `2026-10-03-latencia.md`. Rama `perf/latencia`; worktree `<worktree>` (el checkout principal `<checkout principal>` NO se toca: rutas absolutas y `cd <worktree> && …` en el mismo comando). Commits con identidad hybriqtech y el trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. `go test -race` no corre en este PC. Nunca modelos Haiku.

**Objetivo:** que el agente tarde menos desde que el usuario pide algo hasta que el móvil cambia y responde, sin capar al agente y sin abrir ningún hueco en la guarda. El tiempo se va en los turnos del modelo (~1,8 s cada uno), no en las órdenes (4-14 ms con el teléfono instantáneo): se quitan turnos y esperas del agente.

**Orden de tareas** (cada una compila y pasa sus tests sola):

| Tarea | Qué | Seguridad |
|---|---|---|
| T1 | Texto de las plantillas: encadenar helpers, `@NOTES.md` | ninguna |
| T2 | `--model`/`--effort` opcionales en `adbuddy serve` | no toca la guarda; los valores los pone el usuario, nunca el agente ni el móvil |
| T3 | Daemon: esperar a que la pantalla se asiente y devolverla (`Request.Screen`) | **sí: guarda y journal** (revisión opus de plan y diff) |
| T4 | CLI: `tap --screen`, `type --screen`, `screen --settle`; textos y docs | texto de la guarda; revisión opus del diff junto con T3 |
| T5 | Medir antes y después con el banco (`latbench`) | — |

Lo del dispositivo (guarda en paralelo, `foreground` ligero) va en otro plan cuando haya cupo de emulador.

---

## T1: texto de las plantillas

Archivos: `host/internal/workspace/templates/AGENTS.md`, `host/internal/workspace/templates/CLAUDE.md`, `host/internal/workspace/workspace_test.go` (o el test que ya comprueba las plantillas), `docs/usage.md` si habla de las plantillas.

- [ ] **Paso 1: test que falla.** Un test en `workspace_test.go` que, tras `Init` en un directorio temporal, comprueba que `CLAUDE.md` contiene la línea `@NOTES.md` además de `@AGENTS.md`, y que `AGENTS.md` contiene la frase `in one call` en el apartado de la pantalla (el aviso de encadenar). Debe fallar.
- [ ] **Paso 2: `CLAUDE.md`.** Poner `@NOTES.md` justo debajo de `@AGENTS.md`:

```
@AGENTS.md
@NOTES.md

## Claude Code

- `adb` and `adbuddy` are pre-approved in `.claude/settings.json`. ADBuddy asks the user itself before anything dangerous, so do not ask twice for the same command.
```

(Claude Code carga los ficheros importados con `@` en su contexto: el agente deja de leer `NOTES.md` con una llamada propia.)
- [ ] **Paso 3: `AGENTS.md`.**
  - Cambiar la frase de NOTES: «`NOTES.md` has what the user wants you to remember about this phone. If it is not already in your context, read it before you start.»
  - En el apartado `## The screen`, después de la viñeta de `adbuddy type`, añadir esta viñeta (texto exacto):

```
- Do several steps in one call: `adbuddy tap "<text>" && adbuddy screen` taps and shows the result, and several taps can follow each other in the same command only when both targets are on the screen you already read (`adbuddy tap "A" && adbuddy tap "B" && adbuddy screen`); if B only exists after A, run the screen between them and look before you tap (rule 5 still applies). Each call you make costs you a turn; run `adbuddy screen` after a tap or type in the same call unless you must decide something in between. If the screen still shows the old state, run `adbuddy screen` once more.
```
  - No cambiar nada más (la regla 15, los niveles, etc.).
- [ ] **Paso 4:** `cd <worktree>/host && go test ./internal/workspace ./internal/cli -timeout 10m`. Si algún golden o test de textos (`textlint_test.go`, `docs_test.go`) falla, adaptarlo a lo nuevo y nada más.
- [ ] **Paso 5:** commit `feat(workspace): el agente encadena tap y screen en una llamada y recibe NOTES.md por @`.

## T2: `--model` y `--effort` opcionales en `adbuddy serve`

Principio: por defecto el agente hereda el modelo y el esfuerzo del usuario (no se pasa nada). Solo si el usuario los pone (en `config.toml` o con la bandera) se pasan a Claude Code. Otros agentes: no se aplican y `serve` lo dice una vez.

Archivos: `host/internal/config/config.go` y `config_test.go`; `host/internal/agent/args.go` (nuevo) y `args_test.go`; `host/internal/session/session.go` (+ test); `host/internal/cli/serve.go` y `serve_session.go`; `host/internal/i18n/catalog/en.toml` y `es.toml`; `docs/usage.md`.

- [ ] **Paso 1: `internal/agent/args.go`.**

```go
package agent

import (
	"fmt"
	"regexp"
	"slices"
)

// Efforts are the values Claude Code takes for --effort.
var Efforts = []string{"low", "medium", "high", "xhigh", "max"}

// modelRE is a model alias or id as Claude Code takes it for --model:
// letters, digits and . _ : - [ ] (so opus, claude-sonnet-5-5, opus[1m]).
// It never starts with "-": it cannot be read as another option.
var modelRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:\[\]-]{0,63}$`)

// ValidModel reports whether s is acceptable as a model name for the agent.
func ValidModel(s string) bool { return modelRE.MatchString(s) }

// ValidEffort reports whether s is one of Efforts.
func ValidEffort(s string) bool { return slices.Contains(Efforts, s) }

// Args returns the command-line arguments that make agent use model and
// effort, and the names of the settings that agent does not take (so serve can
// say they were not applied). Empty values add nothing: the agent keeps what
// the user configured in it. Only Claude Code is known to take both.
func Args(name, model, effort string) (args []string, unused []string) {
	if name == "claude" {
		if model != "" {
			args = append(args, "--model", model)
		}
		if effort != "" {
			args = append(args, "--effort", effort)
		}
		return args, nil
	}
	if model != "" {
		unused = append(unused, "model")
	}
	if effort != "" {
		unused = append(unused, "effort")
	}
	return nil, unused
}

// Describe is what serve tells the person for a setting it applies.
func Describe(model, effort string) string {
	switch {
	case model != "" && effort != "":
		return fmt.Sprintf("model %s, effort %s", model, effort)
	case model != "":
		return "model " + model
	case effort != "":
		return "effort " + effort
	}
	return ""
}
```

Tests (`args_test.go`): `ValidModel` acepta `opus`, `claude-sonnet-5-5`, `opus[1m]`, `claude-opus-5-5`; rechaza ``, `-x`, `--model`, `a b`, `a;b`, `a\nb`, 65 caracteres, `$(x)`. `ValidEffort` acepta los cinco y rechaza `LOW`, ``, `ultra`. `Args("claude","","")` es nil; `Args("claude","opus","low")` es `[--model opus --effort low]`; `Args("codex","x","")` es nil con unused `[model]`.

- [ ] **Paso 2: config.** En `Config` añadir

```go
	AgentModel  string `toml:"agent_model"`  // model passed to Claude Code (--model) by `adbuddy serve` only (`adbuddy run` takes it after --); empty = the one the user configured in it
	AgentEffort string `toml:"agent_effort"` // effort passed to Claude Code (--effort); empty = the user's own
```

y en `Validate()`: si `AgentModel != "" && !agent.ValidModel(...)` error `agent_model is not a model name: %q`; si `AgentEffort != "" && !agent.ValidEffort(...)` error `agent_effort must be one of low, medium, high, xhigh, max, got %q`. (Si `config` no puede importar `agent` sin ciclo, mover `ValidModel`/`ValidEffort`/`Efforts` a un paquete hoja `internal/agentopts` y que `agent` los reexporte; comprobar con `go build`.) Tests en `config_test.go`: ambos válidos cargan; inválidos fallan con el nombre de la clave; ausentes dejan vacío y `Save` no escribe valores por defecto raros (los campos vacíos se guardan como `""`; comprobar que `Load` de lo guardado sigue válido).

- [ ] **Paso 3: sesión.** En `session.Options` añadir

```go
	// Args are extra command-line arguments for the agent's program, after
	// --continue: the model and effort the user chose (agent.Args). May be nil.
	Args func(agent string) []string
```

y en `startLocked`, **separar «reanudar» de «los argumentos»** (hoy `resumed: len(args) > 0` los confunde y la reanudación sin éxito se reintenta sin `--continue`):

```go
	var args []string
	resumed := resume && agent == "claude"
	if resumed {
		args = []string{"--continue"}
	}
	if m.o.Args != nil {
		args = append(args, m.o.Args(agent)...)
	}
	...
	pr := &proc{p: p, agent: agent, resumed: resumed, ...
```

Test en `session_test.go` (con el `Start` falso que ya usan los demás): con `Args` que devuelve `[--model x]`, `Start("claude", true)` arranca con `[--continue --model x]` y `resumed` es true; `Start("claude", false)` con `[--model x]` y `resumed` false (y un fallo rápido NO reintenta); `Start("codex", true)` con `Args` que devuelve nil no lleva nada.

- [ ] **Paso 4: serve.** En `newServeCmd`: variables `var model, effort string` y banderas

```go
	cmd.Flags().StringVar(&model, "model", "", app.T("help.serve.flag.model"))
	cmd.Flags().StringVar(&effort, "effort", "", app.T("help.serve.flag.effort"))
```

Después de `loadEnv`, antes de arrancar nada: `if model == "" { model = e.cfg.AgentModel }`, `if effort == "" { effort = e.cfg.AgentEffort }` y validar con `agent.ValidModel`/`ValidEffort`, devolviendo `UsageErrorT("err.serve.usage_model_invalid")` / `UsageErrorT("err.serve.usage_effort_invalid")`. Pasar `model, effort` a `newSession(app, ws, activeSerial, link.sessionInfo, model, effort)`; en `serve_session.go`, `newSession` pone `Args: func(name string) []string { a, _ := agent.Args(name, model, effort); return a }`. Antes de `startAgent`, si `model != "" || effort != ""`: `_, unused := agent.Args(agentName, model, effort)`; si `len(unused) > 0` avisar en stderr (`serve.model_not_applied`: «The model/effort you set is not applied to %[1]s: only Claude Code takes it.», con `%[1]s` en `en` y en `es`: el test de paridad del catálogo lo mira) y si no, decir qué se aplica (`serve.model_applied`: «The agent starts with <Describe>.»). El `session.new` del móvil usa el mismo `Manager`, así que hereda lo mismo; el móvil no puede cambiarlo.

- [ ] **Paso 5: i18n y docs.** Añadir a `en.toml` y `es.toml` las claves `help.serve.flag.model`, `help.serve.flag.effort`, `err.serve.usage_model_invalid`, `err.serve.usage_effort_invalid`, `serve.model_applied`, `serve.model_not_applied` (el español, natural). En `docs/usage.md`, sección de `serve`: «Por defecto el agente usa el modelo y el esfuerzo que tengas configurados en Claude Code. Para fijarlos solo para ADBuddy: `adbuddy serve --model claude-sonnet-5-5 --effort medium` o `agent_model` y `agent_effort` en `config.toml`. Para `adbuddy run`: `adbuddy run claude -- --model … --effort …`. En nuestras medidas el modelo no cambia la latencia (ver `docs/superpowers/plans/2026-10-03-latencia.md`); cambia el coste.» (en el idioma de ese documento).
- [ ] **Paso 6:** tests del `cli` de serve (si hay helper que arranca `serve` con fakes, un caso con `--model x`; si no, `go vet` y los tests de i18n/golden/textlint al día). `cd <worktree>/host && go test ./internal/... ./cmd/... -timeout 15m` (los goldens de ayuda de `serve` se actualizan con `-update-help`).
- [ ] **Paso 7:** commit `feat(serve): --model y --effort opcionales para Claude Code (por defecto, los del usuario)`.

## T3: el daemon espera a que la pantalla se asiente y la devuelve

Idea: tras un `tap` o un `type`, el agente necesita la pantalla nueva; hoy tarda un turno entero (~1,8 s) más un `sleep`. Con `Request.Screen` el daemon, tras la acción, espera a que la pantalla se estabilice y devuelve la lectura, **cada lectura con la misma guarda que un `screen` normal**. También `screen` con `Screen` espera a que se asiente antes de contestar (sustituye al `sleep` tras una orden `adb` cruda). Sin cambios en el protocolo con la app: se sondea el `dump` (funciona con apps viejas y sin app: uiautomator).

Archivos: `host/internal/ipc/ipc.go` (campo), `host/internal/daemon/ui.go` (+ `settle.go` nuevo), `host/internal/daemon/settle_test.go` (nuevo).

- [ ] **Paso 1: ipc.** En `ipc.Request`, después de `UIArgs`:

```go
	// Screen, in a KindUI request: for click and setText, also read the screen
	// once it settles (the result then carries it); for dump, wait for it to
	// settle before answering. Ignored by an older daemon.
	Screen bool `json:"screen,omitempty"`
```

- [ ] **Paso 2: tipos y espera (`daemon/settle.go`).**

```go
package daemon

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// Settling: after an act, or on request, the screen is read again until two
// reads in a row are the same, so the agent sees what its tap did without
// guessing a sleep. Variables for the tests (settle copies them on entry).
var (
	settleFirst = 150 * time.Millisecond // after an act, before the first read: the effect starts
	settleGap   = 120 * time.Millisecond // between two reads
	settleMin   = 400 * time.Millisecond // two equal reads count as settled only this long after the act or the request: a slow transition shows the old screen twice
	settleMax   = 2 * time.Second        // all in all, counted from the act or from the request
	settleReads = 8                      // reads at most
)

// ScreenSettled is dump's data when the client asked to wait for the screen
// to settle. Settled is false when the screen was still changing at the end
// of the wait (an animation, a video).
type ScreenSettled struct {
	bridge.UIDump
	Settled bool `json:"settled"`
}

// ClickedScreen is click's data with the screen after it. Screen is nil, and
// Note says why, when it could not be read (the guard blocked it: the tap may
// have opened a protected app).
type ClickedScreen struct {
	bridge.UIClicked
	Screen *ScreenSettled `json:"screen,omitempty"`
	Note   string         `json:"screenNote,omitempty"`
}

// TextSetScreen is setText's data with the screen after it.
type TextSetScreen struct {
	bridge.UITextSet
	Screen *ScreenSettled `json:"screen,omitempty"`
	Note   string         `json:"screenNote,omitempty"`
}

// asDump returns the dump an app or adb answered.
func asDump(v any) (bridge.UIDump, bool) {
	switch d := v.(type) {
	case *bridge.UIDump:
		if d == nil {
			return bridge.UIDump{}, false
		}
		return *d, true
	case bridge.UIDump:
		return d, true
	}
	return bridge.UIDump{}, false
}

// sleepCtx waits d, or until ctx ends (false).
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// settle reads the screen until two reads in a row are equal and settleMin
// has passed since start, at most settleReads reads and until start+settleMax.
// first is a read already made (nil after an act: the first read waits
// settleFirst). Every read goes through the doors and the guard like any other
// read of the screen: paused, a 🔴 question waiting, and guardForeground (a
// protected app, ADBuddy itself, the shade, a notification...). If one of them
// says no, there is no screen: why says it. why and f are both empty when the
// context ended (the agent or serve went away). It fails closed: nothing it
// could not read is returned. via says how the last read was made.
func (d *Daemon) settle(ctx context.Context, args json.RawMessage, first *bridge.UIDump, start time.Time) (screen *ScreenSettled, via, why string, f *uiFailure) {
	firstWait, gap, min, max, maxReads := settleFirst, settleGap, settleMin, settleMax, settleReads
	prev := first
	deadline := start.Add(max)
	reads := 0
	for {
		wait := gap
		if prev == nil {
			wait = firstWait
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
			return nil, "", why, nil
		}
		if d.o.Paused() { // paused while the guard looked
			return nil, "", PausedMessage, nil
		}
		v, how, uf := d.runUI(ctx, bridge.UIOpDump, args, nil)
		if uf != nil {
			return nil, "", "", uf
		}
		cur, ok := asDump(v)
		if !ok {
			return nil, "", "", fail(1, "UI_FAILED", "the ADBuddy app answered something unreadable")
		}
		via = how
		reads++
		now := time.Now()
		if prev != nil && reflect.DeepEqual(*prev, cur) && now.Sub(start) >= min {
			return &ScreenSettled{UIDump: cur, Settled: true}, via, "", nil
		}
		prev = &cur
		if reads >= maxReads || !now.Before(deadline) {
			return &ScreenSettled{UIDump: cur, Settled: false}, via, "", nil
		}
	}
}
```

(`reflect.DeepEqual` compara bien los `*bool` por valor. Si `settleMin` ≥ `settleMax` en un test, nunca se da por asentada: es el comportamiento esperado.)

- [ ] **Paso 3: `ui()` usa `settle` para `screen` con `Screen`.** En `ui()` (`daemon/ui.go`), justo después de `data, via, f := d.runUI(ctx, req.UIOp, req.UIArgs, s)`:

```go
	// screen with Screen: wait for it to settle; the read above counts as the first one.
	if f == nil && req.Screen && req.UIOp == bridge.UIOpDump {
		if first, ok := asDump(data); ok {
			scr, how, why, sf := d.settle(ctx, req.UIArgs, &first, time.Now())
			switch {
			case sf != nil:
				f = sf
			case why != "":
				return d.blockUI(e, why) // nothing read before is returned
			case scr == nil: // the agent or serve went away
				code := ExitDenied
				e.Status, e.Reason, e.ExitCode = journal.StatusDenied, abandoned(ctx, "abandoned by the agent before it ran", "abandoned: ADBuddy stopped before it ran"), &code
				d.record(e)
				return ipc.Exit{Code: ExitDenied, ErrorCode: "UI_BLOCKED", Error: "stopped"}
			default:
				data, via = *scr, how+", settled"
			}
		}
	}
```

(`time` e `ipc` ya se importan en `ui.go`; si no, añadirlos.) Y al final de `ui()`, sustituir

```go
	if data != nil {
		if err := s.Data(data); err != nil {
```

por

```go
	if data != nil {
		if req.Screen && f == nil && (req.UIOp == bridge.UIOpClick || req.UIOp == bridge.UIOpSetText) {
			release() // the change is done: a question must not wait for the screen to settle
			data = d.withScreen(ctx, req, data)
		}
		if err := s.Data(data); err != nil {
```

- [ ] **Paso 4: `withScreen`** (en `settle.go`): lee tras la acción y deja **su propia entrada de diario** (la acción ya dejó la suya):

```go
// notDone goes first in every note about a screen that could not be read
// after an act: the act happened, and repeating it would do it twice.
const notDone = "The action was done; do not repeat it. The screen could not be read after it: "

// withScreen adds the screen after an act (click, setText) to its result and
// journals the read as the entry of an `adbuddy screen`: a green one, or a
// blocked one when the guard said no. The act itself already happened and is
// reported as such: a block only means there is no screen to show.
func (d *Daemon) withScreen(ctx context.Context, req ipc.Request, data any) any {
	scr, via, why, f := d.settle(ctx, json.RawMessage("{}"), nil, time.Now())
	now := d.o.Now().UTC()
	h := helpers[bridge.UIOpDump]
	e := journal.Entry{
		ID: journal.NewID("j_", now), TS: now, Session: d.o.Session, Serial: d.identity(), Source: journal.SourceUser,
		Argv: helperArgv(bridge.UIOpDump, nil), Level: h.level.String(), Rule: h.rule, Description: h.title.In(d.o.Lang),
	}
	if req.Env["ADBUDDY_SOURCE"] == journal.SourceAgent {
		e.Source = journal.SourceAgent
	}
	note := ""
	code := 0
	switch {
	case f != nil && f.exit == ExitDenied: // the app's own check (a protected screen, FLAG_SECURE, a notification)
		e.Status, e.Reason, code = journal.StatusBlocked, f.msg, ExitDenied
		note = notDone + "Blocked by ADBuddy: " + classify.Printable(f.msg)
	case f != nil:
		e.Status, e.Reason, code = journal.StatusError, f.msg, f.exit
		note = notDone + classify.Printable(f.msg)
	case why != "":
		e.Status, e.Reason, code = journal.StatusBlocked, why, ExitDenied
		note = notDone + "Blocked by ADBuddy: " + classify.Printable(why)
	case scr == nil: // the agent or serve went away
		e.Status, e.Reason, code = journal.StatusDenied, abandoned(ctx, "abandoned by the agent before it ran", "abandoned: ADBuddy stopped before it ran"), ExitDenied
		note = notDone + "stopped"
	default:
		e.Status, e.Reason = journal.StatusOK, "after "+req.UIOp+", via "+via+", settled="+strconv.FormatBool(scr.Settled)
	}
	e.ExitCode = &code
	d.record(e)
	switch v := data.(type) {
	case *bridge.UIClicked:
		return ClickedScreen{UIClicked: *v, Screen: scr, Note: note}
	case bridge.UIClicked:
		return ClickedScreen{UIClicked: v, Screen: scr, Note: note}
	case *bridge.UITextSet:
		return TextSetScreen{UITextSet: *v, Screen: scr, Note: note}
	case bridge.UITextSet:
		return TextSetScreen{UITextSet: v, Screen: scr, Note: note}
	}
	return data
}
```

La nota nunca lleva texto del teléfono sin escapar (`classify.Printable`), empieza por decir que la acción se hizo y no se repita, y la guarda dice `Blocked by ADBuddy:` como siempre para que el agente aplique su regla 8 **solo a leer la pantalla**. El código de salida del CLI sigue siendo el de la acción (0): un 126 haría reintentar un tap que sí ocurrió. Entradas de diario: la de la acción (como hoy) y esta; la spec M5 §4.1 y §11 («una entrada por ayuda») se actualizan en T4.

- [ ] **Paso 5: tests (`settle_test.go`)**, con los fakes de `ui_test.go`. Ojo: el `fakeApp` actual da un resultado fijo por operación (`ui_test.go:23-38`); este paso añade a `settle_test.go` un fake propio que devuelve una **lista** de dumps por llamada a `dump` (y registra la secuencia de operaciones: cada `dump` debe ir precedido de un `foreground`). `settleFirst`, `settleGap`, `settleMin` y `settleMax` se reducen a milisegundos con `t.Cleanup` que los restaura:
  1. `TestSettleStopsWhenTwoReadsMatch`: tras `click` con `Screen`, la `UI` devuelve dumps A, B, B: el resultado trae `Screen` con B y `Settled` true; la secuencia es `foreground, dump, foreground, dump, foreground, dump` (cada lectura guardada).
  2. `TestSettleMinimumTime`: con `settleMin` mayor que dos lecturas, A, A, A, B, B no da por asentada la primera pareja igual: devuelve B asentada (o `Settled` false si el tope llega antes), nunca A.
  3. `TestSettleGivesUpAtTheCap`: la `UI` devuelve un dump distinto cada vez: se detiene con `Settled` false en `settleReads` lecturas o `settleMax`, y el resultado sigue siendo ok.
  4. `TestSettleEveryReadIsGuarded`: la `UI` de `foreground` devuelve un paquete protegido tras el click: `Screen` nil, `Note` empieza por `The action was done; do not repeat it.` y contiene `Blocked by ADBuddy: `, el click se dio una vez, el diario tiene la entrada del click (🟡 ok) y una `blocked` de `adbuddy screen`; la `UI` no recibió ningún `dump` después del bloqueo. Variantes: ADBuddy delante (`UIForeground.Package` = el de la app), la sombra abierta (`mCurrentFocus` = NotificationShade), `UIForeground.Protected` true, y el propio `dump` del app que contesta `CodeProtected` (la nota lleva el prefijo y el diario dice `blocked`, no `error`).
  5. `TestSettlePausedAndQuestionStopTheReads`: `Paused` pasa a true entre lecturas → nota de pausa y ninguna lectura más; una pregunta 🔴 en espera (`d.ask`) → nota `waiting…`.
  6. `TestSettleDumpWaitsAndBlocksMidway`: `screen` con `Screen`: el primer dump pasa, el primer plano cambia a protegida en la segunda lectura → exit 126 con `UI_BLOCKED`, **sin datos** (la primera lectura no sale) y entrada `blocked` con el motivo sin el prefijo repetido (como `blockUI`).
  7. `TestSettleContextCancel`: el contexto se cancela durante la espera, tanto en `screen --settle` como tras un tap → no hay datos, el diario dice `abandoned` (`StatusDenied`), no hay pánico. `settle` copia las variables de tiempo al entrar: el test no compite con su `t.Cleanup`.
  8. `TestSettleWithoutTheApp`: sin app (`UI` nil), por adb (`uiautomator`) funciona igual; con el servicio de accesibilidad activo y sin app, falla como `screen` (UI_UNAVAILABLE) y la nota lo dice.
  9. `TestNoScreenWithoutTheFlag`: sin `Screen` el resultado es el de siempre (mismos bytes, sin campos nuevos); `go test ./internal/daemon` entero verde.
- [ ] **Paso 6:** `cd <worktree>/host && go vet ./... && go test ./internal/daemon ./internal/ipc -timeout 10m`, y `go test ./... -timeout 15m`.
- [ ] **Paso 7:** commit `feat(daemon): tap y type pueden devolver la pantalla ya asentada, cada lectura con la guarda de screen`.

## T4: el CLI (`--screen`, `--settle`), textos, spec y docs

Archivos: `host/internal/cli/helpers.go`, `host/internal/cli/helpers_test.go`, `host/internal/i18n/catalog/{en,es}.toml` (solo la ayuda de las banderas), goldens de ayuda, `host/internal/workspace/templates/AGENTS.md`, `docs/usage.md`, `docs/superpowers/specs/2026-10-01-adbuddy-m5-ayudas-remoto-design.md`, `docs/superpowers/plans/2026-10-03-latencia.md` (validaciones pendientes).

Lo que imprimen las órdenes para el agente va siempre en inglés (M6c §3; `textlint_test.go` exime a `helpers.go`): por el catálogo solo pasan las ayudas de las banderas.

- [ ] **Paso 1: `callHelper` con `Screen`.** Añadir un parámetro `screen bool` (o una variante `callHelperScreen`) que pone `req.Screen = screen`; el resto igual.
- [ ] **Paso 2: banderas.**
  - `tap` y `type`: `--screen` (bool). `tap` sigue imprimiendo `Tapped (l,t)-(r,b).`; con `--screen`, el CLI decodifica la respuesta en una estructura con los campos de `UIClicked` más `Screen *daemon.ScreenSettled` y `Note string` (`json:"screenNote"`) y:
    - con pantalla: línea en blanco, `printScreen` de ella y, si `Settled` es false, `(the screen was still changing: run adbuddy screen again if you need it steady)`;
    - con `Note`: imprime `Note` tal cual (empieza por `The action was done; do not repeat it.`) en stdout y sale 0;
    - sin pantalla ni nota (un daemon antiguo que ignoró `screen`): imprime `This ADBuddy did not return the screen: run adbuddy screen.`;
    - `type` igual con `Typed.`.
  - `screen`: `--settle` (bool): `callHelper(..., true)`; la salida es la de siempre más el aviso de `Settled` false. Decodificar `settled` como `*bool`: ausente (daemon antiguo) no avisa.
  - `--json`: `tap --screen --json` escribe la respuesta completa (`daemon.ClickedScreen`), `type --screen --json` `daemon.TextSetScreen`, `screen --settle --json` `daemon.ScreenSettled`.
  - `tap`/`type` con `--screen` y `--all`: no se añade; la pantalla es la de `screen` sin `--all`.
- [ ] **Paso 3: tests (`helpers_test.go`)** con el daemon falso que ya usan: `tap X --screen` imprime `Tapped` y la pantalla; con nota imprime la nota (contiene `do not repeat`) y sale 0; sin pantalla ni nota imprime el aviso del daemon antiguo; `screen --settle` con `settled=false` imprime el aviso y con `settled` ausente no; `--json` conserva los campos; `tap X` sin bandera imprime lo de siempre.
- [ ] **Paso 4: i18n y goldens.** Claves `help.tap.flag.screen`, `help.type.flag.screen`, `help.screen.flag.settle` (solo ayuda), en `en.toml` y `es.toml`; regenerar los goldens de ayuda que cambien (`go test ./internal/cli -run Golden -update-help`) y revisar que solo cambian las líneas nuevas.
- [ ] **Paso 5: `AGENTS.md`.** Sustituir la viñeta «Do several steps in one call» (T1) y las de `tap`/`type` por:

```
- `adbuddy tap "<text>"`, `adbuddy tap --id <id>`, `adbuddy tap --xy <x,y>`: tap an element by its text or description, by its id, or a point. If several elements match, use `--id` or `--xy` from `adbuddy screen`.
- `adbuddy type "<text>"`: type in the field that has the focus (tap it first). It replaces the field's text.
- Add `--screen` to `tap` or `type` to get the screen right after it, in the same call, once it has settled: `adbuddy tap "Network & internet" --screen`. Do not `sleep` and do not run `adbuddy screen` in a separate call afterwards. After a raw `adb` command (`monkey`, `am start`, `input`) use `adbuddy screen --settle`. Several steps can go in one call (`adbuddy tap "A" && adbuddy tap "B" --screen`) only when both targets are on the screen you already read; if B only exists after A, use `--screen` after A and look before you tap (rule 5 still applies). Each call you make costs you a turn. If the screen still shows the old state, run `adbuddy screen` once more.
- With `--screen`, if the answer says `The action was done; do not repeat it`, the tap or the text already happened: whatever reason follows (rule 8) is only about reading the screen. Never repeat the action; run `adbuddy screen` when the reason allows it.
```

  y la frase de `adbuddy screen`: «Run it before tapping; after a tap or type use `--screen` instead of running it again.» Actualizar el test de T1 (`in one call` → `--screen`). En `docs/usage.md` (sección «Screen helpers»): describir `--screen` y `--settle` con un ejemplo, que esperan como mucho unos 2 s, y que los workspaces ya creados reciben `@NOTES.md` y estos textos con `adbuddy env init --force`.
- [ ] **Paso 6: spec y validaciones.** En la spec M5 (§3.1-3.3, §4.1 y §11) apuntar `--screen`/`--settle` y que `tap --screen` deja dos entradas (la de la acción y una `adbuddy screen` de la lectura). En `2026-10-03-latencia.md`, sección nueva «Validaciones manuales pendientes»: medir en el emulador y en un móvil real (con permiso del usuario) los tiempos de transición y de `uiautomator` para ajustar `settleFirst/Gap/Min/Max`; comprobar que una apertura de app con notificación entrante da la nota de bloqueo y el agente no repite el tap; `screen --settle` tras `am start`.
- [ ] **Paso 7:** `cd <worktree>/host && go vet ./... && go test ./... -timeout 15m`.
- [ ] **Paso 8:** commit `feat(cli): tap --screen, type --screen y screen --settle`.

## T5: medir antes y después

Con `latbench` (ver `2026-10-03-latencia.md`), tareas 3 y 5, `opus:medium` y `sonnet:medium`, 2 pasadas por celda: **antes** en `55809cd` (producción sin cambios; el banco llega con `2c99bd4`), y después de T1 y después de T4. `ADBUDDY_LATENCY=1 ADBUDDY_LATENCY_AGENT=1 ADBUDDY_LATENCY_TASKS=3,5 ADBUDDY_LATENCY_CONFIGS=opus:medium,sonnet:medium go test ./cmd/adbuddy -run TestLatencyAgentFakePhone`. El teléfono falso (`latbench.Phone`) no necesita cambios: el daemon real es quien espera y sondea el `dump` del teléfono falso. Apuntar la tabla antes/después en `2026-10-03-latencia.md`.

---

## Revisión de seguridad que se pide a opus (plan y diff de T3 y T4)

1. Cada lectura de la espera pasa por `paused`, `admit` (pregunta 🔴 en espera) y `guardForeground`, igual que `screen`; ninguna ruta lee la pantalla sin la guarda; el bloqueo a mitad de la espera no devuelve nada leído antes (en `screen --settle`) ni el resto de la pantalla (en `tap --screen`).
2. Sin la app, sin `uiautomator` con el servicio de accesibilidad activo (misma regla).
3. La acción ya hecha nunca se reintenta ni se presenta como fallida; el CLI sale 0 aunque la lectura se bloquee, y la nota dice primero que no se repita y después `Blocked by ADBuddy:` para que el agente aplique su regla 8 solo a la lectura.
4. `release()` antes de la lectura: una pregunta 🔴 no espera la espera; pero una pregunta 🔴 que aparece después bloquea las lecturas siguientes.
5. Tiempo acotado (`settleMax`, `settleReads`, `settleMin`) y cancelación por contexto; sin gorrutinas nuevas.
6. El diario: una entrada por la acción (como hoy) y una por la lectura; el contenido de la pantalla no se escribe (solo el estado).
7. Orden con un daemon o un CLI antiguos (campo `screen` ignorado): degrada a lo de siempre.
