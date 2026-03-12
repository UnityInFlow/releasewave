package metrics

import "testing"

func TestToolCallsTotal_NotNil(t *testing.T) {
	if ToolCallsTotal == nil {
		t.Fatal("ToolCallsTotal should not be nil")
	}
}

func TestToolCallDuration_NotNil(t *testing.T) {
	if ToolCallDuration == nil {
		t.Fatal("ToolCallDuration should not be nil")
	}
}

func TestCacheHitsTotal_NotNil(t *testing.T) {
	if CacheHitsTotal == nil {
		t.Fatal("CacheHitsTotal should not be nil")
	}
}

func TestToolCallsTotal_Increment(t *testing.T) {
	// Should not panic.
	ToolCallsTotal.WithLabelValues("test_tool", "ok").Inc()
}

func TestToolCallDuration_Observe(t *testing.T) {
	// Should not panic.
	ToolCallDuration.WithLabelValues("test_tool").Observe(0.123)
}
