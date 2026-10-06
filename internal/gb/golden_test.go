package gb

import (
	"encoding/binary"
	"hash"
	"path/filepath"
	"testing"
)

// The golden tests run the emulation for a while with scripted inputs and
// hash everything it produces: the audio samples, every frame, and the whole
// machine state at the end. Optimizations must not change a single bit, so
// the hashes below are only updated when the emulation is meant to behave
// differently. They are the same on every architecture.

// goldenHashes holds the expected hash of each golden run.
var goldenHashes = map[string]uint64{
	// The benchmark ROM: see benchrom_test.go.
	"benchrom/game/dmg":  0xe2187d9ed233383c,
	"benchrom/game/cgb":  0xfb4e4ce15dc8dcdc,
	"benchrom/cpu/dmg":   0x7a048f92126765d8,
	"benchrom/cpu/cgb":   0x303e2de73eb38e8e,
	"benchrom/idle/dmg":  0x3d099a77015afeee,
	"benchrom/idle/cgb":  0x3d54f78894b2dcd9,
	"benchrom/color/dmg": 0x2735898ad829c30d,
	"benchrom/color/cgb": 0xf38ded29428dc768,
	"benchrom/sound/dmg": 0x0a7cd29665ce5e37,
	"benchrom/sound/cgb": 0xeb331fd956a8d1aa,
	"benchrom/demo/dmg":  0xbdeadb377e29e676,
	"benchrom/demo/cgb":  0xdf4d10c3c8daed9f,
	"cgb-acid2":          0x9a36deeb081779c3,
	"cpu_instrs/cgb":     0x51f19dcc83a1f1f2,
	"cpu_instrs/dmg":     0xf289a8226e3771d9,
	"dmg-acid2":          0xa594fb83403ba7f4,
	"dmg-acid2/compat":   0x21136690f589b4b3,
	"halt_bug":           0x552af4d607f7f98b,
	"instr_timing":       0xd2684232781c099c,
	"mem_timing":         0xa049aae3d75103c3,
	"sml":                0x45b8842ae142e0a8,
	"synthetic/cgb":      0x1ebcddf6e6c49f2a,
	"synthetic/dmg":      0xb1d7bef80c9c3a10,
	"tetris":             0x74c6046dd0dee522,
	"tetris/compat":      0x0087049e06aef154,
	"zelda":              0x3c75f5bac2e7f469,
	"zelda-dx":           0x23d3d21e48de9b2d,
}

// gameDir holds the games kept locally (see .gitignore); like the test
// ROMs, they are skipped when missing.
const gameDir = "../../roms"

// goldenHash runs g for the given number of frames, pressing buttons on a
// fixed schedule, and hashes its output.
func goldenHash(g *GameBoy, frames int) uint64 {
	return goldenRun(g, frames, goldenInputs, nil)
}

// goldenInputs gets games past their title screen and moves the player.
func goldenInputs(g *GameBoy, frame int) {
	g.SetButton(ButtonStart, frame%120 < 5)
	g.SetButton(ButtonA, frame%90 > 80)
	g.SetButton(ButtonRight, frame%300 > 150)
	g.SetButton(ButtonDown, frame%400 > 330)
}

func hashFrame(h hash.Hash64, g *GameBoy) {
	h.Write(g.Framebuffer()[:])
	var b [ScreenWidth * ScreenHeight * 2]byte
	for i, c := range g.ColorFramebuffer() {
		binary.LittleEndian.PutUint16(b[i*2:], c)
	}
	h.Write(b[:])
}

func checkGolden(t *testing.T, name string, got uint64) {
	t.Helper()
	want, ok := goldenHashes[name]
	switch {
	case !ok:
		t.Errorf("%s: no golden hash, got %#016x", name, got)
	case got != want:
		t.Errorf("%s: hash %#016x, want %#016x", name, got, want)
	}
}

func TestGoldenSynthetic(t *testing.T) {
	for _, model := range []Model{ModelDMG, ModelCGB} {
		name := "synthetic/dmg"
		if model == ModelCGB {
			name = "synthetic/cgb"
		}
		t.Run(name, func(t *testing.T) {
			checkGolden(t, name, goldenHash(newSyntheticGB(t, model), 600))
		})
	}
}

