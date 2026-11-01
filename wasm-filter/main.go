// wasm-filter/main.go
// Entry point Envoy WASM filter.
// Menginisialisasi plugin dan menghubungkan Envoy HTTP lifecycle
// (OnHttpRequestHeaders) ke DecisionEngine.
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"

	"github.com/thyosurya/zta-predictive-cache/wasm-filter/authz"
	localcache "github.com/thyosurya/zta-predictive-cache/wasm-filter/cache"
	"github.com/thyosurya/zta-predictive-cache/wasm-filter/token"
)

func main() {
	proxywasm.SetVMContext(&vmContext{})
}

type vmContext struct {
	types.DefaultVMContext
}

func (*vmContext) OnVMStart(vmConfigurationSize int) types.OnVMStartStatus {
	proxywasm.LogInfo("ZTA Authorization Cache WASM filter loaded")
	return types.OnVMStartStatusOK
}

func (*vmContext) NewPluginContext(contextID uint32) types.PluginContext {
	return &pluginContext{}
}

type pluginContext struct {
	types.DefaultPluginContext
	engine              *authz.DecisionEngine
	cache               *localcache.LocalCache
	signingKey          []byte
	enableBenchmarkMode bool
	trustDomain         string
	namespace           string
}

func (p *pluginContext) OnPluginStart(pluginConfigurationSize int) types.OnPluginStartStatus {
	// Default Fail-Safe: Mode benchmark MATI secara default di produksi
	p.enableBenchmarkMode = false
	p.trustDomain = "cluster.local"
	p.namespace = "zta-research"
	maxEntries := 1000

	if pluginConfigurationSize > 0 {
		type pluginConfig struct {
			SigningKey          string `json:"signing_key"`
			EnableBenchmarkSeed *bool  `json:"enable_benchmark_seed,omitempty"`
			EnableBenchmarkMode *bool  `json:"enable_benchmark_mode,omitempty"`
			TrustDomain         string `json:"trust_domain,omitempty"`
			Namespace           string `json:"namespace,omitempty"`
			MaxEntries          int    `json:"max_entries,omitempty"`
		}
		if cfgData, err := proxywasm.GetPluginConfiguration(); err == nil && len(cfgData) > 0 {
			var cfg pluginConfig
			if err := json.Unmarshal(cfgData, &cfg); err == nil {
				if len(cfg.SigningKey) >= 32 {
					p.signingKey = []byte(cfg.SigningKey)
				}
				if cfg.EnableBenchmarkSeed != nil {
					p.enableBenchmarkMode = *cfg.EnableBenchmarkSeed
				} else if cfg.EnableBenchmarkMode != nil {
					p.enableBenchmarkMode = *cfg.EnableBenchmarkMode
				}
				if cfg.TrustDomain != "" {
					p.trustDomain = cfg.TrustDomain
				}
				if cfg.Namespace != "" {
					p.namespace = cfg.Namespace
				}
				if cfg.MaxEntries > 0 {
					maxEntries = cfg.MaxEntries
				}
			}
		}
	}

	lc := localcache.NewLocalCache(maxEntries)
	p.cache = lc
	p.engine = authz.NewDecisionEngine(lc, nil) // PDP fallback via ext_authz, not inline

	if len(p.signingKey) < 32 {
		if p.enableBenchmarkMode {
			proxywasm.LogWarn("WASM config: using testbed signing key in benchmark mode")
			p.signingKey = []byte("01234567890123456789012345678901")
		} else {
			// Fail-closed di lingkungan produksi: tolak fallback kunci statis dan nonaktifkan cache
			proxywasm.LogCritical("WASM config: signing_key missing or < 32 bytes in production mode. Local cache disabled (fail-closed).")
			return types.OnPluginStartStatusOK
		}
	}

	if p.enableBenchmarkMode {
		p.seedInitialTokens()
	}

	_ = proxywasm.SetTickPeriodMilliSeconds(2000)
	proxywasm.LogInfo("Plugin initialized with local cache (max " + strconv.Itoa(maxEntries) + " entries)")
	return types.OnPluginStartStatusOK
}

