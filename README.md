# Part 2 - TCP Server

A TCP server with a thread pool, file descriptors ("everything is a file"), and the I/O models.

This branch is part 2 of **Redis-k02**, a Redis clone built from scratch in Go, one concept per part.
It is a snapshot of the code as it was at the end of this part.

- Notes for this part: [Documentation/README.md](Documentation/README.md)
- The latest version, its architecture and benchmarks: [main branch](https://github.com/TimSing6584/Multithread-Redis)

## Run

```
go run ./tcp_server     # listens on :3000
go run ./tcp_client
go run ./thread_pool
```
