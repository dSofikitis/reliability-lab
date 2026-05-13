// Webhook HTTP server. Implements controller-runtime's Runnable so the
// manager owns its lifecycle (graceful shutdown on signal, leader-
// election compatible). Handler logic lives next to this file once
// it's wired in — for now it's a tiny router with /healthz so the
// readiness probe in the Deployment has something to hit during the
// build-up commits.
package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Config struct {
	Addr    string
	Client  client.Client
	Log     logr.Logger
	Version string
}

type Server struct {
	cfg Config
}

func New(cfg Config) *Server { return &Server{cfg: cfg} }

// Start blocks until ctx is cancelled and the HTTP server has drained,
// satisfying the controller-runtime manager.Runnable contract.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /alerts", s.handleAlert)

	srv := &http.Server{
		Addr:              s.cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		s.cfg.Log.Info("webhook listening", "addr", s.cfg.Addr)
		errCh <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.cfg.Log.Info("webhook shutting down")
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// handleAlert is a placeholder that 202-acks every payload so
// AlertManager doesn't retry into a black hole during the skeleton
// commit. Real classification + dispatch lands in subsequent commits.
func (s *Server) handleAlert(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusAccepted)
}
