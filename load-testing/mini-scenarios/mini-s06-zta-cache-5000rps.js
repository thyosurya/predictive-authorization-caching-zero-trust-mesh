// load-testing/mini-scenarios/mini-s06-zta-cache-5000rps.js
// Mini Smoke Test S06: ZTA + WASM Cache (5000 RPS Stress Pattern)
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';

const latency = new Trend('mini_s06_latency', true);
const errors = new Counter('mini_s06_errors');

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
    });
    if (!ok) errors.add(1);
    latency.add(res.timings.duration);
    sleep(0.05);
}
