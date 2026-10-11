#!/usr/bin/env bash
# Checks the test coverage of every package against its floor in
# tools/coverage-thresholds.txt, and fails when one is under it.
#
#   tools/coverage.sh                   runs the tests, then checks
#   GBE_RENDER=1 tools/coverage.sh      the rendering tests too (a display is needed)
#   tools/coverage.sh <dir>...          checks coverage data already written by
#                                       go test -cover ... -args -test.gocoverdir=<dir>
#
# The coverage of the runs given is merged: the CI gives that of the tests
# and that of the rendering tests, which run apart. In GitHub Actions, the
# table also goes to the summary of the run.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
thresholds=tools/coverage-thresholds.txt
module=$(go list -m)

dirs=("$@")
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
if [ ${#dirs[@]} -eq 0 ]; then
	mkdir -p "$work/unit"
	GBE_RENDER= go test -count=1 -cover ./... -args -test.gocoverdir="$work/unit" >/dev/null
	dirs=("$work/unit")
	if [ -n "${GBE_RENDER:-}" ]; then
		mkdir -p "$work/render"
		go test -count=1 -cover ./internal/ui/... -args -test.gocoverdir="$work/render" >/dev/null
		dirs+=("$work/render")
	fi
fi
inputs=$(IFS=,; echo "${dirs[*]}")

# One line per package: "<package> <percent>", then the total.
go tool covdata percent -i="$inputs" |
	awk -v m="$module/" '$2 == "coverage:" { sub(m, "", $1); sub("%", "", $3); print $1, $3 }' >"$work/measured"
go tool covdata textfmt -i="$inputs" -o "$work/profile"
go tool cover -func="$work/profile" | awk '$1 == "total:" { sub("%", "", $3); print "total", $3 }' >>"$work/measured"

awk -v summary="${GITHUB_STEP_SUMMARY:-/dev/null}" '
	FNR == NR {
		if ($0 !~ /^#/ && NF == 2) floor[$1] = $2
		next
	}
	{ measured[$1] = $2; order[++n] = $1 }
	END {
		print "| Package | Coverage | Floor | |" >summary
		print "|---|---:|---:|:-:|" >summary
		for (i = 1; i <= n; i++) {
			p = order[i]
			if (!(p in floor)) {
				status = "no floor"
			} else if (measured[p] + 0 < floor[p] + 0) {
				status = "UNDER"
				failed++
			} else {
				status = "ok"
			}
			f = "-"
			if (p in floor) f = floor[p] "%"
			mark = ""
			if (status == "ok") mark = "✅"
			if (status == "UNDER") mark = "❌"
			printf "%-22s %6.1f%%  %5s  %s\n", p, measured[p], f, status
			printf "| `%s` | %.1f%% | %s | %s |\n", p, measured[p], f, mark >summary
		}
		for (p in floor) {
			if (!(p in measured)) {
				printf "%-22s no coverage measured: is it still a package?\n", p
				failed++
			}
		}
		if (failed) {
			printf "\n%d package(s) under their floor: add tests (see tools/coverage-thresholds.txt)\n", failed
			exit 1
		}
	}
' "$thresholds" "$work/measured"
