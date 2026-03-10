package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/UnityInFlow/releasewave/internal/config"
	"github.com/UnityInFlow/releasewave/internal/discovery"
)

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Auto-discover services from a Kubernetes cluster",
	Long:  "Scans Kubernetes deployments and statefulsets for releasewave.io annotations or infers repos from container images.",
	RunE: func(cmd *cobra.Command, args []string) error {
		namespace, _ := cmd.Flags().GetString("namespace")
		kubeconfig, _ := cmd.Flags().GetString("kubeconfig")
		kctx, _ := cmd.Flags().GetString("context")
		merge, _ := cmd.Flags().GetBool("merge")
		jsonFlag, _ := cmd.Flags().GetBool("json")

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		discoverer := discovery.NewK8sDiscoverer(kubeconfig, kctx, namespace)
		services, err := discoverer.Discover(ctx)
		if err != nil {
			return fmt.Errorf("discover services: %w", err)
		}

		if len(services) == 0 {
			fmt.Println("No services discovered.")
			return nil
		}

		if jsonFlag {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(services)
		}

		// Print discovered services
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "NAME\tREPO\tREGISTRY\n")
		fmt.Fprintf(w, "----\t----\t--------\n")
		for _, svc := range services {
			fmt.Fprintf(w, "%s\t%s\t%s\n", svc.Name, svc.Repo, svc.Registry)
		}
		if err := w.Flush(); err != nil {
			return err
		}

		fmt.Printf("\nDiscovered %d service(s)\n", len(services))

		if merge {
			return mergeIntoConfig(services)
		}

		return nil
	},
}

// mergeIntoConfig adds discovered services to the config file without duplicating existing ones.
func mergeIntoConfig(discovered []config.ServiceConfig) error {
	cfgPath := cfgFile
	if cfgPath == "" {
		var err error
		cfgPath, err = config.DefaultConfigPath()
		if err != nil {
			return err
		}
	}

	// Read existing config
	data, err := os.ReadFile(cfgPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read config: %w", err)
	}

	var existingCfg config.Config
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &existingCfg); err != nil {
			return fmt.Errorf("parse config: %w", err)
		}
	}

	// Build set of existing service names
	existing := make(map[string]bool)
	for _, svc := range existingCfg.Services {
		existing[svc.Name] = true
	}

	// Merge new services
	added := 0
	for _, svc := range discovered {
		if !existing[svc.Name] {
			existingCfg.Services = append(existingCfg.Services, svc)
			existing[svc.Name] = true
			added++
		}
	}

	if added == 0 {
		fmt.Println("\nAll discovered services already exist in config.")
		return nil
	}

	// Write back
	out, err := yaml.Marshal(&existingCfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(cfgPath, out, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Printf("\nMerged %d new service(s) into %s\n", added, cfgPath)
	return nil
}

func init() {
	discoverCmd.Flags().String("namespace", "", "Kubernetes namespace (empty for all namespaces)")
	discoverCmd.Flags().String("kubeconfig", "", "path to kubeconfig file (default: ~/.kube/config)")
	discoverCmd.Flags().String("context", "", "Kubernetes context to use")
	discoverCmd.Flags().Bool("merge", false, "merge discovered services into config file")
	discoverCmd.Flags().Bool("json", false, "output as JSON")
	rootCmd.AddCommand(discoverCmd)
}
