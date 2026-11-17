package datastructure
// CMS is a Count-Min Sketch data structure for approximate frequency counting.
import (
	"math"
	"github.com/spaolacci/murmur3"
)


type CMS struct {
	width   uint32
	depth   uint32
	counter [][]uint32 // 2D slice to hold the counts for each hash function and bucket
	count uint64 // count is the sum of every increment ever applied 
}

// NewCMS allocates a zeroed width x depth sketch. Callers must pass
// non-zero dimensions (the command layer validates this).
func NewCMS(width uint32, depth uint32) *CMS {
	c := &CMS{
		width:   width,
		depth:   depth,
		counter: make([][]uint32, depth),
	}
	for i := range c.counter {
		c.counter[i] = make([]uint32, width)
	}
	return c
}

// log10PointFive is log10(0.5), precomputed for CalcCMSDim's depth term.
const log10PointFive = -0.30102999566


// CalcCMSDim converts an error budget into sketch dimensions: 

// errRate is how far an estimate may drift (as a fraction of the total count)
// errProb is how often it may drift further than that.
func CalcCMSDim(errRate float64, errProb float64) (uint32, uint32) {
	width := uint32(math.Ceil(2.0 / errRate))
	depth := uint32(math.Ceil(math.Log10(errProb) / log10PointFive))
	return width, depth
}

// Make each row of the sketch a different hash function by seeding Murmur3 with the row index.
func (c *CMS) hash(item string, row uint32) uint32 {
	hasher := murmur3.New32WithSeed(row)
	hasher.Write([]byte(item))
	return hasher.Sum32()
}

// IncrBy adds value to item's counter in every row and returns the new
// estimate. Counters saturate at MaxUint32 rather than wrapping to 0:
// wrapping would break the "never underestimates" guarantee outright.

func (c *CMS) IncrBy(item string, value uint32) uint32 {
	var minCount uint32 = math.MaxUint32
	for row := range c.depth {
		col := c.hash(item, row) % c.width
		if math.MaxUint32-c.counter[row][col] < value {
			c.counter[row][col] = math.MaxUint32
		} else {
			c.counter[row][col] += value
		}
		if c.counter[row][col] < minCount {
			minCount = c.counter[row][col]
		}
	}
	c.count += uint64(value)
	return minCount
}


// Query returns the estimated number of times item was incremented: the
// minimum across all d rows. Never below the true count, sometimes above.
func (c *CMS) Query(item string) uint32 {
	var minCount uint32 = math.MaxUint32
	for row := range c.depth {
		col := c.hash(item, row) % c.width
		if c.counter[row][col] < minCount {
			minCount = c.counter[row][col]
		}
	}
	return minCount
}
// Info reports the sketch's dimensions and the total of every increment applied to it
func (c *CMS) Info() (width uint32, depth uint32, count uint64) {
	return c.width, c.depth, c.count
}