package i18n

import (
	"regexp"
	"slices"
	"testing"
	"unicode"
)

var verbs = regexp.MustCompile(`%[-+# 0]*\d*(?:\.\d+)?[a-zA-Z%]`)

// Every language must translate exactly the English messages, with the same
// format verbs, in ASCII (the menu font is ASCII only).
func TestLocalesMatchDefault(t *testing.T) {
	en := Get(Default)
	for _, l := range Languages() {
		if !slices.Equal(l.Keys(), en.Keys()) {
			for _, k := range en.Keys() {
				if !l.Has(k) {
					t.Errorf("%s: missing %q", l.Code, k)
				}
			}
			for _, k := range l.Keys() {
				if !en.Has(k) {
					t.Errorf("%s: unknown %q", l.Code, k)
				}
			}
		}
		for _, k := range l.Keys() {
			msg := l.msgs[k]
			if got, want := verbs.FindAllString(msg, -1), verbs.FindAllString(en.msgs[k], -1); !slices.Equal(got, want) {
				t.Errorf("%s: %q has verbs %v, want %v", l.Code, k, got, want)
			}
			for _, r := range msg + l.Name {
				if r > unicode.MaxASCII {
					t.Errorf("%s: %q is not ASCII", l.Code, k)
					break
				}
			}
		}
	}
}

func TestLanguages(t *testing.T) {
	var codes []string
	for _, l := range Languages() {
		codes = append(codes, l.Code)
	}
	if !slices.Contains(codes, "en") || !slices.Contains(codes, "fr") || !slices.IsSorted(codes) {
		t.Fatalf("Languages() = %v", codes)
	}
	if !Has("fr") || Has("xx") {
		t.Fatal("Has")
	}
}

func TestFallbacks(t *testing.T) {
	if l := Get("xx"); l.Code != Default {
		t.Fatalf("Get(xx) = %s", l.Code)
	}
	fr := Get("fr")
	if got := fr.T("menu.quit"); got != "Quitter" {
		t.Fatalf("fr menu.quit = %q", got)
	}
	if got := fr.T("menu.scale", 3); got != "Echelle   < x3 >" {
		t.Fatalf("formatted = %q", got)
	}
	// A message missing from a language comes from English.
	partial := &Locale{Code: "xx", Name: "X", msgs: map[string]string{}}
	if got := partial.T("menu.quit"); got != "Quit" {
		t.Fatalf("fallback = %q", got)
	}
	if got := fr.T("no.such.key"); got != "no.such.key" {
		t.Fatalf("unknown key = %q", got)
	}
}
