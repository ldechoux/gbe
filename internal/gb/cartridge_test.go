package gb

import (
	"archive/zip"
	"bytes"
	"io"
	"path/filepath"
	"testing"
	"time"
)

// newTestCart builds a cartridge of the given type with banks ROM banks, each
// tagged with its number (low byte at offset 0, high byte at offset 1).
func newTestCart(t *testing.T, typ, ramSize byte, banks int) *Cartridge {
	t.Helper()
	rom := make([]byte, banks*0x4000)
	for bank := range banks {
		rom[bank*0x4000] = byte(bank)
		rom[bank*0x4000+1] = byte(bank >> 8)
	}
	copy(rom[0x134:], "TEST")
	rom[0x147] = typ
	rom[0x149] = ramSize
	cart, err := NewCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	return cart
}

// bank reports which ROM bank is mapped at addr (0x0000 or 0x4000).
func bank(c *Cartridge, addr uint16) int {
	return int(c.readROM(addr)) | int(c.readROM(addr+1))<<8
}

func TestNewCartridge(t *testing.T) {
	if _, err := NewCartridge(make([]byte, 0x14F)); err == nil {
		t.Error("ROM smaller than the header accepted")
	}
	rom := make([]byte, 0x8000)
	rom[0x147] = 0x20 // MBC6
	if _, err := NewCartridge(rom); err == nil {
		t.Error("unsupported cartridge type accepted")
	}

	cases := []struct {
		typ, ramCode byte
		battery      bool
		ram          int
	}{
		{0x00, 0, false, 0},
		{0x09, 2, true, 0x2000},
		{0x01, 0, false, 0},
		{0x03, 3, true, 0x8000},
		{0x05, 0, false, 512}, // MBC2 RAM is built in, whatever the header says
		{0x06, 0, true, 512},
		{0x11, 0, false, 0},
		{0x10, 3, true, 0x8000},
		{0x19, 0, false, 0},
		{0x1B, 4, true, 0x20000},
		{0x1E, 5, true, 0x10000},
	}
	for _, c := range cases {
		cart := newTestCart(t, c.typ, c.ramCode, 2)
		if cart.Battery != c.battery || len(cart.ram) != c.ram {
			t.Errorf("type %02X: battery=%v ram=%d, want %v %d",
				c.typ, cart.Battery, len(cart.ram), c.battery, c.ram)
		}
	}

	rom = make([]byte, 0x9000)
	copy(rom[0x134:], "TETRIS  \x00\x00")
	cart, err := NewCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	if cart.Title != "TETRIS" {
		t.Errorf("title %q, want TETRIS", cart.Title)
	}
	// A Game Boy Color title followed by a manufacturer code.
	copy(rom[0x134:], "DK COUNTRY\x00BDDE\xC0")
	if cart, err := NewCartridge(rom); err != nil {
		t.Error(err)
	} else if cart.Title != "DK COUNTRY" {
		t.Errorf("title %q, want DK COUNTRY", cart.Title)
	}
	if len(cart.rom) != 0x10000 || cart.rom[0x9000] != 0xFF || cart.rom[0xFFFF] != 0xFF {
		t.Errorf("ROM not padded to a power of two with 0xFF (size %#x)", len(cart.rom))
	}
}

func TestROMOnly(t *testing.T) {
	cart := newTestCart(t, 0x00, 0, 2)
	cart.writeROM(0x2000, 0x01)
	if got := cart.readROM(0x0000); got != 0 {
		t.Errorf("ROM write changed the data: %02X", got)
	}
	if got := cart.readROM(0x4000); got != 1 {
		t.Errorf("0x4000 reads bank %d, want 1", got)
	}
	cart.writeRAM(0xA000, 0x42)
	if got := cart.readRAM(0xA000); got != 0xFF {
		t.Errorf("missing RAM reads %02X, want FF", got)
	}

	cart = newTestCart(t, 0x09, 2, 2)
	cart.writeRAM(0xA123, 0x42)
	if got := cart.readRAM(0xA123); got != 0x42 {
		t.Errorf("RAM reads %02X, want 42", got)
	}
	if !cart.Dirty() {
		t.Error("battery RAM write not marked dirty")
	}
}

