# Proto Specification
## ZTA Predictive Authorization Caching - gRPC Contract Lengkap

Dokumen ini mendefinisikan semua message, field, constraint, dan perilaku gRPC secara lengkap. File `.proto` aktual ada di `control-plane/cache-distributor/proto/cache.proto`.

---

## Daftar Isi
1. [File Proto Lengkap](#1-file-proto-lengkap)
2. [Service: CacheDistributor](#2-service-cachedistributor)
3. [Message Definitions](#3-message-definitions)
4. [Field Constraints & Validation](#4-field-constraints--validation)
5. [Error Codes (gRPC Status)](#5-error-codes-grpc-status)
6. [Contoh Penggunaan (Go Client)](#6-contoh-penggunaan-go-client)
7. [Evolusi Proto (Backward Compatibility)](#7-evolusi-proto-backward-compatibility)

---

## 1. File Proto Lengkap

```protobuf
// control-plane/cache-distributor/proto/cache.proto
syntax = "proto3";

package cache;

option go_package = "github.com/thyosurya/zta-predictive-cache/control-plane/cache-distributor/proto";

// ─────────────────────────────────────────────────
// SERVICE DEFINITION
// ─────────────────────────────────────────────────

// CacheDistributor mengelola distribusi delegation token
// dan revokasi ke semua Envoy sidecar dalam service mesh.
//
// Semua RPC membutuhkan mTLS: client harus menyajikan
// sertifikat SPIFFE yang valid dari namespace zta-research.
service CacheDistributor {

  // --- Core Active RPCs (Runtime Terkompilasi) ---

  // PushToken mengirim delegation token baru ke sidecar.
  // Dipanggil oleh Cache Distributor server setelah token dibuat.
  // Idempotent: token yang sama dapat di-push ulang tanpa efek samping.
  rpc PushToken(TokenPayload) returns (Ack);

  // RevokeToken memberitahu sidecar untuk invalidasi cache entry.
  // Sidecar harus menghapus token dengan nonce yang cocok
  // dan fallback ke PDP untuk request berikutnya.
  rpc RevokeToken(RevocationRequest) returns (Ack);

  // StreamRevocations adalah server-side streaming RPC.
  // Sidecar subscribe saat startup dan menerima RevocationEvent
  // secara real-time tanpa polling.
  // Server mengirim keepalive setiap 30 detik jika tidak ada event.
  rpc StreamRevocations(SubscribeRequest) returns (stream RevocationEvent);

  // --- Extended RPCs (Tersedia via REST API port 8081 / Skema v2) ---

  // GetActiveTokens mengembalikan daftar token aktif yang terdistribusi.
  // Saat ini diekspos melalui REST endpoint GET /tokens/active (port 8081).
  rpc GetActiveTokens(Empty) returns (ActiveTokensList);

  // HealthCheck memverifikasi koneksi gRPC dan status server.
  // Saat ini diekspos melalui REST endpoint GET /health (port 8081).
  rpc HealthCheck(Empty) returns (HealthStatus);
}

// ─────────────────────────────────────────────────
// MESSAGES
// ─────────────────────────────────────────────────

// TokenPayload membungkus delegation token dan signature-nya.
// Sidecar WAJIB memverifikasi signature sebelum menyimpan ke cache.
message TokenPayload {
  // token_json: Delegation token dalam format JSON.
  // Harus berisi semua field yang didefinisikan di DelegationTokenJSON.
  // Tidak boleh kosong. Maksimum 4096 bytes.
  string token_json = 1;

  // signature: HMAC-SHA256 dari token_json, di-encode base64url.
  // Digunakan sidecar untuk memverifikasi keaslian token.
  // Tidak boleh kosong.
  string signature = 2;

  // version: Versi format token untuk forward compatibility.
  // Saat ini selalu "1". Field reserved untuk evolusi proto.
  string version = 3;
}

// DelegationTokenJSON mendefinisikan struktur JSON dalam token_json.
// Ini bukan message proto - didokumentasikan di sini untuk referensi.
//
// JSON Schema:
// {
//   "iss": string,            // Issuer: "pdp-central.spiffe.io"
//   "sub": string,            // Source SPIFFE ID (pemanggil)
//   "aud": string,            // Target SPIFFE ID (yang dipanggil)
//   "allowed_actions": [],    // ["GET", "POST", ...]
//   "sensitivity": string,    // "low" | "medium" | "high"
//   "exp": int64,             // Unix timestamp expiry
//   "iat": int64,             // Unix timestamp issued
//   "revocation_nonce": string, // 16-char hex nonce
//   "cache_ttl_ms": int64     // TTL dalam milidetik
// }

// RevocationRequest membawa informasi pencabutan token.
message RevocationRequest {
  // source_identity: SPIFFE ID service pemanggil.
  // Format: spiffe://cluster.local/ns/<namespace>/sa/<service-account>
  // Tidak boleh kosong.
  string source_identity = 1;

  // target_identity: SPIFFE ID service yang dipanggil.
  // Format sama dengan source_identity.
  // Tidak boleh kosong.
  string target_identity = 2;

  // old_nonce: Nonce dari token yang akan dicabut.
  // Sidecar mencocokkan ini dengan nonce dalam cache entry.
  // Jika nonce tidak cocok, request diabaikan (idempotent).
  // Tidak boleh kosong.
  string old_nonce = 3;

  // reason: Alasan revokasi untuk audit log.
  // Opsional. Nilai yang valid: "policy_change", "cert_expiry",
  // "incident", "scheduled_rotation", "manual".
  string reason = 4;

  // revoked_at: Unix timestamp milidetik saat revokasi terjadi di server.
  // Digunakan sidecar untuk logging dan audit.
  int64 revoked_at = 5;
}

// SubscribeRequest dikirim sidecar saat membuka stream.
message SubscribeRequest {
  // sidecar_id: Identifier unik sidecar, format: "<pod-name>-<container-id[:8]>"
  // Contoh: "frontend-7d9f4b-istio-a1b2c3d4"
  // Digunakan server untuk tracking koneksi aktif.
  // Tidak boleh kosong.
  string sidecar_id = 1;

  // namespace: Kubernetes namespace tempat sidecar berjalan.
  // Digunakan untuk scoping event revokasi.
  string namespace = 2;

  // spiffe_id: SPIFFE ID workload yang di-proxy oleh sidecar.
  // Digunakan untuk memfilter event yang relevan.
  string spiffe_id = 3;

  // protocol_version: Versi protokol yang didukung client.
  // Server akan menyesuaikan format event jika perlu.
  // Saat ini: "1".
  string protocol_version = 4;
}

// RevocationEvent dikirim server ke sidecar saat ada pencabutan.
message RevocationEvent {
  // source_identity: SPIFFE ID source dari token yang dicabut.
  string source_identity = 1;

  // target_identity: SPIFFE ID target dari token yang dicabut.
  string target_identity = 2;

  // new_nonce: Nonce baru yang menggantikan nonce lama.
  // Sidecar harus mengupdate registry nonce-nya.
  // Kosong jika pasangan service ini tidak punya token aktif baru.
  string new_nonce = 3;

  // timestamp: Unix timestamp milidetik saat event ini dibuat.
  int64 timestamp = 4;

  // event_id: UUID event untuk deduplication dan idempotency.
  // Sidecar dapat mengabaikan event dengan event_id yang sudah diproses.
  string event_id = 5;

  // is_keepalive: True jika ini adalah keepalive heartbeat (bukan event nyata).
  // Sidecar harus mengabaikan isi field lain jika is_keepalive = true.
  bool is_keepalive = 6;
}

// Ack adalah response standar untuk unary RPC.
message Ack {
  // success: True jika operasi berhasil.
  bool success = 1;

  // message: Pesan deskriptif untuk logging.
  // Tidak untuk ditampilkan ke pengguna akhir.
  string message = 2;

  // error_code: Kode error internal jika success = false.
  // Lihat tabel ErrorCode di bawah.
  string error_code = 3;
}

// ActiveTokensList berisi semua token aktif yang sedang terdistribusi.
message ActiveTokensList {
  // tokens: Daftar token aktif.
  repeated ActiveTokenSummary tokens = 1;

  // total: Jumlah total token aktif.
  int32 total = 2;

  // snapshot_at: Unix timestamp milidetik saat snapshot diambil.
  int64 snapshot_at = 3;
}

// ActiveTokenSummary adalah ringkasan token aktif (tanpa payload penuh).
message ActiveTokenSummary {
  string source_identity  = 1;
  string target_identity  = 2;
  string nonce            = 3;
  int64  issued_at        = 4;   // Unix timestamp ms
  int64  expires_at       = 5;   // Unix timestamp ms
  int64  ttl_remaining_ms = 6;   // ms hingga expired
  int64  cache_hit_count  = 7;   // jumlah hit sejak token diterbitkan
  string sensitivity      = 8;
}

// HealthStatus dikembalikan oleh HealthCheck RPC.
message HealthStatus {
  // status: "ok" atau "degraded" atau "unavailable"
  string status = 1;

  // grpc_stream_count: jumlah sidecar yang sedang subscribe via streaming.
  int32 grpc_stream_count = 2;

  // active_token_count: jumlah delegation token aktif saat ini.
  int32 active_token_count = 3;

  // opa_reachable: apakah OPA PDP dapat dijangkau.
  bool opa_reachable = 4;

  // uptime_seconds: waktu server sudah berjalan dalam detik.
  int64 uptime_seconds = 5;
}

// Empty digunakan untuk RPC yang tidak membutuhkan input.
message Empty {}
```

---

## 2. Service: CacheDistributor

### RPC: PushToken

```
Client: Cache Distributor server
Server: Envoy sidecar (via gRPC client dalam WASM atau sidecar agent)
Pattern: Unary
Auth: mTLS wajib
```

**Alur:**
```
Cache Distributor
  → buat DelegationToken
  → sign dengan HMAC-SHA256
  → kirim TokenPayload via PushToken()
  → sidecar verifikasi signature
  → sidecar simpan ke shared data Envoy
  → return Ack{success: true}
```

**Jaminan:** Idempotent. Token yang sama (nonce + identitas sama) dapat di-push ulang tanpa efek samping - sidecar cukup overwrite entri yang ada.

---

### RPC: RevokeToken

```
Client: Cache Distributor server
Server: Envoy sidecar
Pattern: Unary
Auth: mTLS wajib
```

**Alur:**
```
Cache Distributor
  → terima event revokasi (dari /revoke REST atau internal trigger)
  → kirim RevocationRequest ke semua sidecar yang diketahui
  → setiap sidecar: cocokkan old_nonce dengan cache entry
  → jika cocok: hapus entri, catat new_nonce
  → return Ack{success: true}
```

**Jaminan:** Best-effort ke semua sidecar. Sidecar yang tidak terjangkau akan mendapat revokasi via StreamRevocations saat reconnect.

---

### RPC: StreamRevocations

```
Client: Envoy sidecar
Server: Cache Distributor server
Pattern: Server-side streaming (long-lived)
Auth: mTLS wajib
```

**Lifecycle stream:**
```
Sidecar startup
  → subscribe via StreamRevocations(SubscribeRequest)
  → server simpan koneksi dalam Broadcaster registry
  → server kirim RevocationEvent setiap kali ada revokasi
  → server kirim keepalive setiap 30 detik (is_keepalive = true)
  → jika koneksi putus: sidecar reconnect dengan exponential backoff
    (100ms → 200ms → 400ms → ... → max 5000ms)
  → saat reconnect: server kirim semua token aktif via PushToken
    (full state sync - mencegah missed event selama disconnect)
```

**Catatan penting:** Stream ini **tidak** menggantikan TTL sebagai mekanisme keamanan. Meski stream putus, token akan expired berdasarkan `exp` field. Stream hanya mempercepat revokasi di bawah TTL.

---

## 3. Message Definitions

### Field Type Summary

| Message | Field | Proto Type | Go Type | Nullable |
|---|---|---|---|---|
| TokenPayload | token_json | string | string | Tidak |
| TokenPayload | signature | string | string | Tidak |
| TokenPayload | version | string | string | Tidak (default "1") |
| RevocationRequest | source_identity | string | string | Tidak |
| RevocationRequest | target_identity | string | string | Tidak |
| RevocationRequest | old_nonce | string | string | Tidak |
| RevocationRequest | reason | string | string | Ya (opsional) |
| RevocationRequest | revoked_at | int64 | int64 | Tidak |
| SubscribeRequest | sidecar_id | string | string | Tidak |
| SubscribeRequest | namespace | string | string | Tidak |
| SubscribeRequest | spiffe_id | string | string | Tidak |
| SubscribeRequest | protocol_version | string | string | Tidak (default "1") |
| RevocationEvent | source_identity | string | string | Tidak |
| RevocationEvent | target_identity | string | string | Tidak |
| RevocationEvent | new_nonce | string | string | Ya (kosong jika tidak ada token baru) |
| RevocationEvent | timestamp | int64 | int64 | Tidak |
| RevocationEvent | event_id | string | string | Tidak |
| RevocationEvent | is_keepalive | bool | bool | Tidak (default false) |
| Ack | success | bool | bool | Tidak |
| Ack | message | string | string | Ya |
| Ack | error_code | string | string | Ya |

---

## 4. Field Constraints & Validation

### TokenPayload

```
token_json:
  - Tidak boleh kosong (len > 0)
  - Harus valid JSON
  - Harus mengandung field: iss, sub, aud, allowed_actions, exp, iat, revocation_nonce, cache_ttl_ms
  - Maksimum 4096 bytes
  - exp harus > waktu sekarang (server menolak token yang sudah expired)

signature:
  - Tidak boleh kosong
  - Harus valid base64url (tanpa padding)
  - Panjang setelah decode: 32 bytes (HMAC-SHA256 output)
```

### RevocationRequest

```
source_identity:
  - Tidak boleh kosong
  - Harus format SPIFFE ID: dimulai dengan "spiffe://"
  - Maksimum 256 karakter

target_identity:
  - Sama dengan source_identity

old_nonce:
  - Tidak boleh kosong
  - Panjang tepat 16 karakter (8 byte hex-encoded)
  - Hanya karakter hexadecimal: [0-9a-f]

reason (opsional):
  - Jika diisi, harus salah satu dari:
    "policy_change" | "cert_expiry" | "incident" | "scheduled_rotation" | "manual"

revoked_at:
  - Harus > 0
  - Harus unix timestamp milidetik (13 digit)
  - Tidak boleh lebih dari 5 menit di masa depan atau masa lalu
```

### SubscribeRequest

```
sidecar_id:
  - Tidak boleh kosong
  - Format: "<pod-name>-<8char-container-id>"
  - Maksimum 128 karakter
  - Hanya alphanumeric dan tanda hubung

namespace:
  - Tidak boleh kosong
  - Harus namespace Kubernetes yang valid

spiffe_id:
  - Tidak boleh kosong
  - Harus format SPIFFE ID
```

---

## 5. Error Codes (gRPC Status)

| gRPC Status | Error Code Internal | Kondisi |
|---|---|---|
| `OK` | - | Operasi berhasil |
| `INVALID_ARGUMENT` | `ERR_MISSING_FIELD` | Field wajib kosong |
| `INVALID_ARGUMENT` | `ERR_INVALID_FORMAT` | Format SPIFFE ID atau nonce salah |
| `INVALID_ARGUMENT` | `ERR_TOKEN_EXPIRED` | Token dalam TokenPayload sudah expired |
| `INVALID_ARGUMENT` | `ERR_INVALID_SIGNATURE` | Signature HMAC tidak cocok |
| `NOT_FOUND` | `ERR_PAIR_NOT_FOUND` | Pasangan source-target tidak terdaftar untuk revokasi |
| `UNAUTHENTICATED` | `ERR_MTLS_REQUIRED` | Koneksi tanpa mTLS |
| `PERMISSION_DENIED` | `ERR_NAMESPACE_MISMATCH` | Sidecar dari namespace yang tidak diizinkan |
| `RESOURCE_EXHAUSTED` | `ERR_STREAM_LIMIT` | Terlalu banyak stream aktif (max 1000) |
| `INTERNAL` | `ERR_OPA_UNAVAILABLE` | OPA tidak dapat dijangkau |
| `INTERNAL` | `ERR_SIGNING_FAILED` | Gagal menandatangani token |
| `UNAVAILABLE` | `ERR_SERVER_OVERLOAD` | Server kelebihan beban |

**Contoh error response dalam Ack:**
```json
{
  "success": false,
  "message": "token signature verification failed",
  "error_code": "ERR_INVALID_SIGNATURE"
}
```

---

## 6. Contoh Penggunaan (Go Client)

### Push Token ke sidecar

```go
// Dari Cache Distributor ke sidecar (dipanggil di distributeLoop)
func (d *DistributorService) pushTokenToSidecar(
    ctx context.Context,
    conn *grpc.ClientConn,
    tok *token.DelegationToken,
) error {
    client := cache.NewCacheDistributorClient(conn)

    tokenJSON, err := tok.ToJSON()
    if err != nil {
        return fmt.Errorf("serialize token: %w", err)
    }

    sig, err := d.signer.Sign(tokenJSON)
    if err != nil {
        return fmt.Errorf("sign token: %w", err)
    }

    ack, err := client.PushToken(ctx, &cache.TokenPayload{
        TokenJson: string(tokenJSON),
        Signature: sig,
        Version:   "1",
    })
    if err != nil {
        return fmt.Errorf("gRPC PushToken: %w", err)
    }
    if !ack.Success {
        return fmt.Errorf("PushToken rejected: %s (%s)", ack.Message, ack.ErrorCode)
    }
    return nil
}
```

### Subscribe ke RevocationStream (dari sidecar agent)

```go
// Dari sidecar agent - subscribe dan handle event
func (s *SidecarAgent) subscribeRevocations(ctx context.Context) {
    backoff := 100 * time.Millisecond

    for {
        select {
        case <-ctx.Done():
            return
        default:
        }

        err := s.runStream(ctx)
        if err != nil {
            log.Printf("stream error: %v, retry in %v", err, backoff)
            time.Sleep(backoff)
            backoff = min(backoff*2, 5*time.Second)
            continue
        }
        backoff = 100 * time.Millisecond // reset setelah sukses
    }
}

func (s *SidecarAgent) runStream(ctx context.Context) error {
    client := cache.NewCacheDistributorClient(s.conn)

    stream, err := client.StreamRevocations(ctx, &cache.SubscribeRequest{
        SidecarId:       s.sidecarID,
        Namespace:       s.namespace,
        SpiffeId:        s.spiffeID,
        ProtocolVersion: "1",
    })
    if err != nil {
        return fmt.Errorf("open stream: %w", err)
    }

    for {
        event, err := stream.Recv()
        if err != nil {
            return fmt.Errorf("recv: %w", err)
        }

        // Abaikan keepalive
        if event.IsKeepalive {
            continue
        }

        // Proses event revokasi
        s.validator.InvalidateNonce(event.OldNonce)
        if event.NewNonce != "" {
            s.validator.RegisterNonce(event.NewNonce)
        }

        log.Printf("revoked: %s->%s (event_id: %s)",
            event.SourceIdentity, event.TargetIdentity, event.EventId)
    }
}
```

---

## 7. Evolusi Proto (Backward Compatibility)

### Aturan yang TIDAK boleh dilanggar

```
1. Jangan mengubah field number yang sudah ada
   Salah:  string source_identity = 2;  (sebelumnya = 1)
   Benar:  Biarkan nomor field tetap

2. Jangan mengubah tipe field yang sudah ada
   Salah:  int32 revoked_at = 5;  (sebelumnya int64)

3. Jangan menghapus field yang sudah ada - gunakan "reserved"
   reserved 7;  // field lama yang dihapus di v2

4. Field baru selalu menggunakan nomor berikutnya yang belum dipakai
   string new_field = 8;  // bukan 1-7
```

### Cara menambah field baru (aman)

```protobuf
// Tambahkan di akhir message, nomor field baru
message RevocationRequest {
  string source_identity = 1;
  string target_identity = 2;
  string old_nonce       = 3;
  string reason          = 4;
  int64  revoked_at      = 5;

  // v2: tambahan untuk audit trail (backward compatible)
  string initiated_by    = 6;   // SPIFFE ID yang meminta revokasi
  string trace_id        = 7;   // distributed trace ID
}
```

### Versioning Proto

| Versi File | Perubahan |
|---|---|
| v1 (saat ini) | Baseline: PushToken, RevokeToken, StreamRevocations, HealthCheck |
| v2 (planned) | Tambah `initiated_by` dan `trace_id` pada RevocationRequest; tambah `GetActiveTokens` |
