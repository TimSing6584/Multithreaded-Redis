package command

import (
	"Redis-k02/internal/core"
	"Redis-k02/internal/datastructure"
	"Redis-k02/internal/resp"
	"strconv"
)

// Error messages follow RedisBloom's wording for the CMS.* commands.
const (
	cmsKeyExistsErr    = "CMS: key already exists"
	cmsKeyNotExistsErr = "CMS: key does not exist"
)

func handleCMSInitByDim(args []string) []byte {
	if len(args) != 3 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'CMS.INITBYDIM'"))
	}
	key := args[0]
	obj := core.Get(key)
	// check if the key already holds a different type of datastructure
	if err := checkType(obj, core.KindCMS); err != nil {
		return err
	}
	if obj != nil {
		return resp.EncodeReply(resp.Error(cmsKeyExistsErr))
	}
	width, err := strconv.ParseUint(args[1], 10, 32)
	if err != nil || width == 0 {
		return resp.EncodeReply(resp.Error("CMS: invalid width"))
	}
	depth, err := strconv.ParseUint(args[2], 10, 32)
	if err != nil || depth == 0 {
		return resp.EncodeReply(resp.Error("CMS: invalid depth"))
	}
	cms := datastructure.NewCMS(uint32(width), uint32(depth))
	core.Set(key, &core.Object{Kind: core.KindCMS, Value: cms})
	return resp.EncodeReply(resp.SimpleString("OK"))
}

func handleCMSInitByProb(args []string) []byte {
	if len(args) != 3 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'CMS.INITBYPROB'"))
	}
	key := args[0]
	obj := core.Get(key)
	// check if the key already holds a different type of datastructure
	if err := checkType(obj, core.KindCMS); err != nil {
		return err
	}
	if obj != nil {
		return resp.EncodeReply(resp.Error(cmsKeyExistsErr))
	}
	// Both values are fractions strictly between 0 and 1: 0 would need an
	// infinite sketch, 1 would make the sketch meaningless.
	errRate, err := strconv.ParseFloat(args[1], 64)
	if err != nil || errRate <= 0 || errRate >= 1 {
		return resp.EncodeReply(resp.Error("CMS: invalid overestimation value"))
	}
	errProb, err := strconv.ParseFloat(args[2], 64)
	if err != nil || errProb <= 0 || errProb >= 1 {
		return resp.EncodeReply(resp.Error("CMS: invalid prob value"))
	}
	width, depth := datastructure.CalcCMSDim(errRate, errProb)
	cms := datastructure.NewCMS(width, depth)
	core.Set(key, &core.Object{Kind: core.KindCMS, Value: cms})
	return resp.EncodeReply(resp.SimpleString("OK"))
}

func handleCMSIncrBy(args []string) []byte {
	if len(args) < 3 || len(args)%2 != 1 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'CMS.INCRBY'"))
	}
	key := args[0]
	obj := core.Get(key)
	// check if the key already holds a different type of datastructure
	if err := checkType(obj, core.KindCMS); err != nil {
		return err
	}
	// Unlike ZADD, the sketch must be created explicitly with an INITBY* command,
	// since its dimensions can't be guessed.
	if obj == nil {
		return resp.EncodeReply(resp.Error(cmsKeyNotExistsErr))
	}

	// Parse every (item, increment) pair before mutating anything, so a bad
	// increment partway through the command doesn't leave a half-applied INCRBY.
	type pair struct {
		item  string
		value uint32
	}
	pairs := make([]pair, 0, (len(args)-1)/2)
	for i := 1; i < len(args); i += 2 {
		value, err := strconv.ParseUint(args[i+1], 10, 32)
		if err != nil {
			return resp.EncodeReply(resp.Error("CMS: Cannot parse number"))
		}
		pairs = append(pairs, pair{item: args[i], value: uint32(value)})
	}

	cms := obj.Value.(*datastructure.CMS)
	values := make([]resp.Value, len(pairs))
	for i, p := range pairs {
		values[i] = resp.Integer(cms.IncrBy(p.item, p.value))
	}
	return resp.EncodeReply(resp.Array(values))
}

func handleCMSQuery(args []string) []byte {
	if len(args) < 2 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'CMS.QUERY'"))
	}
	key := args[0]
	obj := core.Get(key)
	// check if the key already holds a different type of datastructure
	if err := checkType(obj, core.KindCMS); err != nil {
		return err
	}
	if obj == nil {
		return resp.EncodeReply(resp.Error(cmsKeyNotExistsErr))
	}
	cms := obj.Value.(*datastructure.CMS)
	values := make([]resp.Value, len(args)-1)
	for i, item := range args[1:] {
		values[i] = resp.Integer(cms.Query(item))
	}
	return resp.EncodeReply(resp.Array(values))
}

func handleCMSInfo(args []string) []byte {
	if len(args) != 1 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'CMS.INFO'"))
	}
	key := args[0]
	obj := core.Get(key)
	// check if the key already holds a different type of datastructure
	if err := checkType(obj, core.KindCMS); err != nil {
		return err
	}
	if obj == nil {
		return resp.EncodeReply(resp.Error(cmsKeyNotExistsErr))
	}
	width, depth, count := obj.Value.(*datastructure.CMS).Info()
	// Reply as a flat field/value array, like RedisBloom: width, w, depth, d, count, c.
	return resp.EncodeReply(resp.Array{
		resp.BulkString("width"), resp.Integer(width),
		resp.BulkString("depth"), resp.Integer(depth),
		resp.BulkString("count"), resp.Integer(count),
	})
}
