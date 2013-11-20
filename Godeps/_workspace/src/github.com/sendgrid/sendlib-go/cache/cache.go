package cache

import (
	"time"
)

type ByteCacher interface {
	Get(key string) ([]byte, bool, error)
	Set(key string, data []byte, ttl time.Duration) error
}

type MemoryCache struct {
	data        map[string][]byte
	expirations map[string]time.Time
	Size        int
	SoftLimit   int
	HardLimit   int
	FlushTimer  *time.Timer
}

func NewMemoryCache(softLimit, hardLimit int) *MemoryCache {
	c := &MemoryCache{
		data:        make(map[string][]byte),
		expirations: make(map[string]time.Time),
		SoftLimit:   softLimit,
		HardLimit:   hardLimit,
	}

	return c
}

func (c *MemoryCache) Get(key string) ([]byte, bool, error) {
	if val, exists := c.data[key]; !exists {
		return nil, false, nil
	} else if c.expirations[key].After(time.Now()) {
		return val, true, nil
	} else {
		return nil, false, nil
	}
}

func (c *MemoryCache) Set(key string, data []byte, ttl time.Duration) error {
	size := len(data)
	// If we are going to hit the soft limit, cancel the scheduled flush and do it now
	if size > c.SoftLimit {
		if c.FlushTimer != nil {
			c.FlushTimer.Stop()
			c.FlushTimer = nil
		}

		c.FlushExpired()
	}

	// If we are going to hit the hard limit, flush all entries
	if c.Size > c.HardLimit {
		c.FlushAll()
	}

	c.data[key] = data
	c.expirations[key] = time.Now().Add(ttl)
	c.Size += size

	// If we don't already have a flush scheduled, schedule one now
	if c.FlushTimer == nil && ttl > 0 {
		c.FlushTimer = time.AfterFunc(ttl, func() {
			c.FlushExpired()
		})
	}

	return nil
}

func (c *MemoryCache) FlushAll() {
	c.data = make(map[string][]byte, len(c.data))
	c.Size = 0
}

func (c *MemoryCache) FlushExpired() []string {
	now := time.Now()
	deleted := make([]string, 0)
	for k, _ := range c.data {
		if c.expirations[k].Before(now) {
			c.Size = c.Size - len(c.data[k])
			delete(c.data, k)
			deleted = append(deleted, k)
		}
	}

	return deleted
}
