// load-testing/mini-scenarios/mini-s02-zta-naive.js
// Mini Smoke Test S02: ZTA Naive (PDP evaluation per request)
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';

const latency = new Trend('mini_s02_latency', true);
const errors = new Counter('mini_s02_errors');

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
        'has products array': (r) => r.body && r.body.includes('['),
    });
    if (!ok) errors.add(1);
    latency.add(res.timings.duration);
    sleep(0.05);
}
