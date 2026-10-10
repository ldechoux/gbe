package gb

import "math/bits"

const (
	ScreenWidth  = 160
	ScreenHeight = 144
)

// midLine is the middle line of the screen (see PPU.midSCX).
const midLine = ScreenHeight / 2

// PPU renders one scanline at a time, at the end of mode 3. In DMG mode the
// framebuffer holds shades 0 (lightest) to 3 (darkest), already mapped
// through BGP/OBPx; the actual colors are chosen by the frontend. In CGB mode
// a second framebuffer holds RGB555 colors from the palette RAM.
type PPU struct {
	bus *Bus

	vram [0x4000]byte // 2 banks of 8 KiB; a DMG only uses the first one
	vbk  byte         // VRAM bank seen by the CPU (CGB)
	oam  [0xA0]byte

	// CGB palette RAM: 8 palettes of 4 colors, 2 bytes per color.
	bgPal, objPal [64]byte
	bcps, ocps    byte // palette index registers, bit 7 = auto-increment
	opri          byte // bit 0 set: DMG-style sprite priority (by X)

	lcdc, stat, scy, scx, ly, lyc byte
	bgp, obp0, obp1, wy, wx       byte
	dmaReg                        byte

	mode       byte
	dot        int
	statLine   bool
	windowLine int
	wyReached  bool

	back, front   [ScreenWidth * ScreenHeight]byte
	cback, cfront [ScreenWidth * ScreenHeight]uint16
	frameReady    bool

	// midSCX and midSCY are the scroll of the background on midLine
	// of the last frame, for the rumble detector: below a status bar, which
	// some games keep still while the screen shakes. Not part of the state.
	midSCX, midSCY byte
}

// vramOffset maps a CPU address in 0x8000-0x9FFF to the VRAM array.
func (p *PPU) vramOffset(addr uint16) int {
	return int(p.vbk&1)*0x2000 + int(addr&0x1FFF)
}

func (p *PPU) read(addr uint16) byte {
	switch addr {
	case 0xFF40:
		return p.lcdc
	case 0xFF41:
		v := 0x80 | p.stat&0x78 | p.mode
		if p.ly == p.lyc {
			v |= 0x04
		}
		return v
	case 0xFF42:
		return p.scy
	case 0xFF43:
		return p.scx
	case 0xFF44:
		return p.ly
	case 0xFF45:
		return p.lyc
	case 0xFF46:
		return p.dmaReg
	case 0xFF47:
		return p.bgp
	case 0xFF48:
		return p.obp0
	case 0xFF49:
		return p.obp1
	case 0xFF4A:
		return p.wy
	case 0xFF4B:
		return p.wx
	case 0xFF4F:
		return 0xFE | p.vbk
	case 0xFF68:
		return 0x40 | p.bcps
	case 0xFF69:
		return p.bgPal[p.bcps&0x3F]
	case 0xFF6A:
		return 0x40 | p.ocps
	case 0xFF6B:
		return p.objPal[p.ocps&0x3F]
	case 0xFF6C:
		return 0xFE | p.opri
	}
	return 0xFF
}

// writePalette stores a byte in the palette RAM at the index register and
// increments it if asked to.
func writePalette(pal *[64]byte, idx *byte, v byte) {
	pal[*idx&0x3F] = v
	if *idx&0x80 != 0 {
		*idx = 0x80 | (*idx+1)&0x3F
	}
}

