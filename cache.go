package boundedcache

import (
	"errors"
	"sync"
)

type BoundedCache struct {
	staleItems    map[interface{}]interface{}
	freshItems    map[interface{}]interface{}
	maxItemMapLen int

	rwm sync.RWMutex
}

func New(maxItems int) (*BoundedCache, error) {
	if maxItems < 2 {
		return nil, errors.New("maxItems must be at least 2")
	}

	return &BoundedCache{
		staleItems:    make(map[interface{}]interface{}),
		freshItems:    make(map[interface{}]interface{}),
		maxItemMapLen: maxItems / 2,
	}, nil
}

func (b *BoundedCache) MaxItems() int {
	return b.maxItemMapLen * 2
}

func (b *BoundedCache) Len() int {
	b.rwm.RLock()
	defer b.rwm.RUnlock()

	return len(b.freshItems) + len(b.staleItems)
}

func (b *BoundedCache) Purge() {
	b.rwm.Lock()
	defer b.rwm.Unlock()

	b.staleItems = make(map[interface{}]interface{})
	b.freshItems = make(map[interface{}]interface{})
}

func (b *BoundedCache) Add(key, val interface{}) (evicted bool) {
	b.rwm.Lock()
	defer b.rwm.Unlock()

	return b.addLocked(key, val)
}

// Get looks in the cache for the given key, returning its value.
// This cache can potentially evict items during a Get request, if the value
// is in the stale half of the cache and the fresh half is full.
func (b *BoundedCache) Get(key interface{}) (val interface{}, ok bool, evicted bool) {
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

	return nil, false, false
}

func (b *BoundedCache) GetOrCreate(key interface{}, create func() interface{}) (val interface{}, created bool, evicted bool) {
	// multiple readers can get items concurrently
	var ok bool
	b.rwm.RLock()

	if val, ok = b.freshItems[key]; ok {
		b.rwm.RUnlock()
		return val, created, evicted
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
		return val, created, evicted
	}

	// ensure that this value wasn't added by another concurrent goroutine
	if val, ok = b.freshItems[key]; ok {
		return val, created, evicted
	}

	// if here, a value needs to be created.
	created = true
	val = create()
	evicted = b.addLocked(key, val)

	return val, created, evicted
}

// addLocked adds val to the cache while the cache is already locked
// It checks if the currentItem map is full, and if so, shifts the
// currentItem map to priorItem, and generates a new map.
func (b *BoundedCache) addLocked(key, val interface{}) (evicted bool) {
	if len(b.freshItems) >= b.maxItemMapLen {
		b.staleItems = b.freshItems
		b.freshItems = make(map[interface{}]interface{})
		evicted = true
	}

	b.freshItems[key] = val
	return evicted
}
