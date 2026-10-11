# UX P6a: CLI `adbuddy` — textos, ayuda y errores de uso — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que el CLI del PC se entienda mejor sin cambiar lo que hace: la ayuda legible (sin los textos de flags rotos por los acentos graves), agrupada y con ejemplo; los errores de uso que apuntan a la ayuda del comando correcto; `help` con un tema que no existe como un error de uso; plurales correctos; `journal` que avisa si recorta y pone la fecha cuando no es de hoy; mensajes de espera y de error más accionables; y una línea al arrancar `serve` que dice, con exactitud, dónde saldrán las confirmaciones 🔴.

**Architecture:** Cambios en `host/internal/cli` y en el catálogo `host/internal/i18n/catalog/{en,es}.toml`, más las pocas líneas de documentación que citan literalmente un texto que cambia; ningún cambio en el daemon, el clasificador, el protocolo ni el comportamiento de ningún comando. Salida de máquinas (`--json`, el agente) en inglés y con los mismos campos (solo aditivos). Cada cambio visible lleva su test (catálogo, ayuda dorada en `testdata/help/{en,es}`, ejecución del CLI con `Main`).

**Tech Stack:** Go (cobra 1.10.2, pflag), catálogo i18n TOML propio (`i18n.EN`, `app.T`, `app.TN(key, n, args...)`, `i18n.M`/`i18n.Msg`).

**Spec:** `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md` (CLI) y `docs/superpowers/plans/2026-10-03-ux-revision.md` (hallazgos H1, H3, H4, H6 en parte, H7 en parte, H8, H9, H10, H11, H12, H15 en parte, H16).

## Global Constraints

