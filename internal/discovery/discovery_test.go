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

func TestNewK8sDiscoverer(t *testing.T) {
	d := NewK8sDiscoverer("/path/to/kubeconfig", "my-context", "my-namespace")
	if d.kubeconfig != "/path/to/kubeconfig" {
		t.Errorf("expected kubeconfig '/path/to/kubeconfig', got %q", d.kubeconfig)
	}
	if d.context != "my-context" {
		t.Errorf("expected context 'my-context', got %q", d.context)
	}
	if d.namespace != "my-namespace" {
		t.Errorf("expected namespace 'my-namespace', got %q", d.namespace)
	}
}

func TestNewK8sDiscoverer_EmptyParams(t *testing.T) {
	d := NewK8sDiscoverer("", "", "")
	if d.kubeconfig != "" {
		t.Errorf("expected empty kubeconfig, got %q", d.kubeconfig)
	}
	if d.context != "" {
		t.Errorf("expected empty context, got %q", d.context)
	}
	if d.namespace != "" {
		t.Errorf("expected empty namespace, got %q", d.namespace)
	}
}

func TestDiscoverFromWorkload_MultipleContainersAllInferable(t *testing.T) {
	annotations := map[string]string{}
	containers := []corev1.Container{
		{Image: "ghcr.io/org/frontend:v1.0"},
		{Image: "ghcr.io/org/backend:v2.0"},
	}

	svcs := discoverFromWorkload("my-app", annotations, containers)

	if len(svcs) != 2 {
		t.Fatalf("expected 2 services, got %d", len(svcs))
	}
	if svcs[0].Repo != "github.com/org/frontend" {
		t.Errorf("expected first repo 'github.com/org/frontend', got %q", svcs[0].Repo)
	}
	if svcs[0].Name != "my-app" {
		t.Errorf("expected first name 'my-app', got %q", svcs[0].Name)
	}
	if svcs[1].Repo != "github.com/org/backend" {
		t.Errorf("expected second repo 'github.com/org/backend', got %q", svcs[1].Repo)
	}
}

func TestDiscoverFromWorkload_MixedContainersSomeInferable(t *testing.T) {
	// One container is inferable (ghcr.io), the other is not (nginx).
	// Exercises the `continue` branch when inferRepoFromImage returns "".
	annotations := map[string]string{}
	containers := []corev1.Container{
		{Image: "nginx:latest"},
		{Image: "ghcr.io/org/api:v1.0"},
		{Image: "busybox:1.36"},
	}

	svcs := discoverFromWorkload("mixed-deploy", annotations, containers)

	if len(svcs) != 1 {
		t.Fatalf("expected 1 service (only ghcr.io inferable), got %d", len(svcs))
	}
	if svcs[0].Repo != "github.com/org/api" {
		t.Errorf("expected repo 'github.com/org/api', got %q", svcs[0].Repo)
	}
	if svcs[0].Registry != "ghcr.io/org/api" {
		t.Errorf("expected registry 'ghcr.io/org/api', got %q", svcs[0].Registry)
	}
}

func TestDiscoverFromWorkload_InferredWithAnnotationName(t *testing.T) {
	// When AnnotationRepo is absent but AnnotationName is set,
	// the inferred path should use AnnotationName as the service name.
	annotations := map[string]string{
		AnnotationName: "custom-service-name",
	}
	containers := []corev1.Container{
		{Image: "ghcr.io/org/myapp:v3.0"},
	}

	svcs := discoverFromWorkload("deploy-name", annotations, containers)

	if len(svcs) != 1 {
		t.Fatalf("expected 1 service, got %d", len(svcs))
	}
	if svcs[0].Name != "custom-service-name" {
		t.Errorf("expected name 'custom-service-name', got %q", svcs[0].Name)
	}
	if svcs[0].Repo != "github.com/org/myapp" {
		t.Errorf("expected inferred repo, got %q", svcs[0].Repo)
	}
}

func TestDiscoverFromWorkload_NilAnnotations(t *testing.T) {
	// nil annotations map should not panic; Go map lookups on nil return zero value.
	containers := []corev1.Container{
		{Image: "ghcr.io/org/service:v1"},
	}

	svcs := discoverFromWorkload("nil-ann", nil, containers)

	if len(svcs) != 1 {
		t.Fatalf("expected 1 service, got %d", len(svcs))
	}
	if svcs[0].Name != "nil-ann" {
		t.Errorf("expected name 'nil-ann', got %q", svcs[0].Name)
	}
}

