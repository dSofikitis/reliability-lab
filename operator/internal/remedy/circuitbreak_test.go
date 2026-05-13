package remedy

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/dSofikitis/reliability-lab/operator/internal/classifier"
)

func TestCircuitBreak_PatchesExisting(t *testing.T) {
	s := newScheme(t)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "orders-svc-flags", Namespace: "reliability-lab"},
		Data:       map[string]string{"publish_enabled": "true"},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(cm).Build()

	in := Input{
		Client: c, Log: logr.Discard(), Namespace: "reliability-lab",
		Decision: classifier.Decision{Kind: classifier.CircuitBreak, Target: "orders-svc-flags"},
	}
	if err := (CircuitBreak{}).Apply(context.Background(), in); err != nil {
		t.Fatalf("apply: %v", err)
	}

	got := &corev1.ConfigMap{}
	_ = c.Get(context.Background(), types.NamespacedName{Namespace: "reliability-lab", Name: "orders-svc-flags"}, got)
	if got.Data["publish_enabled"] != "false" {
		t.Fatalf("publish_enabled = %q, want false", got.Data["publish_enabled"])
	}
}

func TestCircuitBreak_CreatesIfMissing(t *testing.T) {
	s := newScheme(t)
	c := fake.NewClientBuilder().WithScheme(s).Build()

	in := Input{
		Client: c, Log: logr.Discard(), Namespace: "reliability-lab",
		Decision: classifier.Decision{Kind: classifier.CircuitBreak, Target: "orders-svc-flags"},
	}
	if err := (CircuitBreak{}).Apply(context.Background(), in); err != nil {
		t.Fatalf("apply: %v", err)
	}

	got := &corev1.ConfigMap{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "reliability-lab", Name: "orders-svc-flags"}, got); err != nil {
		t.Fatalf("expected configmap to be created: %v", err)
	}
	if got.Data["publish_enabled"] != "false" {
		t.Fatalf("publish_enabled = %q, want false", got.Data["publish_enabled"])
	}
}

func TestCircuitBreak_AlreadyEngagedNoOp(t *testing.T) {
	s := newScheme(t)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "orders-svc-flags", Namespace: "reliability-lab",
			ResourceVersion: "42",
		},
		Data: map[string]string{"publish_enabled": "false"},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(cm).Build()

	in := Input{
		Client: c, Log: logr.Discard(), Namespace: "reliability-lab",
		Decision: classifier.Decision{Kind: classifier.CircuitBreak, Target: "orders-svc-flags"},
	}
	if err := (CircuitBreak{}).Apply(context.Background(), in); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got := &corev1.ConfigMap{}
	_ = c.Get(context.Background(), types.NamespacedName{Namespace: "reliability-lab", Name: "orders-svc-flags"}, got)
	if got.ResourceVersion != "42" {
		t.Fatalf("ResourceVersion changed (= %q), want unchanged 42 — no-op should not write", got.ResourceVersion)
	}
}
