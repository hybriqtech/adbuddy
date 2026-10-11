# UX P6b: CLI `adbuddy` — errores de cobra traducidos, `doctor` que avanza, QR y ejemplos — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. **No abrir PR hasta que el líder avise** (P6a, #79, aún no está en `main`); la rama `ux/p6b` parte de `ux/p6`.

**Goal:** Lo que quedó de la revisión del CLI: los errores de uso de cobra (que salen en inglés con el resto en español y con sugerencias pobres) pasan a un texto propio en el idioma de la persona con la mejor sugerencia; `doctor` imprime cada comprobación según termina, en vez de callar hasta 60 s; el QR de `setup` se ve bien y es escaneable con la salida redirigida y en una consola de Windows; y los comandos sin ejemplo (`attach`, `new`, `send`, `stop`, `env`, `version`) lo ganan.

**Architecture:** Cambios en `host/internal/cli` (`errors.go`/`root.go` para H2, `doctor.go` para H5, `setup.go`, `attach_windows.go`/`attach_unix.go` para H13, los constructores de los seis comandos para los ejemplos) y en el catálogo `host/internal/i18n/catalog/{en,es}.toml`. Ningún cambio de comportamiento de los comandos ni del daemon, el clasificador o el protocolo. El `message` de un error de uso en `--json` pasa del texto crudo de cobra a nuestro inglés (ruling abajo); sus campos, código, `fix` y exit no cambian. Cada cambio con su test.

**Tech Stack:** Go (cobra 1.10.2, pflag 1.0.9), catálogo i18n TOML propio, `rsc.io/qr`, `golang.org/x/sys/windows`.

**Spec:** `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md` (CLI) y `docs/superpowers/plans/2026-10-03-ux-revision.md` (hallazgos H2, H5, H6 en parte, H13; H14 `platformtools` sigue fuera de nuestro árbol).

## Global Constraints

- Mismas reglas que P6a (`docs/superpowers/plans/2026-10-04-ux-p6a-cli-textos-y-ayuda.md`, «Global Constraints»): textos en inglés y español con las mismas claves; salida `--json` y del agente en inglés; claves del catálogo usadas SIEMPRE con LITERAL en una llamada reconocida (`T`, `TN`, `i18n.M`, `UsageErrorT`, `NewErrorT`...: `TestKeysExist`, `i18n/catalog_test.go` ~250 y ~471-549, rechaza una clave en una variable local; y el mismo número de argumentos en llamada y clave); paridad en/es; `textlint_test.go` solo baja; ningún cambio de comportamiento de los comandos; NO se toca `host/internal/{daemon,classify,uidump,screenshot,agent,claudecode,platformtools,confirm,bridge}` ni `host/internal/pairing` (el QR solo cambia en `cli`, no en su dibujo); las rutas son relativas a `host/internal/` salvo que se diga otra cosa.
- Go (desde `host/`): `cd C:/Users/Jose/orca/workspaces/adbuddy/ux/host && go test -timeout 300s ./internal/cli/... ./internal/i18n/...`. Un solo `go test` a la vez (el PC anda justo de memoria; sin `-race`). Los goldens de ayuda se reescriben con `go test ./internal/cli -run TestHelp -update-help` y se lee el diff.
- Worktree: `C:/Users/Jose/orca/workspaces/adbuddy/ux`, rama `ux/p6b` (no cambiar de rama; nunca `git worktree add`); la shell vuelve al checkout principal `C:/Users/Jose/Documents/projects/adbuddy`, que NO se toca: rutas absolutas y `cd <worktree> && ...`.
- Commits: identidad hybriqtech, trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Sin push.
- Las filas para el integrador, en `docs/superpowers/fragmentos/ux-p6b.md` (fichero propio de esta rama). No tocar `pendientes-m3-m5.md` ni `2026-10-03-validaciones-manuales.md`.
- **Rulings de este plan** (van al ledger y al fragmento):
  - El `message` de un error de uso en `--json` pasa del texto crudo de cobra (que a veces lleva `\n\nDid you mean this?\n\tenv…`) a nuestro inglés; los campos, el `code` (`USAGE`), el `fix` y el exit (2) no cambian. Nada del repo cita esos textos (comprobado con `grep`).
  - H13: el QR de emparejamiento se dibuja negro sobre blanco con escapes SGR solo cuando stderr es una terminal y `TERM != dumb`; `NO_COLOR` NO lo desactiva (el estándar habla de color decorativo y el QR es funcional: sin fondo blanco, en un terminal oscuro sale invertido y muchos lectores fallan; la alternativa de invertir los bloques falla en terminales claros). Las válvulas que ya existen: `TERM=dumb` o redirigir la salida dan un QR plano sin escapes, y `adbuddy setup --code` empareja sin QR. Con la salida redirigida solo se imprime la URL de ayuda (`HelpURL`, como hoy); el payload del QR de emparejamiento lleva el secreto y NUNCA se imprime en texto.
  - H13, iconos de `doctor` (`✓ ⚠ ✗ ·`) y un QR en ASCII con `TERM=dumb`: se aplazan —cada consola moderna, incluida la de Windows 10+ que exige Go ≥ 1.21, dibuja esos glifos y el ASCII del QR exigiría otro dibujo—; coste: en una terminal sin Unicode los iconos salen como `?`.
  - H5: no se añade una línea de progreso en stderr («Comprobando… 3/9»): el streaming por comprobación ya da el latido y la línea de progreso ensuciaría stderr en los scripts; coste: durante una comprobación lenta no hay señal intermedia más allá de las líneas anteriores.