func (p *PPU) write(addr uint16, v byte) {
	switch addr {
	case 0xFF40:
		wasOn := p.lcdc&0x80 != 0
		p.lcdc = v
		if wasOn && v&0x80 == 0 {
			p.ly, p.dot, p.mode = 0, 0, 0
			p.back = [len(p.back)]byte{}
			p.front = p.back
			for i := range p.cback {
				p.cback[i] = 0x7FFF // a CGB screen turns white
			}
			p.cfront = p.cback
			p.frameReady = true
		} else if !wasOn && v&0x80 != 0 {
			p.ly, p.dot, p.mode = 0, 0, 2
			p.windowLine, p.wyReached = 0, false
		}
	case 0xFF41:
		p.stat = v & 0x78
	case 0xFF42:
		p.scy = v
	case 0xFF43:
		p.scx = v
	case 0xFF45:
		p.lyc = v
	case 0xFF47:
		p.bgp = v
	case 0xFF48:
		p.obp0 = v
	case 0xFF49:
		p.obp1 = v
	case 0xFF4A:
		p.wy = v
	case 0xFF4B:
		p.wx = v
	case 0xFF4F:
		p.vbk = v & 1
	case 0xFF68:
		p.bcps = v & 0xBF
	case 0xFF69:
		writePalette(&p.bgPal, &p.bcps, v)
	case 0xFF6A:
		p.ocps = v & 0xBF
	case 0xFF6B:
		writePalette(&p.objPal, &p.ocps, v)
	case 0xFF6C:
		p.opri = v & 1
	}
	p.updateStat()
}

func (p *PPU) updateStat() {
	on := p.lcdc&0x80 != 0
	line := on && ((p.stat&0x40 != 0 && p.ly == p.lyc) ||
		(p.mode == 0 && p.stat&0x08 != 0) ||
		(p.mode == 1 && p.stat&0x10 != 0) ||
		(p.mode == 2 && p.stat&0x20 != 0))
	if line && !p.statLine {
		p.bus.requestInterrupt(IntSTAT)
	}
	p.statLine = line
}

// modeEnd is the dot at which each mode ends: HBlank and VBlank lines at the
// end of the line, mode 2 after the OAM scan, mode 3 after the transfer.
var modeEnd = [4]int{456, 456, 80, 80 + 172}

// tick advances the PPU by the given number of dots (4 per M-cycle, 2 in
// CGB double speed mode).
func (p *PPU) tick(dots int) {
	if p.lcdc&0x80 == 0 {
		return
	}
	p.dot += dots
	if p.dot < modeEnd[p.mode&3] {
		return // most M-cycles: the mode goes on
	}
	mode, ly := p.mode, p.ly
	switch p.mode {
	case 2:
		if p.dot >= 80 {
			p.mode = 3
		}
	case 3:
		if p.dot >= 80+172 {
			p.renderLine()
			p.mode = 0
			p.bus.hblank()
		}
	case 0:
		if p.dot >= 456 {
			p.dot -= 456
			p.ly++
			if p.ly == ScreenHeight {
				p.mode = 1
				p.bus.requestInterrupt(IntVBlank)
				p.front = p.back
				p.cfront = p.cback
				p.frameReady = true
			} else {
				p.mode = 2
			}
		}
	case 1:
		if p.dot >= 456 {
			p.dot -= 456
			p.ly++
			if p.ly > 153 {
				p.ly = 0
				p.windowLine, p.wyReached = 0, false
				p.mode = 2
			}
		}
	}
	// The other inputs of the STAT line only change through write, which
	// updates it.
	if p.mode != mode || p.ly != ly {
		p.updateStat()
	}
}

func (p *PPU) tileRow(bank int, tile byte, row int, signed bool) (lo, hi byte) {
	var addr int
	if signed {
		addr = 0x1000 + int(int8(tile))*16
	} else {
		addr = int(tile) * 16
	}
	addr += bank*0x2000 + row*2
	return p.vram[addr], p.vram[addr+1]
}

// pixels writes the color indices of the 8 pixels of a tile row, left to
// right.
func pixels(dst []byte, lo, hi byte) {
	_ = dst[7]
	dst[0] = lo>>7&1 | hi>>6&2
	dst[1] = lo>>6&1 | hi>>5&2
	dst[2] = lo>>5&1 | hi>>4&2
	dst[3] = lo>>4&1 | hi>>3&2
	dst[4] = lo>>3&1 | hi>>2&2
	dst[5] = lo>>2&1 | hi>>1&2
	dst[6] = lo>>1&1 | hi&2
	dst[7] = lo&1 | hi<<1&2
}

