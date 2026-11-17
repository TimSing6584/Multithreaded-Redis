// Package multithread is the shared-nothing engine (see Documentation/README.md),
// 1 listener --round robin--> N IO handlers --hash(key) % M--> M workers:
//
//   - The listener (Run's own goroutine) accepts connections and hands each one to an
//     IO handler, round robin.
//   - Each IO handler runs its own epoll loop over its connections: it reads and parses
//     requests and routes every command to the worker that owns the key.
//   - Each worker owns one partition of the keyspace (its own *core.Store) and executes
//     the commands it receives sequentially. No two goroutines ever touch the same
//     store, so there are no locks anywhere on the data path.
package multithread

import (
	"Redis-k02/Part9/internal/config"
	"Redis-k02/Part9/internal/server"
	"fmt"
	"sync"
	"sync/atomic"
	"syscall"
)

type Server struct {
	listener   *server.Listener
	ioHandlers []*ioHandler
	workers    []*worker

	// nextHandler round-robins accepted connections across ioHandlers. Only the accept
	// loop (Run's goroutine) touches it, so it needs no atomic.
	nextHandler int

	closed atomic.Bool
	once   sync.Once
}

// New opens the listener on addr and builds numIOHandlers IO handlers and numWorkers
// workers. Nothing runs until Run is called.
func New(addr string, numIOHandlers, numWorkers int) (*Server, error) {
	if numIOHandlers < 1 || numWorkers < 1 {
		return nil, fmt.Errorf("need at least 1 io handler and 1 worker, got %d and %d", numIOHandlers, numWorkers)
	}

	listener, err := server.Listen(addr)
	if err != nil {
		return nil, err
	}

	// Split the eviction limit across the partitions, so the whole server still holds
	// about config.MaxKeys keys, like the single-thread one. Keys are spread evenly by
	// the hash, so each partition fills up at roughly the same rate.
	maxKeysPerWorker := (config.MaxKeys + numWorkers - 1) / numWorkers
	workers := make([]*worker, numWorkers)
	for i := range workers {
		workers[i] = newWorker(i, maxKeysPerWorker)
	}

	ioHandlers := make([]*ioHandler, numIOHandlers)
	for i := range ioHandlers {
		h, err := newIOHandler(i, workers)
		if err != nil {
			for _, created := range ioHandlers[:i] {
				_ = created.mp.Close()
			}
			_ = listener.Close()
			return nil, fmt.Errorf("create io handler %d: %w", i, err)
		}
		ioHandlers[i] = h
	}

	return &Server{
		listener:   listener,
		ioHandlers: ioHandlers,
		workers:    workers,
	}, nil
}

// Run starts every worker and IO handler, then runs the accept loop on the calling
// goroutine until Close is called. It returns once everything has stopped.
func (s *Server) Run() error {
	var workersWG, ioHandlersWG sync.WaitGroup
	for _, w := range s.workers {
		workersWG.Go(w.run)
	}
	done := make(chan struct{})
	for _, h := range s.ioHandlers {
		ioHandlersWG.Go(func() { h.run(done) })
	}

	s.acceptLoop() // returns once Close has shut the listener down

	// Stop everything in dependency order:
	//  1. IO handlers: the accept loop is gone, so no new connection can reach them.
	//     Each one closes its connections and its multiplexer, then returns.
	//  2. Workers: only now no IO handler can send them a task any more, so closing
	//     their channels can't make a handler panic with "send on closed channel".
	fmt.Println("shutting down")
	close(done)
	ioHandlersWG.Wait()
	for _, w := range s.workers {
		close(w.tasks)
	}
	workersWG.Wait()
	_ = s.listener.Close()
	return nil
}

// acceptLoop blocks in Accept and hands every new connection to the next IO handler.
func (s *Server) acceptLoop() {
	for {
		connFd, err := s.listener.Accept()
		if err != nil {
			if s.closed.Load() {
				return
			}
			if err != syscall.EINTR && err != syscall.ECONNABORTED {
				fmt.Println("accept error:", err)
			}
			continue
		}
		s.ioHandlers[s.nextHandler].newConns <- connFd
		s.nextHandler = (s.nextHandler + 1) % len(s.ioHandlers)
	}
}

// Close asks a running Run to stop. Meant to be called from another goroutine (the
// signal handler). It only wakes up the blocked Accept; Run then stops the IO handlers
// and workers itself and closes the listener's fds, since Run is their owner.
//
// Safe to call more than once.
func (s *Server) Close() error {
	s.once.Do(func() {
		s.closed.Store(true)
		s.listener.Shutdown()
	})
	return nil
}
