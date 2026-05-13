// email-worker: pull-consumes orders.events.created from NATS
// JetStream and pretends to send a confirmation email. The chaos
// experiment chaos/email-worker-oom.yaml slows this consumer with a
// CPU stressor while orders-svc keeps publishing; the backlog grows,
// memory pressure mounts, the OOM-killer fires, and the
// email_delivery_within_60s SLO breaks. The operator's circuit-break
// remedy responds by flipping orders-svc's publishing flag.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/dSofikitis/reliability-lab/pkg/obs"
)

const serviceName = "email-worker"

const (
	streamName   = "ORDER_EVENTS"
	consumerName = "email-worker"
)

type incoming struct {
	OrderID       string `json:"order_id"`
	CustomerEmail string `json:"customer_email"`
}

func main() {
	log := obs.Logger(serviceName)
	health := obs.NewHealth()
	reg := obs.Registry("email_worker")

	httpAddr := envOr("LISTEN_ADDR_HTTP", ":8080")
	natsURL := envOr("NATS_URL", nats.DefaultURL)
	subject := envOr("NATS_SUBJECT", "orders.events.created")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	shutdownTrace, err := obs.InitTracing(ctx, serviceName, envOr("APP_VERSION", "dev"))
	if err != nil {
		log.Error("init tracing", "err", err)
		os.Exit(1)
	}
	defer func() {
		sctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = shutdownTrace(sctx)
	}()

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

	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      streamName,
		Subjects:  []string{"orders.events.>"},
		Retention: jetstream.LimitsPolicy,
		MaxAge:    24 * time.Hour,
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		log.Error("create stream", "err", err)
		os.Exit(1)
	}

	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          consumerName,
		Durable:       consumerName,
		FilterSubject: subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       30 * time.Second,
		MaxAckPending: 100,
	})
	if err != nil {
		log.Error("create consumer", "err", err)
		os.Exit(1)
	}

	go func() {
		if err := obs.Serve(ctx, log, httpAddr, obs.Mux(reg, health)); err != nil {
			log.Error("http serve", "err", err)
		}
	}()

	health.Ready()
	log.Info("consuming", "subject", subject)
	consumeCtx, err := cons.Consume(handler(log))
	if err != nil {
		log.Error("consume start", "err", err)
		os.Exit(1)
	}
	defer consumeCtx.Stop()

	<-ctx.Done()
	log.Info("shutting down")
	health.NotReady()
}

func handler(log *slog.Logger) jetstream.MessageHandler {
	tracer := otel.Tracer("github.com/dSofikitis/reliability-lab/services/email-worker")
	return func(msg jetstream.Msg) {
		// Stitch into the producer's trace by extracting the W3C
		// traceparent the orders-svc publisher injected on send.
		hdr := propagation.HeaderCarrier(msg.Headers())
		ctx := otel.GetTextMapPropagator().Extract(context.Background(), hdr)
		ctx, span := tracer.Start(ctx, "email-worker.process",
			trace.WithSpanKind(trace.SpanKindConsumer),
			trace.WithAttributes(attribute.String("messaging.system", "nats")),
		)
		defer span.End()

		var evt incoming
		if err := json.Unmarshal(msg.Data(), &evt); err != nil {
			log.ErrorContext(ctx, "decode", "err", err)
			span.RecordError(err)
			_ = msg.Term()
			return
		}
		span.SetAttributes(attribute.String("order.id", evt.OrderID))
		// Simulated email send. Chaos slows the worker so the queue
		// backs up; nothing here needs to "know" about that — the
		// pressure is on the consumer rate, not the handler.
		time.Sleep(100 * time.Millisecond)
		if err := msg.Ack(); err != nil && !errors.Is(err, nats.ErrConnectionClosed) {
			log.ErrorContext(ctx, "ack", "err", err)
			span.RecordError(err)
		}
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
