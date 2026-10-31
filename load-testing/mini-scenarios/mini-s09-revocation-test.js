// load-testing/mini-scenarios/mini-s09-revocation-test.js
// Mini Smoke Test S09: Cache Revocation Lifecycle (/distribute, traffic, /revoke)
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';

const trafficLatency = new Trend('mini_s09_traffic_latency', true);
const revokeLatency = new Trend('mini_s09_revoke_latency', true);
const errors = new Counter('mini_s09_errors');

export const options = {
    vus: 2,
    duration: '3s',
    thresholds: {
        http_req_failed: ['rate<0.05'],
        checks: ['rate>0.95'],
    },
};

const BASE_URL = __ENV.BASE_URL || 'http://frontend.zta-research.svc.cluster.local';
const DISTRIBUTOR_URL = __ENV.DISTRIBUTOR_URL || 'http://cache-distributor.zta-research.svc.cluster.local:8081';
const ADMIN_TOKEN = __ENV.ADMIN_TOKEN || 'research-admin-token-2024';

export function setup() {
    const payload = JSON.stringify([{
        source: "spiffe://cluster.local/ns/zta-research/sa/frontend",
        target: "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice",
        allowed_actions: ["GET"],
        suggested_ttl_ms: 30000,
        sensitivity: "low"
    }]);
    const res = http.post(`${DISTRIBUTOR_URL}/distribute`, payload, {
        headers: {
            'Content-Type': 'application/json',
            'X-Admin-Token': ADMIN_TOKEN,
        }
    });
    check(res, {
        'distributor setup accepted': (r) => r.status === 200 || r.status === 202,
    });
}

export default function () {
    // 1. Normal traffic to frontend
    const resTraffic = http.get(`${BASE_URL}/api/products`);
    const okTraffic = check(resTraffic, {
        'traffic status is 200': (r) => r.status === 200,
    });
    if (!okTraffic) errors.add(1);
    trafficLatency.add(resTraffic.timings.duration);

    // 2. Trigger revocation on distributor
    const revokePayload = JSON.stringify({
        source: "spiffe://cluster.local/ns/zta-research/sa/frontend",
        target: "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice",
        reason: "mini-smoke-test-revocation",
    });
    const resRevoke = http.post(`${DISTRIBUTOR_URL}/revoke`, revokePayload, {
        headers: {
            'Content-Type': 'application/json',
            'X-Admin-Token': ADMIN_TOKEN,
        },
    });
    const okRevoke = check(resRevoke, {
        'revocation accepted 202': (r) => r.status === 202,
    });
    if (!okRevoke) errors.add(1);
    revokeLatency.add(resRevoke.timings.duration);

    sleep(0.05);
}
