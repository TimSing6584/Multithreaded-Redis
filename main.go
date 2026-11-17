package main

import (
	"Redis-k02/thread_safe_counter"
	"fmt"
	"sync"
)

func main(){
	counter := threadsafecounter.Counter{}
	var wg sync.WaitGroup
	for i := 0; i < 100000; i ++ {
		wg.Add(1)
		go func(){
			defer wg.Done()
			counter.Increment()
		}()
	}
	wg.Wait()
	fmt.Println("Final value:", counter.GetValue())
}