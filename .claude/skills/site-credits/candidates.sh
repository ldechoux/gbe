#!/usr/bin/env bash
# Lists what the site-credits skill needs (see SKILL.md): the projects the
# Credits section of the site thanks today, and the candidates found in the
# repository, then checks every link of the section. Read-only.
#
# Usage: candidates.sh
set -euo pipefail

root=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)
cd "$root"
tmpl=site/index.html.tmpl

section() { sed -n '/<section id="credits"/,/<\/section>/p' "$tmpl"; }

echo "== Credited today (group: card title)"
section | awk '
	/<h3 class="subhead">/ { gsub(/.*subhead">|<\/h3>.*/, ""); group = $0; next }
	/<article class="card">/ { card = 1; next }
	card && /<h3>/ { t = $0; gsub(/<[^>]*>/, "", t); gsub(/^ +| +$/, "", t); print group ": " t; card = 0 }
'
echo

echo "== Direct dependencies (go.mod, indirect ones excluded)"
go list -m -f '{{if not .Indirect}}{{if not .Main}}{{.Path}} {{.Version}}{{end}}{{end}}' all
echo

echo "== Imported by gbe outside tests, tools and the standard library"
go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./cmd/gbe |
	grep -v "^github.com/ldechoux/gbe" | awk -F/ '{print $1"/"$2"/"$3}' | sort -u
echo

echo "== Projects cited in the code and the docs (name: files)"
names='SameBoy|higan|byuu|MMPX|McGuire|Scale2x|Scale3x|AdvanceMAME|blargg|acid2|Matt Currie|RGBDS|gbdev|Pan Docs|mooneye|Gekkio|mGBA|Gambatte|BGB|Hyllian|libretro'
git grep -ohE "$names" -- '*.go' '*.kage' '*.md' '*.yml' 'benchrom/*' ':!.claude' |
	sort | uniq -c | sort -rn
echo
echo "== URLs cited in the code and the docs"
git grep -ohE 'https?://[A-Za-z0-9./_~%-]+' -- '*.go' '*.kage' '*.md' 'benchrom/*' ':!.claude' |
	grep -E '^https?://[^/]+\.' | grep -vE 'ldechoux|localhost|example' | sort -u
echo

echo "== Links of the Credits section without target=\"_blank\" rel=\"noopener\" (should be none)"
section | grep -oE '<a [^>]*>' | grep -v 'target="_blank" rel="noopener"' || echo "(none)"
echo

echo "== Links of the Credits section"
section | grep -oE 'href="https?://[^"]+"' | sed 's/href="//; s/"$//' | sort -u |
	while read -r url; do
		code=$(curl -s -o /dev/null -L --max-time 15 -w '%{http_code}' "$url" || true)
		echo "$code $url"
	done
