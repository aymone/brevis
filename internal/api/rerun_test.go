package api_test

import (
	"context"
	"time"

	"github.com/google/uuid"

	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// triggerSpy is the scheduler as the trigger sees it, keeping what it got.
type triggerSpy struct {
	got    map[string]string
	slot   *time.Time
	called bool
	err    error // what the scheduler answers, nil for a run
}

func (s *triggerSpy) Toggle(context.Context, string, string) (bool, error) { return false, nil }
func (s *triggerSpy) Trigger(_ context.Context, _ string, _ time.Time, params map[string]string, slot *time.Time) (uuid.UUID, error) {
	s.got, s.slot, s.called = params, slot, true
	if s.err != nil {
		return uuid.Nil, s.err
	}
	return uuid.New(), nil
}

var ()

func (s *triggerSpy) Cancel(context.Context, uuid.UUID, string) (postgres.CancelResult, error) {
	return postgres.CancelResult{}, nil
}
