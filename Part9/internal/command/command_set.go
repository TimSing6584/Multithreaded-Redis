package command

import (
	"Redis-k02/Part9/internal/core"
	"Redis-k02/Part9/internal/datastructure"
	"Redis-k02/Part9/internal/resp"
)

func handleSAdd(store *core.Store, args []string) []byte {
	if len(args) < 2 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'SADD'"))
	}
	key := args[0]
	obj := store.Get(key)
	// check if the key already holds a different type of datastructure
	if err := checkType(obj, core.KindSet); err != nil {
		return err
	}
	if obj == nil {
		obj = &core.Object{Kind: core.KindSet, Value: datastructure.NewSimpleSet()}
		store.Set(key, obj)
	}
	// Adding to an existing set mutates it in place, so the key keeps its TTL.
	added := obj.Value.(*datastructure.SimpleSet).Add(args[1:]...)
	return resp.EncodeReply(resp.Integer(added))
}

func handleSRem(store *core.Store, args []string) []byte {
	if len(args) < 2 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'SREM'"))
	}
	key := args[0]
	obj := store.Get(key)
	// check if the key already holds a different type of datastructure
	if err := checkType(obj, core.KindSet); err != nil {
		return err
	}
	if obj == nil {
		return resp.EncodeReply(resp.Integer(0))
	}
	set := obj.Value.(*datastructure.SimpleSet)
	removed := set.Remove(args[1:]...)
	// An empty set is removed from the keyspace (with its TTL), like real Redis.
	if set.Len() == 0 {
		store.Delete(key)
	}
	return resp.EncodeReply(resp.Integer(removed))
}

func handleSIsMember(store *core.Store, args []string) []byte {
	if len(args) != 2 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'SISMEMBER'"))
	}
	key, member := args[0], args[1]
	obj := store.Get(key)
	// check if the key already holds a different type of datastructure
	if err := checkType(obj, core.KindSet); err != nil {
		return err
	}
	if obj == nil || !obj.Value.(*datastructure.SimpleSet).IsMember(member) {
		return resp.EncodeReply(resp.Integer(0))
	}
	return resp.EncodeReply(resp.Integer(1))
}

func handleSMembers(store *core.Store, args []string) []byte {
	if len(args) != 1 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'SMEMBERS'"))
	}
	key := args[0]
	obj := store.Get(key)
	// check if the key already holds a different type of datastructure
	if err := checkType(obj, core.KindSet); err != nil {
		return err
	}
	if obj == nil {
		return resp.EncodeReply(resp.Array(nil))
	}
	members := obj.Value.(*datastructure.SimpleSet).Members()

	// Convert the members to RESP generic values (BulkString) for encoding as an array.
	values := make([]resp.Value, len(members))
	for i, member := range members {
		values[i] = resp.BulkString(member)
	}
	return resp.EncodeReply(resp.Array(values))
}
