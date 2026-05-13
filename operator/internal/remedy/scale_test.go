package remedy

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/dSofikitis/reliability-lab/operator/internal/classifier"
)

func ptr32(v int32) *int32 { return &v }

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	return s
}

func TestScaleUp_Bumps(t *testing.T) {
	s := newScheme(t)
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-svc", Namespace: "reliability-lab"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MinReplicas: ptr32(2),
			MaxReplicas: 10,
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(hpa).Build()

	in := Input{
		Client: c, Log: logr.Discard(), Namespace: "reliability-lab",
		Decision: classifier.Decision{Kind: classifier.ScaleUp, Target: "payments-svc"},
	}
	if err := (ScaleUp{}).Apply(context.Background(), in); err != nil {
		t.Fatalf("apply: %v", err)
	}

	got := &autoscalingv2.HorizontalPodAutoscaler{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "reliability-lab", Name: "payments-svc"}, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Spec.MinReplicas == nil || *got.Spec.MinReplicas != 4 {
		t.Fatalf("minReplicas = %v, want 4", got.Spec.MinReplicas)
	}
}

func TestScaleUp_CapsAtMax(t *testing.T) {
	s := newScheme(t)
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-svc", Namespace: "reliability-lab"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MinReplicas: ptr32(9),
			MaxReplicas: 10,
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(hpa).Build()

	in := Input{
		Client: c, Log: logr.Discard(), Namespace: "reliability-lab",
		Decision: classifier.Decision{Kind: classifier.ScaleUp, Target: "payments-svc"},
	}
	if err := (ScaleUp{Step: 5}).Apply(context.Background(), in); err != nil {
		t.Fatalf("apply: %v", err)
	}

	got := &autoscalingv2.HorizontalPodAutoscaler{}
	_ = c.Get(context.Background(), types.NamespacedName{Namespace: "reliability-lab", Name: "payments-svc"}, got)
	if got.Spec.MinReplicas == nil || *got.Spec.MinReplicas != 10 {
		t.Fatalf("minReplicas = %v, want capped at 10", got.Spec.MinReplicas)
	}
}

func TestScaleUp_NoOpAtMax(t *testing.T) {
	s := newScheme(t)
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-svc", Namespace: "reliability-lab"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MinReplicas: ptr32(10),
			MaxReplicas: 10,
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(hpa).Build()

	in := Input{
		Client: c, Log: logr.Discard(), Namespace: "reliability-lab",
		Decision: classifier.Decision{Kind: classifier.ScaleUp, Target: "payments-svc"},
	}
	if err := (ScaleUp{}).Apply(context.Background(), in); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got := &autoscalingv2.HorizontalPodAutoscaler{}
	_ = c.Get(context.Background(), types.NamespacedName{Namespace: "reliability-lab", Name: "payments-svc"}, got)
	if *got.Spec.MinReplicas != 10 {
		t.Fatalf("minReplicas changed from 10 to %d (should be no-op)", *got.Spec.MinReplicas)
	}
}
