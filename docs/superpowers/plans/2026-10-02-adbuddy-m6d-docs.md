# M6d · Documentación de usuario y donaciones Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Escribir la documentación de usuario de ADBuddy (README en inglés y español, instalación, uso diario, modelo de seguridad, resolución de problemas, FAQ, guía de preparación del móvil completada) y preparar las donaciones sin enlaces inventados, con un test que impide comandos y enlaces rotos en las guías.

**Architecture:** Markdown publicado por GitHub Pages desde `docs/` (Jekyll `jekyll-theme-minimal`) más `README.md` y `README.es.md` en la raíz. Un test del paquete `cli` (`docs_test.go`) comprueba que los enlaces relativos existen y que cada comando `adbuddy …` de las guías existe con flags válidos (reutiliza el árbol de cobra, como el test de las skills). Cada guía la escribe un implementador con el código delante y la revisa otro agente frente al código y a la spec.

**Tech Stack:** Markdown, Go 1.27 (solo el test), cobra, `gopkg.in/yaml.v3`. Sin dependencias nuevas.

**Spec:** `docs/superpowers/specs/2026-10-02-adbuddy-m6d-docs-design.md` (la "spec"; sus secciones se citan como §N). Contexto: `CLAUDE.md`, `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md` (§3, §4, §8, §9, §12), `docs/releasing.md`, `scripts/install.sh`, `.goreleaser.yaml`, `host/internal/cli/*.go` (los comandos y sus `--help`).

## Global Constraints

Copiadas de la spec; cada tarea las incluye.

- **Idioma:** las guías nuevas en inglés; `README.es.md` y `docs/setup/es/index.md` en español. Ningún texto traducible nuevo en el host ni en la app; no se toca `host/internal/**` salvo `docs_test.go`, ni `android/`.
- **Verdad:** todo comando, flag, ruta, código de error, nombre de fichero de la release, requisito de versión y mensaje literal se comprueba contra el código o el pipeline (`.goreleaser.yaml`, `scripts/install.sh`, `.github/workflows/release.yml`). Lo que no esté en `main` o no esté publicado (pantallas de M5c, i18n de M6c, Homebrew, winget, IzzyOnDroid, firma de binarios de Windows) **no se documenta como disponible**: si hace falta mencionarlo, se dice "no disponible todavía".
- **El código manda, no la spec v1** (la revisión del plan lo comprobó): la spec v1 es diseño; si el código (host y `android/`) y la spec difieren, se documenta el código, y lo que solo está en la spec no se documenta. Ejemplos ya comprobados: la lista de apps protegidas hoy se propone **por heurística de nombre** y el usuario aún no la edita (la pantalla llega con M5c); "Mantener la depuración activa" no existe; el desbloqueo para confirmar en el móvil es solo de Android 12+; no hay Pausar desde una notificación (está en el menú del chat y en el overlay). Las etiquetas de la app que se citen salen de `android/app/src/main/res/values/strings.xml`.
- **Hoy no hay release pública ni certificado fijado** (`git tag` vacío, repo privado, `SigningCertSHA256` vacío en `host/internal/phoneapp/release.go`): `adbuddy setup` no descarga la app (lo dice), y la URL de `install.sh` de la release aún no responde. Por eso `docs/install.md` incluye **"Build from source"** (cierto hoy: ver `host/README.md`) y la instalación por release se presenta como "cuando haya release"; el README dice "no public release yet" sin número de versión y enlaza a la página de Releases.
- **Enlaces:** las páginas sueltas se enlazan con su `.md` (`../install.md`, `../../install.md` desde `docs/setup/es/`: `jekyll-relative-links` los convierte a `.html` en Pages); la barra final (`setup/`) solo para carpetas con `index.md`. Un `.md` publicado de `docs/` no enlaza fuera de `docs/` ni a un fichero excluido (en Pages darían 404): para eso, un enlace `https://github.com/hybriqtech/adbuddy/blob/main/…`.
- **Seguridad con honestidad:** el aviso de §4.6 de la spec v1 ("el shim protege contra errores e inyecciones de prompts, no contra un agente decidido a buscar otro adb; la garantía real es *Cortar acceso*") aparece en el README y en `docs/safety.md`. Sin superlativos ("unhackable", "100 % seguro"…).
- **Sin datos privados:** ni IPs reales, seriales, rutas de usuario ni nombres de equipos. Ejemplos con `192.0.2.x`, `198.51.100.x`, `203.0.113.x` o `100.64.0.x` (documentales) o `<ip:port>`.
- **Jekyll:** cada `.md` publicado lleva front matter (`title`, `layout: default`; `lang: en|es` donde proceda); enlaces relativos entre páginas; la URL `https://hybriqtech.github.io/adbuddy/setup/` está en el código del host: `docs/setup/` no se mueve.
- **Donaciones (§5):** ningún enlace a GitHub Sponsors ni a Ko-fi que no exista; `FUNDING.yml` con las claves comentadas.
- **Cada tarea compila y pasa sus tests sola:** `go test ./internal/cli/ -run TestDocs -count=1 -timeout 5m` (desde `host/`), `gofmt -l .` vacío y `go vet ./...` limpio.
- **Proceso del proyecto (`CLAUDE.md`):** commits con identidad local `hybriqtech` y el trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`; nunca `--no-verify`; nunca modelos Haiku; los tests no tocan un móvil real; el PC Windows no corre `-race`; ficheros de `host/`, `docs/` y `.github/` en LF.

## Reparto de revisiones

| Tarea | Revisor |
|---|---|
| T1 (test), T2, T4, T5, T7 | `adbuddy-reviewer` (Sonnet), con tope de ~40 minutos |
| T3 (`safety.md` y FAQ) y T6 (README: resumen "Safety" y comparación con ARTEMIS) | `adbuddy-reviewer-opus`, tope ~40 minutos |
| Revisión final de la rama | `adbuddy-reviewer-opus`: **todas las afirmaciones de seguridad de todas las guías**, contra el código |

Un revisor de documentación comprueba cada afirmación comprobable **contra el código** (no contra el informe del implementador) y lista las que no pudo comprobar. Las tareas T2-T7 van en serie (todas amplían `requiredDocs`).

## Estructura de ficheros

| Fichero | Tarea |
|---|---|
| `host/internal/cli/docs_test.go` | T1 (y `requiredDocs` en T2-T6) |
| `docs/install.md`, `docs/usage.md` | T2 |
| `docs/safety.md`, `docs/faq.md` | T3 |
| `docs/troubleshooting.md` | T4 |
| `docs/setup/index.md`, `docs/setup/es/index.md`, `docs/index.md` | T5 |
| `README.md`, `README.es.md` | T6 |
| `.github/FUNDING.yml`, `docs/pre-public-checklist.md`, `docs/_config.yml`, `docs/releasing.md`, `docs/superpowers/plans/pendientes-m3-m5.md` | T7 |

---

### Task 1: Guarda de las guías (`docs_test.go`)

**Files:**
- Create: `host/internal/cli/docs_test.go`

**Interfaces:**
- Produces: `var requiredDocs []string` (rutas relativas a la raíz del repo que **deben** existir; cada tarea de documentos añade las suyas), `func docsRepoRoot(t *testing.T) string`, `func publishedDocs(t *testing.T, root string) []string`, `func checkDocLinks(root, file, text string) []string`, `func checkDocCommands(root *cobra.Command, file, text string) []string`.

- [ ] **Step 1: Escribir el test**

`host/internal/cli/docs_test.go`:

```go
package cli

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// requiredDocs are the guides the M6d plan delivers (spec §1). Each task adds
// its own until the whole list is here.
var requiredDocs = []string{"README.md", "docs/index.md", "docs/setup/index.md", "docs/setup/es/index.md"}

