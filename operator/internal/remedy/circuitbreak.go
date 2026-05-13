// CircuitBreak remedy: writes publish_enabled=false into the
// orders-svc-flags ConfigMap. orders-svc mounts that ConfigMap as a
// volume and fsnotify-watches the mount directory, so flipping this
// value pauses event publishing within ~kubelet sync seconds. Pausing
// the publisher drains the email-worker backlog without restarting
// the worker or losing in-flight orders — orders themselves keep
// being authorized, only the async email side-effect is shed.
//
// The remedy is one half of a control loop, not a one-way trip: the
// operator writes the flag, the email-worker SLO returns to budget,
// then a separate path (a follow-up cron, an explicit human step, or
// a future "all clear" remedy) lifts the flag by deleting the key.
// We deliberately do NOT auto-clear here, because the "should we lift
// it yet?" decision needs SLO observation, not the original alert.
package remedy

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CircuitBreak writes the brake flag into the named ConfigMap. The
// ConfigMap is created if missing — this lets the operator engage the
// brake even if the platform never installed the orders-svc-flags
// stub, and keeps the apply path symmetric whether someone deleted
// the CM mid-incident or it never existed in the first place.
type CircuitBreak struct{}

const (
	flagKey         = "publish_enabled"
	flagValuePaused = "false"
)

func (CircuitBreak) Apply(ctx context.Context, in Input) error {
	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{Namespace: in.Namespace, Name: in.Decision.Target}
	err := in.Client.Get(ctx, key, cm)
	switch {
	case apierrors.IsNotFound(err):
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: in.Decision.Target, Namespace: in.Namespace},
			Data:       map[string]string{flagKey: flagValuePaused},
		}
		if err := in.Client.Create(ctx, cm); err != nil {
			return fmt.Errorf("create flags configmap %s: %w", key, err)
		}
		in.Log.Info("circuit-break engaged (configmap created)",
			"configmap", key.String(), "key", flagKey, "reason", in.Decision.Reason)
		return nil
	case err != nil:
		return fmt.Errorf("get flags configmap %s: %w", key, err)
	}

	if cm.Data[flagKey] == flagValuePaused {
		in.Log.Info("circuit-break already engaged (no-op)",
			"configmap", key.String(), "key", flagKey)
		return nil
	}
	patch := client.MergeFrom(cm.DeepCopy())
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data[flagKey] = flagValuePaused
	if err := in.Client.Patch(ctx, cm, patch); err != nil {
		return fmt.Errorf("patch flags configmap %s: %w", key, err)
	}
	in.Log.Info("circuit-break engaged",
		"configmap", key.String(), "key", flagKey, "reason", in.Decision.Reason)
	return nil
}
