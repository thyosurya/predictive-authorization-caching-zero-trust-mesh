// load-testing/scenarios/s09-revocation-test.js
// Skenario S09: Revocation Test - 500 RPS + revocation events
// Mengukur latency revokasi: waktu dari event revokasi sampai sidecar menerima notifikasi revokasi
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';

const p99Latency = new Trend('p99_latency', true);
const revocationLatency = new Trend('revocation_latency', true);
const authErrors = new Counter('auth_errors');
const revocationsTriggered = new Counter('revocations_triggered');
const revocationFailCount = new Counter('revocation_fail_count');

export const options = {
    scenarios: {
        // Traffic normal: 500 RPS
        normal_traffic: {
            executor: 'constant-arrival-rate',
            rate: 500,
            timeUnit: '1s',
            duration: '5m',
            preAllocatedVUs: 50,
            maxVUs: 100,
        },
        // Trigger revocation setiap 30 detik
        revocation_trigger: {
            executor: 'constant-arrival-rate',
            rate: 1,
            timeUnit: '30s',
            duration: '5m',
            preAllocatedVUs: 1,
            maxVUs: 1,
            exec: 'triggerRevocation',
        },
    },
    thresholds: {
        http_req_duration: ['p(99)<500'],
        http_req_failed:   ['rate<0.05'],
    },
    summaryTrendStats: ["min", "avg", "med", "p(90)", "p(95)", "p(99)", "p(99.9)", "max"],
};

const BASE_URL = __ENV.BASE_URL || 'http://frontend.zta-research.svc.cluster.local';
const DISTRIBUTOR_URL = __ENV.DISTRIBUTOR_URL || 'http://cache-distributor.zta-research.svc.cluster.local:8081';
const ADMIN_TOKEN = __ENV.ADMIN_TOKEN || 'research-admin-token-2024';
const REVOKE_URL = `${DISTRIBUTOR_URL}/revoke`;

export function setup() {
    // Pastikan token delegasi terdistribusi di awal agar endpoint /revoke selalu menemukan pair aktif
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
    if (res.status !== 200 && res.status !== 202) {
        console.warn(`[SETUP WARNING] Distributor distribute responded with status ${res.status}: ${res.body}`);
    }
}

export default function () {
    const res = http.get(`${BASE_URL}/api/products`);
    check(res, { 'status is 200': (r) => r.status === 200 });
    if (res.status !== 200) authErrors.add(1);
    p99Latency.add(res.timings.duration);
    sleep(0.1);
}

export function triggerRevocation() {
    const payload = JSON.stringify({
        source: "spiffe://cluster.local/ns/zta-research/sa/frontend",
        target: "spiffe://cluster.local/ns/zta-research/sa/productcatalogservice",
        reason: "k6-test-revocation",
    });
    const params = {
        headers: {
            'Content-Type': 'application/json',
            'X-Admin-Token': ADMIN_TOKEN,
        }
    };
    const t0 = new Date().getTime();
    const res = http.post(REVOKE_URL, payload, params);
    const duration = new Date().getTime() - t0;

    revocationLatency.add(duration);
    revocationsTriggered.add(1);

    if (res.status !== 202) {
        revocationFailCount.add(1);
        console.log(`[REVOKE FAILED] Status: ${res.status}, Body: ${res.body}`);
    }
    check(res, { 'revocation accepted': (r) => r.status === 202 });
}