var (
	docLinkRe    = regexp.MustCompile(`\]\(([^)\s]+)\)`)
	docInlineRe  = regexp.MustCompile("`([^`\n]+)`")
	docIPv4Re    = regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}\b`)
	docPrivateRe = regexp.MustCompile(`C:\\Users|/home/|/Users/|emulator-\d+|ANDROID_SERIAL=`)
	// Documentation ranges (RFC 5737) and the first CGNAT block, which the
	// product's own --help uses as its Tailscale example.
	docIPOK = []string{"192.0.2.", "198.51.100.", "203.0.113.", "100.64.0.", "0.0.0.0", "127.0.0.1"}
	// Languages whose blocks hold commands a person types.
	docShellLangs = map[string]bool{"": true, "sh": true, "bash": true, "shell": true, "console": true, "powershell": true, "ps1": true}
)

// docsRepoRoot walks up from the package folder to the repository root.
func docsRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "docs", "_config.yml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repository root (a folder with docs/_config.yml) not found")
		}
		dir = parent
	}
}

// publishedDocs lists, relative to root and with forward slashes, the Markdown
// files GitHub Pages publishes plus the READMEs: docs/_config.yml's `exclude`
// says what stays out.
func publishedDocs(t *testing.T, root string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "docs", "_config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Exclude []string `yaml:"exclude"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range []string{"README.md", "README.es.md"} {
		if _, err := os.Stat(filepath.Join(root, f)); err == nil {
			out = append(out, f)
		}
	}
	docs := filepath.Join(root, "docs")
	err = filepath.WalkDir(docs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(docs, p)
		rel = filepath.ToSlash(rel)
		if slices.Contains(cfg.Exclude, rel) || strings.HasPrefix(d.Name(), "_") && rel != "." {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(p, ".md") {
			out = append(out, "docs/"+rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// codeSpans splits a Markdown text into the lines inside fenced blocks (with
// the block's language) and the rest.
type docBlock struct {
	lang  string
	lines []string
	line  int // line number of the first line inside the block
}

func splitDoc(text string) (blocks []docBlock, prose string) {
	var cur *docBlock
	var p strings.Builder
	for i, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trim := strings.TrimSpace(line)
		switch {
		case cur == nil && strings.HasPrefix(trim, "```"):
			cur = &docBlock{lang: strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trim, "```"))), line: i + 2}
		case cur != nil && trim == "```":
			blocks = append(blocks, *cur)
			cur = nil
		case cur != nil:
			cur.lines = append(cur.lines, line)
		default:
			p.WriteString(line)
			p.WriteByte('\n')
		}
	}
	if cur != nil {
		blocks = append(blocks, *cur)
	}
	return blocks, p.String()
}

