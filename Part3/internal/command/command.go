// This file is responsible for handling a set of args that is decoded from the RESP protocol. It will return a response in the RESP format.
// or we can say "Encode the decoded args into a RESP response"
package command

import (
	"fmt"
	"strings"
	"sync"
	"Redis-k02/Part3/internal/resp"
)

func encodeSimpleString(s string) []byte {
	return []byte("+" + s + "\r\n")
}

func encodeError(s string) []byte {
	return []byte("-" + s + "\r\n")
}

func encodeBulkString(s string) []byte {
	return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(s), s))
}

var store = struct {
	sync.Mutex
	data map[string]string
}{data: make(map[string]string)}

func Handle(args []string) []byte {
	if len(args) == 0 {
		return resp.NilReply
	}
	switch strings.ToUpper(args[0]) {
	case "PING":
		return encodeSimpleString("PONG")
	case "SET":
		if len(args) < 3 {
			return encodeError("ERR wrong number of arguments for 'SET'")
		}
		store.Lock()
		store.data[args[1]] = args[2]
		store.Unlock()
		return encodeSimpleString("OK")
	case "GET":
		if len(args) < 2 {
			return encodeError("ERR wrong number of arguments for 'GET'")
		}
		store.Lock()
		v, ok := store.data[args[1]]
		store.Unlock()
		if !ok {
			return resp.NilReply
		}
		return encodeBulkString(v)
	default:
		return encodeError("ERR unknown command")
	}
}
