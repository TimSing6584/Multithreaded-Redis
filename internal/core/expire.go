// This file handles key expiration for every type of value: expires map, active/passive delete mode
package core

import (
	"Redis-k02/internal/config"
	"time"
)

// expires holds the expiration time of every key that has a TTL. A key without a TTL
// is simply not in this map, which makes active deletion more efficient because we only sample from keys that have a TTL.
var expires = make(map[string]time.Time)

// SetExpire sets the expiration time of key. The caller must make sure key exists.
func SetExpire(key string, at time.Time) {
	expires[key] = at
}

// GetExpire returns the expiration time of key, and false if it has no TTL.
func GetExpire(key string) (time.Time, bool) {
	at, ok := expires[key]
	return at, ok
}

// Lazy delete helper function. Used as passive mode to delete expired keys when they are accessed.
// Called by Get, so every command that reads a key goes through it.
func expireIfNeeded(key string) bool {
	at, ok := expires[key]
	if ok && time.Now().After(at) {
		removeKey(key)
		return true
	}
	return false
}

// Active mode to delete expired keys
// Every a period of time (for example 100ms), sample a fixed small number of keys (for example 20),
// then check if they are expired, if yes delete them. If the number of deleted keys exceed out threshold (for example 20%)
// then we repeat the cycle. The sample size and threshold live in the config package.

func ActiveExpireDelete(budget time.Duration) {
	start := time.Now()
	for {
		now := time.Now()
		sampled, deleted := 0, 0
		// In go, mapiteration order is random, which is good because we dont want to always follow one order
		// For the first 20 keys have very long expiry time and the rest are mostly expired
		// So this loop only scans first 20 keys and obeserve it does not exceed the threshold -> can't actively remove expired ones
		// We only sample from expires, so keys without TTL are never wasted samples.
		for key, at := range expires {
			if sampled >= config.ActiveExpireSampleSize {
				break
			}
			if !now.Before(at) {
				removeKey(key)
				deleted++
			}
			sampled++
		}
		if time.Since(start) > budget || sampled == 0 || float64(deleted)/float64(sampled) < config.ActiveExpireThreshold {
			return
		}
	}
}
