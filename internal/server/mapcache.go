package server

import (
	"container/list"
	"context"
	"strings"
	"sync"
	"time"
)

const (
	mapCacheTTL            = 5 * time.Minute
	mapCacheMaxBytes       = 32 * 1024 * 1024
	mapCacheMaxSiteEntries = 256
)

type mapCache struct {
	mu          sync.Mutex
	entries     map[string]*list.Element
	order       *list.List
	siteEntries map[string]int
	bytes       int
	flights     map[string]*mapCacheFlight
	generations map[string]uint64
}

func (c *mapCache) deleteSite(siteID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generations[siteID]++
	prefix := siteID + "|"
	for key, element := range c.entries {
		if strings.HasPrefix(key, prefix) {
			c.remove(element)
		}
	}
}

type mapCacheItem struct {
	key       string
	siteID    string
	body      []byte
	etag      string
	expiresAt time.Time
}

type mapCacheFlight struct {
	done chan struct{}
	item mapCacheItem
	err  error
}

func newMapCache() *mapCache {
	return &mapCache{
		entries: make(map[string]*list.Element), order: list.New(), siteEntries: make(map[string]int),
		flights: make(map[string]*mapCacheFlight), generations: make(map[string]uint64),
	}
}

func (c *mapCache) get(key string, now time.Time) (mapCacheItem, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getLocked(key, now)
}

func (c *mapCache) getLocked(key string, now time.Time) (mapCacheItem, bool) {
	element, ok := c.entries[key]
	if !ok {
		return mapCacheItem{}, false
	}
	item := element.Value.(mapCacheItem)
	if !now.Before(item.expiresAt) {
		c.remove(element)
		return mapCacheItem{}, false
	}
	c.order.MoveToFront(element)
	return item, true
}

func (c *mapCache) put(key string, item mapCacheItem) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.putLocked(key, item)
}

func (c *mapCache) putLocked(key string, item mapCacheItem) {
	if old, ok := c.entries[key]; ok {
		c.remove(old)
	}
	if len(item.body) > mapCacheMaxBytes {
		return
	}
	item.key = key
	if item.siteID == "" {
		item.siteID, _, _ = strings.Cut(key, "|")
	}
	element := c.order.PushFront(item)
	c.entries[key] = element
	c.bytes += len(item.body)
	c.siteEntries[item.siteID]++
	for c.siteEntries[item.siteID] > mapCacheMaxSiteEntries {
		if oldest := c.oldestForSite(item.siteID); oldest != nil {
			c.remove(oldest)
		}
	}
	for c.bytes > mapCacheMaxBytes {
		c.remove(c.order.Back())
	}
}

func (c *mapCache) getOrRender(ctx context.Context, key, siteID string, now time.Time, render func() (mapCacheItem, error)) (mapCacheItem, error) {
	c.mu.Lock()
	if item, ok := c.getLocked(key, now); ok {
		c.mu.Unlock()
		return item, nil
	}
	if flight, ok := c.flights[key]; ok {
		c.mu.Unlock()
		select {
		case <-flight.done:
			return flight.item, flight.err
		case <-ctx.Done():
			return mapCacheItem{}, ctx.Err()
		}
	}
	flight := &mapCacheFlight{done: make(chan struct{})}
	generation := c.generations[siteID]
	c.flights[key] = flight
	c.mu.Unlock()

	item, err := render()
	item.key = key
	item.siteID = siteID

	c.mu.Lock()
	if err == nil && c.generations[siteID] == generation {
		c.putLocked(key, item)
	}
	flight.item = item
	flight.err = err
	delete(c.flights, key)
	close(flight.done)
	c.mu.Unlock()
	return item, err
}

func (c *mapCache) oldestForSite(siteID string) *list.Element {
	for element := c.order.Back(); element != nil; element = element.Prev() {
		if element.Value.(mapCacheItem).siteID == siteID {
			return element
		}
	}
	return nil
}

func (c *mapCache) remove(element *list.Element) {
	if element == nil {
		return
	}
	item := element.Value.(mapCacheItem)
	delete(c.entries, item.key)
	c.order.Remove(element)
	c.bytes -= len(item.body)
	c.siteEntries[item.siteID]--
	if c.siteEntries[item.siteID] <= 0 {
		delete(c.siteEntries, item.siteID)
	}
}
