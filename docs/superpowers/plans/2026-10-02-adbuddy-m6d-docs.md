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
- **Seguridad con honestidad:** el aviso de §4.6 de la spec v1 ("el shim protege contra errores e inyecciones de prompts, no contra un agente decidido a buscar otro adb; la garantía real es *Cortar acceso*") aparece en el README y en `docs/safety.md`. Sin superlativos ("unhackable", "100 % seguro"…).
- **Sin datos privados:** ni IPs reales, seriales, rutas de usuario ni nombres de equipos. Ejemplos con `192.0.2.x`, `198.51.100.x`, `203.0.113.x` o `100.64.0.x` (documentales) o `<ip:port>`.
- **Jekyll:** cada `.md` publicado lleva front matter (`title`, `layout: default`; `lang: en|es` donde proceda); enlaces relativos entre páginas; la URL `https://hybriqtech.github.io/adbuddy/setup/` está en el código del host: `docs/setup/` no se mueve.
- **Donaciones (§5):** ningún enlace a GitHub Sponsors ni a Ko-fi que no exista; `FUNDING.yml` con las claves comentadas.
- **Cada tarea compila y pasa sus tests sola:** `go test ./internal/cli/ -run TestDocs -count=1 -timeout 5m` (desde `host/`), `gofmt -l .` vacío y `go vet ./...` limpio.
- **Proceso del proyecto (`CLAUDE.md`):** commits con identidad local `hybriqtech` y el trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`; nunca `--no-verify`; nunca modelos Haiku; los tests no tocan un móvil real; el PC Windows no corre `-race`; ficheros de `host/`, `docs/` y `.github/` en LF.

## Reparto de revisiones

| Tarea | Revisor |
|---|---|
| T1 (test), T2, T4, T5, T6, T7 | `adbuddy-reviewer` (Sonnet), con tope de ~40 minutos |
| T3 (`safety.md` y FAQ: afirmaciones de seguridad) | `adbuddy-reviewer-opus` |
| Revisión final de la rama | `adbuddy-reviewer-opus` |

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

Notas para quien implemente: (a) el código usa `strconv.Itoa` para los números de línea; (b) `NewRoot` se construye una vez aquí (los comandos no guardan estado entre `Find` y `ParseFlags` para esta comprobación; si algún flag deja valores que afecten a la siguiente línea y un test lo demuestra, constrúyelo por comando como hace el test de las skills); (c) `strings.HasPrefix(d.Name(), "_") && rel != "."` salta carpetas y ficheros con `_` (como `_config.yml`, `_site`); (d) comprueba que el nombre `docsRepoRoot` y los demás no choquen con otros del paquete `cli`.

- [ ] **Step 2: Ejecutar**

Run (desde `host/`): `go test ./internal/cli/ -run TestDocs -count=1 -timeout 5m -v`
Expected: PASS con las guías de hoy (`README.md`, `docs/index.md`, `docs/setup/index.md`, `docs/setup/es/index.md` existen). Si falla por un comando o enlace real de esas guías, arréglalo **en la guía** (es un fallo real: p. ej. un flag que no existe) y dilo en el informe.

- [ ] **Step 3: Pruebas negativas permanentes**

Añade `TestDocsGuardCatches` en el mismo fichero con una tabla que llame directamente a `checkDocLinks`, `checkDocCommands` y `checkDocPrivacy` (con un directorio temporal para los enlaces) y exija al menos un mensaje por caso: un enlace a `missing.md`, un enlace a `x/` sin `x/index.md`, `` `adbuddy bogus` `` en un bloque ```sh, `adbuddy setup --nope` en un bloque, `$ adbuddy screen extra extra` (argumentos de más) en un bloque, `` `adbuddy setup --nope` `` en línea, una IP `10.0.0.x`, `C:\Users\x`, `emulator-5554`. Y casos que deben dar **cero** mensajes: `adbuddy setup --code --connect 100.64.0.5:41234`, `adbuddy connect <ip:port>`, `adbuddy doctor --json | jq .`, `` adbuddy is free `` en prosa sin comillas, `` `adbuddy` `` solo, un enlace `https://…`, un enlace con ancla `x.md#y` existente, un bloque ```text con `adbuddy bogus` (no es un bloque de shell), la IP `192.0.2.7`.

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

**Fuentes que hay que leer para no inventar:** `scripts/install.sh` (variables, qué comprueba, dónde instala, qué sistemas admite), `.goreleaser.yaml` (nombres reales de los archivos y de `checksums.txt`, formatos por sistema), `docs/releasing.md` (qué incluye una release, `adbuddy.apk` y su `.sha256`), `host/internal/paths` (rutas de configuración y datos por sistema), `host/internal/cli/setup.go` (`--apk`, `--no-app`, qué instala y comprueba), `adbuddy <cmd> --help` de cada comando (compila con `go build -o <tmp> ./cmd/adbuddy` en el directorio de scratch y ejecútalo), `host/internal/workspace/templates/AGENTS.md`, `host/internal/cli/{serve,run,journal,undo,send,remote,connect,attach,new,stop,helpers}.go`, la sección 3.2 y 3.3 de la spec v1 y lo que la app mergeada hace hoy (`android/`: no documentes pantallas de M5c ni nada que no esté en main).

**`docs/install.md`** (spec §3.2): requisitos; Linux y macOS con `curl … | sh` (`ADBUDDY_VERSION`, `ADBUDDY_INSTALL_DIR`, SHA-256, destino por defecto, leer el script antes es una opción: dilo con el enlace al script en GitHub); Windows con el archivo de la release (nombre real de `.goreleaser.yaml`), descomprimir en una carpeta del `PATH`, comprobar el checksum con una orden de PowerShell **correcta** (`Get-FileHash -Algorithm SHA256` y comparar con `checksums.txt`; no la inventes sin comprobar su salida), aviso honesto de SmartScreen (binarios sin firmar todavía); la app la instala `adbuddy setup`; actualizar y desinstalar con las rutas reales por sistema. **No** menciones winget, Homebrew ni IzzyOnDroid como disponibles.

**`docs/usage.md`** (spec §3.3): `serve` y `run` (con la explicación de por qué dos terminales), confirmaciones 🔴, `new`/`attach`/`stop`, el chat desde el móvil (solo lo que ya hace la app mergeada), `journal`/`undo` (y qué no tiene inverso), `send`, ayudas de pantalla, pausar/cortar acceso, modo remoto (`adbuddy remote`, `adbuddy connect`, Tailscale, el puerto 5555 y su caducidad de 12 h: compruébalo con `adbuddy remote --help`), el entorno `~/ADBuddy/<móvil>/` y las skills (`.claude/skills`, `.agents/skills`: M6b), con enlaces a `safety.md`, `troubleshooting.md` y `setup/`.

`requiredDocs` += `docs/install.md`, `docs/usage.md`.

- [ ] Pasos de "Cómo escribir cada guía".

---

### Task 3: `docs/safety.md` y `docs/faq.md`

**Files:** Create `docs/safety.md`, `docs/faq.md`; modify `requiredDocs`.

**Fuentes:** spec v1 §4 (4.1-4.6) y §9; `host/internal/classify` (`rules/*.yaml`, `level.go`, y los tests de `habitual.jsonl` para ejemplos reales de cada nivel: **cita comandos que realmente salen así**: compruébalo con el clasificador), `host/internal/shim`, `host/internal/journal`, `host/internal/precheck` (qué lleva inverso), `host/internal/confirm`, `host/internal/daemon/guard.go` (apps protegidas), `host/internal/phoneapp` (confianza en el certificado: M6a, `docs/superpowers/specs/2026-10-01-adbuddy-m6a-releases-design.md` §5-§6), `host/internal/workspace/templates/AGENTS.md`, `host/internal/remote`, y `docs/superpowers/specs/2026-10-01-adbuddy-m5-ayudas-remoto-design.md` para lo que bloquean las apps protegidas.

**`docs/safety.md`** (spec §3.4): la estructura de la spec, terminando con **"What ADBuddy does not guarantee"** (el aviso honesto con las mismas palabras de fondo que la spec v1 §4.6; modo remoto y 5555; el clasificador no es una caja fuerte; apps protegidas por heurística de nombre y que el usuario edita; Advanced Protection; lo que lee el agente va a su proveedor de modelo) y "Reporting a security problem" (sin inventar canales: si no hay `SECURITY.md`, abrir una *security advisory* privada en GitHub si el repo la tiene habilitada o un issue sin detalles de explotación; lo que falte se anota en el informe como acción del usuario). Sin superlativos.

**`docs/faq.md`** (spec §3.6): las preguntas de la spec; respuestas de 2-4 líneas con enlace; nada de "100 %" ni "imposible".

`requiredDocs` += `docs/safety.md`, `docs/faq.md`.

- [ ] Pasos de "Cómo escribir cada guía". **Esta tarea la revisa un revisor opus frente al código.**

---

### Task 4: `docs/troubleshooting.md`

**Files:** Create `docs/troubleshooting.md`; modify `requiredDocs`.

**Fuentes:** `host/internal/cli/doctor.go` y su `--help` (orden de comprobaciones, `Run:`/`Guidance:`, `--fix`, `--json`, código de salida), `docs/setup/index.md` (la sección "Troubleshooting" existente: no la dupliques; enlázala), y **todos** los códigos de error del host: `grep -rn 'NewError("' host/internal --include=*.go | grep -v _test` (más los de `host/internal/cli/errors.go`: `UsageError`, etc.). Para cada código: causa y arreglo, tomando como arreglo el que el propio host sugiere (tercer argumento de `NewError`). Verifica los mensajes literales (`Denied by user`, `Denied: nobody could confirm`, `Blocked by ADBuddy`, `device offline`, `ADBuddy host not running (run: adbuddy serve)`, `WORKSPACE_FAILED`, `PAIRING_TIMEOUT`…) con `grep` en el código.

**Contenido** (spec §3.5): `adbuddy doctor` (qué comprueba y cómo leerlo, ejemplo de salida **real** generada con un host de pruebas o descrita sin inventar texto: si no puedes ejecutarlo contra un móvil, describe el formato sin simular la salida), la tabla de códigos (con un comentario HTML `<!-- generated by hand from grep 'NewError(' …; refresh when the CLI changes -->`), problemas por síntoma, y cómo reunir información para un issue sin datos privados.

`requiredDocs` += `docs/troubleshooting.md`.

- [ ] Pasos de "Cómo escribir cada guía". En el informe, la lista de códigos encontrados por `grep` y cuáles quedaron fuera (con motivo).

---

### Task 5: guías de preparación del móvil e índice del sitio

**Files:** Modify `docs/setup/index.md`, `docs/setup/es/index.md`, `docs/index.md`.

**Fuentes:** `host/internal/cli/setup.go` (los pasos reales que ejecuta `adbuddy setup`, en orden, y sus mensajes), `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md` §3.1, `docs/superpowers/specs/2026-10-01-adbuddy-m6a-releases-design.md` §5-§6 (instalación y confianza de la app).

**`docs/setup/index.md` y su versión en español** (spec §3.7): tras la sección de emparejamiento y su resolución de problemas, añadir "What happens next" (conexión, instalación de la app con la comprobación del certificado, perfil del móvil, consentimiento en el móvil —solo lo que `setup.go` y la app mergeada hacen—, elección del agente y creación del entorno) y "Next: start ADBuddy" (`adbuddy serve` en una terminal y `adbuddy run` en otra, con enlace a `../usage/`). Mantener los marcadores `pending.svg`. Los enlaces a las guías nuevas se escriben relativos (`../install/`, `../usage/`, `../troubleshooting/`; en español `../../…`). La versión en español debe decir lo mismo que la inglesa, con los textos de menús de Android en español (`Opciones de desarrollador`, `Depuración inalámbrica`…, los mismos que ya usa la guía).

**`docs/index.md`:** portada con el párrafo de una frase y una lista de enlaces: Set up your phone / Prepara tu móvil, Install, Usage, Safety, Troubleshooting, FAQ.

- [ ] Pasos de "Cómo escribir cada guía" (los `requiredDocs` de estas tres ya están).

---

### Task 6: `README.md` y `README.es.md`

**Files:** Modify `README.md`; create `README.es.md`; modify `requiredDocs` (`README.es.md`).

**Fuentes:** la spec §3.1; `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md` §1 (qué es, por qué, ventajas), §2 (arquitectura), §14; `LICENSE`; la versión de la release más reciente (`git tag --sort=-v:refname | head`; si no hay etiquetas, "no public release yet"); las guías que escribieron T2-T5 (el README las resume y enlaza, no las duplica); `adbuddy --help` y `adbuddy <cmd> --help` para la tabla de comandos.

**Contenido:** las secciones de §3.1 en ese orden. "Support the project" lleva la frase de §5 y **un bloque comentado** `<!-- Enable when the pages exist: [GitHub Sponsors](https://github.com/sponsors/hybriqtech) · [Ko-fi](https://ko-fi.com/<handle>) -->` (sin enlace activo). El "Quick start" usa la orden de instalación de `docs/install.md` (la misma) y enlaza a `docs/setup/` por su URL de Pages (`https://hybriqtech.github.io/adbuddy/setup/`) y a las guías por ruta relativa (`docs/install.md`…), que es lo que funciona en GitHub. La comparación con ARTEMIS: una frase verídica (la spec v1 §1: lleva su propio LLM y claves; ADBuddy usa el agente y la suscripción que el usuario ya tiene), sin descalificar.

`README.es.md`: traducción fiel, enlace cruzado en la primera línea de ambos (`[Leer en español](README.es.md)` / `[Read in English](README.md)`); las guías enlazadas son las inglesas salvo `docs/setup/es/`.

- [ ] Pasos de "Cómo escribir cada guía".

---

### Task 7: donaciones, lista previa a hacer público y limpieza

**Files:** Create `.github/FUNDING.yml`; modify `docs/pre-public-checklist.md`, `docs/_config.yml`, `docs/releasing.md`, `docs/superpowers/plans/pendientes-m3-m5.md`.

- [ ] **`.github/FUNDING.yml`**: un comentario de cabecera ("Uncomment when the pages exist; see docs/pre-public-checklist.md") y las claves **comentadas**: `# github: [hybriqtech]` y `# ko_fi: <handle>`. Debe ser YAML válido (solo comentarios es válido; el fichero no debe tener claves activas con identificadores inventados).
- [ ] **`docs/pre-public-checklist.md`**: marcar hecho lo que M6d cierra (README en inglés con traducción al español); añadir puntos: crear el perfil de GitHub Sponsors y la página de Ko-fi y poner los identificadores reales (descomentar `FUNDING.yml` y el bloque de "Support the project" de `README.md` y `README.es.md`; enlazar en "Acerca de" de la app, M5c); la lista de las **capturas reales** que faltan (los marcadores `pending.svg` de `docs/setup/index.md` y de `docs/setup/es/index.md`, con el nombre de fichero propuesto para cada una); habilitar la *private vulnerability reporting* de GitHub o crear un `SECURITY.md` (y enlazarlo desde `docs/safety.md` cuando exista).
- [ ] **`docs/_config.yml`**: añadir `releasing.md` a `exclude` (es un documento de mantenimiento y hoy se publicaría); el comentario dice "Only the user guide is published".
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
