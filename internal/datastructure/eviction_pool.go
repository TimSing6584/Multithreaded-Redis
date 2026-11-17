// This file defines the pool used by approximated LRU eviction: a small array of the best
// eviction candidates seen so far, sorted LRU -> MRU. It only holds snapshots, sampling keys
// from the keyspace and checking them against their live access time is done by the caller.
package datastructure

import "sort"

// PoolEntry is a snapshot of a key's lastAccess taken when the key was sampled.
// It is not updated when the key is accessed later, so the caller must check it is still current before evicting.
type PoolEntry struct {
	Key        string
	LastAccess uint64
}

// EvictionPool keeps at most capacity entries sorted ascending by LastAccess: index 0 is the LRU end.
type EvictionPool struct {
	entries  []PoolEntry
	capacity int
}

func NewEvictionPool(capacity int) *EvictionPool {
	return &EvictionPool{capacity: capacity}
}

// Add inserts the sampled entries, re-sorts the pool LRU -> MRU and trims the MRU side to capacity.
// Dedup: an entry for a key already in the pool refreshes its snapshot instead of adding a second entry.
func (p *EvictionPool) Add(samples []PoolEntry) {
	for _, s := range samples {
		if i := p.index(s.Key); i >= 0 {
			p.entries[i].LastAccess = s.LastAccess
			continue
		}
		p.entries = append(p.entries, s)
	}
	sort.Slice(p.entries, func(i, j int) bool {
		return p.entries[i].LastAccess < p.entries[j].LastAccess
	})
	if len(p.entries) > p.capacity {
		p.entries = p.entries[:p.capacity]
	}
}

// PopLRU removes and returns the LRU-end entry, and false if the pool is empty.
func (p *EvictionPool) PopLRU() (PoolEntry, bool) {
	if len(p.entries) == 0 {
		return PoolEntry{}, false
	}
	entry := p.entries[0]
	p.entries = p.entries[1:]
	return entry, true
}

func (p *EvictionPool) Len() int {
	return len(p.entries)
}

// index returns the position of key in the pool, or -1. The pool holds at most
// capacity + one sample's worth of entries, so a linear scan is enough.
func (p *EvictionPool) index(key string) int {
	for i, entry := range p.entries {
		if entry.Key == key {
			return i
		}
	}
	return -1
}
