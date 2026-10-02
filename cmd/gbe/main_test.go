package main

import (
	"testing"

	"github.com/ldechoux/gbe/internal/gb"
)

func TestParseModel(t *testing.T) {
	for name, want := range map[string]gb.Model{
		"auto": gb.ModelAuto,
		"gb":   gb.ModelDMG, "dmg": gb.ModelDMG,
		"gbc": gb.ModelCGB, "cgb": gb.ModelCGB, "GBC": gb.ModelCGB,
	} {
		if got, err := parseModel(name); err != nil || got != want {
			t.Errorf("%q: %v %v, want %v", name, got, err, want)
		}
	}
	if _, err := parseModel("gba"); err == nil {
		t.Error("unknown model accepted")
	}
}
