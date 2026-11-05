package api_test

import (
	"context"
	"time"

	"github.com/google/uuid"

	dom "brevis.sh/brevis/internal/domain/run"
	"brevis.sh/brevis/internal/infrastructure/postgres"
)

// cancelSpy is the run repository as the cancel button sees it.
type cancelSpy struct {
	id    uuid.UUID
	actor string
	err   error
}

func (*cancelSpy) Toggle(context.Context, string, string) (bool, error) { return false, nil }
func (*cancelSpy) Disparar(context.Context, string, time.Time, map[string]string, *time.Time) (uuid.UUID, error) {
	return uuid.Nil, nil
}
func (s *cancelSpy) Cancel(_ context.Context, id uuid.UUID, actor string) (postgres.CancelResult, error) {
	s.id, s.actor = id, actor
	return postgres.CancelResult{Outcome: postgres.CancelRequested, Status: dom.StatusRunning}, s.err
}
