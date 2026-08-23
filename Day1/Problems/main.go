package main

import (
	"Redis-k02/Day1/Problems/thread_safe_counter"
	"fmt"
	"sync"
)

func main(){
	counter := threadsafecounter.Counter{}
	var wg sync.WaitGroup
	wg.Add(100000)
	for i := 0; i < 100000; i ++ {
		go counter.Increment(&wg)
	}
	wg.Wait()
	fmt.Println("Final value: ", counter.GetValue())
}