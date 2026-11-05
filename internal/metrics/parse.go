// Package metrics is the console's only time series: a collector that scrapes
// the Prometheus text exposition of the gateways and of the SQL APIs the
// operator named, turns counter deltas into per-minute buckets kept in memory,
// and answers "how much went into this stream / came through this endpoint,
// and when".
//
// WHAT IT IS NOT: a store. History lives in this process only, so a restart of
// the console starts the chart again (the API says since when). Anything
// longer belongs in the operator's Prometheus, which scrapes the same
// endpoints.
package metrics

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Sample is one series at one scrape. Histograms are flattened the way the
// text format spells them (`_bucket` with an `le` label, `_sum`, `_count`), so
// the rest of the package reads one shape whatever the exposition's types.
type Sample struct {
	Name   string
	Labels map[string]string
	Value  float64
}

// ErrTooLarge is a response past the size cap. It is an error and not a
// truncated parse: half an exposition would read as counters that vanished.
var ErrTooLarge = errors.New("exposition over the size cap")

// Parse reads a Prometheus text exposition of at most max bytes.
//
// IT IS WRITTEN HERE, NOT TAKEN FROM prometheus/common, ON PURPOSE. The engine
// links no protobuf: prometheus/common's expfmt pulls the generated client_model
// (29 protobuf packages) into the binary, and .github/scripts/engine-weight.sh
// refuses that (see its comment: the engine renders the exposition itself, and
// the two prometheus modules are test-only). The text format is small and has
// not changed in a decade; the corners a hand-rolled parser gets wrong (escaped
// label values, the NaN/Inf spellings, a trailing comma, a timestamp) are the
// ones parse_test.go checks, against expfmt as the REFERENCE, on expositions
// captured from the real binaries.
//
// Only what the sources use is understood: counters, gauges, untyped and
// histograms (flat `_bucket`/`_sum`/`_count` lines). Summaries are skipped, not
// guessed at. NaN and Inf VALUES are dropped: a counter that is NaN has no
// delta, and one poisoned addition would turn a minute bucket into NaN for
// good.
func Parse(r io.Reader, max int64) ([]Sample, error) {
	lr := &io.LimitedReader{R: r, N: max + 1}
	// A line may be as long as the whole response may be: the cap is on the
	// response, and a long label is still just a line under it.
	sc := bufio.NewScanner(lr)
	sc.Buffer(make([]byte, 0, 64<<10), int(min(max, 1<<30))+1)
	var out []Sample
	summaries := map[string]bool{}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if line[0] == '#' {
			// `# TYPE name summary` is the only comment that matters.
			if f := strings.Fields(line); len(f) == 4 && f[1] == "TYPE" && f[3] == "summary" {
				summaries[f[2]] = true
			}
			continue
		}
		smp, err := parseLine(line)
		if err != nil {
			if lr.N <= 0 {
				return nil, ErrTooLarge
			}
			return nil, fmt.Errorf("unreadable exposition: %w", err)
		}
		if isSummary(smp.Name, summaries) || math.IsNaN(smp.Value) || math.IsInf(smp.Value, 0) {
			continue
		}
		out = append(out, smp)
	}
	if lr.N <= 0 {
		return nil, ErrTooLarge
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("unreadable exposition: %w", err)
	}
	return out, nil
}

func isSummary(name string, summaries map[string]bool) bool {
	if len(summaries) == 0 {
		return false
	}
	if summaries[name] {
		return true
	}
	for _, suffix := range []string{"_sum", "_count"} {
		if base, ok := strings.CutSuffix(name, suffix); ok && summaries[base] {
			return true
		}
	}
	return false
}

// parseLine reads `name{l="v",...} value [timestamp]`.
func parseLine(line string) (Sample, error) {
	i := 0
	for i < len(line) && isNameByte(line[i], i == 0) {
		i++
	}
	if i == 0 {
		return Sample{}, errors.New("a series with no name")
	}
	smp := Sample{Name: line[:i], Labels: map[string]string{}}
	if i < len(line) && line[i] == '{' {
		var err error
		if i, err = parseLabels(line, i+1, smp.Labels); err != nil {
			return Sample{}, err
		}
	}
	rest := strings.Fields(line[i:])
	if len(rest) < 1 || len(rest) > 2 {
		return Sample{}, fmt.Errorf("series %q has no value", smp.Name)
	}
	v, err := strconv.ParseFloat(rest[0], 64) // understands NaN, +Inf, -Inf
	if err != nil {
		return Sample{}, fmt.Errorf("series %q: %w", smp.Name, err)
	}
	if len(rest) == 2 { // the optional timestamp is read to be sure it is one, and ignored
		if _, err := strconv.ParseInt(rest[1], 10, 64); err != nil {
			return Sample{}, fmt.Errorf("series %q: bad timestamp", smp.Name)
		}
	}
	smp.Value = v
	return smp, nil
}

func isNameByte(c byte, first bool) bool {
	return c == '_' || c == ':' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (!first && c >= '0' && c <= '9')
}

// parseLabels reads from just after `{` through `}` and returns the index after it.
func parseLabels(line string, i int, into map[string]string) (int, error) {
	for {
		for i < len(line) && (line[i] == ' ' || line[i] == ',') {
			i++
		}
		if i >= len(line) {
			return 0, errors.New("labels never close")
		}
		if line[i] == '}' {
			return i + 1, nil
		}
		start := i
		for i < len(line) && isNameByte(line[i], i == start) {
			i++
		}
		if i == start || i+1 >= len(line) || line[i] != '=' || line[i+1] != '"' {
			return 0, errors.New("malformed label")
		}
		name := line[start:i]
		i += 2
		var val strings.Builder
		for {
			if i >= len(line) {
				return 0, errors.New("label value never closes")
			}
			c := line[i]
			if c == '"' {
				i++
				break
			}
			if c == '\\' {
				if i+1 >= len(line) {
					return 0, errors.New("dangling escape")
				}
				switch line[i+1] {
				case '\\':
					val.WriteByte('\\')
				case '"':
					val.WriteByte('"')
				case 'n':
					val.WriteByte('\n')
				default:
					return 0, errors.New("unknown escape in a label value")
				}
				i += 2
				continue
			}
			val.WriteByte(c)
			i++
		}
		if !utf8.ValidString(val.String()) {
			return 0, errors.New("label value is not UTF-8")
		}
		into[name] = val.String()
	}
}
