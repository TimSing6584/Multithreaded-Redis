# Benchmark: cache eviction policies


- **Keyspace:** 1,000,000 keys. Each run: 2,000,000 warm-up requests (not counted), then 5,000,000 measured requests.
- **Method:** the program drives `core.Store` directly, with the same `Get` / `Set` calls the GET and SET commands make, so the network is left out. Every request is cache-aside: GET the key, and on a miss SET it. The trace is generated once (fixed seed) and replayed against every policy, so they all see the same requests.
- Hit ratio = hits / measured requests. pp = percentage points.

## 1. Zipfian workload (alpha = 0.99)

A few keys get most of the requests, like real cache traffic: with alpha = 0.99, the hottest 1% of keys get 66.4% of all requests.

| Cache size | Random | LRU | Approx LRU | Ideal: hottest keys always cached |
|---|---:|---:|---:|---:|
| 1% (10,000 keys) | 51.00% | 56.65% | 56.45% | 66.43% |
| 5% (50,000 keys) | 65.29% | 70.38% | 70.18% | 77.98% |
| 10% (100,000 keys) | 71.97% | 76.66% | 76.46% | 83.02% |
| 20% (200,000 keys) | 79.23% | 83.27% | 83.07% | 88.09% |

**Ideal** is a reference, not a policy: the hit ratio of a cache that always holds the C most popular keys (C = cache size). Key k gets weight 1/k^alpha, so Ideal = (sum of the top C weights) / (sum of all weights). Every request for those keys is a hit, and every other request is a miss.

## 2. Memory per key

Heap growth after inserting 1,000,000 keys into an empty store (no eviction), divided by the number of keys. Key strings are allocated beforehand, so they are not counted: this is the store's map entries and Objects plus each policy's bookkeeping.

- Every Object carries an 8-byte `lastAccess` stamp whatever the policy, so random and approx LRU cost the same. Approx LRU's only other bookkeeping is a pool of 16 entries, whatever the number of keys.
- LRU adds a doubly linked list node and an index map entry for every key.

| Policy | Heap per key (bytes) | vs Approx LRU |
|---|---:|---:|
| Random | 87.8 | same |
| LRU | 175.5 | +87.6 bytes (2.00x) |
| Approx LRU | 87.9 | same |
