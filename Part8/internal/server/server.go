// Package server owns the TCP listener, the I/O-multiplexed event loop, and
// every open client connection.
//
// It runs on two goroutines that never share connection state:
//   - Run (goroutine 1) is the event loop. It is the only one that ever
//     touches pending/connFds, so those maps need no mutex.
//   - Close (goroutine 2, e.g. a signal handler) only flips the closed flag
//     and closes the multiplexer + listener. Run notices the flag and does
//     the actual cleanup of client connections itself.
package server

import (
	"Redis-k02/Part8/internal/command"
	"Redis-k02/Part8/internal/config"
	"Redis-k02/Part8/internal/core"
	"Redis-k02/Part8/internal/resp"
	"Redis-k02/Part8/io_multiplexing"
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type Server struct {
	// listener is the original Go listener. listenerFile is a dup of its fd
	// (that's what TCPListener.File() returns), so BOTH must be closed -
	// closing only the dup leaves the socket open and still queuing new
	// connections.
	listener     net.Listener
	listenerFile *os.File
	listenerFd   int
	mp           io_multiplexing.IOMultiplexer

	// pending holds, per connection, the bytes that have been read but not yet processed into a complete command.
	// For example, if the network from the client is slow, we might read only part of a command,
	// and we need to store that until the rest arrives.
	pending map[int][]byte
	// connFds tracks every open client fd, so shutdown can close them all.
	connFds map[int]struct{}

	closed atomic.Bool
	once   sync.Once
}

// New opens the listener on addr and registers it with a fresh multiplexer.
func New(addr string) (*Server, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	tcpListener := listener.(*net.TCPListener)
	// Retrieve the underlying file descriptor for the listener
	listenerFile, err := tcpListener.File()
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("get listener file: %w", err)
	}
	// Convert the file descriptor to an int
	serverFd := int(listenerFile.Fd())

	// Create the I/O multiplexer (epoll on Linux, kqueue on macOS)
	mp, err := io_multiplexing.CreateIOMultiplexer()
	if err != nil {
		_ = listenerFile.Close()
		_ = listener.Close()
		return nil, fmt.Errorf("create multiplexer: %w", err)
	}
	// Let the multiplexer monitor the listener's file descriptor for read events (new connections)
	if err := mp.Monitor(io_multiplexing.Event{Fd: serverFd, Op: io_multiplexing.OpRead}); err != nil {
		_ = mp.Close()
		_ = listenerFile.Close()
		_ = listener.Close()
		return nil, fmt.Errorf("monitor server fd: %w", err)
	}

	return &Server{
		listener:     listener,
		listenerFile: listenerFile,
		listenerFd:   serverFd,
		mp:           mp,
		pending:      make(map[int][]byte),
		connFds:      make(map[int]struct{}),
	}, nil
}

// Close asks a running Run loop to stop. Meant to be called from another
// goroutine (the signal handler). It never touches pending/connFds - Run's
// own goroutine cleans those up once it sees closed == true.
//
// NOTE: on Linux, closing the epoll fd does NOT wake up an epoll_wait that is
// already blocked in another thread - the blocked call holds its own
// reference to the epoll instance. What actually ends the loop is Wait's
// timeout (config.ActiveExpireInterval): after it returns, Run checks the
// closed flag and exits. So shutdown takes at most ~one interval.
//
// Safe to call more than once.
func (s *Server) Close() error {
	s.once.Do(func() {
		s.closed.Store(true)
		_ = s.mp.Close()
		_ = s.listenerFile.Close()
		_ = s.listener.Close()
	})
	return nil
}

