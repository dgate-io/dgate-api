#!/usr/bin/env node
/**
 * WebSocket Test Client
 * 
 * Usage: node client.js <command> [args...]
 * 
 * Commands:
 *   connect <url>           - Connect and print welcome message
 *   ping <url>              - Send ping, expect pong
 *   echo <url> <message>    - Send message, get echo
 *   roundtrip <url> <count> - Measure roundtrip latency
 *   binary <url>            - Test binary message
 */

const WebSocket = require('ws');

const command = process.argv[2];
const url = process.argv[3] || 'ws://localhost:8080/ws/echo';

function connect(url) {
    return new Promise((resolve, reject) => {
        const ws = new WebSocket(url);
        ws.on('open', () => resolve(ws));
        ws.on('error', reject);
        setTimeout(() => reject(new Error('Connection timeout')), 5000);
    });
}

async function testConnect() {
    const ws = await connect(url);
    
    return new Promise((resolve) => {
        ws.on('message', (data) => {
            console.log('Received:', data.toString());
            ws.close();
            resolve(true);
        });
    });
}

async function testPing() {
    const ws = await connect(url);
    
    // Wait for welcome message
    await new Promise(r => ws.once('message', r));
    
    return new Promise((resolve) => {
        ws.send('ping');
        ws.on('message', (data) => {
            const msg = data.toString();
            if (msg === 'pong') {
                console.log('PASS: ping -> pong');
                ws.close();
                resolve(true);
            }
        });
    });
}

async function testEcho() {
    const message = process.argv[4] || 'Hello, World!';
    const ws = await connect(url);
    
    // Wait for welcome message
    await new Promise(r => ws.once('message', r));
    
    return new Promise((resolve) => {
        ws.send(`echo ${message}`);
        ws.on('message', (data) => {
            const msg = data.toString();
            if (msg === message) {
                console.log(`PASS: echo "${message}"`);
                ws.close();
                resolve(true);
            }
        });
    });
}

async function testRoundtrip() {
    const count = parseInt(process.argv[4]) || 100;
    const ws = await connect(url);
    
    // Wait for welcome message
    await new Promise(r => ws.once('message', r));
    
    const latencies = [];
    
    for (let i = 0; i < count; i++) {
        const start = Date.now();
        ws.send('ping');
        await new Promise(resolve => {
            ws.once('message', () => {
                latencies.push(Date.now() - start);
                resolve();
            });
        });
    }
    
    ws.close();
    
    latencies.sort((a, b) => a - b);
    const avg = latencies.reduce((a, b) => a + b, 0) / latencies.length;
    const p50 = latencies[Math.floor(latencies.length * 0.5)];
    const p95 = latencies[Math.floor(latencies.length * 0.95)];
    const p99 = latencies[Math.floor(latencies.length * 0.99)];
    
    console.log(`Roundtrip Latency (${count} messages):`);
    console.log(`  Average: ${avg.toFixed(2)}ms`);
    console.log(`  p50: ${p50}ms`);
    console.log(`  p95: ${p95}ms`);
    console.log(`  p99: ${p99}ms`);
    
    return true;
}

async function testBinary() {
    const ws = await connect(url);
    
    // Wait for welcome message
    await new Promise(r => ws.once('message', r));
    
    return new Promise((resolve) => {
        ws.send('binary');
        ws.on('message', (data) => {
            if (Buffer.isBuffer(data) && data.length === 5) {
                console.log('PASS: Received binary data:', Array.from(data));
                ws.close();
                resolve(true);
            }
        });
    });
}

async function main() {
    try {
        switch (command) {
            case 'connect':
                await testConnect();
                break;
            case 'ping':
                await testPing();
                break;
            case 'echo':
                await testEcho();
                break;
            case 'roundtrip':
                await testRoundtrip();
                break;
            case 'binary':
                await testBinary();
                break;
            default:
                console.log('Usage: node client.js <command> [url] [args...]');
                console.log('Commands: connect, ping, echo, roundtrip, binary');
                process.exit(1);
        }
        process.exit(0);
    } catch (err) {
        console.error('FAIL:', err.message);
        process.exit(1);
    }
}

main();
