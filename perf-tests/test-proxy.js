/**
 * k6 Performance Test: Proxy Server
 * 
 * Tests the proxy functionality with echo server upstream.
 * This measures the overhead DGate adds to proxied requests.
 * 
 * Run: k6 run test-proxy.js
 * 
 * Comparison tests:
 *   Direct echo server:  k6 run -e TARGET=direct test-proxy.js
 *   Via DGate proxy:     k6 run -e TARGET=proxy test-proxy.js
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

// Custom metrics
const requestDuration = new Trend('request_duration');
const requestRate = new Counter('requests');
const successRate = new Rate('success_rate');

// Configuration
const DGATE_URL = __ENV.DGATE_URL || 'http://localhost:8080';
const ECHO_URL = __ENV.ECHO_URL || 'http://localhost:9999';
const TARGET = __ENV.TARGET || 'proxy'; // 'proxy' or 'direct'

const baseUrl = TARGET === 'direct' ? ECHO_URL : `${DGATE_URL}/proxy`;

export const options = {
    scenarios: {
        // Smoke test
        smoke: {
            executor: 'constant-vus',
            vus: 1,
            duration: '10s',
            startTime: '0s',
            tags: { test_type: 'smoke' },
        },
        // Load test - ramp up
        load: {
            executor: 'ramping-vus',
            startVUs: 0,
            stages: [
                { duration: '30s', target: 50 },
                { duration: '1m', target: 50 },
                { duration: '30s', target: 100 },
                { duration: '1m', target: 100 },
                { duration: '30s', target: 0 },
            ],
            startTime: '15s',
            tags: { test_type: 'load' },
        },
        // Stress test
        stress: {
            executor: 'ramping-vus',
            startVUs: 0,
            stages: [
                { duration: '30s', target: 200 },
                { duration: '1m', target: 200 },
                { duration: '30s', target: 500 },
                { duration: '1m', target: 500 },
                { duration: '30s', target: 0 },
            ],
            startTime: '5m',
            tags: { test_type: 'stress' },
        },
    },
    thresholds: {
        http_req_duration: ['p(95)<100', 'p(99)<200'], // 95% under 100ms, 99% under 200ms
        success_rate: ['rate>0.99'], // 99% success rate
    },
};

// Quick test options (uncomment to use)
export const quickOptions = {
    vus: 50,
    duration: '30s',
    thresholds: {
        http_req_duration: ['p(95)<100'],
        success_rate: ['rate>0.99'],
    },
};

export default function () {
    const url = `${baseUrl}/echo`;
    
    const res = http.get(url, {
        headers: {
            'Accept': 'application/json',
        },
    });

    requestDuration.add(res.timings.duration);
    requestRate.add(1);
    
    const passed = check(res, {
        'status is 200': (r) => r.status === 200,
        'response has body': (r) => r.body.length > 0,
        'response is json': (r) => {
            try {
                JSON.parse(r.body);
                return true;
            } catch {
                return false;
            }
        },
    });
    
    successRate.add(passed);
}

export function handleSummary(data) {
    const summary = {
        target: TARGET,
        baseUrl: baseUrl,
        metrics: {
            requests_total: data.metrics.requests.values.count,
            requests_per_sec: data.metrics.http_reqs.values.rate,
            duration_avg: data.metrics.http_req_duration.values.avg,
            duration_p50: data.metrics.http_req_duration.values['p(50)'],
            duration_p95: data.metrics.http_req_duration.values['p(95)'],
            duration_p99: data.metrics.http_req_duration.values['p(99)'],
            success_rate: data.metrics.success_rate.values.rate,
        },
    };
    
    return {
        stdout: JSON.stringify(summary, null, 2) + '\n',
        'proxy-results.json': JSON.stringify(data, null, 2),
    };
}
