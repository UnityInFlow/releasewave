// Package discovery provides auto-discovery of services from Kubernetes clusters.
package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/UnityInFlow/releasewave/internal/config"
	"github.com/UnityInFlow/releasewave/internal/k8s"
)

const (
	// AnnotationRepo is the annotation key for the source repository.
	AnnotationRepo = "releasewave.io/repo"
	// AnnotationName is the annotation key for the service display name.
	AnnotationName = "releasewave.io/name"
)

// Discoverer finds services from an external source and returns config entries.
type Discoverer interface {
	Discover(ctx context.Context) ([]config.ServiceConfig, error)
}

// K8sDiscoverer discovers services from Kubernetes deployments and statefulsets.
type K8sDiscoverer struct {
	kubeconfig string
	context    string
	namespace  string
}

// NewK8sDiscoverer creates a Kubernetes service discoverer.
func NewK8sDiscoverer(kubeconfig, kctx, namespace string) *K8sDiscoverer {
	return &K8sDiscoverer{
		kubeconfig: kubeconfig,
		context:    kctx,
		namespace:  namespace,
	}
}

// Discover scans Kubernetes deployments and statefulsets for service configs.
// It looks for releasewave.io/repo and releasewave.io/name annotations first,
// then falls back to inferring the repository from the container image name.
func (d *K8sDiscoverer) Discover(ctx context.Context) ([]config.ServiceConfig, error) {
	client, err := k8s.New(d.kubeconfig, d.context)
	if err != nil {
		return nil, fmt.Errorf("connect to k8s: %w", err)
	}

	slog.Info("discovery.start", "namespace", d.namespace)

	var services []config.ServiceConfig
	seen := make(map[string]bool)

	// Scan deployments
	deps, err := client.Clientset().AppsV1().Deployments(d.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	for _, dep := range deps.Items {
		svcs := discoverFromWorkload(dep.Name, dep.Annotations, dep.Spec.Template.Spec.Containers)
		for _, svc := range svcs {
			if !seen[svc.Name] {
				services = append(services, svc)
				seen[svc.Name] = true
			}
		}
	}

	// Scan statefulsets
	sets, err := client.Clientset().AppsV1().StatefulSets(d.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list statefulsets: %w", err)
	}
	for _, ss := range sets.Items {
		svcs := discoverFromWorkload(ss.Name, ss.Annotations, ss.Spec.Template.Spec.Containers)
		for _, svc := range svcs {
			if !seen[svc.Name] {
				services = append(services, svc)
				seen[svc.Name] = true
			}
		}
	}

	slog.Info("discovery.done", "discovered", len(services))
	return services, nil
}

// discoverFromWorkload extracts service configs from a workload's annotations and containers.
func discoverFromWorkload(name string, annotations map[string]string, containers []corev1.Container) []config.ServiceConfig {
	// Check for explicit annotations first
	if repo, ok := annotations[AnnotationRepo]; ok && repo != "" {
		svcName := name
		if n, ok := annotations[AnnotationName]; ok && n != "" {
			svcName = n
		}
		slog.Debug("discovery.annotation", "name", svcName, "repo", repo)
		return []config.ServiceConfig{
			{Name: svcName, Repo: repo},
		}
	}

	// Fall back to inferring from image names
	var services []config.ServiceConfig
	for _, c := range containers {
		repo := inferRepoFromImage(c.Image)
		if repo == "" {
			continue
		}
		svcName := name
		if n, ok := annotations[AnnotationName]; ok && n != "" {
			svcName = n
		}
		slog.Debug("discovery.inferred", "name", svcName, "repo", repo, "image", c.Image)
		services = append(services, config.ServiceConfig{
			Name:     svcName,
			Repo:     repo,
			Registry: imageWithoutTag(c.Image),
		})
	}
	return services
}

// inferRepoFromImage tries to convert a container image reference to a source repo.
// For example: ghcr.io/org/app:v1.0 -> github.com/org/app
func inferRepoFromImage(image string) string {
	// Strip tag or digest
	ref := imageWithoutTag(image)

	parts := strings.Split(ref, "/")
	if len(parts) < 3 {
		return ""
	}

	registry := parts[0]
	owner := parts[1]
	repoName := parts[2]

	switch {
	case strings.Contains(registry, "ghcr.io"):
		return fmt.Sprintf("github.com/%s/%s", owner, repoName)
	case strings.Contains(registry, "gitlab"):
		return fmt.Sprintf("gitlab.com/%s/%s", owner, repoName)
	case strings.Contains(registry, "docker.io"):
		return fmt.Sprintf("github.com/%s/%s", owner, repoName)
	default:
		return ""
	}
}

// imageWithoutTag strips the tag or digest from an image reference.
func imageWithoutTag(image string) string {
	// Strip digest
	if idx := strings.Index(image, "@"); idx != -1 {
		image = image[:idx]
	}
	// Strip tag
	if idx := strings.LastIndex(image, ":"); idx != -1 {
		// Make sure it's a tag, not a port
		afterColon := image[idx+1:]
		if !strings.Contains(afterColon, "/") {
			image = image[:idx]
		}
	}
	return image
}
