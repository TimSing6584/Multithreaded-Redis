package command

import (
	"Redis-k02/internal/core"
	"Redis-k02/internal/datastructure"
	"Redis-k02/internal/resp"
	"strconv"
)

// Defaults used when BF.ADD / BF.MADD create a filter on a missing key,
// the same values RedisBloom uses.
const (
	bfDefaultErrorRate = 0.01
	bfDefaultCapacity  = 100

	// bfMaxBytes caps one filter's bit array at 512MB (Redis's max string size),
	// so a huge BF.RESERVE gets an error instead of crashing the server.
	bfMaxBytes = 512 << 20
)

// getOrCreateBloom returns the filter at key, creating one with the default
// error rate and capacity if the key is absent. errReply is non-nil if the
// key holds a different type.
func getOrCreateBloom(key string) (bf *datastructure.BloomFilter, errReply []byte) {
	obj := core.Get(key)
	// check if the key already holds a different type of datastructure
	if err := checkType(obj, core.KindBloom); err != nil {
		return nil, err
	}
	if obj == nil {
		obj = &core.Object{
			Kind:  core.KindBloom,
			Value: datastructure.NewBloomFilter(bfDefaultCapacity, bfDefaultErrorRate),
		}
		core.Set(key, obj)
	}
	return obj.Value.(*datastructure.BloomFilter), nil
}

func handleBFReserve(args []string) []byte {
	if len(args) != 3 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'BF.RESERVE'"))
	}
	key := args[0]
	obj := core.Get(key)
	// check if the key already holds a different type of datastructure
	if err := checkType(obj, core.KindBloom); err != nil {
		return err
	}
	if obj != nil {
		return resp.EncodeReply(resp.Error("ERR item exists"))
	}
	// NewBloomFilter doesn't validate its inputs: an error rate outside (0, 1)
	// gives a nonsense size, and a capacity of 0 divides by zero.
	errorRate, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return resp.EncodeReply(resp.Error("ERR bad error rate"))
	}
	if errorRate <= 0 || errorRate >= 1 {
		return resp.EncodeReply(resp.Error("ERR (0 < error rate range < 1)"))
	}
	capacity, err := strconv.ParseUint(args[2], 10, 64)
	if err != nil {
		return resp.EncodeReply(resp.Error("ERR bad capacity"))
	}
	if capacity == 0 {
		return resp.EncodeReply(resp.Error("ERR (capacity should be larger than 0)"))
	}
	if datastructure.BloomFilterSize(capacity, errorRate) > bfMaxBytes {
		return resp.EncodeReply(resp.Error("ERR filter too large: capacity and error rate need more than 512MB"))
	}
	bf := datastructure.NewBloomFilter(capacity, errorRate)
	core.Set(key, &core.Object{Kind: core.KindBloom, Value: bf})
	return resp.EncodeReply(resp.SimpleString("OK"))
}

func handleBFAdd(args []string) []byte {
	if len(args) != 2 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'BF.ADD'"))
	}
	bf, errReply := getOrCreateBloom(args[0])
	if errReply != nil {
		return errReply
	}
	return resp.EncodeReply(resp.Integer(bf.AddIfNotExist(args[1])))
}

func handleBFMAdd(args []string) []byte {
	if len(args) < 2 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'BF.MADD'"))
	}
	bf, errReply := getOrCreateBloom(args[0])
	if errReply != nil {
		return errReply
	}
	values := make([]resp.Value, len(args)-1)
	for i, item := range args[1:] {
		values[i] = resp.Integer(bf.AddIfNotExist(item))
	}
	return resp.EncodeReply(resp.Array(values))
}

func handleBFExists(args []string) []byte {
	if len(args) != 2 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'BF.EXISTS'"))
	}
	key, item := args[0], args[1]
	obj := core.Get(key)
	// check if the key already holds a different type of datastructure
	if err := checkType(obj, core.KindBloom); err != nil {
		return err
	}
	if obj == nil {
		return resp.EncodeReply(resp.Integer(0))
	}
	return resp.EncodeReply(resp.Integer(obj.Value.(*datastructure.BloomFilter).Exist(item)))
}

func handleBFMExists(args []string) []byte {
	if len(args) < 2 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'BF.MEXISTS'"))
	}
	key := args[0]
	obj := core.Get(key)
	// check if the key already holds a different type of datastructure
	if err := checkType(obj, core.KindBloom); err != nil {
		return err
	}
	// A missing key means nothing was ever added, so every item gets 0.
	values := make([]resp.Value, len(args)-1)
	for i, item := range args[1:] {
		if obj == nil {
			values[i] = resp.Integer(0)
		} else {
			values[i] = resp.Integer(obj.Value.(*datastructure.BloomFilter).Exist(item))
		}
	}
	return resp.EncodeReply(resp.Array(values))
}

func handleBFInfo(args []string) []byte {
	if len(args) != 1 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'BF.INFO'"))
	}
	key := args[0]
	obj := core.Get(key)
	// check if the key already holds a different type of datastructure
	if err := checkType(obj, core.KindBloom); err != nil {
		return err
	}
	if obj == nil {
		return resp.EncodeReply(resp.Error("ERR not found"))
	}
	capacity, errorRate, hashes, size, items := obj.Value.(*datastructure.BloomFilter).Info()
	// Reply as a flat field/value array, like RedisBloom.
	return resp.EncodeReply(resp.Array{
		resp.BulkString("Capacity"), resp.Integer(capacity),
		resp.BulkString("Size"), resp.Integer(size),
		resp.BulkString("Number of items inserted"), resp.Integer(items),
		resp.BulkString("Error rate"), resp.BulkString(strconv.FormatFloat(errorRate, 'f', -1, 64)),
		resp.BulkString("Number of hashes"), resp.Integer(hashes),
	})
}