// seedInitialTokens menyuntikkan token awal untuk isolasi pengujian mikro (benchmarking cold-start).
// Pada lingkungan produksi penuh, inisialisasi awal ditangani secara asinkron melalui OnTick pertama
// atau fallback ext_authz ke Policy Decision Point (PDP).
func (p *pluginContext) seedInitialTokens() {
	pairs := []struct {
		source  string
		target  string
		actions []string
		nonce   string
		ttl     int64
	}{
		{
			source:  "spiffe://cluster.local/ns/zta-research/sa/frontend",
			target:  "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice",
			actions: []string{"GET"},
			nonce:   "a1b2c3d4e5f60001",
			ttl:     30,
		},
		{
			source:  "spiffe://cluster.local/ns/zta-research/sa/frontend",
			target:  "spiffe://cluster.local/ns/zta-research/sa/cartservice",
			actions: []string{"GET", "POST"},
			nonce:   "a1b2c3d4e5f60002",
			ttl:     21,
		},
	}

	for _, pair := range pairs {
		p.engine.GetValidator().RegisterNonce(pair.nonce)
		exp := time.Now().Unix() + pair.ttl
		payload := token.CanonicalPayload(pair.source, pair.target, pair.actions, exp, pair.nonce)
		mac := hmac.New(sha256.New, p.signingKey)
		mac.Write(payload)
		sig := base64.URLEncoding.EncodeToString(mac.Sum(nil))

		tok := &token.CachedToken{
			AllowedActions:  pair.actions,
			ExpiresAt:       exp,
			RevocationNonce: pair.nonce,
			Signature:       sig,
		}
		key := pair.source + "->" + pair.target
		_ = p.engine.GetCache().Set(key, tok)
	}
}

func (p *pluginContext) OnTick() {
	headers := [][2]string{
		{":method", "GET"},
		{":path", "/tokens/active"},
		{":authority", "cache-distributor.zta-research.svc.cluster.local:8081"},
		{"accept", "application/json"},
	}

	_, err := proxywasm.DispatchHttpCall(
		"outbound|8081||cache-distributor.zta-research.svc.cluster.local",
		headers,
		nil,
		nil,
		5000,
		p.onTokenResponse,
	)
	if err != nil {
		proxywasm.LogWarnf("Failed to dispatch HTTP callout to cache-distributor: %v", err)
	}
}

func (p *pluginContext) onTokenResponse(numHeaders, bodySize, numTrailers int) {
	body, err := proxywasm.GetHttpCallResponseBody(0, bodySize)
	if err != nil {
		proxywasm.LogErrorf("Failed to read token response: %v", err)
		return
	}

	type activeTokenResponse struct {
		Source          string   `json:"source"`
		Target          string   `json:"target"`
		AllowedActions  []string `json:"allowed_actions"`
		ExpiresAt       int64    `json:"exp"`
		RevocationNonce string   `json:"revocation_nonce"`
		Signature       string   `json:"signature"`
	}

	var tokens []activeTokenResponse
	if err := json.Unmarshal(body, &tokens); err != nil {
		proxywasm.LogErrorf("Failed to unmarshal tokens: %v", err)
		return
	}

	activeKeys := make(map[string]bool, len(tokens))
	validNonces := make(map[string]bool, len(tokens))
	validTokensToCache := make(map[string]*token.CachedToken, len(tokens))

	for _, t := range tokens {
		key := t.Source + "->" + t.Target

		// Fast-path delta check: lewati verifikasi HMAC ulang jika token di cache masih identik
		if existing, ok := p.cache.Get(key); ok {
			if existing.Signature == t.Signature &&
				existing.RevocationNonce == t.RevocationNonce &&
				existing.ExpiresAt == t.ExpiresAt {
				activeKeys[key] = true
				validNonces[t.RevocationNonce] = true
				validTokensToCache[key] = existing
				continue
			}
		}

		// Verifikasi cryptographic HMAC signature untuk token baru atau yang diperbarui
		payload := token.CanonicalPayload(t.Source, t.Target, t.AllowedActions, t.ExpiresAt, t.RevocationNonce)
		if !token.VerifySignature(payload, t.Signature, p.signingKey) {
			proxywasm.LogWarnf("Token signature verification failed for %s -> %s, skipping", t.Source, t.Target)
			continue
		}

		activeKeys[key] = true
		validNonces[t.RevocationNonce] = true

		validTokensToCache[key] = &token.CachedToken{
			AllowedActions:  t.AllowedActions,
			ExpiresAt:       t.ExpiresAt,
			RevocationNonce: t.RevocationNonce,
			Signature:       t.Signature,
		}
	}

	// 1. Eviksi token lama/stale yang sudah tidak aktif TERLEBIH DAHULU
	//    untuk memastikan kapasitas cache selalu memiliki slot kosong sebelum Set() dipanggil.
	for _, key := range p.cache.Keys() {
		if !activeKeys[key] {
			_ = p.cache.Delete(key)
		}
	}

	// 2. Masukkan token baru ke local cache (zero-deserialization in-memory pointer storage)
	for key, tok := range validTokensToCache {
		if err := p.engine.GetCache().Set(key, tok); err != nil {
			proxywasm.LogWarnf("Failed to cache token for %s: %v", key, err)
		}
	}

	// 3. Sinkronisasi atomik registry nonce aktif (cegah memory leak & cabut nonce lama)
	p.engine.GetValidator().SyncActiveNonces(validNonces)
}

