package tune

import (
	"container/list"
	"image"
	"sync"
)

// rasterCacheBytes caps a session's raster cache: up to 1 GiB of rasterized snapshots stay
// kept, the least recently shown dropped first.
const rasterCacheBytes = 1 << 30

// draftCacheBytes caps a session's draft raster cache: up to 256 MiB of snapshots rasterized at
// scale 1 for draft frames stay kept, the least recently shown dropped first.
const draftCacheBytes = 256 << 20

// rasterCache is the [render.RasterCache] of a session's pipelines: it keeps rasterized snapshots
// by their index in the take, least recently used evicted first once their pixels pass capacity
// bytes. A cache belongs to one take and one raster geometry: a pipeline built on another gets a
// new cache, so a frame still being drawn against the old take can only refill the old one. It
// is safe for concurrent use.
type rasterCache struct {
	mu sync.Mutex
	// capacity is the most pixel bytes kept; used is what the kept rasters hold.
	capacity int
	used     int
	// order runs from the most to the least recently used *rasterEntry; entries finds one by
	// its snapshot index.
	order   *list.List
	entries map[int]*list.Element
}

// rasterEntry is one kept raster and the snapshot index it was drawn from.
type rasterEntry struct {
	index  int
	raster *image.RGBA
}

// newRasterCache returns an empty cache keeping at most capacity bytes of pixels.
func newRasterCache(capacity int) *rasterCache {
	return &rasterCache{capacity: capacity, order: list.New(), entries: map[int]*list.Element{}}
}

// Get returns the raster kept for the snapshot at index, marking it the most recently used.
func (c *rasterCache) Get(index int) (*image.RGBA, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	element, ok := c.entries[index]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(element)
	return element.Value.(*rasterEntry).raster, true
}

// Put keeps raster as the snapshot at index's, evicting the least recently used rasters until
// the cache is back within capacity. A raster larger than the whole capacity is not kept, and an
// index already kept keeps its first raster: concurrent misses draw the same pixels.
func (c *rasterCache) Put(index int, raster *image.RGBA) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if element, ok := c.entries[index]; ok {
		c.order.MoveToFront(element)
		return
	}
	size := len(raster.Pix)
	if size > c.capacity {
		return
	}
	c.entries[index] = c.order.PushFront(&rasterEntry{index: index, raster: raster})
	c.used += size
	for c.used > c.capacity {
		oldest := c.order.Back()
		evicted := c.order.Remove(oldest).(*rasterEntry)
		delete(c.entries, evicted.index)
		c.used -= len(evicted.raster.Pix)
	}
}
