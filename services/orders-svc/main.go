// orders-svc: the public HTTP entrypoint. Validates the order,
// dispatches to payments-svc over gRPC, and publishes the resulting
// orders.events.created event to NATS for the email-worker to consume.
// The /healthz endpoint also reflects the circuit-break flag the
// remediation-operator can flip via ConfigMap.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	paymentsv1 "github.com/dSofikitis/reliability-lab/gen/go/payments/v1"
	"github.com/dSofikitis/reliability-lab/pkg/obs"
)

type orderRequest struct {
	CustomerID     string `json:"customer_id"`
	CustomerEmail  string `json:"customer_email"`
	AmountMinor    int64  `json:"amount_minor"`
	Currency       string `json:"currency"`
	SKU            string `json:"sku"`
	Quantity       int32  `json:"quantity"`
}

type orderEvent struct {
	OrderID       string    `json:"order_id"`
	CustomerID    string    `json:"customer_id"`
	CustomerEmail string    `json:"customer_email"`
	AmountMinor   int64     `json:"amount_minor"`
	Currency      string    `json:"currency"`
	CreatedAt     time.Time `json:"created_at"`
}

type app struct {
	pay              paymentsv1.PaymentsServiceClient
	js               jetstream.JetStream
	subject          string
	publishingPaused atomic.Bool // flipped by the circuit-break ConfigMap watcher (phase 9)
}

func (a *app) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req orderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	orderID := uuid.NewString()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	resp, err := a.pay.Charge(ctx, &paymentsv1.ChargeRequest{
		OrderId:     orderID,
		CustomerId:  req.CustomerID,
		AmountMinor: req.AmountMinor,
		Currency:    req.Currency,
	})
	if err != nil {
		http.Error(w, "payments unavailable", http.StatusBadGateway)
		return
	}
	if resp.GetStatus() != paymentsv1.ChargeResponse_STATUS_AUTHORIZED {
		http.Error(w, "declined: "+resp.GetDeclineReason(), http.StatusPaymentRequired)
		return
	}

	if !a.publishingPaused.Load() {
		evt := orderEvent{
			OrderID:       orderID,
			CustomerID:    req.CustomerID,
			CustomerEmail: req.CustomerEmail,
			AmountMinor:   req.AmountMinor,
			Currency:      req.Currency,
			CreatedAt:     time.Now().UTC(),
		}
		payload, _ := json.Marshal(evt)
		if _, err := a.js.Publish(ctx, a.subject, payload); err != nil {
			// Don't fail the order if the async event publish fails —
			// the order is authorized, the email is best-effort.
			// In a production system this would queue for retry.
			_ = err
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"order_id":   orderID,
		"payment_id": resp.GetPaymentId(),
		"status":     "created",
	})
}

func main() {
	log := obs.Logger("orders-svc")
	health := obs.NewHealth()
	reg := obs.Registry("orders")

	httpAddr := envOr("LISTEN_ADDR_HTTP", ":8080")
	payAddr := envOr("PAYMENTS_ADDR", "payments-svc:9000")
	natsURL := envOr("NATS_URL", nats.DefaultURL)
	subject := envOr("NATS_SUBJECT", "orders.events.created")

	conn, err := grpc.NewClient(payAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Error("dial payments", "err", err)
		os.Exit(1)
	}
	defer conn.Close()

	nc, err := nats.Connect(natsURL, nats.MaxReconnects(-1), nats.ReconnectWait(time.Second))
	if err != nil {
		log.Error("nats connect", "err", err)
		os.Exit(1)
	}
	defer nc.Drain() //nolint:errcheck
	js, err := jetstream.New(nc)
	if err != nil {
		log.Error("jetstream", "err", err)
		os.Exit(1)
	}

	a := &app{
		pay:     paymentsv1.NewPaymentsServiceClient(conn),
		js:      js,
		subject: subject,
	}

	mux := obs.Mux(reg, health)
	mux.HandleFunc("POST /orders", a.handleCreate)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "reliability-lab orders-svc")
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	health.Ready()
	if err := obs.Serve(ctx, log, httpAddr, mux); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("http serve", "err", err)
		os.Exit(1)
	}
	log.Info("shut down clean")
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