// checkDocLinks returns one message per relative link whose target does not
// exist. Jekyll's convention is followed: "x/" is x/index.md, "x" is x.md or
// x/index.md.
func checkDocLinks(root, file, text string) []string {
	var bad []string
	_, prose := splitDoc(text)
	for _, m := range docLinkRe.FindAllStringSubmatch(prose, -1) {
		target := m[1]
		if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") || strings.HasPrefix(target, "#") {
			continue
		}
		if i := strings.IndexAny(target, "#?"); i >= 0 {
			target = target[:i]
		}
		if target == "" {
			continue
		}
		base := filepath.Join(root, filepath.FromSlash(filepath.ToSlash(filepath.Dir(file))), filepath.FromSlash(target))
		var candidates []string
		switch {
		case strings.HasSuffix(target, "/"):
			candidates = []string{filepath.Join(base, "index.md")}
		case filepath.Ext(target) == "":
			candidates = []string{base + ".md", filepath.Join(base, "index.md")}
		default:
			candidates = []string{base}
		}
		found := false
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				found = true
			}
		}
		if !found {
			bad = append(bad, file+": the link "+m[1]+" points to nothing")
		}
	}
	return bad
}

// docCommandSegments returns the `adbuddy …` commands of one line: the
// prompt is dropped and pipes, && and ; split it.
func docCommandSegments(line string) []string {
	line = strings.TrimSpace(line)
	for _, p := range []string{"$ ", "PS> ", "> "} {
		line = strings.TrimPrefix(line, p)
	}
	var out []string
	for _, seg := range regexp.MustCompile(`\|\||&&|;|\|`).Split(line, -1) {
		seg = strings.TrimSpace(seg)
		if seg == "adbuddy" || strings.HasPrefix(seg, "adbuddy ") {
			out = append(out, seg)
		}
	}
	return out
}

// checkDocCommands validates every `adbuddy …` command of a guide against the
// real command tree: the subcommand exists, the flags are valid and the
// positional arguments are accepted. A command in running text whose first
// word is not a subcommand ("adbuddy is free") is not a command.
func checkDocCommands(rootCmd *cobra.Command, file, text string) []string {
	var bad []string
	check := func(where, seg string, inBlock bool) {
		fields := strings.Fields(strings.NewReplacer("'", "", `"`, "").Replace(seg))
		if len(fields) < 2 {
			return
		}
		cmd, rest, err := rootCmd.Find(fields[1:])
		if err != nil || cmd == rootCmd {
			if inBlock && !strings.HasPrefix(fields[1], "-") {
				bad = append(bad, where+": adbuddy has no subcommand "+fields[1]+": "+seg)
			}
			return
		}
		if err := cmd.ParseFlags(rest); err != nil {
			bad = append(bad, where+": "+seg+": "+err.Error())
			return
		}
		if err := cmd.ValidateArgs(cmd.Flags().Args()); err != nil {
			bad = append(bad, where+": "+seg+": "+err.Error())
		}
	}
	blocks, prose := splitDoc(text)
	for _, b := range blocks {
		if !docShellLangs[b.lang] {
			continue
		}
		for i, line := range b.lines {
			for _, seg := range docCommandSegments(line) {
				check(file+":"+strconv.Itoa(b.line+i), seg, true)
			}
		}
	}
	for _, m := range docInlineRe.FindAllStringSubmatch(prose, -1) {
		for _, seg := range docCommandSegments(m[1]) {
			check(file, seg, false)
		}
	}
	return bad
}


// checkDocPrivacy returns a message for every private-looking datum.
func checkDocPrivacy(file, text string) []string {
	var bad []string
	if m := docPrivateRe.FindString(text); m != "" {
		bad = append(bad, file+": contains "+m)
	}
	for _, ip := range docIPv4Re.FindAllString(text, -1) {
		ok := false
		for _, p := range docIPOK {
			ok = ok || strings.HasPrefix(ip, p)
		}
		if !ok {
			bad = append(bad, file+": contains the IP address "+ip+" (use 192.0.2.x or <ip:port>)")
		}
	}
	return bad
}

