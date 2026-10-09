package main

import "testing"

func TestButtons(t *testing.T) {
	ps, err := parseButtons("right:180/240,a:12/40")
	if err != nil {
		t.Fatal(err)
	}
	s := scripted(ps)
	// gb.Buttons: up, down, left, right, a, b, start, select.
	if d := s(0); !d[3] || !d[4] {
		t.Errorf("frame 0: %v", d)
	}
	if d := s(215); d[3] || d[4] {
		t.Errorf("frame 215: %v", d)
	}
	r, err := replayed("a:10-20,right:15-100")
	if err != nil {
		t.Fatal(err)
	}
	for f, want := range map[int][8]bool{9: {}, 10: {4: true}, 15: {3: true, 4: true}, 20: {3: true}, 100: {}} {
		if got := r(f); got != want {
			t.Errorf("frame %d: %v, want %v", f, got, want)
		}
	}
	if _, err := replayed("jump:1-2"); err == nil {
		t.Error("an unknown button is accepted")
	}
}