func (p *pluginContext) NewHttpContext(contextID uint32) types.HttpContext {
	return &httpContext{
		contextID: contextID,
		engine:    p.engine,
		plugin:    p,
	}
}

type httpContext struct {
	types.DefaultHttpContext
	contextID uint32
	engine    *authz.DecisionEngine
	plugin    *pluginContext
}

func isValidSPIFFEURI(uri string) bool {
	if !strings.HasPrefix(uri, "spiffe://") || len(uri) <= len("spiffe://") {
		return false
	}
	if strings.Contains(uri, "->") {
		return false
	}
	for i := 0; i < len(uri); i++ {
		c := uri[i]
		// Tolak karakter kontrol ASCII (0x00-0x1F, 0x7F), spasi, tanda kutip, dan delimiter payload ('|', ',', '"', ';')
		if c <= 0x20 || c == 0x7F || c == '|' || c == ',' || c == '"' || c == ';' {
			return false
		}
	}
	return true
}

func extractSPIFFEID(raw string) string {
	if raw == "" {
		return ""
	}
	// Periksa header mTLS x-forwarded-client-cert (mendukung format dengan/tanpa kutip)
	if idx := strings.Index(raw, "URI="); idx != -1 {
		sub := raw[idx+4:]
		sub = strings.TrimPrefix(sub, "\"")
		if end := strings.IndexAny(sub, ";\","); end != -1 {
			sub = sub[:end]
		}
		sub = strings.TrimSpace(sub)
		if isValidSPIFFEURI(sub) {
			return sub
		}
		return ""
	}
	trimmed := strings.TrimSpace(raw)
	if isValidSPIFFEURI(trimmed) {
		return trimmed
	}
	// Tolak string identitas yang tidak memiliki skema URI spiffe:// yang valid
	return ""
}

func normalizeTargetIdentity(authority string, opts ...string) string {
	trustDomain := "cluster.local"
	defaultNamespace := "zta-research"
	if len(opts) > 0 && opts[0] != "" {
		trustDomain = opts[0]
	}
	if len(opts) > 1 && opts[1] != "" {
		defaultNamespace = opts[1]
	}

	lower := strings.TrimSpace(strings.ToLower(authority))
	if lower == "" {
		return ""
	}
	if isValidSPIFFEURI(lower) {
		return lower
	}

	// Pisahkan port jika ada (contoh: "productcatalogservice:3550")
	host := lower
	if idx := strings.IndexByte(lower, ':'); idx != -1 {
		host = lower[:idx]
	}

	// Pisahkan domain K8s jika ada (contoh: "productcatalogservice.zta-research.svc.cluster.local")
	parts := strings.Split(host, ".")
	svcName := parts[0]
	ns := defaultNamespace
	if len(parts) >= 2 && parts[1] != "svc" && parts[1] != "" {
		if parts[1] == "zta-research" {
			ns = defaultNamespace
		} else if parts[1] == "default" && defaultNamespace == "zta-research" {
			// Kompatibilitas alias klaster uji coba Online Boutique
			ns = defaultNamespace
		} else {
			ns = parts[1]
		}
	}

	if svcName == "" {
		return ""
	}
	candidateURI := "spiffe://" + trustDomain + "/ns/" + ns + "/sa/" + svcName
	if isValidSPIFFEURI(candidateURI) {
		return candidateURI
	}
	return ""
}

