// One thread, no connection pool - instead the OS multiplexer
// (epoll on Linux, kqueue on macOS) tells us which of the many open file
// descriptors are actually ready to read, and we serve every connection
// from a single event loop.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"syscall"
	"time"
	"Redis-k02/internal/command"
	"Redis-k02/internal/resp"
	"Redis-k02/internal/datastructure"
	"Redis-k02/io_multiplexing"
)

// pending holds, per connection, the bytes that have been read but not yet processed into a complete command.
// For example, if the network from the client is slow, we might read only part of a command,
// and we need to store that until the rest arrives.
var pending = make(map[int][]byte)

// closeConn closes the connection and forgets any leftover bytes buffered
// for it, so a future fd reused by the OS for a new connection doesn't
// inherit a stale partial command.
func closeConn(connFd int) {
	_ = syscall.Close(connFd)
	delete(pending, connFd)
}

func handleReadable(connFd int) {
	buf := make([]byte, 4096)
	n, err := syscall.Read(connFd, buf)
	if err != nil || n == 0 {
		if err != nil && err != io.EOF {
			fmt.Println("read error:", err)
		}
		closeConn(connFd)
		return
	}

	// Combine any previously leftover data with the newly read data
	combined := append(pending[connFd], buf[:n]...)

	// Size the reader to fit all of `combined` at once. That forces bufio to
	// pull the whole thing into its internal buffer on the very first read,
	// so reader.Buffered() - snapshotted before each ParseCommand call below
	// - reports exactly how much of combined is unparsed as of the last
	// successfully-parsed command.
	reader := bufio.NewReaderSize(bytes.NewReader(combined), len(combined))

	// This loop helps us handle multiple commands in a single read, which is common when clients send pipelined requests.
	// For example, if a client sends "PING\r\nPING\r\n", we want to process both commands in one go.
	for {


		// Snapshot how many bytes are left in the reader BEFORE this attempt.

		// MY INTIAL MISTAKE: Snapshot AFTER the ParseCommadn call, which is wrong because while reading the pointer 
		// of the reader is moved forward, even when the command is incomplete, so reader.Buffered() will return 0,
		// and we will lose the unprocessed bytes.
		beforeBuffered := reader.Buffered()

		cmd, err := resp.ParseCommand(reader)
		if err == resp.IncompleteError {
			// Not enough data yet for this command - stash everything from
			// where the last successful command left off, and wait for
			// more data. Don't use reader.Buffered() here (see above).
			unread := combined[len(combined)-beforeBuffered:]
			if len(unread) > 0 {
				pending[connFd] = unread
			} else {
				delete(pending, connFd)
			}
			return
		}
		if err != nil {
			_, _ = syscall.Write(connFd, []byte("-ERR Protocol error: "+err.Error()+"\r\n"))
			delete(pending, connFd) // the stream is corrupt - don't try to resume it
			return
		}
		if cmd.Name == "" {
			continue
		}
		reply := command.Handle(cmd)
		if _, err := syscall.Write(connFd, reply); err != nil {
			fmt.Println("write error:", err)
			closeConn(connFd)
			return
		}
	}
}

func main() {
	listener, err := net.Listen("tcp", ":4000")
	if err != nil {
		fmt.Println("listen error:", err)
		return
	}
	fmt.Println("Server started on port 4000")

	tcpListener := listener.(*net.TCPListener)
	// Retrieve the underlying file descriptor for the listener
	listenerFile, err := tcpListener.File()
	if err != nil {
		fmt.Println("failed to get listener file:", err)
		return
	}
	defer listenerFile.Close()
	// Convert the file descriptor to an int
	serverFd := int(listenerFile.Fd())

	// Create the I/O multiplexer (epoll on Linux, kqueue on macOS)
	multiplexer, err := io_multiplexing.CreateIOMultiplexer()
	if err != nil {
		fmt.Println("failed to create multiplexer:", err)
		return
	}
	defer multiplexer.Close()
	// Let the multiplexer monitor the listener's file descriptor for read events (new connections)
	if err := multiplexer.Monitor(io_multiplexing.Event{Fd: serverFd, Op: io_multiplexing.OpRead}); err != nil {
		fmt.Println("failed to monitor server fd:", err)
		return
	}
	// activeExpiredDeleteInterval bounds how long multiplexer.Wait() blocks, so the
	// loop doesn't sit idle waiting for connection I/O forever - it wakes up
	// on its own at least this often and gets a chance to run the active
	// expiry sweep (actively delete expired keys if they were set EX in SET command)
	const activeExpiredDeleteInterval = 100 * time.Millisecond
	// activeExpireBudget bounds how long a single active-expire sweep can run,
	// so a store full of expired keys can't block the event loop for the
	// whole sweep - see ActiveExpireDelete.
	const activeExpireBudget = 1 * time.Millisecond

	
	lastActiveExpiredDeleteTime := time.Now()

	for {
		events, err := multiplexer.Wait(int(activeExpiredDeleteInterval / time.Millisecond)) // blocking call, returns when at least one fd is ready
		if err != nil {
			fmt.Println("wait error:", err)
			continue
		}

		for _, event := range events {
			// When the listener fd is ready to read, it means a new connection is waiting to be accepted
			if event.Fd == serverFd {
				connFd, _, err := syscall.Accept(serverFd)
				if err != nil {
					fmt.Println("accept error:", err)
					continue
				}
				if err := multiplexer.Monitor(io_multiplexing.Event{Fd: connFd, Op: io_multiplexing.OpRead}); err != nil {
					fmt.Println("failed to monitor conn fd:", err)
					closeConn(connFd)
				}
				continue
			}
			// Otherwise, for a read-ready event on a client connection, handle the readable data
			handleReadable(event.Fd)
		}
		// We only run the active expired keys deletion when the Epoll waits more than activeExpiredDeleteInterval, since it starts Wait()
		// For example, at t=0, Wait() is called, after 15ms, Epoll signals the server there's data from one connection, and that data is processed in 10ms
		// so total 25ms from the start, haven't reached 100ms yet, so we don't run the active expired keys deletion.

		// We only run the active expired keys deletion when the Epoll waits more than activeExpiredDeleteInterval, since it starts Wait(),
		// or we  can say "run the active delete mode when the server is not BUSY"

		if time.Since(lastActiveExpiredDeleteTime) >= activeExpiredDeleteInterval {
			datastructure.ActiveExpireDelete(activeExpireBudget)
			lastActiveExpiredDeleteTime = time.Now()
		}
	}

}
