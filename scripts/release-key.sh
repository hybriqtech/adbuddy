#!/bin/sh
# Creates the app's release signing key, once, OUTSIDE the repo
# (docs/releasing.md). Run it yourself in your terminal, never through a
# coding agent. keytool asks for the password: it never goes on a command
# line. Needs keytool: a JDK (winget install EclipseAdoptium.Temurin.21.JDK)
# or Android Studio's jbr/bin.
#
#   sh scripts/release-key.sh
set -eu

die() {
	echo "release-key: $*" >&2
	exit 1
}

main() {
	umask 077
	keytool=keytool
	if ! command -v keytool >/dev/null 2>&1; then
		# shellcheck disable=SC2015 # die exits, so A && B || die is an if-then-else here
		[ -n "${JAVA_HOME:-}" ] && [ -x "$JAVA_HOME/bin/keytool" ] || die "keytool not found: install a JDK or set JAVA_HOME"
		keytool=$JAVA_HOME/bin/keytool
	fi
	dir=${ADBUDDY_RELEASE_KEY_DIR:-$HOME/.adbuddy-release}
	mkdir -p "$dir"
	dir=$(cd "$dir" && pwd)
	# git itself answers, so the path spellings of Git Bash and Windows do not matter.
	if [ "$(git -C "$dir" rev-parse --is-inside-work-tree 2>/dev/null || true)" = true ]; then
		die "$dir is inside a git repository: pick a folder outside it (ADBUDDY_RELEASE_KEY_DIR)"
	fi
	ks=$dir/release.p12
	[ ! -e "$ks" ] || die "$ks already exists: never replace a release key (installed apps could not update)"
	chmod 700 "$dir"
	"$keytool" -genkeypair -keystore "$ks" -storetype PKCS12 -alias adbuddy \
		-keyalg RSA -keysize 4096 -validity 10950 -dname "CN=hybriqtech ADBuddy, O=hybriqtech"
	chmod 600 "$ks"
	echo
	echo "Key created: $ks"
	echo "Back it up with its password somewhere safe. If it is lost, no release can update the app."
	echo
	echo "Certificate SHA-256 (public: it goes into host/internal/phoneapp/release.go):"
	"$keytool" -list -v -keystore "$ks" -storetype PKCS12 -alias adbuddy |
		sed -n 's/^[[:space:]]*SHA256: //p' | tr -d ':' | tr 'A-F' 'a-f'
	echo
	echo "Secrets of the \"release\" environment (create it first, see docs/releasing.md);"
	echo "gh reads each value from stdin or asks for it, and prints nothing:"
	printf '%s\n' "  base64 < \"$ks\" | tr -d '\\n' | gh secret set ADBUDDY_KEYSTORE_B64 --env release --repo hybriqtech/adbuddy"
	echo "  gh secret set ADBUDDY_KEYSTORE_PASSWORD --env release --repo hybriqtech/adbuddy"
}

main "$@"
