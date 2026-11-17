// This file handles key expiration for every type of value: expires map, active/passive delete mode
package core

import (
	"Redis-k02/Part9/internal/config"
	"time"
)

// s.expires holds the expiration time of every key that has a TTL. A key without a TTL
// is simply not in this map, which makes active deletion more efficient because we only sample from keys that have a TTL.

// SetExpire sets the expiration time of key. The caller must make sure key exists.
func (s *Store) SetExpire(key string, at time.Time) {
	s.expires[key] = at
}

// GetExpire returns the expiration time of key, and false if it has no TTL.
func (s *Store) GetExpire(key string) (time.Time, bool) {
	at, ok := s.expires[key]
	return at, ok
}

// Lazy delete helper function. Used as passive mode to delete expired keys when they are accessed.
// Called by Get, so every command that reads a key goes through it.
func (s *Store) lazyDelete(key string) bool {
	at, ok := s.expires[key]
	if ok && time.Now().After(at) {
		s.removeKey(key)
		return true
	}
	return false
}

// Active mode to delete expired keys
// Every a period of time (for example 100ms), sample a fixed small number of keys (for example 20),
// then check if they are expired, if yes delete them. If the number of deleted keys exceed out threshold (for example 20%)
// then we repeat the cycle. The sample size and threshold live in the config package.
//
// Who calls it and when depends on the server: the single-thread server runs it from its
// event loop after Wait() returns, while in the multithread server every worker runs it
// on its own Store from a ticker.
func (s *Store) ActiveExpireDelete(budget time.Duration) {
	start := time.Now()
	for {
		now := time.Now()
		sampled, deleted := 0, 0
		// In go, mapiteration order is random, which is good because we dont want to always follow one order
		// For the first 20 keys have very long expiry time and the rest are mostly expired
		// So this loop only scans first 20 keys and obeserve it does not exceed the threshold -> can't actively remove expired ones
		// We only sample from expires, so keys without TTL are never wasted samples.
		for key, at := range s.expires {
			if sampled >= config.ActiveExpireSampleSize {
				break
			}
			if !now.Before(at) {
				s.removeKey(key)
				deleted++
			}
			sampled++
		}
		if time.Since(start) > budget || sampled == 0 || float64(deleted)/float64(sampled) < config.ActiveExpireThreshold {
			return
		}
	}
}