func TestMBC1(t *testing.T) {
	cart := newTestCart(t, 0x03, 3, 64) // 1 MiB ROM, 4 RAM banks

	cart.writeROM(0x2000, 0x20) // only the low 5 bits count, 0 maps to 1
	if got := bank(cart, 0x4000); got != 1 {
		t.Errorf("bank1=0x20 maps bank %d, want 1", got)
	}
	cart.writeROM(0x2000, 0x05)
	cart.writeROM(0x4000, 0x01) // upper bits
	if got := bank(cart, 0x4000); got != 0x25 {
		t.Errorf("0x4000 maps bank %#x, want 0x25", got)
	}
	if got := bank(cart, 0x0000); got != 0 {
		t.Errorf("mode 0: 0x0000 maps bank %#x, want 0", got)
	}
	cart.writeROM(0x6000, 0x01)
	if got := bank(cart, 0x0000); got != 0x20 {
		t.Errorf("mode 1: 0x0000 maps bank %#x, want 0x20", got)
	}

	cart.writeROM(0x6000, 0x00)
	cart.writeRAM(0xA000, 0x11)
	if got := cart.readRAM(0xA000); got != 0xFF {
		t.Errorf("disabled RAM reads %02X, want FF", got)
	}
	if cart.Dirty() {
		t.Error("write to disabled RAM marked dirty")
	}
	cart.writeROM(0x0000, 0x1A) // only the low nibble is checked
	cart.writeRAM(0xA000, 0x11) // mode 0 always uses RAM bank 0
	if cart.ram[0] != 0x11 {
		t.Error("mode 0 write did not land in RAM bank 0")
	}
	cart.writeROM(0x6000, 0x01)
	cart.writeROM(0x4000, 0x02)
	cart.writeRAM(0xA000, 0x22)
	if cart.ram[2*0x2000] != 0x22 || cart.readRAM(0xA000) != 0x22 {
		t.Error("mode 1 did not select RAM bank 2")
	}
	cart.writeROM(0x0000, 0x00)
	if got := cart.readRAM(0xA000); got != 0xFF {
		t.Errorf("RAM still readable after disable: %02X", got)
	}

	small := newTestCart(t, 0x01, 0, 4)
	small.writeROM(0x2000, 0x05)
	if got := bank(small, 0x4000); got != 1 {
		t.Errorf("bank 5 of 4 maps bank %d, want 1 (wrapped)", got)
	}
	small.writeROM(0x0000, 0x0A)
	if got := small.readRAM(0xA000); got != 0xFF {
		t.Errorf("cartridge without RAM reads %02X, want FF", got)
	}
	small.writeRAM(0xA000, 0x11) // must not panic
}

func TestMBC2(t *testing.T) {
	cart := newTestCart(t, 0x06, 0, 16)

	cart.writeROM(0x2100, 0x03) // address bit 8 set: ROM bank
	if got := bank(cart, 0x4000); got != 3 {
		t.Errorf("0x4000 maps bank %d, want 3", got)
	}
	cart.writeROM(0x2100, 0x10) // 4 bits, 0 maps to 1
	if got := bank(cart, 0x4000); got != 1 {
		t.Errorf("bank 0x10 maps bank %d, want 1", got)
	}
	cart.writeROM(0x4100, 0x05) // above 0x3FFF: ignored
	if got := bank(cart, 0x4000); got != 1 {
		t.Errorf("write to 0x4100 changed the bank to %d", got)
	}
	if got := bank(cart, 0x0000); got != 0 {
		t.Errorf("0x0000 maps bank %d, want 0", got)
	}

	if got := cart.readRAM(0xA000); got != 0xFF {
		t.Errorf("disabled RAM reads %02X, want FF", got)
	}
	cart.writeRAM(0xA000, 0x01)
	if cart.Dirty() {
		t.Error("write to disabled RAM marked dirty")
	}
	cart.writeROM(0x2000, 0x0A) // address bit 8 clear: RAM enable
	if got := bank(cart, 0x4000); got != 1 {
		t.Errorf("RAM enable changed the ROM bank to %d", got)
	}
	cart.writeRAM(0xA005, 0xAB)
	if got := cart.readRAM(0xA005); got != 0xFB {
		t.Errorf("RAM reads %02X, want FB (4 bits, upper nibble set)", got)
	}
	if got := cart.readRAM(0xA205); got != 0xFB {
		t.Errorf("RAM echo at 0xA205 reads %02X, want FB", got)
	}
	if !cart.Dirty() {
		t.Error("battery RAM write not marked dirty")
	}
}

