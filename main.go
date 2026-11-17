// One thread, no connection pool - instead the OS multiplexer
// (epoll on Linux, kqueue on macOS) tells us which of the many open file
// descriptors are actually ready to read, and we serve every connection
// from a single event loop (see internal/server).
//
// Graceful shutdown uses two goroutines:
//   - Goroutine 1 (this one) runs srv.Run(), the event loop. It owns every
//     client connection and closes them all before Run returns.
//   - Goroutine 2 (waitForShutdown) blocks on SIGINT/SIGTERM, then calls
//     srv.Close(), which closes the multiplexer and the listener so the loop
//     stops.
package main

import (
	"Redis-k02/internal/config"
	"Redis-k02/internal/server"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	srv, err := server.New(config.Port)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("Server started on port", config.Port)

	go waitForShutdown(srv)

	if err := srv.Run(); err != nil {
		fmt.Println(err)
	}
	fmt.Println("Server stopped")
}

// waitForShutdown blocks until Ctrl+C (SIGINT) or SIGTERM arrives, then
// tells the server to stop. SIGKILL/SIGSTOP can't be caught, so they never get here.
func waitForShutdown(srv *server.Server) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigs
	fmt.Println("received", sig, "- shutting down")
	_ = srv.Close()
}
