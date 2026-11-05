package postgres

import (
	"context"
	"log/slog"
	"time"
)

// MetricsLockKey is the advisory lock that elects the one process collecting
// the live metrics. A fixed number, the same in every process and release
// ("brevis" and a 1 in hex).
const MetricsLockKey int64 = 0x6272657669730001

// RunExclusive runs fn only while this process holds the session-level
// advisory lock `key`, and returns when ctx ends.
//
// The lock lives on a DEDICATED connection taken out of the pool, so no
// unrelated query can release it. A process that does not get it retries every
// `retry`; when the holder dies, its connection closes, Postgres drops the lock
// and the next retry takes over. The holder pings its connection on the same
// beat: if it breaks, fn's context is canceled before another process could
// have started (it lost the lock the instant the connection died).
func (p *Pool) RunExclusive(ctx context.Context, key int64, retry time.Duration, log *slog.Logger, fn func(ctx context.Context)) {
	for ctx.Err() == nil {
		p.holdOnce(ctx, key, retry, log, fn)
		select {
		case <-ctx.Done():
			return
		case <-time.After(retry):
		}
	}
}

func (p *Pool) holdOnce(ctx context.Context, key int64, retry time.Duration, log *slog.Logger, fn func(ctx context.Context)) {
	pc, err := p.Acquire(ctx)
	if err != nil {
		return
	}
	// Hijacked, then closed for good: handing a connection that still holds the
	// lock back to the pool would keep it locked for as long as the pool lives.
	conn := pc.Hijack()
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()
	var got bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&got); err != nil || !got {
		return
	}
	log.Info("this process holds the metrics collector lock")
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(runCtx)
	}()
	t := time.NewTicker(retry)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			var one int
			if err := conn.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil && ctx.Err() == nil {
				log.Warn("lost the metrics collector lock; stopping the collector", "error", err)
				cancel()
				<-done
				return
			}
		case <-ctx.Done():
			<-done
			return
		}
	}
}