func TestMBC3(t *testing.T) {
	cart := newTestCart(t, 0x13, 3, 256) // 4 MiB ROM, 4 RAM banks

	cart.writeROM(0x2000, 0xFF) // 7 bits
	if got := bank(cart, 0x4000); got != 0x7F {
		t.Errorf("0x4000 maps bank %#x, want 0x7F", got)
	}
	cart.writeROM(0x2000, 0x00)
	if got := bank(cart, 0x4000); got != 1 {
		t.Errorf("bank 0 maps bank %d, want 1", got)
	}
	if got := bank(cart, 0x0000); got != 0 {
		t.Errorf("0x0000 maps bank %d, want 0", got)
	}

	cart.writeRAM(0xA000, 0x11)
	if got := cart.readRAM(0xA000); got != 0xFF || cart.Dirty() {
		t.Errorf("disabled RAM reads %02X (dirty=%v), want FF", got, cart.Dirty())
	}
	cart.writeROM(0x0000, 0x0A)
	for b := range byte(4) {
		cart.writeROM(0x4000, b)
		cart.writeRAM(0xA010, 0x40+b)
	}
	for b := range byte(4) {
		cart.writeROM(0x4000, b)
		if got := cart.readRAM(0xA010); got != 0x40+b {
			t.Errorf("RAM bank %d reads %02X, want %02X", b, got, 0x40+b)
		}
	}
	cart.writeROM(0x4000, 0x04) // neither a RAM bank nor an RTC register
	cart.writeRAM(0xA010, 0x99)
	if got := cart.readRAM(0xA010); got != 0xFF {
		t.Errorf("RAM select 4 reads %02X, want FF", got)
	}

	// The latch only fires on a 0 -> 1 transition.
	cart.writeROM(0x4000, 0x08) // seconds
	cart.writeROM(0x6000, 1)
	cart.writeRAM(0xA000, 30)
	cart.rtc.s = 45
	cart.writeROM(0x6000, 1)
	if got := cart.readRAM(0xA000); got != 30 {
		t.Errorf("latch without a 0 first: seconds %d, want 30", got)
	}
	cart.writeROM(0x6000, 0)
	cart.writeROM(0x6000, 1)
	if got := cart.readRAM(0xA000); got < 45 {
		t.Errorf("latched seconds %d, want at least 45", got)
	}

	noRAM := newTestCart(t, 0x11, 0, 2)
	noRAM.writeROM(0x0000, 0x0A)
	noRAM.writeRAM(0xA000, 0x11)
	if got := noRAM.readRAM(0xA000); got != 0xFF {
		t.Errorf("cartridge without RAM reads %02X, want FF", got)
	}
}

func TestRTC(t *testing.T) {
	const day = 86400
	r := &rtc{last: 1000}
	r.update(1000 + day + 3600 + 60 + 1)
	if r.d != 1 || r.h != 1 || r.m != 1 || r.s != 1 {
		t.Errorf("after 1d1h1m1s: %dd %02d:%02d:%02d", r.d, r.h, r.m, r.s)
	}
	r.update(500) // the host clock went backwards
	if r.d != 1 || r.s != 1 {
		t.Errorf("negative delta moved the clock: %dd %02ds", r.d, r.s)
	}

	r = &rtc{last: 0, d: 511}
	r.update(day + 5)
	if r.d != 0 || !r.carry || r.s != 5 {
		t.Errorf("day overflow: day %d carry %v s %d, want 0 true 5", r.d, r.carry, r.s)
	}

	r = &rtc{last: 0}
	r.set(0, 4, 0x41) // halt, day bit 8
	r.update(day)
	if r.d != 0x100 || r.s != 0 || !r.halt {
		t.Errorf("halted clock moved: day %#x s %d", r.d, r.s)
	}
	if got := r.regs(); got[4] != 0x41 {
		t.Errorf("DH register %02X, want 41", got[4])
	}
	r.set(day, 4, 0x80) // resume, keep carry
	if r.halt || !r.carry || r.d != 0 {
		t.Errorf("DH=80: halt %v carry %v day %d", r.halt, r.carry, r.d)
	}
	if got := r.regs(); got[4] != 0x80 {
		t.Errorf("DH register %02X, want 80", got[4])
	}

	r = &rtc{last: 0}
	r.set(0, 0, 61)
	r.set(0, 1, 75)
	r.set(0, 2, 25)
	r.set(0, 3, 0xAB)
	if r.s != 1 || r.m != 15 || r.h != 1 || r.d != 0xAB {
		t.Errorf("set wraps: %dd %02d:%02d:%02d, want 171d 01:15:01", r.d, r.h, r.m, r.s)
	}
	if r.latched != r.regs() {
		t.Errorf("written registers not visible: latched %v, regs %v", r.latched, r.regs())
	}

	r.latch(0)
	r.update(10)
	if r.latched[0] != 1 {
		t.Errorf("latched seconds moved to %d before the next latch", r.latched[0])
	}
	r.latch(10)
	if r.latched[0] != 11 {
		t.Errorf("latched seconds %d, want 11", r.latched[0])
	}

	now := time.Now().Unix()
	r = &rtc{s: 10, m: 20, h: 3, d: 0x1FF, carry: true, last: now}
	r.latched = r.regs()
	var back rtc
	back.unmarshal(r.marshal())
	if back.latched != r.latched || back.d != 0x1FF || !back.carry || back.h != 3 {
		t.Errorf("marshal round trip: got %+v, want %+v", back, *r)
	}
}

