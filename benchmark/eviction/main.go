// Benchmarks the eviction policies (random, LRU, approx LRU): hit ratio on a Zipfian workload,
// and memory per key. Writes benchmark/eviction_result.md. Run from the repo root (any OS, ~4 min):

//	go run ./benchmark/eviction

// It calls core.Store directly (no network). Each request is cache-aside: GET, and SET on a miss.
// One trace is replayed against every policy; the first warmupOps requests are not counted.
package main

import (
	"Redis-k02/internal/config"
	"Redis-k02/internal/core"
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	numKeys     = 1_000_000 // keyspace size
	warmupOps   = 2_000_000 // fill the cache, not counted
	measuredOps = 5_000_000 // hit ratio is computed on these
	zipfAlpha   = 0.99      // Zipfian skew (YCSB default)
	runs        = 3         // runs per cell, averaged
	seed        = 42
)

// Cache sizes, as a % of the keyspace.
var cachePercents = []int{1, 5, 10, 20}

type policy struct {
	name string
	id   config.EvictionPolicy
}

var policies = []policy{
	{"Random", config.PolicyRandom},
	{"LRU", config.PolicyLRU},
	{"Approx LRU", config.PolicyApproxLRU},
}

// Hit ratio of one (cache size, policy) cell.
type result struct {
	mean   float64 // in [0, 1]
	spread float64 // max - min across runs
}

func main() {
	out := flag.String("out", "benchmark/eviction_result.md", "where to write the report")
	flag.Parse()
	start := time.Now()

	// Built once, so string formatting isn't measured.
	keys := make([]string, numKeys)
	for i := range keys {
		keys[i] = "key:" + strconv.Itoa(i)
	}

	rng := rand.New(rand.NewPCG(seed, seed))
	cdf := zipfCDF(numKeys, zipfAlpha)
	zipf := zipfTrace(cdf, warmupOps+measuredOps, rng)

	zipfResults := measureWorkload("Zipfian", keys, zipf)

	fmt.Fprintln(os.Stderr, ">> Memory per key")
	memory := make([]float64, len(policies))
	for i, p := range policies {
		memory[i] = bytesPerKey(p.id, keys)
	}

	report := buildReport(cdf, zipfResults, memory, time.Since(start))
	if err := os.WriteFile(*out, []byte(report), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err, "(run it from the repo root, or pass -out)")
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, ">> Wrote", *out)
}

// zipfCDF: key i has weight 1/(i+1)^alpha (key 0 is the hottest); cdf[i] is the running sum
// of weights, normalized to 1. (math/rand's Zipf needs alpha > 1, so we build our own.)
func zipfCDF(n int, alpha float64) []float64 {
	cdf := make([]float64, n)
	sum := 0.0
	for i := range cdf {
		sum += 1 / math.Pow(float64(i+1), alpha)
		cdf[i] = sum
	}
	for i := range cdf {
		cdf[i] /= sum
	}
	return cdf
}

// zipfTrace: per request, draw u in [0, 1) and pick the first key with cdf >= u (binary search).
func zipfTrace(cdf []float64, ops int, rng *rand.Rand) []int32 {
	trace := make([]int32, ops)
	for i := range trace {
		trace[i] = int32(sort.SearchFloat64s(cdf, rng.Float64()))
	}
	return trace
}

// measureWorkload returns results[cache size][policy].
func measureWorkload(name string, keys []string, trace []int32) [][]result {
	results := make([][]result, len(cachePercents))
	for c, pct := range cachePercents {
		capacity := numKeys * pct / 100
		results[c] = make([]result, len(policies))
		for p, pol := range policies {
			fmt.Fprintf(os.Stderr, ">> %s, cache %d%% (%s keys): %s\n", name, pct, commas(capacity), pol.name)
			lo, hi, sum := 1.0, 0.0, 0.0
			for range runs {
				r := hitRatio(pol.id, capacity, keys, trace)
				lo, hi, sum = min(lo, r), max(hi, r), sum+r
			}
			results[c][p] = result{mean: sum / runs, spread: hi - lo}
		}
	}
	return results
}

// hitRatio replays trace on a fresh store and returns hits / measured requests.
// Setting config.Policy here is safe: it's set before each new store, and one store runs at a time.
func hitRatio(p config.EvictionPolicy, capacity int, keys []string, trace []int32) float64 {
	config.Policy = p
	store := core.NewStore(capacity)
	hits := 0
	for i, id := range trace {
		key := keys[id]
		if store.Get(key) != nil {
			if i >= warmupOps {
				hits++
			}
			continue
		}
		store.Set(key, &core.Object{Kind: core.KindString, Value: "v"})
	}
	return float64(hits) / float64(len(trace)-warmupOps)
}

