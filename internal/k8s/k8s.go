// Package k8s provides Kubernetes integration for reading deployed versions.
package k8s

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// DeployedService represents a service running in Kubernetes.
type DeployedService struct {
	Name       string            `json:"name"`
	Namespace  string            `json:"namespace"`
	Kind       string            `json:"kind"`
	Images     []string          `json:"images"`
	Replicas   int32             `json:"replicas"`
	Ready      int32             `json:"ready"`
	Labels     map[string]string `json:"labels,omitempty"`
	AppVersion string            `json:"app_version,omitempty"`
}

// Client connects to Kubernetes clusters.
type Client struct {
	clientset *kubernetes.Clientset
	context   string
}

// New creates a K8s client. kubeconfig defaults to ~/.kube/config.
func New(kubeconfig, kctx string) (*Client, error) {
	if kubeconfig == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}

	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
	overrides := &clientcmd.ConfigOverrides{}
	if kctx != "" {
		overrides.CurrentContext = kctx
	}

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create k8s client: %w", err)
	}

	slog.Debug("k8s.connected", "context", kctx)
	return &Client{clientset: clientset, context: kctx}, nil
}

// Clientset returns the underlying Kubernetes clientset.
func (c *Client) Clientset() *kubernetes.Clientset {
	return c.clientset
}

// ListDeployments returns all deployments in a namespace (empty = all namespaces).
func (c *Client) ListDeployments(ctx context.Context, namespace string) ([]DeployedService, error) {
	slog.Debug("k8s.list_deployments", "namespace", namespace)

	deployments, err := c.clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}

	var services []DeployedService
	for _, d := range deployments.Items {
		images := extractImages(d.Spec.Template.Spec.Containers)
		var replicas int32
		if d.Spec.Replicas != nil {
			replicas = *d.Spec.Replicas
		}
		services = append(services, DeployedService{
			Name:       d.Name,
			Namespace:  d.Namespace,
			Kind:       "Deployment",
			Images:     images,
			Replicas:   replicas,
			Ready:      d.Status.ReadyReplicas,
			Labels:     d.Labels,
			AppVersion: extractVersion(images, d.Labels),
		})
	}

	slog.Debug("k8s.list_deployments.done", "namespace", namespace, "count", len(services))
	return services, nil
}

// ListStatefulSets returns all statefulsets in a namespace.
func (c *Client) ListStatefulSets(ctx context.Context, namespace string) ([]DeployedService, error) {
	sets, err := c.clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list statefulsets: %w", err)
	}

	var services []DeployedService
	for _, s := range sets.Items {
		images := extractImages(s.Spec.Template.Spec.Containers)
		var replicas int32
		if s.Spec.Replicas != nil {
			replicas = *s.Spec.Replicas
		}
		services = append(services, DeployedService{
			Name:       s.Name,
			Namespace:  s.Namespace,
			Kind:       "StatefulSet",
			Images:     images,
			Replicas:   replicas,
			Ready:      s.Status.ReadyReplicas,
			Labels:     s.Labels,
			AppVersion: extractVersion(images, s.Labels),
		})
	}

	return services, nil
}

// ListAll returns all deployments and statefulsets in a namespace.
func (c *Client) ListAll(ctx context.Context, namespace string) ([]DeployedService, error) {
	deps, err := c.ListDeployments(ctx, namespace)
	if err != nil {
		return nil, err
	}
	sets, err := c.ListStatefulSets(ctx, namespace)
	if err != nil {
		return nil, err
	}
	return append(deps, sets...), nil
}

// GetDeployment returns a specific deployment.
func (c *Client) GetDeployment(ctx context.Context, namespace, name string) (*DeployedService, error) {
	d, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get deployment %s/%s: %w", namespace, name, err)
	}

	images := extractImages(d.Spec.Template.Spec.Containers)
	var replicas int32
	if d.Spec.Replicas != nil {
		replicas = *d.Spec.Replicas
	}

	return &DeployedService{
		Name:       d.Name,
		Namespace:  d.Namespace,
		Kind:       "Deployment",
		Images:     images,
		Replicas:   replicas,
		Ready:      d.Status.ReadyReplicas,
		Labels:     d.Labels,
		AppVersion: extractVersion(images, d.Labels),
	}, nil
}

// extractImages gets image references from containers.
func extractImages(containers []corev1.Container) []string {
	images := make([]string, 0, len(containers))
	for _, c := range containers {
		images = append(images, c.Image)
	}
	return images
}

// extractVersion tries to determine the app version from image tags or labels.
func extractVersion(images []string, labels map[string]string) string {
	// Check common labels first
	for _, key := range []string{"app.kubernetes.io/version", "version", "app-version"} {
		if v, ok := labels[key]; ok && v != "" {
			return v
		}
	}

	// Extract version from image tag
	for _, img := range images {
		if idx := strings.LastIndex(img, ":"); idx != -1 {
			tag := img[idx+1:]
			if tag != "latest" && tag != "" {
				return tag
			}
		}
	}

	return ""
}
