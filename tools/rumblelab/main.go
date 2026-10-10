// Command rumblelab shows how the rumble detector of gbe judges a game, to
// tune it: it runs the game from its save state with scripted buttons, and
// writes a page with the notes of each channel (music or effect, and how
// hard they shake), the guessed vibration, the motor of rumble cartridges,
// the screen and the sound.
//
//	rumblelab [-frames 3600] [-buttons script] [-out dir] game.gb
//	rumblelab [-out dir] recording.json
//
// The game starts from its save state (game.state next to it) when there is
// one. A recording made with gbe -rumble-record replays a stretch of play
// instead. -params runs the detector with other settings (a JSON file of
// gb.RumbleParams). The game also runs a second time without pressing anything: the notes that
// the buttons did not change are the music and the sounds of the game
// itself, those that only the first run plays answer the player. The page
// marks them, which tells how well the detector separates music from
// effects.
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/rom"
)

// bootROMDirs are where the boot ROMs are searched (-bios).
var bootROMDirs = []string{"bios"}

const (
	sampleRate = 24000 // mono, for the page
	thumbEvery = 10    // frames between screen thumbnails
	thumbCols  = 20
)

// press holds a button for the first frames of every period.
type press struct {
	button gb.Button
	held   int
	period int
}

var buttonNames = map[string]gb.Button{
	"a": gb.ButtonA, "b": gb.ButtonB, "start": gb.ButtonStart, "select": gb.ButtonSelect,
	"up": gb.ButtonUp, "down": gb.ButtonDown, "left": gb.ButtonLeft, "right": gb.ButtonRight,
}

// parseButtons reads a script such as "right:180/240,a:12/40": right held
// for 180 frames out of 240, A for 12 out of 40.
func parseButtons(s string) ([]press, error) {
	var ps []press
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part == "" {
			continue
		}
		name, rest, ok1 := strings.Cut(part, ":")
		held, period, ok2 := strings.Cut(rest, "/")
		b, ok3 := buttonNames[strings.ToLower(name)]
		h, err1 := strconv.Atoi(held)
		p, err2 := strconv.Atoi(period)
		if !ok1 || !ok2 || !ok3 || err1 != nil || err2 != nil || p <= 0 {
			return nil, fmt.Errorf("bad button script %q", part)
		}
		ps = append(ps, press{b, h, p})
	}
	return ps, nil
}

// buttonsAt tells the buttons held on a frame (gb.Buttons order).
type buttonsAt func(frame int) [8]bool

// scripted holds the buttons of a script: each for the first frames of
// every period.
func scripted(ps []press) buttonsAt {
	return func(f int) (down [8]bool) {
		for _, p := range ps {
			for i, b := range gb.Buttons {
				if b == p.button && f%p.period < p.held {
					down[i] = true
				}
			}
		}
		return down
	}
}

// replayed holds the buttons of a recording: "a:10-25,right:0-300", each
// from the first frame to before the last.
func replayed(input string) (buttonsAt, error) {
	type span struct{ button, from, to int }
	var spans []span
	for _, part := range strings.Split(input, ",") {
		if part == "" {
			continue
		}
		name, rest, ok1 := strings.Cut(part, ":")
		from, to, ok2 := strings.Cut(rest, "-")
		b, ok3 := buttonNames[name]
		f, err1 := strconv.Atoi(from)
		t, err2 := strconv.Atoi(to)
		if !ok1 || !ok2 || !ok3 || err1 != nil || err2 != nil {
			return nil, fmt.Errorf("bad input %q", part)
		}
		for i, bb := range gb.Buttons {
			if bb == b {
				spans = append(spans, span{i, f, t})
			}
		}
	}
	return func(f int) (down [8]bool) {
		for _, s := range spans {
			if f >= s.from && f < s.to {
				down[s.button] = true
			}
		}
		return down
	}, nil
}

// recording is what gbe -rumble-record writes.
type recording struct {
	ROM    string `json:"rom"`
	State  string `json:"state"`
	Frames int    `json:"frames"`
	Input  string `json:"input"`
}