func TestDocsAreGuarded(t *testing.T) {
	root := docsRepoRoot(t)
	files := publishedDocs(t, root)
	for _, f := range requiredDocs {
		if !slices.Contains(files, f) {
			t.Errorf("%s is missing: the plan delivers it", f)
		}
	}
	tree := NewRoot(&App{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard, Getenv: func(string) string { return "" }})
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		var bad []string
		bad = append(bad, checkDocLinks(root, f, text)...)
		bad = append(bad, checkDocCommands(tree, f, text)...)
		bad = append(bad, checkDocPrivacy(f, text)...)
		for _, b := range bad {
			t.Error(b)
		}
	}
}
```

Notas para quien implemente: (a) el código usa `strconv.Itoa` para los números de línea; (b) **construye un `NewRoot` por comando** (como `checkSkillExample`): con uno compartido, `adbuddy run codex -- --model o4` seguido de `adbuddy run` hace entrar en pánico a `run.go:27` (`ArgsLenAtDash` arrastra el valor anterior); (c) `strings.HasPrefix(d.Name(), "_") && rel != "."` salta carpetas y ficheros con `_` (como `_config.yml`, `_site`); (d) comprueba que el nombre `docsRepoRoot` y los demás no choquen con otros del paquete `cli`.

**ENMIENDAS OBLIGATORIAS al código de arriba** (la revisión opus del plan réplicó el test: tal cual falla con las guías de hoy y entra en pánico; con estas correcciones pasa). El código de arriba es la base; aplica TODAS:

1. **Marcadores `<…>` y grupos opcionales.** Antes de partir en segmentos y en campos, sustituye `<[^<>\n]*>` por `ARG` y borra los grupos opcionales `\[[^\[\]\n]*\]` (valen `adbuddy connect <new ip:port>`, `adbuddy setup --apk <your build>`, `adbuddy run [agent]`, `adbuddy undo [last|session|<id>]`). Sin esto fallan `docs/setup/index.md` y su versión en español.
2. **Un árbol por comando** (nota (b) de arriba): `NewRoot(...)` dentro de `check`, no compartido.
3. **Formas de comando que las guías usarán** (todas comprobadas por el revisor): (a) `--help`/`-h` dan `pflag: help requested` y `adbuddy help setup` "no subcommand help": llama a `InitDefaultHelpCmd()` y `InitDefaultHelpFlag()` donde haga falta y acepta `errors.Is(err, pflag.ErrHelp)`; (b) comillas: parte los campos con un separador que respete comillas simples y dobles (unas 25 líneas, sin expandir nada) en vez de quitarlas y partir por espacios (`adbuddy tap "Sign in"`, `send … --caption "Device profile"`); (c) comentarios: corta la línea en el primer ` #` fuera de comillas (`adbuddy setup   # pair the phone`); (d) bloques ```console: valida solo las líneas con prompt (`$ ` o `PS> `); la salida de un programa va en bloques ```text (no se valida); (e) Windows: normaliza `adbuddy.exe` y `.\adbuddy.exe` a `adbuddy`, y añade `pwsh` a `docShellLangs`; (f) `adbuddy --nope` (flags de la raíz) se valida cuando `fields[1]` empieza por `-`.
4. **Enlaces** (`checkDocLinks`): (a) un destino sin extensión como `LICENSE` es válido si existe como **fichero** (añade `base` a los candidatos y exige que no sea un directorio); (b) si `file` está bajo `docs/`, falla cuando el destino resuelto **salga de `docs/` o caiga en `exclude`** ("en Pages sería un 404: usa un enlace https://github.com/hybriqtech/adbuddy/blob/main/…"); (c) los enlaces con título (`[a](x.md "t")`) y de referencia (`[a]: x.md`) también se comprueban; (d) normaliza el `/` final de las entradas de `exclude` y compara con `path.Match` (Jekyll admite globs); ignora también ficheros y carpetas que empiezan por `.`.
5. **Esqueletos de las guías (para que cada tarea pase sola, porque las guías se enlazan entre sí).** T1 crea, con front matter (`title`, `layout: default`, `lang: en`) y una frase ("Pending: written in a later task of the M6d plan"), `docs/install.md`, `docs/usage.md`, `docs/safety.md`, `docs/faq.md`, `docs/troubleshooting.md` y `README.es.md` (con su enlace cruzado) y los añade todos a `requiredDocs`; cada tarea siguiente los **sustituye** por la guía real. T1 también añade `releasing.md` a `exclude` en `docs/_config.yml` (documento de mantenimiento que hoy se publicaría; los `.md` de `docs/` no deben enlazarlo).
6. **Privacidad:** no marques versiones de cuatro componentes (`1.0.41.0`) como IP: exige que cada cuarteto esté en 0-255 y que no vaya precedido ni seguido de `.` o dígito (`(?:^|[^\d.])(\d{1,3}(?:\.\d{1,3}){3})(?:$|[^\d.])`).
7. `gofmt -w` del fichero (el código de arriba tiene una línea en blanco doble antes de `checkDocPrivacy`). Y los bloques con vallas `~~~` se tratan como errores ("usa ```").

- [ ] **Step 2: Ejecutar**

Run (desde `host/`): `go test ./internal/cli/ -run TestDocs -count=1 -timeout 5m -v`
Expected: PASS con las guías de hoy (`README.md`, `docs/index.md`, `docs/setup/index.md`, `docs/setup/es/index.md` existen). Si falla por un comando o enlace real de esas guías, arréglalo **en la guía** (es un fallo real: p. ej. un flag que no existe) y dilo en el informe.

- [ ] **Step 3: Pruebas negativas permanentes**

Añade `TestDocsGuardCatches` en el mismo fichero con una tabla que llame directamente a `checkDocLinks`, `checkDocCommands` y `checkDocPrivacy` (con un directorio temporal para los enlaces) y exija al menos un mensaje por caso: un enlace a `missing.md`, un enlace a `x/` sin `x/index.md`, `` `adbuddy bogus` `` en un bloque ```sh, `adbuddy setup --nope` en un bloque, `$ adbuddy screen extra extra` (argumentos de más) en un bloque, `` `adbuddy setup --nope` `` en línea, una IP `10.0.0.x`, `C:\Users\x`, `emulator-5554`. Y casos que deben dar **cero** mensajes: `adbuddy setup --code --connect 100.64.0.5:41234`, `adbuddy connect <new ip:port>`, `adbuddy setup --apk <your build>`, `adbuddy run [agent]`, `adbuddy doctor --json | jq .`, `adbuddy --help`, `adbuddy setup -h`, `adbuddy help setup`, `adbuddy tap "Sign in"`, `adbuddy type 'hello world'`, `adbuddy send outbox/x.md --caption "Device profile"`, `adbuddy setup   # pair the phone`, `.\adbuddy.exe setup --code`, una línea de salida `adbuddy 0.3.0` dentro de un bloque ```console sin prompt, `` `adbuddy is free` `` (entre backticks: en prosa, un primer campo que no es subcomando no es un comando), `` `adbuddy` `` solo, un enlace `https://…`, un enlace con ancla `x.md#y` existente, `[License](LICENSE)` con un fichero `LICENSE` existente, un bloque ```text con `adbuddy bogus` (no es un bloque de shell), la IP `192.0.2.7`, la versión `1.0.41.0`. Casos negativos añadidos: `adbuddy run codex -- --model o4` seguido de `adbuddy run` en el mismo documento (sin pánico), `.\adbuddy.exe setup --nope`, `adbuddy --nope`, un enlace desde `docs/x.md` a `../README.md` (sale de `docs/`) y a `releasing.md` (excluido), `~~~sh` como valla, un enlace `[a](x.md "t")` roto y uno de referencia `[a]: x.md` roto.

