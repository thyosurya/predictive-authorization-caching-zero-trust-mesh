// load-testing/mini-scenarios/mini-s07-zta-cache-fanout.js
// Mini Smoke Test S07: ZTA + WASM Cache Fan-out Batch
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';

const latency = new Trend('mini_s07_latency', true);
const errors = new Counter('mini_s07_errors');

export const options = {
    vus: 2,
    duration: '3s',
    thresholds: {
        http_req_failed: ['rate<0.01'],
        checks: ['rate>0.99'],
    },
};

const BASE_URL = __ENV.BASE_URL || 'http://frontend.zta-research.svc.cluster.local';

export default function () {
    const responses = http.batch([
        ['GET', `${BASE_URL}/api/products`],
        ['POST', `${BASE_URL}/api/cart`, JSON.stringify({ item: 'product-1', qty: 1 }), {
            headers: { 'Content-Type': 'application/json' },
        }],
        ['GET', `${BASE_URL}/health`],
    ]);

    for (const res of responses) {
        const ok = check(res, {
            'status is 200 or 201': (r) => r.status === 200 || r.status === 201,
        });
        if (!ok) errors.add(1);
        latency.add(res.timings.duration);
    }
    sleep(0.05);
}
