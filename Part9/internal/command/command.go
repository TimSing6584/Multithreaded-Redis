// This file execute the parsed command and return the reply to the client.
package command

import (
	"Redis-k02/Part9/internal/core"
	"Redis-k02/Part9/internal/resp"
	"strings"
	"time"
)

// wrongTypeErr matches real Redis's error for e.g. SADD against a key
// that already holds a string.
const wrongTypeErr = "WRONGTYPE Operation against a key holding the wrong kind of value"

// checkType returns a WRONGTYPE reply if obj (from store.Get) is a
// different kind, or nil if the caller may proceed (obj is nil, i.e. the key
// is absent, or already the right kind).
func checkType(obj *core.Object, want core.Kind) []byte {
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

func Handle(store *core.Store, cmd resp.Command) []byte {
	if cmd.Name == "" {
		return resp.NilReply
	}
	switch strings.ToUpper(cmd.Name) {
	case "PING":
		return pingHandler(cmd.Args)
	case "SET":
		return handleSet(store, cmd.Args)
	case "GET":
		return handleGet(store, cmd.Args)
	case "TTL":
		return handleTTL(store, cmd.Args, time.Second)
	case "PTTL":
		return handleTTL(store, cmd.Args, time.Millisecond)
	case "EXPIRE":
		return handleExpire(store, cmd.Args)
	case "DEL":
		return handleDel(store, cmd.Args)
	case "EXISTS":
		return handleExists(store, cmd.Args)
	case "SADD":
		return handleSAdd(store, cmd.Args)
	case "SREM":
		return handleSRem(store, cmd.Args)
	case "SISMEMBER":
		return handleSIsMember(store, cmd.Args)
	case "SMEMBERS":
		return handleSMembers(store, cmd.Args)
	case "ZADD":
		return handleZAdd(store, cmd.Args)
	case "ZREM":
		return handleZRem(store, cmd.Args)
	case "ZSCORE":
		return handleZScore(store, cmd.Args)
	case "ZRANK":
		return handleZRank(store, cmd.Args)
	case "CMS.INITBYDIM":
		return handleCMSInitByDim(store, cmd.Args)
	case "CMS.INITBYPROB":
		return handleCMSInitByProb(store, cmd.Args)
	case "CMS.INCRBY":
		return handleCMSIncrBy(store, cmd.Args)
	case "CMS.QUERY":
		return handleCMSQuery(store, cmd.Args)
	case "CMS.INFO":
		return handleCMSInfo(store, cmd.Args)
	case "BF.RESERVE":
		return handleBFReserve(store, cmd.Args)
	case "BF.ADD":
		return handleBFAdd(store, cmd.Args)
	case "BF.MADD":
		return handleBFMAdd(store, cmd.Args)
	case "BF.EXISTS":
		return handleBFExists(store, cmd.Args)
	case "BF.MEXISTS":
		return handleBFMExists(store, cmd.Args)
	case "BF.INFO":
		return handleBFInfo(store, cmd.Args)
	default:
		return resp.EncodeReply(resp.Error("ERR unknown command"))
	}
}
