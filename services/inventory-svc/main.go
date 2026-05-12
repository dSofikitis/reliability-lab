// inventory-svc: terminal service in the order-flow chain. Reserves
// and releases SKUs against an in-memory store. The chaos experiments
// don't perturb this service from within — they target the network
// hop from payments-svc, which is what trips the retry-storm SLO.
package main

import (
	"context"
	"errors"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	inventoryv1 "github.com/dSofikitis/reliability-lab/gen/go/inventory/v1"
	"github.com/dSofikitis/reliability-lab/pkg/obs"
)

type server struct {
	inventoryv1.UnimplementedInventoryServiceServer
	mu    sync.Mutex
	store map[string][]*inventoryv1.LineItem // reservation_id -> items
}

func (s *server) Reserve(_ context.Context, req *inventoryv1.ReserveRequest) (*inventoryv1.ReserveResponse, error) {
	id := uuid.NewString()
	s.mu.Lock()
	s.store[id] = req.GetItems()
	s.mu.Unlock()
	return &inventoryv1.ReserveResponse{
		ReservationId: id,
		Status:        inventoryv1.ReserveResponse_STATUS_OK,
	}, nil
}

func (s *server) Release(_ context.Context, req *inventoryv1.ReleaseRequest) (*inventoryv1.ReleaseResponse, error) {
	s.mu.Lock()
	_, ok := s.store[req.GetReservationId()]
	delete(s.store, req.GetReservationId())
	s.mu.Unlock()
	return &inventoryv1.ReleaseResponse{Released: ok}, nil
}

func main() {
	log := obs.Logger("inventory-svc")
	health := obs.NewHealth()
	reg := obs.Registry("inventory")

	grpcAddr := envOr("LISTEN_ADDR_GRPC", ":9000")
	httpAddr := envOr("LISTEN_ADDR_HTTP", ":8080")

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Error("listen", "err", err)
		os.Exit(1)
	}
	gs := grpc.NewServer()
	inventoryv1.RegisterInventoryServiceServer(gs, &server{store: make(map[string][]*inventoryv1.LineItem)})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		log.Info("grpc listening", "addr", grpcAddr)
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
