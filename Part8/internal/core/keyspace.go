// This file defines the keyspace: one map from key to an Object that carries
// both the value and the kind of datastructure it is (e.g., string, set, sorted set,...).
// Plus with functions like get, set, delete a key and check its type,...
package core

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

// store holds every key, whatever type of value it holds. It is unexported so every
// access goes through Get, which applies lazy expiry.
var store = make(map[string]*Object)

// Get returns the Object for key, or nil if the key is absent.
// Passive mode to delete expired keys: an expired key is deleted here and reported as absent.
// Every command reaches a key through Get, so this is also where an access is recorded for eviction.
func Get(key string) *Object {
	return lookup(key, true)
}

// Peek is Get without recording an access, for commands that only inspect a key's metadata
// (EXISTS, TTL/PTTL), like Redis' LOOKUP_NOTOUCH flag. It still applies lazy expiry.
func Peek(key string) *Object {
	return lookup(key, false)
}

func lookup(key string, touchKey bool) *Object {
	if expireIfNeeded(key) {
		return nil
	}
	obj := store[key]
	if obj != nil && touchKey {
		touch(key, obj)
	}
	return obj
}

// Set stores obj under key, replacing whatever type the key held before, and clears any leftover TTL.
// Inserting a new key may evict another one first if the keyspace is full.
func Set(key string, obj *Object) {
	// Overwriting an existing key doesn't grow the keyspace, so only a new key can trigger eviction.
	if _, exists := store[key]; !exists {
		evictIfNeeded()
	}
	store[key] = obj
	delete(expires, key)
	touch(key, obj)
}

// Delete removes key and its TTL. It reports whether the key existed (and was therefore removed).
func Delete(key string) bool {
	// Peek: touching a key right before removing it would be wasted work.
	if Peek(key) == nil {
		return false
	}
	removeKey(key)
	return true
}

// removeKey is the single place a key leaves the store (DEL, expiry, eviction),
// so the eviction bookkeeping never keeps a dangling key.
func removeKey(key string) {
	delete(store, key)
	delete(expires, key)
	untrack(key)
}

// Exists reports whether key has a live value. It doesn't count as an access for eviction.
func Exists(key string) bool {
	return Peek(key) != nil
}

// KindOf reports which kind of datastructure key holds, or KindNone if it is absent.
// It doesn't count as an access for eviction.
func KindOf(key string) Kind {
	if obj := Peek(key); obj != nil {
		return obj.Kind
	}
	return KindNone
}
