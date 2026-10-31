// load-testing/scenarios/s10-ttl-comparison.js
// Skenario S10: TTL Comparison - ZTA-Cache Static TTL vs Graph-aware @ 2000 RPS Fan-out
// Menggunakan static TTL (30s konstan) untuk dibandingkan dengan S07 (graph-aware TTL).
// Keduanya di 2000 RPS fan-out agar apple-to-apple.
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Counter, Rate } from 'k6/metrics';

const p99Latency = new Trend('p99_latency', true);
const authErrors  = new Counter('auth_errors');
const cacheHitRate = new Rate('cache_hit_rate');

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
    // Fan-out: sama persis dengan S07, tetapi backend menggunakan static TTL
    const responses = http.batch([
        ['GET',  `${BASE_URL}/api/products`],
        ['POST', `${BASE_URL}/api/cart`, JSON.stringify({ item: 'product-1', qty: 1 }), {
            headers: { 'Content-Type': 'application/json' },
        }],
        ['GET',  `${BASE_URL}/health`],
    ]);

    for (const res of responses) {
        const isHit = res.headers['X-Cache-Status'] === 'HIT' || 
                      res.headers['x-cache-status'] === 'HIT' || 
                      res.headers['x-zta-cache-status'] === 'HIT';
        cacheHitRate.add(isHit);
        const isOk = res.status === 200 || res.status === 201;
        check(res, { 'status is 200 or 201': () => isOk });
        if (!isOk) authErrors.add(1);
        p99Latency.add(res.timings.duration);
    }
    sleep(0.05);
}
