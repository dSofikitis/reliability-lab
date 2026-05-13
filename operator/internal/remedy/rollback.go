// Rollback remedy: aborts an in-flight Argo Rollout, which scales the
// canary ReplicaSet to zero and returns 100% of traffic to the
// previous stable. This is exactly the semantics we want for the
// MTTR demo, where the burning SLO is caused by a bad canary.
//
// Implemented via the dynamic client + unstructured rather than
// importing argoproj's typed v1alpha1 API, on purpose: the typed
// SDK pulls in the entire argo-rollouts module, and the only field
// we touch is status.abort. An unstructured patch is the smaller
// dependency footprint and the patch payload reads as documentation.
//
// If the rollout is already at 100% (no canary in progress), abort
// is a no-op — there's no canary RS to scale down. We log and move
// on; the next-tier remedy (operator can be extended, or human
// follows the runbook) takes over. For this repo's chaos drill, the
// canary is always in progress when the alert fires, so abort is
// always meaningful.
package remedy

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var rolloutGVR = schema.GroupVersionKind{
	Group:   "argoproj.io",
	Version: "v1alpha1",
	Kind:    "Rollout",
}

type Rollback struct{}

func (Rollback) Apply(ctx context.Context, in Input) error {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(rolloutGVR)
	u.SetNamespace(in.Namespace)
	u.SetName(in.Decision.Target)

	// status.abort=true is the field the argo-rollouts controller
	// watches on the Rollout's status subresource. Setting it from
	// outside is the documented mechanism the kubectl-argo-rollouts
	// `abort` command uses under the hood.
	patch := []byte(`{"status":{"abort":true}}`)
	if err := in.Client.Status().Patch(ctx, u, client.RawPatch(types.MergePatchType, patch)); err != nil {
		return fmt.Errorf("patch rollout %s/%s status.abort: %w", in.Namespace, in.Decision.Target, err)
	}
	in.Log.Info("rollback aborted in-flight rollout",
		"target", in.Decision.Target, "namespace", in.Namespace, "reason", in.Decision.Reason)
	return nil
}
