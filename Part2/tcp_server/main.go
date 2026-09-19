// The simplest possible TCP server.

// One goroutine per accepted connection, no pooling, no protocol - just
// prove we can accept a connection, read a line, and write a reply.
package main

import (
	"fmt"
	"io"
	"net"
)

func readCommand(c net.Conn) (string, error) {
	var buf []byte = make([]byte, 512)
	n, err := c.Read(buf) // n is the number of bytes read from the stream
	if err != nil {
		return "", err
	}
	return string(buf[:n]), nil
}
func respond(cmd string, c net.Conn) error {
	if _, err := c.Write([]byte(cmd)); err != nil {
		return err
	}
	return nil
}
func handleConnection(c net.Conn) {
	// Once handling done -> Close
	defer c.Close()
	for {
		cmd, err := readCommand(c)
		if err != nil {
			fmt.Println("client disconnected:", c.RemoteAddr())
			// The server keeps receive request from the client until client calls c.Close(), which will send a signal indicating
			// that the it finishes (EOF) --> we close connection at server side
			if err == io.EOF {
				break
			}
		}
		if err = respond(cmd, c); err != nil {
			fmt.Println("error when write:", err)
			break
		}
	}

}

func main() {
	// open a listening socket on given network and address
	listener, err := net.Listen("tcp", ":3000")
	if err != nil {
		fmt.Println("listen error:", err)
		return
	}
	fmt.Println("Server started on port 3000")
	defer listener.Close()
	// This for loop (actually while True) waits for new connection to accept and process
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}
		fmt.Println("Connection accepted from", conn.RemoteAddr())
		// Create a goroutine to handle each connection. Prevent one connection blocks others
		go handleConnection(conn)
	}

}
