package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

type mcpConfig struct {
	MCPServers map[string]mcpServerEntry `json:"mcpServers"`
}

type mcpServerEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Auto-configure MCP clients (Claude Code, Cursor, VS Code)",
	Long:  "Writes MCP server configuration to supported AI tools.\nDetects which tools are installed and configures them automatically.",
	RunE: func(cmd *cobra.Command, args []string) error {
		target, _ := cmd.Flags().GetString("target")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolve executable path: %w", err)
		}

		// Resolve symlinks to get the real path
		exe, err = filepath.EvalSymlinks(exe)
		if err != nil {
			return fmt.Errorf("resolve symlinks: %w", err)
		}

		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home dir: %w", err)
		}

		targets := map[string]string{
			"claude": filepath.Join(home, ".claude", "claude_desktop_config.json"),
			"cursor": filepath.Join(home, ".cursor", "mcp.json"),
			"vscode": filepath.Join(home, ".vscode", "mcp.json"),
		}

		installed := 0

		for name, path := range targets {
			if target != "all" && target != name {
				continue
			}

			// Check if the parent directory exists (tool is installed)
			dir := filepath.Dir(path)
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				if target == name {
					return fmt.Errorf("%s config directory not found at %s", name, dir)
				}
				continue // Skip if not installed and target is "all"
			}

			if dryRun {
				fmt.Printf("[dry-run] Would configure %s at %s\n", name, path)
				installed++
				continue
			}

			if err := writeMCPConfig(path, exe); err != nil {
				return fmt.Errorf("configure %s: %w", name, err)
			}

			fmt.Printf("Configured %s at %s\n", name, path)
			installed++
		}

		if installed == 0 {
			fmt.Println("No supported MCP clients found. Supported: claude, cursor, vscode")
		} else if !dryRun {
			fmt.Printf("\nReleaseWave is ready. Restart your AI tool to load the new MCP server.\n")
		}

		return nil
	},
}

// writeMCPConfig reads an existing config, merges the releasewave entry, and writes back.
func writeMCPConfig(path, exe string) error {
	var mcpCfg mcpConfig

	// Read existing config if it exists
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &mcpCfg); err != nil {
			// If the file exists but isn't valid JSON, start fresh
			mcpCfg = mcpConfig{}
		}
	}

	if mcpCfg.MCPServers == nil {
		mcpCfg.MCPServers = make(map[string]mcpServerEntry)
	}

	mcpCfg.MCPServers["releasewave"] = mcpServerEntry{
		Command: exe,
		Args:    []string{"serve"},
	}

	out, err := json.MarshalIndent(mcpCfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	return os.WriteFile(path, out, 0o644)
}

func init() {
	installCmd.Flags().String("target", "all", "target tool (claude, cursor, vscode, all)")
	installCmd.Flags().Bool("dry-run", false, "show what would be configured without writing")
	rootCmd.AddCommand(installCmd)
}
