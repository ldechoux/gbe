// Package bench holds benchmarks of the emulation that only use the API all
// releases of gbe have: New, RunFrame, SetButton and DrainSamples. The
// Benchmarks workflow, run by hand to compare two versions, copies this
// directory and the benchmark ROM into both, so that the same benchmarks
// measure them, even versions older than the benchmarks themselves (see
// tools/benchcompare.sh).
//
// What a version cannot do is skipped rather than measured differently:
// Game Boy Color mode before v0.1.3, snapshots before v0.1.4. Those are
// detected at run time, so that this code builds with every version.
package bench

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ldechoux/gbe/internal/gb"
)

// benchROM returns the benchmark ROM: testdata/gbe-bench.gbc, where
// tools/benchcompare.sh puts the same ROM for both versions it compares, or
// that of the repository.
func benchROM(b *testing.B) []byte {
	b.Helper()
	rom, err := os.ReadFile("testdata/gbe-bench.gbc")
	if os.IsNotExist(err) {
		rom, err = os.ReadFile("../benchrom/gbe-bench.gbc")
	}
	if err != nil {
		b.Fatal(err)
	}
	return rom
}

// scenes of the benchmark ROM, with the button that selects them at power
// on (see benchrom/README.md).
var scenes = []struct {
	name   string
	button gb.Button
}{
	{"game", gb.ButtonA},
	{"cpu", gb.ButtonB},
	{"idle", gb.ButtonSelect},
	{"color", gb.ButtonStart},
	{"sound", gb.ButtonRight},
}

func newGameBoy(b *testing.B, rom []byte) *gb.GameBoy {
	b.Helper()
	cart, err := gb.NewCartridge(rom)
	if err != nil {
		b.Fatal(err)
	}
	g, err := gb.New(cart, nil)
	if err != nil {
		b.Fatal(err)
	}
	return g
}

// isCGB reports whether g runs in Game Boy Color mode; versions without it
// have no IsCGB method.
func isCGB(g *gb.GameBoy) bool {
	c, ok := any(g).(interface{ IsCGB() bool })
	return ok && c.IsCGB()
}

// frames runs g one frame per iteration, after the first 300.
func frames(b *testing.B, g *gb.GameBoy) {
	for range 300 {
		g.RunFrame()
		g.APU.DrainSamples()
	}
	for b.Loop() {
		g.RunFrame()
		g.APU.DrainSamples()
	}
}

// BenchmarkBenchROM runs each scene of the benchmark ROM on a DMG and on a
// Game Boy Color. The ROM supports both: with the Game Boy Color flag of its
// header cleared, New runs it on a DMG.
func BenchmarkBenchROM(b *testing.B) {
	rom := benchROM(b)
	dmgROM := slices.Clone(rom)
	dmgROM[0x143] = 0
	for _, s := range scenes {
		for _, model := range []string{"dmg", "cgb"} {
			b.Run(s.name+"/"+model, func(b *testing.B) {
				g := newGameBoy(b, rom)
				if model == "dmg" {
					g = newGameBoy(b, dmgROM)
				} else if !isCGB(g) {
					b.Skip("no Game Boy Color mode in this version")
				}
				g.SetButton(s.button, true) // read at power on
				for range 10 {
					g.RunFrame()
				}
				g.SetButton(s.button, false)
				frames(b, g)
			})
		}
	}
}

// BenchmarkGame runs the games kept locally in roms/, if any (they are not
// in the repository).
func BenchmarkGame(b *testing.B) {
	for _, game := range []struct {
		name, file string
		cgb        bool
	}{
		{"zelda", "Legend_of_Zelda_The_Links_Awakening.gb", false},
		{"zelda-dx", "Legend_of_Zelda_The_Links_Awakening_DX.gbc", true},
		{"sml", "Super_Mario_Land_World_Rev1.gb", false},
	} {
		b.Run(game.name, func(b *testing.B) {
			rom, err := os.ReadFile(filepath.Join("../roms", game.file))
			if err != nil {
				b.Skip(err)
			}
			g := newGameBoy(b, rom)
			if game.cgb && !isCGB(g) {
				b.Skip("no Game Boy Color mode in this version")
			}
			frames(b, g)
		})
	}
}

// snapshotter is the rewind API, from v0.1.4.
type snapshotter interface {
	Snapshot(buf []byte) []byte
	Restore(data []byte) error
}

// busyGameBoy returns the game scene of the benchmark ROM after a second
// of play, for the snapshot benchmarks.
func busyGameBoy(b *testing.B) snapshotter {
	g := newGameBoy(b, benchROM(b))
	s, ok := any(g).(snapshotter)
	if !ok {
		b.Skip("no snapshots in this version")
	}
	g.SetButton(gb.ButtonA, true)
	for range 60 {
		g.RunFrame()
	}
	return s
}

func BenchmarkSnapshot(b *testing.B) {
	g := busyGameBoy(b)
	var buf []byte
	b.ReportAllocs()
	for b.Loop() {
		buf = g.Snapshot(buf)
	}
}

func BenchmarkRestore(b *testing.B) {
	g := busyGameBoy(b)
	snap := g.Snapshot(nil)
	b.ReportAllocs()
	for b.Loop() {
		if err := g.Restore(snap); err != nil {
			b.Fatal(err)
		}
	}
}
