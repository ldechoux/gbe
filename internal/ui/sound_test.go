package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ldechoux/gbe/internal/audiofx"
)

// soundLabel is the text of the sound page entry i.
func soundLabel(g *Game, i int) string {
	m := menu{open: true, page: pageSound}
	_, items, _ := m.lines(g)
	return items[i]
}

func TestSoundPage(t *testing.T) {
	g := newTestGame(t, newFakePads())
	g.fx = audiofx.New(sampleRate, g.cfg.Stereo, g.cfg.AudioFilter)
	m := &g.menu
	do := func(a menuActions) {
		g.actions = a
		m.update(g)
	}

	m.show()
	m.cursor = itemSound
	do(menuActions{ok: true})
	if m.page != pageSound || m.cursor != soundVolume {
		t.Fatalf("Sound...: %+v, want the sound page on Volume", *m)
	}
	do(menuActions{ok: true})
	if g.cfg.Volume != 0.8 {
		t.Errorf("OK on Volume changed it to %v", g.cfg.Volume)
	}
	do(menuActions{left: true})
	if g.cfg.Volume != 0.7 || soundLabel(g, soundVolume) != "Volume    < 70% >" {
		t.Errorf("Left on Volume: %v, entry %q", g.cfg.Volume, soundLabel(g, soundVolume))
	}

	do(menuActions{down: true})
	for _, want := range []string{audiofx.Headphones, audiofx.Mono, audiofx.Stereo} {
		do(menuActions{right: true})
		if cfg, _ := LoadConfig(g.cfgPath); cfg.Stereo != want || g.cfg.Stereo != want {
			t.Errorf("Right on Output: %q, saved %q, want %q", g.cfg.Stereo, cfg.Stereo, want)
		}
	}
	do(menuActions{left: true})
	if g.cfg.Stereo != audiofx.Mono || soundLabel(g, soundStereo) != "Output    < mono >" {
		t.Errorf("Left on Output stereo: %q, entry %q", g.cfg.Stereo, soundLabel(g, soundStereo))
	}
	g.cfg.Stereo = audiofx.Headphones
	if _, _, footer := m.lines(g); footer != "Each ear hears a little of the other side" {
		t.Errorf("Output footer %q", footer)
	}

	do(menuActions{down: true})
	do(menuActions{ok: true}) // OK cycles like Right
	if g.cfg.AudioFilter != audiofx.Soft || soundLabel(g, soundFilter) != "Filter    < soft >" {
		t.Errorf("OK on Filter: %q, entry %q", g.cfg.AudioFilter, soundLabel(g, soundFilter))
	}
	do(menuActions{left: true})
	do(menuActions{left: true})
	if g.cfg.AudioFilter != audiofx.Speaker {
		t.Fatalf("Left twice from soft: %q, want speaker", g.cfg.AudioFilter)
	}
	// The speaker is mono: Output is greyed out, shown as mono, and skipped.
	if v := m.view(g); !v.disabled[soundStereo] || soundLabel(g, soundStereo) != "Output    < mono >" {
		t.Errorf("Output with the speaker: disabled %v, entry %q", v.disabled, soundLabel(g, soundStereo))
	}
	do(menuActions{up: true})
	if m.cursor != soundVolume {
		t.Errorf("Up from Filter with the speaker: cursor %d, want Volume (Output skipped)", m.cursor)
	}
	m.cursor = soundStereo
	do(menuActions{right: true})
	if g.cfg.Stereo != audiofx.Headphones {
		t.Errorf("Right on a greyed-out Output changed it to %q", g.cfg.Stereo)
	}
	// The chain follows the settings: the speaker sums both sides.
	s := []int16{1000, -1000}
	g.fx.Process(s)
	if s[0] != s[1] {
		t.Errorf("speaker filter not mono: %v", s)
	}

	m.cursor = soundBack
	do(menuActions{ok: true})
	if m.page != pageMain || m.cursor != itemSound {
		t.Errorf("Back: %+v, want the main page on Sound", *m)
	}
	m.page, m.cursor = pageSound, soundFilter
	do(menuActions{back: true})
	if m.page != pageMain || m.cursor != itemSound {
		t.Errorf("Esc on the sound page: %+v", *m)
	}
}

func TestSoundConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"stereo": "Headphones", "audio_filter": "warm"}`), 0o644)
	cfg, err := LoadConfig(path)
	if err != nil || cfg.Stereo != audiofx.Headphones || cfg.AudioFilter != audiofx.Warm {
		t.Errorf("loaded stereo %q, filter %q (%v)", cfg.Stereo, cfg.AudioFilter, err)
	}
	os.WriteFile(path, []byte(`{"stereo": "surround", "audio_filter": "loud"}`), 0o644)
	if cfg, _ := LoadConfig(path); cfg.Stereo != audiofx.Stereo || cfg.AudioFilter != audiofx.Off {
		t.Errorf("unknown values kept: stereo %q, filter %q", cfg.Stereo, cfg.AudioFilter)
	}
}

// The panel of the sound page keeps its size whatever is selected and set,
// at every scale: it reserves room for the longest value and explanation.
func TestSoundPageSize(t *testing.T) {
	g := newTestGame(t, newFakePads())
	for _, lang := range []string{"en", "fr"} {
		g.cfg.Language = lang
		for scale := 1; scale <= maxScale; scale++ {
			sw, sh := float64(160*scale), float64(144*scale)
			var first menuLayout
			for i, stereo := range audiofx.StereoModes {
				for j, filter := range audiofx.Filters {
					for cursor := range soundItems {
						g.cfg.Stereo, g.cfg.AudioFilter = stereo, filter
						m := menu{open: true, page: pageSound, cursor: cursor}
						l := layoutMenu(m.view(g), 0, sw, sh)
						if i == 0 && j == 0 && cursor == 0 {
							first = l
						} else if l.w != first.w || l.h != first.h {
							t.Fatalf("%s x%d, %s, %s, entry %d: panel %vx%v, want %vx%v",
								lang, scale, stereo, filter, cursor, l.w, l.h, first.w, first.h)
						}
					}
				}
			}
		}
	}
}
