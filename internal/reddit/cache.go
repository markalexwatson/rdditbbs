package reddit

import (
	"container/list"
	"sync"
	"time"
)

type cacheEntry struct {
	key     string
	value   []byte
	expires time.Time
}

// Cache is a bounded LRU of response bodies with per-entry TTL. Storing bytes
// rather than parsed values means every caller gets its own parse.
type Cache struct {
	capacity int
	now      func() time.Time

	mu    sync.Mutex
	ll    *list.List
	items map[string]*list.Element
}

// NewCache creates a cache holding at most capacity entries.
func NewCache(capacity int, now func() time.Time) *Cache {
	if now == nil {
		now = time.Now
	}
	return &Cache{capacity: capacity, now: now, ll: list.New(), items: map[string]*list.Element{}}
}

// Get returns a live entry and marks it most recently used.
func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	e := el.Value.(*cacheEntry)
	if !c.now().Before(e.expires) {
		c.ll.Remove(el)
		delete(c.items, key)
		return nil, false
	}
	c.ll.MoveToFront(el)
	return e.value, true
}

// Put stores or replaces an entry, evicting the least recently used beyond capacity.
func (c *Cache) Put(key string, v []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value = &cacheEntry{key: key, value: v, expires: c.now().Add(ttl)}
		c.ll.MoveToFront(el)
		return
	}
	c.items[key] = c.ll.PushFront(&cacheEntry{key: key, value: v, expires: c.now().Add(ttl)})
	for c.ll.Len() > c.capacity {
		last := c.ll.Back()
		c.ll.Remove(last)
		delete(c.items, last.Value.(*cacheEntry).key)
	}
}

// Len is the number of entries, expired or not.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}
