package cloudflare

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"testing/synctest"
	"time"
)

func TestCacheConcurrentKeysAndCanceledWaiter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newClient("fixture", "", &http.Client{})
		defer c.Close()
		release := make(chan struct{})
		calls := 0
		fetch := func(ctx context.Context) (any, error) {
			calls++
			select {
			case <-release:
				return []string{"ok"}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		ctx, cancel := context.WithCancel(context.Background())
		first, second := make(chan error, 1), make(chan error, 1)
		go func() { var out []string; _, err := c.cached(ctx, "a", resultTTL, fetch, &out); first <- err }()
		synctest.Wait()
		go func() {
			var out []string
			_, err := c.cached(context.Background(), "a", resultTTL, fetch, &out)
			second <- err
		}()
		synctest.Wait()
		// A blocked lookup must not hold a mutex across a different key's I/O.
		var out []string
		_, err := c.cached(context.Background(), "b", metadataTTL, func(context.Context) (any, error) { return []string{"other"}, nil }, &out)
		if err != nil || out[0] != "other" {
			t.Fatal("unrelated lookup blocked or failed", err)
		}
		cancel()
		if err := <-first; !errors.Is(err, context.Canceled) {
			t.Fatal("waiter did not cancel", err)
		}
		close(release)
		if err := <-second; err != nil {
			t.Fatal("one canceled caller broke another", err)
		}
		reused, err := c.cached(context.Background(), "a", resultTTL, fetch, &out)
		if err != nil || !reused || calls != 1 {
			t.Fatalf("dedup/cache: reused=%v calls=%d err=%v", reused, calls, err)
		}
	})
}

func TestCacheExpiryIsolationErrorsAndBypass(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newClient("fixture", "", &http.Client{})
		defer c.Close()
		calls := 0
		fetch := func(context.Context) (any, error) {
			calls++
			return []Row{{Count: float64(calls), Dimensions: map[string]any{"host": "example.com"}}}, nil
		}
		var rows []Row
		ctx := context.Background()
		reused, err := c.cached(ctx, "rows", resultTTL, fetch, &rows)
		if err != nil || reused {
			t.Fatal(err)
		}
		rows[0].Dimensions["host"] = "mutated"
		reused, err = c.cached(ctx, "rows", resultTTL, fetch, &rows)
		if err != nil || !reused || calls != 1 || rows[0].Dimensions["host"] != "example.com" {
			t.Fatal("cached data was not isolated", err)
		}
		time.Sleep(resultTTL + time.Second)
		reused, err = c.cached(ctx, "rows", resultTTL, fetch, &rows)
		if err != nil || reused || calls != 2 {
			t.Fatal("expired response reused", err)
		}
		for range 2 {
			if hit, err := c.cached(WithoutCache(ctx), "rows", resultTTL, fetch, &rows); err != nil || hit {
				t.Fatal("health check cached", err)
			}
		}
		if calls != 4 {
			t.Fatal("health checks skipped upstream")
		}
		failures := 0
		fail := func(context.Context) (any, error) { failures++; return nil, errors.New("denied") }
		for range 2 {
			if _, err := c.cached(ctx, "error", resultTTL, fail, &rows); err == nil {
				t.Fatal("missing error")
			}
		}
		if failures != 2 {
			t.Fatal("errors cached")
		}
		c2 := newClient("different-token", "", &http.Client{})
		defer c2.Close()
		if hit, err := c2.cached(ctx, "rows", resultTTL, fetch, &rows); err != nil || hit {
			t.Fatal("cross-client cache leak", err)
		}
	})
}

func TestCacheCancelsAbandonedWorkAndDisposal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := newClient("fixture", "", &http.Client{})
		ctx, cancel := context.WithCancel(context.Background())
		upstreamStopped := make(chan struct{})
		done := make(chan error, 1)
		fetch := func(ctx context.Context) (any, error) { <-ctx.Done(); close(upstreamStopped); return nil, ctx.Err() }
		go func() { var out any; _, err := c.cached(ctx, "a", resultTTL, fetch, &out); done <- err }()
		synctest.Wait()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		<-upstreamStopped
		synctest.Wait()
		if len(c.cache.entries) != 0 || len(c.cache.pending) != 0 {
			t.Fatal("abandoned request retained")
		}
		go func() {
			var out any
			_, err := c.cached(context.Background(), "b", resultTTL, func(ctx context.Context) (any, error) { <-ctx.Done(); return nil, ctx.Err() }, &out)
			done <- err
		}()
		synctest.Wait()
		c.Close()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal("dispose did not stop work", err)
		}
	})
}

func TestCacheBounds(t *testing.T) {
	c := responseCache{entries: map[string]cacheEntry{}}
	for i := range maxCacheEntries + 1 {
		c.put(fmt.Sprint(i), []byte("x"), resultTTL)
	}
	if len(c.entries) != maxCacheEntries || c.bytes != maxCacheEntries {
		t.Fatal("entry cap failed")
	}
	c.put("large", make([]byte, maxCacheBytes), resultTTL)
	if len(c.entries) != 1 || c.bytes != maxCacheBytes {
		t.Fatal("byte cap failed")
	}
	c.put("too-large", make([]byte, maxCacheBytes+1), resultTTL)
	if len(c.entries) != 1 || c.bytes > maxCacheBytes {
		t.Fatal("oversized response retained")
	}
}
