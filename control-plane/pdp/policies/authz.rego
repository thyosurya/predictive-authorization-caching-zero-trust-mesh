# control-plane/pdp/policies/authz.rego
package authz

import future.keywords.if
import future.keywords.in

# Default tolak semua - tidak ada allow tanpa aturan eksplisit
default allow := false

# 1. Bypass jika sudah diverifikasi oleh WASM Cache
allow if {
    input.attributes.request.http.headers["x-zta-cache-verified"] == "true"
}

allow if {
    input.headers["x-zta-cache-verified"] == "true"
}

# 2. Evaluasi langsung (misalnya dipanggil oleh Cache Distributor)
allow if {
    some rule in data.service_rules
    rule.source == input.source_identity
    rule.target == input.target_identity
    input.action in rule.allowed_actions
}

# 3. Evaluasi dari Istio Envoy ext_authz (input.attributes.request.http)
effective_source := src if {
    raw := input.attributes.request.http.headers["x-source-identity"]
    raw != ""
    src := raw
} else := src if {
    cert := input.attributes.request.http.headers["x-forwarded-client-cert"]
    cert != ""
    contains(cert, "URI=spiffe://")
    parts := split(cert, "URI=")
    uri_part := split(parts[1], ";")[0]
    src := uri_part
} else := "spiffe://cluster.local/ns/zta-research/sa/frontend" if {
    true
}

effective_target := tgt if {
    authority := input.attributes.request.http.headers[":authority"]
    authority != ""
    host_clean := split(split(authority, ":")[0], ".")[0]
    tgt := concat("", ["spiffe://cluster.local/ns/zta-research/sa/", host_clean])
} else := tgt if {
    host := input.attributes.request.http.headers["host"]
    host != ""
    host_clean := split(split(host, ":")[0], ".")[0]
    tgt := concat("", ["spiffe://cluster.local/ns/zta-research/sa/", host_clean])
} else := "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice" if {
    true
}

effective_action := act if {
    act := input.attributes.request.http.method
} else := "GET"

allow if {
    some rule in data.service_rules
    rule.source == effective_source
    rule.target == effective_target
    effective_action in rule.allowed_actions
}

# Sensitivity level untuk kandidat cache
# (digunakan Cache Distributor saat query OPA untuk dapat sensitivity)
sensitivity := level if {
    some rule in data.service_rules
    rule.source == input.source_identity
    rule.target == input.target_identity
    level := rule.sensitivity
}

default sensitivity := "medium"
