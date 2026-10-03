package main

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/perf/benchmath"
)

// benchOutput builds the output of "go test -bench" with runs results per
// benchmark: name -> nanoseconds per op of the first run, each following
// run 0.1% slower than the previous one (a bit of noise).
func benchOutput(runs int, ns map[string]float64, order ...string) string {
	var b strings.Builder
	b.WriteString("goos: linux\ngoarch: amd64\npkg: github.com/ldechoux/gbe/internal/gb\ncpu: Test CPU\n")
	for r := range runs {
		for _, name := range order {
			v := ns[name] * (1 + 0.001*float64(r))
			fmt.Fprintf(&b, "Benchmark%s-4\t1000\t%.1f ns/op\t64 B/op\t2 allocs/op\n", name, v)
		}
	}
	b.WriteString("PASS\nok\tgithub.com/ldechoux/gbe/internal/gb\t1.0s\n")
	return b.String()
}

func mustRead(t *testing.T, s string) *results {
	t.Helper()
	r, err := readResults(strings.NewReader(s), "test")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestReport(t *testing.T) {
	base := mustRead(t, benchOutput(10, map[string]float64{
		"Faster": 1000, "Slower": 1000, "Same": 1000, "Slight": 1000, "Gone": 1000,
	}, "Faster", "Slower", "Same", "Slight", "Gone"))
	head := mustRead(t, benchOutput(10, map[string]float64{
		"Faster": 800, "Slower": 1250, "Same": 1000, "Slight": 1070, "Added": 500,
	}, "Faster", "Slower", "Same", "Slight", "Added"))

	var out strings.Builder
	report(&out, base, head, labels{"abc1234", "def5678", "Interleaved runs."})
	got := out.String()
	t.Log("\n" + got)

	row := func(name string) string {
		for line := range strings.SplitSeq(got, "\n") {
			if strings.HasPrefix(line, "| `gb/"+name+"`") {
				return line
			}
		}
		t.Fatalf("no row for %s", name)
		return ""
	}
	for _, c := range []struct{ name, delta, verdict string }{
		{"Faster", "-20.0%", "🟢"},
		{"Slower", "+25.0%", "⚠️"},
		{"Same", "+0.0%", "~"},
		{"Slight", "+7.0%", "~"}, // significant, but under the threshold
	} {
		r := row(c.name)
		if !strings.Contains(r, c.delta) || !strings.HasSuffix(r, "| "+c.verdict+" |") {
			t.Errorf("%s: row %q, want %s and %s", c.name, r, c.delta, c.verdict)
		}
	}
	for _, want := range []string{
		Marker,
		"⚠️ **1 regression** · 🟢 **1 improvement** · 2 without significant change",
		"| Benchmark (time/op) | abc1234 | def5678 |",
		"**New benchmarks** (not in abc1234): `gb/Added`",
		"**Removed benchmarks** (not in def5678): `gb/Gone`",
		"def5678 against abc1234, 10 runs each on Test CPU. Interleaved runs.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report lacks %q", want)
		}
	}
	if strings.Contains(got, "Memory allocations") {
		t.Error("allocations reported although they did not change")
	}
}

func TestReportAllocations(t *testing.T) {
	base := mustRead(t, "pkg: x/gb\nBenchmarkSnap-4\t10\t100 ns/op\t1024 B/op\t3 allocs/op\n")
	head := mustRead(t, "pkg: x/gb\nBenchmarkSnap-4\t10\t100 ns/op\t0 B/op\t0 allocs/op\n")
	var out strings.Builder
	report(&out, base, head, labels{base: "base", head: "PR"})
	got := out.String()
	for _, want := range []string{
		"<details><summary>Memory allocations changed</summary>",
		"| `gb/Snap` | B/op | 1KiB | 0B | -100.0% |",
		"| `gb/Snap` | allocs/op | 3 | 0 | -100.0% |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report lacks %q:\n%s", want, got)
		}
	}
}

func TestReportNothingToCompare(t *testing.T) {
	var out strings.Builder
	report(&out, mustRead(t, ""), mustRead(t, ""), labels{base: "base", head: "PR"})
	if !strings.Contains(out.String(), "No benchmark to compare.") {
		t.Errorf("report:\n%s", out.String())
	}
}

func TestBenchName(t *testing.T) {
	r := mustRead(t, "pkg: github.com/ldechoux/gbe/internal/gb\nBenchmarkFrameBenchROM/game/dmg-10\t10\t100 ns/op\n")
	if got := r.names; len(got) != 1 || got[0] != "gb/FrameBenchROM/game/dmg" {
		t.Errorf("names %q", got)
	}
}

func TestFormatValue(t *testing.T) {
	for _, c := range []struct {
		v, lo, hi float64
		unit      string
		want      string
	}{
		{512.3e-6, 500e-6, 520e-6, timeUnit, "512.3µs ±2%"},
		{1.5e-3, 1.5e-3, 1.5e-3, timeUnit, "1.5ms"},
		{42e-9, 41e-9, 43e-9, timeUnit, "42ns ±2%"},
		{2048, 2048, 2048, "B/op", "2KiB"},
		{12, 12, 12, "allocs/op", "12"},
	} {
		s := benchmath.Summary{Center: c.v, Lo: c.lo, Hi: c.hi}
		if got := formatValue(s, c.unit); got != c.want {
			t.Errorf("formatValue(%g %s) = %q, want %q", c.v, c.unit, got, c.want)
		}
	}
}
