; gbe benchmark ROM: a few scenes that keep the emulator busy the way real
; games do, for benchmarks and for rendering and sound regression tests.
;
; The scene is chosen by the button held at power on (see ReadBootButtons):
;   A: game      scrolling, window HUD, mid-screen raster split, 40 sprites,
;                music on the four channels
;   B: cpu       number crunching that never halts, MBC5 ROM banking, WRAM
;                banks, cartridge RAM
;   Select: idle mostly halted, one sprite and a beep now and then
;   Start: color CGB double speed, HBlank DMA, per-line palette changes,
;                tile attributes (per-line BGP changes on a DMG)
;   Right: sound all channels retriggered with random settings
; With no button, a demo runs every scene in turn, 256 frames each.
;
; Everything is deterministic: the same frames and samples on every run.

INCLUDE "hardware.inc"
INCLUDE "font.asm"

DEF SCENE_COUNT EQU 5
DEF DEMO_FRAMES EQU 256     ; frames per scene in the demo (wSceneTimer wraps)
DEF SCENE_TILES EQU $40     ; first tile of the scene graphics
DEF BALL_TILE   EQU SCENE_TILES + 4

; ---------------------------------------------------------------------------
; Interrupt vectors and header

SECTION "VBlank vector", ROM0[$40]
	jp VBlankHandler

SECTION "STAT vector", ROM0[$48]
	jp StatHandler

SECTION "Timer vector", ROM0[$50]
	reti

SECTION "Serial vector", ROM0[$58]
	reti

SECTION "Joypad vector", ROM0[$60]
	reti

SECTION "Header", ROM0[$100]
	nop
	jp Start
	ds $150 - @, 0 ; filled in by rgbfix

; ---------------------------------------------------------------------------
; Variables. Everything the interrupt handlers use is in WRAM0 or HRAM, so
; that the CPU scene can switch the WRAM bank at $D000 at any time; the
; stack is in WRAM0 for the same reason.

SECTION "Shadow OAM", WRAM0, ALIGN[8]
wShadowOAM: ds 160

SECTION "HDMA source", WRAM0, ALIGN[4]
wTileBuf: ds 64 ; animated tiles of the color scene

SECTION "Variables", WRAM0
wScene:       ds 1 ; 1 to SCENE_COUNT
wDemo:        ds 1 ; non-zero: go through every scene
wSceneTimer:  ds 1
wFrame:       ds 2
wStatVec:     ds 2 ; STAT handler of the scene
wLFSR:        ds 2
wSpriteCount: ds 1
wSprites:     ds 40 * 4 ; x, dx, y, dy
wScore:       ds 3 ; BCD, low byte first
wMusicTimer:  ds 1
wMusicStep:   ds 1
; CPU scene
wBank:        ds 1
wOffset:      ds 2
wCRC:         ds 2
wMul:         ds 3
wDiv:         ds 2
wCopyBank:    ds 1
wCopyOff:     ds 1
wCopyBuf:     ds 32

SECTION "Stack", WRAM0
	ds 256
wStackTop:

SECTION "HRAM", HRAM
hVBlankFlag:  ds 1
hIsCGB:       ds 1
hSCX:         ds 1 ; scroll applied at the top of each frame
hSCY:         ds 1
hParallax:    ds 1 ; SCX below the raster split of the game scene
hLCDC:        ds 1 ; set by the scene init routines
hIE:          ds 1
hMusicOn:     ds 1

; ---------------------------------------------------------------------------
; Start-up

SECTION "Start", ROM0

Start:
	di
	ld sp, wStackTop
	ld b, 0
	cp $11 ; the boot ROM of a Game Boy Color leaves $11 in A
	jr nz, .dmg
	inc b
.dmg
	; Clear WRAM0, the WRAM bank 1 and HRAM.
	ld hl, $C000
	ld de, $2000
.clearWRAM
	xor a
	ld [hli], a
	dec de
	ld a, d
	or e
	jr nz, .clearWRAM
	ld hl, $FF80
	ld c, $7F
.clearHRAM
	xor a
	ld [hli], a
	dec c
	jr nz, .clearHRAM
	ld a, b
	ldh [hIsCGB], a

	call LCDOff
	ld hl, OAMDMACode
	ld de, OAMDMA
	ld bc, OAMDMACodeEnd - OAMDMACode
	call MemCopy
	call LoadFont
	call ReadBootButtons
	call EnterScene

MainLoop:
IF DEF(FREEZE_AT)
	; Comparison builds only (see the README): once wFrame reaches
	; FREEZE_AT, the scene stops changing, so that emulators whose boot ROMs
	; take different times end up showing the same picture.
	ld a, [wFrame + 1]
	cp HIGH(FREEZE_AT)
	jr c, .run
	jr nz, .frozen
	ld a, [wFrame]
	cp LOW(FREEZE_AT)
	jr c, .run
.frozen
	xor a
	ldh [hMusicOn], a
	call WaitVBlank
	jr MainLoop
