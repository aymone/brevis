package pages

import (
	"time"

	evkind "brevis.sh/brevis/internal/domain/events"
	"brevis.sh/brevis/internal/infrastructure/postgres"
	"brevis.sh/brevis/web/i18n"
)

// FeedShown is how many events the Overview's log draws: the last hours of a
// busy installation, not its history -- that is what the run list is for.
const FeedShown = 50

// FeedLine is one event as the Overview's log draws it: when, from where (a
// source code the app names), the sentence with its subject apart -- drawn as
// code, and put where the reader's language wants it -- who did it, where it
// leads, and a run's state for its dot.
type FeedLine struct {
	At             time.Time
	Source         string
	What           i18n.Msg
	Subject, Actor string
	Href           string
	State          string
}

// FeedOf is the events as lines, in the order they came. A kind this version
// does not know -- written by a newer one -- is skipped rather than guessed.
func FeedOf(evs []postgres.Event) []FeedLine {
	out := make([]FeedLine, 0, len(evs))
	for _, e := range evs {
		source, ok := evkind.SourceOf(e.Kind)
		if !ok {
			continue
		}
		l := FeedLine{At: e.At, Source: sourceCode(source), What: eventSentence(e.Kind),
			Subject: e.Subject, Actor: e.Actor, State: runState(e.Kind)}
		switch {
		case e.RunID != nil:
			l.Href = "/runs/" + e.RunID.String()
		case source == evkind.FromWorkflow:
			l.Href = "/workflows/" + e.Subject
		}
		out = append(out, l)
	}
	return out
}

// eventSentence is what happened, as the sentence the app says around its
// subject (the {subject} the app fills). One literal key per kind: the
// catalog's tests read the keys the code asks for, and a key built at run
// time is one they cannot see.
func eventSentence(k evkind.Kind) i18n.Msg {
	switch k {
	case evkind.WorkflowPublished:
		return i18n.M("event.workflow.published")
	case evkind.WorkflowRemoved:
		return i18n.M("event.workflow.removed")
	case evkind.SchedulePaused:
		return i18n.M("event.schedule.paused")
	case evkind.ScheduleResumed:
		return i18n.M("event.schedule.resumed")
	case evkind.RunStarted:
		return i18n.M("event.run.started")
	case evkind.RunSucceeded:
		return i18n.M("event.run.succeeded")
	case evkind.RunFailed:
		return i18n.M("event.run.failed")
	case evkind.RunCanceled:
		return i18n.M("event.run.canceled")
	case evkind.StreamPublished:
		return i18n.M("event.stream.published")
	case evkind.GatewayUnpublished:
		return i18n.M("event.gateway.unpublished")
	case evkind.EndpointsPublished:
		return i18n.M("event.endpoints.published")
	case evkind.EndpointsUnpublished:
		return i18n.M("event.endpoints.unpublished")
	}
	return i18n.Msg{}
}

// sourceCode names where an event comes from: "workflow", "stream" or
// "endpoint"; the app says it as the menu does (event.source.*).
func sourceCode(s evkind.Source) string {
	switch s {
	case evkind.FromStream:
		return "stream"
	case evkind.FromEndpoint:
		return "endpoint"
	}
	return "workflow"
}

// runState is a run event's state, for its dot; "" for the rest, which have
// no state to show.
func runState(k evkind.Kind) string {
	switch k {
	case evkind.RunStarted:
		return "running"
	case evkind.RunSucceeded:
		return "success"
	case evkind.RunFailed:
		return "failed"
	case evkind.RunCanceled:
		return "canceled"
	}
	return ""
}
