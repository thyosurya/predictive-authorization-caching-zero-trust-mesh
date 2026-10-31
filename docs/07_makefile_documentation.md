# Makefile Documentation
## ZTA Predictive Authorization Caching - Panduan Semua Target Build

Dokumen ini menjelaskan setiap target dalam `Makefile`, kapan digunakan, dan apa yang terjadi di baliknya.

---

## Cara Menggunakan Makefile

```bash
# Lihat semua target yang tersedia
make help

# Jalankan target
make <target>

# Jalankan target dengan variabel
make run-scenario SCENARIO=S03
make export-data START="2024-01-15T10:00:00Z" END="2024-01-15T10:05:00Z"
```

---

## Makefile Lengkap

```makefile
# =============================================================================
# ZTA Predictive Authorization Caching - Makefile
# =============================================================================

.DEFAULT_GOAL := help
.PHONY: help deps verify-env setup status \
        build build-analyzer build-distributor build-wasm build-services \
        test test-race test-graph test-ttl test-distributor test-wasm test-opa \
        coverage lint ci \
        config-pbs config-zta-naive config-zta-cache \
        warmup run-scenario run-all \
        reset-light reset-full \
        export-data analyze \
        dashboard port-forward-grafana port-forward-jaeger port-forward-prometheus \
        snapshot-config verify-versions \
        clean

# ── Variabel ──────────────────────────────────────────────────────────────────

NAMESPACE        := zta-research
MONITORING_NS    := monitoring
SPIRE_NS         := spire

GRAPH_ANALYZER_URL   := http://localhost:8080
CACHE_DISTRIBUTOR_URL := http://localhost:8081
PROMETHEUS_URL       := http://localhost:9090
GRAFANA_URL          := http://localhost:3000
JAEGER_URL           := http://localhost:16686

SCENARIO         ?= S03
REPETITION       ?= 1
START            ?= $(shell date -u -d '10 minutes ago' '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || \
                    date -u -v-10M '+%Y-%m-%dT%H:%M:%SZ')
END              ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

ADMIN_TOKEN      ?= $(shell cat .admin-token 2>/dev/null || echo "changeme")

GO               := go
PYTHON           := python3
K6               := k6
KUBECTL          := kubectl
ISTIOCTL         := istioctl

# Warna output
GREEN  := \033[0;32m
YELLOW := \033[0;33m
RED    := \033[0;31m
RESET  := \033[0m

# =============================================================================
# HELP
# =============================================================================

help: ## Tampilkan daftar semua target
	@echo ""
	@echo "ZTA Predictive Authorization Caching"
	@echo "====================================="
	@echo ""
	@awk 'BEGIN {FS = ":.*##"; printf "%-30s %s\n", "Target", "Keterangan"} \
	      /^[a-zA-Z_-]+:.*##/ { printf "  $(GREEN)%-28s$(RESET) %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@echo ""

# =============================================================================
# SETUP & ENVIRONMENT
# =============================================================================

deps: ## Install semua dependensi Go dan Python
	@echo "$(YELLOW)Installing Go dependencies...$(RESET)"
	cd control-plane/graph-analyzer  && $(GO) mod download
	cd control-plane/cache-distributor && $(GO) mod download
	cd wasm-filter                   && $(GO) mod download
	cd services/service-a            && $(GO) mod download
	cd services/service-b            && $(GO) mod download
	cd services/service-c            && $(GO) mod download
	@echo "$(YELLOW)Installing Python dependencies...$(RESET)"
	$(PYTHON) -m pip install -r requirements.txt --quiet
	@echo "$(GREEN)Dependencies installed.$(RESET)"

verify-env: ## Verifikasi semua tools tersedia dengan versi yang benar
	@bash scripts/verify-versions.sh

setup: ## Setup cluster lengkap dari nol (estimasi 10-15 menit)
	@echo "$(YELLOW)[1/7] Creating namespaces...$(RESET)"
	$(KUBECTL) apply -f k8s/cluster/namespace.yaml
	$(KUBECTL) apply -f k8s/cluster/resource-quota.yaml
	@echo "$(YELLOW)[2/7] Installing Istio...$(RESET)"
	$(ISTIOCTL) install --set profile=demo -y
	$(KUBECTL) apply -f k8s/istio/peer-auth-strict.yaml
	$(KUBECTL) apply -f k8s/istio/telemetry.yaml
	@echo "$(YELLOW)[3/7] Deploying SPIRE...$(RESET)"
	$(KUBECTL) apply -f k8s/spire/ -n $(SPIRE_NS)
	$(KUBECTL) wait --for=condition=ready pod -l app=spire-server -n $(SPIRE_NS) --timeout=120s
	bash scripts/register-spire-entries.sh
	@echo "$(YELLOW)[4/7] Deploying OPA + policies...$(RESET)"
	$(KUBECTL) apply -f k8s/apps/opa-deployment.yaml -n $(NAMESPACE)
	$(KUBECTL) create configmap opa-policies \
	  --from-file=authz.rego=control-plane/pdp/policies/authz.rego \
	  --from-file=data.json=control-plane/pdp/policies/data.json \
	  -n $(NAMESPACE) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	@echo "$(YELLOW)[5/7] Deploying testbed (Online Boutique)...$(RESET)"
	$(KUBECTL) apply -f k8s/apps/testbed-boutique.yaml -n $(NAMESPACE)
	$(KUBECTL) wait --for=condition=ready pod --all -n $(NAMESPACE) --timeout=300s
	@echo "$(YELLOW)[6/7] Deploying control plane components...$(RESET)"
	$(KUBECTL) apply -f k8s/apps/graph-analyzer.yaml -n $(NAMESPACE)
	$(KUBECTL) apply -f k8s/apps/cache-distributor.yaml -n $(NAMESPACE)
	$(KUBECTL) wait --for=condition=ready pod -l app=graph-analyzer -n $(NAMESPACE) --timeout=60s
	$(KUBECTL) wait --for=condition=ready pod -l app=cache-distributor -n $(NAMESPACE) --timeout=60s
	@echo "$(YELLOW)[7/7] Deploying monitoring stack...$(RESET)"
	helm upgrade --install monitoring prometheus-community/kube-prometheus-stack \
	  -n $(MONITORING_NS) \
	  --set grafana.adminPassword=ztaresearch \
	  --set prometheus.prometheusSpec.scrapeInterval=5s \
	  --wait --timeout=10m
	$(KUBECTL) apply -f observability/jaeger/jaeger-deploy.yaml -n $(MONITORING_NS)
	@echo "$(GREEN)Setup complete. Run 'make status' to verify.$(RESET)"

status: ## Cek status semua komponen sistem
	@echo "$(YELLOW)=== Pod Status ===$(RESET)"
	@$(KUBECTL) get pods -n $(NAMESPACE) 2>/dev/null || echo "Namespace tidak ditemukan"
	@echo ""
	@echo "$(YELLOW)=== Health Checks ===$(RESET)"
	@$(KUBECTL) port-forward svc/graph-analyzer 8080:8080 -n $(NAMESPACE) &>/dev/null & \
	  PF_PID=$$!; sleep 2; \
	  curl -s $(GRAPH_ANALYZER_URL)/health | python3 -c "import sys,json; d=json.load(sys.stdin); print('Graph Analyzer:', '✅' if d.get('status')=='ok' else '❌')" 2>/dev/null || echo "Graph Analyzer: ❌ (tidak terjangkau)"; \
	  kill $$PF_PID 2>/dev/null
	@echo ""
	@echo "$(YELLOW)=== mTLS Status ===$(RESET)"
	@$(ISTIOCTL) x describe pod $$($(KUBECTL) get pod -n $(NAMESPACE) -l app=frontend -o name 2>/dev/null | head -1) \
	  -n $(NAMESPACE) 2>/dev/null | grep -E "mTLS|Service" || echo "Tidak bisa cek mTLS"

# =============================================================================
# BUILD
# =============================================================================

build: build-analyzer build-distributor build-wasm build-services ## Build semua komponen

build-analyzer: ## Build Graph Analyzer binary
	@echo "$(YELLOW)Building graph-analyzer...$(RESET)"
	cd control-plane/graph-analyzer && \
	  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	  $(GO) build -ldflags="-s -w" -o bin/graph-analyzer .
	@echo "$(GREEN)graph-analyzer built: control-plane/graph-analyzer/bin/graph-analyzer$(RESET)"

build-distributor: ## Build Cache Distributor binary
	@echo "$(YELLOW)Building cache-distributor...$(RESET)"
	cd control-plane/cache-distributor && \
	  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	  $(GO) build -ldflags="-s -w" -o bin/cache-distributor .
	@echo "$(GREEN)cache-distributor built.$(RESET)"

build-wasm: ## Build Envoy WASM filter dengan TinyGo
	@echo "$(YELLOW)Building WASM filter (TinyGo)...$(RESET)"
	cd wasm-filter && \
	  tinygo build -o authz-cache.wasm -scheduler=none -target=wasi .
	$(KUBECTL) create configmap wasm-filter \
	  --from-file=authz-cache.wasm=wasm-filter/authz-cache.wasm \
	  -n $(NAMESPACE) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	@echo "$(GREEN)WASM filter built dan upload ke ConfigMap.$(RESET)"

build-services: ## Build semua microservice testbed
	@for svc in service-a service-b service-c; do \
	  echo "$(YELLOW)Building $$svc...$(RESET)"; \
	  cd services/$$svc && \
	    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
	    $(GO) build -ldflags="-s -w" -o bin/$$svc . && \
	  cd ../..; \
	done
	@echo "$(GREEN)All services built.$(RESET)"

proto: ## Generate Go code dari protobuf definition
	@echo "$(YELLOW)Generating protobuf code...$(RESET)"
	cd control-plane/cache-distributor && \
	  protoc --go_out=. --go-grpc_out=. proto/cache.proto
	@echo "$(GREEN)Proto generated.$(RESET)"

# =============================================================================
# TEST
# =============================================================================

test: ## Jalankan semua unit test
	@echo "$(YELLOW)=== Graph Analyzer Tests ===$(RESET)"
	cd control-plane/graph-analyzer  && $(GO) test ./... -v -count=1
	@echo "$(YELLOW)=== Cache Distributor Tests ===$(RESET)"
	cd control-plane/cache-distributor && $(GO) test ./... -v -count=1
	@echo "$(YELLOW)=== WASM Filter Tests ===$(RESET)"
	cd wasm-filter                   && $(GO) test ./... -v -count=1
	@echo "$(YELLOW)=== OPA Policy Tests ===$(RESET)"
	opa test control-plane/pdp/policies/ -v
	@echo "$(GREEN)All tests passed.$(RESET)"

test-race: ## Jalankan test dengan race detector (wajib sebelum commit)
	@echo "$(YELLOW)Running tests with race detector...$(RESET)"
	cd control-plane/graph-analyzer   && $(GO) test ./... -race -count=1
	cd control-plane/cache-distributor && $(GO) test ./... -race -count=1
	cd wasm-filter                    && $(GO) test ./... -race -count=1
	@echo "$(GREEN)Race detector tests passed.$(RESET)"

test-graph: ## Jalankan test graph topology saja
	cd control-plane/graph-analyzer && $(GO) test ./graph/... -v -run TestTopology

test-ttl: ## Jalankan test TTL calculator saja
	cd control-plane/graph-analyzer && $(GO) test ./cache/... -v -run TestCalculateTTL

test-token: ## Jalankan test delegation token dan signer
	cd control-plane/cache-distributor && $(GO) test ./token/... -v

test-revocation: ## Jalankan test revocation manager
	cd control-plane/cache-distributor && $(GO) test ./revocation/... -v

test-distributor: ## Jalankan semua test cache-distributor
	cd control-plane/cache-distributor && $(GO) test ./... -v

test-wasm: ## Jalankan test wasm-filter logic
	cd wasm-filter && $(GO) test ./... -v

test-opa: ## Jalankan OPA policy test (Rego)
	opa test control-plane/pdp/policies/ -v --format=pretty

coverage: ## Generate coverage report HTML (buka di browser)
	@echo "$(YELLOW)Generating coverage report...$(RESET)"
	cd control-plane/graph-analyzer && \
	  $(GO) test ./... -coverprofile=coverage.out -covermode=atomic && \
	  $(GO) tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)Coverage report: control-plane/graph-analyzer/coverage.html$(RESET)"
	@xdg-open control-plane/graph-analyzer/coverage.html 2>/dev/null || \
	  open control-plane/graph-analyzer/coverage.html 2>/dev/null || \
	  echo "Buka manual: control-plane/graph-analyzer/coverage.html"

lint: ## Jalankan linter pada semua Go code
	@which golangci-lint > /dev/null || \
	  (echo "Install golangci-lint: https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run ./control-plane/...
	golangci-lint run ./wasm-filter/...
	golangci-lint run ./services/...

ci: lint test-race coverage ## Jalankan semua check CI (lint + test-race + coverage)
	@echo "$(GREEN)CI checks passed.$(RESET)"

# =============================================================================
# KONFIGURASI SKENARIO
# =============================================================================

config-pbs: ## Aktifkan PBS Baseline (tanpa otorisasi per-call)
	@echo "$(YELLOW)Switching to PBS Baseline...$(RESET)"
	$(KUBECTL) delete authorizationpolicy --all -n $(NAMESPACE) 2>/dev/null || true
	$(KUBECTL) delete wasmplugin --all -n $(NAMESPACE) 2>/dev/null || true
	$(KUBECTL) apply -f k8s/cluster/network-policy-pbs.yaml -n $(NAMESPACE)
	@bash scripts/validate-state.sh pbs
	@echo "$(GREEN)PBS Baseline active.$(RESET)"

config-zta-naive: ## Aktifkan ZTA Naif (setiap call query PDP pusat)
	@echo "$(YELLOW)Switching to ZTA Naive...$(RESET)"
	$(KUBECTL) delete wasmplugin --all -n $(NAMESPACE) 2>/dev/null || true
	$(KUBECTL) apply -f k8s/istio/authz-policy-zta.yaml -n $(NAMESPACE)
	@bash scripts/validate-state.sh zta-naive
	@echo "$(GREEN)ZTA Naive active.$(RESET)"

config-zta-cache: ## Aktifkan ZTA + Predictive Cache (proposed algorithm)
	@echo "$(YELLOW)Switching to ZTA + Predictive Cache...$(RESET)"
	$(KUBECTL) apply -f k8s/istio/authz-policy-zta.yaml -n $(NAMESPACE)
	$(KUBECTL) apply -f k8s/istio/wasm-filter.yaml
	@sleep 10
	@curl -s -X POST $(CACHE_DISTRIBUTOR_URL)/distribute \
	  -H "Content-Type: application/json" \
	  -d @control-plane/cache-distributor/seed-candidates.json | \
	  python3 -c "import sys,json; d=json.load(sys.stdin); print(f'Distributed {d[\"distributed\"]} tokens')" \
	  2>/dev/null || true
	@bash scripts/validate-state.sh zta-cache
	@echo "$(GREEN)ZTA + Predictive Cache active.$(RESET)"

# =============================================================================
# EKSPERIMEN
# =============================================================================

warmup: ## Jalankan warmup 5 menit sebelum eksperimen
	@echo "$(YELLOW)Running 5-minute warmup...$(RESET)"
	$(K6) run --duration 5m --vus 20 load-testing/warmup.js
	@echo "$(GREEN)Warmup done. Wait 60 seconds before first scenario.$(RESET)"
	@sleep 60

run-scenario: ## Jalankan satu skenario (SCENARIO=S03 REPETITION=1)
	@echo "$(YELLOW)Running scenario $(SCENARIO), repetition $(REPETITION)...$(RESET)"
	@mkdir -p data/raw/$(SCENARIO)
	@START_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	$(K6) run \
	  -e SCENARIO_ID=$(SCENARIO) \
	  -e REPETITION=$(REPETITION) \
	  --out json=data/raw/$(SCENARIO)/k6-$(SCENARIO)-rep$(REPETITION).json \
	  load-testing/scenarios/$$(echo $(SCENARIO) | tr '[:upper:]' '[:lower:]')-*.js; \
	END_TIME=$$(date -u '+%Y-%m-%dT%H:%M:%SZ'); \
	$(PYTHON) scripts/export-metrics.py \
	  --prometheus-url $(PROMETHEUS_URL) \
	  --scenario $(SCENARIO) \
	  --start $$START_TIME \
	  --end $$END_TIME \
	  --output data/raw/$(SCENARIO)/ \
	  --repetition $(REPETITION)
	@echo "$(GREEN)Scenario $(SCENARIO) rep $(REPETITION) done.$(RESET)"

run-all: ## Jalankan semua skenario S01-S10 (estimasi 6-8 jam)
	@echo "$(YELLOW)Starting full experiment run...$(RESET)"
	@bash scripts/run-all-scenarios.sh
	@echo "$(GREEN)All scenarios complete. Run 'make analyze' for results.$(RESET)"

# =============================================================================
# RESET
# =============================================================================

reset-light: ## Reset ringan: flush cache dan graph (antar pengulangan)
	@echo "$(YELLOW)Light reset: flushing cache and graph...$(RESET)"
	$(KUBECTL) rollout restart deployment/cache-distributor -n $(NAMESPACE)
	$(KUBECTL) wait --for=condition=ready pod -l app=cache-distributor -n $(NAMESPACE) --timeout=60s
	@curl -s -X DELETE $(GRAPH_ANALYZER_URL)/graph/reset \
	  -H "X-Admin-Token: $(ADMIN_TOKEN)" | \
	  python3 -c "import sys,json; d=json.load(sys.stdin); print(f'Reset: cleared {d[\"edges_cleared\"]} edges')" \
	  2>/dev/null || echo "Graph reset (port-forward may be needed)"
	@echo "$(YELLOW)Cooldown 30s...$(RESET)"
	@sleep 30
	@echo "$(GREEN)Light reset done.$(RESET)"

reset-full: ## Reset penuh: restart semua pod (antar skenario berbeda)
	@echo "$(YELLOW)Full reset: restarting all pods...$(RESET)"
	$(KUBECTL) delete authorizationpolicy --all -n $(NAMESPACE) 2>/dev/null || true
	$(KUBECTL) delete wasmplugin --all -n $(NAMESPACE) 2>/dev/null || true
	$(KUBECTL) rollout restart deployment -n $(NAMESPACE)
	$(KUBECTL) wait --for=condition=ready pod --all -n $(NAMESPACE) --timeout=180s
	@echo "$(YELLOW)Running short warmup after reset...$(RESET)"
	$(K6) run --duration 2m --vus 10 load-testing/warmup.js
	@echo "$(YELLOW)Cooldown 120s...$(RESET)"
	@sleep 120
	@echo "$(GREEN)Full reset done.$(RESET)"

# =============================================================================
# DATA & ANALISIS
# =============================================================================

export-data: ## Export data dari Prometheus ke CSV (START= END= opsional)
	@echo "$(YELLOW)Exporting metrics from Prometheus...$(RESET)"
	@$(KUBECTL) port-forward svc/monitoring-kube-prometheus-prometheus \
	  9090:9090 -n $(MONITORING_NS) &>/dev/null & \
	  PF_PID=$$!; sleep 3; \
	  for scenario in S01 S02 S03 S04 S05 S06 S07 S08 S09 S10; do \
	    echo "  Exporting $$scenario..."; \
	    $(PYTHON) scripts/export-metrics.py \
	      --prometheus-url $(PROMETHEUS_URL) \
	      --scenario $$scenario \
	      --start "$(START)" \
	      --end "$(END)" \
	      --output data/raw/$$scenario/; \
	  done; \
	  kill $$PF_PID 2>/dev/null
	@echo "$(GREEN)Export done. Files in data/raw/$(RESET)"

merge-results: ## Gabungkan semua data k6 & metrik menggunakan analyze-all-data.py
	@echo "$(YELLOW)Merging and analyzing all testbed data...$(RESET)"
	$(PYTHON) scripts/analyze-all-data.py
	@echo "$(GREEN)Processed and merged datasets in data/processed/$(RESET)"

analyze: merge-results ## Jalankan analisis statistik dan pemrosesan dataset lengkap
	@echo "$(GREEN)Statistical analysis completed via analyze-all-data.py. Reports in data/reports/$(RESET)"
	@ls -la data/reports/ 2>/dev/null || true

verify-offline: ## Jalankan verifikasi statis integritas testbed & skrip
	@echo "$(YELLOW)Running offline verification...$(RESET)"
	$(PYTHON) scripts/verify_offline.py

# =============================================================================
# OBSERVABILITAS
# =============================================================================

dashboard: port-forward-grafana ## Buka Grafana dashboard di browser
	@echo "$(GREEN)Grafana: http://localhost:3000 (admin/ztaresearch)$(RESET)"
	@xdg-open http://localhost:3000 2>/dev/null || \
	  open http://localhost:3000 2>/dev/null || true

port-forward-grafana: ## Port-forward Grafana ke localhost:3000
	$(KUBECTL) port-forward svc/monitoring-grafana 3000:80 -n $(MONITORING_NS) &
	@sleep 2
	@echo "$(GREEN)Grafana forwarded to http://localhost:3000$(RESET)"

port-forward-prometheus: ## Port-forward Prometheus ke localhost:9090
	$(KUBECTL) port-forward svc/monitoring-kube-prometheus-prometheus \
	  9090:9090 -n $(MONITORING_NS) &
	@sleep 2
	@echo "$(GREEN)Prometheus forwarded to http://localhost:9090$(RESET)"

port-forward-jaeger: ## Port-forward Jaeger UI ke localhost:16686
	$(KUBECTL) port-forward svc/jaeger-query 16686:16686 -n $(MONITORING_NS) &
	@sleep 2
	@echo "$(GREEN)Jaeger forwarded to http://localhost:16686$(RESET)"

logs-analyzer: ## Live log Graph Analyzer
	$(KUBECTL) logs -f deployment/graph-analyzer -n $(NAMESPACE)

logs-distributor: ## Live log Cache Distributor
	$(KUBECTL) logs -f deployment/cache-distributor -n $(NAMESPACE)

# =============================================================================
# REPRODUCIBILITY
# =============================================================================

snapshot-config: ## Simpan snapshot konfigurasi untuk reproducibility
	@echo "$(YELLOW)Saving configuration snapshot...$(RESET)"
	@bash scripts/snapshot-config.sh
	@echo "$(GREEN)Snapshot saved.$(RESET)"

verify-versions: ## Verifikasi semua versi komponen sesuai target
	@bash scripts/verify-versions.sh

# =============================================================================
# CLEANUP
# =============================================================================

clean: ## Hapus semua binary build
	rm -f control-plane/graph-analyzer/bin/*
	rm -f control-plane/cache-distributor/bin/*
	rm -f wasm-filter/authz-cache.wasm
	rm -f services/*/bin/*
	rm -f control-plane/graph-analyzer/coverage.out
	rm -f control-plane/graph-analyzer/coverage.html
	@echo "$(GREEN)Cleaned.$(RESET)"

clean-results: ## Hapus semua data eksperimen (HATI-HATI: tidak bisa di-undo)
	@read -p "Yakin hapus semua data di data/raw/ dan data/processed/? [y/N] " ans; \
	  [ "$$ans" = "y" ] && rm -rf data/raw/* data/processed/* data/reports/* && \
	  echo "$(GREEN)Results cleaned.$(RESET)" || echo "Dibatalkan."

teardown: ## Hapus semua resource Kubernetes (HATI-HATI)
	@read -p "Yakin hapus semua resource eksperimen? [y/N] " ans; \
	  [ "$$ans" = "y" ] && \
	  $(KUBECTL) delete namespace $(NAMESPACE) && \
	  echo "$(GREEN)Namespace deleted.$(RESET)" || echo "Dibatalkan."
```

