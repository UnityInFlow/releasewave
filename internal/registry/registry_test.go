package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeRegistryServer creates a minimal OCI Distribution Spec-compatible registry
// that supports tag listing and manifest fetching.
func fakeRegistryServer(t *testing.T) *httptest.Server {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// OCI Distribution: GET /v2/ — check API availability
		if path == "/v2/" || path == "/v2" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}

		// OCI Distribution: GET /v2/<name>/tags/list
		if strings.HasSuffix(path, "/tags/list") {
			name := strings.TrimPrefix(path, "/v2/")
			name = strings.TrimSuffix(name, "/tags/list")

			switch name {
			case "testorg/testapp":
				w.Header().Set("Content-Type", "application/json")
				resp := map[string]any{
					"name": "testorg/testapp",
					"tags": []string{"v2.0.0", "v1.1.0", "v1.0.0", "latest", "dev"},
				}
				data, _ := json.Marshal(resp)
				_, _ = w.Write(data)

			case "testorg/empty":
				w.Header().Set("Content-Type", "application/json")
				resp := map[string]any{
					"name": "testorg/empty",
					"tags": []string{},
				}
				data, _ := json.Marshal(resp)
				_, _ = w.Write(data)

			case "testorg/broken":
				w.WriteHeader(http.StatusInternalServerError)

			default:
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"errors":[{"code":"NAME_UNKNOWN"}]}`))
			}
			return
		}

		// OCI Distribution: GET /v2/<name>/manifests/<reference>
		if strings.Contains(path, "/manifests/") {
			parts := strings.SplitN(strings.TrimPrefix(path, "/v2/"), "/manifests/", 2)
			if len(parts) != 2 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			name := parts[0]
			ref := parts[1]

			if name == "testorg/testapp" {
				// Return different manifest bodies for different tags so
				// go-containerregistry computes different digests.
				var annotation string
				switch ref {
				case "v2.0.0", "latest":
					annotation = "v2.0.0"
				case "v1.1.0":
					annotation = "v1.1.0"
				case "v1.0.0":
					annotation = "v1.0.0"
				default:
					w.WriteHeader(http.StatusNotFound)
					return
				}

				manifest := fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[],"annotations":{"version":"%s"}}`, annotation)

				w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
				w.Header().Set("Docker-Content-Digest", fmt.Sprintf("sha256:%x", annotation))
				_, _ = w.Write([]byte(manifest))
				return
			}

			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})

	return httptest.NewServer(handler)
}

// registryHostFromServer extracts just the host:port from the test server URL.
func registryHostFromServer(server *httptest.Server) string {
	return strings.TrimPrefix(strings.TrimPrefix(server.URL, "http://"), "https://")
}

func TestListTags(t *testing.T) {
	server := fakeRegistryServer(t)
	defer server.Close()

	host := registryHostFromServer(server)
	client := New()

	tests := []struct {
		name      string
		imageRef  string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "returns tags for valid image",
			imageRef:  host + "/testorg/testapp",
			wantCount: 5,
			wantErr:   false,
		},
		{
			name:      "returns empty tags for image with no tags",
			imageRef:  host + "/testorg/empty",
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:     "returns error for server error",
			imageRef: host + "/testorg/broken",
			wantErr:  true,
		},
		{
			name:     "returns error for non-existent image",
			imageRef: host + "/testorg/doesnotexist",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := client.ListTags(context.Background(), tt.imageRef)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(info.Tags) != tt.wantCount {
				t.Errorf("got %d tags, want %d", len(info.Tags), tt.wantCount)
			}
		})
	}
}

func TestListTags_ContentCheck(t *testing.T) {
	server := fakeRegistryServer(t)
	defer server.Close()

	host := registryHostFromServer(server)
	client := New()

	info, err := client.ListTags(context.Background(), host+"/testorg/testapp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Image != host+"/testorg/testapp" {
		t.Errorf("Image = %q, want %q", info.Image, host+"/testorg/testapp")
	}

	if info.Registry != host {
		t.Errorf("Registry = %q, want %q", info.Registry, host)
	}

	// Tags should be sorted: v-prefixed first (newest first), then others
	if len(info.Tags) == 0 {
		t.Fatal("expected tags, got none")
	}

	// The latest tag should be a v-prefixed one (sorted by our sorting logic)
	if info.Latest == "" {
		t.Error("expected non-empty Latest tag")
	}

	// Verify v-prefixed tags come first
	foundNonV := false
	for _, tag := range info.Tags {
		isV := strings.HasPrefix(tag, "v")
		if foundNonV && isV {
			t.Errorf("v-prefixed tag %q appears after non-v tag in sorted list", tag)
		}
		if !isV {
			foundNonV = true
		}
	}
}

