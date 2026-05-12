// Package obs is the small chunk of observability boilerplate every
// service in this repo reuses. It deliberately wraps very little —
// just the things every service does identically (slog setup, the
// prometheus registry, and the /healthz + /readyz + /metrics routes).
// Per-service metrics live in each service's own package.
package obs

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Logger returns a JSON slog logger tagged with the service name.
// JSON is the only format that survives kubectl logs → Loki → Grafana
// without re-parsing.
func Logger(service string) *slog.Logger {
	level := slog.LevelInfo
	if v := os.Getenv("LOG_LEVEL"); v == "debug" {
		level = slog.LevelDebug
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(h).With("service", service)
}

// Registry is a per-service prometheus registry. Returning our own
// registry (rather than the default one) means service-A's process
// metrics don't leak into service-B's scrape if they share a host.
func Registry(service string) *prometheus.Registry {
	r := prometheus.NewRegistry()
	r.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{Namespace: service}),
	)
	return r
}

// Health holds the ready/live flags every service flips during
// startup and shutdown. Liveness stays true once the process is up;
// readiness drops to false during graceful shutdown so the cluster
// stops routing new traffic while in-flight requests drain.
type Health struct {
	ready atomic.Bool
	live  atomic.Bool
}

func NewHealth() *Health {
	h := &Health{}
	h.live.Store(true)
	return h
}

func (h *Health) Ready()    { h.ready.Store(true) }
func (h *Health) NotReady() { h.ready.Store(false) }
func (h *Health) Down()     { h.live.Store(false) }

// Mux returns an HTTP mux with /healthz, /readyz, /metrics wired.
// Services that already have an HTTP server (orders-svc) can mount
// these handlers on their own router; gRPC-only services use Serve
// to expose them on their own port.
func Mux(reg *prometheus.Registry, h *Health) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		if h.live.Load() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if h.ready.Load() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ready"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	return mux
}

// Serve runs an HTTP server with graceful shutdown driven by ctx.
// Designed to be called from a goroutine; returns nil when ctx is
// canceled and the server has drained, or the underlying ListenAndServe
// error otherwise.
func Serve(ctx context.Context, log *slog.Logger, addr string, handler http.Handler) error {
	s := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", addr)
		errCh <- s.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		log.Info("http server shutting down", "addr", addr)
		return s.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
