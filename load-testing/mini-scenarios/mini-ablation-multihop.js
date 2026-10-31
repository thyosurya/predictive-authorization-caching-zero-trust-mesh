// load-testing/mini-scenarios/mini-ablation-multihop.js
// Skenario Mini Ablasi Multi-Hop: Verifikasi Cepat Sintaks & Routing Pipeline
import http from 'k6/http';
import { check } from 'k6';
import { Trend, Counter } from 'k6/metrics';

const p99Latency       = new Trend('p99_latency', true);
const multihopLatency  = new Trend('multihop_latency', true);
const directLatency    = new Trend('direct_latency', true);
const authErrors       = new Counter('auth_errors');

export const options = {
    scenarios: {
        mini_ablation: {
            executor: 'constant-arrival-rate',
            rate: 50,
            timeUnit: '1s',
            duration: '15s',
            preAllocatedVUs: 10,
            maxVUs: 20,
        },
    },
    thresholds: {
        http_req_duration: ['p(99)<500'],
        http_req_failed:   ['rate<0.05'],
    },
    summaryTrendStats: ["min", "avg", "med", "p(95)", "p(99)", "max"],
};

const BASE_URL = __ENV.BASE_URL || 'http://frontend.zta-research.svc.cluster.local';

export default function () {
    const isMultiHop = (__ITER % 10) < 4;

    if (isMultiHop) {
        const payload = JSON.stringify({ item: 'product-1', qty: 1 });
        const res = http.post(`${BASE_URL}/api/cart`, payload, {
            headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
        });
        const ok = res.status === 200 || res.status === 201;
        check(res, { 'status 200/201': () => ok });
        if (!ok) authErrors.add(1);
        multihopLatency.add(res.timings.duration);
        p99Latency.add(res.timings.duration);
    } else {
        const res = http.get(`${BASE_URL}/api/products`, {
            headers: { 'Accept': 'application/json' },
        });
        const ok = res.status === 200;
        check(res, { 'status 200': () => ok });
        if (!ok) authErrors.add(1);
        directLatency.add(res.timings.duration);
        p99Latency.add(res.timings.duration);
    }
}
