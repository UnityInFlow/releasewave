package discovery

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestInferRepoFromImage_GHCR(t *testing.T) {
	got := inferRepoFromImage("ghcr.io/org/app:v1.0")
	if got != "github.com/org/app" {
		t.Errorf("expected github.com/org/app, got %q", got)
	}
}

func TestInferRepoFromImage_GitLab(t *testing.T) {
	got := inferRepoFromImage("registry.gitlab.com/org/service:latest")
	if got != "gitlab.com/org/service" {
		t.Errorf("expected gitlab.com/org/service, got %q", got)
	}
}

func TestInferRepoFromImage_DockerHub(t *testing.T) {
	got := inferRepoFromImage("docker.io/org/app:v2.0")
	if got != "github.com/org/app" {
		t.Errorf("expected github.com/org/app, got %q", got)
	}
}

func TestInferRepoFromImage_UnknownRegistry(t *testing.T) {
	got := inferRepoFromImage("custom.registry.io/org/app:v1")
	if got != "" {
		t.Errorf("expected empty for unknown registry, got %q", got)
	}
}

func TestInferRepoFromImage_TooFewParts(t *testing.T) {
	got := inferRepoFromImage("nginx:latest")
	if got != "" {
		t.Errorf("expected empty for short image ref, got %q", got)
	}
}

func TestInferRepoFromImage_Digest(t *testing.T) {
	got := inferRepoFromImage("ghcr.io/org/app@sha256:abc123")
	if got != "github.com/org/app" {
		t.Errorf("expected github.com/org/app, got %q", got)
	}
}

func TestImageWithoutTag(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ghcr.io/org/app:v1.0", "ghcr.io/org/app"},
		{"ghcr.io/org/app@sha256:abc", "ghcr.io/org/app"},
		{"ghcr.io/org/app", "ghcr.io/org/app"},
		{"localhost:5000/app:latest", "localhost:5000/app"},
	}
	for _, tt := range tests {
		got := imageWithoutTag(tt.input)
		if got != tt.want {
			t.Errorf("imageWithoutTag(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDiscoverFromWorkload_WithAnnotation(t *testing.T) {
	annotations := map[string]string{
		AnnotationRepo: "github.com/my-org/my-repo",
		AnnotationName: "custom-name",
	}
	containers := []corev1.Container{
		{Image: "ghcr.io/other/image:v1"},
	}

	svcs := discoverFromWorkload("deploy-name", annotations, containers)

	if len(svcs) != 1 {
		t.Fatalf("expected 1 service, got %d", len(svcs))
	}
	if svcs[0].Name != "custom-name" {
		t.Errorf("expected name 'custom-name', got %q", svcs[0].Name)
	}
	if svcs[0].Repo != "github.com/my-org/my-repo" {
		t.Errorf("expected repo from annotation, got %q", svcs[0].Repo)
	}
}

func TestDiscoverFromWorkload_AnnotationRepoOnly(t *testing.T) {
	annotations := map[string]string{
		AnnotationRepo: "github.com/org/repo",
	}

	svcs := discoverFromWorkload("my-deploy", annotations, nil)

	if len(svcs) != 1 {
		t.Fatalf("expected 1 service, got %d", len(svcs))
	}
	// Falls back to workload name when AnnotationName is absent.
	if svcs[0].Name != "my-deploy" {
		t.Errorf("expected name 'my-deploy', got %q", svcs[0].Name)
	}
}

func TestDiscoverFromWorkload_InferredFromImage(t *testing.T) {
	annotations := map[string]string{}
	containers := []corev1.Container{
		{Image: "ghcr.io/org/service:v1.2.3"},
	}

	svcs := discoverFromWorkload("my-deploy", annotations, containers)

	if len(svcs) != 1 {
		t.Fatalf("expected 1 service, got %d", len(svcs))
	}
	if svcs[0].Repo != "github.com/org/service" {
		t.Errorf("expected inferred repo, got %q", svcs[0].Repo)
	}
	if svcs[0].Registry != "ghcr.io/org/service" {
		t.Errorf("expected registry 'ghcr.io/org/service', got %q", svcs[0].Registry)
	}
}

func TestDiscoverFromWorkload_NoAnnotationUnknownImage(t *testing.T) {
	annotations := map[string]string{}
	containers := []corev1.Container{
		{Image: "nginx:latest"},
	}

	svcs := discoverFromWorkload("web", annotations, containers)

	if len(svcs) != 0 {
		t.Errorf("expected 0 services for unknown image, got %d", len(svcs))
	}
}

func TestDiscoverFromWorkload_EmptyAnnotationRepo(t *testing.T) {
	// Empty AnnotationRepo should fall through to image inference.
	annotations := map[string]string{
		AnnotationRepo: "",
	}
	containers := []corev1.Container{
		{Image: "ghcr.io/org/app:v1"},
	}

	svcs := discoverFromWorkload("deploy", annotations, containers)

	if len(svcs) != 1 {
		t.Fatalf("expected 1 service from image inference, got %d", len(svcs))
	}
	if svcs[0].Repo != "github.com/org/app" {
		t.Errorf("expected inferred repo, got %q", svcs[0].Repo)
	}
}
