package provider

import (
	"context"

	"github.com/UnityInFlow/releasewave/internal/cache"
	"github.com/UnityInFlow/releasewave/internal/model"
)

// CachedProvider wraps a Provider and caches results.
type CachedProvider struct {
	inner Provider
	cache *cache.Cache
}

// NewCachedProvider wraps a provider with caching.
func NewCachedProvider(inner Provider, c *cache.Cache) *CachedProvider {
	return &CachedProvider{inner: inner, cache: c}
}

func (p *CachedProvider) Name() string { return p.inner.Name() }

func (p *CachedProvider) ListReleases(ctx context.Context, owner, repo string) ([]model.Release, error) {
	key := cache.Key(p.inner.Name(), "releases", owner, repo)
	if v, ok := p.cache.Get(key); ok {
		return v.([]model.Release), nil
	}
	releases, err := p.inner.ListReleases(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	p.cache.Set(key, releases)
	return releases, nil
}

func (p *CachedProvider) GetLatestRelease(ctx context.Context, owner, repo string) (*model.Release, error) {
	key := cache.Key(p.inner.Name(), "latest", owner, repo)
	if v, ok := p.cache.Get(key); ok {
		return v.(*model.Release), nil
	}
	release, err := p.inner.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	p.cache.Set(key, release)
	return release, nil
}

func (p *CachedProvider) ListTags(ctx context.Context, owner, repo string) ([]model.Tag, error) {
	key := cache.Key(p.inner.Name(), "tags", owner, repo)
	if v, ok := p.cache.Get(key); ok {
		return v.([]model.Tag), nil
	}
	tags, err := p.inner.ListTags(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	p.cache.Set(key, tags)
	return tags, nil
}
