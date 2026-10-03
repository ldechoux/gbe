; Bouncing sprites, shared by several scenes.

SECTION "Sprites", ROM0

; InitSprites places a sprites (at most 40). The first 12 share a line and
; only move sideways, so that the 10 sprites per line limit applies.
InitSprites:
	ld [wSpriteCount], a
	ld b, a
	ld c, 0 ; index
	ld de, 0 ; running X and Y offsets
	ld hl, wSprites
.loop
	ld a, d
	add 37
	and 127
	ld d, a
	add 9
	ld [hli], a ; x
	ld a, c
	and 1
	ld a, 1
	jr z, .right
	ld a, -1
.right
	ld [hli], a ; dx
	ld a, c
	cp 12
	jr nc, .free
	ld a, 80
	ld [hli], a ; y
	xor a
	ld [hli], a ; dy
	jr .next
.free
	ld a, e
	add 23
	and 127
	ld e, a
	add 17
	ld [hli], a ; y
	ld a, c
	and 2
	ld a, 1
	jr z, .down
	ld a, -1
.down
	ld [hli], a ; dy
.next
	inc c
	dec b
	jr nz, .loop
	ret

; UpdateSprites moves the sprites, bouncing on the edges of the screen, and
; writes them to the shadow OAM. Their attributes vary with their index:
; palette, X flip, priority behind the background.
UpdateSprites:
	ld a, [wSpriteCount]
	and a
	ret z
	ld b, a
	ld hl, wSprites
	ld de, wShadowOAM
.loop
	; X
	ld a, [hli] ; x, hl -> dx
	ld c, a
	add [hl]
	cp 9
	jr c, .bounceX
	cp 160
	jr c, .movedX
.bounceX
	ld a, [hl]
	cpl
	inc a
	ld [hl], a
	ld a, c
.movedX
	dec hl
	ld [hli], a
	inc hl ; -> y
	ld c, a ; x for the OAM
	; Y
	push bc
	ld a, [hli] ; y, hl -> dy
	ld b, a
	add [hl]
	cp 17
	jr c, .bounceY
	cp 152
	jr c, .movedY
.bounceY
	ld a, [hl]
	cpl
	inc a
	ld [hl], a
	ld a, b
.movedY
	dec hl
	ld [hli], a
	inc hl ; next sprite
	pop bc
	; OAM entry: y, x, tile, attributes
	ld [de], a
	inc de
	ld a, c
	ld [de], a
	inc de
	ld a, BALL_TILE
	ld [de], a
	inc de
	ld a, b
	and 7 ; CGB palette
	ld c, a
	ld a, b
	and 1
	swap a ; DMG palette (bit 4)
	or c
	ld c, a
	ld a, b
	and 2
	swap a ; X flip (bit 5)
	or c
	ld c, a
	bit 3, b
	jr z, .front
	set 7, c ; behind the background
.front
	ld a, c
	ld [de], a
	inc de
	dec b
	jr nz, .loop
	ret
