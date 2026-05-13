package classifier

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		name   string
		labels map[string]string
		want   Kind
	}{
		{
			name:   "orders fast burn -> rollback",
			labels: map[string]string{"severity": "page", "service": "orders", "slo": "orders_availability", "burn_speed": "fast"},
			want:   Rollback,
		},
		{
			name:   "orders latency -> rollback even without burn_speed",
			labels: map[string]string{"severity": "page", "service": "orders", "slo": "orders_latency"},
			want:   Rollback,
		},
		{
			name:   "payments fast burn -> scale up",
			labels: map[string]string{"severity": "page", "service": "payments", "slo": "payments_availability", "burn_speed": "fast"},
			want:   ScaleUp,
		},
		{
			name:   "inventory fast burn -> scale up",
			labels: map[string]string{"severity": "page", "service": "inventory", "slo": "inventory_availability", "burn_speed": "fast"},
			want:   ScaleUp,
		},
		{
			name:   "email-worker backlog -> circuit-break",
			labels: map[string]string{"severity": "page", "service": "email-worker", "slo": "email_delivery_within_60s"},
			want:   CircuitBreak,
		},
		{
			name:   "ticket severity -> no remedy",
			labels: map[string]string{"severity": "ticket", "service": "orders", "slo": "orders_availability", "burn_speed": "slow"},
			want:   NoRemedy,
		},
		{
			name:   "unknown service -> no remedy",
			labels: map[string]string{"severity": "page", "service": "frontend", "slo": "frontend_availability"},
			want:   NoRemedy,
		},
		{
			name:   "orders slow burn -> no remedy (humans investigate slow burns)",
			labels: map[string]string{"severity": "page", "service": "orders", "slo": "orders_availability", "burn_speed": "slow"},
			want:   NoRemedy,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.labels)
			if got.Kind != tc.want {
				t.Fatalf("kind = %v, want %v (reason: %s)", got.Kind, tc.want, got.Reason)
			}
		})
	}
}
