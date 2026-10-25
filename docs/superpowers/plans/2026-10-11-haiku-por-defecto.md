# Haiku 5.5 low por defecto en el agente de `adbuddy serve`

Orden de la persona usuaria (2026-10-11): el agente Claude Code que arranca `adbuddy serve` usa `claude-haiku-5-5` con effort `low` salvo que se fije otra cosa. Decisión tomada; este plan no la reabre. Base: `perf/latencia` (trae `--model/--effort` de serve, `cfg.AgentModel/AgentEffort`).

## Diseño

- `agent.DefaultModel = "claude-haiku-5-5"`, `agent.DefaultEffort = "low"`, `agent.Inherit = "inherit"`.
- `agent.Effective(name, model, effort)` (puro): el modelo/effort que de verdad se pasa.
  - Agente `claude`: valor vacío -> el por defecto; `inherit` -> vacío (no se pasa bandera: manda Claude Code); otro valor -> tal cual.
  - Otro agente: vacío o `inherit` -> vacío; un valor explícito se queda (para que `Args` siga avisando «no aplicado»).
- Precedencia (sin cambios): bandera > `config.toml` > por defecto.
- Ruling: volver a lo heredado = `--model inherit` / `--effort inherit` o `agent_model = "inherit"` / `agent_effort = "inherit"` — una palabra reservada es descubrible, no necesita un booleano nuevo y vale para modelo y effort por separado; un config vacío no sirve porque «vacío» ya significa «el por defecto» — cuesta: `inherit` deja de poder ser un alias de modelo (no existe como alias de Claude Code).
- `ValidEffort` acepta también `inherit`; `ValidModel` ya lo acepta (cumple la regexp).
- La resolución se hace donde se arranca el agente, por nombre de agente: en el cierre `Args` de `newSession` (el gestor puede arrancar otro agente que `claude`) y en el mensaje de arranque de `serve`. `newSession` sigue recibiendo los valores crudos (bandera/config).
- `--no-agent`: no se aplica ningún por defecto (no se arranca agente); el aviso `serve.model_no_agent` sigue saliendo solo si la persona fijó algo explícitamente (valores crudos distintos de vacío).
- Mensaje de arranque (agente `claude`, sin `--no-agent`): con valores efectivos `serve.model_applied` (`Describe`) dice «El agente arranca con model claude-haiku-5-5, effort low» y, si el valor viene del defecto, se añade «(por defecto de ADBuddy; --model/--effort o agent_model/agent_effort lo cambian, inherit usa el tuyo)». Si ambos son `inherit`: `serve.model_inherited` «El agente arranca con el modelo y esfuerzo que tengas en Claude Code.». Si solo uno es `inherit`, `Describe` nombra el otro y el mensaje añade el heredado no se lista.
- Agente no `claude`: sin cambios (`serve.model_not_applied` solo si se fijó algo).

## Tareas

### T1: `agent.Effective` y constantes (internal/agent/args.go)

```go
const (
	DefaultModel  = "claude-haiku-5-5"
	DefaultEffort = "low"
	// Inherit as a model or effort means "pass nothing": the agent keeps
	// what the person configured in it.
	Inherit = "inherit"
)

func ValidEffort(s string) bool { return s == Inherit || slices.Contains(Efforts, s) }

// Effective is the model and effort to pass to the agent called name, given
// what the person set (flag or config; "" = not set).
func Effective(name, model, effort string) (string, string) {
	if name == "claude" {
		if model == "" {
			model = DefaultModel
		}
		if effort == "" {
			effort = DefaultEffort
		}
	}
	if model == Inherit {
		model = ""
	}
	if effort == Inherit {
		effort = ""
	}
	return model, effort
}
```

Tests (`args_test.go`): `Effective("claude","","")` = haiku,low; `("claude","opus","")` = opus,low; `("claude","inherit","high")` = "",high; `("claude","inherit","inherit")` = "","" ; `("codex","","")` = "",""; `("codex","opus","")` = opus,"" ; `("codex","inherit","inherit")` = "","". `ValidEffort("inherit")` verdadero. Mutación: quitar el relleno por defecto del modelo -> `Effective("claude","","")` en rojo.

### T2: serve usa `Effective` (internal/cli/serve.go, serve_session.go)

`serve_session.go`, cierre `Args`:
```go
Args: func(name string) []string {
	m, e := agent.Effective(name, model, effort)
	a, _ := agent.Args(name, m, e)
	return a
},
```

