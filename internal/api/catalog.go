package api

import (
	"context"
	"time"

	"brevis.sh/brevis/web/pages"
)

// attentionTTL is how long the Catalog badge's count is kept: long enough
// that a page does not read the whole catalog each time, short enough that
// a destination that went stale shows within a minute.
const attentionTTL = 30 * time.Second

// attentionCount is the count behind the badge, kept for attentionTTL. ok is
// false when there is no catalog or it cannot be read (logged under path).
func (u *UI) attentionCount(ctx context.Context, path string) (n int, ok bool) {
	if u.catalog == nil {
		return 0, false
	}
	u.attentionMu.Lock()
	defer u.attentionMu.Unlock()
	if time.Since(u.attentionAt) > attentionTTL {
		entries, err := u.catalog.Catalog(ctx)
		if err != nil {
			u.log.Error("counting the destinations that need attention", "path", path, "error", err)
			return 0, false
		}
		u.attentionN = pages.BuildCatalog(entries, pages.DataFilter{}, time.Now()).Count("attention")
		u.attentionAt = time.Now()
	}
	return u.attentionN, true
}
