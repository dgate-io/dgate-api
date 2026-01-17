/**
 * k6 Performance Test: JavaScript Modules
 * 
 * Tests the JS module execution performance.
 * Compares simple, echo, and compute handlers.
 * 
 * Run: k6 run test-modules.js
 * 
 * Test specific handler:
 *   k6 run -e HANDLER=simple test-modules.js
 *   k6 run -e HANDLER=echo test-modules.js
 *   k6 run -e HANDLER=compute test-modules.js
 */

import http from 'k6/http';
import { check, group } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

// Custom metrics per handler type
const simpleLatency = new Trend('simple_handler_latency');
const echoLatency = new Trend('echo_handler_latency');
const computeLatency = new Trend('compute_handler_latency');
const successRate = new Rate('success_rate');

// Configuration
const DGATE_URL = __ENV.DGATE_URL || 'http://localhost:8080';
const HANDLER = __ENV.HANDLER || 'all'; // 'simple', 'echo', 'compute', or 'all'

export const options = {
    scenarios: {
        // Warm-up
        warmup: {
            executor: 'constant-vus',
            vus: 5,
            duration: '10s',
            startTime: '0s',
            tags: { phase: 'warmup' },
        },
        // Main load test
        load: {
            executor: 'ramping-vus',
            startVUs: 0,
            stages: [
                { duration: '20s', target: 50 },
                { duration: '1m', target: 50 },
                { duration: '20s', target: 100 },
                { duration: '1m', target: 100 },
                { duration: '20s', target: 0 },
            ],
            startTime: '15s',
            tags: { phase: 'load' },
        },
        // Peak load
        peak: {
            executor: 'constant-vus',
            vus: 200,
            duration: '1m',
            startTime: '4m',
            tags: { phase: 'peak' },
        },
    },
    thresholds: {
        http_req_duration: ['p(95)<50', 'p(99)<100'], // Modules should be fast
        success_rate: ['rate>0.99'],
        simple_handler_latency: ['p(95)<30'],
        echo_handler_latency: ['p(95)<30'],
        compute_handler_latency: ['p(95)<50'],
    },
};

// Quick test options
export const quickOptions = {
    vus: 50,
    duration: '30s',
    thresholds: {
        http_req_duration: ['p(95)<50'],
        success_rate: ['rate>0.99'],
    },
};

function testSimpleHandler() {
    const res = http.get(`${DGATE_URL}/module/simple`, {
        headers: { 'Accept': 'application/json' },
    });
    
    simpleLatency.add(res.timings.duration);
    
    const passed = check(res, {
        'simple: status 200': (r) => r.status === 200,
        'simple: has message': (r) => {
            try {
                const body = JSON.parse(r.body);
                return body.message === 'ok';
            } catch {
                return false;
            }
        },
    });
    
    successRate.add(passed);
}

function testEchoHandler() {
    const res = http.get(`${DGATE_URL}/module/echo?test=perf`, {
        headers: { 'Accept': 'application/json' },
    });
    
    echoLatency.add(res.timings.duration);
    
    const passed = check(res, {
        'echo: status 200': (r) => r.status === 200,
        'echo: has method': (r) => {
            try {
                const body = JSON.parse(r.body);
                return body.method === 'GET';
            } catch {
                return false;
            }
        },
    });
    
    successRate.add(passed);
}

function testComputeHandler() {
    const res = http.get(`${DGATE_URL}/module/compute`, {
        headers: { 'Accept': 'application/json' },
    });
    
    computeLatency.add(res.timings.duration);
    
    const passed = check(res, {
        'compute: status 200': (r) => r.status === 200,
        'compute: has computed value': (r) => {
            try {
                const body = JSON.parse(r.body);
                return typeof body.computed === 'number';
            } catch {
                return false;
            }
        },
    });
    
    successRate.add(passed);
}

export default function () {
    switch (HANDLER) {
        case 'simple':
            testSimpleHandler();
            break;
        case 'echo':
            testEchoHandler();
            break;
        case 'compute':
            testComputeHandler();
            break;
        case 'all':
        default:
            // Round-robin all handlers
            const iteration = __ITER % 3;
            if (iteration === 0) testSimpleHandler();
            else if (iteration === 1) testEchoHandler();
            else testComputeHandler();
    }
}

export function handleSummary(data) {
    const summary = {
        handler: HANDLER,
        metrics: {
            requests_per_sec: data.metrics.http_reqs?.values.rate || 0,
            duration_avg: data.metrics.http_req_duration?.values.avg || 0,
            duration_p95: data.metrics.http_req_duration?.values['p(95)'] || 0,
            duration_p99: data.metrics.http_req_duration?.values['p(99)'] || 0,
            success_rate: data.metrics.success_rate?.values.rate || 0,
            handlers: {
                simple_p95: data.metrics.simple_handler_latency?.values['p(95)'] || 'N/A',
                echo_p95: data.metrics.echo_handler_latency?.values['p(95)'] || 'N/A',
                compute_p95: data.metrics.compute_handler_latency?.values['p(95)'] || 'N/A',
            },
        },
    };
    
    return {
        stdout: JSON.stringify(summary, null, 2) + '\n',
        'modules-results.json': JSON.stringify(data, null, 2),
    };
}
