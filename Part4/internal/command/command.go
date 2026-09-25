// This file execute the parsed command and return the reply to the client.
package command

import (
	"strings"
	"sync"
	"Redis-k02/Part4/internal/resp"
)

var store = struct {
	sync.Mutex
	data map[string]string
}{data: make(map[string]string)}

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
	default:
		return resp.EncodeReply(resp.Error("ERR unknown command"))
	}
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
	store.Lock()
	store.data[args[0]] = args[1]
	store.Unlock()
	return resp.EncodeReply(resp.SimpleString("OK"))
}

func getHandler(args []string) []byte {
	if len(args) < 1 {
		return resp.EncodeReply(resp.Error("ERR wrong number of arguments for 'GET'"))
	}
	store.Lock()
	v, ok := store.data[args[0]]
	store.Unlock()
	if !ok {
		return resp.NilReply
	}
	return resp.EncodeReply(resp.BulkString(v))
}
