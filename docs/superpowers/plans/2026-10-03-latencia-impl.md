# Latencia del agente: plan de implementación

> Para quien ejecute: `superpowers:subagent-driven-development`. Diagnóstico y datos: `2026-10-03-latencia.md`. Rama `perf/latencia`; worktree `C:/Users/Jose/orca/workspaces/adbuddy/latencia` (el checkout principal `C:/Users/Jose/Documents/projects/adbuddy` NO se toca: rutas absolutas y `cd <worktree> && …` en el mismo comando). Commits con identidad hybriqtech y el trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. `go test -race` no corre en este PC. Nunca modelos Haiku.

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
- Do several steps in one call: `adbuddy tap "<text>" && adbuddy screen` taps and shows the result, and several taps can follow each other in the same command (`adbuddy tap "A" && adbuddy tap "B" && adbuddy screen`). Each call you make costs you a turn; run `adbuddy screen` after a tap or type in the same call unless you must decide something in between. If the screen still shows the old state, run `adbuddy screen` once more.
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
	AgentModel  string `toml:"agent_model"`  // model passed to Claude Code (--model); empty = the one the user configured in it
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

Después de `loadEnv`, antes de arrancar nada: `if model == "" { model = e.cfg.AgentModel }`, `if effort == "" { effort = e.cfg.AgentEffort }` y validar con `agent.ValidModel`/`ValidEffort`, devolviendo `UsageErrorT("err.serve.usage_model_invalid")` / `UsageErrorT("err.serve.usage_effort_invalid")`. Pasar `model, effort` a `newSession(app, ws, activeSerial, link.sessionInfo, model, effort)`; en `serve_session.go`, `newSession` pone `Args: func(name string) []string { a, _ := agent.Args(name, model, effort); return a }`. Antes de `startAgent`, si `model != "" || effort != ""`: `_, unused := agent.Args(agentName, model, effort)`; si `len(unused) > 0` avisar en stderr (`serve.model_not_applied`: «The model/effort you set is not applied to <agente>: only Claude Code takes it.») y si no, decir qué se aplica (`serve.model_applied`: «The agent starts with <Describe>.»). El `session.new` del móvil usa el mismo `Manager`, así que hereda lo mismo; el móvil no puede cambiarlo.

- [ ] **Paso 5: i18n y docs.** Añadir a `en.toml` y `es.toml` las claves `help.serve.flag.model`, `help.serve.flag.effort`, `err.serve.usage_model_invalid`, `err.serve.usage_effort_invalid`, `serve.model_applied`, `serve.model_not_applied` (el español, natural). En `docs/usage.md`, sección de `serve`: «Por defecto el agente usa el modelo y el esfuerzo que tengas configurados en Claude Code. Para fijarlos solo para ADBuddy: `adbuddy serve --model claude-sonnet-5-5 --effort medium` o `agent_model` y `agent_effort` en `config.toml`. Para `adbuddy run`: `adbuddy run claude -- --model … --effort …`. En nuestras medidas el modelo no cambia la latencia (ver `docs/superpowers/plans/2026-10-03-latencia.md`); cambia el coste.» (en el idioma de ese documento).
- [ ] **Paso 6:** tests del `cli` de serve (si hay helper que arranca `serve` con fakes, un caso con `--model x`; si no, `go vet` y los tests de i18n/golden/textlint al día). `cd <worktree>/host && go test ./internal/... ./cmd/... -timeout 15m`.
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

- [ ] **Paso 2: tipos y tiempos (`daemon/settle.go`).**

