// ScaleUp remedy: bumps the HPA's minReplicas for the target service
// so the deployment immediately scales out to absorb whatever load
// spike (retry storm, latency cascade) is burning the SLO.
//
// We bump minReplicas rather than the Deployment's spec.replicas
// because an HPA actively manages spec.replicas — any direct write
// would be silently reverted on the next HPA reconcile. Lifting
// minReplicas is the supported way to coerce the HPA upward without
// fighting it; the HPA will scale back down on its own once load
// returns to normal.
//
// The bump is bounded: we increment by Step (default 2) but never
// exceed maxReplicas, and we never increase if the HPA is already at
// or above the bump target. Idempotency falls out of these checks
// for free.
package remedy

import (
	"context"
	"fmt"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ScaleUp struct {
	// Step is how many replicas to add to minReplicas per remedy fire.
	// Defaults to 2 if zero. Capped at maxReplicas regardless.
	Step int32
}

func (s ScaleUp) Apply(ctx context.Context, in Input) error {
	step := s.Step
	if step <= 0 {
		step = 2
	}

	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	key := types.NamespacedName{Namespace: in.Namespace, Name: in.Decision.Target}
	if err := in.Client.Get(ctx, key, hpa); err != nil {
		return fmt.Errorf("get hpa %s: %w", key, err)
	}

	current := int32(0)
	if hpa.Spec.MinReplicas != nil {
		current = *hpa.Spec.MinReplicas
	}
	target := current + step
	if target > hpa.Spec.MaxReplicas {
		target = hpa.Spec.MaxReplicas
	}
	if target <= current {
		in.Log.Info("scale-up no-op (already at or above target)",
			"hpa", key.String(), "current_min", current, "max", hpa.Spec.MaxReplicas)
		return nil
	}

	patch := client.MergeFrom(hpa.DeepCopy())
	hpa.Spec.MinReplicas = &target
	if err := in.Client.Patch(ctx, hpa, patch); err != nil {
		return fmt.Errorf("patch hpa %s minReplicas: %w", key, err)
	}
	in.Log.Info("scale-up bumped HPA minReplicas",
		"hpa", key.String(), "from", current, "to", target,
		"max", hpa.Spec.MaxReplicas, "reason", in.Decision.Reason)
	return nil
}
