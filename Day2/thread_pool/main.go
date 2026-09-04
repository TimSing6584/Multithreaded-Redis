// Bound the number of concurrent connection-handling goroutines
// with a semaphore-backed pool, instead of spawning one unboundedly per
// connection like simple tcp_server does.
package main

import (
	"fmt"
	"net"
)
type GoroutinePool struct{
	semaphore chan struct{}
}

func (g *GoroutinePool) Get(){
	g.semaphore <- struct{}{}
}

func (g *GoroutinePool) Return(){
	<- g.semaphore
}

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

	pool := &GoroutinePool{
		semaphore: make(chan struct{}, 1), // pool-size = 1
	}

	// This for loop (actually while True) waits for new connection to accept and process
	for {
		conn, err := listener.Accept()
		if err != nil{
			fmt.Println("Error accepting connection:", err)
			continue
		}
		fmt.Println("Connection accepted from", conn.RemoteAddr())
		pool.Get() // when a new connection is accepted, add it to the pool (if the pool is full -> waiting)
		// Create a goroutine to handle each connection. Prevent one connection blocks others
		go func(c net.Conn){
			defer pool.Return() // return the pool slot once done
			handleConnection(c)
		}(conn)
	}

}