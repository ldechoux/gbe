package ui

import (
	"log"

	"github.com/ldechoux/gbe/internal/gb"
)

const (
	rewindEvery = 4   // emulated frames between two snapshots
	rewindSlots = 150 // 10 s of play, about 200 KB each
)

// rewinder keeps the most recent snapshots of the machine in a ring, so the
// player can step back in time. The slots are reused to avoid allocating
// several times per second.
type rewinder struct {
	slots  [rewindSlots][]byte
	head   int // next slot to write
	n      int // snapshots stored
	frames int // frames since the last snapshot
}

// record is called after each emulated frame and takes a snapshot every
// rewindEvery frames, overwriting the oldest one when the ring is full.
func (r *rewinder) record(g *gb.GameBoy) {
	r.frames++
	if r.frames < rewindEvery {
		return
	}
	r.frames = 0
	r.slots[r.head] = g.Snapshot(r.slots[r.head])
	r.head = (r.head + 1) % rewindSlots
	r.n = min(r.n+1, rewindSlots)
}

// step restores the most recent snapshot and drops it. It returns false when
// there is nothing left to go back to.
func (r *rewinder) step(g *gb.GameBoy) bool {
	if r.n == 0 {
		return false
	}
	r.head = (r.head + rewindSlots - 1) % rewindSlots
	r.n--
	r.frames = 0
	if err := g.Restore(r.slots[r.head]); err != nil {
		log.Printf("rewind: %v", err)
		r.clear()
		return false
	}
	return true
}

// clear forgets the history, e.g. once another state has been loaded.
func (r *rewinder) clear() { r.n, r.frames = 0, 0 }
