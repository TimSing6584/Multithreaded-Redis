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

	"Redis-k02/Part3/internal/command"
	"Redis-k02/Part3/internal/resp"
	"Redis-k02/Part3/io_multiplexing"
)

// handleReadable is called once per read-ready event on connFd. It does a
// single, non-blocking-in-spirit read of whatever is currently buffered by
// the kernel, then executes every RESP command found in that chunk. A
// command that straddles two reads is not reassembled - good enough for a
// lecture on event loops, not a fully spec-compliant server.
func handleReadable(connFd int) {
	buf := make([]byte, 4096)
	n, err := syscall.Read(connFd, buf)
	if err != nil || n == 0 {
		if err != nil && err != io.EOF {
			fmt.Println("read error:", err)
		}
		_ = syscall.Close(connFd)
		return
	}

	reader := bufio.NewReader(bytes.NewReader(buf[:n]))
	for {
		args, err := resp.ReadCommand(reader)
		if err != nil {
			// EOF / unexpected EOF just mean the buffer ran out (possibly
			// mid-command); anything else is a malformed request.
			if err != io.EOF && err != io.ErrUnexpectedEOF {
				_, _ = syscall.Write(connFd, []byte("-ERR Protocol error: "+err.Error()+"\r\n"))
			}
			return
		}
		if len(args) == 0 {
			continue
		}
		reply := command.Handle(args)
		if _, err := syscall.Write(connFd, reply); err != nil {
			fmt.Println("write error:", err)
			_ = syscall.Close(connFd)
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

	for {
		events, err := multiplexer.Wait() // blocking call, returns when at least one fd is ready
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
					_ = syscall.Close(connFd)
				}
				continue
			}
			// Otherwise, for a read-ready event on a client connection, handle the readable data
			handleReadable(event.Fd)
		}
	}
}
