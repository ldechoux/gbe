package gb

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// Test ROMs are not committed; fetch them into testroms/ (see README) to run
// these tests. They are skipped otherwise.
const testROMDir = "../../testroms"

func loadROM(t *testing.T, path string) *GameBoy {
	t.Helper()
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("%s not available: %v", path, err)
	}
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

func TestBlargg(t *testing.T) {
	for _, name := range []string{"cpu_instrs", "instr_timing", "mem_timing"} {
		t.Run(name, func(t *testing.T) {
			g := loadROM(t, filepath.Join(testROMDir, name+".gb"))
			for range 60 * 60 {
				g.RunFrame()
				g.APU.DrainSamples()
				if bytes.Contains(g.Serial.Output, []byte("Passed")) {
					return
				}
				if bytes.Contains(g.Serial.Output, []byte("Failed")) {
					break
				}
			}
			t.Fatalf("serial output:\n%s", g.Serial.Output)
		})
	}
}

func TestDMGAcid2(t *testing.T) {
	g := loadROM(t, filepath.Join(testROMDir, "dmg-acid2.gb"))
	f, err := os.Open(filepath.Join(testROMDir, "dmg-acid2-ref.png"))
	if err != nil {
		t.Skip(err)
	}
	defer f.Close()
	ref, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	for range 120 {
		g.RunFrame()
		g.APU.DrainSamples()
	}
	fb := g.Framebuffer()
	mismatches := 0
	for y := range ScreenHeight {
		for x := range ScreenWidth {
			r, _, _, _ := ref.At(x, y).RGBA()
			want := byte(3 - (r>>8)/0x55) // 0xFF->0, 0xAA->1, 0x55->2, 0x00->3
			if got := fb[y*ScreenWidth+x]; got != want {
				if mismatches < 10 {
					t.Errorf("pixel (%d,%d) = %d, want %d", x, y, got, want)
				}
				mismatches++
			}
		}
	}
	if mismatches > 0 {
		t.Fatalf("%d pixels differ from the reference", mismatches)
	}
}

func TestBootROM(t *testing.T) {
	boot, err := os.ReadFile("../../bios/gb_bios.bin")
	if err != nil {
		t.Skip(err)
	}
	rom, err := os.ReadFile("../../roms/Super_Mario_Land_World_Rev1.gb")
	if err != nil {
		t.Skip(err)
	}
	cart, err := NewCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	g, err := New(cart, boot)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 400*CyclesPerFrame/4 && g.BootROMActive(); i++ {
		g.CPU.Step()
	}
	if g.BootROMActive() {
		t.Fatalf("boot ROM still active, PC=%04X", g.PC())
	}
	if g.PC() != 0x0100 || g.CPU.a != 0x01 || g.CPU.sp != 0xFFFE {
		t.Errorf("after boot: PC=%04X A=%02X SP=%04X", g.PC(), g.CPU.a, g.CPU.sp)
	}
}