- Textos de usuario en inglés (`en.toml`) y español (`es.toml`) con las mismas claves; la salida con `--json` y la del agente (`ADBUDDY_SOURCE=agent`) siguen en inglés. `go test ./...` de `host/` pasa. El catálogo tiene tests que mandan: `TestKeysExist` (`i18n/catalog_test.go`: toda clave se usa con un LITERAL en una llamada reconocida —`T`, `TN`, `i18n.M(...)`—, nunca con una variable; y el número de argumentos de la llamada y de la clave coinciden), la paridad en/es y `textlint_test.go` (solo puede bajar). Las claves nuevas se usan SIEMPRE con literal.
- Ningún cambio de comportamiento: este plan solo toca textos, la forma de la ayuda y avisos informativos. NO se toca `host/internal/{daemon,classify,uidump,screenshot,agent,claudecode,platformtools,confirm,bridge}`. H10 es una línea de texto impresa en `serve` y no cambia qué confirmador se usa.
- Cada tarea compila y pasa sus tests por sí sola; nada usa clases de tareas posteriores.
- Go (desde `host/`): `cd C:/Users/Jose/orca/workspaces/adbuddy/ux/host && go test -timeout 300s ./internal/cli/... ./internal/i18n/...` (el PC Windows no corre `-race`; la CI sí). Un solo `go test` a la vez: el PC anda justo de memoria. Los goldens de la ayuda se reescriben con `go test ./internal/cli -run TestHelp -update-help` y el diff se lee antes de commitear (es la salida visible: `testdata/help/{en,es}/*.txt`; en español solo existen los de `root` y `doctor`).
- Worktree: `C:/Users/Jose/orca/workspaces/adbuddy/ux`, rama `ux/p6` (no cambiar de rama; nunca `git worktree add`). La shell vuelve al checkout principal `C:/Users/Jose/Documents/projects/adbuddy`, que NO se toca: rutas absolutas y `cd <worktree> && ...` en cada comando.
- Commits: identidad hybriqtech, trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` (el hook de commit lo exige). Sin push.
- Rutas relativas a `host/internal/` salvo que se diga otra cosa. No tocar `pendientes-m3-m5.md` ni `2026-10-03-validaciones-manuales.md`: las filas van en `docs/superpowers/fragmentos/ux-revision.md`. La documentación de usuario (`docs/usage.md`, `docs/safety.md`, `docs/setup`, READMEs) solo se edita donde CITA LITERALMENTE un texto que este plan cambia (las líneas se nombran en cada tarea); las frases de seguridad de `docs/usage.md` y `docs/safety.md` sobre `--confirm deny` NO se tocan aquí: van al fragmento (Task 6) para el controlador de integración y `seguridad`.
- Fuera de este plan (P6b, después): H2 (errores de cobra traducidos y sugerencias propias), H5 (`doctor` que imprime según avanza), H13 (colores y QR con `NO_COLOR`/redirección/Windows), H14 (`platformtools`: fuera de nuestro árbol), `Example` de `attach`/`new`/`send`/`stop`/`env`/`version` (H6), `Long` a ≤ 80 columnas con su test de ancho (H7) y los prefijos fijos `⚠ `/`⏳ ` de `serve.go:247-248` (H15).

---

### Task 1: Textos rotos en la ayuda, plurales y estilo de un arreglo (H3, H8, H15 en parte)

Los textos de flags con acentos graves se rompen en `--help` (pflag toma lo de entre acentos graves como el nombre del valor: `--no-agent adbuddy run   do not start…`); el inglés dice "1 phone(s)", "1 damaged journal lines" y "created 1 skills"; y un arreglo y un aviso tienen un estilo distinto del resto.

**Files:**
- Modify: `i18n/catalog/en.toml`, `i18n/catalog/es.toml`, el comentario de `cli/doctor.go:~312` ("the English is the same in both forms", que deja de ser cierto), `docs/setup/index.md:68` y `docs/setup/es/index.md:68` (citan el `setup.no_phone_hint`)
- Modify (tests que fijan los textos que cambian): `cli/envcmd_test.go:~22` ("created 1 skills"), `cli/messages_lang_test.go:~96` ("1 skills"), `cli/remote_test.go:~261` (cita `setup.no_phone_hint`); busca el resto con `grep -rn "phone(s)\|1 skills\|damaged journal lines\|I can't see any phone" host docs`
- Test (nuevo): `cli/help_text_test.go`, `cli/plurals_test.go`
- Regrabar: `cli/testdata/help/en/{tap,journal,run,serve,undo}.txt` (español: ninguno, solo hay goldens de `root` y `doctor`)

**Interfaces:**
- Consumes: `NewRoot(app)`, `helpPaths(root)` y `helpText(t, lang, path)` de `help_golden_test.go`; `app.T(key, args...)`, `app.TN(key, n, args...)` (el número va como SEGUNDO argumento y se antepone a `args`: los `%[1]d` de la clave son `n`; mira `i18n.go:~174`), `i18n.EN`, `i18n.ES`. `doctor.server.ok` se usa con `i18n.M("doctor.server.ok.one"/".other", n, port)` (`doctor.go:~314-316`); para el test, `TN("doctor.server.ok", 1, port)` vale.

- [ ] **Step 1: Tests que fallan**

`cli/help_text_test.go`: (a) recorre cada comando del árbol (`helpPaths(NewRoot(app))`, con `root.Find`) y, para cada `*pflag.Flag` de `c.LocalFlags()` y de `c.InheritedFlags()`, comprueba que `f.Usage` NO contiene el acento grave "`" en inglés ni en español (el fallo dice comando, flag y texto); (b) `helpText(t, i18n.EN, []string{"serve"})` NO contiene el patrón `--no-agent adbuddy run` (el nombre de valor falso que pflag inventa) y sí contiene el texto completo del flag.
`cli/plurals_test.go`, en `i18n.EN` y `i18n.ES`: `app.TN("doctor.server.ok", 1, 5038)` no contiene "phone(s)" ni "phones" (y con 2 sí el plural); `TN("journal.damaged_journal_lines_were_skipped", 1)` no contiene "lines" en inglés ("1 damaged journal line was skipped"); `TN("envcmd.skills_created", 1, "x")` no contiene "skills" y con 3 sí (el `%[1]d` es `n` y el `%[2]s` el primer argumento extra: no pases un argumento de más, daría `%!s(int=1)`); lo mismo con `envcmd.skills_updated`.
Estilo del arreglo: un test sobre las claves `err.*.fix` (no `help.*.flag.fix`, que es la ayuda del flag `--fix`) de `en.toml` y `es.toml` (cárgalas como lo haga `catalog_test.go`): empiezan con mayúscula (o con un verbo en `%[n]s`: se eximen las que empiezan o terminan con un argumento, como `err.serve.device_not_found.fix` y `err.run.agent_not_found.fix`) y terminan en punto (con la misma exención). Hoy solo `err.connect.wrong_phone.fix` falla, en los dos idiomas.

- [ ] **Step 2: Ejecutar: fallan**: `cd C:/Users/Jose/orca/workspaces/adbuddy/ux/host && go test -timeout 300s ./internal/cli -run 'TestHelpText|TestPlurals|TestFixStyle' -v`.

- [ ] **Step 3: Implementar**

