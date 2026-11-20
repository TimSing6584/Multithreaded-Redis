// Package config holds every tunable number of the server in one place.
// It imports nothing from the project, so any package can use it without an import cycle.
package config

import (
	"runtime"
	"time"
)

// SERVER :

const Port = ":4000"

// MULTITHREAD (shared-nothing) SERVER :

// NumIOHandlers (N) and NumWorkers (M) are sized independently: network I/O and command
// execution scale differently. NumWorkers must stay fixed while the server runs, because
// keys are routed with hash(key) % NumWorkers - changing it would remap almost every key.
var (
	NumIOHandlers = max(1, runtime.NumCPU()/2)
	NumWorkers    = max(1, runtime.NumCPU()/2)
)

// IOHandlerPollInterval bounds how long an IO handler's multiplexer.Wait() blocks.
// A handler can't be woken up by the listener directly, so this is how quickly it picks up
// a newly accepted connection handed to it, and how quickly it notices a shutdown.
const IOHandlerPollInterval = 10 * time.Millisecond

// Channel buffer sizes: commands queued for one worker, and accepted connections queued
// for one IO handler before it registers them with its multiplexer.
const (
	WorkerQueueSize  = 1024
	NewConnQueueSize = 128
)

// EVICTION :

type EvictionPolicy int

const (
	PolicyRandom EvictionPolicy = iota
	PolicyLRU
	PolicyApproxLRU
)

// MaxKeys and Policy are chosen at startup (var, not const, so they could be set from flags later).
// MaxKeys is the limit of the whole server: the multithread server splits it evenly
// across its workers' partitions (each gets ceil(MaxKeys / NumWorkers)).
// Don't switch Policy at runtime: the LRU list is only maintained under PolicyLRU,
// so it would be out of sync with the store.
var (
	MaxKeys = 1000000
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