---

## Referensi Target

### Grup: Environment

| Target | Kapan digunakan |
|---|---|
| `make deps` | Sekali saat pertama clone repo |
| `make verify-env` | Sebelum memulai eksperimen, untuk memastikan versi tools sesuai |
| `make setup` | Sekali untuk menyiapkan seluruh cluster dari nol |
| `make status` | Setiap kali ingin cek apakah semua komponen berjalan normal |

### Grup: Build

| Target | Kapan digunakan |
|---|---|
| `make build` | Setelah mengubah kode apapun, sebelum deploy |
| `make build-analyzer` | Hanya jika mengubah `control-plane/graph-analyzer/` |
| `make build-distributor` | Hanya jika mengubah `control-plane/cache-distributor/` |
| `make build-wasm` | Hanya jika mengubah `wasm-filter/` - juga langsung upload ke ConfigMap |
| `make proto` | Hanya jika mengubah `proto/cache.proto` |

### Grup: Test

| Target | Kapan digunakan |
|---|---|
| `make test` | Sebelum commit |
| `make test-race` | Wajib sebelum commit - mendeteksi race condition |
| `make test-graph` | Saat mengerjakan `graph/topology.go` atau `graph/centrality.go` |
| `make test-ttl` | Saat mengubah formula TTL di `cache/ttl.go` |
| `make test-opa` | Saat mengubah `authz.rego` atau `data.json` |
| `make coverage` | Saat ingin melihat seberapa banyak kode yang tercover |
| `make ci` | Jalankan semua check sebelum push ke remote |

