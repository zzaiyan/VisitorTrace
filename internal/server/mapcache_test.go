package server

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMapCacheEnforcesPerSiteAndGlobalLimits(t *testing.T) {
	now := time.Now()
	cache := newMapCache()
	for index := 0; index <= mapCacheMaxSiteEntries; index++ {
		key := fmt.Sprintf("site-a|%03d", index)
		cache.put(key, mapCacheItem{body: []byte{byte(index)}, expiresAt: now.Add(time.Hour)})
	}
	cache.put("site-b|only", mapCacheItem{body: []byte("b"), expiresAt: now.Add(time.Hour)})
	if cache.siteEntries["site-a"] != mapCacheMaxSiteEntries || cache.siteEntries["site-b"] != 1 {
		t.Fatalf("site entry counts = %#v", cache.siteEntries)
	}
	if _, ok := cache.get("site-a|000", now); ok {
		t.Fatal("oldest Site variant was not evicted")
	}
	if _, ok := cache.get("site-b|only", now); !ok {
		t.Fatal("another Site was evicted by the per-Site limit")
	}

	large := newMapCache()
	body := make([]byte, 1024*1024)
	for index := 0; index < 33; index++ {
		key := fmt.Sprintf("site-%02d|map", index)
		large.put(key, mapCacheItem{body: body, expiresAt: now.Add(time.Hour)})
	}
	if large.bytes > mapCacheMaxBytes || large.order.Len() != 32 {
		t.Fatalf("global cache = %d bytes, %d entries", large.bytes, large.order.Len())
	}
}

func TestMapCacheExpiresAndCoalescesConcurrentMisses(t *testing.T) {
	now := time.Now()
	cache := newMapCache()
	cache.put("site|expired", mapCacheItem{body: []byte("old"), expiresAt: now})
	if _, ok := cache.get("site|expired", now); ok {
		t.Fatal("expired cache item was returned")
	}

	var calls atomic.Int32
	renderStarted := make(chan struct{})
	release := make(chan struct{})
	start := make(chan struct{})
	const workers = 24
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			item, err := cache.getOrRender(context.Background(), "site|shared", "site", now, func() (mapCacheItem, error) {
				if calls.Add(1) == 1 {
					close(renderStarted)
				}
				<-release
				return mapCacheItem{body: []byte("rendered"), expiresAt: now.Add(time.Minute)}, nil
			})
			if err != nil {
				errors <- err
				return
			}
			if string(item.body) != "rendered" {
				errors <- fmt.Errorf("body = %q", item.body)
			}
		}()
	}
	close(start)
	<-renderStarted
	close(release)
	wait.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("render calls = %d, want 1", calls.Load())
	}
}

func TestMapCacheInvalidationRejectsInFlightResult(t *testing.T) {
	now := time.Now()
	cache := newMapCache()
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := cache.getOrRender(context.Background(), "site|map", "site", now, func() (mapCacheItem, error) {
			close(started)
			<-release
			return mapCacheItem{body: []byte("stale"), expiresAt: now.Add(time.Minute)}, nil
		})
		done <- err
	}()
	<-started
	cache.deleteSite("site")
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.get("site|map", now); ok {
		t.Fatal("an in-flight result survived Site invalidation")
	}
}
