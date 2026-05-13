// Webhook HTTP server. Implements controller-runtime's Runnable so the
// manager owns its lifecycle (graceful shutdown on signal, leader-
// election compatible). Handler logic lives next to this file once
// it's wired in — for now it's a tiny router with /healthz so the
// readiness probe in the Deployment has something to hit during the
// build-up commits.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dSofikitis/reliability-lab/operator/internal/classifier"
	"github.com/dSofikitis/reliability-lab/operator/internal/remedy"
)

type Config struct {
	Addr      string
	Client    client.Client
	Log       logr.Logger
	Version   string
	Cooldown  time.Duration // per-fingerprint dedupe window; default 10m if zero
	Namespace string        // namespace remedies act in; default reliability-lab
	Remedies  *remedy.Registry
}

type Server struct {
	cfg    Config
	dedupe *Dedupe
}

func New(cfg Config) *Server {
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = 10 * time.Minute
	}
	if cfg.Namespace == "" {
		cfg.Namespace = "reliability-lab"
	}
	if cfg.Remedies == nil {
		cfg.Remedies = remedy.NewRegistry()
	}
	return &Server{cfg: cfg, dedupe: NewDedupe(cfg.Cooldown)}
}

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

// handleAlert decodes the AlertManager payload, applies fingerprint
// dedupe so AlertManager's repeat_interval doesn't drive us in a
// loop, and forwards each fresh firing alert to the dispatch hook.
// Resolved alerts are logged but never trigger a remedy — the SLO
// returning to budget is the source of truth for recovery, not a
// webhook saying "all clear".
//
// Always returns 202 once the payload parses. AlertManager treats
// non-2xx as retry-worthy, and we don't want a remedy bug to translate
// into a retry storm of duplicate dispatches.
func (s *Server) handleAlert(w http.ResponseWriter, r *http.Request) {
	var p AlertManagerPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		s.cfg.Log.Error(err, "decode AlertManager payload")
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}
	now := time.Now()
	for _, a := range p.Alerts {
		log := s.cfg.Log.WithValues(
			"alertname", a.Labels["alertname"],
			"slo", a.Labels["slo"],
			"service", a.Labels["service"],
			"fingerprint", a.Fingerprint,
		)
		if !a.Firing() {
			log.V(1).Info("resolved alert (no remedy)")
			continue
		}
		if !s.dedupe.Acquire(a.Fingerprint, now) {
			log.V(1).Info("inside cooldown — skipping")
			continue
		}
		log.Info("firing alert accepted for remedy dispatch")
		s.dispatch(r.Context(), a)
	}
	w.WriteHeader(http.StatusAccepted)
}

// dispatch classifies the alert, looks up the matching remedy, and
// runs it. Errors are logged but not propagated upward — the HTTP
// handler always 202s once the payload parses, by design (see the
// retry-storm comment on handleAlert).
func (s *Server) dispatch(ctx context.Context, a Alert) {
	d := classifier.Classify(a.Labels)
	log := s.cfg.Log.WithValues(
		"kind", d.Kind.String(), "target", d.Target,
		"alertname", a.Labels["alertname"], "fingerprint", a.Fingerprint,
	)
	if d.Kind == classifier.NoRemedy {
		log.Info("no remedy for alert", "reason", d.Reason)
		return
	}
	in := remedy.Input{
		Client:    s.cfg.Client,
		Log:       log,
		Namespace: s.cfg.Namespace,
		Decision:  d,
	}
	if err := s.cfg.Remedies.Apply(ctx, in); err != nil {
		log.Error(err, "remedy apply failed")
	}
}
