; The scenes. Each one has an init routine, run with the LCD off, an update
; routine, run once per frame by the main loop (it waits for the next
; VBlank), and a VBlank routine, run by the VBlank handler to update VRAM.

SECTION "Scenes", ROM0

SceneInitTable:
	dw GameInit, CpuInit, IdleInit, ColorInit, SoundInit
SceneUpdateTable:
	dw GameUpdate, CpuUpdate, IdleUpdate, ColorUpdate, SoundUpdate
SceneVBlankTable:
	dw GameVBlank, CpuVBlank, NoVBlank, ColorVBlank, SoundVBlank

NoVBlank:
	ret

; ---------------------------------------------------------------------------
; 1. Game: what a typical game does every frame (the profile of Zelda).

DEF SPLIT_LINE EQU 64

GameInit:
	call FillPatternMap
	ld hl, StrScore
	ld de, _SCRN1 + 1
	call PrintStr
	ld hl, StrGame
	ld de, _SCRN1 + 15
	call PrintStr
	ld a, 128
	ldh [rWY], a
	ld a, 40
	call InitSprites
	ld a, LCDC_ON | LCDC_WIN9C00 | LCDC_WINON | LCDC_TILES8000 | LCDC_OBJON | LCDC_BGON
	ldh [hLCDC], a
	ld a, SPLIT_LINE - 1
	ldh [rLYC], a
	ld a, STAT_LYC
	ldh [rSTAT], a
	ld a, IE_VBLANK | IE_STAT
	ldh [hIE], a
	ld hl, GameStat
	call SetStatVec
	jp MusicStart

GameUpdate:
	ldh a, [hSCX]
	inc a
	ldh [hSCX], a
	srl a
	ldh [hParallax], a
	ld a, [wFrame]
	rrca
	rrca
	rrca
	and 7
	ldh [hSCY], a
	call UpdateSprites
	; Score: one more point per frame.
	ld hl, wScore
	ld a, [hl]
	add 1
	daa
	ld [hli], a
	ld a, [hl]
	adc 0
	daa
	ld [hli], a
	ld a, [hl]
	adc 0
	daa
	ld [hl], a
	jp WaitVBlank

GameVBlank:
	ld hl, wScore + 2
	ld de, _SCRN1 + 7
	ld a, [hld]
	call PrintHex
	ld a, [hld]
	call PrintHex
	ld a, [hl]
	jp PrintHex

; GameStat runs on the line before the split: the bottom of the background
; scrolls at half the speed of the top. Like games do, it writes SCX in the
; HBlank, so that the split does not depend on when the interrupt is
; serviced.
GameStat:
	call WaitHBlank
	ldh a, [hParallax]
	ldh [rSCX], a
	jp StatReturn

; WaitHBlank waits for the HBlank of the current line. It is called at the
; start of a line (LYC interrupt), before the HBlank of that line.
WaitHBlank:
	ldh a, [rSTAT]
	and 3
	jr nz, WaitHBlank
	ret

; ---------------------------------------------------------------------------
; 2. CPU: computations that never halt (the profile of cpu_instrs).

CpuInit:
	ld hl, StrCpu
	ld de, _SCRN0 + 32 + 1
	call PrintStr
	ld hl, StrCRC
	ld de, _SCRN0 + 32 * 3 + 1
	call PrintStr
	ld hl, StrMul
	ld de, _SCRN0 + 32 * 5 + 1
	call PrintStr
	ld hl, StrDiv
	ld de, _SCRN0 + 32 * 7 + 1
	call PrintStr
	ld hl, StrBank
	ld de, _SCRN0 + 32 * 9 + 1
	call PrintStr
	ld a, 1
	ld [wBank], a
	ld a, 2
	ld [wCopyBank], a
	xor a
	ld [wOffset], a
	ld [wOffset + 1], a
	ld [wCopyOff], a
	ld [wMul], a
	ld [wMul + 1], a
	ld [wMul + 2], a
	ld [wDiv], a
	ld [wDiv + 1], a
	dec a
	ld [wCRC], a
	ld [wCRC + 1], a
	; The WRAM banks 2 to 7 start with random data (on a DMG, the writes
	; to SVBK are ignored and bank 1 is used each time).
	ld b, 2
.bank
	ld a, b
	ldh [rSVBK], a
	ld hl, $D000
	ld c, 0