- H3: quitar los acentos graves de los cinco textos de flag (`help.tap.flag.id`, `help.journal.flag.workspace`, `help.run.flag.workspace`, `help.serve.flag.no_agent`, `help.undo.flag.session`) en `en.toml` y `es.toml`: comillas simples (`'adbuddy screen'`) o reescritura sin comillas, lo más legible. Y `help.serve.flag.confirm`: que diga la verdad (ver Task 5: con el móvil conectado las confirmaciones SIEMPRE van a la app y `--confirm` solo decide qué pasa cuando la app NO está conectada: `auto` = terminal si stdin lo es y, si no, se deniega; `terminal` = por la entrada estándar; `deny` = se deniega): "where to confirm 🔴 commands when the phone app is not connected: auto (this terminal if there is one, else deny), terminal or deny" y su equivalente español (con "por defecto" si el catálogo lo traduce; si es pflag quien añade `(default "auto")` con su texto fijo, déjalo y anótalo en el informe). Lee el texto actual de ambos idiomas antes de reescribirlo.
- H8: en `en.toml`, el `.one` de `doctor.server.ok` ("port %[2]d, %[1]d phone connected"; el `.other` "phones"), de `journal.damaged_journal_lines_were_skipped` ("⚠ %[1]d damaged journal line was skipped."), de `envcmd.skills_created` ("  created %[1]d skill in %[2]s") y de `envcmd.skills_updated` ("  updated %[1]d skill in %[2]s"); en `es.toml`, lo mismo para cualquier `.one` cuyo texto sea idéntico al `.other` y cuente con un número (revisa esas cuatro claves y las demás `.one`). Actualiza el comentario de `doctor.go:~312`.
- H15 (en parte): `err.connect.wrong_phone.fix` con mayúscula inicial y punto final en en y es; `setup.no_phone_hint` en primera persona ("I can't see…" / "No veo…") pasa a impersonal ("No phone with wireless debugging is visible on this network. If the phone is on another network (for example Tailscale) or this Wi-Fi blocks mDNS, press Ctrl+C and run `adbuddy setup --code`." / "No se ve ningún móvil con la depuración inalámbrica en esta red. Si el móvil está en otra red (por ejemplo Tailscale) o esta Wi-Fi bloquea mDNS, pulsa Ctrl+C y ejecuta `adbuddy setup --code`."); actualizar las dos líneas de `docs/setup/{,es/}index.md:68` y los tests que citen el texto viejo (los de arriba).
- Regrabar los goldens de ayuda de los cinco comandos (solo inglés) y mirar el diff: solo cambian las líneas de esos flags.

- [ ] **Step 4: Ejecutar, regrabar, mirar**: `go test -timeout 300s ./internal/cli ./internal/i18n/...` → PASS tras `go test ./internal/cli -run TestHelp -update-help`; `git diff --stat host/internal/cli/testdata` solo toca los cinco comandos en inglés; `go vet ./internal/cli`; `go test ./internal/... -run 'TestDocs'` si el repo valida los docs (`docs_test.go`).

- [ ] **Step 5: Commit**

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add host docs/setup && git commit -m "fix(cli): textos de flags sin acentos graves, plurales ingleses y estilo de un arreglo

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Errores de uso que apuntan a la ayuda del comando, `help nope` y `--json` documentado (H1, H4, H16)

Todo error de uso dice "Run `adbuddy --help`…" aunque el usuario se haya equivocado en `tap`; `adbuddy help nope` sale con 0 y vuelca la ayuda; y la ayuda lista `--json` como global en comandos que no lo soportan.

**Files:**
- Modify: `cli/errors.go` (campo `fixArgs`, `in()` y `msgs()` lo usan, y `forCommand`), `cli/root.go` (`mainContext` con `ExecuteContextC`; el comando `help` propio), `i18n/catalog/{en,es}.toml` (`error.usage_fix_cmd`, `err.help.unknown_topic`, `help.root.flag.json`)
- Test: ampliar `cli/errors_lang_test.go`, crear `cli/help_unknown_test.go`
- Regrabar: TODOS los goldens `cli/testdata/help/en/*.txt` (28; la línea de `--json`, que en la raíz está en "Flags:" y en el resto en "Global Flags:") y los 2 de español (`root`, `doctor`); más `completion_*`

**Interfaces:**
- Produces: el campo `fixArgs []any` de `Error` (si no es nil, `in()` y `msgs()` lo usan para el `Fix` en lugar de `args`), `func (e *Error) forCommand(path string) *Error` y la clave `error.usage_fix_cmd` ("Run `%[1]s --help` to see the usage." / "Ejecuta `%[1]s --help` para ver cómo se usa." —el español sigue a `error.usage_fix`, que dice "para ver cómo se usa"—).
- Consumes: `root.ExecuteContextC(ctx)` de cobra 1.10.2 (devuelve el subcomando para un error de flags o de argumentos, y la raíz para un comando desconocido), `cmd.CommandPath()`; `WriteFailureIn(stdout, stderr, json, lang, err error)` (recibe `error`: no cambia su firma), `*exitCodeError` y `ExitCode(err)`.

- [ ] **Step 1: Tests que fallan**