// note is a note on the page.
type note struct {
	F     float64 `json:"f"`  // frame since the start of the run
	Ch    int     `json:"ch"` // 1 to 4
	Regs  string  `json:"r"`
	PC    string  `json:"pc"`
	Music float64 `json:"m"`
	Shock float64 `json:"s"`
	Input bool    `json:"in"` // not played in the run without buttons
}

// run is what one run of the game gives.
type run struct {
	notes   []note
	level   []float64 // guessed vibration per frame
	motor   []float64 // of a rumble cartridge, per frame
	shake   []float64 // of the screen, per frame
	flashes []int     // frames the screen flashed
	samples []int16   // mono
	thumbs  []image.Image
}

func play(path, state string, frames int, buttons buttonsAt, params gb.RumbleParams, record bool) (*run, error) {
	// The boot ROM, when the state was saved while it ran, is searched for
	// as gbe does, from the working folder.
	game, err := rom.Open(path, rom.Options{Model: gb.ModelAuto, BootROMs: &rom.BootROMSearch{Defaults: bootROMDirs}, Fresh: state != ""})
	if err != nil {
		return nil, err
	}
	g := game.Console
	if state == "" {
		state = game.StatePath
	}
	if data, err := os.ReadFile(state); err == nil {
		if err := g.LoadState(data); err != nil {
			return nil, fmt.Errorf("%s: %w", state, err)
		}
	}
	g.APU.SetSampleRate(sampleRate)
	g.GuessRumble(true)
	g.SetRumbleParams(params)
	g.TraceRumble(true)
	start := float64(-1)
	r := &run{}
	for f := range frames {
		down := buttons(f)
		for i, b := range gb.Buttons {
			g.SetButton(b, down[i])
		}
		g.RunFrame()
		for _, n := range g.DrainRumbleNotes() {
			if start < 0 {
				start = n.Frame - float64(f)
			}
			r.notes = append(r.notes, note{
				F: n.Frame - start, Ch: n.Channel, PC: fmt.Sprintf("%04X", n.PC),
				Regs:  fmt.Sprintf("%02X %02X %02X %02X %02X", n.Regs[0], n.Regs[1], n.Regs[2], n.Regs[3], n.Regs[4]),
				Music: round(n.Music), Shock: round(n.Shock),
			})
		}
		r.level = append(r.level, round(g.GuessedRumble()))
		shake, flash := g.ScreenRumble()
		r.shake = append(r.shake, round(shake))
		if flash {
			r.flashes = append(r.flashes, f)
		}
		r.motor = append(r.motor, round(g.Rumble()))
		s := g.APU.DrainSamples()
		if record {
			for i := 0; i+1 < len(s); i += 2 {
				r.samples = append(r.samples, int16((int(s[i])+int(s[i+1]))/2))
			}
			if f%thumbEvery == 0 {
				r.thumbs = append(r.thumbs, screen(g))
			}
		}
	}
	return r, nil
}

func round(v float64) float64 { return math.Round(v*100) / 100 }

// screen is the frame shown, at half size.
func screen(g *gb.GameBoy) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, gb.ScreenWidth/2, gb.ScreenHeight/2))
	shades := [4]uint8{0xE0, 0xA8, 0x60, 0x18}
	for y := 0; y < gb.ScreenHeight; y += 2 {
		for x := 0; x < gb.ScreenWidth; x += 2 {
			var c color.RGBA
			i := y*gb.ScreenWidth + x
			if g.IsCGB() {
				v := g.ColorFramebuffer()[i]
				c = color.RGBA{uint8(v&31) << 3, uint8(v>>5&31) << 3, uint8(v>>10&31) << 3, 0xFF}
			} else {
				s := shades[g.Framebuffer()[i]&3]
				c = color.RGBA{s, s, s, 0xFF}
			}
			img.Set(x/2, y/2, c)
		}
	}
	return img
}

