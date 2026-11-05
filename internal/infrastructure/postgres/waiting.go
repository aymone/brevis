package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Wait is what holds a queued run back when it is its concurrency limit: the
// group or the workflow, the limit, the runs in the slots and how many of the
// same key the claim takes first (#95).
type Wait struct {
	// Group is empty when the run contends only with its own workflow, under
	// `concurrency: N`.
	Group   string
	Limit   int
	Holders []Holder // claimed with the same key, oldest first
	Ahead   int      // queued with the same key, that the claim hands out first
}

// Holder is a run occupying one of the slots.
type Holder struct {
	RunID    uuid.UUID
	Workflow string
	Since    time.Time // when it was claimed
}

// WaitingOn says whether a run waits on its concurrency limit, and on what. Nil
// when it does not -- not in the queue, already claimed, not yet available, no
// limit, or next in line with a slot free -- because a page that names the
// wrong reason is worse than one that names none.
//
// IT IS THE CLAIM'S ARITHMETIC, written a second time (internal/queue's Claim
// is an UPDATE and cannot be asked). The same key, COALESCE('g:' || group,
// 'w:' || slug); in flight means claimed, not `running`; the same order,
// priority DESC, available_at, id; and a run waits when what is in flight plus
// what is ahead of it leaves no slot. A change to the claim is a change here.
func (r *RunRepo) WaitingOn(ctx context.Context, runID uuid.UUID) (*Wait, error) {
	var (
		w   Wait
		key string
	)
	err := r.pool.QueryRow(ctx, `
		WITH self AS (
			SELECT r.max_active, COALESCE(r.concurrency_group, '') AS grp,
			       COALESCE('g:' || r.concurrency_group, 'w:' || r.workflow_slug) AS lane,
			       q.id, q.priority, q.available_at
			FROM runs r
			JOIN queue_items q ON q.run_id = r.id
			WHERE r.id = $1
			  AND r.max_active > 0
			  AND q.claimed_at IS NULL
			  AND q.available_at <= now()
		)
		SELECT self.max_active, self.grp, self.lane,
		       (SELECT count(*)
		        FROM queue_items q
		        JOIN runs r ON r.id = q.run_id
		        WHERE q.claimed_at IS NULL
		          AND q.available_at <= now()
		          AND q.id <> self.id
		          AND COALESCE('g:' || r.concurrency_group, 'w:' || r.workflow_slug) = self.lane
		          AND (q.priority > self.priority
		               OR (q.priority = self.priority
		                   AND (q.available_at, q.id) < (self.available_at, self.id))))
		FROM self`, runID).Scan(&w.Limit, &w.Group, &key, &w.Ahead)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx, `
		SELECT r.id, r.workflow_slug, q.claimed_at
		FROM queue_items q
		JOIN runs r ON r.id = q.run_id
		WHERE q.claimed_at IS NOT NULL
		  AND COALESCE('g:' || r.concurrency_group, 'w:' || r.workflow_slug) = $1
		ORDER BY q.claimed_at, r.id`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var h Holder
		if err := rows.Scan(&h.RunID, &h.Workflow, &h.Since); err != nil {
			return nil, err
		}
		w.Holders = append(w.Holders, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(w.Holders)+w.Ahead+1 <= w.Limit {
		return nil, nil // a slot is free and it is next: nothing holds it
	}
	return &w, nil
}
