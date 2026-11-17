# Part 4 - Basic Commands

Basic commands (PING, SET with EX, GET, TTL, PTTL, EXPIRE, DEL, EXISTS), and how expired keys are deleted (lazily and actively).

This branch is part 4 of **Redis-k02**, a Redis clone built from scratch in Go, one concept per part.
It is a snapshot of the code as it was at the end of this part.

- Notes for this part: [Documentation/README.md](Documentation/README.md)
- The latest version, its architecture and benchmarks: [main branch](https://github.com/TimSing6584/Multithread-Redis)

## Run

Linux only (it uses epoll), so on Windows run it inside WSL:

```
go run .
redis-cli -p 4000
```
