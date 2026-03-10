package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/UnityInFlow/releasewave/internal/mcpserver"
	"github.com/UnityInFlow/releasewave/internal/web"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the MCP server",
	Long:  "Start the ReleaseWave MCP server. Default transport is stdio (for Claude Code, Cursor, etc.).\nUse --transport=sse for HTTP+SSE mode.",
	RunE: func(cmd *cobra.Command, args []string) error {
		transport, _ := cmd.Flags().GetString("transport")
		port, _ := cmd.Flags().GetInt("port")

		srv := mcpserver.New(cfg, version)

		switch transport {
		case "stdio":
			return srv.ServeStdio()

		case "sse":
			if port == 0 {
				port = cfg.Server.Port
			}
			addr := fmt.Sprintf(":%d", port)

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			go func() {
				sig := <-sigCh
				slog.Info("server.shutdown", "signal", sig.String())
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = srv.Shutdown(ctx)
				os.Exit(0)
			}()

			fmt.Fprintln(os.Stderr, srv.Info())
			fmt.Fprintf(os.Stderr, "Dashboard: http://localhost%s/dashboard\n", addr)

			// Serve the web dashboard on the same port alongside the MCP SSE endpoints.
			dashboard := web.Handler(srv.Config(), srv.Providers())
			return srv.StartWithHandlers(addr, map[string]http.Handler{
				"/dashboard": dashboard,
			})

		default:
			return fmt.Errorf("unknown transport %q (supported: stdio, sse)", transport)
		}
	},
}

func init() {
	serveCmd.Flags().String("transport", "stdio", "transport mode (stdio, sse)")
	serveCmd.Flags().Int("port", 0, "port for SSE transport (default from config)")
	rootCmd.AddCommand(serveCmd)
}