### Grup: Konfigurasi Skenario

| Target | Kapan digunakan |
|---|---|
| `make config-pbs` | Sebelum menjalankan S01 |
| `make config-zta-naive` | Sebelum menjalankan S02 atau S04 |
| `make config-zta-cache` | Sebelum menjalankan S03, S05–S10 |

**Urutan wajib sebelum setiap skenario:**
```bash
make reset-full          # reset state
make config-<mode>       # set konfigurasi
make warmup              # warmup singkat
make run-scenario SCENARIO=SXX
```

### Grup: Eksperimen

| Target | Kapan digunakan |
|---|---|
| `make warmup` | Wajib sebelum skenario pertama dan setelah reset-full |
| `make run-scenario SCENARIO=S03` | Menjalankan satu skenario dengan satu pengulangan |
| `make run-all` | Menjalankan seluruh eksperimen secara otomatis (~6-8 jam) |

### Grup: Reset

| Target | Kapan digunakan |
|---|---|
| `make reset-light` | Antar pengulangan dalam skenario yang sama |
| `make reset-full` | Antar skenario yang berbeda konfigurasi |

### Grup: Data & Analisis

| Target | Kapan digunakan |
|---|---|
| `make export-data` | Setelah semua skenario selesai, untuk mengekspor dari Prometheus |
| `make merge-results` | Memproses dan menggabungkan raw data k6 & Prometheus via `analyze-all-data.py` |
| `make analyze` | Menjalankan pipeline analisis statistik lengkap dan pembuatan grafik publikasi |

