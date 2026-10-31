// control-plane/cache-distributor/revocation/manager_test.go
package revocation

import (
	"fmt"
    "testing"
    "time"
)

// TestRegisterNonce_Success memverifikasi nonce berhasil didaftarkan
func TestRegisterNonce_Success(t *testing.T) {
    m := NewManager()
    err := m.RegisterNonce("service-a", "service-b", "nonce-abc")
    if err != nil {
        t.Errorf("RegisterNonce() unexpected error: %v", err)
    }
}

// TestValidateNonce_Valid memverifikasi nonce yang terdaftar dinyatakan valid
func TestValidateNonce_Valid(t *testing.T) {
    m := NewManager()
    m.RegisterNonce("service-a", "service-b", "nonce-xyz")

    if !m.ValidateNonce("service-a", "service-b", "nonce-xyz") {
        t.Error("expected nonce to be valid")
    }
}

// TestValidateNonce_Invalid memverifikasi nonce yang salah dinyatakan tidak valid
func TestValidateNonce_Invalid(t *testing.T) {
    m := NewManager()
    m.RegisterNonce("service-a", "service-b", "nonce-correct")

    if m.ValidateNonce("service-a", "service-b", "nonce-wrong") {
        t.Error("expected wrong nonce to be invalid")
    }
}

// TestRevokeAndReissue_UpdatesNonce memverifikasi nonce lama tidak valid setelah revokasi
func TestRevokeAndReissue_UpdatesNonce(t *testing.T) {
    m := NewManager()
    m.RegisterNonce("service-a", "service-b", "nonce-old")

    newNonce, err := m.Revoke("service-a", "service-b")
    if err != nil {
        t.Fatalf("Revoke() unexpected error: %v", err)
    }

    // Nonce lama harus tidak valid
    if m.ValidateNonce("service-a", "service-b", "nonce-old") {
        t.Error("old nonce should be invalid after revocation")
    }

    // Nonce baru harus valid
    if !m.ValidateNonce("service-a", "service-b", newNonce) {
        t.Errorf("new nonce %q should be valid", newNonce)
    }
}

// TestRevoke_UnknownPair memverifikasi revokasi pada pasangan tidak terdaftar menghasilkan error
func TestRevoke_UnknownPair(t *testing.T) {
    m := NewManager()
    _, err := m.Revoke("service-x", "service-y")
    if err == nil {
        t.Error("expected error when revoking unknown service pair")
    }
}

// TestRevocationLatency memverifikasi operasi revokasi selesai dalam batas waktu yang wajar
func TestRevocationLatency(t *testing.T) {
    m := NewManager()
    m.RegisterNonce("service-a", "service-b", "nonce-init")

    start := time.Now()
    _, err := m.Revoke("service-a", "service-b")
    elapsed := time.Since(start)

    if err != nil {
        t.Fatalf("Revoke() failed: %v", err)
    }

    // Operasi revokasi lokal harus sangat cepat (jauh di bawah 1ms)
    if elapsed > 1*time.Millisecond {
        t.Errorf("Revoke() took %v, expected < 1ms", elapsed)
    }
}

// TestConcurrentRevocations memverifikasi thread-safety saat revokasi bersamaan
func TestConcurrentRevocations(t *testing.T) {
    m := NewManager()

    // Daftarkan 50 pasangan service
    for i := 0; i < 50; i++ {
        src := fmt.Sprintf("service-%d", i)
        m.RegisterNonce(src, "service-target", "nonce-init")
    }

    done := make(chan error, 50)
    for i := 0; i < 50; i++ {
        src := fmt.Sprintf("service-%d", i)
        go func(source string) {
            _, err := m.Revoke(source, "service-target")
            done <- err
        }(src)
    }

    for i := 0; i < 50; i++ {
        if err := <-done; err != nil {
            t.Errorf("concurrent Revoke() failed: %v", err)
        }
    }
}


