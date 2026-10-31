# control-plane/pdp/policies/authz_test.rego
package authz_test

import data.authz

# Test: service yang terdaftar diizinkan GET
test_allowed_service_get {
    authz.allow with input as {
        "source_identity": "spiffe://cluster.local/ns/default/sa/frontend",
        "target_identity": "spiffe://cluster.local/ns/default/sa/productcatalogservice",
        "action": "GET"
    }
}

# Test: service yang terdaftar diizinkan POST
test_allowed_service_post {
    authz.allow with input as {
        "source_identity": "spiffe://cluster.local/ns/default/sa/frontend",
        "target_identity": "spiffe://cluster.local/ns/default/sa/cartservice",
        "action": "POST"
    }
}

# Test: service tidak terdaftar ditolak
test_denied_unknown_source {
    not authz.allow with input as {
        "source_identity": "spiffe://cluster.local/ns/default/sa/unknown-service",
        "target_identity": "spiffe://cluster.local/ns/default/sa/productcatalogservice",
        "action": "GET"
    }
}

# Test: action tidak diizinkan ditolak
test_denied_disallowed_action {
    not authz.allow with input as {
        "source_identity": "spiffe://cluster.local/ns/default/sa/frontend",
        "target_identity": "spiffe://cluster.local/ns/default/sa/productcatalogservice",
        "action": "DELETE"   # hanya GET yang diizinkan untuk pasangan ini
    }
}

# Test: target tidak diizinkan ditolak
test_denied_wrong_target {
    not authz.allow with input as {
        "source_identity": "spiffe://cluster.local/ns/default/sa/frontend",
        "target_identity": "spiffe://cluster.local/ns/default/sa/paymentservice",
        "action": "GET"      # frontend tidak punya akses ke paymentservice
    }
}

# Test: default deny (tanpa input)
test_default_deny {
    not authz.allow with input as {}
}
