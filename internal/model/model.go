// Package model defines the core data types used across ReleaseWave.
//
// Go learning notes:
//   - structs are Go's way of defining custom types (like classes, but no inheritance)
//   - `json:"name"` are struct tags — they tell the JSON encoder/decoder what field names to use
//   - time.Time is Go's standard type for dates/times
package model

import "time"

// Release represents a single release from a git platform.
type Release struct {
	Tag         string    `json:"tag" yaml:"tag"`
	Name        string    `json:"name" yaml:"name"`
	Body        string    `json:"body" yaml:"body"` // Release notes / changelog
	Draft       bool      `json:"draft" yaml:"draft"`
	Prerelease  bool      `json:"prerelease" yaml:"prerelease"`
	PublishedAt time.Time `json:"published_at" yaml:"published_at"`
	HTMLURL     string    `json:"html_url" yaml:"html_url"`
}

// Tag represents a git tag.
type Tag struct {
	Name   string `json:"name" yaml:"name"`
	Commit string `json:"commit" yaml:"commit"`
}

// Service represents a microservice tracked by ReleaseWave.
type Service struct {
	Name     string `json:"name" yaml:"name"`
	Repo     string `json:"repo" yaml:"repo"`         // e.g. "github.com/org/repo"
	Registry string `json:"registry" yaml:"registry"` // e.g. "ghcr.io/org/repo"
	Platform string `json:"platform" yaml:"platform"` // "github", "gitlab", etc.
	Owner    string `json:"owner" yaml:"owner"`       // org or user
	RepoName string `json:"repo_name" yaml:"repo_name"`
}

// VersionStatus shows the version comparison for a service.
type VersionStatus struct {
	Service         string `json:"service"`
	LatestRelease   string `json:"latest_release"`
	DeployedVersion string `json:"deployed_version,omitempty"`
	Behind          int    `json:"behind"` // How many releases behind
	UpToDate        bool   `json:"up_to_date"`
}
