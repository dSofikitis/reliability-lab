// payments-svc: the middle hop. Authorizes a charge and reserves
// inventory in the same call. This is the service the chaos mesh
// targets most often — slow upstream (the simulated bank), retries
// on inventory failures, and a hot-loop bug ready to be rolled back
// by Argo Rollouts.
package main

import (
	"context"
	"errors"
	"math/rand/v2"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	inventoryv1 "github.com/dSofikitis/reliability-lab/gen/go/inventory/v1"
	paymentsv1 "github.com/dSofikitis/reliability-lab/gen/go/payments/v1"
	"github.com/dSofikitis/reliability-lab/pkg/obs"
)

const serviceName = "payments-svc"

type server struct {
	paymentsv1.UnimplementedPaymentsServiceServer
	inv inventoryv1.InventoryServiceClient
}

func (s *server) Charge(ctx context.Context, req *paymentsv1.ChargeRequest) (*paymentsv1.ChargeResponse, error) {
	// Simulated bank-side latency. Steady-state is fast; chaos-mesh
	// injects extra delay at the network layer to break the SLO.
	time.Sleep(time.Duration(5+rand.IntN(15)) * time.Millisecond)

	resv, err := s.inv.Reserve(ctx, &inventoryv1.ReserveRequest{
		OrderId: req.GetOrderId(),
		Items:   []*inventoryv1.LineItem{{Sku: "DEFAULT", Quantity: 1}},
	})
	if err != nil || resv.GetStatus() != inventoryv1.ReserveResponse_STATUS_OK {
		return &paymentsv1.ChargeResponse{
			PaymentId:     uuid.NewString(),
			Status:        paymentsv1.ChargeResponse_STATUS_DECLINED,
			DeclineReason: "inventory unavailable",
		}, nil
	}
	return &paymentsv1.ChargeResponse{
		PaymentId: uuid.NewString(),
		Status:    paymentsv1.ChargeResponse_STATUS_AUTHORIZED,
	}, nil
}

func main() {
	log := obs.Logger(serviceName)
	health := obs.NewHealth()
	reg := obs.Registry("payments")

	grpcAddr := envOr("LISTEN_ADDR_GRPC", ":9000")
	httpAddr := envOr("LISTEN_ADDR_HTTP", ":8080")
	invAddr := envOr("INVENTORY_ADDR", "inventory-svc:9000")

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

	conn, err := grpc.NewClient(invAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		log.Error("dial inventory", "addr", invAddr, "err", err)
		os.Exit(1)
	}
	defer conn.Close()

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Error("listen", "err", err)
		os.Exit(1)
	}
	gs := grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler()))
	paymentsv1.RegisterPaymentsServiceServer(gs, &server{inv: inventoryv1.NewInventoryServiceClient(conn)})

	go func() {
		log.Info("grpc listening", "addr", grpcAddr, "inventory", invAddr)
		health.Ready()
		if err := gs.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			log.Error("grpc serve", "err", err)
			cancel()
		}
	}()

	go func() {
		if err := obs.Serve(ctx, log, httpAddr, obs.Mux(reg, health)); err != nil {
			log.Error("http serve", "err", err)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	health.NotReady()
	gs.GracefulStop()
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