En `cli/errors_lang_test.go` (con `runCLI`, `i18ntest.Lang` y `isolate(t)`): `runCLI(t, "tap")` (sin argumentos: error de uso desde su `RunE`) sale con 2 y su stderr contiene "Run `adbuddy tap --help`" en inglés y "adbuddy tap --help" en español (`i18ntest.Lang(t, i18n.ES)`); `runCLI(t, "journal", "--bad")` apunta a `adbuddy journal --help`; `runCLI(t, "serv")` (comando desconocido) sigue apuntando a `adbuddy --help`; `runCLI(t, "tap", "--json")` (error de uso con `--json`): el campo `"fix"` del envelope lleva "adbuddy tap --help" (en inglés) y el resto de campos no cambia. CONTRATO que no debe romperse (tests nuevos o ya existentes, confírmalo): `adbuddy run` con un agente que sale con código 3 sigue saliendo con 3 y SIN `Error: INTERNAL — exit status` (mira `run.go` y su test); `doctor --json` con fallos sigue escribiendo UN solo envelope.
`cli/help_unknown_test.go`: `runCLI(t, "help", "nope")` sale con 2 (no 0), escribe en stderr `Error: USAGE — ` y "nope", no escribe la ayuda de la raíz en stdout, y su arreglo dice `adbuddy --help` (no `adbuddy help --help`); con `ADBUDDY_LANG=es` el mensaje va en español; `runCLI(t, "help", "tap")` sigue escribiendo la ayuda de `tap` con 0; `runCLI(t, "help")` escribe la de la raíz.

- [ ] **Step 2: Ejecutar: fallan**: `go test -timeout 300s ./internal/cli -run 'TestAnErrorPoints|TestHelpUnknown' -v`.

- [ ] **Step 3: Implementar**

`errors.go`: añadir `fixArgs []any` a `Error`; en `in()` (y en `msgs()`, `errors.go:~165`) el `Fix` usa `fixArgs` si no es nil; y, con la clave LITERAL (no `c.fixKey`: `TestKeysExist` solo reconoce literales):

```go
// forCommand points a usage error at the help of the command that failed, not at the root's.
func (e *Error) forCommand(path string) *Error {
	if e.fixKey != "error.usage_fix" || path == "" {
		return e
	}
	c := *e
	c.fixKey = "error.usage_fix_cmd"
	c.fixArgs = []any{path}
	c.Fix = i18n.EN.T("error.usage_fix_cmd", path)
	return &c
}
```

`root.go`, `mainContext`: NO convertas con `AsError` un `*exitCodeError` (lo devuelven `run`, `doctor`, `undo`, y `AsError` lo volvería `INTERNAL "exit status"` con salida 1):

```go
cmd, err := root.ExecuteContextC(ctx)
if err != nil {
	var code *exitCodeError
	if !errors.As(err, &code) && cmd != nil {
		name := cmd.CommandPath()
		// help's own usage error points at the root, not at `adbuddy help --help`.
		if cmd.Name() == "help" && cmd.Parent() == root {
			name = root.CommandPath()
		}
		err = AsError(err).forCommand(name)
	}
	WriteFailureIn(stdout, stderr, app.JSON, app.speaks(), err)
}
return ExitCode(err)
```

El `help` propio, en `NewRoot` antes de `localizeHelp` (con el `ValidArgsFunction` de cobra, que el comando propio pierde: cópialo de `command.go` `InitDefaultHelpCmd` para no romper `adbuddy help <Tab>`):

```go
root.SetHelpCommand(&cobra.Command{
	Use: "help [command]",
	ValidArgsFunction: func(c *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		/* igual que el de cobra: los subcomandos del que se nombra */
	},
	RunE: func(c *cobra.Command, args []string) error {
		target, _, err := c.Root().Find(args)
		if target == nil || err != nil {
			return UsageErrorT("err.help.unknown_topic", strings.Join(args, " "))
		}
		target.InitDefaultHelpFlag()
		target.InitDefaultVersionFlag()
		return target.Help()
	},
})
```

(`localizeHelp` ya pone `Short` y `Long` del `help` de la raíz por nombre: comprueba que lo sigue haciendo con el comando nuevo y que los goldens de `help` no cambian salvo la línea de `--json`.) Catálogo: `err.help.unknown_topic` ("unknown help topic %[1]q" / "tema de ayuda desconocido %[1]q"), `error.usage_fix_cmd` (arriba), y `help.root.flag.json` pasa a "machine-readable JSON output, for the commands that support it" y su equivalente español (H16; sin lista de comandos). Regrabar los goldens y leer el diff entero: solo cambia esa línea.

- [ ] **Step 4: Ejecutar, regrabar, mirar**: `go test -timeout 300s ./internal/cli ./internal/i18n/...` → PASS tras `-update-help`; `go vet ./internal/cli`; `grep -rn "to see the usage" docs host --include=*.md --include=*_test.go` por si alguna documentación o test cita el texto («Run `adbuddy --help` to see the usage.» sigue siendo el de los errores de comando desconocido).

- [ ] **Step 5: Commit**

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add host && git commit -m "fix(cli): el error de uso apunta a la ayuda del comando, help de un tema inexistente es un error y --json se describe como de maquinas

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Ayuda agrupada, con ejemplo y con descripciones cortas (H6 en parte, H7 en parte)

La ayuda raíz lista los comandos en orden alfabético, con `setup`, `serve` y `doctor` perdidos entre los del agente, sin ejemplos, y muchas descripciones no caben en 80 columnas.

