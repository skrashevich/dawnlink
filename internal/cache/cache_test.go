package cache

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetDeduplicatesConcurrentLoads(t *testing.T) {
	cache := NewTTLCache[string, int](10, time.Minute)
	var loads atomic.Int32
	loader := func() (int, error) {
		loads.Add(1)
		time.Sleep(20 * time.Millisecond)
		return 42, nil
	}

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			value, err := cache.Get("key", loader)
			if err != nil || value != 42 {
				t.Errorf("Get() = %d, %v", value, err)
			}
		})
	}
	wg.Wait()
	if got := loads.Load(); got != 1 {
		t.Fatalf("loader called %d times, want 1", got)
	}
}