### Grup: Observabilitas

| Target | Kapan digunakan |
|---|---|
| `make dashboard` | Saat ingin monitoring real-time via Grafana |
| `make port-forward-prometheus` | Saat ingin query PromQL manual |
| `make port-forward-jaeger` | Saat ingin inspeksi trace individual |
| `make logs-analyzer` | Saat debugging Graph Analyzer |
| `make logs-distributor` | Saat debugging Cache Distributor atau revokasi |

### Grup: Reproducibility & Audit

| Target | Kapan digunakan |
|---|---|
| `make snapshot-config` | Wajib dijalankan SEBELUM sesi eksperimen pertama |
| `make verify-versions` | Saat mereplikasi eksperimen di lingkungan berbeda |
| `make verify-offline` | Audit verifikasi statis kelayakan arsitektur ZTA (tanpa butuh cluster aktif) |

---

## Variabel yang Dapat Di-override

```bash
# Menjalankan skenario S05 pengulangan ke-12
make run-scenario SCENARIO=S05 REPETITION=12

# Export data untuk rentang waktu tertentu
make export-data \
  START="2024-01-15T08:00:00Z" \
  END="2024-01-15T14:00:00Z"

# Menggunakan namespace berbeda
make setup NAMESPACE=my-zta-test

# Menggunakan admin token kustom
make reset-light ADMIN_TOKEN=mytoken123
```

---

## Alur Kerja Lengkap (dari nol ke hasil)

```bash
# Hari pertama: setup
git clone <repo> && cd zta-predictive-cache
make deps
make verify-env      # pastikan semua ✅
make setup           # ~15 menit
make status          # semua Running

# Hari pertama: eksperimen
make snapshot-config # simpan state awal

## Kelompok 1: S01 + S02 + S03 (PBS vs ZTA-Naif vs ZTA-Cache @ 500 RPS)
make config-pbs && make warmup
make run-scenario SCENARIO=S01   # ulangi 30x dengan REPETITION=1..30
make reset-full
make config-zta-naive && make warmup
make run-scenario SCENARIO=S02   # ulangi 30x
make reset-full
make config-zta-cache && make warmup
make run-scenario SCENARIO=S03   # ulangi 30x

# Hari berikutnya: analisis
make export-data
make analyze
# Lihat data/reports/ untuk hasil statistik
```
