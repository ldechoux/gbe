package gb

import (
	"path/filepath"
	"testing"
)

// Benchmarks of the emulation, one frame per iteration. Compare runs with
// benchstat:
//
//	go test ./internal/gb -run '^$' -bench . -count 10 > new.txt
//	benchstat old.txt new.txt
func benchFrames(b *testing.B, g *GameBoy) {
	for range 300 { // past the boot and the start of the title screen
		g.RunFrame()
		g.APU.DrainSamples()
	}
	for b.Loop() {
		g.RunFrame()
		g.APU.DrainSamples()
	}
}

func BenchmarkFrameSyntheticDMG(b *testing.B) { benchFrames(b, newSyntheticGB(b, ModelDMG)) }
func BenchmarkFrameSyntheticCGB(b *testing.B) { benchFrames(b, newSyntheticGB(b, ModelCGB)) }

func BenchmarkFrameCPUInstrs(b *testing.B) {
	benchFrames(b, loadROMModel(b, filepath.Join(testROMDir, "cpu_instrs.gb"), ModelDMG))
}

func BenchmarkFrameZelda(b *testing.B) {
	benchFrames(b, loadROMModel(b, filepath.Join(gameDir, "Legend_of_Zelda_The_Links_Awakening.gb"), ModelDMG))
}

// Zelda DX runs in double speed mode all the time.
func BenchmarkFrameZeldaDX(b *testing.B) {
	benchFrames(b, loadROMModel(b, filepath.Join(gameDir, "Legend_of_Zelda_The_Links_Awakening_DX.gbc"), ModelCGB))
}

// Super Mario Land spends most of its time halted.
func BenchmarkFrameSML(b *testing.B) {
	benchFrames(b, loadROMModel(b, filepath.Join(gameDir, "Super_Mario_Land_World_Rev1.gb"), ModelDMG))
}

// busyCGB is a Game Boy Color with a busy state to snapshot.
func busyCGB(b *testing.B) *GameBoy {
	g := newSyntheticGB(b, ModelCGB)
	for range 60 {
		g.RunFrame()
	}
	return g
}

func BenchmarkSnapshot(b *testing.B) {
	g := busyCGB(b)
	var buf []byte
	b.ReportAllocs()
	for b.Loop() {
		buf = g.Snapshot(buf)
	}
}

func BenchmarkRestore(b *testing.B) {
	g := busyCGB(b)
	snap := g.Snapshot(nil)
	b.ReportAllocs()
	for b.Loop() {
		if err := g.Restore(snap); err != nil {
			b.Fatal(err)
		}
	}
}
