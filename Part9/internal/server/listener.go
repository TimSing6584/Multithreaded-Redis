package server

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

// Listener is the server's TCP listening socket, exposed as a raw fd so it can be
// monitored by a multiplexer (single-thread) or accepted on directly (multithread).
type Listener struct {
	// ln is the original Go listener. file is a dup of its fd (that's what
	// TCPListener.File() returns), so BOTH must be closed - closing only the
	// dup leaves the socket open and still queuing new connections.
	ln   net.Listener
	file *os.File
	// Fd is the raw fd of the dup. file.Fd() puts it in blocking mode, so a
	// syscall.Accept on it waits until a client connects.
	Fd int
}

// Listen opens a TCP listener on addr.
func Listen(addr string) (*Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	// Retrieve the underlying file descriptor for the listener
	file, err := ln.(*net.TCPListener).File()
	if err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("get listener file: %w", err)
	}
	return &Listener{ln: ln, file: file, Fd: int(file.Fd())}, nil
}

// Accept accepts one pending connection and returns its fd.
func (l *Listener) Accept() (int, error) {
	connFd, _, err := syscall.Accept(l.Fd)
	return connFd, err
}

// Shutdown makes the socket stop accepting connections, which also wakes up a
// syscall.Accept that is blocked on it in another goroutine (it returns an error).
// Merely closing the fd is not enough for that: a blocked accept keeps its own
// reference to the socket and would sleep on.
func (l *Listener) Shutdown() {
	_ = syscall.Shutdown(l.Fd, syscall.SHUT_RDWR)
}

// Close closes both the dup fd and the original listener.
func (l *Listener) Close() error {
	_ = l.file.Close()
	return l.ln.Close()
}
