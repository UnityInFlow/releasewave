package logging

import "testing"

func TestSetup_Debug_Text(t *testing.T) {
	// Should not panic.
	Setup("debug", "text")
}

func TestSetup_Info_JSON(t *testing.T) {
	// Should not panic.
	Setup("info", "json")
}

func TestSetup_Defaults(t *testing.T) {
	// Empty strings should use defaults (info level, text format) without panicking.
	Setup("", "")
}

func TestSetup_Warn_Text(t *testing.T) {
	// Should not panic.
	Setup("warn", "text")
}