- [ ] **Step 4: Comprobaciones y commit**

Run: `go test ./internal/cli/ -run TestDocs -count=1 -timeout 5m`, `gofmt -l .` (vacío), `go vet ./...`.

```bash
git add host/internal/cli/docs_test.go
git commit -F - <<'EOF'
test(cli): guarda de las guías (enlaces y comandos de adbuddy que existen)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
EOF
```

---

## Cómo escribir cada guía (Tareas 2 a 7)

**Pasos de cada tarea de documentos** (se repiten; no se copian en cada tarea):

- [ ] Añadir las rutas de la tarea a `requiredDocs` en `host/internal/cli/docs_test.go` (el test falla: faltan los ficheros).
- [ ] Leer el código y los ficheros que cada brief lista, y escribir las guías con front matter (`title`, `layout: default`, `lang: en`).
- [ ] `go test ./internal/cli/ -run TestDocs -count=1 -timeout 5m`, `gofmt -l .` (vacío), `go vet ./...`.
- [ ] Autorrevisión: cada afirmación comprobable de la guía, una a una, contra su fuente; lo que no se pueda comprobar se quita o se dice con cautela. Anotar en el informe una tabla "afirmación → fichero y línea que la respalda".
- [ ] Commit con `git add` de rutas concretas y el trailer exacto.

**Estilo:** frases cortas, imperativo ("Run", "Tap"), pasos numerados, comandos en bloques ```sh o ```powershell, sin adjetivos grandilocuentes, sin emojis salvo 🟢🟡🔴 en `safety.md`; primera aparición de cada término explicada (shim, diario, entorno); sin párrafos de más de cinco líneas; cada guía acaba con "Next" con enlaces.

---

### Task 2: `docs/install.md` y `docs/usage.md`

**Files:** Create `docs/install.md`, `docs/usage.md`; modify `host/internal/cli/docs_test.go` (`requiredDocs`).

**Fuentes que hay que leer para no inventar:** `scripts/install.sh` (variables, qué comprueba, dónde instala, qué sistemas admite), `.goreleaser.yaml` (nombres reales de los archivos y de `checksums.txt`, formatos por sistema), `docs/releasing.md` (qué incluye una release, `adbuddy.apk` y su `.sha256`), `host/internal/paths` (rutas de configuración y datos por sistema), `host/internal/cli/setup.go` (`--apk`, `--no-app`, qué instala y comprueba), `adbuddy <cmd> --help` de cada comando (compila con `go build -o <tmp> ./cmd/adbuddy` en el directorio de scratch y ejecútalo), `host/internal/workspace/templates/AGENTS.md`, `host/internal/agent` (qué agentes conoce y cómo dice que se instalan: el host no los instala), `host/internal/cli/{serve,run,journal,undo,send,remotecmd,connect,attach,helpers,appsetup}.go` (`new` y `stop` están en `attach.go`), `host/README.md` (cómo se compila desde el código, que es lo único cierto hoy), la sección 3.2 y 3.3 de la spec v1 (diseño: manda el código), y lo que la app mergeada hace hoy (`android/app/src/main/kotlin/tech/hybriq/adbuddy/{confirm/ConfirmNotifier.kt,control/CutAccess.kt,control/AccessSwitch.kt,a11y/Overlay.kt,ui/ChatParts.kt,ui/Screens.kt}` y `android/app/src/main/res/values/strings.xml`: no documentes pantallas de M5c ni nada que no esté en main; "varios móviles" solo si el código lo soporta).

**`docs/install.md`** (spec §3.2): requisitos (Android 11+, `minSdk 30` comprobado en `android/app/build.gradle.kts`; PC con Windows, macOS o Linux; el agente elegido); **"Build from source"** primero o en igual rango (cierto hoy; con `adbuddy setup --apk <file>` para la app, como dice `host/README.md`); Linux y macOS con `curl … | sh` (`ADBUDDY_VERSION`, `ADBUDDY_INSTALL_DIR`, SHA-256, destino por defecto, leer el script antes es una opción: dilo con el enlace al script en GitHub) presentado como "cuando haya una release pública" (hoy no la hay: la URL aún no responde); Windows con el archivo de la release (nombre real de `.goreleaser.yaml`), descomprimir en una carpeta del `PATH`, comprobar el checksum con una orden de PowerShell **correcta** (`Get-FileHash -Algorithm SHA256` y comparar con `checksums.txt`; no la inventes sin comprobar su salida), aviso honesto de SmartScreen (binarios sin firmar todavía); la app la instala `adbuddy setup` **cuando exista una release firmada y fijada** (hoy `setup` dice que no descarga ninguna: léelo en `host/internal/phoneapp/release.go` y `setup.go`); actualizar y desinstalar con las rutas reales por sistema (en Windows, `%LOCALAPPDATA%` y `%APPDATA%`, nunca `C:\Users\…`: la guarda lo rechaza). **No** menciones winget, Homebrew ni IzzyOnDroid como disponibles.

