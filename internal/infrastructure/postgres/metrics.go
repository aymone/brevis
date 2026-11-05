package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"brevis.sh/brevis/internal/metrics"
)

// MetricsStore keeps the live volume of gateways and SQL APIs: written by the
// one collector, read by every API process (see internal/metrics/persist.go).
// It implements metrics.Store.
type MetricsStore struct{ pool *Pool }

func NewMetricsStore(pool *Pool) *MetricsStore { return &MetricsStore{pool: pool} }

var _ metrics.Store = (*MetricsStore)(nil)

type sourceJSON struct {
	Kind        string     `json:"kind"`
	Owner       string     `json:"owner"`
	Instance    string     `json:"instance"`
	Started     time.Time  `json:"started"`
	IntervalSec float64    `json:"interval"`
	LastTry     *time.Time `json:"last_try"`
	LastOK      *time.Time `json:"last_ok"`
	LastError   string     `json:"last_error"`
	Diffs       int        `json:"diffs"`
	Dropped     int        `json:"dropped"`
}

type minuteJSON struct {
	Kind    string             `json:"kind"`
	Owner   string             `json:"owner"`
	Entity  string             `json:"entity"`
	Minute  int64              `json:"minute"`
	Counts  []float64          `json:"counts"`
	Latency map[string]float64 `json:"latency"`
	Covered float64            `json:"covered"`
	Gauges  []float64          `json:"gauges"`
	Rate    []float64          `json:"rate"`
}

func orNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// Write is one transaction per collector round: the sources (the heartbeat),
// the minutes, and the removal of the sources that are no longer announced.
func (s *MetricsStore) Write(ctx context.Context, e metrics.Export) error {
	srcs := make([]sourceJSON, 0, len(e.Sources))
	for _, r := range e.Sources {
		srcs = append(srcs, sourceJSON{string(r.Kind), r.Owner, r.Instance, r.Started, r.IntervalSec,
			orNil(r.LastTry), orNil(r.LastOK), r.LastError, r.Diffs, r.Dropped})
	}
	mins := make([]minuteJSON, 0, len(e.Minutes))
	for _, r := range e.Minutes {
		if r.Latency == nil {
			r.Latency = map[string]float64{}
		}
		mins = append(mins, minuteJSON{string(r.Kind), r.Owner, r.Entity, r.Minute, r.Counts, r.Latency, r.Covered, r.Gauges, r.Rate})
	}
	sb, err := json.Marshal(srcs)
	if err != nil {
		return err
	}
	mb, err := json.Marshal(mins)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO metric_sources (kind, owner, instance, started_at, interval_seconds, last_try, last_ok, last_error, diffs, dropped, updated_at)
		SELECT kind, owner, instance, started, interval, last_try, last_ok, last_error, diffs, dropped, now()
		FROM jsonb_to_recordset($1::jsonb) AS x(kind text, owner text, instance text, started timestamptz, interval double precision,
		     last_try timestamptz, last_ok timestamptz, last_error text, diffs int, dropped int)
		ON CONFLICT (kind, owner, instance) DO UPDATE SET started_at = excluded.started_at,
		     interval_seconds = excluded.interval_seconds, last_try = excluded.last_try, last_ok = excluded.last_ok,
		     last_error = excluded.last_error, diffs = excluded.diffs, dropped = excluded.dropped, updated_at = now()`, sb); err != nil {
		return fmt.Errorf("writing the sources: %w", err)
	}
	// A replica that is gone (a pod scaled down) leaves with its row, so it
	// never lingers as a silent one.
	if _, err := tx.Exec(ctx, `
		DELETE FROM metric_sources s WHERE NOT EXISTS (
		  SELECT 1 FROM jsonb_to_recordset($1::jsonb) AS x(kind text, owner text, instance text)
		  WHERE x.kind = s.kind AND x.owner = s.owner AND x.instance = s.instance)`, sb); err != nil {
		return fmt.Errorf("removing the sources that left: %w", err)
	}
	if len(mins) > 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO metric_minutes (kind, owner, entity, minute, counts, latency, covered_seconds, gauges, rate, updated_at)
			SELECT kind, owner, entity, minute, counts, latency, covered, gauges, rate, now()
			FROM jsonb_to_recordset($1::jsonb) AS x(kind text, owner text, entity text, minute bigint, counts jsonb,
			     latency jsonb, covered double precision, gauges jsonb, rate jsonb)
			ON CONFLICT (kind, owner, entity, minute) DO UPDATE SET counts = excluded.counts, latency = excluded.latency,
			     covered_seconds = excluded.covered_seconds, gauges = excluded.gauges, rate = excluded.rate, updated_at = now()`, mb); err != nil {
			return fmt.Errorf("writing the minutes: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// Load reads one kind: its sources and its minutes from sinceMinute, each with
// its age by the database's clock.
func (s *MetricsStore) Load(ctx context.Context, k metrics.Kind, sinceMinute int64) (metrics.Rows, error) {
	var out metrics.Rows
	rows, err := s.pool.Query(ctx, `
		SELECT owner, instance, started_at, interval_seconds, last_try, last_ok, last_error, diffs, dropped,
		       extract(epoch FROM now() - updated_at)
		FROM metric_sources WHERE kind = $1`, string(k))
	if err != nil {
		return out, err
	}
	for rows.Next() {
		r := metrics.SourceRow{Kind: k}
		var try, ok *time.Time
		var age float64
		if err := rows.Scan(&r.Owner, &r.Instance, &r.Started, &r.IntervalSec, &try, &ok, &r.LastError, &r.Diffs, &r.Dropped, &age); err != nil {
			rows.Close()
			return out, err
		}
		if try != nil {
			r.LastTry = *try
		}
		if ok != nil {
			r.LastOK = *ok
		}
		r.Age = time.Duration(age * float64(time.Second))
		out.Sources = append(out.Sources, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	if len(out.Sources) == 0 {
		return out, nil
	}
	rows, err = s.pool.Query(ctx, `
		SELECT owner, entity, minute, counts, latency, covered_seconds, gauges, rate, extract(epoch FROM now() - updated_at)
		FROM metric_minutes WHERE kind = $1 AND minute >= $2`, string(k), sinceMinute)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		r := metrics.MinuteRow{Kind: k}
		var counts, latency, gauges, rate []byte
		var age float64
		if err := rows.Scan(&r.Owner, &r.Entity, &r.Minute, &counts, &latency, &r.Covered, &gauges, &rate, &age); err != nil {
			return out, err
		}
		if err := json.Unmarshal(counts, &r.Counts); err != nil {
			return out, err
		}
		if err := json.Unmarshal(latency, &r.Latency); err != nil {
			return out, err
		}
		if gauges != nil {
			if err := json.Unmarshal(gauges, &r.Gauges); err != nil {
				return out, err
			}
		}
		if rate != nil {
			if err := json.Unmarshal(rate, &r.Rate); err != nil {
				return out, err
			}
		}
		r.Age = time.Duration(age * float64(time.Second))
		out.Minutes = append(out.Minutes, r)
	}
	return out, rows.Err()
}

// Fingerprint moves with every collector round for the kind (the newest row
// write) and with a replica coming or going (the count): two reads that each
// walk one index, the same for every API pod.
func (s *MetricsStore) Fingerprint(ctx context.Context, k metrics.Kind) (string, error) {
	var fp string
	err := s.pool.QueryRow(ctx, `
		SELECT concat_ws('|',
		  (SELECT count(*) FROM metric_sources WHERE kind = $1),
		  (SELECT max(updated_at) FROM metric_minutes WHERE kind = $1))`, string(k)).Scan(&fp)
	return fp, err
}

// Prune deletes at most limit minutes older than beforeMinute (and, with them,
// the sources that stopped being written for as long), by ctid so one statement
// stays short whatever the backlog.
func (s *MetricsStore) Prune(ctx context.Context, beforeMinute int64, limit int) (int, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM metric_minutes WHERE ctid IN (
		  SELECT ctid FROM metric_minutes WHERE minute < $1 LIMIT $2)`, beforeMinute, limit)
	if err != nil {
		return 0, err
	}
	// A source whose collector is long gone: as old as the oldest minute kept.
	if _, err := s.pool.Exec(ctx, `DELETE FROM metric_sources WHERE updated_at < to_timestamp($1 * 60.0)`, beforeMinute); err != nil {
		return int(tag.RowsAffected()), err
	}
	return int(tag.RowsAffected()), nil
}
