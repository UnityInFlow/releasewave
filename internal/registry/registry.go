// Package registry provides container registry integration for checking deployed image versions.
package registry

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// ImageInfo represents information about a container image.
type ImageInfo struct {
	Registry string   `json:"registry"`
	Image    string   `json:"image"`
	Tags     []string `json:"tags"`
	Latest   string   `json:"latest_tag"`
	Digest   string   `json:"digest,omitempty"`
}

// Client queries container registries using the OCI Distribution Spec.
// Works with Docker Hub, GHCR, GitLab Registry, ECR, GCR, Harbor, etc.
type Client struct {
	keychain authn.Keychain
}

// New creates a registry client. Uses default Docker credential helpers
// (reads ~/.docker/config.json for auth).
func New() *Client {
	return &Client{
		keychain: authn.DefaultKeychain,
	}
}

// ListTags returns all tags for an image reference.
// imageRef examples: "ghcr.io/org/app", "docker.io/library/nginx", "registry.gitlab.com/org/app"
func (c *Client) ListTags(ctx context.Context, imageRef string) (*ImageInfo, error) {
	repo, err := name.NewRepository(imageRef)
	if err != nil {
		return nil, fmt.Errorf("parse image ref %q: %w", imageRef, err)
	}

	slog.Debug("registry.list_tags", "image", imageRef)

	tags, err := remote.List(repo, remote.WithAuthFromKeychain(c.keychain), remote.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("list tags for %q: %w", imageRef, err)
	}

	// Sort tags: semver-like tags first (v-prefixed), newest first
	sort.Slice(tags, func(i, j int) bool {
		// Prefer v-prefixed tags
		iV := strings.HasPrefix(tags[i], "v")
		jV := strings.HasPrefix(tags[j], "v")
		if iV != jV {
			return iV
		}
		// Reverse alphabetical (higher versions first)
		return tags[i] > tags[j]
	})

	latest := ""
	if len(tags) > 0 {
		latest = tags[0]
	}

	info := &ImageInfo{
		Registry: repo.RegistryStr(),
		Image:    imageRef,
		Tags:     tags,
		Latest:   latest,
	}

	slog.Debug("registry.list_tags.done", "image", imageRef, "count", len(tags), "latest", latest)
	return info, nil
}

// GetDigest returns the digest of a specific tag.
func (c *Client) GetDigest(ctx context.Context, imageRef string, tag string) (string, error) {
	ref, err := name.ParseReference(fmt.Sprintf("%s:%s", imageRef, tag))
	if err != nil {
		return "", fmt.Errorf("parse ref: %w", err)
	}

	desc, err := remote.Get(ref, remote.WithAuthFromKeychain(c.keychain), remote.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("get digest for %s:%s: %w", imageRef, tag, err)
	}

	return desc.Digest.String(), nil
}

// CompareTag checks if two tags point to the same digest (i.e., same image).
func (c *Client) CompareTag(ctx context.Context, imageRef, tag1, tag2 string) (bool, error) {
	d1, err := c.GetDigest(ctx, imageRef, tag1)
	if err != nil {
		return false, err
	}
	d2, err := c.GetDigest(ctx, imageRef, tag2)
	if err != nil {
		return false, err
	}
	return d1 == d2, nil
}