**Files:**
- Modify: `cli/root.go` (grupos y `Example` de la raíz), `GroupID` de cada comando (en `root.go` tras `AddCommand`, lo que deje menos cambios), `i18n/catalog/{en,es}.toml` (títulos de grupo terminados en ":", ejemplo raíz y los `*.short` que sobrepasen 60 caracteres), `README.md:~104-123` y `README.es.md` (copian los `Short` literalmente bajo «These are the commands of `adbuddy --help`»)
- Test: `cli/help_groups_test.go` (nuevo)
- Regrabar: `cli/testdata/help/en/root.txt` y `es/root.txt` (la raíz cambia entera) y los de los comandos cuyo `Short` cambie (`attach`, `new`, `connect`, `doctor`, `env`, `env_init`, `screen`, `tap`, `type`, `notifications`, `remote`, `send`, `serve`, `setup` y, en español, `stop`, `foreground`)

**Interfaces:**
- Produces: tres grupos de comandos en la raíz con sus títulos en el catálogo (`help.group.start`, `help.group.daily`, `help.group.agent`), con los títulos terminados en ":" como los demás encabezados de cobra: "Getting started:" (`setup`, `doctor`, `serve`), "Day to day:" (`run`, `attach`, `new`, `stop`, `journal`, `undo`, `connect`, `remote`, `env`, `version`) y "For the agent:" (`screen`, `tap`, `type`, `screenshot`, `foreground`, `notifications`, `send`); en español "Primeros pasos:", "Día a día:", "Para el agente:". `hook` sigue oculto; `help` y `completion` quedan en "Additional Commands" como hoy. Cobra ordena alfabéticamente dentro de cada grupo (saldrá doctor, serve, setup): vale así.
- Produces: `Example` de la raíz (`help.root.example`): `adbuddy setup` y `adbuddy serve` con una línea de contexto cada uno, en los dos idiomas.
- Consumes: `cobra.Group`, `root.AddGroup(...)`, `cmd.GroupID` (cobra 1.10.2 los tiene).

- [ ] **Step 1: Tests que fallan** (`cli/help_groups_test.go`)
1. La ayuda raíz en inglés y en español contiene los tres títulos de grupo en este orden y, bajo cada uno, exactamente los comandos de arriba (el texto de `helpText(t, lang, nil)`: busca cada nombre como línea que empieza por dos espacios, el nombre y un espacio, `^  <nombre>\s`, porque "new", "run", "type" y "send" aparecen dentro de otras descripciones); ningún comando VISIBLE (salta `Hidden` y `__complete*`) queda sin grupo salvo `help` y `completion`.
2. La ayuda raíz contiene el ejemplo (`adbuddy setup` y `adbuddy serve`).
3. (H7) Para todo comando visible del árbol y salvo la raíz y los de cobra (`help`, `completion` y sus subcomandos, que tienen texto fijo), en inglés y en español, `utf8.RuneCountInString(c.Short) <= 60`; el fallo lista comando, idioma, longitud y texto. Excepción comentada: `help.root.short` (69 en inglés y 90 en español; es el título de la aplicación, no una fila de la lista).
4. Los goldens de la ayuda pasan tras regrabarlos.

- [ ] **Step 2: Ejecutar: fallan**: `go test -timeout 300s ./internal/cli -run 'TestHelpGroups' -v`.

- [ ] **Step 3: Implementar**

`root.AddGroup(&cobra.Group{ID: "start", Title: app.T("help.group.start")}, &cobra.Group{ID: "daily", ...}, &cobra.Group{ID: "agent", ...})` antes de los `AddCommand`, y `GroupID` en cada comando; `root.Example = app.T("help.root.example")`. Reescribir los `*.short` que pasen de 60 caracteres (la lista sale del test: 13 en inglés —attach, new, connect, doctor, env init, screen, tap, type, notifications, remote, send, serve, setup— y 15 en español, además de stop y foreground), manteniendo el sentido y subiendo al `Long` lo que el `Short` decía y el `Long` no (no pierdas información). Actualizar en `README.md` y `README.es.md` la lista que copia los `Short`. Regrabar los goldens y leer el diff entero de la raíz y de los comandos con `Short` nuevo.

- [ ] **Step 4: Ejecutar, regrabar, mirar**: `go test -timeout 300s ./internal/cli ./internal/i18n/...` → PASS tras `-update-help`; leer `testdata/help/en/root.txt` y `es/root.txt` a 80 columnas; `go vet ./internal/cli`; los tests de documentación (`docs_test.go`) siguen pasando.

- [ ] **Step 5: Commit**

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add host README.md README.es.md && git commit -m "feat(cli): ayuda raiz agrupada en tres bloques con ejemplo y descripciones de comando que caben en 80 columnas

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: `journal` que avisa y pone fecha; esperas y `doctor` más accionables (H9, H11, H12)

