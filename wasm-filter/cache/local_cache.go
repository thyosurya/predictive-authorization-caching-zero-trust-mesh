// wasm-filter/cache/local_cache.go
package cache

import (
	"errors"
	"sync"
	"time"

	"github.com/thyosurya/zta-predictive-cache/wasm-filter/token"
)

// ErrCacheFull dikembalikan saat kapasitas maksimum tercapai
var ErrCacheFull = errors.New("cache full")

// LocalCache adalah in-memory token cache thread-safe dengan kompleksitas O(1)
// yang menyimpan parsed struct token untuk mengeliminasi deserialisasi JSON berulang pada critical path.
type LocalCache struct {
	mu            sync.RWMutex
	data          map[string]*token.CachedToken
	maxEntries    int
	evictionCount int64
}

// NewLocalCache membuat LocalCache baru dengan batas maksimum entri
func NewLocalCache(maxEntries int) *LocalCache {
	return &LocalCache{
		data:       make(map[string]*token.CachedToken),
		maxEntries: maxEntries,
	}
}

// Get mengambil pointer token dari cache berdasarkan key.
// Return (*token.CachedToken, true) jika ada, (nil, false) jika tidak ada.
// Key format: "source_spiffe_id->target_spiffe_id"
func (c *LocalCache) Get(key string) (*token.CachedToken, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.data[key]
	return val, ok
}

// Set menyimpan pointer token ke cache.
// Jika kapasitas penuh, bersihkan entri yang sudah kedaluwarsa terlebih dahulu (stale-first sweep).
// Jika seluruh entri masih aktif, gusur entri dengan waktu kedaluwarsa terdekat (earliest-expiry active eviction).
func (c *LocalCache) Set(key string, value *token.CachedToken) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.maxEntries <= 0 {
		return ErrCacheFull
	}

	if _, exists := c.data[key]; !exists && len(c.data) >= c.maxEntries {
		now := time.Now().Unix()
		// 1. Stale-first sweep: bersihkan token yang sudah kedaluwarsa
		for k, v := range c.data {
			if v != nil && v.ExpiresAt < now {
				delete(c.data, k)
			}
		}

		// 2. Active eviction: jika masih penuh, gusur entri dengan ExpiresAt paling awal
		if len(c.data) >= c.maxEntries {
			var earliestKey string
			var earliestExp int64 = 1<<63 - 1

			for k, v := range c.data {
				if v == nil {
					earliestKey = k
					break
				}
				if v.ExpiresAt < earliestExp {
					earliestExp = v.ExpiresAt
					earliestKey = k
				}
			}

			if earliestKey != "" {
				delete(c.data, earliestKey)
				c.evictionCount++
			}
		}
	}
	c.data[key] = value
	return nil
}


// SetBytes menyimpan data token mentah JSON ke cache dengan melakukan unmarshal satu kali saat ingestion.
func (c *LocalCache) SetBytes(key string, raw []byte) error {
	tok, err := token.ParseToken(raw)
	if err != nil {
		return err
	}
	return c.Set(key, tok)
}

// Delete menghapus entri dari cache.
func (c *LocalCache) Delete(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
	return nil
}

// MakeKey membuat cache key dari source dan target identity: source + "->" + target
func (c *LocalCache) MakeKey(source, target string) string {
	return source + "->" + target
}

// Keys mengembalikan seluruh key yang tersimpan dalam cache secara thread-safe.
func (c *LocalCache) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	keys := make([]string, 0, len(c.data))
	for k := range c.data {
		keys = append(keys, k)
	}
	return keys
}

// Size mengembalikan jumlah entri saat ini dalam cache.
func (c *LocalCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}

// EvictionCount mengembalikan total jumlah token aktif yang terpaksa digusur karena batas kapasitas.
func (c *LocalCache) EvictionCount() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.evictionCount
}

