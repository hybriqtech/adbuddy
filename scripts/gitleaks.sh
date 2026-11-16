#!/usr/bin/env bash
# Busca secretos en los commits con gitleaks, en un contenedor y sin cuenta en ningún servicio. Ver docs/hooks.md.
#
# Uso: scripts/gitleaks.sh [opciones de gitleaks]
#   scripts/gitleaks.sh                                 todo el historial (la CI de cada noche)
#   scripts/gitleaks.sh --log-opts=origin/main..HEAD    solo los commits de la rama (el hook del push)
set -euo pipefail

# Fijada por digest (v8.28.0): una etiqueta se puede mover. Para subir de versión, cambia las dos cosas.
IMAGEN='ghcr.io/gitleaks/gitleaks@sha256:bf00b5e039f0fad4b32935dc5ec1e358f227ccd097bcb64b971f0331072fe2ae'

motor=$(command -v podman || command -v docker) || {
  echo "gitleaks.sh: hace falta podman o docker" >&2
  exit 1
}
# En un worktree, .git es un fichero que apunta al .git común: se montan los dos, en su misma ruta y solo lectura.
raiz=$(git rev-parse --show-toplevel)
comun=$(git rev-parse --path-format=absolute --git-common-dir)

# --redact --verbose: dice fichero, línea, regla y huella de cada hallazgo, sin imprimir el secreto.
# label=disable: sin él, SELinux no deja leer el repo y la alternativa (:Z) le cambia la etiqueta a todos los ficheros.
exec "$motor" run --rm --security-opt label=disable \
  -v "$raiz:$raiz:ro" -v "$comun:$comun:ro" -w "$raiz" \
  "$IMAGEN" git . --no-banner --redact --verbose "$@"
