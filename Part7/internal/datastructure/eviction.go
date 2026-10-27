// This file handles cache eviction: when the keyspace is full, pick a key to remove
// according to the configured policy (random, traditional LRU, approximated LRU).
// For simplicity the limit is a maximum number of keys instead of a maximum memory size.
package datastructure

import "sort"

type EvictionPolicy int

const (
	PolicyRandom EvictionPolicy = iota
	PolicyLRU
	PolicyApproxLRU
)

// maxKeys and evictionPolicy are chosen at startup. Don't switch the policy at runtime:
// the LRU list is only maintained under PolicyLRU, so it would be out of sync with the store.
var (
	maxKeys        = 1000
	evictionPolicy = PolicyApproxLRU
)

// accessClock is a logical clock: every access gets a unique, strictly increasing stamp.
// Unlike time.Now(), two accesses can never tie, which the stale-entry check in evictApproxLRU relies on.
var accessClock uint64

// touch records an access to key. Called by Get and Set, so every command that reads or
// writes a key goes through it without having to call it manually.
func touch(key string, obj *Object) {
	accessClock++
	obj.lastAccess = accessClock
	if evictionPolicy == PolicyLRU {
		lruList.Touch(key)
	}
}

// untrack forgets key in the eviction bookkeeping. Called by removeKey whenever a key leaves the store.
func untrack(key string) {
	if evictionPolicy == PolicyLRU {
		lruList.Remove(key)
	}
}

// evictIfNeeded is called by Set before a new key is inserted, so the key being created is never evicted.
func evictIfNeeded() {
	for len(store) >= maxKeys && len(store) > 0 {
		switch evictionPolicy {
		case PolicyRandom:
			evictRandom()
		case PolicyLRU:
			evictLRU()
		case PolicyApproxLRU:
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

var lruList = NewLRUList()

func evictLRU() {
	if key, ok := lruList.PopLRU(); ok {
		removeKey(key)
		return
	}
	// The list should always mirror the store; fall back to random so we never loop forever.
	evictRandom()
}

// !!(3): APPROXIMATED LRU EVICTION ___________________________________________________:

// Same numbers as Redis: each eviction samples 5 keys into a pool that keeps the 16 best candidates.
const (
	evictionPoolSize   = 16
	evictionSampleSize = 5
)

// poolEntry is a snapshot of a key's lastAccess taken when the key was sampled.
// It is not updated when the key is accessed later, see evictApproxLRU.
type poolEntry struct {
	key        string
	lastAccess uint64
}

// evictionPool is sorted ascending by lastAccess: index 0 is the LRU end.
var evictionPool []poolEntry

// evictApproxLRU follows Redis' approximated LRU:
//  1. Sample evictionSampleSize keys from the store and add them to the pool.
//  2. Sort the pool LRU -> MRU and trim the MRU side so it holds at most evictionPoolSize entries.
//  3. Evict the first (LRU) entry of the pool.
func evictApproxLRU() {
	for {
		sampleIntoPool()
		for len(evictionPool) > 0 {
			entry := evictionPool[0]
			evictionPool = evictionPool[1:]
			obj := store[entry.key]
			// Skip the entry if the key was deleted, or was accessed after it was sampled:
			// its snapshot is stale, so evicting it would evict a recently used key.
			if obj == nil || obj.lastAccess != entry.lastAccess {
				continue
			}
			removeKey(entry.key)
			return
		}
		// Every pool entry was stale: sample again. The store is not empty, so this terminates.
	}
}

func sampleIntoPool() {
	sampled := 0
	for key, obj := range store {
		if sampled >= evictionSampleSize {
			break
		}
		sampled++
		// Dedup: a key already in the pool gets its snapshot refreshed instead of a second entry.
		if i := poolIndex(key); i >= 0 {
			evictionPool[i].lastAccess = obj.lastAccess
			continue
		}
		evictionPool = append(evictionPool, poolEntry{key: key, lastAccess: obj.lastAccess})
	}
	sort.Slice(evictionPool, func(i, j int) bool {
		return evictionPool[i].lastAccess < evictionPool[j].lastAccess
	})
	if len(evictionPool) > evictionPoolSize {
		evictionPool = evictionPool[:evictionPoolSize]
	}
}

// poolIndex returns the position of key in the pool, or -1. The pool has at most
// evictionPoolSize+evictionSampleSize entries, so a linear scan is enough.
func poolIndex(key string) int {
	for i, entry := range evictionPool {
		if entry.key == key {
			return i
		}
	}
	return -1
}
