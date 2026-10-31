// wasm-filter/cache/local_cache_test.go
package cache

import (
	"testing"
	"time"

	"github.com/thyosurya/zta-predictive-cache/wasm-filter/token"
)

// TestNewLocalCache memverifikasi inisialisasi cache kosong
func TestNewLocalCache(t *testing.T) {
	c := NewLocalCache(100)
	if c == nil {
		t.Fatal("NewLocalCache() returned nil")
	}
}

// TestSetAndGet memverifikasi data yang disimpan dapat diambil kembali
func TestSetAndGet(t *testing.T) {
	c := NewLocalCache(100)
	tok := &token.CachedToken{
		AllowedActions:  []string{"GET"},
		ExpiresAt:       9999999999,
		RevocationNonce: "nonce-1",
	}
	err := c.Set("service-a->service-b", tok)
	if err != nil {
		t.Fatalf("unexpected error setting token: %v", err)
	}

	got, ok := c.Get("service-a->service-b")
	if !ok {
		t.Fatal("expected key to exist")
	}
	if got.ExpiresAt != 9999999999 || got.RevocationNonce != "nonce-1" {
		t.Errorf("unexpected token data: %+v", got)
	}
}

// TestSetBytes memverifikasi parsing JSON saat ingestion
func TestSetBytes(t *testing.T) {
	c := NewLocalCache(100)
	raw := []byte(`{"allowed_actions":["GET","POST"],"exp":88888888,"revocation_nonce":"nonce-bytes"}`)
	err := c.SetBytes("service-x->service-y", raw)
	if err != nil {
		t.Fatalf("unexpected error setting bytes: %v", err)
	}

	got, ok := c.Get("service-x->service-y")
	if !ok {
		t.Fatal("expected key to exist")
	}
	if got.ExpiresAt != 88888888 || len(got.AllowedActions) != 2 {
		t.Errorf("unexpected token data: %+v", got)
	}
}

// TestGet_Miss memverifikasi key yang tidak ada mengembalikan false
func TestGet_Miss(t *testing.T) {
	c := NewLocalCache(100)
	_, ok := c.Get("nonexistent")
	if ok {
		t.Error("expected miss for nonexistent key")
	}
}

// TestDelete memverifikasi entri yang dihapus tidak bisa diambil lagi
func TestDelete(t *testing.T) {
	c := NewLocalCache(100)
	tok := &token.CachedToken{ExpiresAt: 1234}
	c.Set("key", tok)
	c.Delete("key")

	_, ok := c.Get("key")
	if ok {
		t.Error("expected key to be deleted")
	}
}

// TestMakeKey memverifikasi format key konsisten
func TestMakeKey(t *testing.T) {
	c := NewLocalCache(100)
	key := c.MakeKey("service-a", "service-b")
	expected := "service-a->service-b"
	if key != expected {
		t.Errorf("expected %q, got %q", expected, key)
	}
}

// TestKeys memverifikasi pengembalian seluruh key secara akurat
func TestKeys(t *testing.T) {
	c := NewLocalCache(100)
	c.Set("a->b", &token.CachedToken{ExpiresAt: 1})
	c.Set("c->d", &token.CachedToken{ExpiresAt: 2})

	keys := c.Keys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}

	keyMap := make(map[string]bool)
	for _, k := range keys {
		keyMap[k] = true
	}
	if !keyMap["a->b"] || !keyMap["c->d"] {
		t.Errorf("missing expected keys: %v", keys)
	}
}

// TestSet_MaxEntriesEnforced memverifikasi batas kapasitas cache ditegakkan melalui penggusuran aktif
func TestSet_MaxEntriesEnforced(t *testing.T) {
	c := NewLocalCache(2)
	now := time.Now().Unix()
	tok1 := &token.CachedToken{ExpiresAt: now + 1000}
	tok2 := &token.CachedToken{ExpiresAt: now + 2000}

	if err := c.Set("k1", tok1); err != nil {
		t.Fatalf("failed to insert k1: %v", err)
	}
	if err := c.Set("k2", tok2); err != nil {
		t.Fatalf("failed to insert k2: %v", err)
	}

	// Memperbarui key yang sudah ada harus diizinkan tanpa penggusuran
	if err := c.Set("k1", tok1); err != nil {
		t.Errorf("updating existing key should succeed, got: %v", err)
	}
	if c.EvictionCount() != 0 {
		t.Errorf("updating existing key should not trigger eviction, got: %d", c.EvictionCount())
	}

	// Menambahkan key baru saat kapasitas penuh harus menggusur entri dengan exp terendah (k1)
	tok3 := &token.CachedToken{ExpiresAt: now + 3000}
	err := c.Set("k3", tok3)
	if err != nil {
		t.Fatalf("inserting into full cache should succeed via active eviction, got: %v", err)
	}
	if c.Size() != 2 {
		t.Errorf("expected cache size 2, got: %d", c.Size())
	}
	if c.EvictionCount() != 1 {
		t.Errorf("expected 1 eviction, got: %d", c.EvictionCount())
	}
	if _, ok := c.Get("k1"); ok {
		t.Errorf("expected k1 to be evicted")
	}
	if _, ok := c.Get("k2"); !ok {
		t.Errorf("expected k2 to remain")
	}
	if _, ok := c.Get("k3"); !ok {
		t.Errorf("expected k3 to exist")
	}
}