// Run drives the event loop until Close is called.
func (s *Server) Run() error {
	// The active expiry interval and budget are explained in the config package.
	lastActiveExpiredDeleteTime := time.Now()

	for {
		// Check at the top of every iteration, not only on a Wait error: once
		// Close ran, the multiplexer fd number may already be reused by the
		// OS for something else, so we must not call Wait on it again.
		if s.closed.Load() {
			s.shutdown()
			return nil
		}

		events, err := s.mp.Wait(int(config.ActiveExpireInterval / time.Millisecond)) // blocking call, returns when at least one fd is ready
		if err != nil {
			if s.closed.Load() {
				s.shutdown()
				return nil
			}
			fmt.Println("wait error:", err)
			continue
		}

		for _, event := range events {
			// When the listener fd is ready to read, it means a new connection is waiting to be accepted
			if event.Fd == s.listenerFd {
				s.acceptConn()
				continue
			}
			// Otherwise, for a read-ready event on a client connection, handle the readable data
			s.handleReadable(event.Fd)
		}
		// We only run the active expired keys deletion when the Epoll waits more than config.ActiveExpireInterval, since it starts Wait()
		// For example, at t=0, Wait() is called, after 15ms, Epoll signals the server there's data from one connection, and that data is processed in 10ms
		// so total 25ms from the start, haven't reached 100ms yet, so we don't run the active expired keys deletion.

		// We only run the active expired keys deletion when the Epoll waits more than config.ActiveExpireInterval, since it starts Wait(),
		// or we  can say "run the active delete mode when the server is not BUSY"

		if time.Since(lastActiveExpiredDeleteTime) >= config.ActiveExpireInterval {
			core.ActiveExpireDelete(config.ActiveExpireBudget)
			lastActiveExpiredDeleteTime = time.Now()
		}
	}
}

// shutdown runs on Run's goroutine after Close was observed. The loop is
// single-threaded, so any command that was being handled has already
// finished by the time we get here; half-received commands in pending are
// dropped together with their connection.
func (s *Server) shutdown() {
	fmt.Println("shutting down: closing", len(s.connFds), "client connection(s)")
	s.closeAllConns()
}

func (s *Server) acceptConn() {
	connFd, _, err := syscall.Accept(s.listenerFd)
	if err != nil {
		if !s.closed.Load() {
			fmt.Println("accept error:", err)
		}
		return
	}
	if err := s.mp.Monitor(io_multiplexing.Event{Fd: connFd, Op: io_multiplexing.OpRead}); err != nil {
		fmt.Println("failed to monitor conn fd:", err)
		_ = syscall.Close(connFd)
		return
	}
	s.connFds[connFd] = struct{}{}
}

func (s *Server) handleReadable(connFd int) {
	buf := make([]byte, 4096)
	n, err := syscall.Read(connFd, buf)
	if err != nil || n == 0 {
		if err != nil && err != io.EOF {
			fmt.Println("read error:", err)
		}
		s.closeConn(connFd)
		return
	}

	// Combine any previously leftover data with the newly read data
	combined := append(s.pending[connFd], buf[:n]...)

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
				s.pending[connFd] = unread
			} else {
				delete(s.pending, connFd)
			}
			return
		}
		if err != nil {
			_, _ = syscall.Write(connFd, []byte("-ERR Protocol error: "+err.Error()+"\r\n"))
			delete(s.pending, connFd) // the stream is corrupt - don't try to resume it
			return
		}
		if cmd.Name == "" {
			continue
		}
		reply := command.Handle(cmd)
		if _, err := syscall.Write(connFd, reply); err != nil {
			fmt.Println("write error:", err)
			s.closeConn(connFd)
			return
		}
	}
}

// closeConn closes the connection and forgets any leftover bytes buffered
// for it, so a future fd reused by the OS for a new connection doesn't
// inherit a stale partial command.
func (s *Server) closeConn(connFd int) {
	_ = syscall.Close(connFd)
	delete(s.pending, connFd)
	delete(s.connFds, connFd)
}

// closeAllConns closes every client connection. Deleting from a map while
// ranging over it is allowed in Go.
func (s *Server) closeAllConns() {
	for fd := range s.connFds {
		s.closeConn(fd)
	}
}
