// load-testing/warmup.js
import http from "k6/http";
import { sleep } from "k6";

export const options = {
    vus: 10,
    duration: "30s",
    thresholds: {},  // Tidak ada threshold saat warmup
};

const BASE_URL = __ENV.BASE_URL || 'http://frontend.zta-research.svc.cluster.local';

export default function () {
    http.get(`${BASE_URL}/api/products`);
    sleep(0.5);
}
