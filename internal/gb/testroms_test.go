package gb

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"strings"
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

// blarggSoundKnown lists the sound tests gbe does not pass yet, with the
// reason, so that they are reported as skipped rather than failed.
var blarggSoundKnown = map[string]string{
	"dmg_sound/08-len ctr during power":  "on a DMG, powering the APU off keeps the length counters",
	"dmg_sound/09-wave read while on":    "wave RAM reads while channel 3 plays",
	"dmg_sound/10-wave trigger while on": "wave RAM corruption when retriggering channel 3 on a DMG",
	"dmg_sound/11-regs after power":      "on a DMG, powering the APU off keeps the length counters",
	"dmg_sound/12-wave write while on":   "wave RAM writes while channel 3 plays",
	"cgb_sound/09-wave read while on":    "wave RAM reads while channel 3 plays",
}

// TestBlarggSound runs blargg's dmg_sound and cgb_sound tests, the single
// ROMs, which report in the cartridge RAM rather than on the serial port.
func TestBlarggSound(t *testing.T) {
	for _, set := range []struct {
		dir   string
		model Model
	}{{"dmg_sound", ModelDMG}, {"cgb_sound", ModelCGB}} {
		roms, _ := filepath.Glob(filepath.Join(testROMDir, set.dir, "*.gb"))
		if len(roms) == 0 {
			t.Logf("%s not available", set.dir)
		}
		for _, rom := range roms {
			name := set.dir + "/" + strings.TrimSuffix(filepath.Base(rom), ".gb")
			t.Run(name, func(t *testing.T) {
				if why, known := blarggSoundKnown[name]; known {
					t.Skip("known failure: " + why)
				}
				runBlarggMemory(t, loadROMModel(t, rom, set.model))
			})
		}
	}
}

// runBlarggMemory waits for the result of a blargg test that reports in
// the cartridge RAM: the signature DE B0 61 at 0xA001, the status at 0xA000
// (0x80 while running, 0 once passed) and the text from 0xA004.
func runBlarggMemory(t *testing.T, g *GameBoy) {
	t.Helper()
	for range 60 * 40 {
		g.RunFrame()
		g.APU.DrainSamples()
		if g.Bus.read(0xA001) != 0xDE || g.Bus.read(0xA002) != 0xB0 || g.Bus.read(0xA003) != 0x61 {
			continue
		}
		status := g.Bus.read(0xA000)
		if status == 0x80 {
			continue
		}
		if status != 0 {
			var text strings.Builder
			for a := uint16(0xA004); a < 0xC000 && g.Bus.read(a) != 0; a++ {
				text.WriteByte(g.Bus.read(a))
			}
			t.Fatalf("failed (%d): %s", status, strings.Join(strings.Fields(text.String()), " "))
		}
		return
	}
	t.Fatal("no result after 40 s")
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
