// Package events tells the browser WHEN a screen may have changed, so it can
// refetch, instead of every tab polling the REST API on its own timer.
//
// The cost model is the point of the design: one detector goroutine per ACTIVE
// topic, shared by every client watching it, each running one cheap
// fingerprint query. A hundred tabs on the Runs screen cost one query per
// second, not a hundred.
package events

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// MaxTopics bounds one connection's subscriptions: each is a detector
// (a database query per tick), and a client has no reason to want more.
const MaxTopics = 20

// MaxDynamicTopics bounds the topics a client NAMES (run:<id>, workflow:<slug>)
// per connection. A screen watches one run or one workflow, a few at most; the
// fixed topics are shared by everyone, a named one is a detector of its own.
const MaxDynamicTopics = 4

// Fixed topics. The parameterized ones are `run:<uuid>` and `workflow:<slug>`.
const (
	Runs      = "runs"
	Workflows = "workflows"
	Catalog   = "catalog"
	Ingest    = "ingest"
	Overview  = "overview"
	Nav       = "nav"

	// The live volume screens. Fixed (one detector each, shared) but NOT in
	// the six every connection carries: the SPA subscribes only while a
	// screen that draws them is mounted. Their fingerprint is the collector's
	// scrape counter, so one change event is at most one scrape.
	MetricsStreams   = "metrics:streams"
	MetricsEndpoints = "metrics:endpoints"
)

var (
	fixed = map[string]bool{Runs: true, Workflows: true, Catalog: true, Ingest: true, Overview: true, Nav: true,
		MetricsStreams: true, MetricsEndpoints: true}
	// A slug is what the rest of the console accepts in a path: it ends up as
	// a bound query parameter, so this is about refusing junk early (and
	// bounding the key), not about SQL safety.
	slugRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
)

// fixedTopic reports whether name is one of the shared topics.
func fixedTopic(name string) bool { return fixed[name] }

// ErrTopics is the 400 for a topic list that cannot be served.
var ErrTopics = errors.New("invalid topics")

// ParseTopics splits "a,b,c", drops duplicates (keeping first-seen order) and
// refuses anything outside the contract, so that an unbounded or malformed
// name never becomes a goroutine.
func ParseTopics(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("%w: no topics given", ErrTopics)
	}
	seen := map[string]bool{}
	var out []string
	for _, t := range strings.Split(raw, ",") {
		if err := validTopic(t); err != nil {
			return nil, err
		}
		// One run is one slot however the id is spelled (upper case, braces,
		// no dashes): the canonical form is what a detector is keyed by.
		if strings.HasPrefix(t, "run:") {
			if u, err := uuid.Parse(t[4:]); err == nil {
				t = "run:" + u.String()
			}
		}
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	if len(out) > MaxTopics {
		return nil, fmt.Errorf("%w: at most %d topics", ErrTopics, MaxTopics)
	}
	dynamic := 0
	for _, t := range out {
		if !fixed[t] {
			dynamic++
		}
	}
	if dynamic > MaxDynamicTopics {
		return nil, fmt.Errorf("%w: at most %d run or workflow topics", ErrTopics, MaxDynamicTopics)
	}
	return out, nil
}

func validTopic(t string) error {
	switch {
	case fixed[t]:
		return nil
	case strings.HasPrefix(t, "run:"):
		if _, err := uuid.Parse(t[4:]); err == nil {
			return nil
		}
	case strings.HasPrefix(t, "workflow:"):
		if slugRe.MatchString(t[9:]) {
			return nil
		}
	}
	return fmt.Errorf("%w: unknown topic %q", ErrTopics, truncate(t, 40))
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
