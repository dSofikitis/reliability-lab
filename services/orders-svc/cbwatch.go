// Circuit-break flag watcher. The remediation-operator's third remedy
// (when the email-worker's SLO burns) patches `publish_enabled: "false"`
// in the orders-svc-flags ConfigMap. That ConfigMap is mounted as a
// volume into the pod; we watch the mount directory (not the file)
// because kubelet rotates the projected ConfigMap via an atomic symlink
// swap on the parent directory — file-level fsnotify never sees those.
//
// Choosing file-watch over a client-go informer is deliberate: this
// keeps circuit-break working when the API server is unreachable, which
// is exactly the kind of incident a circuit breaker exists to handle.
// The trade-off is propagation latency bounded by kubelet's syncFrequency
// (default 60s) — acceptable for a load-shed remedy.
package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

// watchCircuitBreak blocks until ctx is cancelled, flipping `paused` to
// match the current value of `publish_enabled` in the ConfigMap mount.
// `flagFile` is the full path to the value file (e.g.
// /etc/orders-flags/publish_enabled). Returns nil cleanly on ctx cancel.
func watchCircuitBreak(ctx context.Context, log *slog.Logger, flagFile string, paused *atomic.Bool) error {
	dir := filepath.Dir(flagFile)
	read := func() {
		b, err := os.ReadFile(flagFile)
		if err != nil {
			// Missing file = treat as enabled. The operator removes the
			// key (rather than setting it false) when it lifts the brake;
			// we don't want a stale "paused" to outlive the incident.
			paused.Store(false)
			return
		}
		enabled := strings.EqualFold(strings.TrimSpace(string(b)), "true")
		prev := paused.Swap(!enabled)
		if prev != !enabled {
			log.InfoContext(ctx, "circuit-break flag changed",
				"publish_enabled", enabled, "paused", !enabled)
		}
	}
	read()

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	if err := w.Add(dir); err != nil {
		return err
	}
	// Re-read on a slow tick too: covers the case where fsnotify misses
	// an event (rare on Linux, possible if the volume backend changes).
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			read()
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			// Any event in the mount dir is worth a re-read; kubelet's
			// atomic update produces Create + Rename + Remove on the
			// inner symlinks, so don't try to be clever about which.
			_ = ev
			read()
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			log.ErrorContext(ctx, "fsnotify", "err", err)
		}
	}
}
