// Package main is the entry point for the releasewave CLI.
//
// Go learning notes:
//   - main() in package "main" is where Go programs start (like Java's public static void main)
//   - os.Args contains command-line arguments
//   - os.Exit(1) exits with error code 1
//   - fmt.Fprintf(os.Stderr, ...) writes to stderr (for errors)
//   - switch/case doesn't need "break" in Go — it stops automatically (use "fallthrough" if you want C behavior)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/UnityInFlow/releasewave/internal/config"
	"github.com/UnityInFlow/releasewave/internal/mcpserver"
	gh "github.com/UnityInFlow/releasewave/internal/provider/github"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		cmdServe()
	case "releases":
		cmdReleases()
	case "latest":
		cmdLatest()
	case "tags":
		cmdTags()
	case "check":
		cmdCheck()
	case "version":
		fmt.Printf("releasewave %s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`releasewave - Universal release/version aggregator for microservices

Usage:
  releasewave <command> [arguments]

Commands:
  serve                    Start the MCP server (for AI agent integration)
  releases <owner/repo>    List releases for a GitHub repository
  latest   <owner/repo>    Show the latest release
  tags     <owner/repo>    List tags for a repository
  check                    Check all configured services for outdated versions
  version                  Print version

Examples:
  releasewave serve
  releasewave releases golang/go
  releasewave latest kubernetes/kubernetes
  releasewave tags docker/compose
  releasewave check

Configuration:
  ~/.config/releasewave/config.yaml`)
}

// cmdServe starts the MCP server.
//
// GO LEARNING: Signal Handling & Graceful Shutdown
//   When you press Ctrl+C, the OS sends SIGINT to your process.
//   We catch this signal to shut down the server gracefully (finish
//   ongoing requests, close connections) instead of just dying.
//
//   signal.Notify(ch, signals...) sends signals to a channel.
//   We use a goroutine (go func(){...}()) to listen for the signal
//   in the background while the server runs in the foreground.
func cmdServe() {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	// Determine port
	port := cfg.Server.Port
	if port == 0 {
		port = 7891
	}
	addr := fmt.Sprintf(":%d", port)

	srv := mcpserver.New(cfg)

	// GO LEARNING: Goroutine for Signal Handling
	//   go func() { ... }() starts a function in a new goroutine (lightweight thread).
	//   This runs concurrently with the main goroutine.
	//
	//   make(chan os.Signal, 1) creates a buffered channel that can hold 1 signal.
	//   Channels are Go's way of communicating between goroutines.
	//   <-sigCh blocks until a value is received on the channel.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh // Block until we receive a signal
		fmt.Printf("\nReceived %s, shutting down...\n", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
		os.Exit(0)
	}()

	// This blocks until the server stops
	if err := srv.Start(addr); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

// parseOwnerRepo splits "owner/repo" into two strings.
func parseOwnerRepo(arg string) (string, string) {
	parts := strings.SplitN(arg, "/", 2)
	if len(parts) != 2 {
		fmt.Fprintf(os.Stderr, "error: expected owner/repo format, got %q\n", arg)
		os.Exit(1)
	}
	return parts[0], parts[1]
}

// getClient creates a GitHub client, optionally using a token from config or env.
func getClient() *gh.Client {
	// First check environment variable
	token := os.Getenv("GITHUB_TOKEN")

	// Then check config file
	if token == "" {
		cfg, err := config.Load("")
		if err == nil && cfg.Tokens.GitHub != "" {
			token = cfg.Tokens.GitHub
		}
	}

	return gh.New(token)
}

func cmdReleases() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: releasewave releases <owner/repo>")
		os.Exit(1)
	}

	owner, repo := parseOwnerRepo(os.Args[2])
	client := getClient()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	releases, err := client.ListReleases(ctx, owner, repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(releases) == 0 {
		fmt.Println("No releases found.")
		return
	}

	// Check for --json flag
	if hasFlag("--json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(releases)
		return
	}

	// Pretty table output using tabwriter
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "TAG\tNAME\tDATE\tPRE-RELEASE\n")
	fmt.Fprintf(w, "---\t----\t----\t-----------\n")
	for _, r := range releases {
		pre := ""
		if r.Prerelease {
			pre = "yes"
		}
		date := r.PublishedAt.Format("2006-01-02")
		name := r.Name
		if len(name) > 50 {
			name = name[:47] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Tag, name, date, pre)
	}
	w.Flush()
}

func cmdLatest() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: releasewave latest <owner/repo>")
		os.Exit(1)
	}

	owner, repo := parseOwnerRepo(os.Args[2])
	client := getClient()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	release, err := client.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if hasFlag("--json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(release)
		return
	}

	fmt.Printf("Repository:  %s/%s\n", owner, repo)
	fmt.Printf("Latest:      %s\n", release.Tag)
	fmt.Printf("Name:        %s\n", release.Name)
	fmt.Printf("Published:   %s\n", release.PublishedAt.Format("2006-01-02 15:04"))
	fmt.Printf("URL:         %s\n", release.HTMLURL)
	if release.Prerelease {
		fmt.Printf("Pre-release: yes\n")
	}
	if release.Body != "" {
		body := release.Body
		if len(body) > 500 {
			body = body[:497] + "..."
		}
		fmt.Printf("\n--- Release Notes ---\n%s\n", body)
	}
}

func cmdTags() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: releasewave tags <owner/repo>")
		os.Exit(1)
	}

	owner, repo := parseOwnerRepo(os.Args[2])
	client := getClient()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tags, err := client.ListTags(ctx, owner, repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(tags) == 0 {
		fmt.Println("No tags found.")
		return
	}

	if hasFlag("--json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(tags)
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "TAG\tCOMMIT\n")
	fmt.Fprintf(w, "---\t------\n")
	for _, t := range tags {
		sha := t.Commit
		if len(sha) > 8 {
			sha = sha[:8]
		}
		fmt.Fprintf(w, "%s\t%s\n", t.Name, sha)
	}
	w.Flush()
}

func cmdCheck() {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	if len(cfg.Services) == 0 {
		fmt.Println("No services configured.")
		fmt.Println("Add services to ~/.config/releasewave/config.yaml")
		fmt.Println()
		fmt.Println("Example:")
		fmt.Println("  services:")
		fmt.Println("    - name: my-service")
		fmt.Println("      repo: github.com/org/my-service")
		return
	}

	client := getClient()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "SERVICE\tPLATFORM\tLATEST\tURL\n")
	fmt.Fprintf(w, "-------\t--------\t------\t---\n")

	for _, svc := range cfg.Services {
		parsed, err := config.ParseRepo(svc.Repo)
		if err != nil {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", svc.Name, "?", "error: "+err.Error(), "")
			continue
		}

		if parsed.Platform != "github" {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", svc.Name, parsed.Platform, "not yet supported", "")
			continue
		}

		release, err := client.GetLatestRelease(ctx, parsed.Owner, parsed.RepoName)
		if err != nil {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", svc.Name, parsed.Platform, "error: "+err.Error(), "")
			continue
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", svc.Name, parsed.Platform, release.Tag, release.HTMLURL)
	}
	w.Flush()
}

func hasFlag(flag string) bool {
	for _, arg := range os.Args {
		if arg == flag {
			return true
		}
	}
	return false
}
