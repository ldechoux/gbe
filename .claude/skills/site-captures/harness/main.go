// Command zz_capture is the temporary entry point of the site-captures skill
// (.claude/skills/site-captures): capture.sh copies it to zz_capture/main.go
// and removes it afterwards.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/ldechoux/gbe/internal/gb"
	"github.com/ldechoux/gbe/internal/ui"
)

func main() {
	romPath := flag.String("rom", "", "ROM to run")
	model := flag.String("model", "gb", "gb or gbc")
	var opts ui.CaptureOptions
	flag.StringVar(&opts.Dir, "out", "", "output directory")
	flag.IntVar(&opts.TitleAt, "title-at", 200, "frame at which the title screen shows")
	flag.IntVar(&opts.StartAt, "start-at", 0, "frame at which Start is pressed, 0 for never")
	flag.IntVar(&opts.Scale, "scale", 8, "window scale")
	flag.StringVar(&opts.Filter, "filter", "lcd", "display filter")
	flag.StringVar(&opts.Language, "lang", "fr", "user interface language")
	flag.Parse()

	rom, err := os.ReadFile(*romPath)
	if err != nil {
		log.Fatal(err)
	}
	cart, err := gb.NewCartridge(rom)
	if err != nil {
		log.Fatal(err)
	}
	m := gb.ModelDMG
	if *model == "gbc" {
		m = gb.ModelCGB
	}
	console, err := gb.NewModel(cart, nil, m)
	if err != nil {
		log.Fatal(err)
	}
	if err := ui.Capture(console, opts); err != nil {
		log.Fatal(err)
	}
}
