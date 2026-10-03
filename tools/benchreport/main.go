// Command benchreport compares the benchmark results of a pull request with
// those of its base branch and writes a Markdown report of the differences,
// which the CI posts on the pull request:
//
//	benchreport [-base label] [-head label] [-note text] base.txt head.txt > report.md
//
// The files hold the output of "go test -bench", several runs each. The
// statistics are those of benchstat (golang.org/x/perf): medians, 95%
// confidence intervals and a Mann-Whitney U test.
//
// The report is informative: CI runners are shared machines, so it flags
// changes but never fails.
package main

import (
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"strings"

	"golang.org/x/perf/benchfmt"
	"golang.org/x/perf/benchmath"
)

// Marker identifies the comment of the report, so that the CI updates it
// instead of adding one on every push.
const Marker = "<!-- gbe-benchmarks -->"

// Thresholds of the time changes that the report flags, when significant.
const (
	improvementThreshold = -0.05
	regressionThreshold  = 0.10
)

const timeUnit = "sec/op"

func main() {
	baseLabel := flag.String("base", "base", "label of the base results (e.g. a commit)")
	headLabel := flag.String("head", "PR", "label of the pull request results")
	note := flag.String("note", "", "how the results were obtained, added to the footer")
	flag.Parse()
	if flag.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: benchreport [-base label] [-head label] [-note text] base.txt head.txt")
		os.Exit(2)
	}
	base, err := readFile(flag.Arg(0))
	if err == nil {
		var head *results
		head, err = readFile(flag.Arg(1))
		if err == nil {
			report(os.Stdout, base, head, labels{*baseLabel, *headLabel, *note})
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "benchreport:", err)
		os.Exit(1)
	}
}

// results holds the measurements of one side, by benchmark and unit.
type results struct {
	names  []string // in the order of the file
	values map[string]map[string][]float64
	cpu    string
}

func readFile(name string) (*results, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readResults(f, name)
}

func readResults(r io.Reader, fileName string) (*results, error) {
	res := &results{values: map[string]map[string][]float64{}}
	br := benchfmt.NewReader(r, fileName)
	for br.Scan() {
		rec, ok := br.Result().(*benchfmt.Result)
		if !ok {
			continue // syntax errors: lines that are not results
		}
		name := benchName(rec)
		units := res.values[name]
		if units == nil {
			units = map[string][]float64{}
			res.values[name] = units
			res.names = append(res.names, name)
		}
		for _, v := range rec.Values {
			units[v.Unit] = append(units[v.Unit], v.Value)
		}
		if res.cpu == "" {
			res.cpu = rec.GetConfig("cpu")
		}
	}
	return res, br.Err()
}

// benchName is "pkg/Name" without the GOMAXPROCS suffix: "gb/FrameZelda".
func benchName(r *benchfmt.Result) string {
	base, parts := r.Name.Parts()
	var b strings.Builder
	if pkg := r.GetConfig("pkg"); pkg != "" {
		b.WriteString(path.Base(pkg) + "/")
	}
	b.Write(base)
	for _, p := range parts {
		if len(p) > 0 && p[0] == '-' {
			continue // -GOMAXPROCS
		}
		b.Write(p)
	}
	return b.String()
}

// change is the comparison of one benchmark in one unit.
type change struct {
	name, unit string
	base, head benchmath.Summary
	delta      float64 // head/base - 1
	p          float64
	sig        bool // p < alpha
}

func compare(name, unit string, base, head []float64) change {
	s1 := benchmath.NewSample(base, &benchmath.DefaultThresholds)
	s2 := benchmath.NewSample(head, &benchmath.DefaultThresholds)
	c := benchmath.AssumeNothing.Compare(s1, s2)
	ch := change{
		name: name,
		unit: unit,
		base: benchmath.AssumeNothing.Summary(s1, 0.95),
		head: benchmath.AssumeNothing.Summary(s2, 0.95),
		p:    c.P,
		sig:  c.P < c.Alpha,
	}
	if ch.base.Center != 0 {
		ch.delta = ch.head.Center/ch.base.Center - 1
	}
	return ch
}

// verdict is the flag of a time change.
func (c change) verdict() string {
	switch {
	case c.sig && c.delta <= improvementThreshold:
		return "🟢"
	case c.sig && c.delta >= regressionThreshold:
		return "⚠️"
	}
	return "~"
}

// labels name the two sides in the report; note says how the results were
// obtained.
type labels struct{ base, head, note string }

