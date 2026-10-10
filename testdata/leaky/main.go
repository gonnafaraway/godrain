// Command leaky intentionally retains unbounded heap allocations for godrain demos.
package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"sync"
	"time"
)

var (
	mu   sync.Mutex
	leak = make(map[int][]byte)
	n    int
)

func main() {
	go func() {
		for {
			mu.Lock()
			leak[n] = make([]byte, 32*1024)
			n++
			mu.Unlock()
			time.Sleep(20 * time.Millisecond)
		}
	}()

	addr := envOr("GODRAIN_DEMO_ADDR", ":6061")
	fmt.Println("leaky listening on", addr, "pprof at /debug/pprof/")
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