- Fuera de este plan: `Long` a ≤ 80 columnas, los prefijos `⚠ `/`⏳ ` de `serve.go` (son símbolos), H14.

---

### Task 1: Errores de uso de cobra en el idioma de la persona, con la mejor sugerencia (H2)

Con `ADBUDDY_LANG=es`, `adbuddy connect` da `Error: USAGE — accepts 1 arg(s), received 0`, `adbuddy journal --bad` da `unknown flag: --bad` y `adbuddy serv`, `unknown command "serv"… Did you mean this? env send serve`. El mensaje queda en inglés y la sugerencia propone comandos que no se parecen.

**Qué produce cobra 1.10.2 / pflag 1.0.9** (verificado en `args.go`, `command.go`, `pflag/errors.go`):
- Comando desconocido: `unknown command "X" for "Y"` (con `Did you mean this?\n\t…` si hay sugerencias). También sale de `cobra.NoArgs` en un subcomando: `unknown command "foo" for "adbuddy journal"` (y NO es un comando desconocido de la raíz).
- Argumentos: `accepts %d arg(s), received %d` (`ExactArgs`), `accepts at most %d arg(s), received %d` (`MaximumNArgs`: lo usan `new`, `undo` y `tap`), `accepts between … arg(s)`/`requires at least …` (ningún comando los usa hoy).
- Flags (pflag, ya tipados): `*pflag.NotExistError` (`unknown flag: --x` / `unknown shorthand flag: 'x' in -x`), `*pflag.ValueRequiredError` (`flag needs an argument: --x`), `*pflag.InvalidValueError` (`invalid argument "v" for "-o, --output" flag: …`, con el nombre largo Y el corto), `*pflag.InvalidSyntaxError` (`bad flag syntax: %s`; hoy cae en `INTERNAL` con exit 1: debe ser uso, exit 2).

**Files:**
- Modify: `cli/errors.go` (`translateFlagError`, `translateArgsError`, `suggestCommand`), `cli/root.go` (`SetFlagErrorFunc` y `mainContext`; el `help` propio usa la sugerencia), `i18n/catalog/{en,es}.toml`
- Test: `cli/usage_errors_test.go` (nuevo)
- No cambian goldens de ayuda.