// TestGoldenRestore checks that a busy machine carries on exactly the same
// after going back to a snapshot or a save state.
func TestGoldenRestore(t *testing.T) {
	for _, model := range []Model{ModelDMG, ModelCGB} {
		g := newSyntheticGB(t, model)
		goldenHash(g, 100)
		snap, state := g.Snapshot(nil), g.SaveState()
		want := goldenHash(g, 120)
		if err := g.Restore(snap); err != nil {
			t.Fatal(err)
		}
		if got := goldenHash(g, 120); got != want {
			t.Errorf("model %d: emulation diverged after Restore", model)
		}
		if err := g.LoadState(state); err != nil {
			t.Fatal(err)
		}
		if got := goldenHash(g, 120); got != want {
			t.Errorf("model %d: emulation diverged after LoadState", model)
		}
	}
}

func TestGoldenROMs(t *testing.T) {
	runs := []struct {
		name, path string
		model      Model
		frames     int
	}{
		{"cpu_instrs/dmg", filepath.Join(testROMDir, "cpu_instrs.gb"), ModelDMG, 600},
		{"cpu_instrs/cgb", filepath.Join(testROMDir, "cpu_instrs.gb"), ModelCGB, 600},
		{"instr_timing", filepath.Join(testROMDir, "instr_timing.gb"), ModelDMG, 120},
		{"mem_timing", filepath.Join(testROMDir, "mem_timing.gb"), ModelDMG, 120},
		{"halt_bug", filepath.Join(testROMDir, "halt_bug.gb"), ModelDMG, 120},
		{"dmg-acid2", filepath.Join(testROMDir, "dmg-acid2.gb"), ModelDMG, 30},
		{"dmg-acid2/compat", filepath.Join(testROMDir, "dmg-acid2.gb"), ModelCGB, 30},
		{"cgb-acid2", filepath.Join(testROMDir, "cgb-acid2.gbc"), ModelCGB, 30},
		{"zelda", filepath.Join(gameDir, "Legend_of_Zelda_The_Links_Awakening.gb"), ModelDMG, 1500},
		{"zelda-dx", filepath.Join(gameDir, "Legend_of_Zelda_The_Links_Awakening_DX.gbc"), ModelCGB, 1500},
		{"sml", filepath.Join(gameDir, "Super_Mario_Land_World_Rev1.gb"), ModelDMG, 1500},
		{"tetris", filepath.Join(gameDir, "Tetris_World_Rev1.gb"), ModelDMG, 1500},
		{"tetris/compat", filepath.Join(gameDir, "Tetris_World_Rev1.gb"), ModelCGB, 1500},
	}
	for _, r := range runs {
		t.Run(r.name, func(t *testing.T) {
			checkGolden(t, r.name, goldenHash(loadROMModel(t, r.path, r.model), r.frames))
		})
	}
}

// asm assembles a test program, with labels for the relative jumps.
type asm struct {
	code   []byte
	labels map[string]int
	jumps  map[int]string // offset of a JR operand -> its target
}

func (a *asm) emit(b ...byte)       { a.code = append(a.code, b...) }
func (a *asm) label(name string)    { a.labels[name] = len(a.code) }
func (a *asm) ldh(reg, v byte)      { a.emit(0x3E, v, 0xE0, reg) } // LD A,v; LDH (reg),A
func (a *asm) jr(op byte, l string) { a.emit(op, 0); a.jumps[len(a.code)-1] = l }

func (a *asm) assemble() []byte {
	for at, l := range a.jumps {
		a.code[at] = byte(a.labels[l] - (at + 1))
	}
	return a.code
}

// fill writes n bytes from addr: A starts at v and grows by step each time.
// It uses B and HL.
func (a *asm) fill(addr uint16, n, v, step byte, l string) {
	a.emit(0x21, byte(addr), byte(addr>>8)) // LD HL,addr
	a.emit(0x06, n, 0x3E, v)                // LD B,n; LD A,v
	a.label(l)
	a.emit(0x22, 0xC6, step, 0x05) // LD (HL+),A; ADD A,step; DEC B
	a.jr(0x20, l)                  // JR NZ
}