.fill
	call Rand
	ld [hli], a
	dec c
	jr nz, .fill
	inc b
	ld a, b
	cp 8
	jr nz, .bank
	ld a, 1
	ldh [rSVBK], a
	; Cartridge RAM on.
	ld a, $0A
	ld [MBC5_RAMG], a
	ret

CpuUpdate:
	xor a
	ldh [hVBlankFlag], a
.loop
	call CpuChunk
	ldh a, [hVBlankFlag]
	and a
	jr z, .loop
	ret

; CpuChunk does a bit of everything: a CRC over ROM data in a switched bank,
; a multiplication, a division, a copy between WRAM banks and writes to the
; cartridge RAM.
CpuChunk:
	; CRC-16/CCITT of 16 bytes of the current ROM bank.
	ld a, [wBank]
	ld [MBC5_ROMB0], a
	ld a, [wOffset]
	ld l, a
	ld a, [wOffset + 1]
	add HIGH($4000)
	ld h, a
	ld c, 16
.byte
	ld a, [hli]
	push hl
	ld hl, wCRC + 1
	xor [hl]
	ld [hl], a
	ld b, 8
.bit
	ld hl, wCRC
	sla [hl]
	inc hl
	rl [hl]
	jr nc, .noXor
	dec hl
	ld a, [hl]
	xor $21
	ld [hli], a
	ld a, [hl]
	xor $10
	ld [hl], a
.noXor
	dec b
	jr nz, .bit
	pop hl
	dec c
	jr nz, .byte
	; Next 16 bytes, and the next bank (1 to 7) every 1 KiB.
	ld a, [wOffset]
	add 16
	ld [wOffset], a
	ld a, [wOffset + 1]
	adc 0
	and 3
	ld [wOffset + 1], a
	ld b, a
	ld a, [wOffset]
	or b
	jr nz, .sameBank
	ld a, [wBank]
	inc a
	cp 8
	jr c, .setBank
	ld a, 1
.setBank
	ld [wBank], a
.sameBank

	; Multiplication of two random bytes, added up on 24 bits.
	call Rand
	ld b, a
	call Rand
	ld c, a
	call Mul8
	ld a, [wMul]
	add l
	ld [wMul], a
	ld a, [wMul + 1]
	adc h
	ld [wMul + 1], a
	ld a, [wMul + 2]
	adc 0
	ld [wMul + 2], a

	; Division of the product by a random odd byte.
	call Rand
	or 1
	ld c, a
	call Div16by8
	ld a, [wDiv]
	xor l
	ld [wDiv], a
	ld a, [wDiv + 1]
	xor h
	ld [wDiv + 1], a

	; Copy 32 bytes from a WRAM bank to the next one, plus one.
	ld a, [wCopyBank]
	ldh [rSVBK], a
	ld a, [wCopyOff]
	ld l, a
	ld h, HIGH($D000)
	ld de, wCopyBuf
	ld c, 32
.read
	ld a, [hli]
	ld [de], a
	inc de
	dec c
	jr nz, .read
	ld a, [wCopyBank]
	inc a
	cp 8
	jr c, .nextBank
	ld a, 2
.nextBank
	ld [wCopyBank], a
	ldh [rSVBK], a
	ld a, [wCopyOff]
	ld l, a
	ld de, wCopyBuf
	ld c, 32
.write
	ld a, [de]
	inc a
	ld [hli], a
	inc de
	dec c
	jr nz, .write
	ld a, 1
	ldh [rSVBK], a
	ld a, [wCopyOff]
	add 32
	ld [wCopyOff], a

	; The CRC goes to the cartridge RAM.
	ld l, a
	ld h, HIGH(_SRAM)
	ld a, [wCRC]
	ld [hli], a
	ld a, [wCRC + 1]
	ld [hl], a
	ret

; Mul8 returns b * c in hl.
Mul8:
	ld hl, 0
	ld d, 0
	ld e, c
	ld a, b
	ld b, 8
.loop
	srl a
	jr nc, .skip
	add hl, de
.skip
	sla e
	rl d
	dec b
	jr nz, .loop
	ret

; Div16by8 divides hl by c: quotient in hl, remainder in a.
Div16by8:
	xor a
	ld b, 16
.loop
	add hl, hl
	rla
	jr c, .subtract
	cp c
	jr c, .next
.subtract
	sub c
	inc l
.next
	dec b
	jr nz, .loop
	ret

