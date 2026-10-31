// control-plane/cache-distributor/revocation/manager.go
package revocation

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// NonceEntry merepresentasikan satu entri nonce dalam registry
type NonceEntry struct {
	Nonce     string
	IssuedAt  time.Time
	RevokedAt *time.Time // nil jika masih aktif
}

// Manager adalah registry pusat untuk nonce aktif semua delegation token.
// Satu-satunya komponen yang berwenang mengubah state nonce.
type Manager struct {
	mu        sync.RWMutex
	active    map[string]*NonceEntry // key: "source->target"
	blacklist map[string]time.Time   // nonce → waktu revokasi
}

// NewManager membuat Manager baru
func NewManager() *Manager {
	return &Manager{
		active:    make(map[string]*NonceEntry),
		blacklist: make(map[string]time.Time),
	}
}

// GenerateNonce membuat 8 byte random, encode hex. Return 16-char string.
// Fungsi level paket - bisa dipanggil dari luar manager.
func GenerateNonce() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// RegisterNonce mendaftarkan nonce baru untuk pasangan service.
// Error jika source/target/nonce kosong.
func (m *Manager) RegisterNonce(source, target, nonce string) error {
	if source == "" || target == "" || nonce == "" {
		return fmt.Errorf("source, target, and nonce must not be empty")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := source + "->" + target
	now := time.Now()
	m.active[key] = &NonceEntry{
		Nonce:    nonce,
		IssuedAt: now,
	}
	return nil
}

// ValidateNonce return true HANYA jika:
//  1. Ada entri aktif untuk pasangan ini
//  2. Nonce cocok dengan entri aktif
//  3. Nonce tidak ada dalam blacklist
//
// Thread-safe (read lock).
func (m *Manager) ValidateNonce(source, target, nonce string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := source + "->" + target

	entry, exists := m.active[key]
	if !exists {
		return false
	}
	if entry.Nonce != nonce {
		return false
	}
	// Cek blacklist
	if _, blacklisted := m.blacklist[nonce]; blacklisted {
		return false
	}
	return true
}

// Revoke mencabut nonce aktif untuk pasangan service dan generate nonce baru.
// Alur:
//  1. Cek pasangan ada dalam active map
//  2. Masukkan nonce lama ke blacklist dengan timestamp sekarang
//  3. Generate nonce baru (GenerateNonce())
//  4. Update active map dengan nonce baru
//  5. Return nonce baru
//
// Thread-safe (write lock - blokir semua read saat revokasi).
func (m *Manager) Revoke(source, target string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := source + "->" + target

	entry, exists := m.active[key]
	if !exists {
		return "", fmt.Errorf("pair %s not found", key)
	}

	// Masukkan nonce lama ke blacklist
	now := time.Now()
	m.blacklist[entry.Nonce] = now
	entry.RevokedAt = &now

	// Generate nonce baru
	newNonce, err := GenerateNonce()
	if err != nil {
		return "", err
	}

	// Update active map dengan nonce baru
	m.active[key] = &NonceEntry{
		Nonce:    newNonce,
		IssuedAt: now,
	}

	return newNonce, nil
}

// PruneBlacklist menghapus entri blacklist yang sudah lebih tua dari threshold.
// Dipanggil periodik dari main.go. Threshold disarankan >= 2 x MaxTTL (= 60 detik).
func (m *Manager) PruneBlacklist(olderThan time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cutoff := time.Now().Add(-olderThan)
	for nonce, revokedAt := range m.blacklist {
		if revokedAt.Before(cutoff) {
			delete(m.blacklist, nonce)
		}
	}
}
