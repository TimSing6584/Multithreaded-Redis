// The simplest possible TCP server.

// One goroutine per accepted connection, no pooling, no protocol - just
// prove we can accept a connection, read a line, and write a reply.
package main

import (
	"fmt"
	"net"
)

func handleConnection(c net.Conn){
	// Once handling done -> Close
	defer c.Close()
	buffer := make([]byte, 1024)
	_, err := c.Read(buffer) // blocks until the client sends data
	if err != nil {
		fmt.Println("Error reading from connection:", err)
		return
	}

	fmt.Println(string(buffer))
	c.Write([]byte("Hello from low-level TCP server!\n"))

}

func main(){
	// open a listening socket on given network and address
	listener, err := net.Listen("tcp", ":3000")
	if err != nil{
		fmt.Println("listen error:", err)
		return
	}
	fmt.Println("Server started on port 3000")
	defer listener.Close()
	// This for loop (actually while True) waits for new connection to accept and process
	for {
		conn, err := listener.Accept()
		if err != nil{
			fmt.Println("Error accepting connection:", err)
			continue
		}
		fmt.Println("Connection accepted from", conn.RemoteAddr())
		// Create a goroutine to handle each connection. Prevent one connection blocks others
		go handleConnection(conn)
	}

}