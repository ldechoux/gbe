package gb

import (
	"errors"
	"hash/fnv"
	"os"
	"testing"
)

// runAndHash runs n frames with Right held and hashes video and audio.
func runAndHash(g *GameBoy, n int) uint64 {
	h := fnv.New64a()
	g.SetButton(ButtonRight, true)
	for range n {
		g.RunFrame()
		h.Write(g.Framebuffer()[:])
		for _, s := range g.APU.DrainSamples() {
			h.Write([]byte{byte(s), byte(s >> 8)})
		}
	}
	return h.Sum64()
}

func TestSaveStateRestoresExactly(t *testing.T) {
	rom, err := os.ReadFile("../../roms/Super_Mario_Land_World_Rev1.gb")
	if err != nil {
		t.Skip(err)
	}
	cart, _ := NewCartridge(rom)
	g, _ := New(cart, nil)
	g.SetButton(ButtonStart, true)
	for range 200 {
		g.RunFrame()
	}
	g.SetButton(ButtonStart, false)
	for range 100 {
		g.RunFrame()
	}
	g.APU.DrainSamples()

	state := g.SaveState()
	want := runAndHash(g, 300)

	cart2, _ := NewCartridge(rom)
	g2, _ := New(cart2, nil)
	if err := g2.LoadState(state); err != nil {
		t.Fatal(err)
	}
	if got := runAndHash(g2, 300); got != want {
		t.Fatal("emulation diverged after restoring the state")
	}
	// Loading into the machine that produced it rewinds it as well.
	if err := g.LoadState(state); err != nil {
		t.Fatal(err)
	}
	if got := runAndHash(g, 300); got != want {
		t.Fatal("emulation diverged after rewinding")
	}
}

func TestLoadStateRejectsOtherROMAndGarbage(t *testing.T) {
	g := newTestGB(t, 0x00)
	state := g.SaveState()

	rom := make([]byte, 0x8000)
	copy(rom[0x134:], "OTHER GAME")
	cart, _ := NewCartridge(rom)
	other, _ := New(cart, nil)
	if err := other.LoadState(state); !errors.Is(err, ErrStateMismatch) {
		t.Fatalf("got %v, want ErrStateMismatch", err)
	}

	pc := g.PC()
	g.CPU.Step()
	if err := g.LoadState(state[:len(state)/2]); err == nil {
		t.Fatal("truncated state accepted")
	}
	if g.PC() != pc+1 {
		t.Fatalf("failed load changed the machine: PC=%04X", g.PC())
	}
}
