package remedy

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/dSofikitis/reliability-lab/operator/internal/classifier"
)

// rolloutScheme registers the argoproj.io/v1alpha1 Rollout GVK as
// an unstructured type. Lets the fake client's tracker store and
// patch it without us having to pull in the argo-rollouts module
// just for its typed CRD definitions — the same trade-off the
// remedy itself makes (see rollback.go).
func rolloutScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	gv := schema.GroupVersion{Group: "argoproj.io", Version: "v1alpha1"}
	s.AddKnownTypeWithName(gv.WithKind("Rollout"), &unstructured.Unstructured{})
	s.AddKnownTypeWithName(gv.WithKind("RolloutList"), &unstructured.UnstructuredList{})
	return s
}

func newRollout(name, namespace string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "argoproj.io", Version: "v1alpha1", Kind: "Rollout",
	})
	u.SetName(name)
	u.SetNamespace(namespace)
	return u
}

func TestRollback_PatchesStatusAbort(t *testing.T) {
	rollout := newRollout("orders-svc", "reliability-lab")
	c := fake.NewClientBuilder().
		WithScheme(rolloutScheme(t)).
		WithObjects(rollout).
		// WithStatusSubresource opts the GVK into the fake client's
		// status-subresource handling — without this, Status().Patch
		// silently writes to spec instead and the test wouldn't
		// catch a regression where the remedy targeted the wrong
		// subresource.
		WithStatusSubresource(rollout).
		Build()

	in := Input{
		Client: c, Log: logr.Discard(), Namespace: "reliability-lab",
		Decision: classifier.Decision{Kind: classifier.Rollback, Target: "orders-svc"},
	}
	if err := (Rollback{}).Apply(context.Background(), in); err != nil {
		t.Fatalf("apply: %v", err)
	}

	got := newRollout("orders-svc", "reliability-lab")
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "reliability-lab", Name: "orders-svc"}, got); err != nil {
		t.Fatalf("get rollout: %v", err)
	}
	abort, found, err := unstructured.NestedBool(got.Object, "status", "abort")
	if err != nil {
		t.Fatalf("read status.abort: %v", err)
	}
	if !found || !abort {
		t.Fatalf("status.abort = (%v, found=%v), want (true, true)", abort, found)
	}
}

func TestRollback_ErrorsWhenRolloutMissing(t *testing.T) {
	c := fake.NewClientBuilder().
		WithScheme(rolloutScheme(t)).
		Build()

	in := Input{
		Client: c, Log: logr.Discard(), Namespace: "reliability-lab",
		Decision: classifier.Decision{Kind: classifier.Rollback, Target: "orders-svc"},
	}
	err := (Rollback{}).Apply(context.Background(), in)
	if err == nil {
		t.Fatal("expected error when target rollout doesn't exist; got nil")
	}
	// Don't pin the exact wording — the error wraps whatever the
	// kube client returns, which can vary across client-go versions.
	// Existence-of-error is the contract; that's what we test.
}
