package serve

import (
	"sync"

	"github.com/nbyoung/tableaud/internal/source"
)

// cacheSize is the number of results the server keeps.
const cacheSize = 64

// cache keeps the last cacheSize results, the oldest out first. It holds no
// error.
type cache struct {
	mu    sync.Mutex
	keys  []string
	items map[string]source.Result
}

func newCache() *cache { return &cache{items: map[string]source.Result{}} }

func (c *cache) get(key string) (source.Result, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.items[key]
	return r, ok
}

func (c *cache) put(key string, r source.Result) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[key]; !ok {
		c.keys = append(c.keys, key)
		for len(c.keys) > cacheSize {
			delete(c.items, c.keys[0])
			c.keys = c.keys[1:]
		}
	}
	c.items[key] = r
}
