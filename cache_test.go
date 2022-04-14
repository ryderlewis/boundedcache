package boundedcache

import (
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestNewBoundedCache(t *testing.T) {
	if b, err := NewBoundedCache[int](-1); err == nil || b != nil {
		t.Fatalf("expect negative size cache to produce an error")
	}

	if b, err := NewBoundedCache[int](0); err == nil || b != nil {
		t.Fatalf("expect cache with size less than 1 to produce an error")
	}

	if b, err := NewBoundedCache[int](1); err != nil || b == nil || b.MaxItems() != 2 {
		t.Fatalf("expect cache with size 1 to be valid but round up to 2")
	}

	if b, err := NewBoundedCache[int](100); err != nil || b == nil || b.MaxItems() != 100 {
		t.Fatalf("expect cache with size 100 to be valid")
	}
}

func TestBoundedCacheBehavior(t *testing.T) {
	// create a cache with 100 items, in order, with key "0" through "99"
	b, err := NewBoundedCache[int](100)
	if err != nil || b == nil {
		t.Fatalf("expected cache creation to be successful")
	}

	for i := 0; i < 100; i++ {
		if evicted := b.Add(strconv.Itoa(i), i); evicted == true {
			t.Fatalf("no evictions expected as cache is populated")
		}
	}

	// peek into all the items
	for i := 0; i < 100; i++ {
		val, ok, stale := b.Peek(strconv.Itoa(i))
		if !ok || val != i {
			t.Fatalf("unexpected missing item from cache, %d is not present", i)
		}

		expectStale := i < 50
		if stale != expectStale {
			t.Fatalf("unexpected staleness in cache, value = %d, expected %v, got %v", i, expectStale, stale)
		}
	}

	// any items in the "fresh" half of the cache should be present, and not cause an eviction
	for i := 50; i < 100; i++ {
		val, ok, evicted := b.Get(strconv.Itoa(i))
		if evicted {
			t.Fatalf("no evictions expected in fresh half of cache")
		}
		if !ok || val != i {
			t.Fatalf("unexpected absent item in fresh half of cache")
		}
	}

	// get "25" from the cache. The expected behavior is that an eviction occurs, dropping items "1" - "49",
	// and moving "50" through "99" to the stale half of the cache.
	if val, ok, evicted := b.Get("25"); !ok || val != 25 {
		t.Fatalf("missing value 25 from the cache")
	} else if !evicted {
		t.Fatalf("expected a stale Get to evict items from cache")
	}

	// peek again this time "0" through "49" should all be missing (except for "25").
	// "50" - "99" are all stale, and "25" is the only fresh item
	for i := 0; i < 100; i++ {
		val, ok, stale := b.Peek(strconv.Itoa(i))
		expectOk := i >= 50 || i == 25
		expectStale := i >= 50

		if expectOk {
			if !ok || val != i {
				t.Fatalf("unexpected missing item from cache, %d is not present", i)
			}
			if stale != expectStale {
				t.Fatalf("unexpected staleness in cache, value = %d, expected %v, got %v", i, expectStale, stale)
			}
		} else {
			if ok {
				t.Fatalf("unexpected present item in cache, %d is present", i)
			}
		}
	}
}

func Benchmark_BoundedCacheWithHeadroom(b *testing.B) {
	keyCounts := make(map[string]int)
	for i := 0; i < 256; i++ {
		keyCounts[strconv.Itoa(i)] = 1
	}

	benchmarkBoundedCache(b, 512, keyCounts, createCachedInt)
}

func Benchmark_BoundedCacheWithEvictions(b *testing.B) {
	keyCounts := make(map[string]int)
	for i := 0; i < 256; i++ {
		keyCounts[strconv.Itoa(i)] = 1
	}

	benchmarkBoundedCache(b, 128, keyCounts, createCachedInt)
}

func Benchmark_BoundedCacheWithHotKeys(b *testing.B) {
	keyCounts := make(map[string]int)
	for i := 0; i < 256+10; i++ {
		keyCounts[strconv.Itoa(i)] = 1
	}
	keyCounts["hot1"] = 2500
	keyCounts["hot2"] = 2500

	benchmarkBoundedCache(b, 512, keyCounts, createCachedInt)
}

func benchmarkBoundedCache(b *testing.B, maxItems int, keyCounts map[string]int, createFn func() int) {
	b.Helper()
	cache, err := NewBoundedCache[int](maxItems)
	if err != nil {
		b.Fatalf("error creating cache: %v", err)
	}

	// convert keys into a slice
	keys := make([]string, 0)
	for key, count := range keyCounts {
		for i := 0; i < count; i++ {
			keys = append(keys, key)
		}
	}

	// shuffle the slice
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })

	b.ReportAllocs()
	b.ResetTimer()

	var mu sync.Mutex
	var totalHitCount, totalMissCount, totalEvictCount int

	b.RunParallel(func(pb *testing.PB) {
		var hitCount, missCount, evictCount int
		var created, evicted bool

		mu.Lock()
		index := rand.Intn(len(keys))
		_, created, evicted = cache.GetOrCreate(keys[index], createFn)
		if created || evicted {

		}
		mu.Unlock()

		for pb.Next() {
			if index >= len(keys) {
				index = 0
			}

			_, created, evicted = cache.GetOrCreate(keys[index], createFn)
			if created {
				missCount++
			} else {
				hitCount++
			}
			if evicted {
				evictCount++
			}

			index++
		}

		mu.Lock()
		totalHitCount += hitCount
		totalMissCount += missCount
		totalEvictCount += evictCount
		mu.Unlock()
	})

	/*
		b.Logf("Hit: %d miss: %d ratio: %f evictions: %d", totalHitCount, totalMissCount,
			float64(totalHitCount)/float64(totalMissCount), totalEvictCount)

	*/
}

func createCachedInt() int {
	return 0
}
