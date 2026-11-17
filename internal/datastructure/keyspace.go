// This file defines the keyspace: one map from key to an Object that carries
// both the value and the kind of datastructure it is (e.g., string, set, sorted set,...).
// Plus with functions like get, set, delete a key and check its type,...
package datastructure

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
// KindString -> string, KindSet -> *SimpleSet.
type Object struct {
	Kind  Kind
	Value any
}

// store holds every key, whatever type of value it holds. It is unexported so every
// access goes through Get, which applies lazy expiry.
var store = make(map[string]*Object)

// Get returns the Object for key, or nil if the key is absent.
// Passive mode to delete expired keys: an expired key is deleted here and reported as absent.
func Get(key string) *Object {
	if expireIfNeeded(key) {
		return nil
	}
	return store[key]
}

// Set stores obj under key, replacing whatever type the key held before, and clears any leftover TTL
func Set(key string, obj *Object) {
	store[key] = obj
	delete(expires, key)
}

// Delete removes key and its TTL. It reports whether the key existed (and was therefore removed).
func Delete(key string) bool {
	if Get(key) == nil {
		return false
	}
	delete(store, key)
	delete(expires, key)
	return true
}

// Exists reports whether key has a live value.
func Exists(key string) bool {
	return Get(key) != nil
}

// KindOf reports which kind of datastructure key holds, or KindNone if it is absent.
func KindOf(key string) Kind {
	if obj := Get(key); obj != nil {
		return obj.Kind
	}
	return KindNone
}
