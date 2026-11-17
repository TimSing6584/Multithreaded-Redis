// command_string.go handles the plain-string commands: SET, GET.
package command

import (
	"Redis-k02/internal/datastructure"
	"Redis-k02/internal/resp"
	"strconv"
	"strings"
	"time"
)

func handleSet(args []string) []byte {
	if len(args) < 2 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'SET'"))
	}
	// Set the value for the key:
	key, value := args[0], args[1]
	// Zero expireAt means no EX/PX flag was given.
	var expireAt time.Time

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
			expireAt = time.Now().Add(time.Duration(duration) * timeUnit)
			i++ // Skip the next argument since it's the duration value
		default:
			return resp.EncodeReply(resp.Error("ERR syntax error"))
		}

	}
	// Set overwrites whatever type the key held before and clears its old TTL.
	datastructure.Set(key, &datastructure.Object{Kind: datastructure.KindString, Value: value})
	if !expireAt.IsZero() {
		datastructure.SetExpire(key, expireAt)
	}
	return resp.EncodeReply(resp.SimpleString("OK"))
}

func handleGet(args []string) []byte {
	if len(args) != 1 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'GET'"))
	}
	// Get applies lazy expiry: an expired key comes back as nil.
	obj := datastructure.Get(args[0])
	if obj == nil {
		return resp.NilReply
	}
	// check if the key already holds a different type of datastructure
	if err := checkType(obj, datastructure.KindString); err != nil {
		return err
	}
	return resp.EncodeReply(resp.BulkString(obj.Value.(string)))
}
