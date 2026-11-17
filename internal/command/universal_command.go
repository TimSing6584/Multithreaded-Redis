// universal_command.go handles the commands that work on a key whatever type of
// value it holds: DEL, EXISTS, TTL/PTTL, EXPIRE.
package command

import (
	"Redis-k02/internal/core"
	"Redis-k02/internal/resp"
	"math"
	"strconv"
	"time"
)

func handleDel(store *core.Store, args []string) []byte {
	if len(args) < 1 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'DEL'"))
	}
	// Count the number of keys that are deleted
	count := 0
	for _, key := range args {
		if store.Delete(key) {
			count++
		}
	}
	return resp.EncodeReply(resp.Integer(count))
}

func handleExists(store *core.Store, args []string) []byte {
	if len(args) < 1 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'EXISTS'"))
	}
	// Count the number of keys that exist. A repeated key is counted each time, like real Redis.
	count := 0
	for _, key := range args {
		if store.Exists(key) {
			count++
		}
	}
	return resp.EncodeReply(resp.Integer(count))
}

func handleTTL(store *core.Store, args []string, timeUnit time.Duration) []byte {
	if len(args) != 1 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'TTL'"))
	}
	// Peek: reading a key's TTL doesn't count as an access for eviction, like real Redis.
	if store.Peek(args[0]) == nil {
		return resp.EncodeReply(resp.Integer(-2))
	}
	expireAt, ok := store.GetExpire(args[0])
	if !ok {
		return resp.EncodeReply(resp.Integer(-1))
	}
	remaining := time.Until(expireAt)
	if remaining < 0 {
		remaining = 0
	}
	return resp.EncodeReply(resp.Integer((remaining + timeUnit - 1) / timeUnit))
}

// maxExpireSeconds bounds EXPIRE's seconds argument so seconds*time.Second
// cannot silently overflow int64 - anything past this is already an
// unreachable expiration date, so it is rejected rather than wrapped.
const maxExpireSeconds = math.MaxInt64 / int64(time.Second)

func handleExpire(store *core.Store, args []string) []byte {
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
	key := args[0]
	if store.Get(key) == nil {
		return resp.EncodeReply(resp.Integer(0))
	}
	now := time.Now()
	expireAt := now.Add(time.Duration(seconds) * time.Second)
	if !expireAt.After(now) {
		// A non-positive timeout means "expire right now": real Redis
		// deletes the key immediately rather than storing a past
		// expireAt for the next lazy/active sweep to find.
		store.Delete(key)
		return resp.EncodeReply(resp.Integer(1))
	}
	store.SetExpire(key, expireAt)
	return resp.EncodeReply(resp.Integer(1))
}