.run
ENDC
	ld a, [wScene]
	dec a
	ld hl, SceneUpdateTable
	call CallTable
	ld a, [wDemo]
	and a
	jr z, MainLoop
	ld hl, wSceneTimer
	dec [hl]
	jr nz, MainLoop
	ld a, [wScene]
	inc a
	cp SCENE_COUNT + 1
	jr c, .next
	ld a, 1
.next
	ld [wScene], a
	call EnterScene
	jr MainLoop

; ReadBootButtons picks the scene from the button held at power on.
ReadBootButtons:
IF DEF(FORCE_SCENE)
	ld a, FORCE_SCENE ; comparison builds only (see the README)
	ld [wScene], a
	ret
ENDC
	ld a, $10 ; action buttons
	call ReadP1
	ld b, a
	ld a, $20 ; directions
	call ReadP1
	ld c, a
	ld a, $30
	ldh [rP1], a
	ld a, 1
	bit 0, b ; A
	jr nz, .set
	inc a
	bit 1, b ; B
	jr nz, .set
	inc a
	bit 2, b ; Select
	jr nz, .set
	inc a
	bit 3, b ; Start
	jr nz, .set
	inc a
	bit 0, c ; Right
	jr nz, .set
	ld a, 1
	ld [wDemo], a
.set
	ld [wScene], a
	ret

; ReadP1 selects a button group and returns its pressed buttons (bits set).
ReadP1:
	ldh [rP1], a
	REPT 4 ; let the lines settle
		ldh a, [rP1]
	ENDR
	cpl
	and $0F
	ret

; EnterScene sets the screen, the sound and the interrupts up for wScene.
EnterScene:
	di
	call LCDOff
	call SetSpeed
	xor a
	ldh [rSTAT], a
	ldh [rSCX], a
	ldh [rSCY], a
	ldh [hSCX], a
	ldh [hSCY], a
	ldh [hParallax], a
	ldh [hMusicOn], a
	ldh [rWY], a
	ld [wSceneTimer], a
	ld [wSpriteCount], a
	ld a, 7
	ldh [rWX], a
	ld a, LOW($ACE1)
	ld [wLFSR], a
	ld a, HIGH($ACE1)
	ld [wLFSR + 1], a

	; Empty tile maps (and attributes), sprites and the score.
	ld hl, _SCRN0
	ld bc, $800
	xor a
	call MemSet
	ldh a, [hIsCGB]
	and a
	jr z, .noAttributes
	ld a, 1
	ldh [rVBK], a
	ld hl, _SCRN0
	ld bc, $800
	xor a
	call MemSet
	xor a
	ldh [rVBK], a
.noAttributes
	ld hl, wShadowOAM
	ld bc, 160
	xor a
	call MemSet
	ld hl, _OAMRAM
	ld bc, 160
	xor a
	call MemSet
	ld hl, wScore
	ld bc, 3
	xor a
	call MemSet

	; Scene tiles, at SCENE_TILES.
	ld hl, SceneTiles
	ld de, _VRAM + SCENE_TILES * 16
	ld bc, SceneTilesEnd - SceneTiles
	call MemCopy

	; Sound on, all channels on both sides. Resetting DIV first also resets
	; the frame sequencer of the APU, which DIV clocks: lengths, sweeps and
	; envelopes then run the same whatever the boot ROM took.
	xor a
	ldh [rDIV], a
	ldh [rNR52], a
	ld a, $80
	ldh [rNR52], a
	ld a, $77
	ldh [rNR50], a
	ld a, $FF
	ldh [rNR51], a

	; Palettes.
	ld a, %11100100
	ldh [rBGP], a
	ldh [rOBP0], a
	ld a, %00011011
	ldh [rOBP1], a
	ldh a, [hIsCGB]
	and a
	jr z, .noColor
	ld hl, GreyPalettes
	ld c, LOW(rBCPS)
	call LoadPalettes
	ld hl, SpritePalettes
	ld c, LOW(rOCPS)
	call LoadPalettes
.noColor

	ld hl, StatReturn
	call SetStatVec
	ld a, LCDC_ON | LCDC_TILES8000 | LCDC_BGON
	ldh [hLCDC], a
	ld a, IE_VBLANK
	ldh [hIE], a

	ld a, [wScene]
	dec a
	ld hl, SceneInitTable
	call CallTable

	ldh a, [hLCDC]
	ldh [rLCDC], a
	xor a
	ldh [rIF], a
	ldh a, [hIE]
	ldh [rIE], a
	ei
	ret

; SetSpeed puts a Game Boy Color in double speed mode for the color scene
; and back to normal speed for the others. The LCD must be off.
SetSpeed:
	ldh a, [hIsCGB]
	and a
	ret z
	ld b, 0
	ld a, [wScene]
	cp 4
	jr nz, .wanted
	ld b, $80
.wanted
	ldh a, [rKEY1]
	and $80
	cp b
	ret z
	ld a, 1
	ldh [rKEY1], a
	xor a
	ldh [rIE], a
	ld a, $30
	ldh [rP1], a
	stop
	ret

