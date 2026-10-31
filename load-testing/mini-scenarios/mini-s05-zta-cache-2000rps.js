// load-testing/mini-scenarios/mini-s05-zta-cache-2000rps.js
// Mini Smoke Test S05: ZTA + WASM Cache (2000 RPS Target)
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';

const latency = new Trend('mini_s05_latency', true);
const errors = new Counter('mini_s05_errors');

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
