/**
 * k6 Performance Test: Admin API
 * 
 * Tests the Admin API performance for various operations:
 * - List resources (GET)
 * - Create/Update resources (PUT)
 * - Delete resources (DELETE)
 * 
 * Run: k6 run test-admin.js
 * 
 * Test specific operation:
 *   k6 run -e OP=read test-admin.js
 *   k6 run -e OP=write test-admin.js
 *   k6 run -e OP=mixed test-admin.js
 */

import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

// Custom metrics
const readLatency = new Trend('admin_read_latency');
const writeLatency = new Trend('admin_write_latency');
const deleteLatency = new Trend('admin_delete_latency');
const successRate = new Rate('success_rate');

// Configuration
const ADMIN_URL = __ENV.ADMIN_URL || 'http://localhost:9080';
const OP = __ENV.OP || 'mixed'; // 'read', 'write', 'mixed'

export const options = {
    scenarios: {
        // Read-heavy scenario (typical usage)
        read_heavy: {
            executor: 'ramping-vus',
            startVUs: 0,
            stages: [
                { duration: '15s', target: 20 },
                { duration: '30s', target: 20 },
                { duration: '15s', target: 50 },
                { duration: '30s', target: 50 },
                { duration: '15s', target: 0 },
            ],
            startTime: '0s',
            tags: { scenario: 'read_heavy' },
        },
        // Write scenario (stress storage)
        write_stress: {
            executor: 'ramping-vus',
            startVUs: 0,
            stages: [
                { duration: '15s', target: 10 },
                { duration: '30s', target: 10 },
                { duration: '15s', target: 30 },
                { duration: '30s', target: 30 },
                { duration: '15s', target: 0 },
            ],
            startTime: '2m',
            tags: { scenario: 'write_stress' },
        },
    },
    thresholds: {
        http_req_duration: ['p(95)<100', 'p(99)<200'],
        success_rate: ['rate>0.95'], // Admin ops can have contention
        admin_read_latency: ['p(95)<50'],
        admin_write_latency: ['p(95)<100'],
    },
};

// Quick test options
export const quickOptions = {
    vus: 20,
    duration: '30s',
    thresholds: {
        http_req_duration: ['p(95)<100'],
        success_rate: ['rate>0.95'],
    },
};

function testRead() {
    group('Read Operations', function () {
        // List namespaces
        let res = http.get(`${ADMIN_URL}/api/v1/namespace`, {
            headers: { 'Accept': 'application/json' },
        });
        readLatency.add(res.timings.duration);
        successRate.add(check(res, {
            'list namespaces: 200': (r) => r.status === 200,
        }));

        // List routes
        res = http.get(`${ADMIN_URL}/api/v1/route`, {
            headers: { 'Accept': 'application/json' },
        });
        readLatency.add(res.timings.duration);
        successRate.add(check(res, {
            'list routes: 200': (r) => r.status === 200,
        }));

        // List services
        res = http.get(`${ADMIN_URL}/api/v1/service`, {
            headers: { 'Accept': 'application/json' },
        });
        readLatency.add(res.timings.duration);
        successRate.add(check(res, {
            'list services: 200': (r) => r.status === 200,
        }));

        // List modules
        res = http.get(`${ADMIN_URL}/api/v1/module`, {
            headers: { 'Accept': 'application/json' },
        });
        readLatency.add(res.timings.duration);
        successRate.add(check(res, {
            'list modules: 200': (r) => r.status === 200,
        }));
    });
}

function testWrite() {
    const vuId = __VU;
    const iter = __ITER;
    const resourceId = `perf-test-${vuId}-${iter}`;
    
    group('Write Operations', function () {
        // Create a namespace
        let res = http.put(`${ADMIN_URL}/api/v1/namespace`, 
            JSON.stringify({
                name: resourceId,
                tags: ['perf-test'],
            }), {
                headers: { 
                    'Content-Type': 'application/json',
                    'Accept': 'application/json',
                },
            }
        );
        writeLatency.add(res.timings.duration);
        const createSuccess = check(res, {
            'create namespace: 200/201': (r) => r.status === 200 || r.status === 201,
        });
        successRate.add(createSuccess);

        // Create a collection
        res = http.put(`${ADMIN_URL}/api/v1/collection`, 
            JSON.stringify({
                name: `coll-${resourceId}`,
                namespace: resourceId,
                visibility: 'private',
            }), {
                headers: { 
                    'Content-Type': 'application/json',
                    'Accept': 'application/json',
                },
            }
        );
        writeLatency.add(res.timings.duration);
        successRate.add(check(res, {
            'create collection: 200/201': (r) => r.status === 200 || r.status === 201,
        }));

        // Create a document
        res = http.put(`${ADMIN_URL}/api/v1/collection/${resourceId}/coll-${resourceId}/doc-${iter}`, 
            JSON.stringify({
                test: 'data',
                iteration: iter,
                timestamp: Date.now(),
            }), {
                headers: { 
                    'Content-Type': 'application/json',
                    'Accept': 'application/json',
                },
            }
        );
        writeLatency.add(res.timings.duration);
        successRate.add(check(res, {
            'create document: 200/201': (r) => r.status === 200 || r.status === 201,
        }));

        // Cleanup - delete the namespace (cascades)
        res = http.del(`${ADMIN_URL}/api/v1/namespace/${resourceId}`, null, {
            headers: { 'Accept': 'application/json' },
        });
        deleteLatency.add(res.timings.duration);
        successRate.add(check(res, {
            'delete namespace: 200/204': (r) => r.status === 200 || r.status === 204,
        }));
    });
}

function testMixed() {
    // 80% reads, 20% writes (typical production ratio)
    if (__ITER % 5 === 0) {
        testWrite();
    } else {
        testRead();
    }
}

export default function () {
    switch (OP) {
        case 'read':
            testRead();
            break;
        case 'write':
            testWrite();
            break;
        case 'mixed':
        default:
            testMixed();
    }
}

export function handleSummary(data) {
    const summary = {
        operation: OP,
        metrics: {
            requests_per_sec: data.metrics.http_reqs?.values.rate || 0,
            duration_avg: data.metrics.http_req_duration?.values.avg || 0,
            duration_p95: data.metrics.http_req_duration?.values['p(95)'] || 0,
            duration_p99: data.metrics.http_req_duration?.values['p(99)'] || 0,
            success_rate: data.metrics.success_rate?.values.rate || 0,
            latencies: {
                read_p95: data.metrics.admin_read_latency?.values['p(95)'] || 'N/A',
                write_p95: data.metrics.admin_write_latency?.values['p(95)'] || 'N/A',
                delete_p95: data.metrics.admin_delete_latency?.values['p(95)'] || 'N/A',
            },
        },
    };
    
    return {
        stdout: JSON.stringify(summary, null, 2) + '\n',
        'admin-results.json': JSON.stringify(data, null, 2),
    };
}