; ---------------------------------------------------------------------------
; Interrupts

VBlankHandler:
	push af
	push bc
	push de
	push hl
	call OAMDMA
	ldh a, [hSCX]
	ldh [rSCX], a
	ldh a, [hSCY]
	ldh [rSCY], a
	ld a, [wScene]
	dec a
	ld hl, SceneVBlankTable
	call CallTable
	ldh a, [hMusicOn]
	and a
	call nz, MusicTick
	ld hl, wFrame
	inc [hl]
	jr nz, .noCarry
	inc hl
	inc [hl]
.noCarry
	ld a, 1
	ldh [hVBlankFlag], a
	pop hl
	pop de
	pop bc
	pop af
	reti

; StatHandler jumps to the handler of the scene, which ends with
; "jp StatReturn".
StatHandler:
	push af
	push hl
	ld hl, wStatVec
	ld a, [hli]
	ld h, [hl]
	ld l, a
	jp hl

StatReturn:
	pop hl
	pop af
	reti

; SetStatVec sets the STAT handler of the scene to hl.
SetStatVec:
	ld a, l
	ld [wStatVec], a
	ld a, h
	ld [wStatVec + 1], a
	ret

; WaitVBlank halts until the next VBlank interrupt has run.
WaitVBlank:
	xor a
	ldh [hVBlankFlag], a
.wait
	halt
	ldh a, [hVBlankFlag]
	and a
	jr z, .wait
	ret

; The OAM DMA routine runs from HRAM, the only memory the CPU can read
; during the transfer. Start copies it there.
OAMDMACode:
LOAD "OAM DMA", HRAM
OAMDMA:
	ld a, HIGH(wShadowOAM)
	ldh [rDMA], a
	ld a, 40
.wait
	dec a
	jr nz, .wait
	ret
ENDL
OAMDMACodeEnd:

; ---------------------------------------------------------------------------
; Helpers

; CallTable jumps to entry a of the table of addresses at hl.
CallTable:
	add a
	add l
	ld l, a
	adc h
	sub l
	ld h, a
	ld a, [hli]
	ld h, [hl]
	ld l, a
	jp hl

; LCDOff turns the screen off, waiting for VBlank if it is on.
LCDOff:
	ldh a, [rLCDC]
	bit 7, a
	ret z
.wait
	ldh a, [rLY]
	cp 144
	jr c, .wait
	xor a
	ldh [rLCDC], a
	ret

; MemCopy copies bc bytes from hl to de.
MemCopy:
	ld a, [hli]
	ld [de], a
	inc de
	dec bc
	ld a, b
	or c
	jr nz, MemCopy
	ret

; MemSet fills bc bytes at hl with a.
MemSet:
	ld d, a
.loop
	ld a, d
	ld [hli], a
	dec bc
	ld a, b
	or c
	jr nz, .loop
	ret

; LoadFont expands the 1 bpp font into tiles 0 and up, in color 3.
LoadFont:
	ld hl, Font
	ld de, _VRAM
	ld bc, FontEnd - Font
.loop
	ld a, [hli]
	ld [de], a
	inc de
	ld [de], a
	inc de
	dec bc
	ld a, b
	or c
	jr nz, .loop
	ret

; LoadPalettes writes 64 bytes from hl to the CGB palette RAM whose index
; register is at $FF00 + c (rBCPS or rOCPS).
LoadPalettes:
	ld a, $80 ; index 0, auto-increment
	ldh [c], a
	inc c
	ld b, 64
.loop
	ld a, [hli]
	ldh [c], a
	dec b
	jr nz, .loop
	ret

; PrintStr writes the string at hl, ended by $FF, to the tile map at de.
PrintStr:
	ld a, [hli]
	cp $FF
	ret z
	ld [de], a
	inc de
	jr PrintStr

; PrintHex writes a as two hexadecimal digits to the tile map at de.
PrintHex:
	ld b, a
	swap a
	and $0F
	inc a ; "0" is tile 1, "A" tile 11
	ld [de], a
	inc de
	ld a, b
	and $0F
	inc a
	ld [de], a
	inc de
	ret

; Rand steps the 16-bit Galois LFSR and returns its low byte in a.
Rand:
	push hl
	ld hl, wLFSR + 1
	srl [hl]
	dec hl
	rr [hl]
	jr nc, .done
	inc hl
	ld a, [hl]
	xor $B4
	ld [hl], a
	dec hl
.done
	ld a, [hl]
	pop hl
	ret

; FillPatternMap fills the tile map at $9800 with the scene tiles.
FillPatternMap:
	ld hl, _SCRN0
	ld d, 0 ; row
.row
	ld e, 0 ; column
.column
	ld a, e
	add d
	srl a
	and 3
	add SCENE_TILES
	ld [hli], a
	inc e
	ld a, e
	cp 32
	jr nz, .column
	inc d
	ld a, d
	cp 32
	jr nz, .row
	ret

INCLUDE "sprites.asm"
INCLUDE "scenes.asm"
INCLUDE "sound.asm"
INCLUDE "data.asm"