func fill8(dst []byte, v byte) {
	_ = dst[7]
	dst[0], dst[1], dst[2], dst[3], dst[4], dst[5], dst[6], dst[7] = v, v, v, v, v, v, v, v
}

func colorIndex(lo, hi byte, bit int) byte {
	return (lo>>bit)&1 | ((hi>>bit)&1)<<1
}

func shade(pal, idx byte) byte { return (pal >> (idx * 2)) & 3 }

// color reads color idx of palette n in a CGB palette RAM.
func color(pal *[64]byte, n, idx byte) uint16 {
	i := int(n&7)*8 + int(idx)*2
	return (uint16(pal[i]) | uint16(pal[i+1])<<8) & 0x7FFF
}

// mapRow reads a run of pixels of the tile map at base, from (px, py)
// rightwards, wrapping around at 256: their color indices go to idx and the
// CGB attributes of their tiles to attrs (always 0 on a DMG): palette (bits
// 0-2), VRAM bank (3), X flip (5), Y flip (6), priority (7).
func (p *PPU) mapRow(base, px, py int, signed, cgb bool, idx, attrs []byte) {
	rowBase := base + (py/8)*32
	for i := 0; i < len(idx); {
		off := rowBase + px/8
		var attr byte
		if cgb {
			attr = p.vram[0x2000+off]
		}
		row := py & 7
		if attr&0x40 != 0 {
			row = 7 - row
		}
		lo, hi := p.tileRow(int(attr>>3&1), p.vram[off], row, signed)
		if attr&0x20 != 0 {
			lo, hi = bits.Reverse8(lo), bits.Reverse8(hi)
		}
		if px&7 == 0 && i+8 <= len(idx) {
			// A whole tile, the common case: the 8 pixels at once.
			pixels(idx[i:i+8:i+8], lo, hi)
			fill8(attrs[i:i+8:i+8], attr)
			i += 8
		} else {
			for x := px & 7; x < 8 && i < len(idx); x++ {
				idx[i], attrs[i] = colorIndex(lo, hi, 7-x), attr
				i++
			}
		}
		px = (px | 7 + 1) & 0xFF // next tile
	}
}

