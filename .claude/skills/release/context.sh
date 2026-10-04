#!/usr/bin/env bash
# Gathers what the release skill needs (see SKILL.md): the last published
# release, the version to create, the pull requests and commits merged since,
# and whether the save state format changed. Read-only: it creates nothing.
#
# Usage: context.sh [version]
#   version: the version to create (1.2.3 or v1.2.3, semver, an optional
#   pre-release suffix like -rc.1 included). Without it, the patch number of
#   the last release is incremented.
set -euo pipefail

root=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)
cd "$root"
repo=$(gh repo view --json nameWithOwner -q .nameWithOwner)
git fetch -q --tags origin main

semver='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$'

# The last published release (drafts excluded), by publication date.
last=$(gh release list --exclude-drafts --limit 100 --json tagName,publishedAt \
	-q 'sort_by(.publishedAt) | last | .tagName // ""')
if [[ -z $last ]]; then
	echo "error: no published release to start from" >&2
	exit 1
fi
if ! [[ $last =~ $semver ]]; then
	echo "error: the last release $last is not semver" >&2
	exit 1
fi

if [[ $# -ge 1 && -n $1 ]]; then
	version=$1
	[[ $version == v* ]] || version=v$version
	if ! [[ $version =~ $semver ]]; then
		echo "error: $1 is not a semver version (e.g. 1.2.3, v1.2.3, v2.0.0-rc.1)" >&2
		exit 1
	fi
	reason="given"
else
	[[ $last =~ $semver ]]
	version="v${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.$((BASH_REMATCH[3] + 1))"
	reason="patch increment of $last"
fi

# newer reports whether semver version $1 comes after $2: a higher core
# (major.minor.patch), or the same core when $2 is a pre-release and $1 is
# the release itself or a later pre-release.
newer() {
	local a=${1%%-*} b=${2%%-*}
	if [[ $a != "$b" ]]; then
		[[ $(printf '%s\n%s\n' "$a" "$b" | sort -V | tail -1) == "$a" ]]
		return
	fi
	[[ $2 == *-* ]] || return 1             # same core: only after a pre-release
	[[ $1 != *-* ]] && return 0             # the release after its pre-releases
	[[ $1 != "$2" && $(printf '%s\n%s\n' "$1" "$2" | sort -V | tail -1) == "$1" ]]
}
if ! newer "$version" "$last"; then
	echo "error: $version does not come after the last release $last" >&2
	exit 1
fi
if git rev-parse -q --verify "refs/tags/$version" >/dev/null; then
	echo "error: the tag $version already exists" >&2
	exit 1
fi
existing=$(gh release list --limit 100 --json tagName,isDraft -q ".[] | select(.tagName == \"$version\") | if .isDraft then \"draft\" else \"published\" end")

head=$(git rev-parse origin/main)
prerelease=false
[[ $version == *-* ]] && prerelease=true

echo "repo:          $repo"
echo "last release:  $last ($(git rev-parse --short "$last"))"
echo "version:       $version ($reason)"
echo "prerelease:    $prerelease"
echo "target:        origin/main $head"
echo "existing:      ${existing:-none}"
echo "compare:       https://github.com/$repo/compare/$last...$version"
echo

echo "== Pull requests merged since $last"
prs=$(git log --merges --format=%s "$last..origin/main" | sed -nE 's/^Merge pull request #([0-9]+).*/\1/p' | sort -n)
if [[ -z $prs ]]; then
	echo "(none)"
fi
for pr in $prs; do
	gh pr view "$pr" --json number,title,mergedAt -q '"#\(.number) \(.title) (merged \(.mergedAt[:10]))"'
done
echo

echo "== Commits on main outside pull requests since $last"
git log --first-parent --no-merges --format='%h %s' "$last..origin/main" || true
echo

echo "== Save state format"
old=$(git show "$last:internal/gb/state.go" 2>/dev/null | sed -nE 's/^[[:space:]]*stateVersion[[:space:]]*=[[:space:]]*([0-9]+).*/\1/p')
new=$(git show "origin/main:internal/gb/state.go" | sed -nE 's/^[[:space:]]*stateVersion[[:space:]]*=[[:space:]]*([0-9]+).*/\1/p')
if [[ $old == "$new" ]]; then
	echo "unchanged (stateVersion $new): save states of $version load in $last"
else
	echo "changed (stateVersion $old -> $new): save states of $version do not load in $last"
fi
echo

echo "== Files changed since $last"
git diff --stat "$last..origin/main" | tail -1
git diff --name-only "$last..origin/main" | cut -d/ -f1-2 | sort | uniq -c | sort -rn
