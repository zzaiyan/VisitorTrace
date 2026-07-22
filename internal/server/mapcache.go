package server

import (
	"container/list"
	"strings"
	"sync"
	"time"
)

const (
	mapCacheTTL        = 5 * time.Minute
	mapCacheMaxBytes   = 32 * 1024 * 1024
	mapCacheMaxEntries = 256
)

type mapCache struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	order   *list.List
	bytes   int
}

func (c *mapCache) deleteSite(siteID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := siteID + "|"
	for key, element := range c.entries {
		if strings.HasPrefix(key, prefix) {
			c.remove(element)
		}
	}
}

type mapCacheItem struct {
	key       string
	body      []byte
	etag      string
	expiresAt time.Time
}

func newMapCache() *mapCache {
	return &mapCache{entries: make(map[string]*list.Element), order: list.New()}
}

func (c *mapCache) get(key string, now time.Time) (mapCacheItem, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
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
	if old, ok := c.entries[key]; ok {
		c.remove(old)
	}
	if len(item.body) > mapCacheMaxBytes {
		return
	}
	item.key = key
	element := c.order.PushFront(item)
	c.entries[key] = element
	c.bytes += len(item.body)
	for c.bytes > mapCacheMaxBytes || c.order.Len() > mapCacheMaxEntries {
		c.remove(c.order.Back())
	}
}

func (c *mapCache) remove(element *list.Element) {
	if element == nil {
		return
	}
	item := element.Value.(mapCacheItem)
	delete(c.entries, item.key)
	c.order.Remove(element)
	c.bytes -= len(item.body)
}