`serve.go`, mensaje de arranque (rama `else` de `noAgent`) sustituye el bloque `if model != "" || effort != ""`:
```go
m, e := agent.Effective(agentName, model, effort)
switch {
case model != "" || effort != "":
	if _, unused := agent.Args(agentName, m, e); len(unused) > 0 {
		fmt.Fprintln(app.Stderr, app.T("serve.model_not_applied", agentName))
	} else if m == "" && e == "" {
		fmt.Fprintln(app.Stderr, app.T("serve.model_inherited"))
	} else {
		fmt.Fprintln(app.Stderr, app.T("serve.model_applied", agent.Describe(m, e)))
	}
case m != "" || e != "":
	fmt.Fprintln(app.Stderr, app.T("serve.model_default", agent.Describe(m, e)))
}
```
Catálogos en/es: `serve.model_inherited`, `serve.model_default` («The agent starts with %[1]s (ADBuddy's default; --model/--effort or agent_model/agent_effort change it, "inherit" uses your own Claude Code settings).»), ayudas de bandera: `--model` «(default: agent_model in config.toml, else claude-haiku-5-5; "inherit" = the one you set in Claude Code)», `--effort` análogo («default: ..., else low»), actualizar `testdata/help/en/serve.txt` con `-update-help` en el código inglés, y `err.serve.usage_effort_invalid` («... or inherit»). Comentarios de `config.go` líneas 37-38 y `docs/usage.md` línea 44 (+ README si lo menciona: no lo hace) actualizados: «por defecto Haiku 5.5 con effort low para bajar la latencia (medido en `2026-10-03-latencia.md`)».

Tests (`serve_test.go`, con el `startPTY` falso existente que captura los argumentos): sin flags ni config -> args `--model claude-haiku-5-5 --effort low`; `--model opus` -> `--model opus --effort low`; config `agent_model="opus"`, bandera `--model sonnet` -> sonnet; `--model inherit --effort inherit` -> sin bandera alguna y mensaje `model_inherited`; agente `codex` sin flags -> sin banderas y sin mensaje; `--no-agent` sin flags -> sin aviso `model_no_agent`. Mutación: en el cierre `Args` pasar `model, effort` crudos -> el primer test en rojo.

### T3: docs, fragmento y ledger

`docs/usage.md`, fila en `perf-latencia.md`, `CLAUDE.md` del proyecto no se toca (lo gestiona el líder). `go vet ./... && go test ./... -count=1 -timeout 15m` en `host/`.

## Medición (T4, tras T1-T3)

Banco de `2026-10-03-latencia.md` (`ADBUDDY_LATENCY=1 ADBUDDY_LATENCY_AGENT=1 go test ./cmd/adbuddy -run TestLatencyAgentFakePhone`), configuraciones haiku:low, sonnet:medium, opus:medium, 5 tareas, presupuesto total <= 1 $ (`--max-budget-usd` por pasada). Se apunta en `2026-10-03-latencia.md` (sección nueva) con tiempo, turnos, coste y corrección. Si Haiku low va peor se informa al líder sin revertir.

## Correcciones tras la revisión del plan (adbuddy-reviewer; mandan sobre lo de arriba)

1. **Mensaje de arranque como función pura** (en `serve.go`), probada en tabla; sustituye el `switch` de T2:
```go
// startNote is the catalog key and arguments of what serve says about the
// model and effort the agent starts with; key "" says nothing.
func startNote(agentName, model, effort string) (key string, args []any) {
	explicit := func(s string) bool { return s != "" && s != agent.Inherit }
	if agentName != "claude" {
		if explicit(model) || explicit(effort) {
			return "serve.model_not_applied", []any{agentName}
		}
		return "", nil
	}
	m, e := agent.Effective(agentName, model, effort)
	switch {
	case m == "" && e == "":
		return "serve.model_inherited", nil
	case model == "" || effort == "":
		return "serve.model_default", []any{agent.Describe(m, e)}
	}
	return "serve.model_applied", []any{agent.Describe(m, e)}
}
```
   Uso: `if key, a := startNote(agentName, model, effort); key != "" { fmt.Fprintln(app.Stderr, app.T(key, a...)) }`. Un agente no `claude` con `inherit` calla. `model_default` se dice si alguno de los dos viene del defecto (el otro puede ser explícito o `inherit`).
2. **Tests**: tabla de `startNote` (claude y codex × vacío/explícito/inherit para modelo y effort; comprobar clave y argumento) + un test de serve completo solo para el caso por defecto (si el existente `TestServeModelWithNoAgentSaysItIsNotApplied` se puede reutilizar con `--no-agent`, comprobar además que sin flags no sale `model_no_agent`). `TestNewSessionPassesModelAndEffort` se amplía: claude `"",""` -> haiku,low; claude inherit,inherit -> sin banderas; codex `"",""` y codex `opus,""` -> sin banderas. Mutación: quitar `Effective` del cierre `Args` -> rojo el caso `"",""`.
3. **TOML**: en `serve.model_default` escapar `\"inherit\"` (o escribir inherit sin comillas) en en.toml y es.toml.
4. **Doc sin «medido»**: T3 redacta `docs/usage.md:44` entero de nuevo («Por defecto el agente de `serve` usa Claude Haiku 5.5 con effort low, para reducir la latencia; `--model/--effort` o `agent_model/agent_effort` lo cambian y `inherit` vuelve a lo que tengas en Claude Code. La comparación con otros modelos está en `docs/superpowers/plans/2026-10-03-latencia.md`.») sin afirmar cifras; el comentario de `config.go:37-38` igual. T4 (medición) va antes del texto final de la nota en `2026-10-03-latencia.md` y corrige lo que haga falta (`:36-38` «hoy se hereda», `:83` «Nunca Haiku» pasan a decir que Haiku es ahora el defecto por orden de la persona y a enlazar la tabla nueva).
5. **Menores**: `ValidEffort` con comentario nuevo y tests `inherit` verdadero, `INHERIT` falso; config: `agent_model="inherit"`/`agent_effort="inherit"` pasan `Validate` y hacen ida y vuelta (junto a `config_test.go:199-239`); `TestServeModelAndEffortAreChecked` acepta `--effort inherit`; el error de `config.go:123` y `err.serve.usage_effort_invalid` mencionan `inherit`.
6. **Entorno**: T4 empieza con un humo `claude -p hi --model claude-haiku-5-5 --effort low` (que el CLI acepta la combinación). `docs/troubleshooting.md` (si existe): entrada «el agente no arranca tras actualizar (ANTHROPIC_MODEL, Bedrock/Vertex usan otros ids): `adbuddy serve --model inherit`».
