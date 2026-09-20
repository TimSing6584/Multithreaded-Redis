// Package command holds the "CPU work" of the server: the in-memory store
// and command dispatch. It has no notion of sockets, goroutines, or event
// loops, so any I/O strategy (thread pool, single-threaded, io-multiplexed)
// can call Handle the same way.
package command

import (
	"fmt"
	"strings"
	"sync"

	"Redis-k02/Part3/internal/protocol"
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
		return protocol.NilReply
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
			return protocol.NilReply
		}
		return encodeBulkString(v)
	default:
		return encodeError("ERR unknown command")
	}
}
