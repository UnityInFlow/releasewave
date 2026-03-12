// Package daemon provides a background polling loop for watching releases.
package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/UnityInFlow/releasewave/internal/config"
	"github.com/UnityInFlow/releasewave/internal/notify"
	"github.com/UnityInFlow/releasewave/internal/provider"
	"github.com/UnityInFlow/releasewave/internal/store"
)

// Daemon polls configured services for new releases at a regular interval.
type Daemon struct {
	cfg       *config.Config
	providers map[string]provider.Provider
	notifier  notify.Notifier
	store     *store.Store
	interval  time.Duration
	known     map[string]string
	mu        sync.Mutex
	stopCh    chan struct{}
	stopped   chan struct{}
}

// New creates a new Daemon.
func New(cfg *config.Config, providers map[string]provider.Provider, notifier notify.Notifier, st *store.Store, interval time.Duration) *Daemon {
	return &Daemon{
		cfg:       cfg,
		providers: providers,
		notifier:  notifier,
		store:     st,
		interval:  interval,
		known:     make(map[string]string),
		stopCh:    make(chan struct{}),
		stopped:   make(chan struct{}),
	}
}

// Start begins the polling loop. It blocks until Stop is called or ctx is cancelled.
func (d *Daemon) Start(ctx context.Context) {
	defer close(d.stopped)

	slog.Info("daemon.start", "interval", d.interval, "services", len(d.cfg.Services))

	// Load known versions from store if available.
	if d.store != nil {
		for _, svc := range d.cfg.Services {
			if val, found, err := d.store.GetKV("version:" + svc.Name); err == nil && found {
				d.known[svc.Name] = val
			}
		}
	}

	// Run first check immediately.
	d.poll(ctx)

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.poll(ctx)
		case <-d.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// Stop signals the daemon to stop.
func (d *Daemon) Stop() {
	close(d.stopCh)
	<-d.stopped
}

// RunOnce executes a single poll cycle.
func (d *Daemon) RunOnce(ctx context.Context) {
	d.poll(ctx)
}

func (d *Daemon) poll(ctx context.Context) {
	slog.Debug("daemon.poll", "services", len(d.cfg.Services))

	var wg sync.WaitGroup
	for _, svc := range d.cfg.Services {
		wg.Add(1)
		go func(svc config.ServiceConfig) {
			defer wg.Done()
			d.checkService(ctx, svc)
		}(svc)
	}
	wg.Wait()
}

func (d *Daemon) checkService(ctx context.Context, svc config.ServiceConfig) {
	parsed, err := config.ParseRepo(svc.Repo)
	if err != nil {
		slog.Error("daemon.parse", "service", svc.Name, "error", err)
		return
	}

	p, ok := d.providers[parsed.Platform]
	if !ok {
		return
	}

	release, err := p.GetLatestRelease(ctx, parsed.Owner, parsed.RepoName)
	if err != nil {
		slog.Error("daemon.check", "service", svc.Name, "error", err)
		return
	}

	d.mu.Lock()
	old, seen := d.known[svc.Name]
	d.known[svc.Name] = release.Tag
	d.mu.Unlock()

	// Persist to store.
	if d.store != nil {
		_ = d.store.SetKV("version:"+svc.Name, release.Tag)
		_ = d.store.RecordRelease(store.Release{
			Service:      svc.Name,
			Tag:          release.Tag,
			Platform:     parsed.Platform,
			URL:          release.HTMLURL,
			PublishedAt:  release.PublishedAt,
			DiscoveredAt: time.Now(),
		})
	}

	isNew := seen && old != release.Tag
	if isNew {
		slog.Info("daemon.new_release", "service", svc.Name, "old", old, "new", release.Tag)
		fmt.Printf("[NEW] %s  %s  %s → %s  %s\n",
			time.Now().Format("15:04:05"),
			svc.Name,
			old,
			release.Tag,
			release.HTMLURL,
		)

		if d.notifier != nil {
			event := notify.Event{
				ServiceName: svc.Name,
				OldVersion:  old,
				NewVersion:  release.Tag,
				ReleaseURL:  release.HTMLURL,
				Platform:    parsed.Platform,
			}
			if err := d.notifier.Notify(ctx, event); err != nil {
				slog.Error("daemon.notify", "service", svc.Name, "error", err)
			}
		}
	}
}
