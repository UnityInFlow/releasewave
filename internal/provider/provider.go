// Package provider defines the interface that all platform providers must implement.
//
// Go learning notes:
//   - interfaces in Go are implicit — a type implements an interface just by having the right methods
//   - no "implements" keyword needed (unlike Java/C#)
//   - context.Context is used for cancellation, timeouts, and passing request-scoped values
//   - error is a built-in interface — functions return (result, error) by convention
package provider

import (
	"context"

	"github.com/UnityInFlow/releasewave/internal/model"
)

// Provider is the interface that all platform providers (GitHub, GitLab, etc.) must satisfy.
// Any struct that has these methods automatically implements this interface.
type Provider interface {
	// Name returns the provider name (e.g. "github", "gitlab").
	Name() string

	// ListReleases returns all releases for a repository.
	ListReleases(ctx context.Context, owner, repo string) ([]model.Release, error)

	// GetLatestRelease returns the most recent release.
	GetLatestRelease(ctx context.Context, owner, repo string) (*model.Release, error)

	// ListTags returns all tags for a repository.
	ListTags(ctx context.Context, owner, repo string) ([]model.Tag, error)

	// GetFileContent fetches the contents of a file from a repository at the default branch.
	GetFileContent(ctx context.Context, owner, repo, path string) ([]byte, error)
}
