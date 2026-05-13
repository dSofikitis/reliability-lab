// AlertManager webhook payload schema. Mirrored from the Prometheus
// docs at prometheus.io/docs/alerting/latest/configuration/#webhook_config.
// We deliberately keep only the fields the operator actually reads
// (and a couple useful for logging) so a payload schema evolution
// upstream surfaces as missing data we can detect, not as a silently
// ignored mismatch.
package server

import "time"

type AlertManagerPayload struct {
	Version  string  `json:"version"`
	GroupKey string  `json:"groupKey"`
	Status   string  `json:"status"` // firing | resolved
	Receiver string  `json:"receiver"`
	Alerts   []Alert `json:"alerts"`
}

type Alert struct {
	Status      string            `json:"status"` // firing | resolved
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt"`
	Fingerprint string            `json:"fingerprint"`
}

// Firing is the only status the operator acts on. Resolved alerts are
// logged but never trigger a remedy — recovery is observed via the
// SLO returning to budget, not via an "all clear" webhook.
func (a Alert) Firing() bool { return a.Status == "firing" }
