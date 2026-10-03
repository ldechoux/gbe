; A small music driver for the game scene: a melody on channel 1, a counter
; melody on channel 2, a bass line on the wave channel and drums on the
; noise channel, 32 steps of 8 frames.

DEF MUSIC_TEMPO EQU 8 ; frames per step

SECTION "Music", ROM0

MusicStart:
	xor a
	ldh [rNR30], a ; the wave RAM can only be written with the DAC off
	ldh [rNR10], a
	ld hl, WaveData
	ld de, _AUD3WAVERAM
	ld bc, 16
	call MemCopy
	ld a, $80
	ldh [rNR30], a
	ld a, $20 ; full volume
	ldh [rNR32], a
	ld a, $55 ; master volume: four channels together would clip
	ldh [rNR50], a
	ld a, $80 ; 50% duty
	ldh [rNR11], a
	ld a, $40 ; 25% duty
	ldh [rNR21], a
	ld a, -1
	ld [wMusicStep], a
	ld a, 1
	ld [wMusicTimer], a
	ldh [hMusicOn], a
	ret

; MusicTick is called by the VBlank handler.
MusicTick:
	ld hl, wMusicTimer
	dec [hl]
	ret nz
	ld [hl], MUSIC_TEMPO
	ld a, [wMusicStep]
	inc a
	and 31
	ld [wMusicStep], a
	ld c, a
	ld b, 0

	ld hl, Melody
	add hl, bc
	ld a, [hl]
	and a
	jr z, .noMelody
	call NoteFreq
	ld a, $F3
	ldh [rNR12], a
	ld a, e
	ldh [rNR13], a
	ld a, d
	or $80
	ldh [rNR14], a
.noMelody

	ld hl, Counter
	add hl, bc
	ld a, [hl]
	and a
	jr z, .noCounter
	call NoteFreq
	ld a, $A2
	ldh [rNR22], a
	ld a, e
	ldh [rNR23], a
	ld a, d
	or $80
	ldh [rNR24], a
.noCounter

	ld hl, Bass
	add hl, bc
	ld a, [hl]
	and a
	jr z, .noBass
	call NoteFreq
	ld a, e
	ldh [rNR33], a
	ld a, d
	or $80
	ldh [rNR34], a
.noBass

	ld hl, Drums
	add hl, bc
	ld a, [hl]
	dec a
	jr z, .kick
	dec a
	ret nz
	ld a, $81 ; hi-hat: short, narrow noise
	ldh [rNR42], a
	ld a, $08
	jr .noise
.kick
	ld a, $F1
	ldh [rNR42], a
	ld a, $51
.noise
	ldh [rNR43], a
	xor a
	ldh [rNR41], a
	ld a, $80
	ldh [rNR44], a
	ret

; NoteFreq returns in de the frequency register value of note a (1 = C3).
NoteFreq:
	dec a
	add a
	ld e, a
	ld d, 0
	ld hl, FreqTable
	add hl, de
	ld a, [hli]
	ld e, a
	ld d, [hl]
	ret