// markInput marks the notes of r that the run without buttons does not
// play, about at the same time with the same registers.
func markInput(r, quiet *run) {
	type key struct {
		ch   int
		regs string
	}
	seen := map[key][]float64{}
	for _, n := range quiet.notes {
		k := key{n.Ch, n.Regs}
		seen[k] = append(seen[k], n.F)
	}
	for i := range r.notes {
		n := &r.notes[i]
		n.Input = true
		for _, f := range seen[key{n.Ch, n.Regs}] {
			if math.Abs(f-n.F) < 2 {
				n.Input = false
				break
			}
		}
	}
}

func main() {
	frames := flag.Int("frames", 3600, "frames to run (60 s)")
	script := flag.String("buttons", "right:180/240,a:12/40,b:5/90", "buttons held: name:held/period,...")
	out := flag.String("out", "rumblelab", "folder for the page")
	paramsFile := flag.String("params", "", "settings of the detector: a JSON file of gb.RumbleParams, missing ones at their default")
	bios := flag.String("bios", "bios", "folder of the boot ROMs, for the states saved while one ran")
	quick := flag.Bool("quick", false, "only the notes and the vibration, in data.json: no page, no run without buttons")
	flag.Parse()
	bootROMDirs = []string{*bios}
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	path, state, name := flag.Arg(0), "", filepath.Base(flag.Arg(0))
	params := gb.DefaultRumbleParams()
	if *paramsFile != "" {
		data, err := os.ReadFile(*paramsFile)
		if err == nil {
			err = json.Unmarshal(data, &params)
		}
		if err != nil {
			log.Fatal(err)
		}
	}
	var buttons buttonsAt
	if strings.HasSuffix(path, ".json") {
		data, err := os.ReadFile(path)
		if err != nil {
			log.Fatal(err)
		}
		var rec recording
		if err := json.Unmarshal(data, &rec); err != nil {
			log.Fatal(err)
		}
		if buttons, err = replayed(rec.Input); err != nil {
			log.Fatal(err)
		}
		state = filepath.Join(filepath.Dir(path), rec.State)
		path, *frames = rec.ROM, rec.Frames
	} else {
		ps, err := parseButtons(*script)
		if err != nil {
			log.Fatal(err)
		}
		buttons = scripted(ps)
	}
	r, err := play(path, state, *frames, buttons, params, !*quick)
	if err != nil {
		log.Fatal(err)
	}
	if *quick {
		data, err := json.Marshal(map[string]any{"notes": r.notes, "level": r.level, "shake": r.shake, "flashes": r.flashes})
		if err == nil {
			err = os.MkdirAll(*out, 0o755)
		}
		if err == nil {
			err = os.WriteFile(filepath.Join(*out, "data.json"), data, 0o644)
		}
		if err != nil {
			log.Fatal(err)
		}
		summary(os.Stdout, r)
		return
	}
	quiet, err := play(path, state, *frames, func(int) [8]bool { return [8]bool{} }, params, false)
	if err != nil {
		log.Fatal(err)
	}
	markInput(r, quiet)
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	if err := write(*out, name, *script, r); err != nil {
		log.Fatal(err)
	}
	summary(os.Stdout, r)
}

// summary prints what the detector did.
func summary(w *os.File, r *run) {
	var effects, inputEffects, ambientShocks, inputShocks, music, inputMusic int
	for _, n := range r.notes {
		if n.Ch == 2 || n.Ch == 3 {
			continue
		}
		switch {
		case n.Music >= 0.5:
			music++
			if n.Input {
				inputMusic++
			}
		default:
			effects++
			if n.Input {
				inputEffects++
			}
		}
		if n.Shock > 0 {
			if n.Input {
				inputShocks++
			} else {
				ambientShocks++
			}
		}
	}
	shaking, motor, both := 0, 0, 0
	for i, v := range r.level {
		m := r.motor[i] > 0
		near := false
		for j := max(0, i-3); j < min(len(r.level), i+4); j++ {
			near = near || r.level[j] > 0
		}
		if v > 0 {
			shaking++
		}
		if m {
			motor++
			if near {
				both++
			}
		}
	}
	fmt.Fprintf(w, "channels 1 and 4: %d notes judged music (%d answering the buttons), %d effects (%d answering the buttons)\n",
		music, inputMusic, effects, inputEffects)
	fmt.Fprintf(w, "shocks: %d answering the buttons, %d not\n", inputShocks, ambientShocks)
	fmt.Fprintf(w, "frames shaking: %d of %d\n", shaking, len(r.level))
	screen := 0
	for _, v := range r.shake {
		if v > 0 {
			screen++
		}
	}
	fmt.Fprintf(w, "screen: %d frames shaking, %d flashes\n", screen, len(r.flashes))
	if motor > 0 {
		fmt.Fprintf(w, "motor: %d frames, %d of them with a guessed vibration within 3 frames\n", motor, both)
	}
}