**`docs/usage.md`** (spec §3.3): `serve` y `run` (con la explicación de por qué dos terminales), confirmaciones 🔴, `new`/`attach`/`stop`, el chat desde el móvil (solo lo que ya hace la app mergeada), `journal`/`undo` (y qué no tiene inverso), `send`, ayudas de pantalla, pausar/cortar acceso, modo remoto (`adbuddy remote`, `adbuddy connect`, Tailscale, **el riesgo de que el puerto 5555 quede abierto en redes públicas** y su caducidad de 12 h: compruébalo con `adbuddy remote --help`), el entorno `~/ADBuddy/<móvil>/` y las skills (`.claude/skills`, `.agents/skills`: M6b), con enlaces `.md` a `safety.md` y `troubleshooting.md` y a `setup/`. "Pausar" y "Cortar acceso" tal como existen hoy en la app (`CutAccess.kt`, `AccessSwitch.kt`, `Overlay.kt`, `ChatParts.kt`), sin la notificación fija que la spec v1 describe si no existe.

`requiredDocs` += `docs/install.md`, `docs/usage.md`.

- [ ] Pasos de "Cómo escribir cada guía".

---

### Task 3: `docs/safety.md` y `docs/faq.md`

**Files:** Create `docs/safety.md`, `docs/faq.md`; modify `requiredDocs`.

**Fuentes:** spec v1 §4 (4.1-4.6) y §9 (diseño: manda el código); `android/app/src/main/kotlin/tech/hybriq/adbuddy/{a11y/ProtectedApps.kt,confirm/ConfirmNotifier.kt,control/CutAccess.kt,control/AccessSwitch.kt}` y `strings.xml` (lo que la app hace hoy: la lista de apps protegidas se propone por heurística de nombre y **el usuario aún no la edita** —"la lista editable llega con M5c, no disponible todavía"—; el desbloqueo para confirmar es de Android 12+, no de Android 11); `host/internal/classify` (`rules/*.yaml`, `level.go`; **ejemplos de cada nivel: cita solo comandos que aparecen en `host/internal/classify/testdata/habitual.jsonl` o en otro corpus con ese nivel, o comprueba el nivel con un test de scratch fuera del repo; no hay un comando de la CLI que clasifique**), **privacidad ("cero telemetría; qué habla con internet")**: `grep -rhoE 'https?://[^"]+' host/internal host/cmd --include=*.go | grep -v _test` (hoy: `dl.google.com/android/repository/`, github.com, `docs.claude.com`) y que la app solo pide `INTERNET` para el WebSocket local (`AndroidManifest.xml`), `host/internal/shim`, `host/internal/journal`, `host/internal/precheck` (qué lleva inverso), `host/internal/confirm`, `host/internal/daemon/guard.go` (apps protegidas), `host/internal/phoneapp` (confianza en el certificado: M6a, `docs/superpowers/specs/2026-10-01-adbuddy-m6a-releases-design.md` §5-§6), `host/internal/workspace/templates/AGENTS.md`, `host/internal/remote`, y `docs/superpowers/specs/2026-10-01-adbuddy-m5-ayudas-remoto-design.md` para lo que bloquean las apps protegidas.

**`docs/safety.md`** (spec §3.4): la estructura de la spec, terminando con **"What ADBuddy does not guarantee"** (el aviso honesto con las mismas palabras de fondo que la spec v1 §4.6; modo remoto y 5555; el clasificador no es una caja fuerte; apps protegidas hoy por heurística de nombre (la lista editable llega con M5c: no disponible todavía); Advanced Protection; lo que lee el agente va a su proveedor de modelo) y "Reporting a security problem" (sin inventar canales: si no hay `SECURITY.md`, abrir una *security advisory* privada en GitHub si el repo la tiene habilitada o un issue sin detalles de explotación; lo que falte se anota en el informe como acción del usuario). Sin superlativos.

**`docs/faq.md`** (spec §3.6): las preguntas de la spec; respuestas de 2-4 líneas con enlace; nada de "100 %" ni "imposible".

`requiredDocs` += `docs/safety.md`, `docs/faq.md`.

- [ ] Pasos de "Cómo escribir cada guía". **Esta tarea la revisa un revisor opus frente al código.**

---

### Task 4: `docs/troubleshooting.md`

**Files:** Create `docs/troubleshooting.md`; modify `requiredDocs`.

**Fuentes:** `host/internal/cli/doctor.go` y su `--help` (orden de comprobaciones, `Run:`/`Guidance:`, `--fix`, `--json`, código de salida), `docs/setup/index.md` (la sección "Troubleshooting" existente: no la dupliques; enlázala), y **todos** los códigos de error del host, con DOS búsquedas (la revisión del plan comprobó que `NewError` solo no basta: se escapan `ConfigError`, `TransientError`, `USAGE`, `DAEMON_ERROR` y los códigos que llegan del daemon por IPC): `grep -rhoE '(NewError|ConfigError|TransientError)\("[A-Z_]+"' host/internal --include=*.go | grep -v _test` y `grep -rhoE 'Code: *"[A-Z_]+"' host/internal --include=*.go`. Para cada código: causa y arreglo; el arreglo es el tercer argumento de `NewError` cuando es un literal y, si no, sale de los mapas `helperFixes` (`host/internal/cli/helpers.go`), `remoteFixes` (`remotecmd.go`) y `undoFixes` (`undo.go`); los códigos del daemon (`PAUSED`, `WAITING_FOR_ANSWER`, `UNAUTHORIZED`, `UI_PROTECTED`, `UI_BLOCKED`, `NOTHING_TO_UNDO`, `UNDO_FAILED`, `JOURNAL_UNWRITABLE`…) se leen en el daemon. Verifica los mensajes literales (`Denied by user`, `Denied: nobody could confirm`, `Blocked by ADBuddy`, `device offline`, `ADBuddy host not running (run: adbuddy serve)`, `WORKSPACE_FAILED`, `PAIRING_TIMEOUT`…) con `grep` en el código.

