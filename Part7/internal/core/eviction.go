// This file handles cache eviction: when the keyspace is full, pick a key to remove
// according to the configured policy (random, traditional LRU, approximated LRU).
// For simplicity the limit is a maximum number of keys instead of a maximum memory size.
// The limit, the policy and the pool/sample sizes live in the config package.
package core

import (
	"Redis-k02/Part7/internal/config"
	"Redis-k02/Part7/internal/datastructure"
)

// touch records an access to key. Called by Get and Set, so every command that reads or
// writes a key goes through it without having to call it manually. See accessClock below.
func touch(key string, obj *Object) {
	accessClock++
	obj.lastAccess = accessClock
	if config.Policy == config.PolicyLRU {
		lruList.Touch(key)
	}
}

// untrack forgets key in the eviction bookkeeping. Called by removeKey whenever a key leaves the store.
func untrack(key string) {
	if config.Policy == config.PolicyLRU {
		lruList.Remove(key)
	}
}

// evictIfNeeded is called by Set before a new key is inserted, so the key being created is never evicted.
func evictIfNeeded() {
	for len(store) >= config.MaxKeys && len(store) > 0 {
		switch config.Policy {
		case config.PolicyRandom:
			evictRandom()
		case config.PolicyLRU:
			evictLRU()
		case config.PolicyApproxLRU:
			evictApproxLRU()
		}
	}
}

// !!(1): RANDOM EVICTION ___________________________________________________________:

// evictRandom removes one random key. Go map iteration starts at a random position,
// so the first key of a range is a random pick.
func evictRandom() {
	for key := range store {
		removeKey(key)
		return
	}
}

// !!(2): LRU EVICTION ________________________________________________________________:

// lruList tracks every key in access order, only used by PolicyLRU.
var lruList = datastructure.NewLRUList()

// evictLRU removes the head of the LRU list, i.e. the least recently used key.
func evictLRU() {
	if key, ok := lruList.PopLRU(); ok {
		removeKey(key)
		return
	}
	// The list should always mirror the store; fall back to random so we never loop forever.
	evictRandom()
}

// !!(3): APPROXIMATED LRU EVICTION ___________________________________________________:

// accessClock is a logical clock: every access gets a unique, strictly increasing stamp.
// Unlike time.Now(), two accesses can never tie, which the stale-entry check in evictApproxLRU relies on.
var accessClock uint64

var evictionPool = datastructure.NewEvictionPool(config.EvictionPoolSize)

// evictApproxLRU follows Redis' approximated LRU:
//  1. Sample EvictionSampleSize keys from the store and add them to the pool.
//  2. The pool sorts itself LRU -> MRU and trims the MRU side to EvictionPoolSize entries.
//  3. Evict the first (LRU) entry of the pool.
func evictApproxLRU() {
	for {
		evictionPool.Add(sampleKeys(config.EvictionSampleSize))
		for entry, ok := evictionPool.PopLRU(); ok; entry, ok = evictionPool.PopLRU() {
			obj := store[entry.Key]
			// Skip the entry if the key was deleted, or was accessed after it was sampled:
			// its snapshot is stale, so evicting it would evict a recently used key.
			if obj == nil || obj.lastAccess != entry.LastAccess {
				continue
			}
			removeKey(entry.Key)
			return
		}
		// Every pool entry was stale: sample again. The store is not empty, so this terminates.
	}
}

// sampleKeys picks n random keys from the store with their current lastAccess.
// Like evictRandom, it relies on Go's random map iteration order.
func sampleKeys(n int) []datastructure.PoolEntry {
	samples := make([]datastructure.PoolEntry, 0, n)
	for key, obj := range store {
		if len(samples) >= n {
			break
		}
		samples = append(samples, datastructure.PoolEntry{Key: key, LastAccess: obj.lastAccess})
	}
	return samples
}
