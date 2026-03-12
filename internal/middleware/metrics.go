package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "releasewave_http_requests_total",
		Help: "Total number of HTTP requests.",
	}, []string{"method", "path", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "releasewave_http_request_duration_seconds",
		Help:    "HTTP request latency in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush delegates to the underlying ResponseWriter if it implements http.Flusher.
// This is required for SSE streaming to work through the metrics middleware.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// normalizePath collapses dynamic path segments to avoid unbounded label cardinality.
func normalizePath(p string) string {
	switch {
	case strings.HasPrefix(p, "/api/"):
		// Normalize /api/v1/services/<name>/releases → /api/v1/services/{name}/releases
		parts := strings.Split(p, "/")
		if len(parts) >= 5 && parts[2] == "v1" && parts[3] == "services" {
			parts[4] = "{name}"
			return strings.Join(parts, "/")
		}
		return p
	case strings.HasPrefix(p, "/sse"):
		return "/sse"
	case strings.HasPrefix(p, "/message"):
		return "/message"
	case strings.HasPrefix(p, "/dashboard"):
		return "/dashboard"
	default:
		// Cap unknown paths at 3 segments to prevent cardinality explosion.
		parts := strings.SplitN(p, "/", 5) // ["", seg1, seg2, seg3, rest...]
		if len(parts) > 4 {
			return strings.Join(parts[:4], "/")
		}
		return p
	}
}

// Metrics returns HTTP middleware that records request count and latency.
func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		duration := time.Since(start).Seconds()
		path := normalizePath(r.URL.Path)
		httpRequestsTotal.WithLabelValues(r.Method, path, strconv.Itoa(rec.status)).Inc()
		httpRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
	})
}