**Contenido** (spec §3.5): `adbuddy doctor` (qué comprueba y cómo leerlo, ejemplo de salida **real** generada con un host de pruebas o descrita sin inventar texto: si no puedes ejecutarlo contra un móvil, describe el formato sin simular la salida), la tabla de códigos (con un comentario HTML `<!-- generated by hand from grep 'NewError(' …; refresh when the CLI changes -->`), problemas por síntoma, y cómo reunir información para un issue sin datos privados.

`requiredDocs` += `docs/troubleshooting.md`.

- [ ] Pasos de "Cómo escribir cada guía". En el informe, la lista de códigos encontrados por `grep` y cuáles quedaron fuera (con motivo).

---

### Task 5: guías de preparación del móvil e índice del sitio

**Files:** Modify `docs/setup/index.md`, `docs/setup/es/index.md`, `docs/index.md`.

**Fuentes:** `host/internal/cli/setup.go` y `appsetup.go` (los pasos reales que ejecuta `adbuddy setup`, en orden, y sus mensajes; incluido que sin release fijada no descarga la app y qué hace con `--apk`), `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md` §3.1 (diseño: manda el código), la app mergeada para el consentimiento (`android/app/src/main/kotlin/tech/hybriq/adbuddy/ui/Screens.kt`, `strings.xml`), `docs/superpowers/specs/2026-10-01-adbuddy-m6a-releases-design.md` §5-§6 (instalación y confianza de la app).

**`docs/setup/index.md` y su versión en español** (spec §3.7): tras la sección de emparejamiento y su resolución de problemas, añadir "What happens next" (conexión, instalación de la app con la comprobación del certificado, perfil del móvil, consentimiento en el móvil —solo lo que `setup.go` y la app mergeada hacen—, elección del agente y creación del entorno) y "Next: start ADBuddy" (`adbuddy serve` en una terminal y `adbuddy run` en otra, con enlace a `../usage.md`). Mantener los marcadores `pending.svg`. Los enlaces a las guías nuevas llevan su `.md` (`../install.md`, `../usage.md`, `../troubleshooting.md`; en español `../../…`: Pages los convierte a `.html`; `install/` daría 404 y la guarda lo rechaza). La versión en español debe decir lo mismo que la inglesa, con los textos de menús de Android en español (`Opciones de desarrollador`, `Depuración inalámbrica`…, los mismos que ya usa la guía).

**`docs/index.md`:** portada con el párrafo de una frase y una lista de enlaces: Set up your phone / Prepara tu móvil (`setup/`, `setup/es/`), Install, Usage, Safety, Troubleshooting, FAQ (con `.md`: `install.md`, `usage.md`…).

- [ ] Pasos de "Cómo escribir cada guía" (los `requiredDocs` de estas tres ya están).

---

### Task 6: `README.md` y `README.es.md`

**Files:** Modify `README.md`, `README.es.md` (sustituye el esqueleto de T1), `host/README.md`.

**Fuentes:** la spec §3.1; `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md` §1 (qué es, por qué, ventajas), §2 (arquitectura), §14 (diseño: manda el código); `LICENSE`; `host/internal/agent` (qué agentes conoce; el host no los instala); la versión de la release más reciente: **hoy no hay etiquetas ni release pública**: "Status" dice "no public release yet" y enlaza a la página de Releases del repo, **sin número de versión** (se desfasa); las guías que escribieron T2-T5 (el README las resume y enlaza, no las duplica); `adbuddy --help` y `adbuddy <cmd> --help` para la tabla de comandos.

**Contenido:** las secciones de §3.1 en ese orden. "Support the project": **toda la sección (título, frase y enlaces) va dentro de un comentario HTML** (`<!-- Enable when the GitHub Sponsors and Ko-fi pages exist (docs/pre-public-checklist.md): ## Support the project … [GitHub Sponsors](https://github.com/sponsors/hybriqtech) · [Ko-fi](https://ko-fi.com/<handle>) -->`), porque una frase visible "you can support its development" sin enlace promete algo que no existe. El "Quick start" usa la orden de instalación de `docs/install.md` (la misma, y con la nota de que hoy se compila desde el código: "Build from source") y enlaza a `docs/setup/` por su URL de Pages (`https://hybriqtech.github.io/adbuddy/setup/`) y a las guías por ruta relativa (`docs/install.md`…), que es lo que funciona en GitHub. La comparación con ARTEMIS: una frase verídica **con fecha y ámbito** ("as of September 2026", según `pendientes-m3-m5.md` §M6 y la spec v1 §1: ARTEMIS lleva su propio LLM y claves de API; ADBuddy usa el agente y la suscripción que el usuario ya tiene), sin hablar de la seguridad de ARTEMIS ni descalificar.

