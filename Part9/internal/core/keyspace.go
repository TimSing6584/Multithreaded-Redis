// This file defines the keyspace: one map from key to an Object that carries
// both the value and the kind of datastructure it is (e.g., string, set, sorted set,...).
// Plus with functions like get, set, delete a key and check its type,...

// Every piece of keyspace state (the keys, their TTLs, the eviction bookkeeping) lives
// in a Store value instead of package-level variables. That way each owner gets its own
// independent keyspace:
//   - the single-thread server owns exactly one Store,
//   - the multithread server gives every worker its own Store (one partition each).
//
// A Store is NOT safe for concurrent use: only its owner goroutine may touch it, which
// is exactly what the shared-nothing design guarantees, so no locks are needed.
package core

import (
	"Redis-k02/Part9/internal/config"
	"Redis-k02/Part9/internal/datastructure"
	"time"
)

// Kind identifies which datastructure a key currently holds.
// A key can only hold one datastructure at a time: overwriting it with a different type
// (e.g. SET on a key that held a set) simply replaces the Object.
type Kind int

const (
	KindNone Kind = iota
	KindString
	KindSet
	KindZSet
	KindCMS
	KindBloom
)

// Object is the value stored for a key. Value's concrete type depends on Kind:
// KindString -> string, KindSet -> *datastructure.SimpleSet, KindZSet -> *datastructure.ZSet,...
type Object struct {
	Kind  Kind
	Value any
	// lastAccess is the accessClock stamp of the last Get/Set of this key, set by touch().
	// Unexported so commands never have to fill it in. Used by approximated LRU eviction.
	lastAccess uint64
}

// Store is one independent keyspace. Its fields are unexported so every
// access goes through Get/Peek, which apply lazy expiry.
type Store struct {
	// data holds every key, whatever type of value it holds.
	data map[string]*Object

	// expires holds the expiration time of every key that has a TTL (see expire.go).
	expires map[string]time.Time

	// maxKeys is the eviction limit of THIS store (see eviction.go). The single-thread
	// server uses config.MaxKeys; the multithread server splits it across its workers.
	maxKeys int

	// Eviction bookkeeping (see eviction.go).
	accessClock  uint64
	lruList      *datastructure.LRUList
	evictionPool *datastructure.EvictionPool
}

// NewStore returns an empty keyspace that holds at most maxKeys keys before evicting.
func NewStore(maxKeys int) *Store {
	return &Store{
		data:         make(map[string]*Object),
		expires:      make(map[string]time.Time),
		maxKeys:      maxKeys,
		lruList:      datastructure.NewLRUList(),
		evictionPool: datastructure.NewEvictionPool(config.EvictionPoolSize),
	}
}

// We split Get into two functions: GET and PEEK. Get is the normal function that records an access for eviction, while Peek is used by commands
// that only inspect a key's metadata (EXISTS, TTL/PTTL), like Redis' LOOKUP_NOTOUCH flag. It still applies lazy expiry.
// We use a single helper function lookup with a touchKey flag, which is true for Get and false for Peek.
func (s *Store) lookup(key string, touchKey bool) *Object {
	if s.lazyDelete(key) {
		return nil
	}
	obj := s.data[key]
	if obj != nil && touchKey {
		s.touch(key, obj)
	}
	return obj
}

func (s *Store) Get(key string) *Object {
	return s.lookup(key, true)
}

func (s *Store) Peek(key string) *Object {
	return s.lookup(key, false)
}

// Set stores obj under key, replacing whatever type the key held before, and clears any leftover TTL.
// Inserting a new key may evict another one first if the keyspace is full.
func (s *Store) Set(key string, obj *Object) {
	// Overwriting an existing key doesn't grow the keyspace, so only a new key can trigger eviction.
	if _, exists := s.data[key]; !exists {
		s.evictIfNeeded()
	}
	s.data[key] = obj
	delete(s.expires, key)
	s.touch(key, obj)
}

// Delete removes key and its TTL. It reports whether the key existed (and was therefore removed).
func (s *Store) Delete(key string) bool {
	// Peek: touching a key right before removing it would be wasted work.
	if s.Peek(key) == nil {
		return false
	}
	s.removeKey(key)
	return true
}

// Exists reports whether key has a live value. It doesn't count as an access for eviction.
func (s *Store) Exists(key string) bool {
	return s.Peek(key) != nil
}

// KindOf reports which kind of datastructure key holds, or KindNone if it is absent.
// It doesn't count as an access for eviction.
func (s *Store) KindOf(key string) Kind {
	if obj := s.Peek(key); obj != nil {
		return obj.Kind
	}
	return KindNone
}

// removeKey is the single place a key leaves the store (DEL, expiry, eviction),
// so the eviction bookkeeping never keeps a dangling key.
func (s *Store) removeKey(key string) {
	delete(s.data, key)
	delete(s.expires, key)
	s.evictionUntrack(key)
}
