package gb

const (
	ScreenWidth  = 160
	ScreenHeight = 144
)

// PPU renders one scanline at a time, at the end of mode 3. The framebuffer
// holds shades 0 (lightest) to 3 (darkest), already mapped through BGP/OBPx;
// the actual colors are chosen by the frontend.
type PPU struct {
	bus *Bus

	vram [0x2000]byte
	oam  [0xA0]byte

	lcdc, stat, scy, scx, ly, lyc byte
	bgp, obp0, obp1, wy, wx       byte
	dmaReg                        byte

	mode       byte
	dot        int
	statLine   bool
	windowLine int
	wyReached  bool

	back, front [ScreenWidth * ScreenHeight]byte
	frameReady  bool
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
	}
	return 0xFF
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

// tick advances the PPU by one M-cycle.
func (p *PPU) tick() {
	if p.lcdc&0x80 == 0 {
		return
	}
	p.dot += 4
	switch p.mode {
	case 2:
		if p.dot >= 80 {
			p.mode = 3
		}
	case 3:
		if p.dot >= 80+172 {
			p.renderLine()
			p.mode = 0
		}
	case 0:
		if p.dot >= 456 {
			p.dot -= 456
			p.ly++
			if p.ly == ScreenHeight {
				p.mode = 1
				p.bus.requestInterrupt(IntVBlank)
				p.front = p.back
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
	p.updateStat()
}

func (p *PPU) tileRow(tile byte, row int, signed bool) (lo, hi byte) {
	var addr int
	if signed {
		addr = 0x1000 + int(int8(tile))*16
	} else {
		addr = int(tile) * 16
	}
	addr += row * 2
	return p.vram[addr], p.vram[addr+1]
}

func colorIndex(lo, hi byte, bit int) byte {
	return (lo>>bit)&1 | ((hi>>bit)&1)<<1
}

func shade(pal, idx byte) byte { return (pal >> (idx * 2)) & 3 }

func (p *PPU) renderLine() {
	ly := int(p.ly)
	out := p.back[ly*ScreenWidth : (ly+1)*ScreenWidth]
	var bgIdx [ScreenWidth]byte // raw color indices, used for sprite priority

	if p.ly == p.wy {
		p.wyReached = true
	}

	if p.lcdc&0x01 != 0 {
		signed := p.lcdc&0x10 == 0
		bgMap := 0x1800
		if p.lcdc&0x08 != 0 {
			bgMap = 0x1C00
		}
		y := (int(p.scy) + ly) & 0xFF
		for x := range ScreenWidth {
			px := (int(p.scx) + x) & 0xFF
			tile := p.vram[bgMap+(y/8)*32+px/8]
			lo, hi := p.tileRow(tile, y&7, signed)
			bgIdx[x] = colorIndex(lo, hi, 7-px&7)
		}

		wx := int(p.wx) - 7
		if p.lcdc&0x20 != 0 && p.wyReached && wx < ScreenWidth {
			winMap := 0x1800
			if p.lcdc&0x40 != 0 {
				winMap = 0x1C00
			}
			wy := p.windowLine
			for x := max(wx, 0); x < ScreenWidth; x++ {
				px := x - wx
				tile := p.vram[winMap+(wy/8)*32+px/8]
				lo, hi := p.tileRow(tile, wy&7, signed)
				bgIdx[x] = colorIndex(lo, hi, 7-px&7)
			}
			p.windowLine++
		}
	}
	for x := range ScreenWidth {
		out[x] = shade(p.bgp, bgIdx[x])
	}

	if p.lcdc&0x02 != 0 {
		p.renderSprites(ly, out, &bgIdx)
	}
}

func (p *PPU) renderSprites(ly int, out []byte, bgIdx *[ScreenWidth]byte) {
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
	// the OAM order stable).
	for i := 1; i < n; i++ {
		for j := i; j > 0 && p.oam[sel[j]*4+1] < p.oam[sel[j-1]*4+1]; j-- {
			sel[j], sel[j-1] = sel[j-1], sel[j]
		}
	}
	var drawn [ScreenWidth]bool
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
		lo, hi := p.tileRow(tile, row, false)
		pal := p.obp0
		if attr&0x10 != 0 {
			pal = p.obp1
		}
		for i := range 8 {
			sx := x + i
			if sx < 0 || sx >= ScreenWidth || drawn[sx] {
				continue
			}
			bit := 7 - i
			if attr&0x20 != 0 {
				bit = i
			}
			idx := colorIndex(lo, hi, bit)
			if idx == 0 {
				continue
			}
			drawn[sx] = true
			if attr&0x80 != 0 && bgIdx[sx] != 0 {
				continue
			}
			out[sx] = shade(pal, idx)
		}
	}
}