// write writes the page, its sound and its thumbnails in dir.
func write(dir, name, script string, r *run) error {
	var wav bytes.Buffer
	n := len(r.samples) * 2
	wav.WriteString("RIFF")
	binary.Write(&wav, binary.LittleEndian, uint32(36+n))
	wav.WriteString("WAVEfmt ")
	for _, v := range []any{uint32(16), uint16(1), uint16(1), uint32(sampleRate), uint32(sampleRate * 2), uint16(2), uint16(16)} {
		binary.Write(&wav, binary.LittleEndian, v) // PCM, mono, 16 bits
	}
	wav.WriteString("data")
	binary.Write(&wav, binary.LittleEndian, uint32(n))
	binary.Write(&wav, binary.LittleEndian, r.samples)
	if err := os.WriteFile(filepath.Join(dir, "sound.wav"), wav.Bytes(), 0o644); err != nil {
		return err
	}
	tw, th := gb.ScreenWidth/2, gb.ScreenHeight/2
	rows := (len(r.thumbs) + thumbCols - 1) / thumbCols
	sheet := image.NewRGBA(image.Rect(0, 0, thumbCols*tw, rows*th))
	for i, t := range r.thumbs {
		x, y := i%thumbCols*tw, i/thumbCols*th
		for py := range th {
			for px := range tw {
				sheet.Set(x+px, y+py, t.At(px, py))
			}
		}
	}
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, sheet, &jpeg.Options{Quality: 80}); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "screens.jpg"), jpg.Bytes(), 0o644); err != nil {
		return err
	}
	data, err := json.Marshal(map[string]any{
		"name": name, "script": script, "notes": r.notes, "level": r.level, "motor": r.motor, "shake": r.shake, "flashes": r.flashes,
		"thumbEvery": thumbEvery, "thumbCols": thumbCols, "tw": tw, "th": th,
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "data.json"), data, 0o644); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "index.html"))
	if err != nil {
		return err
	}
	defer f.Close()
	return page.Execute(f, map[string]any{"Name": name, "Data": template.JS(data)})
}

