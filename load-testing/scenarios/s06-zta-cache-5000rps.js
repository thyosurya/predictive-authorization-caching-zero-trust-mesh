// load-testing/scenarios/s06-zta-cache-5000rps.js
// Skenario S06: ZTA + Cache - 5000 RPS (stress test)
// Mengukur batas atas cache di bawah tekanan ekstrem
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';

const p99Latency = new Trend('p99_latency', true);
const authErrors  = new Counter('auth_errors');

export const options = {
    scenarios: {
        constant_load: {
            executor: 'constant-arrival-rate',
            rate: 5000,
            timeUnit: '1s',
            duration: '5m',
            preAllocatedVUs: 500,
            maxVUs: 1500,
        },
    },
    thresholds: {
        http_req_duration: ['p(99)<1000'],
        http_req_failed:   ['rate<0.05'],
    },
    summaryTrendStats: ["min", "avg", "med", "p(90)", "p(95)", "p(99)", "p(99.9)", "max"],
};

const BASE_URL = __ENV.BASE_URL || 'http://frontend.zta-research.svc.cluster.local';

export default function () {
    const res = http.get(`${BASE_URL}/api/products`);
    check(res, { 'status is 200': (r) => r.status === 200 });
    if (res.status !== 200) authErrors.add(1);
    p99Latency.add(res.timings.duration);
}

