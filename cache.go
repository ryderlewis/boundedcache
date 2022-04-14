package boundedcache

import (
	"errors"
	"sync"
)

type BoundedCache[V any] struct {
	staleItems    map[string]V
	freshItems    map[string]V
	maxItemMapLen int

	rwm sync.RWMutex
}

func NewBoundedCache[V any](maxItems int) (*BoundedCache[V], error) {
	if maxItems < 1 {
		return nil, errors.New("maxItems must be at least 1")
	}

	return &BoundedCache[V]{
		staleItems:    make(map[string]V),
		freshItems:    make(map[string]V),
		maxItemMapLen: (maxItems + 1) / 2,
	}, nil
}

func (b *BoundedCache[V]) MaxItems() int {
	return b.maxItemMapLen * 2
}

func (b *BoundedCache[V]) Len() int {
	b.rwm.RLock()
	defer b.rwm.RUnlock()

	return len(b.freshItems) + len(b.staleItems)
}

func (b *BoundedCache[V]) Purge() {
	b.rwm.Lock()
	defer b.rwm.Unlock()

	b.staleItems = make(map[string]V)
	b.freshItems = make(map[string]V)
}

func (b *BoundedCache[V]) Add(key string, val V) (evicted bool) {
	b.rwm.Lock()
	defer b.rwm.Unlock()

	return b.addLocked(key, val)
}

// Get looks in the cache for the given key, returning its value.
// This cache can potentially evict items during a Get request, if the value
// is in the stale half of the cache and the fresh half is full.
func (b *BoundedCache[V]) Get(key string) (val V, ok bool, evicted bool) {
	// multiple readers can get items concurrently
	b.rwm.RLock()
	if val, ok = b.freshItems[key]; ok {
		b.rwm.RUnlock()
		return val, ok, evicted
	}
	b.rwm.RUnlock()

	// enter the write-protected zone
	b.rwm.Lock()
	defer b.rwm.Unlock()

	// see if the item is in priorItems. If so, move to the currentItems map
	// so that it remains in the "fresh" half of the cache
	if val, ok = b.staleItems[key]; ok {
		delete(b.staleItems, key)
		evicted = b.addLocked(key, val)
		return val, ok, evicted
	}

	var nilV V
	return nilV, false, false
}

// Peek looks in the cache for the given key, returning its value.
// This operation does not update the underlying cache. It does however
// indicate whether the cached item is "stale", meaning it is subject
// to eviction if the fresh half of the cache becomes full.
func (b *BoundedCache[V]) Peek(key string) (val V, ok bool, stale bool) {
	// multiple readers can get items concurrently
	b.rwm.RLock()
	defer b.rwm.RUnlock()

	if val, ok = b.freshItems[key]; ok {
		return val, true, false
	}

	if val, ok = b.staleItems[key]; ok {
		return val, true, true
	}

	var nilV V
	return nilV, false, false
}

func (b *BoundedCache[V]) GetOrCreate(key string, create func() V) (_ V, created bool, evicted bool) {
	// multiple readers can get items concurrently
	b.rwm.RLock()

	if val, ok := b.freshItems[key]; ok {
		b.rwm.RUnlock()
		return val, created, evicted
	}
	b.rwm.RUnlock()

	// enter the write-protected zone
	b.rwm.Lock()
	defer b.rwm.Unlock()

	// see if the item is in priorItems. If so, move to the currentItems map
	// so that it remains in the "fresh" half of the cache
	if val, ok := b.staleItems[key]; ok {
		delete(b.staleItems, key)
		evicted = b.addLocked(key, val)
		return val, created, evicted
	}

	// ensure that this value wasn't added by another concurrent goroutine
	if val, ok := b.freshItems[key]; ok {
		return val, created, evicted
	}

	// if here, a value needs to be created.
	created = true
	val := create()
	evicted = b.addLocked(key, val)

	return val, created, evicted
}

// addLocked adds val to the cache while the cache is already locked
// It checks if the currentItem map is full, and if so, shifts the
// currentItem map to priorItem, and generates a new map.
func (b *BoundedCache[V]) addLocked(key string, val V) (evicted bool) {
	if len(b.freshItems) >= b.maxItemMapLen {
		evicted = len(b.staleItems) > 0
		b.staleItems = b.freshItems
		b.freshItems = make(map[string]V)
	}

	b.freshItems[key] = val
	return evicted
}