var page = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>rumblelab: {{.Name}}</title>
<style>
body{font:13px system-ui,sans-serif;margin:16px;background:#fafafa;color:#222}
#wrap{overflow-x:auto;border:1px solid #ccc;background:#fff}
canvas{display:block}
#top{display:flex;gap:16px;align-items:flex-start}
#shot{width:320px;height:288px;image-rendering:pixelated;border:1px solid #ccc;background-repeat:no-repeat}
#info{white-space:pre;font:12px ui-monospace,monospace}
.k{display:inline-block;width:10px;height:10px;margin:0 4px 0 12px;vertical-align:middle}
</style></head><body>
<h1>{{.Name}}</h1>
<div id="top"><div id="shot"></div><div>
<audio id="audio" src="sound.wav" controls></audio>
<p><span class="k" style="background:#bbb"></span>music
<span class="k" style="background:#f0a020"></span>effect, no shock
<span class="k" style="background:#d02020"></span>shock (height: strength)
<span class="k" style="border:2px solid #2060f0;box-sizing:border-box"></span>answers the buttons
<span class="k" style="background:#2a9d3a"></span>guessed vibration
<span class="k" style="background:#7a3fc0"></span>cartridge motor
<span class="k" style="background:#e07b00"></span>screen shaking
<span class="k" style="background:#e0c000"></span>screen flash</p>
<p>Click the timeline to go there. Hover a note for its registers.</p>
<div id="info"></div></div></div>
<div id="wrap"><canvas id="c"></canvas></div>
<script>
const D = {{.Data}};
const fps = 4194304 / 70224, px = 3, lane = 34, top = 20;
const frames = D.level.length, W = frames * px, H = top + 4 * lane + 130;
const c = document.getElementById('c'), x = c.getContext('2d');
c.width = W; c.height = H;
function draw(cursor) {
  x.clearRect(0, 0, W, H);
  x.font = '11px system-ui';
  for (let s = 0; s * fps < frames; s++) { x.fillStyle = '#eee'; x.fillRect(s * fps * px, 0, 1, H); x.fillStyle = '#999'; x.fillText(s + 's', s * fps * px + 2, 12); }
  for (let ch = 1; ch <= 4; ch++) { x.fillStyle = '#666'; x.fillText('ch' + ch, 2, top + (ch - 1) * lane + 12); }
  for (const n of D.notes) {
    const y = top + (n.ch - 1) * lane, X = n.f * px;
    let col = '#bbb', h = 8;
    if (n.m < 0.5) { col = '#f0a020'; h = 14; }
    if (n.s > 0) { col = '#d02020'; h = 8 + 22 * n.s; }
    x.fillStyle = col; x.fillRect(X, y + lane - 2 - h, 2, h);
    if (n.in) { x.strokeStyle = '#2060f0'; x.strokeRect(X - 1, y + lane - 4 - h, 4, h + 3); }
  }
  const base = top + 4 * lane + 110;
  for (let f = 0; f < frames; f++) {
    if (D.motor[f] > 0) { x.fillStyle = '#7a3fc0'; x.fillRect(f * px, base - 100 - 8, px, 8 * D.motor[f]); }
    x.fillStyle = '#2a9d3a'; x.fillRect(f * px, base - 90 * D.level[f], px, 90 * D.level[f]);
    if (D.shake && D.shake[f] > 0) { x.fillStyle = '#e07b00'; x.fillRect(f * px, base + 2, px, 6 * D.shake[f] + 2); }
  }
  for (const f of D.flashes || []) { x.fillStyle = '#e0c000'; x.fillRect(f * px - 1, top - 6, 3, 4 * lane + 8); }
  x.fillStyle = '#000'; x.fillRect(cursor * px, 0, 1, H);
}
const audio = document.getElementById('audio'), shot = document.getElementById('shot'), info = document.getElementById('info');
shot.style.backgroundImage = 'url(screens.jpg)';
shot.style.backgroundSize = (D.thumbCols * D.tw * 4) + 'px auto';
function show(f) {
  draw(f);
  const i = Math.min(Math.floor(f / D.thumbEvery), Math.ceil(frames / D.thumbEvery) - 1);
  shot.style.backgroundPosition = (-(i % D.thumbCols) * D.tw * 4) + 'px ' + (-Math.floor(i / D.thumbCols) * D.th * 4) + 'px';
}
audio.addEventListener('timeupdate', () => show(audio.currentTime * fps));
(function tick() { if (!audio.paused) show(audio.currentTime * fps); requestAnimationFrame(tick); })();
c.addEventListener('click', e => { const f = e.offsetX / px; audio.currentTime = f / fps; show(f); });
c.addEventListener('mousemove', e => {
  const f = e.offsetX / px, ch = Math.floor((e.offsetY - top) / lane) + 1;
  const n = D.notes.filter(n => n.ch == ch && Math.abs(n.f - f) < 2)[0];
  info.textContent = 'frame ' + f.toFixed(1) + '  vibration ' + (D.level[Math.floor(f)] || 0) + '  motor ' + (D.motor[Math.floor(f)] || 0) +
    (n ? '\nch' + n.ch + '  ' + n.r + '  pc ' + n.pc + '  music ' + n.m + '  shock ' + n.s + (n.in ? '  (answers the buttons)' : '') : '');
});
show(0);
</script></body></html>
`))
