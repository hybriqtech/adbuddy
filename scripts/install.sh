#!/bin/sh
# Installs the adbuddy host on Linux or macOS (spec M6a §8):
#
#   curl -fsSL https://github.com/hybriqtech/adbuddy/releases/latest/download/install.sh | sh
#
# ADBUDDY_VERSION=v0.3.0 installs that release instead of the latest one.
# ADBUDDY_INSTALL_DIR picks the folder (default: ~/.local/bin; no sudo).
# ADBUDDY_DOWNLOAD_URL replaces the release URL, for tests only: the
# checksums come from the same place as the archive.
#
# Everything runs from main, called on the last line, so a cut download
# runs nothing.
set -eu

say() { printf 'adbuddy install: %s\n' "$*" >&2; }
die() {
	say "$*"
	exit 1
}
have() { command -v "$1" >/dev/null 2>&1; }

# sum file: the SHA-256 and the name, as sha256sum prints them.
sum() { if have sha256sum; then sha256sum "$@"; else shasum -a 256 "$@"; fi; }

# fetch url file: HTTPS only, unless the URL is a test's.
fetch() {
	if have curl; then
		if [ -n "${ADBUDDY_DOWNLOAD_URL:-}" ]; then
			curl -fsSL -o "$2" "$1"
		else
			curl --proto '=https' --tlsv1.2 -fsSL -o "$2" "$1"
		fi
	elif [ -n "${ADBUDDY_DOWNLOAD_URL:-}" ]; then
		wget -q -O "$2" "$1"
	else
		wget --https-only -q -O "$2" "$1"
	fi
}

main() {
	have curl || have wget || die "needs curl or wget"
	have tar || die "needs tar"
	have awk || die "needs awk"
	have sha256sum || have shasum || die "needs sha256sum or shasum"

	os=$(uname -s)
	arch=$(uname -m)
	case $os in
	Linux)
		case $arch in
		x86_64 | amd64) target=linux_amd64 ;;
		aarch64 | arm64) target=linux_arm64 ;;
		*) die "no build for Linux $arch" ;;
		esac
		;;
	Darwin) target=darwin_all ;;
	MINGW* | MSYS* | CYGWIN*) die "on Windows, download the .zip from https://github.com/hybriqtech/adbuddy/releases (see host/README.md)" ;;
	*) die "no build for $os" ;;
	esac

	version=${ADBUDDY_VERSION:-latest}
	if [ "$version" != latest ]; then
		printf '%s\n' "$version" | grep -Eq '^v(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})(-rc\.[1-9][0-9]?)?$' ||
			die "ADBUDDY_VERSION=$version is not a release tag (vX.Y.Z or vX.Y.Z-rc.N)"
	fi
	if [ -n "${ADBUDDY_DOWNLOAD_URL:-}" ]; then
		base=${ADBUDDY_DOWNLOAD_URL%/}
		say "downloading from $base: not the official release"
	elif [ "$version" = latest ]; then
		base=https://github.com/hybriqtech/adbuddy/releases/latest/download
	else
		base=https://github.com/hybriqtech/adbuddy/releases/download/$version
	fi

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT INT TERM
	fetch "$base/checksums.txt" "$tmp/checksums.txt" || die "could not download $base/checksums.txt"
	name=$(awk -v t="$target" '$2 ~ "^adbuddy_[^/ ]+_" t "[.]tar[.]gz$" { print $2 }' "$tmp/checksums.txt")
	[ -n "$name" ] && [ "$(printf '%s\n' "$name" | wc -l | tr -d ' ')" -eq 1 ] ||
		die "no adbuddy archive for $target in $base/checksums.txt"
	want=$(awk -v f="$name" '$2 == f { print $1 }' "$tmp/checksums.txt")
	fetch "$base/$name" "$tmp/$name" || die "could not download $base/$name"
	got=$(cd "$tmp" && sum "$name")
	got=${got%% *}
	[ "$got" = "$want" ] || die "$name does not match its SHA-256 in checksums.txt: not installing it"

	mkdir "$tmp/x"
	tar -xzf "$tmp/$name" -C "$tmp/x" adbuddy || die "$name has no adbuddy inside"
	dir=${ADBUDDY_INSTALL_DIR:-$HOME/.local/bin}
	mkdir -p "$dir"
	cp "$tmp/x/adbuddy" "$dir/.adbuddy.new"
	chmod 755 "$dir/.adbuddy.new"
	mv -f "$dir/.adbuddy.new" "$dir/adbuddy"
	say "installed $("$dir/adbuddy" --version 2>/dev/null || echo adbuddy) in $dir"
	case ":$PATH:" in
	*":$dir:"*) ;;
	*) say "$dir is not in your PATH: add it to run adbuddy" ;;
	esac
}

main "$@"
