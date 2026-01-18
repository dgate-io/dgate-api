/**
 * k6 Quick Performance Test - All Components
 * 
 * A quick 30-second test of all components to validate performance.
 * 
 * Run: k6 run quick-test.js
 */

import http from 'k6/http';
import { check } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// Metrics
const proxyLatency = new Trend('proxy_latency');
const moduleLatency = new Trend('module_latency');
const adminLatency = new Trend('admin_latency');
const successRate = new Rate('success_rate');

// Configuration
const DGATE_URL = __ENV.DGATE_URL || 'http://localhost:8080';
const ADMIN_URL = __ENV.ADMIN_URL || 'http://localhost:9080';
const ECHO_URL = __ENV.ECHO_URL || 'http://localhost:9999';

export const options = {
    vus: 50,
    duration: '30s',
    thresholds: {
        http_req_duration: ['p(95)<100'],
        success_rate: ['rate>0.99'],
        proxy_latency: ['p(95)<100'],
        module_latency: ['p(95)<50'],
        admin_latency: ['p(95)<50'],
    },
};

export default function () {
    const component = __ITER % 3;
    
    if (component === 0) {
        // Test Proxy
        const res = http.get(`${DGATE_URL}/proxy/test`);
        proxyLatency.add(res.timings.duration);
        successRate.add(check(res, {
            'proxy: 200': (r) => r.status === 200,
        }));
    } else if (component === 1) {
        // Test JS Module
        const res = http.get(`${DGATE_URL}/module/simple`);
        moduleLatency.add(res.timings.duration);
        successRate.add(check(res, {
            'module: 200': (r) => r.status === 200,
        }));
    } else {
        // Test Admin API
        const res = http.get(`${ADMIN_URL}/api/v1/namespace`);
        adminLatency.add(res.timings.duration);
        successRate.add(check(res, {
            'admin: 200': (r) => r.status === 200,
        }));
    }
}

export function handleSummary(data) {
    console.log('\n========== QUICK TEST RESULTS ==========\n');
    
    const summary = {
        total_requests: data.metrics.http_reqs?.values.count || 0,
        requests_per_sec: data.metrics.http_reqs?.values.rate?.toFixed(2) || 0,
        overall: {
            avg: data.metrics.http_req_duration?.values.avg?.toFixed(2) + 'ms',
            p95: data.metrics.http_req_duration?.values['p(95)']?.toFixed(2) + 'ms',
            p99: data.metrics.http_req_duration?.values['p(99)']?.toFixed(2) + 'ms',
        },
        components: {
            proxy_p95: data.metrics.proxy_latency?.values['p(95)']?.toFixed(2) + 'ms' || 'N/A',
            module_p95: data.metrics.module_latency?.values['p(95)']?.toFixed(2) + 'ms' || 'N/A',
            admin_p95: data.metrics.admin_latency?.values['p(95)']?.toFixed(2) + 'ms' || 'N/A',
        },
        success_rate: ((data.metrics.success_rate?.values.rate || 0) * 100).toFixed(2) + '%',
    };
    
    return {
        stdout: JSON.stringify(summary, null, 2) + '\n',
    };
}
