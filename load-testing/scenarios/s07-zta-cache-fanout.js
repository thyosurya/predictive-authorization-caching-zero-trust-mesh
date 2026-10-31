// load-testing/scenarios/s07-zta-cache-fanout.js
// Skenario S07: ZTA + Cache Fan-out - 2000 RPS multi-endpoint via frontend
// Mengukur efektivitas graph topology analyzer pada fan-out topology.
// Setiap iterasi memanggil 3 downstream service secara batch (simulasi fan-out).
// Rate=2000 iter/s → ~6000 HTTP requests/s (3 per batch).
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
    // Fan-out: panggil beberapa endpoint sekaligus dalam satu iterasi (batch)
    const responses = http.batch([
        ['GET',  `${BASE_URL}/api/products`],
        ['POST', `${BASE_URL}/api/cart`, JSON.stringify({ item: 'product-1', qty: 1 }), {
            headers: { 'Content-Type': 'application/json' },
        }],
        ['GET',  `${BASE_URL}/health`],
    ]);

    for (const res of responses) {
        const isOk = res.status === 200 || res.status === 201;
        check(res, { 'status is 200 or 201': () => isOk });
        if (!isOk) authErrors.add(1);
        p99Latency.add(res.timings.duration);
    }
    sleep(0.05);
}
