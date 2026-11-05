package metrics

import "strings"

// Kind is which family of sources a target belongs to.
type Kind string

const (
	// Gateway is a brevis-gateway's /metrics: volume sent to streams.
	Gateway Kind = "gateway"
	// Endpoints is a brevis-sql-api's /metrics: consumption of endpoints.
	Endpoints Kind = "endpoints"
)

// maxTargets bounds what this process scrapes every few seconds: replicas of a
// few gateways and APIs, not a fleet.
const maxTargets = 64

// Target is one /metrics URL and the name the console knows its owner by: the
// gateway's `name:` or the API's name as the console lists it.
//
// Targets come ONLY from the Kubernetes API server's answer about the pods of
// this process's own namespace (see discover.go). Nothing a request carries can
// add one, which is what keeps the console from being aimed at an internal
// address by whoever can log in.
//
// The URL is unexported and String leaves it out: a token may ride in it, and
// a struct that prints itself is how a token reaches a log line.
type Target struct {
	Kind  Kind
	Owner string
	url   string
}

// NewTarget is for tests of other packages, which scrape an httptest server on
// loopback (an address discovery refuses). Production targets come only from
// TargetsFromPods.
func NewTarget(kind Kind, owner, url string) Target {
	return Target{Kind: kind, Owner: owner, url: url}
}

func (t Target) String() string { return string(t.Kind) + ":" + t.Owner }

func (t Target) key() string { return string(t.Kind) + "\x00" + t.Owner + "\x00" + t.url }

type class int

const (
	skip class = iota
	counter
	gauge
	bucket
)

// spec says which series of an exposition feed which measure.
type spec struct {
	counters []string // measure names, index = position
	gauges   []string
	classify func(Sample) (entity string, c class, idx int)
}

// Measure names are the JSON keys the SPA reads.
var (
	// GatewayMeasures and EndpointMeasures are exported for the API tests.
	GatewayMeasures  = []string{"received", "rejected", "dropped", "deadLetter", "delivered", "retried", "buried", "saturated", "refused"}
	EndpointMeasures = []string{"ok", "rateLimited", "clientError", "serverError", "cacheHit", "cacheMiss", "cacheOther", "hookFailed"}
)

var specs = map[Kind]*spec{
	Gateway: {
		counters: GatewayMeasures,
		gauges:   []string{"buffered", "queued"},
		classify: func(s Sample) (string, class, int) {
			ent := s.Labels["stream"]
			if ent == "" {
				return "", skip, 0
			}
			switch s.Name {
			case "brevis_gateway_events_received_total":
				return ent, counter, 0
			case "brevis_gateway_events_rejected_total":
				return ent, counter, 1
			case "brevis_gateway_events_dropped_total":
				return ent, counter, 2
			case "brevis_gateway_dead_letter_records_total":
				return ent, counter, 3
			case "brevis_gateway_batches_total":
				// Batches, not events: the gateway has no per-event delivery
				// counter unless the opt-in ingestion switch is on. Said as
				// "delivered batches" on screen rather than converted.
				switch s.Labels["outcome"] {
				case "delivered":
					return ent, counter, 4
				case "retried":
					return ent, counter, 5
				case "buried":
					return ent, counter, 6
				}
			case "brevis_gateway_saturated_total":
				return ent, counter, 7
			case "brevis_gateway_requests_refused_total":
				// Requests that never decoded into events (a body that is not
				// JSON, empty or too large): the client's fault and invisible
				// in events_rejected_total. `unauthorized` is left out (a wrong
				// key or a scan is not the stream's health) and `saturated`
				// has its own counter above.
				switch s.Labels["reason"] {
				case "malformed", "body_too_large", "empty":
					return ent, counter, 8
				}
			case "brevis_gateway_buffer_records":
				return ent, gauge, 0
			case "brevis_gateway_queue_batches":
				return ent, gauge, 1
			case "brevis_gateway_delivery_seconds_bucket":
				return ent, bucket, 0
			}
			return "", skip, 0
		},
	},
	Endpoints: {
		counters: EndpointMeasures,
		classify: func(s Sample) (string, class, int) {
			ent := s.Labels["endpoint"]
			// "-" is a request that reached no endpoint (a 401, a 404): it
			// belongs to no endpoint's screen, so it is not counted as one's.
			if ent == "" || ent == "-" {
				return "", skip, 0
			}
			switch s.Name {
			case "brevis_sql_api_requests_total":
				st := s.Labels["status"]
				switch {
				case st == "429":
					return ent, counter, 1
				case strings.HasPrefix(st, "5"):
					return ent, counter, 3
				case strings.HasPrefix(st, "4"):
					return ent, counter, 2
				default:
					return ent, counter, 0
				}
			case "brevis_sql_api_cache_requests_total":
				switch s.Labels["result"] {
				case "hit":
					return ent, counter, 4
				case "miss":
					return ent, counter, 5
				default:
					return ent, counter, 6
				}
			case "brevis_sql_api_hook_total":
				if s.Labels["outcome"] != "ok" {
					return ent, counter, 7
				}
			case "brevis_sql_api_request_seconds_bucket":
				return ent, bucket, 0
			}
			return "", skip, 0
		},
	},
}
