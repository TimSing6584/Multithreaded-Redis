package command

import (
	"Redis-k02/Part9/internal/core"
	"Redis-k02/Part9/internal/datastructure"
	"Redis-k02/Part9/internal/resp"
	"strconv"
)

func handleZAdd(args []string) []byte {
	if len(args) < 3 || len(args)%2 != 1 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'ZADD'"))
	}
	key := args[0]
	obj := core.Get(key)
	// check if the key already holds a different type of datastructure
	if err := checkType(obj, core.KindZSet); err != nil {
		return err
	}
	if obj == nil {
		obj = &core.Object{Kind: core.KindZSet, Value: datastructure.NewSortedSet(4)}
		core.Set(key, obj)
	}

	// Parse every (score, member) pair before mutating anything, so a bad
	// score partway through the command doesn't leave a half-applied ZADD.
	type pair struct {
		score  float64
		member string
	}
	pairs := make([]pair, 0, (len(args)-1)/2)
	for i := 1; i < len(args); i += 2 {
		score, err := strconv.ParseFloat(args[i], 64)
		if err != nil {
			return resp.EncodeReply(resp.Error("ERR value is not a valid float"))
		}
		pairs = append(pairs, pair{score: score, member: args[i+1]})
	}

	var added int64
	for _, p := range pairs {
		added += int64(obj.Value.(*datastructure.ZSet).Add(p.score, p.member))
	}
	return resp.EncodeReply(resp.Integer(added))
}

func handleZRem(args []string) []byte {
	if len(args) < 2 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'ZREM'"))
	}
	key := args[0]
	obj := core.Get(key)
	// check if the key already holds a different type of datastructure
	if err := checkType(obj, core.KindZSet); err != nil {
		return err
	}
	if obj == nil {
		return resp.EncodeReply(resp.Integer(0))
	}
	zset := obj.Value.(*datastructure.ZSet)
	var removed int64
	for _, member := range args[1:] {
		removed += int64(zset.Rem(member))
	}
	return resp.EncodeReply(resp.Integer(removed))
}

func handleZScore(args []string) []byte {
	if len(args) != 2 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'ZSCORE'"))
	}
	key, member := args[0], args[1]
	obj := core.Get(key)
	// check if the key already holds a different type of datastructure
	if err := checkType(obj, core.KindZSet); err != nil {
		return err
	}
	if obj == nil {
		return resp.NilReply
	}
	score, exists := obj.Value.(*datastructure.ZSet).Score(member)
	if !exists {
		return resp.NilReply
	}
	return resp.EncodeReply(resp.BulkString(strconv.FormatFloat(score, 'f', -1, 64)))

}

func handleZRank(args []string) []byte {
	if len(args) != 2 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'ZRANK'"))
	}
	key, member := args[0], args[1]
	obj := core.Get(key)
	// check if the key already holds a different type of datastructure
	if err := checkType(obj, core.KindZSet); err != nil {
		return err
	}
	if obj == nil {
		return resp.NilReply
	}
	rank, exists := obj.Value.(*datastructure.ZSet).Rank(member)
	if !exists {
		return resp.NilReply
	}
	return resp.EncodeReply(resp.Integer(rank))
}