func (ctx *httpContext) OnHttpRequestHeaders(numHeaders int, endOfStream bool) types.Action {
	// Sanitasi wajib: hapus header status cache internal dari request masuk untuk mencegah spoofing klien
	_ = proxywasm.RemoveHttpRequestHeader("x-zta-cache-verified")

	// Ambil identitas source dari header mTLS x-forwarded-client-cert (disanitasi oleh TLS inspector Envoy).
	// Fallback x-source-identity HANYA diizinkan jika enableBenchmarkMode == true (lingkungan testbed k6).
	rawSource, _ := proxywasm.GetHttpRequestHeader("x-forwarded-client-cert")
	if rawSource == "" && ctx.plugin != nil && ctx.plugin.enableBenchmarkMode {
		rawSource, _ = proxywasm.GetHttpRequestHeader("x-source-identity")
	}
	sourceID := extractSPIFFEID(rawSource)
	if sourceID == "" {
		// Zero Trust Fail-Closed: request tanpa identitas terverifikasi langsung ditolak
		proxywasm.LogWarn("Request rejected: missing or invalid client identity (fail-closed)")
		proxywasm.SendHttpResponse(401, nil, []byte("Unauthorized: missing client certificate"), -1)
		return types.ActionPause
	}

	// Ambil target identity dari :authority atau Host
	rawTarget, _ := proxywasm.GetHttpRequestHeader(":authority")
	if rawTarget == "" {
		rawTarget, _ = proxywasm.GetHttpRequestHeader("host")
	}
	trustDomain := "cluster.local"
	namespace := "zta-research"
	if ctx.plugin != nil {
		if ctx.plugin.trustDomain != "" {
			trustDomain = ctx.plugin.trustDomain
		}
		if ctx.plugin.namespace != "" {
			namespace = ctx.plugin.namespace
		}
	}
	targetID := normalizeTargetIdentity(rawTarget, trustDomain, namespace)

	// Ambil HTTP method
	method, _ := proxywasm.GetHttpRequestHeader(":method")

	// Delegasikan keputusan ke DecisionEngine
	result := ctx.engine.Decide(sourceID, targetID, method)

	switch result.Decision {
	case authz.Allow:
		// Tandai bahwa request telah diotorisasi oleh WASM cache agar ext_authz (OPA) melakukan bypass
		_ = proxywasm.AddHttpRequestHeader("x-zta-cache-verified", "true")
		proxywasm.LogDebugf("Cache hit: authorized %s -> %s [%s]", sourceID, targetID, method)
		return types.ActionContinue

	case authz.Miss:
		// Cache miss: lanjutkan tanpa header x-zta-cache-verified agar dievaluasi oleh ext_authz (PDP)
		proxywasm.LogInfof("Cache miss for %s -> %s, forwarding to PDP", sourceID, targetID)
		return types.ActionContinue

	case authz.Expired:
		// Token expired: lanjutkan tanpa header x-zta-cache-verified agar dievaluasi ulang oleh ext_authz (PDP)
		proxywasm.LogInfof("Token expired for %s -> %s, forwarding to PDP", sourceID, targetID)
		return types.ActionContinue

	case authz.Revoked:
		proxywasm.LogInfof("Token revoked for %s -> %s", sourceID, targetID)
		proxywasm.SendHttpResponse(403, nil, []byte("Forbidden: token revoked"), -1)
		return types.ActionPause

	case authz.Deny:
		proxywasm.LogInfof("Action denied: %s -> %s [%s]", sourceID, targetID, method)
		proxywasm.SendHttpResponse(403, nil, []byte("Forbidden"), -1)
		return types.ActionPause

	default:
		return types.ActionContinue
	}
}
