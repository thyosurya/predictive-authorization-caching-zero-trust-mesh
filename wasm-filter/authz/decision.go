// wasm-filter/authz/decision.go
package authz

import (
	"github.com/thyosurya/zta-predictive-cache/wasm-filter/token"
)

// Decision merepresentasikan keputusan otorisasi
type Decision int

const (
	Allow   Decision = iota // token valid, akses diizinkan
	Deny                    // token ada tapi action tidak diizinkan
	Expired                 // token ada tapi sudah expired
	Revoked                 // nonce tidak cocok
	Miss                    // tidak ada token di cache (fallback ke PDP)
)

// DecisionResult berisi detail keputusan otorisasi
type DecisionResult struct {
	Decision  Decision
	FromCache bool
	Reason    string // untuk logging
}

// Cache adalah interface untuk operasi cache lokal yang beroperasi langsung pada struct token
type Cache interface {
	Get(key string) (*token.CachedToken, bool)
	Set(key string, value *token.CachedToken) error
	Delete(key string) error
}

// PDPClient adalah interface untuk Policy Decision Point fallback
type PDPClient interface {
	Evaluate(source, target, action string) bool
}

// DecisionEngine adalah mesin keputusan otorisasi utama di dalam sidecar.
// Memutuskan apakah sebuah request diizinkan berdasarkan local cache.
type DecisionEngine struct {
	cache     Cache
	validator *token.Validator
	pdp       PDPClient // fallback ke PDP saat cache miss (bisa nil)
}

// NewDecisionEngine membuat DecisionEngine baru dengan cache dan PDP client.
// pdp bisa nil jika tidak ada fallback PDP.
func NewDecisionEngine(c Cache, pdp PDPClient) *DecisionEngine {
	return &DecisionEngine{
		cache:     c,
		validator: token.NewValidator(""),
		pdp:       pdp,
	}
}

// GetValidator mengembalikan validator yang digunakan oleh engine
func (e *DecisionEngine) GetValidator() *token.Validator {
	return e.validator
}

// GetCache mengembalikan instance Cache lokal yang digunakan oleh engine
func (e *DecisionEngine) GetCache() Cache {
	return e.cache
}

// Decide memutuskan apakah request dari source ke target dengan action tertentu diizinkan.
// Alur in-memory evaluation pada critical path:
//  1. Buat cache key: source + "->" + target
//  2. cache.Get(key) → pointer struct token atau miss (O(1), zero-deserialization, no json.Unmarshal)
//  3. Jika miss → fallback ke PDP jika tersedia, atau return Miss
//  4. Jika ada → validator.Validate(token, action) (in-memory struct comparison)
//  5. Jika Expired → cache.Delete(key), fallback ke PDP
//  6. Jika Revoked → cache.Delete(key), return Revoked
//  7. Jika Deny → return Deny (jangan hapus - bisa jadi action lain valid)
//  8. Jika Allow → return Allow
func (e *DecisionEngine) Decide(source, target, action string) DecisionResult {
	// 1. Buat cache key
	key := source + "->" + target

	// 2. Lookup cache (O(1) in-memory pointer lookup)
	tok, ok := e.cache.Get(key)
	if !ok {
		// 3. Cache miss - fallback ke PDP jika ada
		return e.fallbackToPDP(source, target, action)
	}

	// 4. Validate token langsung dari memori tanpa alokasi JSON
	result := e.validator.Validate(tok, action)

	switch result {
	case token.DecisionExpired:
		// Hapus entri expired dari cache, lalu fallback ke PDP
		e.cache.Delete(key)
		return e.fallbackToPDP(source, target, action)
	case token.DecisionRevoked:
		e.cache.Delete(key)
		return DecisionResult{Decision: Revoked, FromCache: true, Reason: "nonce revoked"}
	case token.DecisionDeny:
		return DecisionResult{Decision: Deny, FromCache: true, Reason: "action not allowed"}
	default: // DecisionAllow
		return DecisionResult{Decision: Allow, FromCache: true, Reason: "cache hit authorized"}
	}
}

// fallbackToPDP mengevaluasi request via PDP jika tersedia
func (e *DecisionEngine) fallbackToPDP(source, target, action string) DecisionResult {
	if e.pdp != nil {
		allowed := e.pdp.Evaluate(source, target, action)
		if allowed {
			return DecisionResult{Decision: Allow, FromCache: false, Reason: "PDP allowed"}
		}
		return DecisionResult{Decision: Deny, FromCache: false, Reason: "PDP denied"}
	}
	return DecisionResult{Decision: Miss, FromCache: false, Reason: "cache miss, no PDP"}
}
