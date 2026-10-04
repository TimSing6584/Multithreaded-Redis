// This file execute the parsed command and return the reply to the client.
package command

import (
	"Redis-k02/Part5/internal/resp"
	"Redis-k02/Part5/internal/datastructure"
	"strings"
	"time"
)

// wrongTypeErr matches real Redis's error for e.g. SADD against a key
// that already holds a string.
const wrongTypeErr = "WRONGTYPE Operation against a key holding the wrong kind of value"

// checkType returns a WRONGTYPE reply if obj (from datastructure.Get) is a
// different kind, or nil if the caller may proceed (obj is nil, i.e. the key
// is absent, or already the right kind).
func checkType(obj *datastructure.Object, want datastructure.Kind) []byte {
	if obj != nil && obj.Kind != want {
		return resp.EncodeReply(resp.Error(wrongTypeErr))
	}
	return nil
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


func Handle(cmd resp.Command) []byte {
	if cmd.Name == "" {
		return resp.NilReply
	}
	switch strings.ToUpper(cmd.Name) {
	case "PING":
		return pingHandler(cmd.Args)
	case "SET":
		return handleSet(cmd.Args)
	case "GET":
		return handleGet(cmd.Args)
	case "TTL":
		return handleTTL(cmd.Args, time.Second)
	case "PTTL":
		return handleTTL(cmd.Args, time.Millisecond)
	case "EXPIRE":
		return handleExpire(cmd.Args)
	case "DEL":
		return handleDel(cmd.Args)
	case "EXISTS":
		return handleExists(cmd.Args)
	case "SADD":
		return handleSAdd(cmd.Args)
	case "SREM":
		return handleSRem(cmd.Args)
	case "SISMEMBER":
		return handleSIsMember(cmd.Args)
	case "SMEMBERS":
		return handleSMembers(cmd.Args)

	default:
		return resp.EncodeReply(resp.Error("ERR unknown command"))
	}
}
