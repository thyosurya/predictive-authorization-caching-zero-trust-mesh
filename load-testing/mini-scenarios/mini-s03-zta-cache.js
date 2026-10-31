// load-testing/mini-scenarios/mini-s03-zta-cache.js
// Mini Smoke Test S03: ZTA + WASM Cache (500 RPS Target)
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';

const latency = new Trend('mini_s03_latency', true);
const errors = new Counter('mini_s03_errors');

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
    const res = http.get(`${BASE_URL}/api/products`);
    const ok = check(res, {
        'status is 200': (r) => r.status === 200,
        'valid product response': (r) => r.body && r.body.length > 10,
    });
    if (!ok) errors.add(1);
    latency.add(res.timings.duration);
    sleep(0.05);
}