CpuVBlank:
	ld de, _SCRN0 + 32 * 3 + 6
	ld a, [wCRC + 1]
	call PrintHex
	ld a, [wCRC]
	call PrintHex
	ld de, _SCRN0 + 32 * 5 + 6
	ld a, [wMul + 2]
	call PrintHex
	ld a, [wMul + 1]
	call PrintHex
	ld a, [wMul]
	call PrintHex
	ld de, _SCRN0 + 32 * 7 + 6
	ld a, [wDiv + 1]
	call PrintHex
	ld a, [wDiv]
	call PrintHex
	ld de, _SCRN0 + 32 * 9 + 6
	ld a, [wBank]
	jp PrintHex

; ---------------------------------------------------------------------------
; 3. Idle: little to do, the CPU is halted most of the time (the profile of
; Super Mario Land).

IdleInit:
	ld hl, StrIdle
	ld de, _SCRN0 + 32 + 1
	call PrintStr
	ld a, 1
	call InitSprites
	ld a, LCDC_ON | LCDC_TILES8000 | LCDC_OBJON | LCDC_BGON
	ldh [hLCDC], a
	ret

IdleUpdate:
	call UpdateSprites
	ld a, [wFrame]
	and 63
	jp nz, WaitVBlank
	; A short beep every 64 frames.
	xor a
	ldh [rNR10], a
	ld a, $80
	ldh [rNR11], a
	ld a, $A3
	ldh [rNR12], a
	ld a, LOW(1798) ; C5
	ldh [rNR13], a
	ld a, $80 | HIGH(1798)
	ldh [rNR14], a
	jp WaitVBlank

; ---------------------------------------------------------------------------
; 4. Color: the Game Boy Color hardware. On a DMG, BGP changes on every
; line instead and the tiles are copied by the CPU.

ColorInit:
	call FillPatternMap
	ld hl, StrColor
	ld de, _SCRN0 + 32 + 1
	call PrintStr
	ldh a, [hIsCGB]
	and a
	jr z, .noColor
	; Attributes: every palette, flipped tiles.
	ld a, 1
	ldh [rVBK], a
	ld hl, _SCRN0
	ld d, 0 ; row
.row
	ld e, 0 ; column
.column
	ld a, e
	add d
	and 7 ; palette
	bit 0, e
	jr z, .noXFlip
	set 5, a
.noXFlip
	bit 1, d
	jr z, .noYFlip
	set 6, a
.noYFlip
	ld [hli], a
	inc e
	ld a, e
	cp 32
	jr nz, .column
	inc d
	ld a, d
	cp 32
	jr nz, .row
	xor a
	ldh [rVBK], a
	ld hl, ColorPalettes
	ld c, LOW(rBCPS)
	call LoadPalettes
.noColor
	ld hl, SceneTiles
	ld de, wTileBuf
	ld bc, 64
	call MemCopy
	ld a, 10
	call InitSprites
	ld a, LCDC_ON | LCDC_TILES8000 | LCDC_OBJON | LCDC_BGON
	ldh [hLCDC], a
	; Per-line colors on a CGB, in each HBlank; BGP every 16 lines on a DMG.
	ld a, STAT_HBLANK
	ld b, a
	ldh a, [hIsCGB]
	and a
	jr nz, .stat
	ld a, 15
	ldh [rLYC], a
	ld b, STAT_LYC
.stat
	ld a, b
	ldh [rSTAT], a
	ld a, IE_VBLANK | IE_STAT
	ldh [hIE], a
	ld hl, ColorStat
	jp SetStatVec

ColorUpdate:
	ld a, [wFrame]
	srl a
	ldh [hSCX], a
	call UpdateSprites
	; Animate the tiles: rotate every byte.
	ld hl, wTileBuf
	ld c, 64
.rotate
	rlc [hl]
	inc hl
	dec c
	jr nz, .rotate
	jp WaitVBlank

; ColorVBlank sends the animated tiles to VRAM: by HBlank DMA, 16 bytes per
; line during the next frame, on a CGB, or copied by the CPU on a DMG.
ColorVBlank:
	ldh a, [hIsCGB]
	and a
	jr z, .dmg
	ld a, HIGH(wTileBuf)
	ldh [rHDMA1], a
	ld a, LOW(wTileBuf)
	ldh [rHDMA2], a
	ld a, HIGH(_VRAM + SCENE_TILES * 16) & $1F
	ldh [rHDMA3], a
	ld a, LOW(_VRAM + SCENE_TILES * 16)
	ldh [rHDMA4], a
	ld a, $80 | (64 / 16 - 1) ; HBlank mode, 4 blocks
	ldh [rHDMA5], a
	ret
