// Package single_thread is the single-threaded engine: one I/O-multiplexed event loop
// owns the TCP listener, every open client connection, and the one and only keyspace.
//
// It runs on two goroutines that never share connection state:
//   - Run (goroutine 1) is the event loop. It is the only one that ever
//     touches conns/store, so those need no mutex.
//   - Close (goroutine 2, e.g. a signal handler) only flips the closed flag
//     and closes the multiplexer + listener. Run notices the flag and does
//     the actual cleanup of client connections itself.
package single_thread

import (
	"Redis-k02/Part9/internal/command"
	"Redis-k02/Part9/internal/config"
	"Redis-k02/Part9/internal/core"
	"Redis-k02/Part9/internal/resp"
	"Redis-k02/Part9/internal/server"
	"Redis-k02/Part9/io_multiplexing"
	"fmt"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type Server struct {
	listener *server.Listener
	mp       io_multiplexing.IOMultiplexer

	// conns holds every open client connection and its not-yet-parsed bytes.
	conns *server.Conns
	// store is the whole keyspace: with a single thread there is nothing to partition.
	store *core.Store

	closed atomic.Bool
	once   sync.Once
}

// New opens the listener on addr and registers it with a fresh multiplexer.
func New(addr string) (*Server, error) {
	listener, err := server.Listen(addr)
	if err != nil {
		return nil, err
	}

	// Create the I/O multiplexer (epoll on Linux, kqueue on macOS)
	mp, err := io_multiplexing.CreateIOMultiplexer()
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("create multiplexer: %w", err)
	}
	// Let the multiplexer monitor the listener's file descriptor for read events (new connections)
	if err := mp.Monitor(io_multiplexing.Event{Fd: listener.Fd, Op: io_multiplexing.OpRead}); err != nil {
		_ = mp.Close()
		_ = listener.Close()
		return nil, fmt.Errorf("monitor server fd: %w", err)
	}

	return &Server{
		listener: listener,
		mp:       mp,
		conns:    server.NewConns(),
		store:    core.NewStore(config.MaxKeys),
	}, nil
}

// Close asks a running Run loop to stop. Meant to be called from another
// goroutine (the signal handler). It never touches conns - Run's
// own goroutine cleans those up once it sees closed == true.

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
			if event.Fd == s.listener.Fd {
				s.acceptConn()
				continue
			}
			// Otherwise, for a read-ready event on a client connection, handle the readable data
			s.conns.HandleReadable(event.Fd, s.execute)
		}
		// We only run the active expired keys deletion when the Epoll waits more than config.ActiveExpireInterval, since it starts Wait()
		// For example, at t=0, Wait() is called, after 15ms, Epoll signals the server there's data from one connection, and that data is processed in 10ms
		// so total 25ms from the start, haven't reached 100ms yet, so we don't run the active expired keys deletion.

		// We only run the active expired keys deletion when the Epoll waits more than config.ActiveExpireInterval, since it starts Wait(),
		// or we  can say "run the active delete mode when the server is not BUSY"

		if time.Since(lastActiveExpiredDeleteTime) >= config.ActiveExpireInterval {
			s.store.ActiveExpireDelete(config.ActiveExpireBudget)
			lastActiveExpiredDeleteTime = time.Now()
		}
	}
}

// execute runs one parsed command against the server's only keyspace.
func (s *Server) execute(cmd resp.Command) []byte {
	return command.Handle(s.store, cmd)
}

// shutdown runs on Run's goroutine after Close was observed. The loop is
// single-threaded, so any command that was being handled has already
// finished by the time we get here; half-received commands in pending are
// dropped together with their connection.
func (s *Server) shutdown() {
	fmt.Println("shutting down: closing", s.conns.Len(), "client connection(s)")
	s.conns.CloseAll()
}

func (s *Server) acceptConn() {
	connFd, err := s.listener.Accept()
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
	s.conns.Add(connFd)
}
