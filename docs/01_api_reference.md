# API Reference
## ZTA Predictive Authorization Caching - Kontrak Antar Komponen

Versi: 1.0.0 | Base path semua REST: `http://<service>.<namespace>.svc.cluster.local`

---

## Daftar Isi
1. [Graph Analyzer REST API](#1-graph-analyzer-rest-api)
2. [Cache Distributor REST API](#2-cache-distributor-rest-api)
3. [Cache Distributor gRPC API](#3-cache-distributor-grpc-api)
4. [OPA PDP REST API](#4-opa-pdp-rest-api)
5. [Prometheus Metrics Endpoints](#5-prometheus-metrics-endpoints)
6. [Skema Data Bersama](#6-skema-data-bersama)
7. [Kode Error](#7-kode-error)

---

## 1. Graph Analyzer REST API

**Base URL:** `http://graph-analyzer:8080`

---

### POST /record
Mencatat satu panggilan antar service ke dalam graph topologi. Dipanggil oleh Envoy access log filter setiap kali terjadi East-West traffic.

**Request**
```
POST /record
Content-Type: application/json
```
```json
{
  "source": "spiffe://cluster.local/ns/zta-research/sa/frontend",
  "target": "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice",
  "method": "GET",
  "timestamp": 1700000000000
}
```

| Field | Tipe | Wajib | Keterangan |
|---|---|---|---|
| `source` | string | Ya | SPIFFE ID service pemanggil |
| `target` | string | Ya | SPIFFE ID service yang dipanggil |
| `method` | string | Tidak | HTTP method (GET, POST, dll) |
| `timestamp` | int64 | Tidak | Unix epoch ms, default = waktu server |

**Response**
```
202 Accepted
```
```json
{ "recorded": true }
```

**Error**
```
400 Bad Request - source atau target kosong
```
```json
{ "error": "source and target are required" }
```

---

### GET /candidates
Mengembalikan daftar pasangan service yang direkomendasikan untuk menerima delegation cache berdasarkan analisis graph topologi.

**Request**
```
GET /candidates?threshold=10&limit=50
```

| Query param | Tipe | Default | Keterangan |
|---|---|---|---|
| `threshold` | int | 10 | Minimum panggilan/menit agar masuk kandidat |
| `limit` | int | 50 | Jumlah maksimum kandidat yang dikembalikan |

**Response**
```
200 OK
Content-Type: application/json
```
```json
[
  {
    "source": "spiffe://cluster.local/ns/zta-research/sa/frontend",
    "target": "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice",
    "calls_per_min": 342,
    "suggested_ttl_ms": 30000,
    "sensitivity": "low",
    "centrality_score": 0.87
  },
  {
    "source": "spiffe://cluster.local/ns/zta-research/sa/checkoutservice",
    "target": "spiffe://cluster.local/ns/zta-research/sa/cartservice",
    "calls_per_min": 128,
    "suggested_ttl_ms": 28000,
    "sensitivity": "medium",
    "centrality_score": 0.54
  }
]
```

| Field | Tipe | Keterangan |
|---|---|---|
| `calls_per_min` | int64 | Frekuensi panggilan dalam 1 menit terakhir |
| `suggested_ttl_ms` | int64 | TTL yang dihitung algoritma, dalam milidetik |
| `sensitivity` | string | `low` / `medium` / `high` |
| `centrality_score` | float64 | Skor betweenness centrality (0.0–1.0) |

---

### GET /graph
Mengembalikan snapshot lengkap graph topologi dalam format adjacency list. Digunakan untuk debugging dan visualisasi.

**Request**
```
GET /graph
```

**Response**
```json
{
  "nodes": [
    { "id": "spiffe://cluster.local/ns/zta-research/sa/frontend", "label": "frontend" },
    { "id": "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice", "label": "productcatalogservice" }
  ],
  "edges": [
    {
      "source": "spiffe://cluster.local/ns/zta-research/sa/frontend",
      "target": "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice",
      "weight": 342,
      "last_seen": "2024-01-15T10:30:00Z"
    }
  ],
  "snapshot_at": "2024-01-15T10:31:00Z"
}
```

---

### DELETE /graph/reset
Menghapus seluruh data graph. Digunakan saat reset antar skenario eksperimen.

**Request**
```
DELETE /graph/reset
X-Admin-Token: <ADMIN_TOKEN>
```

**Response**
```
200 OK
```
```json
{ "reset": true, "edges_cleared": 47 }
```

---

### GET /health
Health check endpoint.

```
GET /health
```
```json
{ "status": "ok", "uptime_seconds": 3600 }
```

---

## 2. Cache Distributor REST API

**Base URL:** `http://cache-distributor:8081`

---

### POST /distribute
Meminta Cache Distributor untuk membuat dan mendistribusikan delegation token ke sidecar berdasarkan daftar kandidat.

**Request**
```
POST /distribute
Content-Type: application/json
```
```json
[
  {
    "source": "spiffe://cluster.local/ns/zta-research/sa/frontend",
    "target": "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice",
    "allowed_actions": ["GET"],
    "suggested_ttl_ms": 30000
  }
]
```

**Response**
```
202 Accepted
```
```json
{
  "tokens_created": 1
}
```

---

### POST /revoke
Mencabut delegation token untuk pasangan service tertentu dan mem-push nonce baru ke semua sidecar.

**Request**
```
POST /revoke
Content-Type: application/json
```
```json
{
  "source": "spiffe://cluster.local/ns/zta-research/sa/frontend",
  "target": "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice",
  "reason": "policy_change"
}
```

| Field | Tipe | Wajib | Keterangan |
|---|---|---|---|
| `source` | string | Ya | SPIFFE ID source service |
| `target` | string | Ya | SPIFFE ID target service |
| `reason` | string | Tidak | `policy_change` / `cert_expiry` / `incident` |

**Response**
```
202 Accepted
```
```json
{
  "revoked": true,
  "new_nonce": "f7e3a1c9d2b50468"
}
```

---

### GET /tokens/active
Mengembalikan daftar delegation token aktif yang sedang terdistribusi. Untuk monitoring dan sinkronisasi sidecar.

**Request**
```
GET /tokens/active
```

**Response**
```
200 OK
Content-Type: application/json
```
```json
[
  {
    "source": "spiffe://cluster.local/ns/zta-research/sa/frontend",
    "target": "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice",
    "allowed_actions": ["GET"],
    "exp": 1700045000,
    "ttl_ms": 30000,
    "revocation_nonce": "a3f9c2e1b4d70812"
  }
]
```

---

### GET /health
```
GET /health
```
```json
{ "status": "ok", "grpc_stream_count": 3 }
```

---

## 3. Cache Distributor gRPC API

**Proto package:** `cache` | **Port:** `50051`

```protobuf
service CacheDistributor {
  rpc PushToken(TokenPayload) returns (Ack);
  rpc RevokeToken(RevocationRequest) returns (Ack);
  rpc StreamRevocations(SubscribeRequest) returns (stream RevocationEvent);
}
```

---

### RPC PushToken
Mengirim delegation token ke sidecar Envoy. Dipanggil oleh Cache Distributor setelah token dibuat.

**Request: TokenPayload**
```json
{
  "token_json": "{\"iss\":\"pdp-central\",\"sub\":\"...\",\"aud\":\"...\",\"exp\":1700030000,\"allowed_actions\":[\"GET\"],\"revocation_nonce\":\"a3f9c2e1\",\"cache_ttl_ms\":30000}",
  "signature": "hmac-sha256-base64-encoded-signature"
}
```

**Response: Ack**
```json
{ "success": true, "message": "token stored in local cache" }
```

---

### RPC RevokeToken
Memberitahu sidecar bahwa token untuk pasangan service tertentu telah dicabut.

**Request: RevocationRequest**
```json
{
  "source_identity": "spiffe://cluster.local/ns/zta-research/sa/frontend",
  "target_identity": "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice",
  "old_nonce": "a3f9c2e1b4d70812"
}
```

**Response: Ack**
```json
{ "success": true, "message": "cache entry invalidated" }
```

---

### RPC StreamRevocations
Stream unidireksional dari Cache Distributor ke sidecar. Sidecar subscribe saat startup dan menerima event revokasi secara real-time tanpa polling.

**Request: SubscribeRequest**
```json
{ "sidecar_id": "sidecar-frontend-7d9f" }
```

**Stream: RevocationEvent**
```json
{
  "source_identity": "spiffe://cluster.local/ns/zta-research/sa/frontend",
  "target_identity": "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice",
  "new_nonce": "f7e3a1c9d2b50468",
  "timestamp": 1700000012345
}
```

**Catatan:** Sidecar harus melakukan reconnect dengan exponential backoff jika stream terputus.

---

## 4. OPA PDP REST API

**Base URL:** `http://opa-service:8181`

OPA menggunakan REST API standar. Berikut endpoint yang relevan untuk penelitian ini.

---

### POST /v1/data/authz/allow
Mengevaluasi apakah suatu panggilan diizinkan berdasarkan policy Rego.

**Request**
```
POST /v1/data/authz/allow
Content-Type: application/json
```
```json
{
  "input": {
    "source_identity": "spiffe://cluster.local/ns/zta-research/sa/frontend",
    "target_identity": "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice",
    "action": "GET"
  }
}
```

**Response: Diizinkan**
```json
{ "result": true }
```

**Response: Ditolak**
```json
{ "result": false }
```

---

### PUT /v1/data/service_rules
Memperbarui data rules service (digunakan saat ada perubahan kebijakan).

**Request**
```json
[
  {
    "source": "spiffe://cluster.local/ns/zta-research/sa/frontend",
    "target": "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice",
    "allowed_actions": ["GET"]
  }
]
```

**Response**
```
204 No Content
```

---

### GET /v1/health
```json
{ "bundles": {}, "plugins": { "bundle": { "state": "OK" } } }
```

---

## 5. Prometheus Metrics Endpoints

**Port:** `2112` (Graph Analyzer & Cache Distributor)

---

### Graph Analyzer Metrics

| Metric name | Type | Label | Keterangan |
|---|---|---|---|
| `zta_graph_edges_total` | Gauge | - | Jumlah edge aktif dalam graph |
| `zta_graph_calls_recorded_total` | Counter | `source`, `target` | Total panggilan yang dicatat |
| `zta_graph_candidates_selected_total` | Counter | - | Total pasangan yang dipilih sebagai kandidat |
| `zta_graph_analysis_duration_ms` | Histogram | - | Durasi analisis graph per siklus |

---

### Cache Distributor Metrics

| Metric name | Type | Label | Keterangan |
|---|---|---|---|
| `zta_cache_hits_total` | Counter | `source`, `target` | Keputusan otorisasi dari local cache |
| `zta_cache_misses_total` | Counter | `source`, `target` | Keputusan yang jatuh ke PDP |
| `zta_cache_hit_ratio` | Gauge | - | Rasio hit/(hit+miss) saat ini |
| `zta_tokens_active` | Gauge | - | Jumlah token aktif terdistribusi |
| `zta_tokens_issued_total` | Counter | - | Total token yang pernah diterbitkan |
| `zta_tokens_expired_total` | Counter | - | Total token yang expired |
| `zta_revocation_latency_ms` | Histogram | - | Latensi revokasi end-to-end (ms) |
| `zta_revocation_events_total` | Counter | `reason` | Total event revokasi |
| `zta_grpc_stream_count` | Gauge | - | Jumlah sidecar yang terkoneksi via stream |
| `zta_token_ttl_ms` | Histogram | `source`, `target` | Distribusi TTL token yang diterbitkan |

---

### Contoh Query Prometheus

```promql
# Cache hit ratio keseluruhan
rate(zta_cache_hits_total[1m]) /
(rate(zta_cache_hits_total[1m]) + rate(zta_cache_misses_total[1m]))

# Revocation latency p99
histogram_quantile(0.99, rate(zta_revocation_latency_ms_bucket[5m]))

# Top 5 pasangan service berdasarkan cache hit
topk(5, sum by (source, target) (rate(zta_cache_hits_total[5m])))
```

---

## 6. Skema Data Bersama

### DelegationToken (JSON)

```json
{
  "iss": "pdp-central.spiffe.io",
  "sub": "spiffe://cluster.local/ns/zta-research/sa/<source>",
  "aud": "spiffe://cluster.local/ns/zta-research/sa/<target>",
  "allowed_actions": ["GET", "POST"],
  "sensitivity": "low|medium|high",
  "exp": 1700045000,
  "iat": 1700000000,
  "revocation_nonce": "a3f9c2e1b4d70812",
  "cache_ttl_ms": 30000
}
```

### ServiceEdge (internal Graph Analyzer)

```json
{
  "source": "spiffe://...",
  "target": "spiffe://...",
  "call_count": 342,
  "last_seen": "2024-01-15T10:30:00Z"
}
```

---

## 7. Kode Error

| HTTP Code | Kode internal | Keterangan |
|---|---|---|
| 400 | `ERR_INVALID_INPUT` | Field wajib kosong atau format salah |
| 401 | `ERR_UNAUTHORIZED` | Admin token tidak valid |
| 403 | `ERR_FORBIDDEN` | Otorisasi ditolak oleh policy |
| 404 | `ERR_NOT_FOUND` | Pasangan service tidak ditemukan |
| 409 | `ERR_NONCE_CONFLICT` | Nonce sudah tidak sinkron |
| 500 | `ERR_INTERNAL` | Kesalahan internal server |
| 503 | `ERR_PDP_UNAVAILABLE` | OPA tidak dapat dijangkau |

**Format error response (semua endpoint):**
```json
{
  "error": "ERR_INVALID_INPUT",
  "message": "source and target are required",
  "timestamp": "2024-01-15T10:00:00Z"
}
```
