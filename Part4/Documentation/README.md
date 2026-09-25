# 1. Review and refactor RESP and server code:

- **Recall that a command that client sends to server has the following format: it's an array of bulk strings. For example, the command `SET key value` is sent as an array of three bulk strings: `["SET", "key", "value"]`. The server should parse the incoming data according to this format and handle it appropriately.**

- Edge case handling on server: When client sends data, they can send many commands in one go, which can result in one command is not fully read. The server should somehow save the leftover data and read it in the next turn rather than discarding it.

![alt text](image.png)

Solution: whenever we face incomplete command (read resp.go to understand when InCompleteError happens), we should keep the leftover data in a buffer, and we have a map that each connection has its own buffer. When we read data from the connection, we first check if there is leftover data in the buffer, and if so, we prepend it to the new data before parsing.

--> So in this above example, we push the partial command "PING" into map[connection_1] buffer.


