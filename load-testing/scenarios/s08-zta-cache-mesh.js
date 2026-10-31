// load-testing/scenarios/s08-zta-cache-mesh.js
// Skenario S08: ZTA + Cache Mesh - 2000 RPS all-to-all
// Mengukur efektivitas cache saat semua service saling memanggil (mesh topology)
// Semua request dikirim via frontend sebagai entry point publik.
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';

const p99Latency = new Trend('p99_latency', true);
const authErrors  = new Counter('auth_errors');

export const options = {
    scenarios: {
        mesh_traffic: {
            executor: 'constant-arrival-rate',
            rate: 2000,
            timeUnit: '1s',
            duration: '5m',
            preAllocatedVUs: 200,
            maxVUs: 400,
        },
    },
    thresholds: {
        http_req_duration: ['p(99)<500'],
        http_req_failed:   ['rate<0.01'],
    },
    summaryTrendStats: ["min", "avg", "med", "p(90)", "p(95)", "p(99)", "p(99.9)", "max"],
};

const BASE = __ENV.BASE_URL || 'http://frontend.zta-research.svc.cluster.local';

// Simulasi mesh topology: berbagai pasangan service-to-service
// Semua dikirim via frontend (entry point) karena ZTA menjaga inter-service di layer backend
const PAIRS = [
    // frontend → productcatalogservice (via GET /api/products)
    { method: 'GET',  path: '/api/products', body: null, headers: {} },
    // frontend → cartservice (via POST /api/cart)
    { method: 'POST', path: '/api/cart',
      body: JSON.stringify({ item: 'product-1', qty: 1 }),
      headers: { 'Content-Type': 'application/json' } },
    // health check (monitoring path)
    { method: 'GET',  path: '/health', body: null, headers: {} },
];

export default function () {
    const pair = PAIRS[__ITER % PAIRS.length];
    const res = http.request(pair.method, `${BASE}${pair.path}`, pair.body || null, {
        headers: pair.headers,
    });
    const isOk = res.status === 200 || res.status === 201;
    check(res, { 'status is 200 or 201': () => isOk });
    if (!isOk) authErrors.add(1);
    p99Latency.add(res.timings.duration);
    sleep(0.05);
}
