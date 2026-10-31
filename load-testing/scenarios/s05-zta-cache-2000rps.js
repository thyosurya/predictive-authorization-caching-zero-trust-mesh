// load-testing/scenarios/s05-zta-cache-2000rps.js
// Skenario S05: ZTA + Predictive Cache - 2000 RPS (high load)
// Mengukur apakah cache tetap efektif pada beban tinggi
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';

const p99Latency = new Trend('p99_latency', true);
const authErrors  = new Counter('auth_errors');

export const options = {
    scenarios: {
        constant_load: {
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

const BASE_URL = __ENV.BASE_URL || 'http://frontend.zta-research.svc.cluster.local';

export default function () {
    const res = http.get(`${BASE_URL}/api/products`);
    check(res, { 'status is 200': (r) => r.status === 200 });
    if (res.status !== 200) authErrors.add(1);
    p99Latency.add(res.timings.duration);
    sleep(0.05);
}