**Interfaces:**
- Produces: `translateFlagError(err error) *Error` (privada): con `errors.As` sobre los tipos de pflag (`*pflag.NotExistError`: `GetSpecifiedName()` y `GetSpecifiedShortnames()`; `*pflag.ValueRequiredError`; `*pflag.InvalidValueError`: `GetFlag().Name` y `GetValue()`; `*pflag.InvalidSyntaxError`) devuelve un `UsageErrorT(...)` con clave LITERAL por rama; `nil` si no es ninguno. Se usa en `root.SetFlagErrorFunc(func(c, err) error { if t := translateFlagError(err); t != nil { return t }; return UsageError(err.Error()) })`.
- Produces: `translateArgsError(root *cobra.Command, err error) *Error` (privada), solo para lo que NO pasa por `SetFlagErrorFunc`: regex compiladas una vez sobre `unknown command "X" for "Y"` (si `Y` es `root.CommandPath()`: comando desconocido de la raíz CON sugerencia; si no: «`Y` no admite argumentos (recibió «X»)», sin sugerencia), `accepts N arg(s), received M` y `accepts at most N arg(s), received M`; `nil` en cualquier otro caso. Se llama en `mainContext` solo cuando el error no es ya un `*Error` ni un `*exitCodeError` (`err = t` si no es nil).
- Claves nuevas (en/es, mismo orden; con los plurales elegidos por N con clave LITERAL, como `doctor.go:313-317`, no por `TN`: los errores usan `T`): `err.usage.unknown_command` (`unknown command %[1]q`), `err.usage.unknown_command_suggest` (`unknown command %[1]q: did you mean %[2]q?`), `err.usage.no_args` (`%[1]s takes no arguments (got %[2]q)`), `err.usage.unknown_flag` (`unknown option %[1]s`), `err.usage.flag_needs_value` (`%[1]s needs a value`), `err.usage.bad_value` (`invalid value %[1]q for %[2]s`), `err.usage.bad_syntax` (`malformed option: %[1]s`), `err.usage.args_exact.one`/`.other` (`%[1]s takes %[2]d argument, got %[3]d` / `…takes %[2]d arguments, got %[3]d`), `err.usage.args_at_most.one`/`.other` (`%[1]s takes at most %[2]d argument, got %[3]d` / `…arguments…`) y `err.help.unknown_topic_suggest` (`unknown help topic %[1]q: did you mean %[2]q?`). En español natural y SIN anglicismos: «opción desconocida: --bad», «--limit necesita un valor», «el comando "serv" no existe: ¿querías decir "serve"?», «valor no válido "abc" para --limit», «connect admite 1 argumento, recibió 0».
- Produces: `suggestCommand(root *cobra.Command, name string) string`: sobre los comandos de la raíz con `IsAvailableCommand()` (excluye ocultos y `help`; ni `completion`), en minúsculas; si `name` tiene ≥ 2 letras, los que EMPIEZAN por `name` (el más corto, y a igual longitud el alfabéticamente primero); si no hay ninguno, el de menor distancia de edición (Levenshtein, implementación mínima propia) siempre que sea ≤ 2 (a igual distancia, alfabético); `""` si no hay candidato.
- Consumes: `ExecuteContextC` y `forCommand` (P6a: el arreglo apunta al comando que falló; el error traducido lleva `fixKey == "error.usage_fix"`, así que `forCommand` lo sigue ajustando). `root.DisableSuggestions = true` se mantiene para que el texto «Did you mean this?» de cobra no se cuele en lo que NO se traduce.

