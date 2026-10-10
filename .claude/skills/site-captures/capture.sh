#!/usr/bin/env bash
# Takes the captures of the project site (see SKILL.md), through the
# rendering of gbe, by groups:
#   hero       gb/screenshot, gbc/screenshot (the top of the page)
#   palettes   gb/palette_{dmg,bgb,sepia,violet}
#   colorized  gbc/colorized_{tetris,zelda,sml,tetris_leftb}
#   filters    {gb,gbc}/filter_lcd, gb/filter_{nearest,scale2x,scale3x,mmpx}
#   scenes     {gb,gbc}/{game_title,menu,save_state_resume} (Captures section)
#   drop       gb/drop_screen (quick start)
# Writes <out>/{gb,gbc}/<name>.png and .webp (lossless), and
# <out>/montage.png to check them at a glance. The repository is left
# untouched: the temporary harness is removed even on failure.
#
# Usage: capture.sh <out dir> [group...]   (all groups by default)
# Environment, for the scenes and the drop screen only: SCALE (default 8),
# FILTER (default lcd), UI_LANG (default fr).
set -euo pipefail

out=${1:?usage: capture.sh <out dir> [group...]}
shift
groups=("$@")
[[ ${#groups[@]} -gt 0 ]] || groups=(hero palettes colorized filters scenes drop)
want() { [[ " ${groups[*]} " == *" $1 "* ]]; }

skill=$(cd "$(dirname "$0")" && pwd)
root=$(git -C "$skill" rev-parse --show-toplevel)
cd "$root"
out=$(mkdir -p "$out" && cd "$out" && pwd)

sml=roms/Super_Mario_Land_World_Rev1.gb
ladx=roms/Legend_of_Zelda_The_Links_Awakening_DX.gbc
la=roms/Legend_of_Zelda_The_Links_Awakening.gb
tetris=roms/Tetris_World_Rev1.gb
# The recent games of the drop screen, the first one selected, with the
# names the lists show (the file names of the usual collections).
recent=(
	"$ladx=Zelda - Link's Awakening DX"
	"$sml=Super Mario Land (World)"
	"roms/Tetris_DX.zip=Tetris DX (World)"
	"roms/Wario_Land_3.zip=Wario Land 3 (World)"
	"roms/Donkey_Kong_Country.zip=Donkey Kong Country (Europe)"
)
for rom in "$sml" "$ladx" "$la" "$tetris" "${recent[@]%%=*}"; do
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
zz=$out/zz_capture
go build -o "$zz" ./zz_capture
cleanup

mkdir -p "$out/gb" "$out/gbc" "$out/work"
rows=() row=()
webp() { # <console>/<name>: converts the PNG, and adds it to the montage row
	cwebp -quiet -lossless -z 9 "$out/$1.png" -o "$out/$1.webp"
	row+=("$out/$1.png")
}
endrow() { # one montage row per group, each image 320 pixels wide
	[[ ${#row[@]} -gt 0 ]] || return 0
	rows+=("(" "${row[@]}" -resize 320x -background white -gravity north +append ")")
	row=()
}

# Without a filter, at x4 (640x576), through ui.Screenshot.
if want hero; then
	# Title screens: Super Mario Land at frame 200; Link's Awakening DX at
	# frame 900, after Start (frame 500) skipped its intro.
	"$zz" -shot -rom "$sml" -model gb -frames 200 -palette dmg -scale 4 -file "$out/gb/screenshot.png"
	"$zz" -shot -rom "$ladx" -model gbc -frames 900 -press start:500-510 -correct -scale 4 -file "$out/gbc/screenshot.png"
	webp gb/screenshot
	webp gbc/screenshot
	endrow
fi
if want palettes; then
	# The start of level 1-1 (Start at frame 200, frame 360), in 4 palettes.
	for p in dmg:dmg bgb:bgb sepia:sepia purple:violet; do
		"$zz" -shot -rom "$sml" -model gb -frames 360 -press start:200-210 -palette "${p%%:*}" -scale 4 -file "$out/gb/palette_${p##*:}.png"
		webp "gb/palette_${p##*:}"
	done
	endrow
fi
if want colorized; then
	# DMG games on a Game Boy Color, colors corrected: the palette the boot
	# ROM picks by title, and Tetris with Left + B (combination 9).
	"$zz" -shot -rom "$tetris" -model gbc -frames 1300 -press start:400-410 -correct -scale 4 -file "$out/gbc/colorized_tetris.png"
	"$zz" -shot -rom "$la" -model gbc -frames 1500 -press start:700-710 -correct -scale 4 -file "$out/gbc/colorized_zelda.png"
	"$zz" -shot -rom "$sml" -model gbc -frames 400 -press start:200-210 -correct -scale 4 -file "$out/gbc/colorized_sml.png"
	"$zz" -shot -rom "$tetris" -model gbc -frames 1300 -press start:400-410 -compat 9 -correct -scale 4 -file "$out/gbc/colorized_tetris_leftb.png"
	for n in tetris zelda sml tetris_leftb; do webp "gbc/colorized_$n"; done
	endrow
fi
if want filters; then
	# Sources at x1: Super Mario Land walking right in level 1-1 (frame
	# 360), Link's Awakening DX at frame 400 (the boat of the intro).
	w=$out/work
	"$zz" -shot -rom "$sml" -model gb -frames 360 -press start:200-210,right:300-360 -palette dmg -scale 1 -file "$w/src_sml.png"
	"$zz" -shot -rom "$ladx" -model gbc -frames 400 -correct -scale 1 -file "$w/src_ladx.png"
	# Every filter at x15 (2400x2160, the height of a 4K screen) for the LCD
	# details, at x8 for the others; the DMG LCD grid gap is DMG green.
	mkdir -p "$w/r15_sml" "$w/r15_ladx" "$w/r8_sml"
	"$zz" -filters -src "$w/src_sml.png" -model gb -size 2400x2160 -gap 9BBC0F -out "$w/r15_sml"
	"$zz" -filters -src "$w/src_ladx.png" -model gbc -size 2400x2160 -gap 9BBC0F -out "$w/r15_ladx"
	"$zz" -filters -src "$w/src_sml.png" -model gb -size 1280x1152 -gap 9BBC0F -out "$w/r8_sml"
	# Details, shown at their own size: the MARI of the status bar; the boat
	# on the waves; the slope of the pyramid with Mario and the palm trees.
	magick "$w/r15_sml/lcd.png" -crop 480x360+0+0 +repage "$out/gb/filter_lcd.png"
	magick "$w/r15_ladx/lcd.png" -crop 480x360+1890+1050 +repage "$out/gbc/filter_lcd.png"
	webp gb/filter_lcd
	webp gbc/filter_lcd
	for f in nearest scale2x scale3x mmpx; do
		magick "$w/r8_sml/$f.png" -crop 480x432+176+592 +repage "$out/gb/filter_$f.png"
		webp "gb/filter_$f"
	done
	endrow
fi

# Through the real Game, at x8 with the LCD filter, as the screenshot
# hotkey saves them.
common=(-scale "${SCALE:-8}" -filter "${FILTER:-lcd}" -lang "${UI_LANG:-fr}")
if want scenes; then
	"$zz" -rom "$sml" -model gb -title-at 200 -out "$out/gb" "${common[@]}"
	"$zz" -rom "$ladx" -model gbc -title-at 900 -start-at 500 -out "$out/gbc" "${common[@]}"
	for c in gb gbc; do
		for n in game_title menu save_state_resume; do webp "$c/$n"; done
	done
	endrow
fi
if want drop; then
	"$zz" -drop -games roms -out "$out/gb" "${common[@]}" "${recent[@]}"
	webp gb/drop_screen
	endrow
fi

magick "${rows[@]}" -background white -gravity west -append "$out/montage.png"
rm -f "$zz"

ls -l "$out"/gb/*.webp "$out"/gbc/*.webp 2>/dev/null
echo "montage: $out/montage.png"
