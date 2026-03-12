package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRelease_JSONRoundtrip(t *testing.T) {
	original := Release{
		Tag:         "v1.2.3",
		Name:        "Release 1.2.3",
		Body:        "Bug fixes and improvements",
		Draft:       false,
		Prerelease:  true,
		PublishedAt: time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
		HTMLURL:     "https://github.com/org/repo/releases/tag/v1.2.3",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded Release
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Tag != original.Tag {
		t.Errorf("Tag: got %q, want %q", decoded.Tag, original.Tag)
	}
	if decoded.Name != original.Name {
		t.Errorf("Name: got %q, want %q", decoded.Name, original.Name)
	}
	if decoded.Body != original.Body {
		t.Errorf("Body: got %q, want %q", decoded.Body, original.Body)
	}
	if decoded.Draft != original.Draft {
		t.Errorf("Draft: got %v, want %v", decoded.Draft, original.Draft)
	}
	if decoded.Prerelease != original.Prerelease {
		t.Errorf("Prerelease: got %v, want %v", decoded.Prerelease, original.Prerelease)
	}
	if !decoded.PublishedAt.Equal(original.PublishedAt) {
		t.Errorf("PublishedAt: got %v, want %v", decoded.PublishedAt, original.PublishedAt)
	}
	if decoded.HTMLURL != original.HTMLURL {
		t.Errorf("HTMLURL: got %q, want %q", decoded.HTMLURL, original.HTMLURL)
	}
}

func TestTag_JSONRoundtrip(t *testing.T) {
	original := Tag{
		Name:   "v2.0.0",
		Commit: "abc123def456",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded Tag
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Name != original.Name {
		t.Errorf("Name: got %q, want %q", decoded.Name, original.Name)
	}
	if decoded.Commit != original.Commit {
		t.Errorf("Commit: got %q, want %q", decoded.Commit, original.Commit)
	}
}

func TestService_JSONRoundtrip(t *testing.T) {
	original := Service{
		Name:     "my-service",
		Repo:     "github.com/org/repo",
		Registry: "ghcr.io/org/repo",
		Platform: "github",
		Owner:    "org",
		RepoName: "repo",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded Service
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Name != original.Name {
		t.Errorf("Name: got %q, want %q", decoded.Name, original.Name)
	}
	if decoded.Repo != original.Repo {
		t.Errorf("Repo: got %q, want %q", decoded.Repo, original.Repo)
	}
	if decoded.Registry != original.Registry {
		t.Errorf("Registry: got %q, want %q", decoded.Registry, original.Registry)
	}
	if decoded.Platform != original.Platform {
		t.Errorf("Platform: got %q, want %q", decoded.Platform, original.Platform)
	}
	if decoded.Owner != original.Owner {
		t.Errorf("Owner: got %q, want %q", decoded.Owner, original.Owner)
	}
	if decoded.RepoName != original.RepoName {
		t.Errorf("RepoName: got %q, want %q", decoded.RepoName, original.RepoName)
	}
}

func TestVersionStatus_OmitEmptyDeployedVersion(t *testing.T) {
	vs := VersionStatus{
		Service:       "my-service",
		LatestRelease: "v1.0.0",
		Behind:        0,
		UpToDate:      true,
	}

	data, err := json.Marshal(vs)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	raw := make(map[string]json.RawMessage)
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	if _, exists := raw["deployed_version"]; exists {
		t.Error("deployed_version should be omitted when empty")
	}

	// Now set DeployedVersion and verify it appears.
	vs.DeployedVersion = "v0.9.0"
	data, err = json.Marshal(vs)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	raw = make(map[string]json.RawMessage)
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	if _, exists := raw["deployed_version"]; !exists {
		t.Error("deployed_version should be present when set")
	}
}
