package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/dSofikitis/reliability-lab/operator/internal/classifier"
	"github.com/dSofikitis/reliability-lab/operator/internal/remedy"
)

// countingRemedy is a Remedy stub that records every Apply call so
// the test can assert on dispatch behaviour without needing a real
// kube interaction.
type countingRemedy struct{ n atomic.Int32 }

func (c *countingRemedy) Apply(_ context.Context, _ remedy.Input) error {
	c.n.Add(1)
	return nil
}

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	return s
}

func newServerWithRemedy(t *testing.T, kind classifier.Kind, rem remedy.Remedy, cooldown time.Duration) *Server {
	t.Helper()
	reg := remedy.NewRegistry()
	reg.Register(kind, rem)
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()
	return New(Config{
		Client:    c,
		Log:       logr.Discard(),
		Cooldown:  cooldown,
		Namespace: "reliability-lab",
		Remedies:  reg,
	})
}

func postAlert(s *Server, p AlertManagerPayload) *httptest.ResponseRecorder {
	body, _ := json.Marshal(p)
	req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleAlert(w, req)
	return w
}

func TestHandleAlert_DispatchesFiringRemedy(t *testing.T) {
	rem := &countingRemedy{}
	s := newServerWithRemedy(t, classifier.Rollback, rem, 1*time.Second)

	w := postAlert(s, AlertManagerPayload{
		Alerts: []Alert{{
			Status:      "firing",
			Fingerprint: "fp1",
			Labels:      map[string]string{"severity": "page", "service": "orders", "burn_speed": "fast"},
		}},
	})
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	if got := rem.n.Load(); got != 1 {
		t.Fatalf("remedy applied %d times, want 1", got)
	}
}

func TestHandleAlert_SkipsResolved(t *testing.T) {
	rem := &countingRemedy{}
	s := newServerWithRemedy(t, classifier.Rollback, rem, 1*time.Second)

	postAlert(s, AlertManagerPayload{
		Alerts: []Alert{{
			Status:      "resolved",
			Fingerprint: "fp2",
			Labels:      map[string]string{"severity": "page", "service": "orders", "burn_speed": "fast"},
		}},
	})
	if got := rem.n.Load(); got != 0 {
		t.Fatalf("remedy applied %d times for resolved alert, want 0", got)
	}
}

func TestHandleAlert_DedupesWithinCooldown(t *testing.T) {
	rem := &countingRemedy{}
	s := newServerWithRemedy(t, classifier.Rollback, rem, 10*time.Minute)

	a := Alert{
		Status:      "firing",
		Fingerprint: "fp3",
		Labels:      map[string]string{"severity": "page", "service": "orders", "burn_speed": "fast"},
	}
	postAlert(s, AlertManagerPayload{Alerts: []Alert{a}})
	postAlert(s, AlertManagerPayload{Alerts: []Alert{a}})
	postAlert(s, AlertManagerPayload{Alerts: []Alert{a}})
	if got := rem.n.Load(); got != 1 {
		t.Fatalf("remedy applied %d times for duplicate fingerprint, want 1", got)
	}
}

func TestHandleAlert_NoRemedyForTicketSeverity(t *testing.T) {
	rem := &countingRemedy{}
	s := newServerWithRemedy(t, classifier.Rollback, rem, 1*time.Second)

	postAlert(s, AlertManagerPayload{
		Alerts: []Alert{{
			Status:      "firing",
			Fingerprint: "fp4",
			Labels:      map[string]string{"severity": "ticket", "service": "orders", "burn_speed": "slow"},
		}},
	})
	if got := rem.n.Load(); got != 0 {
		t.Fatalf("remedy applied %d times for ticket-severity alert, want 0", got)
	}
}
