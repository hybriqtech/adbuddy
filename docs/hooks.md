# Hooks de git (prek)

Lo que se puede comprobar en segundos (formato de Go, ficheros mal añadidos) lo pasan los hooks de git en cada commit, y `go vet` y los tests de `host/`, en cada push. Los gestiona [prek](https://github.com/j178/prek), un sustituto de pre-commit en Rust, con la configuración en `.pre-commit-config.yaml`. Esta página es para quien mantiene el repo y no se publica en GitHub Pages (está en `exclude` de `docs/_config.yml`).

**La CI sigue cubriendo lo suyo.** Corre en runners de GitHub y cubre lo que el hook no puede: la matriz de Linux, Windows y macOS, `go test -race` (en Windows necesita cgo), `internal/cli`, y Gradle y el emulador de Android (`android.yml`). Los hooks son un filtro previo, no un sustituto. `security.yml` repite en cada PR los filtros de seguridad del push (ver «Seguridad»), para quien no tiene los hooks instalados.

## Instalación

Una vez por máquina:

```bash
uv tool install prek==0.5.5   # deja prek.exe en ~/.local/bin, que está en el PATH
prek install --allow-missing-config   # desde cualquier worktree; siempre con esta opción
```

prek se instala fuera del repo, y no como dependencia, a propósito. Los hooks viven en el `.git/hooks` común a todos los worktrees y guardan la ruta del `prek` con el que se instalaron. Si fuera el de un worktree, al borrar ese worktree los hooks de todos los demás dejarían de funcionar.

- `--allow-missing-config` es obligatorio. El `.git/hooks` es común a todos los worktrees, y sin la opción el hook falla («No `prek.toml` or `.pre-commit-config.yaml` found») en cualquier commit de una rama sin configuración, incluidos los de otros agentes. Con la opción, esos commits no ejecutan nada: el hook lo detecta y se lo salta.
- Si ya había un hook propio (aquí, el `pre-push` local que bloquea IPs privadas y pushes que no son fast-forward), prek lo mueve a `pre-push.legacy` y lo sigue ejecutando, junto a los suyos. El `commit-msg` local (identidad y trailer) no se toca. Ninguno de los dos está versionado: en un clon nuevo no existen, y `prek install` no los crea.
- Hace falta `uv` (Semgrep) y `podman` o `docker` (gitleaks): ver «Seguridad».
- Hace falta `bash` y `go` en el PATH (Git Bash en Windows). Los hooks de Go llaman a `scripts/hook-go.sh`.

## Qué corre

**Antes de cada commit**, sobre los ficheros que van en él (unos segundos):

- `check-merge-conflict`, `check-case-conflict`, `detect-private-key` y `check-added-large-files` (hasta 1 MB: el mayor fichero versionado es un plan de 600 KB): los de prek, sin dependencias.
- `semgrep (secretos)`: busca claves, tokens y JWT en los ficheros del commit (ver «Seguridad»).
- `gofmt -l` sobre los `.go` de `host/` (como la CI; `spikes/` no se comprueba). Solo lista; si hay algo, el commit se para. Arréglalo con `gofmt -w <fichero>`, `git add` y repite.

**Antes de cada push**, en dos tandas:

1. A la vez: `semgrep` y `gitleaks` (siempre; ver «Seguridad») y, si el push toca `host/`, `go vet ./...` en `host/` (unos 9 s).
2. Si el push toca `host/`, `go test -p 2` de todos los paquetes de `host/` **menos `internal/cli`**, solos y después de `go vet`. Con la máquina cargada, el `go test ./...` entero tarda 150-300 s y la caché de resultados no sirve; `internal/cli` son 143 s, así que solo corre en la CI. Un fallo de `internal/cli` se ve en el PR, no antes del push. Sin `internal/cli`, el hook tarda unos 75 s sin caché y 10-40 s con ella (la caché sí sirve para el resto de paquetes).

`-p 2` y las tandas (`priorities:` en la configuración) limitan la CPU: la máquina la comparten otros agentes.

Los hooks del push comprueban el directorio de trabajo, no los commits que se suben. Deja el árbol limpio antes de hacer push.

Quedan en la CI, no en los hooks: `internal/cli` (incluido `TestDocs*`), `go test -race`, Windows y macOS, y todo lo de `android/`.

## A mano

```bash
prek run                                   # los del commit, sobre lo que está en el índice
prek run --all-files                       # los del commit, sobre todo el repo
prek run --all-files --stage pre-push      # los del push: go vet y go test sin internal/cli
```

Para `internal/cli` a mano: `go test ./internal/cli/` desde `host/`.

## Seguridad

Tres hooks buscan secretos y fallos de seguridad antes de que salgan de la máquina. Ninguno necesita cuenta ni token: Semgrep baja sus reglas del registro público (hace falta red) con las métricas apagadas, y gitleaks corre en un contenedor.

| Hook              | Cuándo | Qué mira                                                                                                     |
| ----------------- | ------ | ------------------------------------------------------------------------------------------------------------ |
| `semgrep-secrets` | commit | Secretos en los ficheros del commit (reglas `p/secrets`).                                                    |
| `semgrep`         | push   | Fallos de seguridad y secretos (`p/default` y `p/secrets`) que la rama añade respecto a `origin/main`.       |
| `gitleaks`        | push   | Secretos en los commits de la rama (`origin/main..HEAD`), también en ficheros que un commit posterior borró. |

`.github/workflows/security.yml` pasa `semgrep` y `gitleaks` en cada PR, sobre lo que el PR añade a su rama base.

Hace falta `uv` (Semgrep se lanza con `uvx`, fijado a una versión en `.pre-commit-config.yaml` y en `security.yml`) y `podman` o `docker` (gitleaks, con la imagen fijada por digest en `scripts/gitleaks.sh`).

`semgrep` compara con `origin/main`: los avisos que ya estaban en el código al añadir el hook (21, sin triar: `exec.Command` con argumentos variables, `Unlock` sin `defer`, actividades exportadas de Android…) no paran el push. Si tocas una de esas líneas, el aviso pasa a ser tuyo.

Cuando un hook avisa:

- **Es un fallo o una clave de verdad:** se arregla la causa. Una clave que ha llegado a un commit se rota aunque el commit no se haya subido, y se saca del historial de la rama antes del push.
- **Es un falso positivo** (un token de relleno de un test, una fixture del protocolo): se marca en su línea, con el motivo, y se dice en el PR. No se salta el hook.
  - Semgrep: `// nosemgrep: <id de la regla> -- <motivo>` en la misma línea o en la anterior.
  - gitleaks: `// gitleaks:allow` en la misma línea. Si la línea no admite comentarios (JSON, un plan), su huella va a `.gitleaksignore`: es el campo `Fingerprint` que imprime el hook.

A mano, sobre todo el repo:

```bash
uvx --from semgrep==1.185.0 semgrep scan --config p/default --config p/secrets --metrics=off   # todo el código, con lo que ya estaba
scripts/gitleaks.sh                                                                            # todo el historial
```

## Saltarse un hook

Solo para una emergencia, y diciéndolo en el commit o en el handoff:

```bash
PREK_SKIP=go-test git push       # salta los hooks con esos ids (separados por comas): gofmt, go-vet, go-test, semgrep…
git commit --no-verify           # salta todos
```