```go
package daemon

import (
	"context"
	"encoding/json"
	"reflect"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// Settling: after an act, or on request, the screen is read again until two
// reads in a row are the same, so the agent sees what its tap did without
// guessing a sleep. Variables for the tests.
var (
	settleFirst = 150 * time.Millisecond // after an act, before the first read: the effect starts
	settleGap   = 120 * time.Millisecond // between two reads
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
	Note   string         `json:"screen_note,omitempty"`
}

// TextSetScreen is setText's data with the screen after it.
type TextSetScreen struct {
	bridge.UITextSet
	Screen *ScreenSettled `json:"screen,omitempty"`
	Note   string         `json:"screen_note,omitempty"`
}

// asDump returns the dump an app or adb answered.
func asDump(v any) (bridge.UIDump, bool) {
	switch d := v.(type) {
	case *bridge.UIDump:
		return *d, d != nil
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

// settle reads the screen until two reads in a row are equal, at most
// settleReads of them and until start+settleMax. first is a read already made
// (nil after an act: the first read waits settleFirst). Every read goes through
// the doors and the guard like any other read of the screen: paused, a 🔴
// question waiting, and guardForeground (a protected app, ADBuddy itself, the
// shade, a notification...). If one of them says no, there is no screen: why
// says it ("" if the context ended: the agent or serve went away). It fails
// closed: nothing it could not read is returned.
func (d *Daemon) settle(ctx context.Context, args json.RawMessage, first *bridge.UIDump, start time.Time) (screen *ScreenSettled, why string, f *uiFailure) {
	prev := first
	deadline := start.Add(settleMax)
	reads := 0
	for {
		wait := settleGap
		if prev == nil {
			wait = settleFirst
		}
		if !sleepCtx(ctx, wait) {
			return nil, "", nil
		}
		if d.o.Paused() {
			return nil, PausedMessage, nil
		}
		release, other := d.admit("Read the screen", false)
		if other != "" {
			return nil, other, nil
		}
		release()
		if why := d.guardForeground(ctx, nil); why != "" {
			if ctx.Err() != nil {
				return nil, "", nil
			}
			return nil, why, nil
		}
		if d.o.Paused() { // paused while the guard looked
			return nil, PausedMessage, nil
		}
		v, _, uf := d.runUI(ctx, bridge.UIOpDump, args, nil)
		if uf != nil {
			return nil, "", uf
		}
		cur, ok := asDump(v)
		if !ok {
			return nil, "", fail(1, "UI_FAILED", "the ADBuddy app answered something unreadable")
		}
		reads++
		if prev != nil && reflect.DeepEqual(*prev, cur) {
			return &ScreenSettled{UIDump: cur, Settled: true}, "", nil
		}
		prev = &cur
		if reads >= settleReads || !time.Now().Before(deadline) {
			return &ScreenSettled{UIDump: cur, Settled: false}, "", nil
		}
	}
}
```

(`classify` y `journal` se usan en el Paso 3; si el compilador no los ve aquí, mover los imports.)

- [ ] **Paso 3: `ui()` usa `settle`.** En `ui()` (`daemon/ui.go`), después de calcular `data, via, f := d.runUI(...)` y antes de armar `code`:

```go
	// screen=dump con Screen: espera a que se asiente (la lectura de arriba cuenta como la primera).
	if f == nil && req.Screen && req.UIOp == bridge.UIOpDump {
		if first, ok := asDump(data); ok {
			scr, why, sf := d.settle(ctx, req.UIArgs, &first, d.o.Now())
			switch {
			case sf != nil:
				f = sf
			case why != "":
				f = fail(ExitDenied, "UI_BLOCKED", "Blocked by ADBuddy: "+why)
			case scr == nil: // el contexto terminó
				f = fail(1, "UI_FAILED", "stopped")
			default:
				data, via = *scr, via+", settled"
			}
		}
	}
```

(`d.o.Now()` es un reloj de pared que los tests pueden fijar; usar `time.Now()` para el plazo si `Options.Now` se sustituye en tests con un reloj fijo: el plazo debe ser tiempo real. Usar `time.Now()`.)

Y al final de `ui()`, sustituir

```go
	if data != nil {
		if err := s.Data(data); err != nil {
```

por

```go
	if data != nil {
		if req.Screen && f == nil && (req.UIOp == bridge.UIOpClick || req.UIOp == bridge.UIOpSetText) {
			release() // the change is done: a question must not wait for the screen to settle
			data = d.withScreen(ctx, req, data, via)
		}
		if err := s.Data(data); err != nil {
```

- [ ] **Paso 4: `withScreen`** (en `settle.go`): hace la lectura tras la acción y deja **su propia entrada de diario** (la acción ya dejó la suya):