func TestListTags_EmptyImage(t *testing.T) {
	server := fakeRegistryServer(t)
	defer server.Close()

	host := registryHostFromServer(server)
	client := New()

	info, err := client.ListTags(context.Background(), host+"/testorg/empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(info.Tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(info.Tags))
	}
	if info.Latest != "" {
		t.Errorf("expected empty Latest, got %q", info.Latest)
	}
}

func TestListTags_InvalidRef(t *testing.T) {
	client := New()
	_, err := client.ListTags(context.Background(), ":::invalid-ref")
	if err == nil {
		t.Error("expected error for invalid image reference, got nil")
	}
}

func TestGetDigest(t *testing.T) {
	server := fakeRegistryServer(t)
	defer server.Close()

	host := registryHostFromServer(server)
	client := New()

	tests := []struct {
		name    string
		image   string
		tag     string
		wantErr bool
	}{
		{
			name:    "returns digest for valid tag",
			image:   host + "/testorg/testapp",
			tag:     "v2.0.0",
			wantErr: false,
		},
		{
			name:    "returns error for non-existent tag",
			image:   host + "/testorg/testapp",
			tag:     "nonexistent",
			wantErr: true,
		},
		{
			name:    "returns error for non-existent image",
			image:   host + "/testorg/doesnotexist",
			tag:     "v1.0.0",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			digest, err := client.GetDigest(context.Background(), tt.image, tt.tag)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if digest == "" {
				t.Error("expected non-empty digest")
			}
			if !strings.HasPrefix(digest, "sha256:") {
				t.Errorf("digest %q should start with sha256:", digest)
			}
		})
	}
}

func TestGetDigest_InvalidRef(t *testing.T) {
	client := New()
	_, err := client.GetDigest(context.Background(), ":::invalid", "v1.0.0")
	if err == nil {
		t.Error("expected error for invalid image reference, got nil")
	}
}

func TestCompareTag(t *testing.T) {
	server := fakeRegistryServer(t)
	defer server.Close()

	host := registryHostFromServer(server)
	client := New()

	tests := []struct {
		name     string
		image    string
		tag1     string
		tag2     string
		wantSame bool
		wantErr  bool
	}{
		{
			name:     "same digest for aliased tags",
			image:    host + "/testorg/testapp",
			tag1:     "v2.0.0",
			tag2:     "latest",
			wantSame: true,
			wantErr:  false,
		},
		{
			name:     "different digest for different versions",
			image:    host + "/testorg/testapp",
			tag1:     "v2.0.0",
			tag2:     "v1.0.0",
			wantSame: false,
			wantErr:  false,
		},
		{
			name:    "error when first tag does not exist",
			image:   host + "/testorg/testapp",
			tag1:    "nonexistent",
			tag2:    "v1.0.0",
			wantErr: true,
		},
		{
			name:    "error when second tag does not exist",
			image:   host + "/testorg/testapp",
			tag1:    "v1.0.0",
			tag2:    "nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			same, err := client.CompareTag(context.Background(), tt.image, tt.tag1, tt.tag2)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if same != tt.wantSame {
				t.Errorf("same = %v, want %v", same, tt.wantSame)
			}
		})
	}
}

func TestImageInfo_Structure(t *testing.T) {
	info := &ImageInfo{
		Registry: "ghcr.io",
		Image:    "ghcr.io/org/app",
		Tags:     []string{"v1.0.0", "latest"},
		Latest:   "v1.0.0",
		Digest:   "sha256:abc123",
	}

	if info.Registry != "ghcr.io" {
		t.Errorf("Registry = %q, want %q", info.Registry, "ghcr.io")
	}
	if info.Image != "ghcr.io/org/app" {
		t.Errorf("Image = %q, want %q", info.Image, "ghcr.io/org/app")
	}
	if len(info.Tags) != 2 {
		t.Errorf("Tags count = %d, want 2", len(info.Tags))
	}
	if info.Latest != "v1.0.0" {
		t.Errorf("Latest = %q, want %q", info.Latest, "v1.0.0")
	}
	if info.Digest != "sha256:abc123" {
		t.Errorf("Digest = %q, want %q", info.Digest, "sha256:abc123")
	}
}