// bytesPerKey: heap growth per key after inserting every key (no eviction). Key strings exist
// before the first reading, so only the store and the policy's bookkeeping are counted.
func bytesPerKey(p config.EvictionPolicy, keys []string) float64 {
	config.Policy = p
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	store := core.NewStore(len(keys) + 1)
	for _, key := range keys {
		store.Set(key, &core.Object{Kind: core.KindString, Value: "v"})
	}

	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(store) // so the GC above can't free the store before the reading
	return float64(int64(after.HeapAlloc)-int64(before.HeapAlloc)) / float64(len(keys))
}

// buildReport renders the markdown report.
func buildReport(cdf []float64, zipf [][]result, memory []float64, took time.Duration) string {
	var b strings.Builder
	maxSpread := 0.0
	for _, row := range zipf {
		for _, r := range row {
			maxSpread = max(maxSpread, r.spread)
		}
	}

	fmt.Fprintf(&b, `# Benchmark: cache eviction policies

Generated by `+"`go run ./benchmark/eviction`"+` on %s (took %.0f s).

- **Machine:** %s/%s, %d CPUs, %s.
- **Keyspace:** %s keys. Each run: %s warm-up requests (not counted), then %s measured requests.
- **Approx LRU:** samples %d keys per eviction into a pool of %d candidates (`+"`config.EvictionSampleSize` / `config.EvictionPoolSize`"+`, same as Redis).
- **Method:** the program drives `+"`core.Store`"+` directly, with the same `+"`Get` / `Set`"+` calls the GET and SET commands make, so the network is left out. Every request is cache-aside: GET the key, and on a miss SET it. The trace is generated once (fixed seed) and replayed against every policy, so they all see the same requests.
- Random and approx LRU pick keys through Go's random map iteration order, which can't be seeded, so every cell ran **%d times** and shows the mean. The largest spread between runs was **%.3f pp**.
- Hit ratio = hits / measured requests. pp = percentage points.

`, time.Now().Format("2006-01-02 15:04"), took.Seconds(),
		runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.Version(),
		commas(numKeys), commas(warmupOps), commas(measuredOps),
		config.EvictionSampleSize, config.EvictionPoolSize, runs, maxSpread*100)

	fmt.Fprintf(&b, `## 1. Zipfian workload (alpha = %.2f)

A few keys get most of the requests, like real cache traffic: with alpha = %.2f, the hottest 1%% of keys get %.1f%% of all requests.

| Cache size | Random | LRU | Approx LRU | Ideal: hottest keys always cached |
|---|---:|---:|---:|---:|
`, zipfAlpha, zipfAlpha, cdf[numKeys/100-1]*100)
	for c, pct := range cachePercents {
		capacity := numKeys * pct / 100
		row := zipf[c]
		fmt.Fprintf(&b, "| %d%% (%s keys) | %s | %s | %s | %s |\n", pct, commas(capacity),
			percent(row[0].mean), percent(row[1].mean), percent(row[2].mean), percent(cdf[capacity-1]))
	}

	b.WriteString(`
**Ideal** is a reference, not a policy: the hit ratio of a cache that always holds the C most popular keys (C = cache size). Key k gets weight 1/k^alpha, so Ideal = (sum of the top C weights) / (sum of all weights). Every request for those keys is a hit, and every other request is a miss.
`)

	approx := memory[2]
	fmt.Fprintf(&b, `
## 2. Memory per key

Heap growth after inserting %s keys into an empty store (no eviction), divided by the number of keys. Key strings are allocated beforehand, so they are not counted: this is the store's map entries and Objects plus each policy's bookkeeping.

- Every Object carries an 8-byte `+"`lastAccess`"+` stamp whatever the policy, so random and approx LRU cost the same. Approx LRU's only other bookkeeping is a pool of %d entries, whatever the number of keys.
- LRU adds a doubly linked list node and an index map entry for every key.

| Policy | Heap per key (bytes) | vs Approx LRU |
|---|---:|---:|
`, commas(numKeys), config.EvictionPoolSize)
	for i, p := range policies {
		vs := "same"
		if diff := memory[i] - approx; math.Abs(diff) >= 0.05 {
			vs = fmt.Sprintf("%+.1f bytes (%.2fx)", diff, memory[i]/approx)
		}
		fmt.Fprintf(&b, "| %s | %.1f | %s |\n", p.name, memory[i], vs)
	}
	return b.String()
}

func percent(ratio float64) string {
	return fmt.Sprintf("%.2f%%", ratio*100)
}

// commas: 1000000 -> "1,000,000".
func commas(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