func TestDiscoverFromWorkload_NoContainersNoAnnotation(t *testing.T) {
	svcs := discoverFromWorkload("empty", map[string]string{}, nil)

	if len(svcs) != 0 {
		t.Errorf("expected 0 services, got %d", len(svcs))
	}
}

func TestDiscoverFromWorkload_NilAnnotationsNilContainers(t *testing.T) {
	svcs := discoverFromWorkload("empty", nil, nil)

	if len(svcs) != 0 {
		t.Errorf("expected 0 services, got %d", len(svcs))
	}
}

func TestDiscoverFromWorkload_AnnotationRepoIgnoresContainers(t *testing.T) {
	// When AnnotationRepo is set, containers should be ignored entirely.
	annotations := map[string]string{
		AnnotationRepo: "github.com/org/explicit",
	}
	containers := []corev1.Container{
		{Image: "ghcr.io/org/frontend:v1"},
		{Image: "ghcr.io/org/backend:v2"},
	}

	svcs := discoverFromWorkload("workload", annotations, containers)

	if len(svcs) != 1 {
		t.Fatalf("expected 1 service from annotation (not containers), got %d", len(svcs))
	}
	if svcs[0].Repo != "github.com/org/explicit" {
		t.Errorf("expected annotated repo, got %q", svcs[0].Repo)
	}
	// Registry should not be set when using annotation path
	if svcs[0].Registry != "" {
		t.Errorf("expected empty registry for annotation path, got %q", svcs[0].Registry)
	}
}

func TestImageWithoutTag_AdditionalCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"digest with tag", "ghcr.io/org/app:v1@sha256:abc", "ghcr.io/org/app"},
		{"port only no tag", "localhost:5000/app", "localhost:5000/app"},
		{"port with nested path", "registry.example.com:5000/org/app:v1", "registry.example.com:5000/org/app"},
		{"just registry and image", "ghcr.io/app:latest", "ghcr.io/app"},
		{"digest only no tag", "ghcr.io/org/app@sha256:deadbeef", "ghcr.io/org/app"},
		{"multiple colons with port and tag", "myhost:5000/org/img:v2.1", "myhost:5000/org/img"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := imageWithoutTag(tt.input)
			if got != tt.want {
				t.Errorf("imageWithoutTag(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestInferRepoFromImage_AdditionalCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"single part", "nginx", ""},
		{"two parts no tag", "org/app", ""},
		{"ghcr no tag", "ghcr.io/org/app", "github.com/org/app"},
		{"docker.io library image", "docker.io/library/nginx:latest", "github.com/library/nginx"},
		{"gitlab with nested path", "registry.gitlab.com/group/project:v1", "gitlab.com/group/project"},
		{"ghcr with extra path segments", "ghcr.io/org/app/extra:v1", "github.com/org/app"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferRepoFromImage(tt.input)
			if got != tt.want {
				t.Errorf("inferRepoFromImage(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDiscoverFromWorkload_InferredWithEmptyAnnotationName(t *testing.T) {
	// AnnotationName present but empty string should fall back to workload name.
	annotations := map[string]string{
		AnnotationName: "",
	}
	containers := []corev1.Container{
		{Image: "ghcr.io/org/svc:v1"},
	}

	svcs := discoverFromWorkload("workload-name", annotations, containers)

	if len(svcs) != 1 {
		t.Fatalf("expected 1 service, got %d", len(svcs))
	}
	if svcs[0].Name != "workload-name" {
		t.Errorf("expected name 'workload-name' (empty annotation should not override), got %q", svcs[0].Name)
	}
}

func TestDiscoverFromWorkload_MultipleContainersWithGitLab(t *testing.T) {
	annotations := map[string]string{}
	containers := []corev1.Container{
		{Image: "registry.gitlab.com/team/api:v1.0"},
		{Image: "docker.io/org/worker:v2.0"},
	}

	svcs := discoverFromWorkload("multi-registry", annotations, containers)

	if len(svcs) != 2 {
		t.Fatalf("expected 2 services, got %d", len(svcs))
	}
	if svcs[0].Repo != "gitlab.com/team/api" {
		t.Errorf("expected gitlab repo, got %q", svcs[0].Repo)
	}
	if svcs[0].Registry != "registry.gitlab.com/team/api" {
		t.Errorf("expected gitlab registry, got %q", svcs[0].Registry)
	}
	if svcs[1].Repo != "github.com/org/worker" {
		t.Errorf("expected docker.io -> github.com repo, got %q", svcs[1].Repo)
	}
	if svcs[1].Registry != "docker.io/org/worker" {
		t.Errorf("expected docker.io registry, got %q", svcs[1].Registry)
	}
}
