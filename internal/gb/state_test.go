package gb

import (
	"bytes"
	"compress/gzip"
	"errors"
	"hash/fnv"
	"io"
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

// A state cut short inside its data, still valid gzip, is refused and
// leaves the machine as it was.
func TestLoadStateKeepsMachineOnTruncatedData(t *testing.T) {
	g := newSyntheticGB(t, ModelCGB)
	for range 30 {
		g.RunFrame()
	}
	zr, err := gzip.NewReader(bytes.NewReader(g.SaveState()))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(zr)
	var cut bytes.Buffer
	zw := gzip.NewWriter(&cut)
	zw.Write(data[:len(data)-100])
	zw.Close()

	for range 5 {
		g.RunFrame()
	}
	g.APU.DrainSamples()
	before := g.Snapshot(nil)
	if err := g.LoadState(cut.Bytes()); err == nil {
		t.Fatal("truncated state accepted")
	}
	if !bytes.Equal(g.Snapshot(nil), before) {
		t.Fatal("failed load changed the machine")
	}
}

// Copying the Game Boy Color screens and the sound samples as they lie in
// memory writes the same bytes as encoding them one by one: the states stay
// the same on every machine.
func TestSnapshotSameBytesOnEveryMachine(t *testing.T) {
	if !littleEndian {
		t.Skip("big-endian machine: the bytes are always encoded one by one")
	}
	g := newSyntheticGB(t, ModelCGB)
	for range 30 {
		g.RunFrame()
	}
	fast := g.Snapshot(nil)
	littleEndian = false
	defer func() { littleEndian = true }()
	if slow := g.Snapshot(nil); !bytes.Equal(fast, slow) {
		t.Fatal("the snapshots differ")
	}
	if err := g.Restore(fast); err != nil {
		t.Fatal(err)
	}
	if again := g.Snapshot(nil); !bytes.Equal(fast, again) {
		t.Fatal("encoded one by one, the snapshot does not load back the same")
	}
}

// newBusyGB runs a loop that keeps changing BGP, so every frame differs.
func newBusyGB(t *testing.T) *GameBoy {
	// loop: INC A; LDH (0x47),A; JR loop
	return newTestGB(t, 0x3C, 0xE0, 0x47, 0x18, 0xFB)
}

func TestSnapshotRestoresExactly(t *testing.T) {
	g := newBusyGB(t)
	for range 10 {
		g.RunFrame()
	}
	g.APU.DrainSamples()

	snap := g.Snapshot(nil)
	want := runAndHash(g, 30)
	if err := g.Restore(snap); err != nil {
		t.Fatal(err)
	}
	if got := runAndHash(g, 30); got != want {
		t.Fatal("emulation diverged after restoring the snapshot")
	}
}

func TestSnapshotReusesBuffer(t *testing.T) {
	g := newBusyGB(t)
	first := g.Snapshot(nil)
	g.RunFrame()
	second := g.Snapshot(first)
	if &second[0] != &first[0] {
		t.Error("Snapshot allocated although the buffer was large enough")
	}
}

func TestRestoreRejectsTruncatedSnapshot(t *testing.T) {
	g := newBusyGB(t)
	snap := g.Snapshot(nil)
	if err := g.Restore(snap[:len(snap)/2]); err == nil {
		t.Fatal("truncated snapshot accepted")
	}
}
