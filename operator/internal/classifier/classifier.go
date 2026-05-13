// Classifier: maps an AlertManager alert (by its labels) to a remedy
// decision. Pure function over the labels — no I/O, no kube client —
// so the policy is trivial to unit-test and easy to reason about as
// a flat truth table.
//
// The mapping is intentionally hardcoded rather than driven by a CRD
// or annotations: the rule set is small (5-ish alerts, 3 remedies),
// it changes when the SLO rules change, and a Go function with a
// switch is the most readable + reviewable form for that. CRD-driven
// policies belong in a multi-team platform; here the operator and
// the alert rules are co-developed and co-versioned in the same repo.
package classifier

import "fmt"

// Kind enumerates the three remedies the operator can take. NoRemedy
// is returned when the alert is real but the operator should stay out
// of the way — e.g. ticket-severity slow burns that humans handle.
type Kind int

const (
	NoRemedy Kind = iota
	Rollback
	ScaleUp
	CircuitBreak
)

func (k Kind) String() string {
	switch k {
	case Rollback:
		return "rollback"
	case ScaleUp:
		return "scale-up"
	case CircuitBreak:
		return "circuit-break"
	default:
		return "no-remedy"
	}
}

// Decision is the output of Classify. Target is the kube object name
// the remedy operates on (a Rollout name for Rollback, an HPA name
// for ScaleUp, a ConfigMap name for CircuitBreak). Namespace is left
// to the caller to default — usually reliability-lab.
type Decision struct {
	Kind   Kind
	Target string
	Reason string
}

// Classify returns the remedy decision for a single alert's labels.
// The caller has already filtered by Status==firing and dedupe.
func Classify(labels map[string]string) Decision {
	severity := labels["severity"]
	service := labels["service"]
	slo := labels["slo"]
	burn := labels["burn_speed"]

	// Slow burns and tickets are humans' to handle — auto-acting on
	// them would deny the team the chance to investigate the
	// underlying drift, which is the whole point of having a slow
	// burn alert in the first place.
	if severity != "page" {
		return Decision{Kind: NoRemedy, Reason: fmt.Sprintf("severity=%q, only page is auto-actioned", severity)}
	}

	switch {
	// Email worker SLO breaches — pause the publisher to drain the
	// queue while the worker catches up. Both backlog-growing and
	// OOM-restart alerts route here; the remedy is the same.
	case service == "email-worker":
		return Decision{
			Kind:   CircuitBreak,
			Target: "orders-svc-flags",
			Reason: fmt.Sprintf("email-worker SLO %q burning; pause orders-svc publish to drain backlog", slo),
		}

	// Orders availability or latency burning fast — most likely a
	// regression in the most-recent canary. Rolling back is the
	// cheapest, fastest way to clear the burn; if the regression is
	// upstream the rollback is a no-op and the next alert (payments
	// or inventory burn) gets the right remedy.
	case service == "orders" && (burn == "fast" || slo == "orders_latency"):
		return Decision{
			Kind:   Rollback,
			Target: "orders-svc",
			Reason: fmt.Sprintf("orders SLO %q burning fast; rollback to previous stable", slo),
		}

	// Payments fast burn is canonically the inventory retry storm —
	// scaling payments-svc absorbs the retry traffic; if the cause
	// is something else, the HPA settles back down on its own.
	case service == "payments" && burn == "fast":
		return Decision{
			Kind:   ScaleUp,
			Target: "payments-svc",
			Reason: fmt.Sprintf("payments SLO %q burning fast; scale up to absorb load", slo),
		}

	// Inventory fast burn — bump inventory-svc directly. Distinct
	// from the payments path because the symptom can appear on
	// either side depending on which chaos experiment fired.
	case service == "inventory" && burn == "fast":
		return Decision{
			Kind:   ScaleUp,
			Target: "inventory-svc",
			Reason: fmt.Sprintf("inventory SLO %q burning fast; scale up", slo),
		}
	}

	return Decision{Kind: NoRemedy, Reason: "no rule matched alert labels"}
}
