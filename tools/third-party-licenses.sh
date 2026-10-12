#!/usr/bin/env bash
# Writes the licenses of the code linked into gbe, which its binaries must
# come with: Go's (the runtime and the standard library), then those of the
# modules, for every release platform (macOS, Linux and Windows, amd64 and
# arm64), with their NOTICE files when they have one.
#
#	tools/third-party-licenses.sh THIRD_PARTY_LICENSES.txt
#
# Fails when a module has no license file: a new dependency must have one.
set -euo pipefail

out=${1:?usage: third-party-licenses.sh <out file>}
cd "$(git rev-parse --show-toplevel)"

modules=$(
	for os in darwin linux windows; do
		for arch in amd64 arm64; do
			CGO_ENABLED=0 GOOS=$os GOARCH=$arch go list -deps \
				-f '{{with .Module}}{{if not .Main}}{{.Path}}{{end}}{{end}}' ./cmd/gbe
		done
	done | sort -u
)

section() { # <title> <file>
	printf '\n%s\n%s\n\n' "================================================================" "$1"
	cat "$2"
}

{
	echo "Third-party licenses of gbe"
	echo
	echo "The binaries of gbe contain the code below, under these licenses."
	section "Go ($(go env GOVERSION)), https://go.dev" "$(go env GOROOT)/LICENSE"
	for m in $modules; do
		dir=$(go list -m -f '{{.Dir}}' "$m")
		version=$(go list -m -f '{{.Version}}' "$m")
		files=$(find "$dir" -maxdepth 1 -type f \( -iname 'LICENSE*' -o -iname 'LICENCE*' -o -iname 'COPYING*' -o -iname 'UNLICENSE*' -o -iname 'NOTICE*' \) | sort)
		if [[ -z $files ]]; then
			echo "no license file in $m ($dir)" >&2
			exit 1
		fi
		for f in $files; do
			section "$m $version: $(basename "$f")" "$f"
		done
	done
} >"$out"
