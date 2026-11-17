package multithread

import (
	"Redis-k02/internal/config"
	"Redis-k02/internal/resp"
	"Redis-k02/internal/server"
	"Redis-k02/io_multiplexing"
	"fmt"
	"strings"
	"syscall"
	"time"
)

// ioHandler is one epoll event loop serving the connections the listener assigned to it.
// It reads and parses requests exactly like the single-thread server does (server.Conns),
// but instead of executing a command itself it routes it to the worker owning the key.
//
// Every field is touched only by this handler's own run goroutine. New connections
// arrive over the newConns channel instead of the listener touching conns directly,
// which is what lets conns skip a mutex.
type ioHandler struct {
	id int
	mp io_multiplexing.IOMultiplexer

	newConns chan int
	conns    *server.Conns

	workers []*worker
	// replies is where workers send back the replies for this handler's tasks. Its
	// capacity is len(workers), so even a DEL fanned out to every worker never makes a
	// worker block on sending its reply. Reused for every command: dispatch always
	// receives every reply it asked for before returning, so nothing stale is left in it.
	replies chan []byte
	// nextWorker round-robins keyless commands (e.g. PING) across workers.
	nextWorker int
}

func newIOHandler(id int, workers []*worker) (*ioHandler, error) {
	mp, err := io_multiplexing.CreateIOMultiplexer()
	if err != nil {
		return nil, err
	}
	return &ioHandler{
		id:       id,
		mp:       mp,
		newConns: make(chan int, config.NewConnQueueSize),
		conns:    server.NewConns(),
		workers:  workers,
		replies:  make(chan []byte, len(workers)),
	}, nil
}

// run drives the event loop until done is closed. The server closes done only after
// its accept loop has stopped, so nothing can be sent on newConns any more by then.
func (h *ioHandler) run(done <-chan struct{}) {
	// The handler owns its multiplexer, so it closes it itself - after its last Wait.
	defer h.mp.Close()
	timeoutMs := int(config.IOHandlerPollInterval / time.Millisecond)

	for {
		select {
		case <-done:
			h.registerNewConns() // so connections still queued get closed too
			fmt.Printf("io handler %d: closing %d client connection(s)\n", h.id, h.conns.Len())
			h.conns.CloseAll()
			return
		default:
		}

		h.registerNewConns()

		events, err := h.mp.Wait(timeoutMs)
		if err != nil {
			fmt.Println("io handler", h.id, "wait error:", err)
			continue
		}
		for _, event := range events {
			h.conns.HandleReadable(event.Fd, h.dispatch)
		}
	}
}

// registerNewConns adds every connection the listener handed over since the last call
// to this handler's monitoring list, without blocking.
func (h *ioHandler) registerNewConns() {
	for {
		select {
		case connFd := <-h.newConns:
			if err := h.mp.Monitor(io_multiplexing.Event{Fd: connFd, Op: io_multiplexing.OpRead}); err != nil {
				fmt.Println("io handler", h.id, "failed to monitor conn fd:", err)
				_ = syscall.Close(connFd)
				continue
			}
			h.conns.Add(connFd)
		default:
			return
		}
	}
}

// partitionOf maps key to the worker that owns it: hash(key) % numWorkers.
// The hash is 32-bit FNV-1a: deterministic (the same key always lands on the same
// worker, so SET then GET meet in the same store) and spreads keys evenly.
func partitionOf(key string, numWorkers int) int {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	hash := uint32(offset32)
	for i := 0; i < len(key); i++ {
		hash ^= uint32(key[i])
		hash *= prime32
	}
	return int(hash % uint32(numWorkers))
}

// dispatch sends cmd to the worker(s) that must execute it and returns the reply.

// It blocks until the reply arrives. That keeps the replies of pipelined commands in
// order, and other IO handlers and workers keep making progress in the meantime.
func (h *ioHandler) dispatch(cmd resp.Command) []byte {
	// Keyless commands (PING), and wrong-arity calls like "GET" with no key, touch no
	// data, so any worker can answer them.
	if len(cmd.Args) == 0 {
		w := h.workers[h.nextWorker]
		h.nextWorker = (h.nextWorker + 1) % len(h.workers)
		return h.call(w, cmd)
	}

	switch strings.ToUpper(cmd.Name) {
	case "DEL", "EXISTS":
		// Multi-key commands: the keys can live in several partitions.
		return h.fanOutCount(cmd)
	}

	// Every other command works on a single key, always its first argument
	// (multi-ITEM commands like BF.MADD or CMS.INCRBY still have one key).
	return h.call(h.workers[partitionOf(cmd.Args[0], len(h.workers))], cmd)
}

// call runs cmd on worker w and waits for its reply.
func (h *ioHandler) call(w *worker, cmd resp.Command) []byte {
	w.tasks <- task{cmd: cmd, reply: h.replies}
	return <-h.replies
}

// fanOutCount runs a multi-key command whose reply is a count (DEL, EXISTS) across
// partitions: the keys are grouped by owning worker, every worker gets one sub-command
// with its own keys (all sent before waiting, so the workers run them in parallel),
// and the integer replies are summed. A repeated key stays in its group, so
// EXISTS k k still counts it twice, like real Redis.

// Unlike real Redis this is not atomic: another client can observe the keys of one
// partition already deleted while another partition's DEL hasn't run yet.
func (h *ioHandler) fanOutCount(cmd resp.Command) []byte {
	groups := make([][]string, len(h.workers))
	for _, key := range cmd.Args {
		id := partitionOf(key, len(h.workers))
		groups[id] = append(groups[id], key)
	}

	sent := 0
	for id, keys := range groups {
		if len(keys) == 0 {
			continue
		}
		h.workers[id].tasks <- task{cmd: resp.Command{Name: cmd.Name, Args: keys}, reply: h.replies}
		sent++
	}

	// Receive every reply, even after an unexpected one, so none is left behind in
	// h.replies to be mistaken for the reply of the next command.
	total := 0
	var errReply []byte
	for i := 0; i < sent; i++ {
		reply := <-h.replies
		n, err := resp.DecodeInteger(reply)
		if err != nil {
			errReply = reply
			continue
		}
		total += n
	}
	if errReply != nil {
		return errReply
	}
	return resp.EncodeReply(resp.Integer(total))
}
