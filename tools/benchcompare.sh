#!/usr/bin/env bash
# Runs the benchmarks of two checkouts of gbe on this machine, interleaved,
# and writes their results to <out>/base.txt and <out>/head.txt for
# tools/benchreport. The CI uses it on every pull request; locally:
#
#   git worktree add /tmp/gbe-main main
#   tools/benchcompare.sh /tmp/gbe-main . 10 /tmp/bench
#   go run ./tools/benchreport /tmp/bench/base.txt /tmp/bench/head.txt
#
# The test binaries are built with the PGO profile of each checkout, like
# the released binaries. The runs alternate between the two sides, so that a
# machine slowing down or speeding up during the runs affects both alike.
#
# By default, each checkout runs its own benchmarks (internal/gb and
# internal/ui), which is right for a pull request against its base branch.
# HARNESS=<dir> runs the benchmarks of <dir>, normally bench/, in both
# checkouts instead, with the benchmark ROM next to it: the same
# benchmarks then measure two versions of any age, releases older than the
# benchmarks included. For instance, to compare the release v0.1.4 with main:
#
#   git worktree add /tmp/gbe-v0.1.4 v0.1.4
#   HARNESS=bench tools/benchcompare.sh /tmp/gbe-v0.1.4 /tmp/gbe-main 10 /tmp/bench
#
# BENCH (default ".") selects the benchmarks, BENCHTIME (default 0.5s) sets
# how long each one runs.
set -euo pipefail

if [ $# -lt 2 ]; then
	echo "usage: $0 <base checkout> <head checkout> [runs=10] [out=bench-out]" >&2
	exit 2
fi
base=$(cd "$1" && pwd)
head=$(cd "$2" && pwd)
runs=${3:-10}
mkdir -p "${4:-bench-out}"
out=$(cd "${4:-bench-out}" && pwd)
packages=(internal/gb internal/ui)

if [ -n "${HARNESS:-}" ]; then
	harness=$(cd "$HARNESS" && pwd)
	rom=$(cd "$harness/.." && pwd)/benchrom/gbe-bench.gbc
	for tree in "$base" "$head"; do
		dst="$tree/bench"
		if [ "$dst" != "$harness" ]; then
			rm -rf "$dst"
			mkdir -p "$dst"
			# Releases before v0.1.2 have another module path.
			module=$(awk '$1 == "module" { print $2 }' "$tree/go.mod")
			for f in "$harness"/*.go; do
				sed "s#\"github.com/ldechoux/gbe/#\"$module/#" "$f" >"$dst/$(basename "$f")"
			done
		fi
		# The same ROM for both sides.
		mkdir -p "$dst/testdata"
		cp "$rom" "$dst/testdata/gbe-bench.gbc"
	done
	packages=(bench)
fi

build() { # checkout side
	local pgo=off
	if [ -f "$1/cmd/gbe/default.pgo" ]; then
		pgo="$1/cmd/gbe/default.pgo"
	fi
	for p in "${packages[@]}"; do
		(cd "$1" && go test -c -pgo="$pgo" -o "$out/$2-$(basename "$p").test" "./$p")
	done
}

# The games and the test ROMs are not committed: share those of the head
# checkout, so that their benchmarks run on both sides.
for d in roms testroms; do
	if [ -d "$head/$d" ] && [ ! -e "$base/$d" ]; then
		ln -s "$head/$d" "$base/$d"
	fi
done

build "$base" base
build "$head" head
: >"$out/base.txt"
: >"$out/head.txt"
for i in $(seq "$runs"); do
	echo "run $i/$runs" >&2
	for side in base head; do
		tree=$base
		if [ "$side" = head ]; then
			tree=$head
		fi
		for p in "${packages[@]}"; do
			# From the package directory: the tests find their ROMs there.
			(cd "$tree/$p" && "$out/$side-$(basename "$p").test" -test.run '^$' \
				-test.bench "${BENCH:-.}" -test.benchtime "${BENCHTIME:-0.5s}" \
				-test.benchmem -test.count 1) >>"$out/$side.txt"
		done
	done
done