.dmg
	ld a, [RasterBGP] ; first band
	ldh [rBGP], a
	ld a, 15
	ldh [rLYC], a
	ld hl, wTileBuf
	ld de, _VRAM + SCENE_TILES * 16
	ld bc, 64
	jp MemCopy

; ColorStat changes the colors of the next line: color 0 of BG palette 0 on
; a CGB, at every HBlank (a gradient), BGP on a DMG, on the last line of
; every band of 16 lines (LYC), once its HBlank has come.
ColorStat:
	ldh a, [hIsCGB]
	and a
	jr z, .dmg
	push bc
	ld a, $80 ; color 0 of palette 0
	ldh [rBCPS], a
	ldh a, [rLY]
	srl a
	srl a
	and 31
	ld c, a ; red
	ld a, 31
	sub c ; blue
	add a
	add a
	ld b, a ; blue in bits 10-14
	ld a, c
	ldh [rBCPD], a
	ld a, b
	ldh [rBCPD], a
	pop bc
	jp StatReturn
.dmg
	ldh a, [rLYC]
	inc a ; first line of the next band
	swap a
	and 3
	ld hl, RasterBGP
	add l
	ld l, a
	adc h
	sub l
	ld h, a
	ld l, [hl]
	call WaitHBlank
	ld a, l
	ldh [rBGP], a
	ldh a, [rLYC]
	add 16
	ldh [rLYC], a
	jp StatReturn

; ---------------------------------------------------------------------------
; 5. Sound: every channel retriggered with random settings.

SoundInit:
	ld hl, StrSound
	ld de, _SCRN0 + 32 + 1
	call PrintStr
	ld hl, StrNR52
	ld de, _SCRN0 + 32 * 3 + 1
	call PrintStr
	ld a, LCDC_ON | LCDC_TILES8000 | LCDC_OBJON | LCDC_BGON
	ldh [hLCDC], a
	ret

SoundUpdate:
	; One channel per frame, each one every 4 frames.
	ld a, [wFrame]
	and 3
	ld hl, RetriggerTable
	call CallTable
	call Rand
	and $77
	ldh [rNR50], a
	call Rand
	ldh [rNR51], a
	; A sprite per channel, shown while it plays.
	ldh a, [rNR52]
	ld b, a
	ld hl, wShadowOAM
	ld c, 40 ; x
	REPT 4
		xor a
		srl b
		jr nc, .off\@
		ld a, 100
.off\@
		ld [hli], a ; y
		ld a, c
		ld [hli], a ; x
		add 24
		ld c, a
		ld a, BALL_TILE
		ld [hli], a
		xor a
		ld [hli], a
	ENDR
	jp WaitVBlank

SoundVBlank:
	ld de, _SCRN0 + 32 * 3 + 6
	ldh a, [rNR52]
	jp PrintHex

RetriggerTable:
	dw Retrigger1, Retrigger2, Retrigger3, Retrigger4

Retrigger1:
	call Rand
	and $7F
	ldh [rNR10], a
	call Rand
	ldh [rNR11], a
	call Rand
	ldh [rNR12], a
	call Rand
	ldh [rNR13], a
	call Rand
	and $47
	or $80
	ldh [rNR14], a
	ret

Retrigger2:
	call Rand
	ldh [rNR21], a
	call Rand
	ldh [rNR22], a
	call Rand
	ldh [rNR23], a
	call Rand
	and $47
	or $80
	ldh [rNR24], a
	ret

Retrigger3:
	xor a
	ldh [rNR30], a ; the wave RAM can only be written with the DAC off
	ld hl, _AUD3WAVERAM
	ld c, 16
.wave
	call Rand
	ld [hli], a
	dec c
	jr nz, .wave
	ld a, $80
	ldh [rNR30], a
	call Rand
	ldh [rNR31], a
	call Rand
	and $60
	ldh [rNR32], a
	call Rand
	ldh [rNR33], a
	call Rand
	and $47
	or $80
	ldh [rNR34], a
	ret

Retrigger4:
	call Rand
	and $3F
	ldh [rNR41], a
	call Rand
	ldh [rNR42], a
	call Rand
	ldh [rNR43], a
	call Rand
	and $40
	or $80
	ldh [rNR44], a
	ret
