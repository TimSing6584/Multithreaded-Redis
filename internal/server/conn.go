// Package server holds the plumbing both server engines share:
//   - Listener (listener.go): the TCP listener socket as a raw fd.
//   - Conns (this file): the table of open client connections of one event loop, and
//     the read -> parse -> execute -> reply path for a readable connection.
//
// The engines themselves live in the subpackages:
//   - single_thread: one event loop that executes every command itself.
//   - multithread:   N IO handlers (event loops) routing commands to M workers.
//
// The only difference between the two in this read path is WHO executes a parsed
// command, so HandleReadable takes it as a function (exec).
package server

import (
	"Redis-k02/internal/resp"
	"bufio"
	"bytes"
	"fmt"
	"io"
	"syscall"
)

// Conns tracks the open client connections of ONE event loop (the single-thread server,
// or one multithread IO handler). It is not safe for concurrent use: only the goroutine
// running that event loop may touch it, so its maps need no mutex.
type Conns struct {
	// pending holds, per connection, the bytes that have been read but not yet processed into a complete command.
	// For example, if the network from the client is slow, we might read only part of a command,
	// and we need to store that until the rest arrives.
	pending map[int][]byte
	// fds tracks every open client fd, so shutdown can close them all.
	fds map[int]struct{}
}

func NewConns() *Conns {
	return &Conns{
		pending: make(map[int][]byte),
		fds:     make(map[int]struct{}),
	}
}

// Add starts tracking connFd. The caller has already registered it with its multiplexer.
func (c *Conns) Add(connFd int) {
	c.fds[connFd] = struct{}{}
}

// Len reports how many connections are open.
func (c *Conns) Len() int {
	return len(c.fds)
}

// HandleReadable reads what is available on connFd, parses every complete command in it,
// and writes exec(cmd) back to the client for each one, in order. Bytes of a command that
// hasn't fully arrived yet are kept in pending for the next call.
func (c *Conns) HandleReadable(connFd int, exec func(resp.Command) []byte) {
	buf := make([]byte, 4096)
	n, err := syscall.Read(connFd, buf)
	if err != nil || n == 0 {
		if err != nil && err != io.EOF {
			fmt.Println("read error:", err)
		}
		c.Close(connFd)
		return
	}

	// Combine any previously leftover data with the newly read data
	combined := append(c.pending[connFd], buf[:n]...)

	// Size the reader to fit all of `combined` at once. That forces bufio to
	// pull the whole thing into its internal buffer on the very first read,
	// so after every successfully-parsed command reader.Buffered() reports
	// exactly how much of combined is still unparsed.
	reader := bufio.NewReaderSize(bytes.NewReader(combined), len(combined))

	// consumed is how many bytes of combined belong to commands that were fully parsed.
	// It is only updated right after a SUCCESSFUL parse.
	//
	// MISTAKE REMINDER: never compute the leftover from reader.Buffered() around a FAILED parse.
	// After a failed ParseCommand the reader has already moved forward (Buffered() = 0), and before
	// the very first ParseCommand bufio hasn't filled its buffer yet (Buffered() = 0 too). Either
	// way the leftover comes out empty and the bytes of the incomplete command are lost.
	consumed := 0

	// This loop helps us handle multiple commands in a single read, which is common when clients send pipelined requests.
	// For example, if a client sends "PING\r\nPING\r\n", we want to process both commands in one go.
	for {
		cmd, err := resp.ParseCommand(reader)
		if err == resp.IncompleteError {
			// Not enough data yet for this command - stash everything from
			// where the last successful command left off, and wait for more data.
			unread := combined[consumed:]
			if len(unread) > 0 {
				c.pending[connFd] = unread
			} else {
				delete(c.pending, connFd)
			}
			return
		}
		if err != nil {
			_, _ = syscall.Write(connFd, []byte("-ERR Protocol error: "+err.Error()+"\r\n"))
			delete(c.pending, connFd) // the stream is corrupt - don't try to resume it
			return
		}
		consumed = len(combined) - reader.Buffered()
		if cmd.Name == "" {
			continue
		}
		reply := exec(cmd)
		if _, err := syscall.Write(connFd, reply); err != nil {
			fmt.Println("write error:", err)
			c.Close(connFd)
			return
		}
	}
}

// Close closes the connection and forgets any leftover bytes buffered
// for it, so a future fd reused by the OS for a new connection doesn't
// inherit a stale partial command.
func (c *Conns) Close(connFd int) {
	_ = syscall.Close(connFd)
	delete(c.pending, connFd)
	delete(c.fds, connFd)
}

// CloseAll closes every client connection. Deleting from a map while
// ranging over it is allowed in Go.
func (c *Conns) CloseAll() {
	for fd := range c.fds {
		c.Close(fd)
	}
}
