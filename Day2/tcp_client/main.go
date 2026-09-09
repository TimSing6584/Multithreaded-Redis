package main
import (
    "net"
    "os"
)

func main() {
    messages := [...]string{"1st message", "2nd message"}
    servAddr := "localhost:3000"
    tcpAddr, err := net.ResolveTCPAddr("tcp", servAddr)
    if err != nil {
        println("ResolveTCPAddr failed:", err.Error())
        os.Exit(1)
    }

    conn, err := net.DialTCP("tcp", nil, tcpAddr)
    if err != nil {
        println("Dial failed:", err.Error())
        os.Exit(1)
    }
    for i := 0; i < len(messages); i++{
        _, err = conn.Write([]byte(messages[i]))
        if err != nil {
            println("Write to server failed:", err.Error())
            os.Exit(1)
        }

        println("write to server = ", messages[i])

        reply := make([]byte, 1024)

        _, err = conn.Read(reply)
        if err != nil {
            println("Write to server failed:", err.Error())
            os.Exit(1)
        }

        println("reply from server=", string(reply))
    }

    conn.Close()
}