func report(w io.Writer, base, head *results, l labels) {
	var times, others []change
	var onlyBase, onlyHead []string
	for _, name := range head.names {
		if _, ok := base.values[name]; !ok {
			onlyHead = append(onlyHead, name)
			continue
		}
		for _, unit := range []string{timeUnit, "B/op", "allocs/op"} {
			b, h := base.values[name][unit], head.values[name][unit]
			if len(b) == 0 || len(h) == 0 {
				continue
			}
			c := compare(name, unit, b, h)
			if unit == timeUnit {
				times = append(times, c)
			} else if c.base.Center != c.head.Center {
				others = append(others, c)
			}
		}
	}
	for _, name := range base.names {
		if _, ok := head.values[name]; !ok {
			onlyBase = append(onlyBase, name)
		}
	}

	fmt.Fprintln(w, Marker)
	fmt.Fprintln(w, "## Performance report")
	fmt.Fprintln(w)
	fmt.Fprintln(w, summary(times))
	fmt.Fprintln(w)
	if len(times) > 0 {
		fmt.Fprintf(w, "| Benchmark (time/op) | %s | %s | Δ | p | |\n", l.base, l.head)
		fmt.Fprintln(w, "|---|---:|---:|---:|---:|:-:|")
		for _, c := range times {
			fmt.Fprintf(w, "| `%s` | %s | %s | %s | %s | %s |\n", c.name,
				formatValue(c.base, timeUnit), formatValue(c.head, timeUnit), formatDelta(c), formatP(c.p), c.verdict())
		}
		fmt.Fprintln(w)
	}
	if len(others) > 0 {
		fmt.Fprintln(w, "<details><summary>Memory allocations changed</summary>")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "| Benchmark | Unit | %s | %s | Δ | p |\n", l.base, l.head)
		fmt.Fprintln(w, "|---|---|---:|---:|---:|---:|")
		for _, c := range others {
			fmt.Fprintf(w, "| `%s` | %s | %s | %s | %s | %s |\n", c.name, c.unit,
				formatValue(c.base, c.unit), formatValue(c.head, c.unit), formatDelta(c), formatP(c.p))
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, "</details>")
		fmt.Fprintln(w)
	}
	if len(onlyHead) > 0 {
		fmt.Fprintf(w, "**New benchmarks** (not in %s): %s\n\n", l.base, codeList(onlyHead))
	}
	if len(onlyBase) > 0 {
		fmt.Fprintf(w, "**Removed benchmarks** (not in %s): %s\n\n", l.head, codeList(onlyBase))
	}
	runs := 0
	if len(times) > 0 {
		runs = len(head.values[times[0].name][timeUnit])
	}
	fmt.Fprintf(w, "<sub>%s against %s, %d %s each", l.head, l.base, runs, plural(runs, "run", "runs"))
	if head.cpu != "" {
		fmt.Fprintf(w, " on %s", head.cpu)
	}
	fmt.Fprint(w, ".")
	if l.note != "" {
		fmt.Fprint(w, " "+l.note)
	}
	fmt.Fprintf(w, " Medians ± 95%% confidence interval; ~ marks the differences that are not significant (p ≥ 0.05) or below the thresholds: 🟢 %.0f%% faster, ⚠️ %.0f%% slower. CI runners are shared machines, so changes of a few percent are often noise.</sub>\n",
		-improvementThreshold*100, regressionThreshold*100)
}

// summary is the first line of the report.
func summary(times []change) string {
	if len(times) == 0 {
		return "No benchmark to compare."
	}
	var better, worse int
	logSum := 0.0
	for _, c := range times {
		switch c.verdict() {
		case "🟢":
			better++
		case "⚠️":
			worse++
		}
		logSum += math.Log(c.head.Center / c.base.Center)
	}
	geomean := math.Exp(logSum/float64(len(times))) - 1
	var parts []string
	if worse > 0 {
		parts = append(parts, fmt.Sprintf("⚠️ **%d %s**", worse, plural(worse, "regression", "regressions")))
	}
	if better > 0 {
		parts = append(parts, fmt.Sprintf("🟢 **%d %s**", better, plural(better, "improvement", "improvements")))
	}
	if same := len(times) - better - worse; same > 0 {
		parts = append(parts, fmt.Sprintf("%d without significant change", same))
	}
	return fmt.Sprintf("%s. Time per operation, geometric mean: **%+.1f%%**.", strings.Join(parts, " · "), geomean*100)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func codeList(names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = "`" + n + "`"
	}
	return strings.Join(quoted, ", ")
}

func formatDelta(c change) string {
	return fmt.Sprintf("%+.1f%%", c.delta*100)
}

func formatP(p float64) string {
	if p < 0.001 {
		return "<0.001"
	}
	return fmt.Sprintf("%.3f", p)
}

// formatValue writes a median and its confidence interval, in a readable
// unit: "512.3µs ±2%".
func formatValue(s benchmath.Summary, unit string) string {
	v := s.Center
	var text string
	switch unit {
	case timeUnit:
		switch {
		case v >= 1:
			text = fmt.Sprintf("%.3gs", v)
		case v >= 1e-3:
			text = fmt.Sprintf("%.4gms", v*1e3)
		case v >= 1e-6:
			text = fmt.Sprintf("%.4gµs", v*1e6)
		default:
			text = fmt.Sprintf("%.4gns", v*1e9)
		}
	case "B/op":
		switch {
		case v >= 1<<20:
			text = fmt.Sprintf("%.4gMiB", v/(1<<20))
		case v >= 1<<10:
			text = fmt.Sprintf("%.4gKiB", v/(1<<10))
		default:
			text = fmt.Sprintf("%.4gB", v)
		}
	default:
		text = fmt.Sprintf("%.4g", v)
	}
	if v != 0 {
		spread := math.Max(s.Hi-v, v-s.Lo) / v * 100
		if spread >= 0.5 && !math.IsInf(spread, 0) { // infinite with too few runs
			text += fmt.Sprintf(" ±%.0f%%", spread)
		}
	}
	return text
}
