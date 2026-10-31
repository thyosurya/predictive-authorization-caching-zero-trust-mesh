// load-testing/scenarios/s-ablation-multihop-contention.js
// Skenario Ablasi Multi-Hop: Graph-Aware vs Frequency-Only (LFU) pada Kapasitas Terbatas (K=2)
// Mengukur latensi end-to-end, throughput, dan error rate pada persaingan edge direct vs multi-hop East-West.
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter } from 'k6/metrics';

const p99Latency       = new Trend('p99_latency', true);
const multihopLatency  = new Trend('multihop_latency', true);
const directLatency    = new Trend('direct_latency', true);
const authErrors       = new Counter('auth_errors');
const multihopReqs     = new Counter('multihop_reqs');
const directReqs       = new Counter('direct_reqs');

export const options = {
    scenarios: {
        ablation_load: {
            executor: 'constant-arrival-rate',
            rate: 500,
            timeUnit: '1s',
            duration: '3m',
            preAllocatedVUs: 50,
            maxVUs: 150,
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
    // Distribusi probabilitas: 40% multi-hop (/api/cart) dan 60% direct (/api/products)
    // __ITER % 10 < 4 menghasilkan rasio 4:6 yang deterministik per Virtual User
    const isMultiHop = (__ITER % 10) < 4;

    if (isMultiHop) {
        // Multi-Hop Path: frontend -> cartservice -> productcatalogservice (East-West)
        const payload = JSON.stringify({ item: 'product-1', qty: 1 });
        const params = {
            headers: {
                'Content-Type': 'application/json',
                'Accept': 'application/json',
            },
        };
        const res = http.post(`${BASE_URL}/api/cart`, payload, params);
        const ok = res.status === 200 || res.status === 201;
        check(res, {
            'multihop cart status 200/201': () => ok,
        });
        if (!ok) {
            authErrors.add(1);
        }
        multihopReqs.add(1);
        multihopLatency.add(res.timings.duration);
        p99Latency.add(res.timings.duration);
    } else {
        // Direct Path: frontend -> productcatalogservice
        const res = http.get(`${BASE_URL}/api/products`, {
            headers: {
                'Accept': 'application/json',
            },
        });
        const ok = res.status === 200;
        check(res, {
            'direct products status 200': () => ok,
        });
        if (!ok) {
            authErrors.add(1);
        }
        directReqs.add(1);
        directLatency.add(res.timings.duration);
        p99Latency.add(res.timings.duration);
    }
}
