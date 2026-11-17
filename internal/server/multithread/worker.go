package multithread

import (
	"Redis-k02/internal/command"
	"Redis-k02/internal/config"
	"Redis-k02/internal/core"
	"Redis-k02/internal/resp"
	"time"
)

// task is one parsed command an IO handler routed to a worker, plus the channel
// the worker sends the encoded reply back on (the IO handler's own replies channel).
type task struct {
	cmd   resp.Command
	reply chan<- []byte
}

// worker owns one partition of the keyspace: its own *core.Store, holding every key k
// with partitionOf(k) == id. Its run goroutine is the only one that ever touches that
// store, so executing commands needs no lock - the core benefit of shared-nothing.
type worker struct {
	id    int
	store *core.Store
	tasks chan task
}

func newWorker(id, maxKeys int) *worker {
	return &worker{
		id:    id,
		store: core.NewStore(maxKeys),
		tasks: make(chan task, config.WorkerQueueSize),
	}
}

// run executes the tasks it receives one by one, in arrival order, until tasks is closed.
//
// Active expiry happens HERE, per partition: a worker has no multiplexer.Wait() whose
// timeout it could piggyback on (unlike the single-thread server), so a ticker wakes it
// up every config.ActiveExpireInterval to sweep its own store.
func (w *worker) run() {
	ticker := time.NewTicker(config.ActiveExpireInterval)
	defer ticker.Stop()
	for {
		select {
		case t, ok := <-w.tasks:
			if !ok {
				return
			}
			t.reply <- command.Handle(w.store, t.cmd)
		case <-ticker.C:
			w.store.ActiveExpireDelete(config.ActiveExpireBudget)
		}
	}
}
