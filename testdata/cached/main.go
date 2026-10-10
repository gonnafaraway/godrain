// Command cached retains a bounded in-memory cache that plateaus under load.
package main

import (
	"container/list"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"sync"
	"time"
)

type entry struct {
	key   int
	value []byte
}

type LRU struct {
	mu       sync.Mutex
	capacity int
	ll       *list.List
	items    map[int]*list.Element
}

func NewLRU(cap int) *LRU {
	return &LRU{
		capacity: cap,
		ll:       list.New(),
		items:    make(map[int]*list.Element),
	}
}

func (c *LRU) Add(key int, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		c.ll.MoveToFront(e)
		e.Value.(*entry).value = value
		return
	}
	e := c.ll.PushFront(&entry{key: key, value: value})
	c.items[key] = e
	for c.ll.Len() > c.capacity {
		back := c.ll.Back()
		if back == nil {
			break
		}
		c.ll.Remove(back)
		delete(c.items, back.Value.(*entry).key)
	}
}

func main() {
	const capacity = 64
	cache := NewLRU(capacity)
	// Prefill so the watch window observes steady-state, not cold cache growth.
	for i := 0; i < capacity; i++ {
		cache.Add(i, make([]byte, 32*1024))
	}

	go func() {
		i := 0
		for {
			// Only touch keys inside capacity so size stays bounded.
			cache.Add(i%capacity, make([]byte, 32*1024))
			i++
			time.Sleep(10 * time.Millisecond)
		}
	}()

	addr := envOr("GODRAIN_DEMO_ADDR", ":6062")
	fmt.Println("cached listening on", addr, "pprof at /debug/pprof/")
	if err := http.ListenAndServe(addr, nil); err != nil {
		panic(err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
