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
	grep -q '^Verified using v1 scheme (JAR signing): false' "$out" || die "$1 has a v1 signature"
	grep -q '^Verified using v2 scheme (APK Signature Scheme v2): true' "$out" || die "$1 has no v2 signature"
	grep -q '^Verified using v3 scheme (APK Signature Scheme v3): true' "$out" || die "$1 has no v3 signature"
	signers=$(grep -c '^Signer #[0-9]* certificate SHA-256 digest: ' "$out" || true)
	[ "$signers" -eq 1 ] || die "$1 has $signers signers, not 1"
	theirs=$(sed -n 's/^Signer #1 certificate SHA-256 digest: //p' "$out")
	ours=$(cd "$root/host" && go run ./cmd/apkcert "$apk") || die "the host refuses $1"
	[ "$ours" = "$theirs" ] || die "apksigner says $theirs, the host says $ours"
	if [ "${2:-}" = --pinned ]; then
		pinned=$(cd "$root/host" && go run ./cmd/apkcert -pinned) || die "the host pins no certificate: see docs/releasing.md"
		[ "$ours" = "$pinned" ] || die "$1 is signed by $ours, but the host pins $pinned"
	fi
	echo "release-cert: $1 is signed by $ours"
}

main "$@"