```go
// withScreen adds the screen after an act (click, setText) to its result and
// journals the read as the entry of an `adbuddy screen`: a green one, or a
// blocked one when the guard said no. The act itself already happened and is
// reported as such: a block only means there is no screen to show.
func (d *Daemon) withScreen(ctx context.Context, req ipc.Request, data any, via string) any {
	start := time.Now()
	scr, why, f := d.settle(ctx, json.RawMessage("{}"), nil, start)
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
	case f != nil:
		e.Status, e.Reason, code = journal.StatusError, f.msg, f.exit
		note = classify.Printable(f.msg)
	case why != "":
		e.Status, e.Reason, code = journal.StatusBlocked, why, ExitDenied
		note = "Blocked by ADBuddy: " + classify.Printable(why)
	case scr == nil: // the agent or serve went away
		e.Status, e.Reason, code = journal.StatusDenied, abandoned(ctx, "abandoned by the agent before it ran", "abandoned: ADBuddy stopped before it ran"), ExitDenied
		note = "stopped"
	default:
		e.Status, e.Reason = journal.StatusOK, "after "+req.UIOp+", "+via+", settled="+strconv.FormatBool(scr.Settled)
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

(Imports: `ipc`, `strconv`. `helperArgv(bridge.UIOpDump, nil)` — si `json.Unmarshal(nil, …)` falla en silencio es seguro: ya ignora el error.) La nota nunca lleva texto del teléfono sin escapar (`classify.Printable`). La guarda dice `Blocked by ADBuddy:` como siempre, y el agente aplica su regla 8; el código de salida del CLI sigue siendo el de la acción (0): un 126 haría reintentar un tap que sí ocurrió.

- [ ] **Paso 5: tests (`settle_test.go`)**, con los fakes de `ui_test.go` (adb falso y `UI` que cuenta llamadas), `settleFirst`/`settleGap`/`settleMax` reducidos a milisegundos con `t.Cleanup` que los restaura (leer los valores antes de lanzar gorrutinas: no hay `-race` local):
  1. `TestSettleStopsWhenTwoReadsMatch`: tras `click` con `Screen`, la `UI` devuelve dumps A, B, B: el resultado trae `Screen` con B, `Settled` true, y la `UI` recibió 3 `dump` (más los `foreground` de la guarda, uno por lectura).
  2. `TestSettleGivesUpAtTheCap`: la `UI` devuelve un dump distinto cada vez: se detiene con `Settled` false en `settleReads` lecturas o `settleMax`, lo que llegue antes, y el resultado sigue siendo ok.
  3. `TestSettleEveryReadIsGuarded`: la `UI` de `foreground` devuelve un paquete protegido tras el click: `Screen` nil, `Note` empieza por `Blocked by ADBuddy: `, el click se dio una vez, el journal tiene la entrada del click (🟡 ok) y una `blocked` de `adbuddy screen`; la `UI` no recibió ningún `dump` después del bloqueo. Variante: ADBuddy delante, la sombra abierta (`mCurrentFocus` = NotificationShade) y `FLAG_SECURE`/`Protected` de la app (`UIForeground.Protected` true).
  4. `TestSettlePausedAndQuestionStopTheReads`: `Paused` pasa a true entre lecturas → nota de pausa y ninguna lectura más; una pregunta 🔴 en espera (`d.ask`) → nota `waiting…`.
  5. `TestSettleDumpWaitsAndBlocksMidway`: `screen` con `Screen`: el primer dump pasa, el primer plano cambia a protegida en la segunda lectura → `UI_BLOCKED` con exit 126 y **sin datos** (la pantalla no se devuelve); entrada `blocked`.
  6. `TestSettleContextCancel`: el contexto se cancela durante la espera → no hay datos, el diario dice `abandoned`, no hay pánico ni fuga de gorrutina.
  7. `TestSettleWithoutTheApp`: sin app (`UI` nil), por adb (`uiautomator`) funciona igual; con el servicio de accesibilidad activo y sin app, falla como `screen` (UI_UNAVAILABLE) y la nota lo dice, sin tocar nada más.
  8. `TestNoScreenWithoutTheFlag`: sin `Screen` el resultado es el de siempre (mismos bytes que antes, sin campos nuevos): `go test ./internal/daemon` entero verde.
  9. `TestSettleSameAsScreenGuard`: una tabla con los estados grabados (`uidump/testdata/api34`: shade, quick-settings, headsup, locked, doze, split, keyboard…) comprobando que `settle` bloquea exactamente donde `guardForeground` bloquea (usa los mismos lectores que `guard_api34_test.go`).
- [ ] **Paso 6:** `cd <worktree>/host && go vet ./... && go test ./internal/daemon ./internal/ipc -timeout 10m`, y `go test ./... -timeout 15m`.
- [ ] **Paso 7:** commit `feat(daemon): tap y type pueden devolver la pantalla ya asentada, cada lectura con la guarda de screen`.

## T4: el CLI (`--screen`, `--settle`), textos y docs

Archivos: `host/internal/cli/helpers.go`, `host/internal/cli/helpers_test.go`, `host/internal/i18n/catalog/{en,es}.toml`, goldens de ayuda (`testdata`), `host/internal/workspace/templates/AGENTS.md`, `docs/usage.md`.

- [ ] **Paso 1: `callHelper` con `Screen`.** Añadir un parámetro `screen bool` (o una variante `callHelperScreen`) que pone `req.Screen = screen`; el resto igual.
- [ ] **Paso 2: banderas.**
  - `tap` y `type`: `--screen` (bool): «after the tap, wait for the screen to settle and show it». `tap` sigue imprimiendo `Tapped (l,t)-(r,b).`; con `--screen`, después imprime una línea en blanco y `printScreen` del resultado, y si `Settled` es false añade `(the screen was still changing: run adbuddy screen again if you need it steady)`; si el resultado trae `screen_note`, imprime `The screen could not be read after it: <note>` en lugar de la pantalla (stdout, exit 0). `type` igual con `Typed.`.
  - `screen`: `--settle` (bool): «wait for the screen to settle before showing it»: con él `callHelper(..., true)`; la salida es la de siempre más el aviso de `Settled` false.
  - `--json`: `tap --screen --json` escribe `daemon.ClickedScreen` completo, `type --screen --json` `daemon.TextSetScreen`, `screen --settle --json` `daemon.ScreenSettled`.
  - Un daemon antiguo contesta sin `screen`: el CLI imprime solo lo de la acción (no es error).
  - `tap`/`type` con `--screen` y `--all`: no se añade; la pantalla es la de `screen` sin `--all`.
- [ ] **Paso 3: tests (`helpers_test.go`)** con el daemon falso que ya usan: `tap X --screen` imprime `Tapped` y la pantalla; con nota imprime la nota y sale 0; `screen --settle` con `settled=false` imprime el aviso; `--json` conserva los campos; `tap X` sin bandera imprime lo de siempre; textos en `en` y `es` (el aviso).
- [ ] **Paso 4: i18n y goldens.** Claves `help.tap.flag.screen`, `help.type.flag.screen`, `help.screen.flag.settle` (y las del aviso y de la nota si pasan por `app.T`), en `en.toml` y `es.toml`; regenerar los goldens de ayuda que cambien (`go test ./internal/cli -run Golden -update` si existe la bandera; si no, a mano) y revisar que solo cambian las líneas nuevas.
- [ ] **Paso 5: `AGENTS.md`.** Sustituir la viñeta de «Do several steps in one call» (T1) y las de `tap`/`type` por:

```
- `adbuddy tap "<text>"`, `adbuddy tap --id <id>`, `adbuddy tap --xy <x,y>`: tap an element by its text or description, by its id, or a point. If several elements match, use `--id` or `--xy` from `adbuddy screen`.
- `adbuddy type "<text>"`: type in the field that has the focus (tap it first). It replaces the field's text.
- Add `--screen` to `tap` or `type` to get the screen right after it, once it has settled, in the same call: `adbuddy tap "Network & internet" --screen`. Do not `sleep` and do not run `adbuddy screen` in a separate call afterwards. After a raw `adb` command (`monkey`, `am start`, `input`) use `adbuddy screen --settle`. Several steps can go in one call (`adbuddy tap "A" && adbuddy tap "B" --screen`): each call you make costs you a turn.
```

  y la frase de `adbuddy screen`: «Run it before tapping; after a tap or type use `--screen` instead of running it again.» Actualizar el test de T1 (`in one call` → `--screen`). En `docs/usage.md` (sección «Screen helpers»): describir `--screen` y `--settle` con un ejemplo, y que esperan como mucho unos 2 s.
- [ ] **Paso 6:** `cd <worktree>/host && go vet ./... && go test ./... -timeout 15m`.
- [ ] **Paso 7:** commit `feat(cli): tap --screen, type --screen y screen --settle`.

## T5: medir antes y después

Con `latbench` (ver `2026-10-03-latencia.md`), tareas 3 y 5, `opus:medium` y `sonnet:medium`, 2 pasadas por celda, antes (rama en `4a745ef`) y después de cada tarea que cambie el comportamiento del agente (T1, y T4). `ADBUDDY_LATENCY=1 ADBUDDY_LATENCY_AGENT=1 ADBUDDY_LATENCY_TASKS=3,5 ADBUDDY_LATENCY_CONFIGS=opus:medium,sonnet:medium go test ./cmd/adbuddy -run TestLatencyAgentFakePhone`. El teléfono falso (`latbench.Phone`) debe aprender a contestar a `tap --screen`: no necesita cambios, el daemon real es quien espera. Apuntar tabla antes/después en `2026-10-03-latencia.md`.

---

## Revisión de seguridad que se pide a opus (plan y diff de T3 y T4)

1. Cada lectura de la espera pasa por `paused`, `admit` (pregunta 🔴 en espera) y `guardForeground`, igual que `screen`; ninguna ruta lee la pantalla sin la guarda; el bloqueo a mitad de la espera no devuelve nada leído antes (en `screen --settle`) ni el resto de la pantalla (en `tap --screen`).
2. Sin la app, sin `uiautomator` con el servicio de accesibilidad activo (misma regla).
3. La acción ya hecha nunca se reintenta ni se presenta como fallida; el CLI sale 0 aunque la lectura se bloquee, y la nota empieza por `Blocked by ADBuddy:` para que el agente aplique su regla.
4. `release()` antes de la lectura: una pregunta 🔴 no espera la espera; pero una pregunta 🔴 que aparece después bloquea las lecturas siguientes.
5. Tiempo acotado (`settleMax`, `settleReads`) y cancelación por contexto; sin gorrutinas nuevas.
6. El diario: una entrada por la acción (como hoy) y una por la lectura; el contenido de la pantalla no se escribe (solo el estado).
7. Orden con un daemon o un CLI antiguos (campo `screen` ignorado): degrada a lo de siempre.
