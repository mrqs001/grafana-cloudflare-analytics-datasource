package cloudflare

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

const resultTTL = 30 * time.Second
const metadataTTL = 5 * time.Minute
const maxCacheEntries = 128
const maxCacheBytes = 32 << 20

type cacheEntry struct {
	value   []byte
	expires time.Time
	used    time.Time
}
type flight struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	value   []byte
	err     error
}
type responseCache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
	pending map[string]*flight
	bytes   int
	closed  bool
}

type bypassCacheKey struct{}

// WithoutCache makes health checks verify current upstream access.
func WithoutCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, bypassCacheKey{}, true)
}

// cached shares only identical requests, scoped to this client/token. JSON keeps
// cached responses immutable: every caller decodes its own maps and slices.
// A canceled caller stops waiting without aborting other callers' work.
func (c *Client) cached(ctx context.Context, key string, ttl time.Duration, fetch func(context.Context) (any, error), out any) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if ctx.Value(bypassCacheKey{}) == true {
		v, err := fetch(ctx)
		if err != nil {
			return false, err
		}
		b, err := json.Marshal(v)
		if err != nil {
			return false, err
		}
		return false, json.Unmarshal(b, out)
	}
	cache := &c.cache
	cache.mu.Lock()
	if cache.closed {
		cache.mu.Unlock()
		return false, context.Canceled
	}
	now := time.Now()
	if e, ok := cache.entries[key]; ok && now.Before(e.expires) {
		e.used = now
		cache.entries[key] = e
		cache.mu.Unlock()
		return true, json.Unmarshal(e.value, out)
	}
	f, shared := cache.pending[key]
	if !shared {
		// Bound shared work even if callers have no deadline. Caller cancellation
		// below also cancels upstream as soon as the last waiter leaves.
		workCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 90*time.Second)
		f = &flight{done: make(chan struct{}), cancel: cancel}
		cache.pending[key] = f
		go func() {
			defer cancel()
			value, err := fetch(workCtx)
			var b []byte
			if err == nil {
				b, err = json.Marshal(value)
			}
			cache.mu.Lock()
			defer cache.mu.Unlock()
			f.value, f.err = b, err
			if cache.pending[key] == f {
				delete(cache.pending, key)
				if err == nil && workCtx.Err() == nil && !cache.closed {
					cache.put(key, b, ttl)
				}
			}
			close(f.done)
		}()
	}
	f.waiters++
	cache.mu.Unlock()
	defer func() {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		f.waiters--
		if f.waiters == 0 {
			f.cancel()
			if cache.pending[key] == f {
				delete(cache.pending, key)
			}
		}
	}()
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-f.done:
		if f.err != nil {
			return shared, f.err
		}
		return shared, json.Unmarshal(f.value, out)
	}
}

// put is called under mu. Evict expired entries first, then least recently used.
func (c *responseCache) put(key string, value []byte, ttl time.Duration) {
	now := time.Now()
	for k, e := range c.entries {
		if k == key || !now.Before(e.expires) {
			c.bytes -= len(e.value)
			delete(c.entries, k)
		}
	}
	if len(value) > maxCacheBytes {
		return
	}
	for len(c.entries) >= maxCacheEntries || c.bytes+len(value) > maxCacheBytes {
		oldestKey := ""
		var oldest time.Time
		for k, e := range c.entries {
			if oldest.IsZero() || e.used.Before(oldest) {
				oldest, oldestKey = e.used, k
			}
		}
		c.bytes -= len(c.entries[oldestKey].value)
		delete(c.entries, oldestKey)
	}
	c.entries[key] = cacheEntry{value: value, expires: now.Add(ttl), used: now}
	c.bytes += len(value)
}
