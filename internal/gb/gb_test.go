package gb

import (
	"testing"
)

// newTestGB builds a machine around a 32 KiB ROM-only cartridge whose code
// starts at 0x0100.
func newTestGB(t *testing.T, program ...byte) *GameBoy {
	t.Helper()
	rom := make([]byte, 0x8000)
	copy(rom[0x100:], program)
	cart, err := NewCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	g, err := New(cart, nil)
	if err != nil {
		t.Fatal(err)
	}
	g.Bus.ifl = 0
	return g
}

func TestDAA(t *testing.T) {
	// LD A,0x45; ADD A,0x38; DAA -> 0x83
	g := newTestGB(t, 0x3E, 0x45, 0xC6, 0x38, 0x27)
	for range 3 {
		g.CPU.Step()
	}
	if g.CPU.a != 0x83 || g.CPU.f&flagC != 0 {
		t.Fatalf("A=%02X F=%02X, want A=83 no carry", g.CPU.a, g.CPU.f)
	}
	// LD A,0x10; SUB 0x01; DAA -> 0x09
	g = newTestGB(t, 0x3E, 0x10, 0xD6, 0x01, 0x27)
	for range 3 {
		g.CPU.Step()
	}
	if g.CPU.a != 0x09 {
		t.Fatalf("A=%02X, want 09", g.CPU.a)
	}
}

func TestInstructionCycles(t *testing.T) {
	cases := []struct {
		name    string
		program []byte
		cycles  uint64
	}{
		{"NOP", []byte{0x00}, 4},
		{"LD BC,d16", []byte{0x01, 0x34, 0x12}, 12},
		{"PUSH BC", []byte{0xC5}, 16},
		{"CALL a16", []byte{0xCD, 0x00, 0x02}, 24},
		{"JR NZ taken", []byte{0x20, 0x00}, 12}, // Z is clear after setup
		{"BIT 0,(HL)", []byte{0xCB, 0x46}, 12},
		{"SET 0,(HL)", []byte{0xCB, 0xC6}, 16},
	}
	for _, c := range cases {
		g := newTestGB(t, c.program...)
		g.CPU.f = 0
		start := g.Bus.cycles
		g.CPU.Step()
		if got := g.Bus.cycles - start; got != c.cycles {
			t.Errorf("%s: %d cycles, want %d", c.name, got, c.cycles)
		}
	}
}

func TestTimerInterrupt(t *testing.T) {
	g := newTestGB(t)
	g.Bus.write(0xFF06, 0xF0) // TMA
	g.Bus.write(0xFF05, 0xFF) // TIMA about to overflow
	g.Bus.write(0xFF07, 0x05) // enabled, 16 cycles per increment
	for i := 0; i < 8 && g.Bus.ifl&IntTimer == 0; i++ {
		g.Bus.tick()
	}
	if g.Bus.ifl&IntTimer == 0 {
		t.Fatal("timer interrupt not requested")
	}
	if tima := g.Bus.read(0xFF05); tima != 0xF0 {
		t.Fatalf("TIMA=%02X, want reload value F0", tima)
	}
}

func TestMBC1Banking(t *testing.T) {
	rom := make([]byte, 0x80000) // 512 KiB = 32 banks
	for bank := range 32 {
		rom[bank*0x4000] = byte(bank)
	}
	rom[0x147] = 0x01
	cart, err := NewCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	if got := cart.readROM(0x4000); got != 1 {
		t.Fatalf("default bank %d, want 1", got)
	}
	cart.writeROM(0x2000, 0x00) // bank 0 maps to 1
	if got := cart.readROM(0x4000); got != 1 {
		t.Fatalf("bank 0 selected %d, want 1", got)
	}
	cart.writeROM(0x2000, 0x13)
	if got := cart.readROM(0x4000); got != 0x13 {
		t.Fatalf("bank %d, want 0x13", got)
	}
}

func TestMBC5Banking(t *testing.T) {
	rom := make([]byte, 0x800000) // 8 MiB = 512 banks
	for bank := range 512 {
		rom[bank*0x4000] = byte(bank)
		rom[bank*0x4000+1] = byte(bank >> 8)
	}
	rom[0x147] = 0x1B
	rom[0x149] = 0x03
	cart, err := NewCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	cart.writeROM(0x2000, 0x2A)
	cart.writeROM(0x3000, 0x01)
	if lo, hi := cart.readROM(0x4000), cart.readROM(0x4001); lo != 0x2A || hi != 1 {
		t.Fatalf("bank %02X%02X, want 012A", hi, lo)
	}
	cart.writeROM(0x0000, 0x0A) // RAM enable
	cart.writeROM(0x4000, 2)
	cart.writeRAM(0xA000, 0x77)
	if !cart.Dirty() {
		t.Fatal("battery RAM should be dirty")
	}
	saved := cart.SaveData()
	if saved[2*0x2000] != 0x77 {
		t.Fatal("RAM bank 2 not saved at the right offset")
	}
}

func TestMBC3RTC(t *testing.T) {
	rom := make([]byte, 0x8000)
	rom[0x147] = 0x10
	rom[0x149] = 0x03
	cart, err := NewCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	cart.writeROM(0x0000, 0x0A)
	cart.writeROM(0x4000, 0x0A) // hours
	cart.writeRAM(0xA000, 5)
	cart.writeROM(0x6000, 0)
	cart.writeROM(0x6000, 1) // latch
	if got := cart.readRAM(0xA000); got != 5 {
		t.Fatalf("RTC hours %d, want 5", got)
	}
	data := cart.SaveData()
	if len(data) != 0x8000+48 {
		t.Fatalf("save size %d", len(data))
	}
	other, _ := NewCartridge(rom)
	other.LoadSaveData(data)
	if other.rtc.h != 5 {
		t.Fatalf("restored RTC hours %d, want 5", other.rtc.h)
	}
}

func TestAPUProducesSamples(t *testing.T) {
	g := newTestGB(t, 0x18, 0xFE) // JR -2: spin forever
	// Square wave on channel 2, both speakers, full volume.
	g.Bus.write(0xFF24, 0x77)
	g.Bus.write(0xFF25, 0x22)
	g.Bus.write(0xFF17, 0xF0)
	g.Bus.write(0xFF18, 0x00)
	g.Bus.write(0xFF19, 0x87)
	g.RunFrame()
	s := g.APU.DrainSamples()
	if len(s) < 1500 {
		t.Fatalf("only %d samples for one frame", len(s))
	}
	var peak int16
	for _, v := range s {
		peak = max(peak, v)
	}
	if peak < 1000 {
		t.Fatalf("channel 2 is silent (peak %d)", peak)
	}
	if g.Bus.read(0xFF26)&0x02 == 0 {
		t.Fatal("NR52 should report channel 2 as active")
	}
}
