// This file handles cache eviction: when the keyspace is full, pick a key to remove
// according to the configured policy (random, traditional LRU, approximated LRU).
// For simplicity the limit is a maximum number of keys instead of a maximum memory size.
// The limit is per Store (s.maxKeys); the policy and the pool/sample sizes live in the config package.
package core

import (
	"Redis-k02/Part9/internal/config"
	"Redis-k02/Part9/internal/datastructure"
)

// touch records an access to key. Called by Get and Set, so every command that reads or
// writes a key goes through it without having to call it manually. See accessClock below.
func (s *Store) touch(key string, obj *Object) {
	s.accessClock++
	obj.lastAccess = s.accessClock
	if config.Policy == config.PolicyLRU {
		s.lruList.Touch(key)
	}
}

// evictionUntrack forgets key in the eviction bookkeeping. Called by removeKey whenever a key leaves the store.
func (s *Store) evictionUntrack(key string) {
	if config.Policy == config.PolicyLRU {
		s.lruList.Remove(key)
	}
}

// evictIfNeeded is called by Set before a new key is inserted, so the key being created is never evicted.
func (s *Store) evictIfNeeded() {
	for len(s.data) >= s.maxKeys && len(s.data) > 0 {
		switch config.Policy {
		case config.PolicyRandom:
			s.evictRandom()
		case config.PolicyLRU:
			s.evictLRU()
		case config.PolicyApproxLRU:
			s.evictApproxLRU()
		}
	}
}

// !!(1): RANDOM EVICTION ___________________________________________________________:

// evictRandom removes one random key. Go map iteration starts at a random position,
// so the first key of a range is a random pick.
func (s *Store) evictRandom() {
	for key := range s.data {
		s.removeKey(key)
		return
	}
}

// !!(2): LRU EVICTION ________________________________________________________________:

// s.lruList tracks every key in access order, only used by PolicyLRU.

// evictLRU removes the head of the LRU list, i.e. the least recently used key.
func (s *Store) evictLRU() {
	if key, ok := s.lruList.PopLRU(); ok {
		s.removeKey(key)
		return
	}
	// The list should always mirror the store; fall back to random so we never loop forever.
	s.evictRandom()
}

// !!(3): APPROXIMATED LRU EVICTION ___________________________________________________:

// s.accessClock is a logical clock: every access gets a unique, strictly increasing stamp.
// Unlike time.Now(), two accesses can never tie, which the stale-entry check in evictApproxLRU relies on.

// evictApproxLRU follows Redis' approximated LRU:
//  1. Sample EvictionSampleSize keys from the store and add them to the pool.
//  2. The pool sorts itself LRU -> MRU and trims the MRU side to EvictionPoolSize entries.
//  3. Evict the first (LRU) entry of the pool.
func (s *Store) evictApproxLRU() {
	for {
		s.evictionPool.Add(s.sampleKeys(config.EvictionSampleSize))
		for entry, ok := s.evictionPool.PopLRU(); ok; entry, ok = s.evictionPool.PopLRU() {
			obj := s.data[entry.Key]
			// Skip the entry if the key was deleted, or was accessed after it was sampled:
			// its snapshot is stale, so evicting it would evict a recently used key.
			if obj == nil || obj.lastAccess != entry.LastAccess {
				continue
			}
			s.removeKey(entry.Key)
			return
		}
		// Every pool entry was stale: sample again. The store is not empty, so this terminates.
	}
}

// sampleKeys picks n random keys from the store with their current lastAccess.
// Like evictRandom, it relies on Go's random map iteration order.
func (s *Store) sampleKeys(n int) []datastructure.PoolEntry {
	samples := make([]datastructure.PoolEntry, 0, n)
	for key, obj := range s.data {
		if len(samples) >= n {
			break
		}
		samples = append(samples, datastructure.PoolEntry{Key: key, LastAccess: obj.lastAccess})
	}
	return samples
}
