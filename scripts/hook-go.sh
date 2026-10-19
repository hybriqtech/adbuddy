#!/usr/bin/env bash
# Hooks de git de Go (ver docs/hooks.md y .pre-commit-config.yaml).
#   hook-go.sh gofmt <ficheros.go…>   lista los mal formateados y falla si hay alguno (no reescribe)
#   hook-go.sh vet                    go vet ./... en host/
#   hook-go.sh test                   go test -p 2 de todos los paquetes de host/ menos internal/cli
set -euo pipefail

# Desde un worktree enlazado, git exporta GIT_DIR y similares a los hooks. Los tests que llaman a git
# (p. ej. scripts/release-version_test.sh) los obedecerían y operarían sobre el repo del push.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_PREFIX GIT_COMMON_DIR GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cmd="${1:-}"
[ "$#" -gt 0 ] && shift

case "$cmd" in
gofmt)
	[ "$#" -gt 0 ] || exit 0
	out="$(gofmt -l "$@")"
	if [ -n "$out" ]; then
		printf 'gofmt: ficheros sin formatear (arréglalos con gofmt -w):\n%s\n' "$out" >&2
		exit 1
	fi
	;;
vet)
	cd "$root/host"
	go vet ./...
	;;
test)
	cd "$root/host"
	# tr -d '\r': go list en Windows puede terminar las líneas en CRLF.
	pkgs="$(go list ./... | tr -d '\r' | grep -v '/internal/cli$')"
	# shellcheck disable=SC2086 # una ruta de paquete por palabra, sin espacios
	go test -p 2 $pkgs
	;;
*)
	echo "uso: hook-go.sh gofmt <ficheros…> | vet | test" >&2
	exit 2
	;;
esac
