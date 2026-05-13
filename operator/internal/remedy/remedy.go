// Remedy interface + registry. A Remedy is a concrete action the
// operator takes against the cluster in response to a classified
// alert. The registry maps a classifier.Kind to its Remedy so the
// dispatch path is just classify -> registry.Lookup -> Apply.
//
// Each remedy implementation lives in its own file so the runbook
// alongside it can stay tightly scoped — no scrolling through 500
// lines of unrelated remedies to read the one that just fired.
package remedy

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dSofikitis/reliability-lab/operator/internal/classifier"
)

// Input is everything a remedy needs to act. Kept narrow so the
// dependency surface for tests is just (Client + Log + Decision).
type Input struct {
	Client    client.Client
	Log       logr.Logger
	Namespace string
	Decision  classifier.Decision
}

// Remedy is the contract every remedy implementation satisfies. Apply
// must be idempotent — the dedupe layer above is best-effort, not a
// hard guarantee, so a double-apply must converge to the same end state.
type Remedy interface {
	Apply(ctx context.Context, in Input) error
}

// Registry holds the kind -> Remedy mapping. Built once at startup,
// looked up on every dispatch. A nil entry means "no remedy registered
// for this kind" — Lookup returns ok=false and the dispatcher logs.
type Registry struct {
	m map[classifier.Kind]Remedy
}

func NewRegistry() *Registry { return &Registry{m: map[classifier.Kind]Remedy{}} }

func (r *Registry) Register(k classifier.Kind, rem Remedy) { r.m[k] = rem }

func (r *Registry) Lookup(k classifier.Kind) (Remedy, bool) {
	rem, ok := r.m[k]
	return rem, ok
}

// Apply is a convenience wrapper that finds and runs the remedy for a
// decision in one step. Unknown kinds are reported as a typed error so
// the caller can distinguish "no remedy" from "remedy ran and failed".
func (r *Registry) Apply(ctx context.Context, in Input) error {
	rem, ok := r.Lookup(in.Decision.Kind)
	if !ok {
		return fmt.Errorf("no remedy registered for kind %s", in.Decision.Kind)
	}
	return rem.Apply(ctx, in)
}
