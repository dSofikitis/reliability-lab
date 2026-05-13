// Per-alert idempotency. AlertManager's repeat_interval re-sends the
// same firing alert every few minutes until it resolves; we don't
// want to re-trigger the remedy on every re-send. The dedupe stamps
// the alert fingerprint at action time and refuses follow-up
// dispatches inside the cooldown window.
//
// In-memory only, deliberately. The cooldown window is short (default
// 10 minutes) and the operator runs single-replica, so a restart
// effectively resets dedupe — a re-fire after a restart will trigger
// a fresh remedy, which is the right behavior anyway: we want to act
// if the operator just came back and the SLO is still burning.
package server

import (
	"sync"
	"time"
)

type Dedupe struct {
	cooldown time.Duration
	mu       sync.Mutex
	seen     map[string]time.Time // fingerprint -> last action time
}

func NewDedupe(cooldown time.Duration) *Dedupe {
	return &Dedupe{cooldown: cooldown, seen: map[string]time.Time{}}
}

// Acquire returns true if the caller is allowed to act on the alert
// (and stamps the fingerprint), false if the fingerprint was acted on
// within the cooldown. Empty fingerprints are always allowed — they
// can't be deduped anyway, and refusing them would silently drop
// alerts emitted by tests or non-AM sources.
func (d *Dedupe) Acquire(fingerprint string, now time.Time) bool {
	if fingerprint == "" {
		return true
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if t, ok := d.seen[fingerprint]; ok && now.Sub(t) < d.cooldown {
		return false
	}
	d.seen[fingerprint] = now
	d.gcLocked(now)
	return true
}

// gcLocked drops fingerprints whose cooldown has elapsed, so the map
// doesn't grow unbounded over the operator's lifetime. Cheap because
// the map only ever has a handful of entries — alerts fire on the
// scale of dozens per day, not thousands per second.
func (d *Dedupe) gcLocked(now time.Time) {
	for fp, t := range d.seen {
		if now.Sub(t) >= d.cooldown {
			delete(d.seen, fp)
		}
	}
}