`README.es.md`: traducción fiel, enlace cruzado en la primera línea de ambos (`[Leer en español](README.es.md)` / `[Read in English](README.md)`); las guías enlazadas son las inglesas salvo `docs/setup/es/`.

**`host/README.md`** (la revisión del plan lo encontró desfasado: dice "This is milestone M1… The phone app and the chat arrive in M2-M4", duplica instalación y uso, y `scripts/install.sh` manda a los usuarios de Windows a él): redúcelo a una nota **para desarrolladores** (compilar, tests, el oráculo del clasificador y el test del emulador, las skills: lo que ya tiene de M1-M6b y sigue siendo cierto) que enlace a `docs/install.md` y `docs/usage.md` para el uso. **No toques `scripts/install.sh`** (es un artefacto de la release).

- [ ] Pasos de "Cómo escribir cada guía".

---

### Task 7: donaciones, lista previa a hacer público y limpieza

**Files:** Create `.github/FUNDING.yml`; modify `docs/pre-public-checklist.md`, `docs/releasing.md`, `docs/superpowers/plans/pendientes-m3-m5.md` (el `exclude` de `releasing.md` en `docs/_config.yml` ya lo hizo T1).

- [ ] **`.github/FUNDING.yml`**: un comentario de cabecera ("Uncomment when the pages exist; see docs/pre-public-checklist.md") y las claves **comentadas**: `# github: [hybriqtech]` y `# ko_fi: <handle>`. Debe ser YAML válido (solo comentarios es válido; el fichero no debe tener claves activas con identificadores inventados).
- [ ] **`docs/pre-public-checklist.md`**: marcar hecho lo que M6d cierra (README en inglés con traducción al español); añadir puntos: crear el perfil de GitHub Sponsors y la página de Ko-fi y poner los identificadores reales (descomentar `FUNDING.yml` y el bloque de "Support the project" de `README.md` y `README.es.md`; enlazar en "Acerca de" de la app, M5c); la lista de las **capturas reales** que faltan (los marcadores `pending.svg` de `docs/setup/index.md` y de `docs/setup/es/index.md`, con el nombre de fichero propuesto para cada una); habilitar la *private vulnerability reporting* de GitHub o crear un `SECURITY.md` (y enlazarlo desde `docs/safety.md` cuando exista).
- [ ] **`docs/pre-public-checklist.md`, además:** marcar hecho el punto de `LICENSE` (ya existe) y añadir "repasar `docs/install.md` y el Quick start del README contra la **primera release publicada**: nombres de los archivos, que la URL de `install.sh` responda, que `adbuddy setup` descargue la app y que el aviso de SmartScreen siga siendo cierto; y quitar de `docs/install.md` y del README lo de 'cuando haya release'".
- [ ] **`docs/_config.yml`:** comprobar que `releasing.md` ya está en `exclude` (lo hizo T1) y que el comentario de cabecera sigue siendo cierto.
- [ ] **`docs/releasing.md`**: añadir un paso a "Cutting a release": "Si cambió la CLI (comandos, flags o códigos de error), revisa `docs/troubleshooting.md` (tabla de códigos) y las guías; `go test ./internal/cli/ -run TestDocs` comprueba los comandos y los enlaces pero no los códigos de error ni las afirmaciones".
- [ ] **`docs/superpowers/plans/pendientes-m3-m5.md`** (sección `## M6 · Publicación`): "Hecho en M6d" (README en EN y ES, guías, FAQ, resolución de problemas, test de guarda, FUNDING.yml); pendientes: traducción al español de las guías nuevas (solo README y preparación del móvil están en español); capturas reales; `SECURITY.md`; "Acerca de" con los enlaces de donación (M5c); mantener `docs/troubleshooting.md` al día cuando cambien los códigos de error.
- [ ] `go test ./internal/cli/ -run TestDocs -count=1 -timeout 5m` (el test sigue en verde con `releasing.md` excluido), `go test ./... -count=1 -timeout 10m`, `gofmt -l .`, `go vet ./...`.
- [ ] Commit con `git add` de rutas concretas.

---

## Self-review (de la spec contra el plan)

| Spec | Tarea |
|---|---|
| §1 README (EN) y `README.es.md` | T6 |
| §1 `docs/index.md` | T5 |
| §1 `docs/install.md`, `docs/usage.md` | T2 |
| §1 `docs/safety.md`, `docs/faq.md` | T3 |
| §1 `docs/troubleshooting.md` | T4 |
| §1 guía de preparación del móvil (EN/ES) | T5 |
| §1 `FUNDING.yml` y lista previa | T7 |
| §1 guarda `docs_test.go` y §6 | T1 |
| §1.1 criterios (códigos de error con fila) | T4 + revisión; el test de §6 no comprueba los códigos: queda en el paso de `docs/releasing.md` (T7) |
| §2 principios 1-7 | Global Constraints + estilo + revisión |
| §3.1-§3.7 contenido | briefs de T2-T6 |
| §4.2 URL de Pages no se mueve | Global Constraints |
| §4.3 capturas | T5 (marcadores) y T7 (lista) |
| §5 donaciones sin enlaces inventados | T6 (bloque comentado) y T7 |

Tipos: `requiredDocs`, `docsRepoRoot`, `publishedDocs`, `checkDocLinks`, `checkDocCommands`, `checkDocPrivacy`, `splitDoc` y `docBlock` se definen en T1 y T2-T7 solo amplían `requiredDocs`.
