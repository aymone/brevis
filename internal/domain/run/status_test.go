package run

import (
	"errors"
	"testing"
	"time"
)

// The happy path of section 7: created -> queued -> running -> success.
func TestTheHappyPath(t *testing.T) {
	r := &Run{Status: StatusCreated}
	now := time.Now()

	for _, s := range []Status{StatusQueued, StatusRunning, StatusSuccess} {
		if err := r.Transition(s, now); err != nil {
			t.Fatalf("transition to %s: %v", s, err)
		}
	}
	if r.StartedAt == nil || r.FinishedAt == nil {
		t.Error("running and success must stamp the times")
	}
}

// The retry cycle: failed -> retrying -> queued, and back to running.
func TestTheRetryCycle(t *testing.T) {
	r := &Run{Status: StatusRunning}
	now := time.Now()

	for _, s := range []Status{StatusFailed, StatusRetrying, StatusQueued, StatusRunning} {
		if err := r.Transition(s, now); err != nil {
			t.Fatalf("transition to %s: %v", s, err)
		}
	}
}

// Re-queueing clears the stamps: the new attempt does not inherit the previous
// one's times, otherwise the reported duration would be another run's.
func TestRequeueingClearsTheStamps(t *testing.T) {
	now := time.Now()
	r := &Run{Status: StatusCreated}
	_ = r.Transition(StatusQueued, now)
	_ = r.Transition(StatusRunning, now)
	_ = r.Transition(StatusFailed, now)
	_ = r.Transition(StatusRetrying, now)
	_ = r.Transition(StatusQueued, now)

	if r.StartedAt != nil || r.FinishedAt != nil {
		t.Error("re-queued has to clear the previous attempt's stamps")
	}
}

// Section 7 requires an invalid transition to return an error -- not to be
// ignored.
func TestInvalidTransitionsAreRefused(t *testing.T) {
	cases := []struct{ from, to Status }{
		{StatusSuccess, StatusRunning},  // terminal does not come back
		{StatusCanceled, StatusQueued},  // terminal does not come back
		{StatusCreated, StatusRunning},  // does not skip the queue
		{StatusCreated, StatusSuccess},  // does not skip the run
		{StatusQueued, StatusSuccess},   // does not finish without running
		{StatusRunning, StatusRetrying}, // only a failure goes to retry
		{StatusFailed, StatusRunning},   // a retry goes through the queue
	}
	for _, c := range cases {
		err := Validate(c.from, c.to)
		if err == nil {
			t.Errorf("%s -> %s should have been refused", c.from, c.to)
			continue
		}
		var inv ErrInvalidTransition
		if !errors.As(err, &inv) {
			t.Errorf("%s -> %s returned %T, wanted ErrInvalidTransition", c.from, c.to, err)
		}
	}
}

// FAILED is not terminal: what decides whether there is another attempt is the
// retry policy, not the state machine.
func TestFailedIsNotTerminal(t *testing.T) {
	if StatusFailed.Terminal() {
		t.Error("failed cannot be terminal -- it goes to retrying")
	}
	if !StatusSuccess.Terminal() || !StatusCanceled.Terminal() {
		t.Error("success and canceled are terminal")
	}
}

// Canceling has to be possible from any active state -- and only from active
// ones.
func TestCanceling(t *testing.T) {
	for _, from := range []Status{StatusCreated, StatusQueued, StatusRunning, StatusRetrying} {
		if err := Validate(from, StatusCanceled); err != nil {
			t.Errorf("it should be possible to cancel from %s: %v", from, err)
		}
	}
	if err := Validate(StatusSuccess, StatusCanceled); err == nil {
		t.Error("what already succeeded is not canceled")
	}
}

func TestAnUnknownState(t *testing.T) {
	if err := Validate(Status("made-up"), StatusQueued); err == nil {
		t.Error("expected an error for an unknown state")
	}
}