- [ ] **Step 1: Tests que fallan** (`cli/usage_errors_test.go`, con `runCLI` e `i18ntest.Lang`; `isolate(t)` si hace falta):
1. `runCLI(t, "serv")`: exit 2; inglés: `Error: USAGE — unknown command "serv": did you mean "serve"?` y el arreglo ``Run `adbuddy --help` ``; español: el mensaje en español y NINGUNA de estas cadenas de cobra en stderr: `unknown command`, `Did you mean`, `arg(s)`, `flag needs`, `accepts ` (`i18ntest.NoEnglish` solo vigila el catálogo: añade `!strings.Contains` explícitos), sin `env` ni `send` como sugerencia; `--json`: el `message` del envelope es nuestro inglés, `code` `USAGE`, `fix` presente.
2. `runCLI(t, "doctor", "serv")` (`NoArgs`): español «doctor no admite argumentos…» y SIN sugerencia (no se sugiere un comando de la raíz para un argumento de otro).
3. `runCLI(t, "connect")` (falta el argumento: `ExactArgs(1)`): español «connect admite 1 argumento, recibió 0» (singular); con dos argumentos de más, el plural; `runCLI(t, "new", "a", "b", "c")` (`MaximumNArgs`): `args_at_most`.
4. `runCLI(t, "journal", "--bad")`: «opción desconocida: --bad», con el arreglo apuntando a `adbuddy journal --help`; `-z` (corto): también; `runCLI(t, "journal", "--limit")`: `flag_needs_value`; `runCLI(t, "journal", "--limit", "abc")`: `bad_value`; y con un flag con abreviatura (uno que tenga `-o`: `adbuddy screenshot --scale x`/`-o`: `invalid argument "x" for "--scale" flag` o con el par `"-o, --output"`): el nombre del flag sale limpio; `runCLI(t, "--nope")` (flag desconocido en la raíz): traducido.
5. `suggestCommand` (tabla): `serv` → `serve`; `jour` → `journal`; `jurnal` → `journal` (distancia 1); `zzz` → ``; `ver` → `version`; `s` → `` (una letra: sin sugerencia); `set` → `setup` (prefijo); no sugiere `hook`, `help` ni `completion`; a igual longitud, el alfabéticamente primero.
6. Los errores que `translate*` NO reconocen siguen como hoy (`translateFlagError(errors.New("algo raro"))` y `translateArgsError(root, errors.New("algo raro"))` devuelven `nil`; un `bad flag syntax` sí es USAGE con exit 2).
7. Los textos de comandos que ya devolvían su propio `UsageErrorT` (`tap` sin argumentos, `help nope`) no cambian salvo `help serv`, que gana `err.help.unknown_topic_suggest`; `help nope` (sin candidato) sigue con `err.help.unknown_topic`.

- [ ] **Step 2: Ejecutar: fallan**: `cd C:/Users/Jose/orca/workspaces/adbuddy/ux/host && go test -timeout 300s ./internal/cli -run 'TestUsageErrors|TestSuggest' -v`.

- [ ] **Step 3: Implementar** `translateFlagError`, `translateArgsError`, `suggestCommand`, las claves y el cableado: `SetFlagErrorFunc` en `NewRoot` y, en `mainContext`, `translateArgsError` antes del `AsError`; el `help` propio (`RunE` de P6a) usa `suggestCommand` para elegir entre `err.help.unknown_topic` y `err.help.unknown_topic_suggest`.

- [ ] **Step 4: Ejecutar y mirar**: `go test -timeout 300s ./internal/cli ./internal/i18n/...` → PASS; `go vet ./internal/cli`; los goldens de ayuda no cambian.

- [ ] **Step 5: Commit**

