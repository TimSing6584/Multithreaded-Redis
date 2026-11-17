// Package config holds every tunable number of the server in one place.
// It imports nothing from the project, so any package can use it without an import cycle.
package config

import "time"

// SERVER :

const Port = ":4000"

// EVICTION :

type EvictionPolicy int

const (
	PolicyRandom EvictionPolicy = iota
	PolicyLRU
	PolicyApproxLRU
)

// MaxKeys and Policy are chosen at startup (var, not const, so they could be set from flags later).
// Don't switch Policy at runtime: the LRU list is only maintained under PolicyLRU,
// so it would be out of sync with the store.
var (
	MaxKeys = 1000
	Policy  = PolicyApproxLRU
)

// Same numbers as Redis: each eviction samples 5 keys into a pool that keeps the 16 best candidates.
const (
	EvictionPoolSize   = 16
	EvictionSampleSize = 5
)

// ACTIVE EXPIRY :

// Every sweep samples ActiveExpireSampleSize keys that have a TTL and deletes the expired ones.
// If more than ActiveExpireThreshold of the sample was expired, the sweep repeats.
const (
	ActiveExpireSampleSize = 20
	ActiveExpireThreshold  = 0.25
)

// ActiveExpireInterval bounds how long multiplexer.Wait() blocks, so the
// loop doesn't sit idle waiting for connection I/O forever - it wakes up
// on its own at least this often and gets a chance to run the active
// expiry sweep (actively delete expired keys if they were set EX in SET command)
const ActiveExpireInterval = 100 * time.Millisecond

// ActiveExpireBudget bounds how long a single active-expire sweep can run,
// so a store full of expired keys can't block the event loop for the
// whole sweep - see core.ActiveExpireDelete.
const ActiveExpireBudget = 1 * time.Millisecond
