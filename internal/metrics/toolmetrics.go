// Package metrics defines Prometheus metrics for MCP tool calls and caching.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ToolCallsTotal counts MCP tool invocations by tool name and status.
	ToolCallsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "releasewave_tool_calls_total",
		Help: "Total number of MCP tool calls.",
	}, []string{"tool", "status"})

	// ToolCallDuration records MCP tool call latency.
	ToolCallDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "releasewave_tool_call_duration_seconds",
		Help:    "MCP tool call latency in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"tool"})

	// CacheHitsTotal counts cache lookups by result.
	CacheHitsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "releasewave_cache_hits_total",
		Help: "Total cache lookups by result (hit or miss).",
	}, []string{"result"})
)
