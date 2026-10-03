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
	return loadROMModel(t, path, ModelAuto)
}

func loadROMModel(t testing.TB, path string, model Model) *GameBoy {
	t.Helper()
	rom, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("%s not available: %v", path, err)
	}
	cart, err := NewCartridge(rom)
	if err != nil {
		t.Fatal(err)
	}
	g, err := NewModel(cart, nil, model)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestBlargg(t *testing.T) {
	models := map[string]Model{"dmg": ModelDMG, "cgb": ModelCGB}
	for _, name := range []string{"cpu_instrs", "instr_timing", "mem_timing"} {
		for mname, model := range models {
			t.Run(name+"/"+mname, func(t *testing.T) {
				runBlargg(t, loadROMModel(t, filepath.Join(testROMDir, name+".gb"), model))
			})
		}
	}
}

func runBlargg(t *testing.T, g *GameBoy) {
	t.Helper()
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

func TestCGBAcid2(t *testing.T) {
	g := loadROM(t, filepath.Join(testROMDir, "cgb-acid2.gbc"))
	f, err := os.Open(filepath.Join(testROMDir, "cgb-acid2-ref.png"))
	if err != nil {
		t.Skip(err)
	}
	defer f.Close()
	ref, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if !g.IsCGB() {
		t.Fatal("cgb-acid2 not run in CGB mode")
	}
	for range 120 {
		g.RunFrame()
		g.APU.DrainSamples()
	}
	fb := g.ColorFramebuffer()
	scale := func(c uint16) uint32 { return uint32(c<<3 | c>>2) }
	mismatches := 0
	for y := range ScreenHeight {
		for x := range ScreenWidth {
			r, gr, b, _ := ref.At(x, y).RGBA()
			c := fb[y*ScreenWidth+x]
			got := [3]uint32{scale(c & 0x1F), scale(c >> 5 & 0x1F), scale(c >> 10 & 0x1F)}
			if want := [3]uint32{r >> 8, gr >> 8, b >> 8}; got != want {
				if mismatches < 10 {
					t.Errorf("pixel (%d,%d) = %v, want %v", x, y, got, want)
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

func TestCGBBootROM(t *testing.T) {
	boot, err := os.ReadFile("../../bios/gbc_bios.bin")
	if err != nil {
		t.Skip(err)
	}
	rom, err := os.ReadFile(filepath.Join(testROMDir, "cgb-acid2.gbc"))
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
	if g.PC() != 0x0100 || g.CPU.a != 0x11 || g.CPU.sp != 0xFFFE {
		t.Errorf("after boot: PC=%04X A=%02X SP=%04X", g.PC(), g.CPU.a, g.CPU.sp)
	}
}