func TestMBC5(t *testing.T) {
	cart := newTestCart(t, 0x1B, 4, 512) // 8 MiB ROM, 16 RAM banks

	cart.writeROM(0x2000, 0x00) // unlike other MBCs, bank 0 is selectable
	if got := bank(cart, 0x4000); got != 0 {
		t.Errorf("0x4000 maps bank %d, want 0", got)
	}
	cart.writeROM(0x3000, 0x03) // only bit 0 counts
	if got := bank(cart, 0x4000); got != 0x100 {
		t.Errorf("0x4000 maps bank %#x, want 0x100", got)
	}
	cart.writeROM(0x2000, 0xFF)
	if got := bank(cart, 0x4000); got != 0x1FF {
		t.Errorf("0x4000 maps bank %#x, want 0x1FF", got)
	}
	cart.writeROM(0x6000, 0xFF) // no register there
	if got := bank(cart, 0x4000); got != 0x1FF {
		t.Errorf("write to 0x6000 changed the bank to %#x", got)
	}
	if got := bank(cart, 0x0000); got != 0 {
		t.Errorf("0x0000 maps bank %#x, want 0", got)
	}

	if got := cart.readRAM(0xA000); got != 0xFF {
		t.Errorf("disabled RAM reads %02X, want FF", got)
	}
	cart.writeROM(0x0000, 0x0A)
	cart.writeROM(0x4000, 0x1F) // 4 bits: bank 15
	cart.writeRAM(0xBFFF, 0x5A)
	if cart.ram[15*0x2000+0x1FFF] != 0x5A || cart.readRAM(0xBFFF) != 0x5A {
		t.Error("RAM bank 15 not selected")
	}
	cart.writeROM(0x0000, 0x00)
	cart.writeRAM(0xBFFF, 0x00)
	if cart.ram[15*0x2000+0x1FFF] != 0x5A {
		t.Error("write to disabled RAM went through")
	}
}

func TestSaveData(t *testing.T) {
	cart := newTestCart(t, 0x03, 2, 2)
	cart.writeROM(0x0000, 0x0A)
	cart.writeRAM(0xA000, 0x42)
	data := cart.SaveData()
	if cart.Dirty() {
		t.Error("still dirty after SaveData")
	}
	if len(data) != 0x2000 || data[0] != 0x42 {
		t.Fatalf("save is %d bytes starting with %02X", len(data), data[0])
	}
	other := newTestCart(t, 0x03, 2, 2)
	other.LoadSaveData(data)
	if !bytes.Equal(other.ram, cart.ram) {
		t.Error("RAM not restored")
	}

	// A save written before the RTC existed has no clock data: keep the clock.
	mbc3 := newTestCart(t, 0x10, 2, 2)
	mbc3.rtc.h = 7
	mbc3.LoadSaveData(data)
	if mbc3.ram[0] != 0x42 || mbc3.rtc.h != 7 {
		t.Errorf("RAM-only save: ram[0]=%02X hours=%d, want 42 7", mbc3.ram[0], mbc3.rtc.h)
	}
}

