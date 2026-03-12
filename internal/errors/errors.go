// Package errors defines sentinel errors and error types for ReleaseWave.
package errors

import (
	"errors"
	"fmt"
)

// Sentinel errors.
var (
	ErrNotFound  = errors.New("not found")
	ErrRateLimit = errors.New("rate limit exceeded")
	ErrAuth      = errors.New("authentication failed")
)

// ProviderError wraps an error with platform context.
type ProviderError struct {
	Platform string
	Status   int
	Message  string
	Err      error
}

func (e *ProviderError) Error() string {
	if e.Status > 0 {
		return fmt.Sprintf("%s: HTTP %d: %s", e.Platform, e.Status, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Platform, e.Message)
}

func (e *ProviderError) Unwrap() error { return e.Err }

// NewProviderError creates a ProviderError from an HTTP status code.
func NewProviderError(platform string, status int, url string) *ProviderError {
	switch {
	case status == 401 || status == 403:
		return &ProviderError{Platform: platform, Status: status, Message: "unauthorized — check your API token", Err: ErrAuth}
	case status == 404:
		return &ProviderError{Platform: platform, Status: status, Message: fmt.Sprintf("resource not found: %s", url), Err: ErrNotFound}
	case status == 429:
		return &ProviderError{Platform: platform, Status: status, Message: "rate limit exceeded — try again later", Err: ErrRateLimit}
	default:
		return &ProviderError{Platform: platform, Status: status, Message: fmt.Sprintf("unexpected status for %s", url), Err: nil}
	}
}

// ConfigError represents a configuration problem.
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("config error: %s: %s", e.Field, e.Message)
}

// IsNotFound checks if an error chain contains ErrNotFound.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// IsRateLimit checks if an error chain contains ErrRateLimit.
func IsRateLimit(err error) bool { return errors.Is(err, ErrRateLimit) }

// IsAuth checks if an error chain contains ErrAuth.
func IsAuth(err error) bool { return errors.Is(err, ErrAuth) }