// newSyntheticGB builds a machine running a program that keeps every
// component busy: the four sound channels (sweep, envelopes, wave RAM, noise
// in both widths), the timer and the STAT interrupts, scrolling, the window
// and sprites, and on a CGB the color palettes, the VRAM bank 1 attributes
// and the double speed mode.
func newSyntheticGB(t testing.TB, model Model) *GameBoy {
	t.Helper()
	a := &asm{labels: map[string]int{}, jumps: map[int]string{}}
	a.emit(0xF3) // DI
	// Sound: everything on, all channels on both sides.
	a.ldh(0x26, 0x80)
	a.ldh(0x25, 0xFF)
	a.ldh(0x24, 0x77)
	a.ldh(0x10, 0x2B) // sweep: period 2, down, shift 3
	a.ldh(0x11, 0x80)
	a.ldh(0x12, 0xF3)
	a.ldh(0x13, 0x00)
	a.ldh(0x14, 0x87)
	a.ldh(0x16, 0x40)
	a.ldh(0x17, 0x8F)
	a.ldh(0x18, 0x50)
	a.ldh(0x19, 0xC6)
	a.fill(0xFF30, 16, 0x01, 0x13, "wave")
	a.ldh(0x1A, 0x80)
	a.ldh(0x1C, 0x20)
	a.ldh(0x1D, 0x00)
	a.ldh(0x1E, 0x85)
	a.ldh(0x21, 0xF1)
	a.ldh(0x22, 0x08) // narrow noise, period 8
	a.ldh(0x23, 0x80)
	// Tiles, maps and sprites (CGB attributes in VRAM bank 1).
	for i := range uint16(16) {
		a.fill(0x8000+i*0x100, 0, byte(i*0x5A), 0x35+byte(i)*2, "tiles"+string(rune('a'+i)))
	}
	a.ldh(0x4F, 1)
	a.fill(0x9800, 0, 0x00, 0x29, "attr1")
	a.fill(0x9C00, 0, 0x03, 0x4B, "attr2")
	a.ldh(0x4F, 0)
	for i := range uint16(4) {
		a.fill(0x9800+i*0x100, 0, byte(i), 0x07, "map"+string(rune('0'+i)))
	}
	a.fill(0xFE00, 0xA0, 0x10, 0x0B, "oam")
	// Palettes: DMG ones, and the CGB ones with auto-increment.
	a.ldh(0x47, 0xE4)
	a.ldh(0x48, 0xD2)
	a.ldh(0x49, 0x1B)
	a.ldh(0x68, 0x80)
	a.emit(0x06, 64, 0x3E, 0x11) // LD B,64; LD A,0x11
	a.label("bgpal")
	a.emit(0xE0, 0x69, 0xC6, 0x3D, 0x05) // LDH (0x69),A; ADD A,0x3D; DEC B
	a.jr(0x20, "bgpal")
	a.ldh(0x6A, 0x80)
	a.emit(0x06, 64, 0x3E, 0x07)
	a.label("objpal")
	a.emit(0xE0, 0x6B, 0xC6, 0x5B, 0x05)
	a.jr(0x20, "objpal")
	// Screen: window, 8x8 sprites.
	a.ldh(0x4A, 0x20)
	a.ldh(0x4B, 0x30)
	a.ldh(0x40, 0xF3)
	// Double speed on a CGB (ignored on a DMG).
	a.ldh(0x4D, 0x01)
	a.emit(0x10, 0x00) // STOP
	// Timer, LYC and HBlank STAT interrupts, VBlank.
	a.ldh(0x06, 0x80)
	a.ldh(0x07, 0x06)
	a.ldh(0x45, 0x40)
	a.ldh(0x41, 0x48)
	a.ldh(0x0F, 0x00)
	a.ldh(0xFF, 0x07)
	a.emit(0x0E, 0x00, 0xFB) // LD C,0; EI
	a.label("main")
	a.emit(0x76, 0x00)                         // HALT; NOP
	a.emit(0xF0, 0x43, 0x3C, 0xE0, 0x43)       // SCX++
	a.emit(0xF0, 0x42, 0xC6, 0x03, 0xE0, 0x42) // SCY += 3
	a.emit(0x0C, 0x79, 0xE0, 0x13)             // INC C; NR13 = C
	a.emit(0x79, 0xE0, 0x4A)                   // WY = C
	a.emit(0xF0, 0x00)                         // read the joypad
	a.emit(0x79, 0xE6, 0x3F)                   // AND 0x3F
	a.jr(0x20, "main")
	// Every 64 interrupts: retrigger the channels, change the noise.
	a.ldh(0x14, 0x87)
	a.ldh(0x19, 0xC6)
	a.ldh(0x1E, 0x85)
	a.emit(0x79, 0xE0, 0x22) // NR43 = C
	a.ldh(0x23, 0x80)
	a.emit(0x79, 0xE0, 0x41) // STAT = C
	a.jr(0x18, "main")

	rom := make([]byte, 0x8000)
	for _, v := range []int{0x40, 0x48, 0x50, 0x58, 0x60} {
		rom[v] = 0xD9 // RETI
	}
	copy(rom[0x100:], []byte{0xC3, 0x50, 0x01}) // JP 0x0150
	if model == ModelCGB {
		rom[0x143] = 0x80
	}
	copy(rom[0x150:], a.assemble())
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