```bash
cd C:/Users/Jose/orca/workspaces/adbuddy/ux && git add host && git commit -m "feat(cli): errores de uso de cobra en el idioma de la persona y con la mejor sugerencia

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: `doctor` imprime cada comprobación según termina (H5)

`doctor` ejecuta todas las comprobaciones (hasta 60 s en total, 10 s por llamada a adb) y solo imprime al final: con el móvil apagado o adb colgado parece bloqueado.

**Files:**
- Modify: `cli/doctor.go` (`runDoctor` y su llamada en `newDoctorCmd`, líneas ~172 y ~182; `printReport`)
- Test: `cli/doctor_stream_test.go` (nuevo; andamiaje de `cli/doctor_test.go`)

**Interfaces:**
- Modifies: `runDoctor(ctx, app, onCheck func(check))` (`nil` = como hoy). Solo cambian sus DOS llamadas en `doctor.go` (~172 y ~182): ningún test llama a `runDoctor` ni a `printReport`. `add` llama a `onCheck(c)` justo después de añadir; `skip` y `addPCChecks` pasan por `add`. El orden de aparición es el del informe (el de `add`: config, adb, adb server, serve…), no el de cálculo.
- Produces: `printCheck(app, c check)` (el cuerpo del bucle de `printReport`, que pasa a usarla) y `printVerdict(app, rep)`.
- Streaming solo si: modo humano (`!app.JSON`), SIN `--fix` (con `--fix` el informe puede recalcularse tras reiniciar adb y `noteOnServer` reescribe comprobaciones: se imprime entero al final como hoy) y no es la sesión del agente (`askTheUser` reescribe al final). El callback de `RunE` mira el contexto de Ctrl+C, NO el del tope de 60 s: `func(c check) { if stop.Err() == nil { printCheck(app, c) } }` (al cancelar, las comprobaciones que quedan se añaden como ✗ «context canceled» y no deben imprimirse antes de que `RunE` devuelva `Interrupted()`; el tope de 60 s sí se imprime como hoy).

- [ ] **Step 1: Tests que fallan** (`cli/doctor_stream_test.go`):
1. Con un adb falso que tarda (`FakeRule.SleepMs` sobre una regla de `shell`, mira cómo lo usan `doctor_test.go`/`main_test.go`: `runCtx` escribe en la misma gorrutina, sin mutex), un `io.Writer` que anota el instante de cada `Write` muestra que la línea de `config` salió ANTES de que acabara la última comprobación; y el texto final es BYTE A BYTE idéntico al de `printReport` sobre el mismo informe (compara la salida streaming con la que da la ruta por lotes del mismo estado: p. ej. con `--fix` o llamando a `printReport(app, rep)` sobre el `rep` resultante).
2. Con `--json`, con `--fix` y con `ADBUDDY_SOURCE=agent`, la salida es la de hoy (un único bloque al final; `--json` un solo envelope).
3. Ctrl+C a mitad (contexto cancelado): sale `Interrupted()` (exit 130), sin veredicto y SIN ninguna línea con `context canceled` ni ✗ por la cancelación.
4. El veredicto y el código de salida (0/1) no cambian.

- [ ] **Step 2: Ejecutar: fallan**: `go test -timeout 300s ./internal/cli -run 'TestDoctor' -v`.
- [ ] **Step 3: Implementar** como arriba.
- [ ] **Step 4: Ejecutar y mirar**: `go test -timeout 300s ./internal/cli ./internal/i18n/...` → PASS; `go vet ./internal/cli`; goldens de ayuda sin cambios.
- [ ] **Step 5: Commit** (`feat(cli): doctor imprime cada comprobacion segun termina`, con el trailer exacto).

---

### Task 3: El QR de `setup` con la salida redirigida y en una consola de Windows (H13)

`colorOK` solo mira `NO_COLOR` y `TERM`: con `setup 2>log` los escapes ANSI van al fichero, y con `NO_COLOR` el QR sale sin fondo blanco (invertido en un terminal oscuro); y en una consola de Windows antigua los `\x1b[` salen como texto porque solo `attach` activa el modo de terminal virtual. (El dibujo del QR, `pairing/qr.go`, NO cambia: `RenderQR(text, color)` ya hace lo que hace con `color` true/false.)

**Files:**
- Modify: `cli/setup.go` (`colorOK` → `qrStyle`; las dos llamadas a `RenderQR`; `enableVT` al empezar), `cli/attach_windows.go` y `cli/attach_unix.go` (una variante de `enableVT` que dice si lo consiguió)
- Test: `cli/setup_qr_test.go` (nuevo; camino de `TestSetupPairsWithCodeAndPreparesEverything`, `setup_test.go:~80`, que llega a `pair` y dibuja el QR de `HelpURL`), un test de regresión de `RenderQR` en `pairing/qr_test.go` solo SI ya existe (si no, no se crea: el paquete no se toca)

**Interfaces:**
- Produces: `qrStyle(app *App, vt bool) (color bool)`: `true` solo si `isTerminal(app.Stderr)` (variable sustituible de `common.go:~77`: úsala tal cual con `setVar` como `setup_test.go:~306`), `TERM != "dumb"` y `vt` (el modo de terminal virtual de Windows se consiguió; en Unix siempre `true`). `NO_COLOR` no interviene (ruling arriba). `colorOK` se borra si deja de usarse.
- Produces: `enableVTOK(f *os.File) (restore func(), ok bool)` en `attach_windows.go` (`SetConsoleMode` con `ENABLE_VIRTUAL_TERMINAL_PROCESSING`, `ok` si no falló) y `attach_unix.go` (`return func() {}, true`); `enableVT` (la de `attach`) puede quedarse o delegar en ella. `setup` hace `restore, vt := enableVTOK(asFile(app.Stderr)); defer restore()` al principio de su `RunE` (antes de dibujar nada) y pasa `vt` a `qrStyle`.
- `setup` con la salida redirigida: ningún escape `\x1b` en ningún sitio; solo `HelpURL` en texto (como hoy). El payload del QR de emparejamiento NUNCA se imprime en texto.

- [ ] **Step 1: Tests que fallan** (`cli/setup_qr_test.go`; fija `t.Setenv("NO_COLOR", "")` y `t.Setenv("TERM", "xterm")` para que el RED sea fiable):
1. `qrStyle` (tabla, con `isTerminal` sustituida): terminal + `vt` → true; terminal + `NO_COLOR=1` + `vt` → true (el QR es funcional); terminal + `TERM=dumb` → false; no terminal → false (en cualquier caso); terminal sin `vt` (consola de Windows sin modo virtual) → false.
2. `setup` hasta el QR de `HelpURL` con stderr un `bytes.Buffer` (no terminal): su salida no contiene `\x1b`; con `isTerminal` forzada a true y `NO_COLOR=1`, el QR lleva `\x1b[30;47m` y `\x1b[0m` por línea.
3. El texto de `setup` con `--code` (donde se pide la dirección) no contiene el payload del QR ni el secreto (comprueba que la salida no incluye el `Payload()` del emparejamiento en ningún camino).
4. `NO_COLOR=1`: el resto de la salida de `setup` no añade colores (hoy no los hay: el test lo fija).

- [ ] **Step 2: Ejecutar: fallan**: `go test -timeout 300s ./internal/cli -run 'TestQRStyle|TestSetupQR' -v`.
- [ ] **Step 3: Implementar** `qrStyle`, `enableVTOK` (las dos plataformas: comprueba que compila para Windows y Linux con `GOOS=linux go vet ./internal/cli` y `GOOS=windows go vet ./internal/cli`), y sustituir `colorOK(app)` en las dos llamadas de `RenderQR`.
- [ ] **Step 4: Ejecutar y mirar**: `go test -timeout 300s ./internal/cli ./internal/i18n/...` → PASS; `go vet ./internal/cli` en Windows y `GOOS=linux go vet ./internal/cli`; `git diff --stat host/internal/pairing` vacío.
- [ ] **Step 5: Commit** (`fix(cli): el QR de setup lleva escapes solo en una terminal con modo virtual y activa ese modo en Windows`, con el trailer exacto).

---

### Task 4: Ejemplos en los comandos que no tienen (H6)

`attach`, `new`, `stop`, `send`, `env` y `version` no llevan `Example` (el `Example` de `envcmd.go:22` es el de `env init`).

**Files:**
- Modify: `cli/attach.go` (`attach`, `new` y `stop`, ~líneas 135-157), `cli/send.go` (~48-56), `cli/envcmd.go` (el comando `env`), el de `version`, con `Example` en el mismo estilo que los demás (literales con dos espacios, como `journal.go:28`: comandos y flags iguales en los dos idiomas); `cli/testdata/help/en/{attach,new,stop,send,env,version}.txt` (regrabar: seis)
- Test: ampliar `cli/help_groups_test.go` (de P6a)

**Interfaces:** ejemplos reales (lee el `Use` y los flags de cada uno): `attach` (solo `adbuddy attach`; no tiene flags), `new` (`adbuddy new` y `adbuddy new codex`), `stop` (`adbuddy stop`), `send` (`adbuddy send screen.png` y, si el flag existe, `adbuddy send screen.png --caption "Wi-Fi settings"`), `env` (`adbuddy env init` y `adbuddy env init --dir ~/ADBuddy/pixel`), `version` (`adbuddy version` y `adbuddy version --json`). Español: ninguno de estos comandos tiene golden en español (solo `root` y `doctor`).

- [ ] **Step 1: Test que falla**: recorre los comandos visibles (`helpPaths`, saltando la raíz y los de cobra: `help`, `completion` y sus hijos) y exige `Example != ""`; y que cada línea de cada `Example` (con `strings.TrimSpace`) empieza por `adbuddy ` y que su primer token tras `adbuddy` es un comando existente del árbol (`root.Find`) (no un comando inventado).
- [ ] **Step 2: Ejecutar: falla**.
- [ ] **Step 3: Implementar** los seis `Example` y regrabar los goldens (el diff solo añade el bloque `Examples:` a seis ficheros de `en`).
- [ ] **Step 4: Ejecutar y mirar**: `go test -timeout 300s ./internal/cli ./internal/i18n/...` → PASS; `git diff --stat host/internal/cli/testdata` solo toca esos seis; `go vet ./internal/cli`.
- [ ] **Step 5: Commit** (`feat(cli): ejemplos en attach, new, stop, send, env y version`, con el trailer exacto).

---

### Task 5: Cierre del paquete

**Files:**
- Create: `docs/superpowers/fragmentos/ux-p6b.md`

- [ ] **Step 1: Suite completa de `host/`**: `cd C:/Users/Jose/orca/workspaces/adbuddy/ux/host && go vet ./... && GOOS=linux go vet ./internal/cli && go test -timeout 600s ./...` → PASS.
- [ ] **Step 2: Fragmento** (`docs/superpowers/fragmentos/ux-p6b.md`): qué cambió por tarea; goldens regrabados (los seis de `en`); los rulings del plan (JSON: `message` de USAGE en nuestro inglés; QR y `NO_COLOR`; iconos de `doctor`/ASCII aplazados; sin línea de progreso); aplazado con motivo (`Long` a ≤ 80, prefijos `⚠ `/`⏳ `, H14); notas honestas (la traducción de errores de cobra para argumentos y comando desconocido depende de su texto en inglés de la 1.10.2: si cobra cambia sus mensajes, `translateArgsError` devuelve `nil` y el error queda como antes —hay un test del caso no reconocido—; los errores de flags usan los tipos de pflag; la sugerencia es por prefijo y luego por distancia ≤ 2; `bad flag syntax` pasa de INTERNAL/exit 1 a USAGE/exit 2; el streaming de `doctor` solo en modo humano sin `--fix` y sin la sesión del agente; el QR de `setup` conserva el fondo blanco con `NO_COLOR` y va sin escapes con la salida redirigida o sin modo virtual en Windows); y validaciones manuales (el QR en una consola de Windows antigua, en un terminal oscuro con `NO_COLOR=1` y redirigido a un fichero; `doctor` con el móvil apagado: las líneas van saliendo).
- [ ] **Step 3: Commit** (`docs: fragmento del paquete P6b de UX`, con el trailer exacto).

## Validaciones manuales pendientes

Solo para el fragmento; ninguna se pide al usuario ahora: el QR en una consola de Windows antigua, en un terminal oscuro con `NO_COLOR=1` y redirigido, y `doctor` con el móvil apagado viendo salir las líneas una a una.