type romWrite struct {
	addr uint16
	v    byte
}

func TestSaveStateRestoresMBC(t *testing.T) {
	cases := []struct {
		name    string
		typ     byte
		ramCode byte
		setup   []romWrite
		romBank int
	}{
		{"ROM only", 0x09, 2, nil, 1},
		{"MBC1", 0x03, 3, []romWrite{{0x0000, 0x0A}, {0x2000, 0x05}, {0x4000, 0x01}, {0x6000, 0x01}}, 0x25},
		{"MBC2", 0x06, 0, []romWrite{{0x0000, 0x0A}, {0x2100, 0x07}}, 7},
		{"MBC3", 0x10, 3, []romWrite{{0x0000, 0x0A}, {0x2000, 0x2A}, {0x4000, 0x02}}, 0x2A},
		{"MBC5", 0x1B, 4, []romWrite{{0x0000, 0x0A}, {0x2000, 0x34}, {0x3000, 0x01}, {0x4000, 0x09}}, 0x134},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cart := newTestCart(t, c.typ, c.ramCode, 512)
			for _, w := range c.setup {
				cart.writeROM(w.addr, w.v)
			}
			cart.writeRAM(0xA001, 0x0C)
			if cart.rtc != nil {
				cart.rtc.d, cart.rtc.halt = 300, true
			}
			g, err := New(cart, nil)
			if err != nil {
				t.Fatal(err)
			}
			state := g.SaveState()

			fresh := newTestCart(t, c.typ, c.ramCode, 512)
			g2, err := New(fresh, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := g2.LoadState(state); err != nil {
				t.Fatal(err)
			}
			if got := bank(fresh, 0x4000); got != c.romBank {
				t.Errorf("ROM bank %#x, want %#x", got, c.romBank)
			}
			if got, want := fresh.readRAM(0xA001), cart.readRAM(0xA001); got != want {
				t.Errorf("RAM reads %02X, want %02X", got, want)
			}
			if !bytes.Equal(fresh.ram, cart.ram) {
				t.Error("RAM contents differ")
			}
			if !fresh.Dirty() {
				t.Error("restored battery RAM not marked dirty")
			}
			if cart.rtc != nil && (fresh.rtc.d != 300 || !fresh.rtc.halt) {
				t.Errorf("RTC day %d halt %v, want 300 true", fresh.rtc.d, fresh.rtc.halt)
			}
		})
	}
}

func TestMotorShare(t *testing.T) {
	c := &Cartridge{rumble: true}
	c.setMotor(true, 100)
	c.setMotor(false, 600)
	c.setMotor(true, 900)
	if got := c.motorShare(0, 1000); got != 0.6 {
		t.Errorf("share %v, want 0.6", got)
	}
	if got := c.motorShare(1000, 2000); got != 1 {
		t.Errorf("share %v while the motor kept running, want 1", got)
	}
	c.setMotor(false, 2500)
	if got := c.motorShare(2000, 3000); got != 0.5 {
		t.Errorf("share %v, want 0.5", got)
	}
	if got := c.motorShare(3000, 4000); got != 0 {
		t.Errorf("share %v with the motor off, want 0", got)
	}
}

