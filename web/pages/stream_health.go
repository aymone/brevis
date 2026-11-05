package pages

import "brevis.sh/brevis/internal/metrics"

// A stream's health, as far as the console can truthfully say it.
//
// WHAT THE ENGINE HOLDS: a stream's published CONFIGURATION (gateway_streams,
// gateway_destinations, the config history) and nothing about its traffic. The
// gateway shares no database or call with the engine (docs/GATEWAY.md): it
// writes no landings, reports no counts, and its dead letter is files or a
// bucket the console never reads. Accepted, rejected, delivered and buried
// records live on the gateway's /metrics, by decision, because a console
// number a minute old is worse than the dashboard.
//
// So there is NO honest "healthy", "degraded" or "failing" for a gateway
// stream today, and this file does not invent one: the catalog's "continuous"
// is a statement about HOW the stream writes (as events arrive), not about
// whether it is working, and landings on the same destination from workflow
// steps say how THOSE steps are doing, which the catalog itself refuses to
// read as the stream's (a gateway counts as a writer that is always
// arriving). What can be said truthfully is whether the console is able to
// know anything, and why not.

// HealthUnknown is what the console says when it cannot tell: the static
// answer below, and the live one (internal/metrics.Assess) when the gateway's
// /metrics say nothing usable.
const HealthUnknown = metrics.HealthUnknown

// The reasons a stream's health is unknown that the manifest decides. They are
// machine keys; the app writes the sentence in the reader's language.
const (
	// ReasonNotReported: the stream has no name to match the gateway's series
	// by, so nothing it reports can be attributed to it.
	ReasonNotReported = metrics.ReasonNotReported

	// ReasonDestinationUnnamed: the manifest could not name where the stream
	// lands, so there is not even a destination to look at.
	ReasonDestinationUnnamed = metrics.ReasonDestinationUnnamed
)

// StreamHealth is a stream's state, the key of its reason and the numbers that
// justify it (see internal/metrics.Health).
type StreamHealth = metrics.Health

// Health is what the console can say about the stream with no live source.
func (s Stream) Health() StreamHealth {
	if s.Sink.URI == "" {
		return StreamHealth{State: HealthUnknown, Reason: ReasonDestinationUnnamed}
	}
	return StreamHealth{State: HealthUnknown, Reason: ReasonNotReported}
}
