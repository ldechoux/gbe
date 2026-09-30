package ui

import (
	"fmt"
	"time"

	"github.com/ldechoux/gbe/internal/i18n"
)

// fpsRefreshInterval is how often the frame rate is measured and the window
// title updated.
const fpsRefreshInterval = 250 * time.Millisecond

// fpsCounter measures emulated frames per second over fixed intervals.
type fpsCounter struct {
	frames int
	since  time.Time // start of the current interval
	fps    float64   // rate measured over the last complete interval
}

// frame records one emulated frame.
func (c *fpsCounter) frame() { c.frames++ }

// update closes the current interval once fpsRefreshInterval has elapsed and
// reports whether a new measure is available.
func (c *fpsCounter) update(now time.Time) bool {
	if c.since.IsZero() {
		c.since = now
		return false
	}
	elapsed := now.Sub(c.since)
	if elapsed < fpsRefreshInterval {
		return false
	}
	c.fps = float64(c.frames) / elapsed.Seconds()
	c.frames, c.since = 0, now
	return true
}

// windowTitle builds "gbe - <rom title> - <fps>". While paused no frame is
// emulated, so the title says so (in language l) instead of showing 0 FPS.
func windowTitle(l *i18n.Locale, romTitle string, fps float64, paused bool) string {
	t := "gbe"
	if romTitle != "" {
		t += " - " + romTitle
	}
	if paused {
		return t + " - " + l.T("title.paused")
	}
	return fmt.Sprintf("%s - %.0f FPS", t, fps)
}
