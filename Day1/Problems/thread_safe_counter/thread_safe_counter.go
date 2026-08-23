package threadsafecounter

import (
	"sync"
)

type Counter struct{
	v int
	mu sync.Mutex
}

func (c *Counter) Increment(){
	defer c.mu.Unlock()
	c.mu.Lock()
	c.v ++
}

func (c *Counter) GetValue() int{
	defer c.mu.Unlock()
	c.mu.Lock()
	return c.v
}

