package datastructure
// BloomFilter data structure. 
import (
	"math"
	"math/rand/v2"
	"github.com/spaolacci/murmur3"
)

const ln2Square = math.Ln2 * math.Ln2


// BloomFilter is one filter. array is the bit array, packed 8 bits per byte -
// []bool would work and be shorter, but it would spend a whole byte per
// bit and throw away the 8x memory win that is the entire point of the
// structure.

type BloomFilter struct {
	hashes      int     // how many hash positions to set per entry
	entries     uint64  // Maximum number of entries intended to be added to the filter or we can call Capacity
	errorRate   float64
	array       []uint8
	bits        uint64 // len(array) in bits
	bytes       uint64 // len(array) in bytes
	items       uint64 // number of items added (only adds that set a new bit count)

	// seed is random per filter and fixed for its lifetime, so colliding items
	// can't be precomputed. Keep it secret, and persist it alongside array.
	seed uint32
}

// optimalBits returns the bit-array size for a capacity and error rate:
// bits = -capacity * ln(errorRate) / ln(2)^2
// It stays a float64 so a huge request can be checked against a limit
// before converting to uint64, which would overflow.
func optimalBits(entries uint64, errorRate float64) float64 {
	return math.Ceil(-float64(entries) * math.Log(errorRate) / ln2Square)
}

// BloomFilterSize returns how many bytes NewBloomFilter(entries, errorRate)
// would allocate, so the command layer can reject oversized filters first.
func BloomFilterSize(entries uint64, errorRate float64) float64 {
	return math.Ceil(optimalBits(entries, errorRate) / 8)
}

// optimalHashes returns the number of hash positions per item:
// hashes = (bits / capacity) * ln(2)
func optimalHashes(bits, entries uint64) int {
	return int(math.Ceil(float64(bits) / float64(entries) * math.Ln2))
}

// NewBloomFilter allocates an empty filter sized for entries items at errorRate.
// Callers must pass entries > 0, 0 < errorRate < 1, and a size (BloomFilterSize)
// they can afford (the command layer validates this).
func NewBloomFilter(entries uint64, errorRate float64) *BloomFilter {
	bytes := uint64(BloomFilterSize(entries, errorRate)) // rounded up to whole bytes

	b := &BloomFilter{
		entries:   entries,
		errorRate: errorRate,
		bits:      bytes * 8,
		bytes:     bytes,
		seed:      rand.Uint32(), // math/rand/v2 is seeded from OS randomness
	}
	b.hashes = optimalHashes(b.bits, entries)
	b.array = make([]uint8, b.bytes)
	return b
}

// hash returns the two 64-bit halves of one murmur3-128 digest. k
// independent hashes are then synthesised as a + b*i (the
// Kirsch-Mitzenmacher trick) instead of running k separate hash
// functions
func (b *BloomFilter) hash(item string) (uint64, uint64) {
	return murmur3.Sum128WithSeed([]byte(item), b.seed)
}

// bitPos returns the i-th bit index for a digest, plus the byte holding it
// and the mask selecting it inside that byte.
func (b *BloomFilter) bitPos(h1, h2 uint64, i int) (byteIdx uint64, mask uint8) {
	pos := (h1 + h2*uint64(i)) % b.bits
	return pos >> 3, 1 << (pos % 8) // pos>>3 == pos/8, pos%8 == bit in byte
}

// AddIfNotExist sets all k bits for item and reports whether the item was
// new - 1 if at least one of those bits was still 0, 0 means "possibly already added".
func (b *BloomFilter) AddIfNotExist(item string) int {
	h1, h2 := b.hash(item)
	added := 0
	for i := range b.hashes {
		byteIdx, mask := b.bitPos(h1, h2, i)
		if b.array[byteIdx]&mask == 0 {
			added = 1
		}
		b.array[byteIdx] |= mask
	}
	b.items += uint64(added)
	return added
}

// Exist reports whether item may be in the filter. 0 is certain (some
// bit is 0, so item was never added); 1 is probabilistic.
func (b *BloomFilter) Exist(item string) int {
	h1, h2 := b.hash(item)
	for i := range b.hashes {
		byteIdx, mask := b.bitPos(h1, h2, i)
		if b.array[byteIdx]&mask == 0 {
			return 0
		}
	}
	return 1
}

// Info reports the filter's capacity, target error rate, hashes per item,
// size in bytes and items added. The seed is deliberately left out so it stays secret.
func (b *BloomFilter) Info() (entries uint64, errorRate float64, hashes int, bytes uint64, items uint64) {
	return b.entries, b.errorRate, b.hashes, b.bytes, b.items
}