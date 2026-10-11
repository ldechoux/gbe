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
# and that of the rendering tests, which run apart. COVERAGE_REPORT=<file>
# also writes the result as Markdown, for the CI to post on the pull
# request and in the summary of its run.
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

status=0
awk -v table="$work/table" '
	FNR == NR {
		if ($0 !~ /^#/ && NF == 2) floor[$1] = $2
		next
	}
	{ measured[$1] = $2; order[++n] = $1 }
	END {
		for (i = 1; i <= n; i++) {
			p = order[i]
			f = "-"
			mark = ""
			if (!(p in floor)) {
				status = "no floor"
			} else if (measured[p] + 0 < floor[p] + 0) {
				status = "UNDER"
				mark = "❌"
				failed++
			} else {
				status = "ok"
				mark = "✅"
			}
			if (p in floor) f = floor[p] "%"
			printf "%-22s %6.1f%%  %5s  %s\n", p, measured[p], f, status
			name = "`" p "`"
			if (p == "total") name = "**total**"
			printf "| %s | %.1f%% | %s | %s |\n", name, measured[p], f, mark >table
		}
		for (p in floor) {
			if (!(p in measured)) {
				printf "%-22s no coverage measured: is it still a package?\n", p
				printf "| `%s` | not measured | %s%% | ❌ |\n", p, floor[p] >table
				failed++
			}
		}
		if (failed) {
			printf "\n%d package(s) under their floor: add tests (see tools/coverage-thresholds.txt)\n", failed
			exit 1
		}
	}
' "$thresholds" "$work/measured" || status=$?

if [ -n "${COVERAGE_REPORT:-}" ]; then
	total=$(awk '$1 == "total" { print $2 }' "$work/measured")
	floor=$(awk '$1 == "total" { print $2 }' "$thresholds")
	under=$(grep -c "❌" "$work/table" || true)
	{
		echo "<!-- gbe-coverage -->"
		echo "## Test coverage"
		echo
		if [ "$status" -eq 0 ]; then
			echo "✅ **Every package keeps its floor**. Total: **${total}%** of the statements (floor ${floor}%)."
		else
			echo "❌ **${under} under their floor**: add tests rather than lower a floor. Total: **${total}%** of the statements (floor ${floor}%)."
		fi
		echo
		echo "| Package | Coverage | Floor | |"
		echo "|---|---:|---:|:-:|"
		cat "$work/table"
		echo
		echo "<sub>The tests and the rendering tests merged, measured without the games and test ROMs, which are not in the repository. The floors are in [tools/coverage-thresholds.txt](${GITHUB_SERVER_URL:-https://github.com}/${GITHUB_REPOSITORY:-ldechoux/gbe}/blob/main/tools/coverage-thresholds.txt).</sub>"
	} >"$COVERAGE_REPORT"
fi
exit "$status"
