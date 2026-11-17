// This file execute the parsed command and return the reply to the client.
package command

import (
	"Redis-k02/internal/core"
	"Redis-k02/internal/resp"
	"strings"
	"time"
)

// wrongTypeErr matches real Redis's error for e.g. SADD against a key
// that already holds a string.
const wrongTypeErr = "WRONGTYPE Operation against a key holding the wrong kind of value"

// checkType returns a WRONGTYPE reply if obj (from core.Get) is a
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
	case "ZADD":
		return handleZAdd(cmd.Args)
	case "ZREM":
		return handleZRem(cmd.Args)
	case "ZSCORE":
		return handleZScore(cmd.Args)
	case "ZRANK":
		return handleZRank(cmd.Args)
	case "CMS.INITBYDIM":
		return handleCMSInitByDim(cmd.Args)
	case "CMS.INITBYPROB":
		return handleCMSInitByProb(cmd.Args)
	case "CMS.INCRBY":
		return handleCMSIncrBy(cmd.Args)
	case "CMS.QUERY":
		return handleCMSQuery(cmd.Args)
	case "CMS.INFO":
		return handleCMSInfo(cmd.Args)
	case "BF.RESERVE":
		return handleBFReserve(cmd.Args)
	case "BF.ADD":
		return handleBFAdd(cmd.Args)
	case "BF.MADD":
		return handleBFMAdd(cmd.Args)
	case "BF.EXISTS":
		return handleBFExists(cmd.Args)
	case "BF.MEXISTS":
		return handleBFMExists(cmd.Args)
	case "BF.INFO":
		return handleBFInfo(cmd.Args)
	default:
		return resp.EncodeReply(resp.Error("ERR unknown command"))
	}
}
