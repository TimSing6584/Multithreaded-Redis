# Benchmark: real Redis vs Redis-k02 single-thread vs Redis-k02 multithread


- **Machine:** 11th Gen Intel(R) Core(TM) i5-1135G7 @ 2.40GHz, 8 CPUs, Linux 6.18.58.1-microsoft-standard-WSL2
- **Real Redis 7.0.15** on port 5000, started as `redis-server --port 5000 --save "" --appendonly no` (no persistence, like Redis-k02).
- `PORT` in the commands is 5000 for real Redis and 4000 for our server.
- Throughput is requests per second; latencies are in ms.
- **Bold** = the better of single-thread vs multithread in each column (higher throughput, lower latency). Real Redis is a reference point, not a competitor, so it is never bold.

## 1. Latency: 1 client

200,000 SET, then 200,000 GET, from **1 client**, on random keys out of 100,000. Only one request is in flight at a time, so requests/sec is just 1 / latency: compare the latency columns.

```
redis-benchmark -p PORT -t set,get -n 200000 -r 100000 -c 1
```

### SET requests

| | Throughput (req/s) | p50 latency (ms) | Mean latency (ms) | p99 latency (ms) |
|---|---:|---:|---:|---:|
| Single-thread | 12,583 | **0.063** | **0.074** | 0.239 |
| Multithread | **15,322** | 0.071 | 0.077 | **0.167** |
| Real Redis (reference only) | 15,962 | 0.055 | 0.059 | 0.159 |

### GET requests

| | Throughput (req/s) | p50 latency (ms) | Mean latency (ms) | p99 latency (ms) |
|---|---:|---:|---:|---:|
| Single-thread | **15,769** | **0.055** | **0.059** | **0.143** |
| Multithread | 10,852 | 0.079 | 0.087 | 0.215 |
| Real Redis (reference only) | 17,139 | 0.055 | 0.065 | 0.175 |

## 2. Normal load: 50 clients

1,000,000 SET, then 1,000,000 GET, from **50 clients** (one benchmark thread, the redis-benchmark default), on random keys out of 100,000.

```
redis-benchmark -p PORT -t set,get -n 1000000 -r 100000 -c 50
```

### SET requests

| | Throughput (req/s) | p50 latency (ms) | Mean latency (ms) | p99 latency (ms) |
|---|---:|---:|---:|---:|
| Single-thread | **143,349** | **0.183** | **0.200** | **0.591** |
| Multithread | 56,057 | 0.495 | 0.544 | 1.383 |
| Real Redis (reference only) | 194,401 | 0.183 | 0.201 | 0.543 |

### GET requests

| | Throughput (req/s) | p50 latency (ms) | Mean latency (ms) | p99 latency (ms) |
|---|---:|---:|---:|---:|
| Single-thread | **125,109** | **0.191** | **0.248** | 1.031 |
| Multithread | 54,277 | 0.479 | 0.491 | **0.823** |
| Real Redis (reference only) | 139,919 | 0.175 | 0.204 | 0.519 |

## 3. High concurrency: 200 clients

2,000,000 SET, then 2,000,000 GET, from **200 clients** spread over 4 benchmark threads, on random keys out of 100,000.

```
redis-benchmark -p PORT -t set,get -n 2000000 -r 100000 -c 200 --threads 4
```

### SET requests

| | Throughput (req/s) | p50 latency (ms) | Mean latency (ms) | p99 latency (ms) |
|---|---:|---:|---:|---:|
| Single-thread | 55,486 | 3.295 | 3.792 | **12.135** |
| Multithread | **68,220** | **1.199** | **2.768** | 22.031 |
| Real Redis (reference only) | 119,962 | 1.831 | 1.966 | 4.687 |

### GET requests

| | Throughput (req/s) | p50 latency (ms) | Mean latency (ms) | p99 latency (ms) |
|---|---:|---:|---:|---:|
| Single-thread | 42,175 | 3.439 | 4.990 | 30.015 |
| Multithread | **82,450** | **1.231** | **2.403** | **17.839** |
| Real Redis (reference only) | 61,082 | 2.679 | 3.387 | 15.911 |

## 4. Heavy command: SMEMBERS on ~400-member sets

Prefill: 500,000 SADD from 50 clients into 1,000 sets, with members picked at random out of 1,000, so each set ends up with ~400 members. Then 200,000 SMEMBERS from **200 clients** over 4 benchmark threads, on random sets. Each reply is ~8 KB, so every request costs the server real CPU.

```
redis-benchmark -p PORT -n 500000 -r 1000 -c 50 SADD set:__rand_int__ m:__rand_int__   # prefill
redis-benchmark -p PORT -n 200000 -r 1000 -c 200 --threads 4 SMEMBERS set:__rand_int__
```

### SMEMBERS requests

| | Throughput (req/s) | p50 latency (ms) | Mean latency (ms) | p99 latency (ms) |
|---|---:|---:|---:|---:|
| Single-thread | 3,640 | 55.967 | 59.513 | 126.847 |
| Multithread | **5,367** | **36.127** | **40.455** | **99.519** |
| Real Redis (reference only) | 4,261 | 44.767 | 48.260 | 116.223 |

