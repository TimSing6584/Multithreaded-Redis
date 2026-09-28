// This file execute the parsed command and return the reply to the client.
package command

import (
	"Redis-k02/Part4/internal/resp"
	"math"
	"strconv"
	"strings"
	"time"
)

// A simple in-memory key-value store to hold the data.
var store = make(map[string]valueSet)

// This stores value plus its optional expiration variables.
// If hasExpirey is true, then expireAt holds the expiration time. If hasExpiry is false, then expireAt is ignored.
type valueSet struct {
	value     string
	hasExpiry bool
	expireAt  time.Time
}

func Handle(cmd resp.Command) []byte {
	if cmd.Name == "" {
		return resp.NilReply
	}
	switch strings.ToUpper(cmd.Name) {
	case "PING":
		return pingHandler(cmd.Args)
	case "SET":
		return setHandler(cmd.Args)
	case "GET":
		return getHandler(cmd.Args)
	case "TTL":
		return ttlHandler(cmd.Args, time.Second)
	case "PTTL":
		return ttlHandler(cmd.Args, time.Millisecond)
	case "EXPIRE":
		return expireHandler(cmd.Args)
	case "DEL":
		return delHandler(cmd.Args)
	case "EXISTS":
		return existsHandler(cmd.Args)

	default:
		return resp.EncodeReply(resp.Error("ERR unknown command"))
	}
}

// Active mode to delete expired keys
// Every a period of time (for example 100ms), sample a fixed small number of keys (for example 20),
// then check if they are expired, if yes delete them. If the number of deleted keys exceed out threshold (for example 20%)
// then we repeat the cycle

const (
	activeExpireSampleSize = 20
	activeExpireThreshold  = 0.25
)

func ActiveExpireDelete(budget time.Duration) {
	start := time.Now()
	for {
		now := time.Now()
		sampled, deleted := 0, 0
		// In go, mapiteration order is random, which is good because we dont want to always follow one order
		// For the first 20 keys have very long expiry time and the rest are mostly expired
		// So this loop only scans first 20 keys and obeserve it does not exceed the threshold -> can't actively remove expired ones
		for key, v := range store {
			if sampled >= activeExpireSampleSize {
				break
			}
			if v.hasExpiry && !now.Before(v.expireAt) {
				delete(store, key)
				deleted++
			}
			sampled++
		}
		if time.Since(start) > budget || sampled == 0 || float64(deleted)/float64(sampled) < activeExpireThreshold {
			return
		}
	}
}

// Lazy delete helper function. Used as passive mode to delete expired keys when they are accessed.
// Currently used in GET, EXISTS, TTL, EXPIRE
func lazyDeleteIfExpired(v valueSet, key string) bool {
	if v.hasExpiry && time.Now().After(v.expireAt) {
		delete(store, key)
		return true
	}
	return false
}

func pingHandler(args []string) []byte {
	if len(args) > 1 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'PING'"))
	}
	if len(args) == 1 {
		return resp.EncodeReply(resp.BulkString(args[0]))
	}
	return resp.EncodeReply(resp.SimpleString("PONG"))
}

func setHandler(args []string) []byte {
	if len(args) < 2 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'SET'"))
	}
	// Set the value for the key:
	key, value := args[0], args[1]
	// Initialize the valueSet with the value and default expiration settings:
	v := valueSet{value: value}

	// Handle optional flags like EX, PX,... :
	for i := 2; i < len(args); i++ {
		switch strings.ToUpper(args[i]) {
		case "EX", "PX":
			if i+1 >= len(args) {
				return resp.EncodeReply(resp.Error("ERR syntax error"))
			}
			duration, err := strconv.ParseInt(args[i+1], 10, 64)
			if err != nil || duration <= 0 {
				return resp.EncodeReply(resp.Error("ERR invalid expire time in SET command"))
			}
			timeUnit := time.Second
			if strings.ToUpper(args[i]) == "PX" {
				timeUnit = time.Millisecond
			}
			v.hasExpiry = true
			v.expireAt = time.Now().Add(time.Duration(duration) * timeUnit)
			i++ // Skip the next argument since it's the duration value
		default:
			return resp.EncodeReply(resp.Error("ERR syntax error"))
		}

	}
	store[key] = v
	return resp.EncodeReply(resp.SimpleString("OK"))
}

func getHandler(args []string) []byte {
	if len(args) != 1 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'GET'"))
	}
	v, ok := store[args[0]]
	if !ok {
		return resp.NilReply
	}
	// Passive mode to delete expired keys: if the key is expired, we delete it and return nil.
	if lazyDeleteIfExpired(v, args[0]) {
		return resp.NilReply
	}
	return resp.EncodeReply(resp.BulkString(v.value))
}

func ttlHandler(args []string, timeUnit time.Duration) []byte {
	if len(args) != 1 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'TTL'"))
	}
	v, ok := store[args[0]]
	if !ok || lazyDeleteIfExpired(v, args[0]) {
		return resp.EncodeReply(resp.Integer(-2))
	}
	if !v.hasExpiry {
		return resp.EncodeReply(resp.Integer(-1))
	}
	remaining := time.Until(v.expireAt)
	if remaining < 0 {
		remaining = 0
	}
	return resp.EncodeReply(resp.Integer((remaining + timeUnit - 1) / timeUnit))

}

func delHandler(args []string) []byte {
	if len(args) < 1 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'DEL'"))
	}
	// Count the number of keys that are deleted
	count := 0
	for i := 0; i < len(args); i++ {
		_, ok := store[args[i]]
		if ok {
			delete(store, args[i])
			count++
		}
	}
	return resp.EncodeReply(resp.Integer(count))
}

func existsHandler(args []string) []byte {
	if len(args) < 1 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'EXISTS'"))
	}
	// Count the number of keys that exist
	count := 0
	for i := 0; i < len(args); i++ {
		v, ok := store[args[i]]
		// Passive mode to delete expired keys: if the key is expired, we delete it when we access.
		if !ok || lazyDeleteIfExpired(v, args[i]) {
			continue
		}
		count++
	}
	return resp.EncodeReply(resp.Integer(count))
}

// maxExpireSeconds bounds EXPIRE's seconds argument so seconds*time.Second
// cannot silently overflow int64 - anything past this is already an
// unreachable expiration date, so it is rejected rather than wrapped.
const maxExpireSeconds = math.MaxInt64 / int64(time.Second)

func expireHandler(args []string) []byte {
	if len(args) < 2 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'EXPIRE'"))
	}
	seconds, err := strconv.ParseInt(args[1], 10, 64)

	if err != nil {
		return resp.EncodeReply(resp.Error("ERR value is not an integer or out of range"))
	}
	if seconds > maxExpireSeconds || seconds < -maxExpireSeconds {
		return resp.EncodeReply(resp.Error("ERR value is not an integer or out of range"))
	}
	v, ok := store[args[0]]
	if !ok || lazyDeleteIfExpired(v, args[0]) {
		return resp.EncodeReply(resp.Integer(0))
	}
	now := time.Now()
	expireAt := now.Add(time.Duration(seconds) * time.Second)
	if !expireAt.After(now) {
		// A non-positive timeout means "expire right now": real Redis
		// deletes the key immediately rather than storing a past
		// expireAt for the next lazy/active sweep to find.
		delete(store, args[0])
		return resp.EncodeReply(resp.Integer(1))
	}
	v.expireAt = expireAt
	v.hasExpiry = true
	store[args[0]] = v
	return resp.EncodeReply(resp.Integer(1))
}
