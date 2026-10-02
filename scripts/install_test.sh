#!/bin/sh
# Tests scripts/install.sh against a fake release in a temporary folder
# (file:// URLs) and a fake uname. Run: sh scripts/install_test.sh
set -eu

here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT INT TERM
fail=0

# sha prints "<hex>  <file>" like goreleaser (Git Bash marks binary files with " *").
sha() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$@"; else shasum -a 256 "$@"; fi | sed 's/^\([0-9a-f]*\) [ *]/\1  /'; }

# url turns a folder into a file:// URL that this system's curl understands.
url() {
	if command -v cygpath >/dev/null 2>&1; then
		printf 'file:///%s' "$(cygpath -m "$1")"
	else
		printf 'file://%s' "$1"
	fi
}

# A release with an archive for each target; each fake adbuddy says its target.
rel=$work/release
mkdir -p "$rel"
for target in linux_amd64 linux_arm64 darwin_all; do
	mkdir -p "$work/pkg/$target"
	printf '#!/bin/sh\necho "adbuddy 9.9.9 %s"\n' "$target" >"$work/pkg/$target/adbuddy"
	printf 'extra\n' >"$work/pkg/$target/README.md"
	chmod 755 "$work/pkg/$target/adbuddy"
	tar -czf "$rel/adbuddy_9.9.9_${target}.tar.gz" -C "$work/pkg/$target" adbuddy README.md
done
printf 'zip\n' >"$rel/adbuddy_9.9.9_windows_amd64.zip"
(cd "$rel" && sha adbuddy_9.9.9_*) >"$rel/checksums.txt"

# fakeuname prints a folder whose uname answers $1 to -s and $2 to -m.
fakeuname() {
	d=$work/uname-$1-$2
	mkdir -p "$d"
	# shellcheck disable=SC2016 # the $1 is for the fake uname to expand, not for this shell
	printf '#!/bin/sh\ncase "$1" in -s) echo %s ;; -m) echo %s ;; esac\n' "$1" "$2" >"$d/uname"
	chmod 755 "$d/uname"
	printf '%s' "$d"
}

# try os arch base dir [version]: runs install.sh; its output goes to $work/out.
try() {
	PATH="$(fakeuname "$1" "$2"):$PATH" ADBUDDY_DOWNLOAD_URL="$3" ADBUDDY_INSTALL_DIR="$4" ADBUDDY_VERSION="${5:-latest}" \
		sh "$here/install.sh" >"$work/out" 2>&1
}

# refused name os arch base dir [version]: install.sh must fail.
refused() {
	name=$1
	shift
	if try "$@"; then
		echo "FAIL $name: accepted"
		sed 's/^/    /' "$work/out"
		fail=1
	fi
}

says() { # name text
	if ! grep -q "$2" "$work/out"; then
		echo "FAIL $1: no \"$2\" in:"
		sed 's/^/    /' "$work/out"
		fail=1
	fi
}

for c in "Linux x86_64 linux_amd64" "Linux aarch64 linux_arm64" "Darwin arm64 darwin_all" "Darwin x86_64 darwin_all"; do
	# shellcheck disable=SC2086 # $c is split on purpose: os, arch and target
	set -- $c
	dir=$work/bin-$1-$2
	if ! try "$1" "$2" "$(url "$rel")" "$dir"; then
		echo "FAIL $1 $2: refused"
		sed 's/^/    /' "$work/out"
		fail=1
		continue
	fi
	"$dir/adbuddy" | grep -q "$3" || { echo "FAIL $1 $2: wrong binary"; fail=1; }
	[ ! -e "$dir/README.md" ] || { echo "FAIL $1 $2: extracted more than adbuddy"; fail=1; }
done
says "warns about PATH" "not in your PATH"
says "warns about the origin" "not the official release"

refused "riscv64" Linux riscv64 "$(url "$rel")" "$work/bin-riscv"
says "riscv64" "no build for Linux riscv64"
refused "Windows" MINGW64_NT-10.0 x86_64 "$(url "$rel")" "$work/bin-win"
says "Windows" "Windows"
refused "bad version" Linux x86_64 "$(url "$rel")" "$work/bin-v" 'v1.2.3;rm'
says "bad version" "not a release tag"

# A tampered archive is not installed.
bad=$work/bad
cp -r "$rel" "$bad"
printf 'evil\n' >>"$bad/adbuddy_9.9.9_linux_amd64.tar.gz"
refused "tampered" Linux x86_64 "$(url "$bad")" "$work/bin-bad"
says "tampered" "does not match its SHA-256"
[ ! -e "$work/bin-bad/adbuddy" ] || { echo "FAIL tampered: installed"; fail=1; }

# A checksums line whose name only ends like ours does not count.
odd=$work/odd
mkdir -p "$odd"
cp "$rel"/adbuddy_9.9.9_linux_amd64.tar.gz "$odd/"
(cd "$odd" && sha adbuddy_9.9.9_linux_amd64.tar.gz | sed 's/  /  x/') >"$odd/checksums.txt"
refused "no exact line" Linux x86_64 "$(url "$odd")" "$work/bin-odd"
says "no exact line" "no adbuddy archive for linux_amd64"

# It needs curl (BusyBox wget cannot promise HTTPS-only redirects) and pins HTTPS on every hop.
if grep -v "^#" "$here/install.sh" | grep -q wget; then echo "FAIL install.sh still uses wget"; fail=1; fi
grep -q -- '--proto "=https" --proto-redir "=https"' "$here/install.sh" || { echo "FAIL install.sh does not pin HTTPS on redirections"; fail=1; }

[ "$fail" -eq 0 ] && echo "install: all tests pass"
exit "$fail"