// TestSet_ActiveEvictionEarliestExpiry memverifikasi bahwa penggusuran aktif memilih entri dengan sisa masa berlaku terpendek
func TestSet_ActiveEvictionEarliestExpiry(t *testing.T) {
	c := NewLocalCache(2)
	now := time.Now().Unix()

	tok1 := &token.CachedToken{ExpiresAt: now + 50}  // Expire paling awal
	tok2 := &token.CachedToken{ExpiresAt: now + 100} // Expire paling lama
	tok3 := &token.CachedToken{ExpiresAt: now + 75}  // Expire menengah

	if err := c.Set("token-short", tok1); err != nil {
		t.Fatalf("failed to insert token-short: %v", err)
	}
	if err := c.Set("token-long", tok2); err != nil {
		t.Fatalf("failed to insert token-long: %v", err)
	}

	// Kapasitas 2 tercapai. Sisipkan token-mid.
	// Harus menggusur token-short (exp +50) dan mempertahankan token-long (exp +100).
	if err := c.Set("token-mid", tok3); err != nil {
		t.Fatalf("failed to insert token-mid: %v", err)
	}

	if _, ok := c.Get("token-short"); ok {
		t.Errorf("token-short with earliest expiration should have been evicted")
	}
	if _, ok := c.Get("token-long"); !ok {
		t.Errorf("token-long should have been preserved")
	}
	if _, ok := c.Get("token-mid"); !ok {
		t.Errorf("token-mid should have been inserted")
	}
	if c.Size() != 2 {
		t.Errorf("expected cache size 2, got: %d", c.Size())
	}
	if c.EvictionCount() != 1 {
		t.Errorf("expected eviction count 1, got: %d", c.EvictionCount())
	}
}

// TestSet_ZeroOrNegativeCapacity memverifikasi kapasitas <= 0 mengembalikan ErrCacheFull
func TestSet_ZeroOrNegativeCapacity(t *testing.T) {
	c := NewLocalCache(0)
	tok := &token.CachedToken{ExpiresAt: 9999999999}
	err := c.Set("k1", tok)
	if err != ErrCacheFull {
		t.Errorf("expected ErrCacheFull for 0 capacity, got: %v", err)
	}
}

// TestSet_StaleFirstEviction memverifikasi entri kedaluwarsa dibersihkan saat kapasitas penuh
func TestSet_StaleFirstEviction(t *testing.T) {
	c := NewLocalCache(2)
	staleTok := &token.CachedToken{ExpiresAt: 100} // Unix 100 is long expired
	activeTok := &token.CachedToken{ExpiresAt: 9999999999}

	if err := c.Set("stale-1", staleTok); err != nil {
		t.Fatalf("failed to insert stale-1: %v", err)
	}
	if err := c.Set("active-1", activeTok); err != nil {
		t.Fatalf("failed to insert active-1: %v", err)
	}

	// Kapasitas penuh (2 entri), tapi 1 stale. Set() harus meng-evict stale-1 dan mengizinkan active-2
	if err := c.Set("active-2", activeTok); err != nil {
		t.Fatalf("expected successful insert with stale eviction, got: %v", err)
	}

	if _, ok := c.Get("stale-1"); ok {
		t.Errorf("expected stale-1 to be evicted")
	}
	if _, ok := c.Get("active-1"); !ok {
		t.Errorf("expected active-1 to remain")
	}
	if _, ok := c.Get("active-2"); !ok {
		t.Errorf("expected active-2 to exist")
	}
}

// BenchmarkLocalCache_Get mengukur latensi O(1) in-memory cache lookup
func BenchmarkLocalCache_Get(b *testing.B) {
	c := NewLocalCache(100)
	tok := &token.CachedToken{ExpiresAt: time.Now().Add(60 * time.Second).Unix()}
	_ = c.Set("test-key", tok)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Get("test-key")
	}
}

// BenchmarkLocalCache_Set_WithEviction mengukur latensi Set di bawah persaingan kapasitas dan active EEF eviction
func BenchmarkLocalCache_Set_WithEviction(b *testing.B) {
	c := NewLocalCache(2)
	now := time.Now().Unix()
	_ = c.Set("k1", &token.CachedToken{ExpiresAt: now + 100})
	_ = c.Set("k2", &token.CachedToken{ExpiresAt: now + 50})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tok := &token.CachedToken{ExpiresAt: now + 75}
		_ = c.Set("k3", tok)
	}
}
