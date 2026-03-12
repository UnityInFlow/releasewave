package main

import (
	"database/sql"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/UnityInFlow/releasewave/internal/tenant"

	_ "modernc.org/sqlite"
)

func openTenantDB() (*sql.DB, error) {
	if cfg.Storage.Path == "" {
		return nil, fmt.Errorf("storage.path not configured — set it in config to use tenants")
	}
	return sql.Open("sqlite", cfg.Storage.Path)
}

var tenantCmd = &cobra.Command{
	Use:   "tenant",
	Short: "Manage tenants",
}

var tenantCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new tenant",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := openTenantDB()
		if err != nil {
			return err
		}
		defer db.Close()

		ts, err := tenant.NewStore(db)
		if err != nil {
			return err
		}

		t, err := ts.Create(args[0])
		if err != nil {
			return fmt.Errorf("create tenant: %w", err)
		}

		// Generate an API key for the new tenant.
		ks, err := tenant.NewKeyStore(db)
		if err != nil {
			return err
		}

		rawKey, _, err := ks.Generate(t.ID)
		if err != nil {
			return fmt.Errorf("generate API key: %w", err)
		}

		fmt.Printf("Tenant created:\n")
		fmt.Printf("  Name:    %s\n", t.Name)
		fmt.Printf("  API Key: %s\n", rawKey)
		fmt.Printf("\nSave this API key — it won't be shown again.\n")
		return nil
	},
}

var tenantListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tenants",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := openTenantDB()
		if err != nil {
			return err
		}
		defer db.Close()

		ts, err := tenant.NewStore(db)
		if err != nil {
			return err
		}

		tenants, err := ts.List()
		if err != nil {
			return err
		}

		if len(tenants) == 0 {
			fmt.Println("No tenants.")
			return nil
		}

		fmt.Printf("%-20s %s\n", "NAME", "CREATED")
		for _, t := range tenants {
			fmt.Printf("%-20s %s\n", t.Name, t.CreatedAt.Format("2006-01-02"))
		}
		return nil
	},
}

var tenantDeleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete a tenant",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := openTenantDB()
		if err != nil {
			return err
		}
		defer db.Close()

		ts, err := tenant.NewStore(db)
		if err != nil {
			return err
		}

		if err := ts.Delete(args[0]); err != nil {
			return err
		}
		fmt.Printf("Tenant %q deleted.\n", args[0])
		return nil
	},
}

func init() {
	tenantCmd.AddCommand(tenantCreateCmd)
	tenantCmd.AddCommand(tenantListCmd)
	tenantCmd.AddCommand(tenantDeleteCmd)
	rootCmd.AddCommand(tenantCmd)
}
