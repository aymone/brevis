package postgres

import (
	"context"
	"fmt"
	"time"

	"brevis.sh/brevis/internal/notify"
)

// Insights aggregates a window for the periodic report.
//
// It is a QUERY and not a rollup table, which is a departure from the plan
// worth stating. A rollup written as each run finishes would make this read
// cheap and would also add a write to the hottest path in the system, plus a
// second source of truth for numbers that already exist and can therefore
// drift from them. This runs once a week, off-peak, over an indexed window.
//
// When it becomes slow, the rollup is the answer and the measurement is what
// says so. The duration is logged by the caller for exactly that reason.
func (r *ReadRepo) Insights(ctx context.Context, from, to time.Time, env string) (notify.Report, error) {
	rep := notify.Report{From: from, To: to, Environment: env}

	// The runs, their outcomes and their durations.
	//
	// The duration is measured from CREATED_AT, not from started_at: "how long
	// did my pipeline take" includes the wait in the queue, and a number that
	// excludes queue time looks healthy exactly when the queue is the problem.
	//
	// finished_at IS NOT NULL is what keeps a run still going out of the
	// averages. Counting it as zero would drag every one of them down at the
	// moment something is stuck.
	rows, err := r.pool.Query(ctx, `
		SELECT
			workflow_slug,
			count(*)                                          AS runs,
			count(*) FILTER (WHERE status = 'failed')         AS failed,
			COALESCE(min(EXTRACT(EPOCH FROM finished_at - created_at))
			         FILTER (WHERE finished_at IS NOT NULL), 0) AS min_s,
			COALESCE(avg(EXTRACT(EPOCH FROM finished_at - created_at))
			         FILTER (WHERE finished_at IS NOT NULL), 0) AS avg_s,
			COALESCE(max(EXTRACT(EPOCH FROM finished_at - created_at))
			         FILTER (WHERE finished_at IS NOT NULL), 0) AS max_s
		FROM runs
		WHERE created_at >= $1 AND created_at < $2
		GROUP BY workflow_slug`, from, to)
	if err != nil {
		return rep, fmt.Errorf("aggregating the runs: %w", err)
	}
	defer rows.Close()

	byWorkflow := map[string]*notify.WorkflowInsight{}
	for rows.Next() {
		var w notify.WorkflowInsight
		var minS, avgS, maxS float64
		if err := rows.Scan(&w.Slug, &w.Runs, &w.Failed, &minS, &avgS, &maxS); err != nil {
			return rep, err
		}
		w.Min, w.Avg, w.Max = seconds(minS), seconds(avgS), seconds(maxS)
		byWorkflow[w.Slug] = &w

		rep.Runs += w.Runs
		rep.Failed += w.Failed
	}
	if err := rows.Err(); err != nil {
		return rep, err
	}
	rep.Succeeded = rep.Runs - rep.Failed

	if err := r.volume(ctx, from, to, byWorkflow); err != nil {
		// The volume is a bonus. A report without rows is still a report; a
		// report that is not sent because one aggregate failed is a week of
		// silence over the least important half of the message.
		return rep, err
	}

	for _, w := range byWorkflow {
		rep.Workflows = append(rep.Workflows, *w)
	}
	return rep, nil
}

// volume sums the rows and bytes the SDK announced.
//
// Only the SUCCESSFUL attempts. Counting a failed attempt's rows reports work
// that was rolled back, and counting every attempt of a retried step reports
// the same rows twice.
//
// ::numeric::bigint and not ::bigint: JSON has one number type, so the SDK's
// counters can arrive as 48213 or as 48213.0 depending on what serialized them,
// and the direct cast refuses the second.
func (r *ReadRepo) volume(ctx context.Context, from, to time.Time,
	into map[string]*notify.WorkflowInsight,
) error {
	rows, err := r.pool.Query(ctx, `
		SELECT
			r.workflow_slug,
			COALESCE(sum((COALESCE(e->'numbers', e->'numeros')->>'rows')::numeric), 0)::bigint  AS rows,
			COALESCE(sum((COALESCE(e->'numbers', e->'numeros')->>'bytes')::numeric), 0)::bigint AS bytes
		FROM runs r
		JOIN task_runs t ON t.run_id = r.id AND t.status = 'success'
		CROSS JOIN LATERAL jsonb_array_elements(t.stages) AS e
		WHERE r.created_at >= $1 AND r.created_at < $2
		GROUP BY r.workflow_slug`, from, to)
	if err != nil {
		return fmt.Errorf("aggregating the volume: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var slug string
		var rowCount, bytes int64
		if err := rows.Scan(&slug, &rowCount, &bytes); err != nil {
			return err
		}
		if w, ok := into[slug]; ok {
			w.Rows, w.Bytes = rowCount, bytes
		}
	}
	return rows.Err()
}

func seconds(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}
