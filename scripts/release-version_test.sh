#!/bin/sh
# Tests scripts/release-version.sh. Run: sh scripts/release-version_test.sh
set -eu

here=$(dirname "$0")
fail=0
nl='
'

ok() { # tag name code prerelease
	if ! got=$(sh "$here/release-version.sh" "$1" 2>&1); then
		echo "FAIL $1: refused: $got"
		fail=1
		return
	fi
	want="name=$2${nl}code=$3${nl}prerelease=$4"
	if [ "$got" != "$want" ]; then
		printf 'FAIL %s: got\n%s\nwant\n%s\n' "$1" "$got" "$want"
		fail=1
	fi
}

bad() { # tag
	if out=$(sh "$here/release-version.sh" "$1" 2>&1); then
		echo "FAIL $1: accepted: $out"
		fail=1
	fi
}

code() { sh "$here/release-version.sh" "$1" | sed -n 's/^code=//p'; }

ok v0.3.0 0.3.0 300099 false
ok v0.3.0-rc.2 0.3.0-rc.2 300002 true
ok v1.2.3 1.2.3 100200399 false
ok v0.0.0-rc.1 0.0.0-rc.1 1 true
ok v20.999.999 20.999.999 2099999999 false
ok v10.0.0-rc.98 10.0.0-rc.98 1000000098 true

# shellcheck disable=SC2016 # the $(id) is a literal on purpose: it must be refused, not run
for t in 0.3.0 v1.2 v1.2.3.4 v01.2.3 v1.02.3 v1.2.03 v1000.0.0 v21.0.0 \
	v1.2.3-rc.0 v1.2.3-rc.99 v1.2.3-rc.01 v1.2.3-beta.1 v1.2.3-rc.1x v1.2.3-rc \
	'v1.2.3 ' '' 'v1.2.3$(id)' "v1.2.3${nl}v1.2.4"; do
	bad "$t"
done

set +e
sh "$here/release-version.sh" >/dev/null 2>&1
[ $? -eq 2 ] || { echo "FAIL no argument: exit is not 2"; fail=1; }
sh "$here/release-version.sh" a b >/dev/null 2>&1
[ $? -eq 2 ] || { echo "FAIL two arguments: exit is not 2"; fail=1; }
set -e

# An rc comes before its release, which comes before the next rc.
a=$(code v0.3.0-rc.1)
b=$(code v0.3.0-rc.98)
c=$(code v0.3.0)
d=$(code v0.3.1-rc.1)
e=$(code v0.4.0)
if ! { [ "$a" -lt "$b" ] && [ "$b" -lt "$c" ] && [ "$c" -lt "$d" ] && [ "$d" -lt "$e" ]; }; then
	echo "FAIL order: $a $b $c $d $e"
	fail=1
fi

[ "$fail" -eq 0 ] && echo "release-version: all tests pass"
exit "$fail"