func (p *PPU) renderLine() {
	ly := int(p.ly)
	cgb := p.bus.cgbMode()
	var bgIdx [ScreenWidth]byte  // raw color indices, used for sprite priority
	var bgAttr [ScreenWidth]byte // CGB tile attributes

	if p.ly == p.wy {
		p.wyReached = true
	}
	if ly == midLine {
		p.midSCX, p.midSCY = p.scx, p.scy
	}

	// On a CGB, LCDC bit 0 does not hide the background, it only takes its
	// priority over the sprites away (except in compatibility mode).
	if cgb || p.lcdc&0x01 != 0 {
		signed := p.lcdc&0x10 == 0
		bgMap := 0x1800
		if p.lcdc&0x08 != 0 {
			bgMap = 0x1C00
		}
		y := (int(p.scy) + ly) & 0xFF
		p.mapRow(bgMap, int(p.scx), y, signed, cgb, bgIdx[:], bgAttr[:])

		wx := int(p.wx) - 7
		if p.lcdc&0x20 != 0 && p.wyReached && wx < ScreenWidth {
			winMap := 0x1800
			if p.lcdc&0x40 != 0 {
				winMap = 0x1C00
			}
			x := max(wx, 0)
			p.mapRow(winMap, x-wx, p.windowLine, signed, cgb, bgIdx[x:], bgAttr[x:])
			p.windowLine++
		}
	}

	var obj [ScreenWidth]objPixel
	sprites := p.lcdc&0x02 != 0 && p.renderSprites(ly, &obj)

	if cgb {
		out := p.cback[ly*ScreenWidth : (ly+1)*ScreenWidth]
		if !sprites { // many lines: the background alone
			for x := range ScreenWidth {
				out[x] = color(&p.bgPal, bgAttr[x]&7, bgIdx[x])
			}
			return
		}
		master := p.lcdc&0x01 != 0
		for x := range ScreenWidth {
			o := obj[x]
			bgWins := master && bgIdx[x] != 0 && (bgAttr[x]&0x80 != 0 || o.attr&0x80 != 0)
			if o.idx != 0 && !bgWins {
				out[x] = color(&p.objPal, o.attr&7, o.idx)
			} else {
				out[x] = color(&p.bgPal, bgAttr[x]&7, bgIdx[x])
			}
		}
		return
	}

	// DMG, or a DMG game on a CGB: there the shades index the color
	// palettes, BG palette 0 and OBJ palettes 0 and 1.
	out := p.back[ly*ScreenWidth : (ly+1)*ScreenWidth]
	cout := p.cback[ly*ScreenWidth : (ly+1)*ScreenWidth]
	compat := p.bus.compat
	if !sprites && !compat {
		for x := range ScreenWidth {
			out[x] = shade(p.bgp, bgIdx[x])
		}
		return
	}
	for x := range ScreenWidth {
		o := obj[x]
		if o.idx != 0 && (o.attr&0x80 == 0 || bgIdx[x] == 0) {
			pal, n := p.obp0, byte(0)
			if o.attr&0x10 != 0 {
				pal, n = p.obp1, 1
			}
			out[x] = shade(pal, o.idx)
			if compat {
				cout[x] = color(&p.objPal, n, out[x])
			}
		} else {
			out[x] = shade(p.bgp, bgIdx[x])
			if compat {
				cout[x] = color(&p.bgPal, 0, out[x])
			}
		}
	}
}

// objPixel is the frontmost opaque sprite pixel at some X: its color index
// (0 when there is none) and the attributes of its sprite.
type objPixel struct{ idx, attr byte }

// renderSprites draws the sprites of line ly in out, and reports whether it
// drew any pixel.
func (p *PPU) renderSprites(ly int, out *[ScreenWidth]objPixel) bool {
	drawn := false
	height := 8
	if p.lcdc&0x04 != 0 {
		height = 16
	}
	// Select up to 10 sprites on this line, in OAM order.
	var sel [10]int
	n := 0
	for i := 0; i < 40 && n < 10; i++ {
		y := int(p.oam[i*4]) - 16
		if ly >= y && ly < y+height {
			sel[n] = i
			n++
		}
	}
	// DMG priority: lower X first, then lower OAM index (insertion sort keeps
	// the OAM order stable). A CGB only uses the OAM order.
	if !p.bus.cgbMode() || p.opri&1 != 0 {
		for i := 1; i < n; i++ {
			for j := i; j > 0 && p.oam[sel[j]*4+1] < p.oam[sel[j-1]*4+1]; j-- {
				sel[j], sel[j-1] = sel[j-1], sel[j]
			}
		}
	}
	for k := range n {
		o := p.oam[sel[k]*4 : sel[k]*4+4]
		y, x, tile, attr := int(o[0])-16, int(o[1])-8, o[2], o[3]
		row := ly - y
		if attr&0x40 != 0 {
			row = height - 1 - row
		}
		if height == 16 {
			tile &^= 1
		}
		bank := 0
		if p.bus.cgbMode() {
			bank = int(attr >> 3 & 1)
		}
		lo, hi := p.tileRow(bank, tile, row, false)
		for i := range 8 {
			sx := x + i
			if sx < 0 || sx >= ScreenWidth || out[sx].idx != 0 {
				continue
			}
			bit := 7 - i
			if attr&0x20 != 0 {
				bit = i
			}
			if idx := colorIndex(lo, hi, bit); idx != 0 {
				out[sx] = objPixel{idx, attr}
				drawn = true
			}
		}
	}
	return drawn
}
