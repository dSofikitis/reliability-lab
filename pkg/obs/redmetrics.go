// HTTP RED metrics. A small wrapper around prometheus.CounterVec +
// HistogramVec that emits request count + duration with the standard
// {handler, method, code} label set, plus a middleware that records
// them around any http.Handler.
//
// Why not use Linkerd's mesh metrics for the SLO instead: the
// linkerd-proxy sidecar emits its metrics on the admin port (4191),
// which kube-prometheus-stack does NOT scrape by default — pulling
// them in would mean federating from linkerd-viz's own Prometheus
// AND punching a hole through linkerd-viz's AuthorizationPolicy.
// App-side metrics are simpler, deterministic, and survive the
// service mesh being absent (kind without injection, local `go run`,
// etc.) — strictly fewer moving parts on the SLO data path.
package obs

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type HTTPMetrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewHTTPMetrics builds the two collectors and registers them. The
// Namespace prefixes the metric name, so each service ends up with
// e.g. `orders_http_requests_total` — keeps the global Prometheus
// surface scoped per service without label-cardinality wrestling.
func NewHTTPMetrics(reg prometheus.Registerer, namespace string) *HTTPMetrics {
	m := &HTTPMetrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_requests_total",
			Help:      "Count of HTTP requests handled, by handler/method/response code.",
		}, []string{"handler", "method", "code"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds, by handler/method/response code.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"handler", "method", "code"}),
	}
	reg.MustRegister(m.requests, m.duration)
	return m
}

// Wrap returns an http.Handler that records the per-request metrics
// around `next`. `handler` is the static label used to identify the
// route (e.g. "post_orders") — keep it constant per route to avoid
// label-cardinality explosions on path-parameter values.
func (m *HTTPMetrics) Wrap(handler string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rw, r)
		labels := prometheus.Labels{
			"handler": handler,
			"method":  r.Method,
			"code":    strconv.Itoa(rw.code),
		}
		m.requests.With(labels).Inc()
		m.duration.With(labels).Observe(time.Since(start).Seconds())
	})
}

// statusRecorder captures the response status without buffering the
// body. Wraps http.ResponseWriter at the call site; doesn't try to
// implement the full ResponseWriter optional interfaces (Hijacker,
// Flusher) because the orders-svc handler doesn't use them.
type statusRecorder struct {
	http.ResponseWriter
	code        int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(c int) {
	if !r.wroteHeader {
		r.code = c
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(c)
}