`journal` corta a 50 entradas sin decirlo y sin fecha; el mensaje de espera de `serve` habla de "adb server (port 5038)"; el error de `setup --code` sin terminal lleva el arreglo genérico; y `doctor` no ofrece `adbuddy setup` cuando el móvil configurado no conecta.

**Files:**
- Modify: `cli/journal.go`, `cli/common.go` (`formatEntryIn`: la fecha), `cli/serve.go` (`serve.waiting_for_the_phone_on`: el texto nuevo CONSERVA el argumento, ver abajo), `cli/setup.go:~123` (el error de `ask` sin respuesta en stdin), `i18n/catalog/{en,es}.toml`, `docs/usage.md:~29` (cita literal «Waiting for the phone on ADBuddy's adb server (port 5038)…»)
- Test: ampliar `cli/serve_test.go` (los tests de `journal` están ahí: `appendEntries`, `isolate`, `--workspace`; `cli/journal_test.go` no existe; ajustar su aserción de `--json` `{"ok":true,"data":{"entries":[],"skippedLines":0}}` de la línea ~57, que pasa a llevar `total` y `truncated` en el orden en que `WriteData` serializa el mapa: alfabético), `cli/messages_lang_test.go` (`formatEntryAt`), `cli/doctor_lang_test.go`, el test de `setup` sin terminal

**Interfaces:**
- Produces: `formatEntryAt(lang, j, now time.Time)` (privada) que `formatEntryIn(lang, j)` llama con `time.Now()`: la hora corta (`15:04:05`) si la entrada es de HOY respecto de `now` (compara `j.TS.In(now.Location())` con `now`) y `2006-01-02 15:04:05` si no. (`TestFormatEntryInSpanish`, `messages_lang_test.go:~28`, usa una fecha fija con `Contains`: sigue pasando; si algún test compara la línea entera con una fecha antigua, se ajusta con `now` inyectado.)
- Produces: en `--json`, `journal` añade `"total"` (entradas que cumplían el filtro antes de recortar) y `"truncated"` (bool) a `{"entries", "skippedLines"}`; aditivo.
- Produces las claves (en/es): `journal.showing_last_of` ("Showing the last %[1]d of %[2]d entries (use --limit 0 for all)." / "Mostrando las últimas %[1]d de %[2]d entradas (usa --limit 0 para verlas todas)."), el texto nuevo de `serve.waiting_for_the_phone_on` con el MISMO número de argumentos que la clave actual (un `%[1]d`): "Waiting for the phone (up to %[1]d s). Is Wireless debugging on, and are the phone and this PC on the same Wi-Fi?" / "Esperando al móvil (hasta %[1]d s). ¿La depuración inalámbrica está activada y el móvil y este PC están en la misma Wi-Fi?", y la llamada pasa `int(deviceWait/time.Second)` (`deviceWait` es una variable de `serve.go:~39`: escribe el valor real, no 45 a mano; `TestKeysExist` exige que llamada y clave cuenten los mismos argumentos); `err.setup.usage_no_answer_on_stdin.fix` ("Run it in a terminal, or give the answers on stdin, one per line." / "Ejecútalo en una terminal o pásale las respuestas por stdin, una por línea."; `ask` también se usa para la dirección de conexión y para elegir agente, por eso «las respuestas»); y la ampliación de `doctor.phone.how_wifi` y `doctor.phone.how_tailscale` con la frase "If you unpaired it or this is another PC, run `adbuddy setup`." / "Si lo desemparejaste o es otro PC, ejecuta `adbuddy setup`.".

- [ ] **Step 1: Tests que fallan**
- `journal` con 60 entradas sin `Inverse` ni `Reason` (esos campos añaden líneas a `formatEntryIn`) y `--limit 50`: stdout tiene 50 líneas, stderr contiene "Showing the last 50 of 60 entries (use --limit 0 for all)."; con `--limit 0`, 60 líneas y sin aviso; con `--json`: `"total":60` y `"truncated":true` (y `false` sin recorte); en español el aviso va en español.
- `formatEntryAt`: una entrada de hoy muestra solo la hora; la de otro día, `YYYY-MM-DD HH:MM:SS`; con `now` en una zona fija (`time.FixedZone`), independiente de la zona del PC (no uses `t.Setenv("TZ")`: `time.Local` se fija al arrancar el proceso y en Windows `TZ` no se usa).
- `serve`: `app.T("serve.waiting_for_the_phone_on", 45)` no contiene "adb server" ni "5038" y sí "45 s", en `en` y `es`; y una prueba de que la llamada real pasa los segundos de `deviceWait`.
- `setup --code` sin terminal: el error es un error de USO (exit 2, `Error: USAGE — ...`) con el arreglo propio; no el genérico de `--help`. Se construye con `errorT(ExitUsage, false, "USAGE", "err.setup.usage_no_answer_on_stdin", "err.setup.usage_no_answer_on_stdin.fix", nil)` (no `UsageErrorT`, que fija `error.usage_fix`, ni `NewErrorT`, que cambiaría el código de salida).
- `doctor`: `doctor.phone.how_wifi` y `how_tailscale` contienen "adbuddy setup" en inglés y español.

- [ ] **Step 2: Ejecutar: fallan**: `go test -timeout 300s ./internal/cli -run 'TestJournal|TestFormatEntry|TestServeWaiting|TestSetupCode|TestDoctor' -v`.

- [ ] **Step 3: Implementar** como arriba: en `journal.go`, `total := len(out)` antes de recortar, `truncated := limit > 0 && total > limit`, aviso por `app.Stderr` si `truncated && !app.JSON`, y `WriteData` con `"total"` y `"truncated"` además de `entries` y `skippedLines`; `formatEntryAt` en `common.go`; catálogo; la cita de `docs/usage.md:~29`.

- [ ] **Step 4: Ejecutar y mirar**: `go test -timeout 300s ./internal/cli ./internal/i18n/...` → PASS; `go vet ./internal/cli`; los goldens de ayuda no cambian (si cambia alguno, no es de esta tarea: para y cuéntalo).

- [ ] **Step 5: Commit**

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add host docs/usage.md && git commit -m "feat(cli): journal avisa si recorta y pone fecha, esperas y doctor mas accionables

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: `serve` dice al arrancar dónde saldrán las confirmaciones 🔴 (H10)

En modo `auto` sin terminal, `serve` usa `DenyAll` sin avisar y quien lo lanza desde un script o un servicio descubre que todo 🔴 se deniega solo cuando el agente falla. Solo texto: una línea al arrancar y la corrección de los cinco avisos que hoy dicen "aquí" aunque no sea verdad. NO cambia qué confirmador se usa ni el comportamiento; lo revisa `adbuddy-reviewer-opus`.

**Lo que hace el código de verdad** (verificado en `phone.go:~202`, `bridge/confirm.go:~86-87` y `confirm/confirm.go`): `startPhoneLink` SIEMPRE envuelve el confirmador en `bridge.NewConfirmer(srv, fallback)`, sea cual sea `--confirm`: con la app conectada, la 🔴 se pregunta EN LA APP; solo si no lo está (o se desconecta con la pregunta pendiente) se usa el confirmador de respaldo:

| `--confirm` | stdin | app conectada | app no conectada |
|---|---|---|---|
| auto | terminal | pregunta en la app | código de dos cifras en la terminal |
| auto | no terminal | pregunta en la app | `DenyAll`: se deniega |
| terminal | terminal | pregunta en la app | terminal |
| terminal | no terminal (tubería) | pregunta en la app | se lee de la tubería; al acabarse, se deniega |
| deny | cualquiera | **pregunta en la app** | `DenyAll` |

Por eso «`--confirm deny` deniega todo» es FALSO con el móvil conectado. Los textos describen exactamente esto (el implementador lo vuelve a comprobar leyendo esos tres ficheros antes de escribirlos).

**Files:**
- Modify: `cli/serve.go` (la nota, justo ANTES de la línea de `serve.adbuddy_is_serving_session_journal`, que imprime el mensaje con "Press Ctrl+C to stop" en el mismo texto; y `interactive := isTerminal(app.Stdin)` calculado UNA vez y usado a la vez en el `switch confirmMode` y en la nota), `i18n/catalog/{en,es}.toml` (las cinco claves nuevas y la corrección de los cinco avisos `phone.*` de abajo), los tests que fijen esas frases (`grep -rn "happen here" host/internal/cli/*_test.go`)
- Test: ampliar `cli/serve_test.go` (o el test que arranca `serve` con un adb falso: mira `serve_test.go`/`invariant_test.go`, que cancelan al ver «ADBuddy is serving»)

**Interfaces:**
- Produces: `func confirmersNote(mode string, interactive bool) i18n.Msg` (privada, pura) que devuelve `i18n.M("<clave literal>")` por caso (con LITERALES en cada rama: `TestKeysExist` solo reconoce literales en llamadas a `T`, `TN` e `i18n.M`), y se imprime con `confirmersNote(mode, interactive).In(app.speaks())` (`Msg.In` existe en `i18n.go`; mira su firma). Cinco claves (en/es, mismo orden):
  - `serve.confirmations_auto_terminal` y `serve.confirmations_terminal_terminal` (el mismo texto): "🔴 confirmations: in the ADBuddy app while it is connected; otherwise in this terminal." / "Confirmaciones 🔴: en la app de ADBuddy mientras esté conectada; si no, en esta terminal."
  - `serve.confirmations_auto_no_terminal`: "⚠ 🔴 confirmations: in the ADBuddy app while it is connected; otherwise every one is denied, because stdin is not a terminal (use --confirm terminal to answer on stdin)." / "⚠ Confirmaciones 🔴: en la app de ADBuddy mientras esté conectada; si no, se deniegan todas, porque la entrada estándar no es una terminal (usa --confirm terminal para responder por ella)."
  - `serve.confirmations_terminal_pipe`: "🔴 confirmations: in the ADBuddy app while it is connected; otherwise answered on stdin (not a terminal); once stdin ends, every one is denied." / "Confirmaciones 🔴: en la app de ADBuddy mientras esté conectada; si no, se responden por la entrada estándar (no es una terminal); cuando se acaba, se deniegan todas."
  - `serve.confirmations_deny`: "🔴 confirmations: in the ADBuddy app while it is connected; otherwise every one is denied (--confirm deny)." / "Confirmaciones 🔴: en la app de ADBuddy mientras esté conectada; si no, se deniegan todas (--confirm deny)."
  («mientras esté conectada» es verdad aunque la app no esté instalada: nunca se conecta.)
- Corrige los cinco avisos existentes que dicen que las confirmaciones «happen here» en cualquier modo (`phone.confirmations_happen_here_without_the`, `phone.could_not_check_whether_the`, `phone.the_adbuddy_app_is_not`, `phone.the_phone_app_disconnected_confirmations`, `phone.could_not_reach_the_phone`; `en.toml:~659-669`): "happen here" pasa a "will not reach the phone" y "no llegarán al móvil" (o la redacción equivalente que no afirme DÓNDE ocurren: eso lo dice la nota de arranque); ajusta los tests que fijen las frases.

- [ ] **Step 1: Tests que fallan**: tabla de `confirmersNote(mode, interactive).Key` para `("auto", true)` = `serve.confirmations_auto_terminal`, `("auto", false)` = `serve.confirmations_auto_no_terminal`, `("terminal", true)` = `serve.confirmations_terminal_terminal`, `("terminal", false)` = `serve.confirmations_terminal_pipe`, `("deny", true)` y `("deny", false)` = `serve.confirmations_deny`; los cinco textos en inglés NO afirman que se deniegue «todo» sin matizar la app (`--confirm deny` menciona «while it is connected»); un test que arranca `serve` como los existentes y comprueba que stderr contiene la línea del confirmador en inglés (y en español con `i18ntest.Lang`) y que sale ANTES de la de «ADBuddy is serving»; y que los cinco avisos `phone.*` ya no dicen «happen here».
- [ ] **Step 2: Ejecutar: fallan**: `go test -timeout 300s ./internal/cli -run 'TestConfirmersNote|TestServeSays' -v`.
- [ ] **Step 3: Implementar** `confirmersNote` y la línea con los textos de arriba, `interactive` una sola vez, y los avisos corregidos.
- [ ] **Step 4: Ejecutar y mirar**: `go test -timeout 300s ./internal/cli ./internal/i18n/...` → PASS; `go vet ./internal/cli`; ningún golden de ayuda cambia; `git diff --stat host/internal` no toca nada fuera de `cli` y del catálogo.
- [ ] **Step 5: Commit**

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add host && git commit -m "feat(cli): serve dice al arrancar donde saldran las confirmaciones rojas

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: Cierre del paquete

**Files:**
- Modify: `docs/superpowers/fragmentos/ux-revision.md` (nueva sección "P6a")

- [ ] **Step 1: Suite completa de `host/`**: `cd C:/Users/Jose/orca/workspaces/adbuddy/ux/host && go vet ./... && go test -timeout 600s ./...` → PASS (si algún test no corre en Windows y ya fallaba antes de P6a, di cuál y comprueba que falla igual en `main`).
- [ ] **Step 2: Fragmento**: sección "P6a · CLI: textos, ayuda y errores de uso": qué cambió por tarea; goldens de ayuda regrabados (lista); que `--json` solo gana campos (`total`, `truncated`) y el `fix` de un error de uso (que ahora nombra el comando); aplazado con motivo (el párrafo «Fuera de este plan» del plan, tal cual: H2, H5, H13 a P6b; H14 `platformtools` fuera de nuestro árbol; `Example` de `attach`/`new`/`send`/`stop`/`env`/`version`; `Long` a ≤ 80 con test de ancho; prefijos `⚠ `/`⏳ `); notas honestas (H10 es solo texto y sigue lo que el código hace; la fecha del journal es ISO, no localizada; el `Short` de ciertos comandos perdió detalle que sube al `Long`; `adbuddy help <Tab>` conserva su autocompletado); y PARA EL CONTROLADOR DE INTEGRACIÓN Y `seguridad` (no se tocan aquí): `docs/usage.md:~107` dice «`--confirm deny` refuses every red command without asking» y `docs/safety.md:~84` va en la misma línea, pero con el móvil conectado `--confirm deny` SÍ pregunta en la app (solo deniega cuando la app no está conectada); y los cinco avisos `phone.*` ya corregidos.
- [ ] **Step 3: Commit** (`docs: fragmento del paquete P6a de UX`, con el trailer exacto).
