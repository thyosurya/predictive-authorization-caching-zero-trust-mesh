// control-plane/cache-distributor/token/signer_test.go
package token

import (
    "testing"
)

// TestSignAndVerify memverifikasi token yang ditandatangani dapat diverifikasi kembali
func TestSignAndVerify(t *testing.T) {
    signer, err := NewSigner([]byte("test-secret-key-32-bytes-padded!"))
    if err != nil {
        t.Fatalf("NewSigner() failed: %v", err)
    }
    payload := []byte(`{"sub":"service-a","aud":"service-b","exp":9999999999}`)

    sig, err := signer.Sign(payload)
    if err != nil {
        t.Fatalf("Sign() failed: %v", err)
    }

    if err := signer.Verify(payload, sig); err != nil {
        t.Errorf("Verify() failed for valid signature: %v", err)
    }
}

// TestVerify_TamperedPayload memverifikasi payload yang diubah gagal verifikasi
func TestVerify_TamperedPayload(t *testing.T) {
    signer, _ := NewSigner([]byte("test-secret-key-32-bytes-padded!"))
    original := []byte(`{"sub":"service-a","exp":9999999999}`)
    tampered := []byte(`{"sub":"service-EVIL","exp":9999999999}`)

    sig, _ := signer.Sign(original)
    err := signer.Verify(tampered, sig)

    if err == nil {
        t.Error("expected Verify() to fail for tampered payload, but it succeeded")
    }
}

// TestVerify_WrongKey memverifikasi kunci berbeda gagal verifikasi
func TestVerify_WrongKey(t *testing.T) {
    signerA, _ := NewSigner([]byte("key-a-32-bytes-padded-xxxxxxxxxx!"))
    signerB, _ := NewSigner([]byte("key-b-32-bytes-padded-yyyyyyyyyy!"))

    payload := []byte(`{"sub":"service-a"}`)
    sig, _ := signerA.Sign(payload)

    err := signerB.Verify(payload, sig)
    if err == nil {
        t.Error("expected Verify() to fail with wrong key")
    }
}

// TestVerify_EmptySignature memverifikasi signature kosong selalu ditolak
func TestVerify_EmptySignature(t *testing.T) {
    signer, _ := NewSigner([]byte("test-secret-key-32-bytes-padded!"))
    payload := []byte(`{"sub":"service-a"}`)

    err := signer.Verify(payload, "")
    if err == nil {
        t.Error("expected Verify() to fail for empty signature")
    }
}

// TestNewSigner_ShortKey memverifikasi kunci terlalu pendek ditolak
func TestNewSigner_ShortKey(t *testing.T) {
    _, err := NewSigner([]byte("short-key"))
    if err == nil {
        t.Error("expected error for key shorter than 32 bytes")
    }
}
