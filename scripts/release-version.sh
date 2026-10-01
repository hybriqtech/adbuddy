#!/bin/sh
# Turns a release tag into the app's versionName and versionCode
# (spec M6a §3):
#
#   sh scripts/release-version.sh v0.3.0
#   name=0.3.0
#   code=300099
#   prerelease=false
#
# Tags: vX.Y.Z or vX.Y.Z-rc.N, with X <= 20, Y and Z <= 999 and 1 <= N <= 98.
# versionCode = ((X*1000 + Y)*1000 + Z)*100 + S, where S is N for an rc and 99
# for the release, so an rc always comes before its release.
set -eu

main() {
	if [ $# -ne 1 ]; then
		echo "usage: release-version.sh vX.Y.Z[-rc.N]" >&2
		exit 2
	fi
	tag=$1
	# grep matches line by line: refuse a tag with a newline first.
	case $tag in
	*"
"*) bad "$tag" ;;
	esac
	printf '%s\n' "$tag" | grep -Eq '^v(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})(-rc\.[1-9][0-9]?)?$' || bad "$tag"
	v=${tag#v}
	core=${v%%-*}
	x=${core%%.*}
	rest=${core#*.}
	y=${rest%%.*}
	z=${rest#*.}
	case $v in
	*-rc.*)
		s=${v##*-rc.}
		[ "$s" -le 98 ] || bad "$tag" "rc numbers go from 1 to 98"
		pre=true
		;;
	*)
		s=99
		pre=false
		;;
	esac
	[ "$x" -le 20 ] || bad "$tag" "the major version is over 20, so the versionCode would not fit"
	code=$((((x * 1000 + y) * 1000 + z) * 100 + s))
	printf 'name=%s\ncode=%s\nprerelease=%s\n' "$v" "$code" "$pre"
}

bad() {
	echo "release-version: \"$1\" is not a release tag (vX.Y.Z or vX.Y.Z-rc.N)${2:+: $2}" >&2
	exit 1
}

main "$@"
