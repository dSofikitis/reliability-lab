// remediation-operator entrypoint.
//
// Wired around controller-runtime's manager because it gives us, for
// free, the things we'd otherwise rebuild: leader election, signal-
// driven shutdown, a shared kube client, and a Runnable interface for
// our webhook HTTP server. The operator does NOT reconcile any CRs of
// its own — its trigger is an AlertManager webhook, not a watch — so
// the manager is used purely for plumbing.
package main

import (
	"flag"
	"fmt"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/dSofikitis/reliability-lab/operator/internal/classifier"
	"github.com/dSofikitis/reliability-lab/operator/internal/remedy"
	"github.com/dSofikitis/reliability-lab/operator/internal/server"
)

func main() {
	var (
		webhookAddr     string
		metricsAddr     string
		probeAddr       string
		leaderElect     bool
		operatorVersion = "dev"
	)
	flag.StringVar(&webhookAddr, "webhook-addr", ":8088", "address the AlertManager webhook server listens on")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8090", "address the Prometheus metrics endpoint listens on")
	flag.StringVar(&probeAddr, "health-probe-addr", ":8091", "address the health/ready probes listen on")
	flag.BoolVar(&leaderElect, "leader-elect", false, "enable leader election (required if replicas > 1)")
	opts := zap.Options{Development: false}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	log := ctrl.Log.WithName("manager")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), manager.Options{
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         leaderElect,
		LeaderElectionID:       "remediation-operator.reliability-lab",
	})
	if err != nil {
		log.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		log.Error(err, "add healthz")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		log.Error(err, "add readyz")
		os.Exit(1)
	}

	registry := remedy.NewRegistry()
	registry.Register(classifier.Rollback, remedy.Rollback{})
	registry.Register(classifier.ScaleUp, remedy.ScaleUp{})
	registry.Register(classifier.CircuitBreak, remedy.CircuitBreak{})

	srv := server.New(server.Config{
		Addr:     webhookAddr,
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("webhook"),
		Version:  operatorVersion,
		Remedies: registry,
	})
	if err := mgr.Add(srv); err != nil {
		log.Error(err, "add webhook server runnable")
		os.Exit(1)
	}

	log.Info("starting manager",
		"webhook", webhookAddr, "metrics", metricsAddr, "probes", probeAddr,
		"leader-elect", leaderElect, "version", operatorVersion)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		fmt.Fprintf(os.Stderr, "manager exited: %v\n", err)
		os.Exit(1)
	}
}
