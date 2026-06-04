package cache

import (
	"sync"
	"time"
)

type entry[T any] struct {
	value   T
	expires time.Time
}

type call[T any] struct {
	done  chan struct{}
	value T
	err   error
}

type TTLCache[K comparable, V any] struct {
	mu    sync.Mutex
	items map[K]entry[V]
	calls map[K]*call[V]
	ttl   time.Duration
	max   int
}

func NewTTLCache[K comparable, V any](maxItems int, ttl time.Duration) *TTLCache[K, V] {
	return &TTLCache[K, V]{
		items: make(map[K]entry[V]),
		calls: make(map[K]*call[V]),
		ttl:   ttl,
		max:   maxItems,
	}
}

func (c *TTLCache[K, V]) Get(key K, loader func() (V, error)) (V, error) {
	c.mu.Lock()
	if e, ok := c.items[key]; ok && time.Now().Before(e.expires) {
		c.mu.Unlock()
		return e.value, nil
	}
	delete(c.items, key)
	if pending, ok := c.calls[key]; ok {
		c.mu.Unlock()
		<-pending.done
		return pending.value, pending.err
	}
	pending := &call[V]{done: make(chan struct{})}
	c.calls[key] = pending
	c.mu.Unlock()

	v, err := loader()

	c.mu.Lock()
	pending.value = v
	pending.err = err
	delete(c.calls, key)
	if err == nil && c.max > 0 {
		if len(c.items) >= c.max {
			c.evictOneLocked()
		}
		c.items[key] = entry[V]{value: v, expires: time.Now().Add(c.ttl)}
	}
	close(pending.done)
	c.mu.Unlock()
	return v, nil
}

func (c *TTLCache[K, V]) evictOneLocked() {
	var oldest K
	var oldestTime time.Time
	first := true
	for k, e := range c.items {
		if first || e.expires.Before(oldestTime) {
			oldest = k
			oldestTime = e.expires
			first = false
		}
	}
	delete(c.items, oldest)
}
