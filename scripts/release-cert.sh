#!/bin/sh
# Checks the signing certificate of an APK two ways, apksigner and the
# host's own parser (host/cmd/apkcert, every check of spec M6a §5), and
# with --pinned also against the certificate the host pins:
#
#   APKSIGNER=$ANDROID_HOME/build-tools/37.0.0/apksigner sh scripts/release-cert.sh app.apk [--pinned]
set -eu

die() {
	echo "release-cert: $*" >&2
	exit 1
}

main() {
	# shellcheck disable=SC2015 # die exits, so A && B || die is an if-then-else here
	[ $# -ge 1 ] && [ $# -le 2 ] || die "usage: release-cert.sh <file.apk> [--pinned]"
	case ${2:---} in
	-- | --pinned) ;;
	*) die "unknown option $2" ;;
	esac
	[ -n "${APKSIGNER:-}" ] || die "set APKSIGNER to the apksigner of the build-tools"
	apk=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
	root=$(cd "$(dirname "$0")/.." && pwd)
	out=$(mktemp)
	trap 'rm -f "$out"' EXIT INT TERM

	"$APKSIGNER" verify --verbose --print-certs "$apk" >"$out" || die "apksigner does not verify $1"
	fail() {
		cat "$out" >&2
		die "$@"
	}
	grep -q '^Verified using v1 scheme (JAR signing): false' "$out" || fail "$1 has a v1 signature"
	# apksigner leaves v2 out when v3 is there and minSdk is 28 or more, so
	# one of the two is enough; either way every signer line is the same one.
	grep -Eq '^Verified using v[23] scheme \(APK Signature Scheme v[23]\): true' "$out" || fail "$1 has no v2 or v3 signature"
	theirs=$(grep -E '^(Signer #[0-9]+ certificate|V3\.[0-9] Signer: certificate) SHA-256 digest: ' "$out" | sed 's/.*digest: //' | sort -u)
	[ "$(printf '%s\n' "$theirs" | grep -c .)" -eq 1 ] || fail "$1 has not exactly 1 signing certificate"
	ours=$(cd "$root/host" && go run ./cmd/apkcert "$apk") || die "the host refuses $1"
	[ "$ours" = "$theirs" ] || die "apksigner says $theirs, the host says $ours"
	if [ "${2:-}" = --pinned ]; then
		pinned=$(cd "$root/host" && go run ./cmd/apkcert -pinned) || die "the host pins no certificate: see docs/releasing.md"
		[ "$ours" = "$pinned" ] || die "$1 is signed by $ours, but the host pins $pinned"
	fi
	echo "release-cert: $1 is signed by $ours"
}

main "$@"