// rumbleGB runs a program that switches the motor on (bit 3 of a write to
// 0x4000), waits 256 loops of 16 T-cycles, switches it off and loops.
func rumbleGB(t *testing.T, cartType byte) *GameBoy {
	t.Helper()
	rom := make([]byte, 0x8000)
	rom[0x147] = cartType
	copy(rom[0x100:], []byte{
		0x3E, 0x08, 0xEA, 0x00, 0x40, // LD A,0x08; LD (0x4000),A: motor on
		0x06, 0x00, // LD B,0 (256 loops)
		0x05, 0x20, 0xFD, // loop: DEC B; JR NZ,loop
		0xAF, 0xEA, 0x00, 0x40, // XOR A; LD (0x4000),A: motor off
		0x18, 0xFE, // JR -2
	})
	cart, err := NewCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	g, err := New(cart, nil)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestRumble(t *testing.T) {
	g := rumbleGB(t, 0x1C) // MBC5 + rumble
	start := g.Bus.cycles
	g.RunFrame()
	frame := float64(g.Bus.cycles - start)
	want := (256*16 - 4 + 12 + 4) / frame // the loop, the last JR not taken, LD A,0x08... to the write off
	if got := g.Rumble(); got < want*0.95 || got > want*1.05 {
		t.Errorf("rumble %.4f in the first frame, want about %.4f", got, want)
	}
	g.RunFrame()
	if got := g.Rumble(); got != 0 {
		t.Errorf("rumble %v once the motor is off, want 0", got)
	}

	// Restoring a state stops the motor, which the state does not hold.
	g = rumbleGB(t, 0x1C)
	snap := g.Snapshot(nil)
	g.Cart.setMotor(true, g.Bus.cycles)
	if err := g.Restore(snap); err != nil {
		t.Fatal(err)
	}
	if g.Cart.motorOn {
		t.Error("motor still running after Restore")
	}

	// Without a motor, bit 3 selects a RAM bank and nothing shakes.
	g = rumbleGB(t, 0x19)
	g.RunFrame()
	if got := g.Rumble(); got != 0 {
		t.Errorf("rumble %v on a cartridge without a motor", got)
	}
}

// TestPokemonPinballRumble plays Pokemon Pinball (kept locally, as a zip)
// and checks that its motor runs only once the game is on.
func TestPokemonPinballRumble(t *testing.T) {
	zr, err := zip.OpenReader(filepath.Join(gameDir, "Pokemon_Pinball.zip"))
	if err != nil {
		t.Skip(err)
	}
	defer zr.Close()
	f, err := zr.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	rom, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	cart, err := NewCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	g, err := New(cart, nil)
	if err != nil {
		t.Fatal(err)
	}
	shaking := 0
	for frame := range 1500 {
		// Through the menus, then launch the ball and flip.
		g.SetButton(ButtonStart, frame%200 < 3)
		g.SetButton(ButtonA, frame%37 < 6)
		g.SetButton(ButtonLeft, frame%23 < 4)
		g.RunFrame()
		g.APU.DrainSamples()
		r := g.Rumble()
		if frame < 300 && r != 0 {
			t.Fatalf("rumble %v at frame %d, on the title screens", r, frame)
		}
		if r > 0 {
			shaking++
		}
	}
	if shaking < 20 {
		t.Errorf("the motor ran during %d frames of play, want more", shaking)
	}
	t.Logf("the motor ran during %d of 1500 frames", shaking)
}

// The titles of real Game Boy Color headers: an 11 characters long title
// is followed by a manufacturer code, which is not part of it.
func TestCGBTitles(t *testing.T) {
	for _, c := range []struct {
		header   string // 0x0134-0x0143
		licensee byte   // 0x014B
		want     string
	}{
		{"POKEMONPINBVPHP\x80", 0x33, "POKEMONPINB"},
		{"ALONE IN THBIDP\xC0", 0x33, "ALONE IN TH"},
		{"ZELDA NAYRUAZ8P\xC0", 0x33, "ZELDA NAYRU"},
		{"ZELDA DIN\x00\x00AZ7P\xC0", 0x33, "ZELDA DIN"},
		{"WARIOLAND3\x00AW8A\xC0", 0x33, "WARIOLAND3"},
		{"TETRIS DX\x00\x00\x00\x00\x00\x00\x80", 0x33, "TETRIS DX"},
		// 15 characters long titles, without a manufacturer code.
		{"POKEMON CRYSTAL\xC0", 0x01, "POKEMON CRYSTAL"},
		{"SUPER GAME 1998\x80", 0x33, "SUPER GAME 1998"},
		{"A GAME FOR YOU \x80", 0x33, "A GAME FOR YOU"},
		// DMG games: 16 bytes, whatever the licensee.
		{"SUPER MARIOLAND\x00", 0x01, "SUPER MARIOLAND"},
	} {
		rom := make([]byte, 0x8000)
		copy(rom[0x134:], c.header)
		rom[0x14B] = c.licensee
		cart, err := NewCartridge(rom)
		if err != nil {
			t.Fatal(err)
		}
		if cart.Title != c.want {
			t.Errorf("header %q (licensee %02X): title %q, want %q", c.header, c.licensee, cart.Title, c.want)
		}
	}
}
