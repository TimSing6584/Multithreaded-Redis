// Entry point: asks which engine to run, then starts it.
//
//  1. Single-thread (internal/server/single_thread): one event loop. The OS multiplexer
//     (epoll on Linux) tells it which connections are ready, and it executes every
//     command itself against the one keyspace.
//  2. Multi-thread (internal/server/multithread): shared-nothing. One listener hands
//     connections to N IO handlers (one epoll loop each), which route every command to
//     one of M workers by hash(key) % M. Each worker owns its own partition of the
//     keyspace, so no locks are needed.
//
// Graceful shutdown is the same for both:
//   - The main goroutine runs srv.Run(). The engine owns every client connection and
//     closes them all before Run returns.
//   - Goroutine 2 (waitForShutdown) blocks on SIGINT/SIGTERM, then calls srv.Close(),
//     which tells the engine to stop.
package main

import (
	"Redis-k02/Part9/internal/config"
	"Redis-k02/Part9/internal/server/multithread"
	"Redis-k02/Part9/internal/server/single_thread"
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

// server is what main needs from either engine.
type server interface {
	Run() error
	Close() error
}

func main() {
	srv, err := chooseServer()
	if err != nil {
		fmt.Println(err)
		return
	}

	go waitForShutdown(srv)

	if err := srv.Run(); err != nil {
		fmt.Println(err)
	}
	fmt.Println("Server stopped")
}

// chooseServer asks on stdin which engine to run until it gets a valid answer, then builds it.
func chooseServer() (server, error) {
	fmt.Println("Choose the server version:")
	fmt.Println("  1) Single-thread")
	fmt.Println("  2) Multi-thread")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			return nil, errors.New("no server version chosen (stdin closed)")
		}
		switch strings.TrimSpace(scanner.Text()) {
		case "1":
			// Return nil explicitly on error: a nil *single_thread.Server wrapped in
			// the server interface would NOT compare equal to nil.
			srv, err := single_thread.New(config.Port)
			if err != nil {
				return nil, err
			}
			fmt.Println("Single-thread server started on port", config.Port)
			return srv, nil
		// case "2":
		// 	srv, err := multithread.New(config.Port, config.NumIOHandlers, config.NumWorkers)
		// 	if err != nil {
		// 		return nil, err
		// 	}
		// 	fmt.Printf("Multi-thread server started on port %s (io handlers=%d, workers=%d)\n",
		// 		config.Port, config.NumIOHandlers, config.NumWorkers)
		// 	return srv, nil
		// default:
		// 	fmt.Println("Please type 1 or 2.")
		// }
	}
}

// waitForShutdown blocks until Ctrl+C (SIGINT) or SIGTERM arrives, then
// tells the server to stop. SIGKILL/SIGSTOP can't be caught, so they never get here.
func waitForShutdown(srv server) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigs
	fmt.Println("received", sig, "- shutting down")
	_ = srv.Close()
}
