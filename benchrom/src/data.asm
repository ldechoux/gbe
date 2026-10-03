; Graphics, palettes, strings, music and the data read by the CPU scene.

MACRO rgb ; red, green, blue (0-31)
	dw (\1) | (\2) << 5 | (\3) << 10
ENDM

SECTION "Data", ROM0

; Scene tiles: 4 background tiles, then the ball used by the sprites.
SceneTiles:
	; brick
	dw `33333333
	dw `11131113
	dw `11131113
	dw `33333333
	dw `13111311
	dw `13111311
	dw `33333333
	dw `11111111
	; grass
	dw `00000000
	dw `00000000
	dw `02000200
	dw `22202220
	dw `22222222
	dw `21212121
	dw `11111111
	dw `11111111
	; sky
	dw `00000000
	dw `00000000
	dw `00010000
	dw `00000000
	dw `00000000
	dw `00000001
	dw `00000000
	dw `00000000
	; block
	dw `33333333
	dw `32222223
	dw `32111123
	dw `32100123
	dw `32100123
	dw `32111123
	dw `32222223
	dw `33333333
	; ball
	dw `00333300
	dw `03222230
	dw `32211223
	dw `32111123
	dw `32111123
	dw `32211223
	dw `03222230
	dw `00333300
SceneTilesEnd:

; CGB palettes: 8 palettes of 4 colors each.
GreyPalettes:
	REPT 8
		rgb 31, 31, 31
		rgb 21, 21, 21
		rgb 10, 10, 10
		rgb 0, 0, 0
	ENDR

SpritePalettes:
	FOR SPRITE_PAL, 8
		rgb 31, 31, 31
		rgb 31, SPRITE_PAL * 4, 0
		rgb SPRITE_PAL * 2, 0, 31 - SPRITE_PAL * 4
		rgb 0, 0, 0
	ENDR

ColorPalettes:
	FOR COLOR_PAL, 8
		rgb 31, 31, 28
		rgb COLOR_PAL * 4, 31 - COLOR_PAL * 4, 16
		rgb 31 - COLOR_PAL * 4, COLOR_PAL * 2, COLOR_PAL * 4
		rgb 2, 2, 8
	ENDR

; BGP of the bands of 16 lines of the color scene on a DMG.
RasterBGP:
	db %11100100, %10010011, %01001110, %00111001

StrScore: db "SCORE", $FF
StrGame:  db "GAME", $FF
StrCpu:   db "CPU", $FF
StrCRC:   db "CRC", $FF
StrMul:   db "MUL", $FF
StrDiv:   db "DIV", $FF
StrBank:  db "BANK", $FF
StrIdle:  db "IDLE", $FF
StrColor: db "COLOR", $FF
StrSound: db "SOUND", $FF
StrNR52:  db "NR52", $FF

; Frequency register values of the notes from C3 (note 1) to B5 (note 36).
FreqTable:
	dw 1046, 1102, 1155, 1205, 1253, 1297, 1339, 1379, 1417, 1452, 1486, 1517
	dw 1547, 1575, 1602, 1627, 1650, 1673, 1694, 1714, 1732, 1750, 1767, 1783
	dw 1798, 1812, 1825, 1837, 1849, 1860, 1871, 1881, 1890, 1899, 1907, 1915

; 32 steps each; 0 is a rest. C4 is 13, C5 is 25.
Melody:
	db 13, 0, 17, 0, 20, 0, 17, 0, 18, 0, 22, 0, 25, 0, 22, 0
	db 20, 0, 24, 0, 27, 0, 24, 0, 20, 0, 17, 0, 15, 0, 13, 0
Counter:
	db 0, 0, 25, 0, 0, 0, 29, 0, 0, 0, 30, 0, 0, 0, 29, 0
	db 0, 0, 32, 0, 0, 0, 30, 0, 0, 0, 29, 0, 0, 0, 25, 0
Bass:
	db 1, 0, 0, 0, 1, 0, 0, 0, 6, 0, 0, 0, 6, 0, 0, 0
	db 8, 0, 0, 0, 8, 0, 0, 0, 8, 0, 0, 0, 1, 0, 0, 0
Drums: ; 1: kick, 2: hi-hat
	REPT 4
		db 1, 0, 2, 0, 1, 0, 2, 2
	ENDR

WaveData: ; a triangle
	db $01, $23, $45, $67, $89, $AB, $CD, $EF
	db $FE, $DC, $BA, $98, $76, $54, $32, $10

; 1 KiB of pseudo-random data in each ROM bank, for the CPU scene.
FOR BANK_NUM, 1, 8
SECTION "Bank {d:BANK_NUM} data", ROMX[$4000], BANK[BANK_NUM]
	DEF SEED = BANK_NUM * 77
	REPT 1024
		REDEF SEED = (SEED * 109 + 89) & $FFFF
		db SEED >> 8
	ENDR
	PURGE SEED
ENDR
