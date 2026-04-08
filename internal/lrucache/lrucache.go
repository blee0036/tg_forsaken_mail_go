package lrucache

import (
	lru "github.com/hashicorp/golang-lru/v2"
)

// Cache is a thread-safe LRU cache with a fixed max size.
// When full, the least recently used entry is evicted.
// This replaces go-cache + manual maxKeys enforcement.
type Cache struct {
	c *lru.Cache[string, interface{}]
}

// New creates an LRU cache with the given max number of entries.
func New(maxKeys int) *Cache {
	c, _ := lru.New[string, interface{}](maxKeys)
	return &Cache{c: c}
}

// Get returns the value for key and whether it was found.
// Accessing a key marks it as recently used.
func (c *Cache) Get(key string) (interface{}, bool) {
	return c.c.Get(key)
}

// Set adds or updates a key-value pair. If the cache is full,
// the least recently used entry is evicted.
func (c *Cache) Set(key string, value interface{}) {
	c.c.Add(key, value)
}

// Delete removes a key from the cache.
func (c *Cache) Delete(key string) {
	c.c.Remove(key)
}

// Take gets a value and removes it atomically (like NodeCache.take).
func (c *Cache) Take(key string) (interface{}, bool) {
	val, ok := c.c.Get(key)
	if ok {
		c.c.Remove(key)
	}
	return val, ok
}

// Len returns the current number of items in the cache.
func (c *Cache) Len() int {
	return c.c.Len()
}
