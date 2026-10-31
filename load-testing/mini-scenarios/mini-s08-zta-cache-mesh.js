// load-testing/mini-scenarios/mini-s08-zta-cache-mesh.js
// Mini Smoke Test S08: ZTA + Cache Mesh Pair Rotation
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';

const latency = new Trend('mini_s08_latency', true);
const errors = new Counter('mini_s08_errors');

export const options = {
    vus: 2,
    duration: '3s',
    thresholds: {
        http_req_failed: ['rate<0.01'],
        checks: ['rate>0.99'],
    },
};

const BASE = __ENV.BASE_URL || 'http://frontend.zta-research.svc.cluster.local';

const PAIRS = [
    { method: 'GET',  path: '/api/products', body: null, headers: {} },
    { method: 'POST', path: '/api/cart',
      body: JSON.stringify({ item: 'product-1', qty: 1 }),
      headers: { 'Content-Type': 'application/json' } },
    { method: 'GET',  path: '/health', body: null, headers: {} },
];

export default function () {
    const pair = PAIRS[__ITER % PAIRS.length];
    const res = http.request(pair.method, `${BASE}${pair.path}`, pair.body || null, {
        headers: pair.headers,
    });
    const ok = check(res, {
        'status is 200 or 201': (r) => r.status === 200 || r.status === 201,
    });
    if (!ok) errors.add(1);
    latency.add(res.timings.duration);
    sleep(0.05);
}
