#!/usr/bin/env bash
# Takes the captures of the project site (see SKILL.md): save_state_resume,
# game_title and menu, for the Game Boy (Super Mario Land) and the Game Boy
# Color (Link's Awakening DX), and the drop screen (gb/drop_screen), through
# the screenshot rendering of gbe. Writes <out>/{gb,gbc}/<name>.png and
# .webp (lossless), and <out>/montage.png to check them at a glance. The
# repository is left untouched: the temporary harness is removed even on
# failure.
#
# Usage: capture.sh <out dir>
# Environment: SCALE (default 8), FILTER (default lcd), UI_LANG (default fr).
set -euo pipefail

out=${1:?usage: capture.sh <out dir>}
skill=$(cd "$(dirname "$0")" && pwd)
root=$(git -C "$skill" rev-parse --show-toplevel)
cd "$root"

gb_rom=roms/Super_Mario_Land_World_Rev1.gb
gbc_rom=roms/Legend_of_Zelda_The_Links_Awakening_DX.gbc
# The recent games of the drop screen, the first one selected.
recent=("$gbc_rom" "$gb_rom" roms/Tetris_DX.zip roms/Wario_Land_3.zip roms/Donkey_Kong_Country.zip)
for rom in "${recent[@]}"; do
	[[ -f $rom ]] || { echo "missing $rom (the ROMs are not in git: copy them to roms/)" >&2; exit 1; }
done
for tool in go cwebp magick; do
	command -v "$tool" >/dev/null || { echo "missing $tool" >&2; exit 1; }
done
if [[ -e internal/ui/zz_capture.go || -e zz_capture ]]; then
	echo "internal/ui/zz_capture.go or zz_capture/ already exists: remove the leftover harness first" >&2
	exit 1
fi

cleanup() {
	rm -f internal/ui/zz_capture.go
	rm -rf zz_capture
}
trap cleanup EXIT
cp "$skill/harness/capture.go" internal/ui/zz_capture.go
mkdir zz_capture
cp "$skill/harness/main.go" zz_capture/main.go

mkdir -p "$out/gb" "$out/gbc"
common=(-scale "${SCALE:-8}" -filter "${FILTER:-lcd}" -lang "${UI_LANG:-fr}")
# Super Mario Land shows its title screen at once; Link's Awakening DX after
# its intro, which Start skips (pressed at frame 500, title at frame 900).
go run ./zz_capture -rom "$gb_rom" -model gb -title-at 200 -out "$out/gb" "${common[@]}"
go run ./zz_capture -rom "$gbc_rom" -model gbc -title-at 900 -start-at 500 -out "$out/gbc" "${common[@]}"
go run ./zz_capture -drop -out "$out/gb" "${common[@]}" "${recent[@]}"

for console in gb gbc; do
	for name in game_title menu save_state_resume drop_screen; do
		[[ -f $out/$console/$name.png ]] || continue # drop_screen: gb only
		cwebp -quiet -lossless -z 9 "$out/$console/$name.png" -o "$out/$console/$name.webp"
	done
done
magick \( "$out"/gb/{game_title,menu,save_state_resume}.png +append \) \
	\( "$out"/gbc/{game_title,menu,save_state_resume}.png "$out"/gb/drop_screen.png +append \) \
	-append -resize 30% "$out/montage.png"

ls -l "$out"/gb/*.webp "$out"/gbc/*.webp
echo "montage: $out/montage.png"
