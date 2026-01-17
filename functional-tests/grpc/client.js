#!/usr/bin/env node
/**
 * gRPC Test Client
 * 
 * Usage: node client.js <command> [target] [args...]
 * 
 * Commands:
 *   unary <target> <name>          - Test unary RPC
 *   server-stream <target> <name>  - Test server streaming
 *   client-stream <target> <names> - Test client streaming
 *   bidi-stream <target> <names>   - Test bidirectional streaming
 */

const grpc = require('@grpc/grpc-js');
const protoLoader = require('@grpc/proto-loader');
const path = require('path');

const PROTO_PATH = path.join(__dirname, 'greeter.proto');

// Load proto
const packageDefinition = protoLoader.loadSync(PROTO_PATH, {
    keepCase: true,
    longs: String,
    enums: String,
    defaults: true,
    oneofs: true
});
const greeterProto = grpc.loadPackageDefinition(packageDefinition).helloworld;

const command = process.argv[2];
const target = process.argv[3] || 'localhost:50051';

function createClient(target) {
    return new greeterProto.Greeter(target, grpc.credentials.createInsecure());
}

async function testUnary() {
    const name = process.argv[4] || 'World';
    const client = createClient(target);
    
    return new Promise((resolve, reject) => {
        client.sayHello({ name }, (err, response) => {
            if (err) {
                console.log('FAIL:', err.message);
                reject(err);
            } else {
                console.log('Response:', response.message);
                if (response.message === `Hello ${name}`) {
                    console.log('PASS: Unary RPC works');
                    resolve(true);
                } else {
                    console.log('FAIL: Unexpected response');
                    resolve(false);
                }
            }
        });
    });
}

async function testServerStream() {
    const name = process.argv[4] || 'World';
    const client = createClient(target);
    
    return new Promise((resolve) => {
        const messages = [];
        const call = client.sayHelloServerStream({ name });
        
        call.on('data', (response) => {
            console.log('Stream:', response.message);
            messages.push(response.message);
        });
        
        call.on('end', () => {
            if (messages.length === 5) {
                console.log('PASS: Server streaming works');
                resolve(true);
            } else {
                console.log(`FAIL: Expected 5 messages, got ${messages.length}`);
                resolve(false);
            }
        });
        
        call.on('error', (err) => {
            console.log('FAIL:', err.message);
            resolve(false);
        });
    });
}

async function testClientStream() {
    const names = (process.argv[4] || 'Alice,Bob,Charlie').split(',');
    const client = createClient(target);
    
    return new Promise((resolve) => {
        const call = client.sayHelloClientStream((err, response) => {
            if (err) {
                console.log('FAIL:', err.message);
                resolve(false);
            } else {
                console.log('Response:', response.message);
                if (response.message.includes(names[0])) {
                    console.log('PASS: Client streaming works');
                    resolve(true);
                } else {
                    console.log('FAIL: Unexpected response');
                    resolve(false);
                }
            }
        });
        
        for (const name of names) {
            call.write({ name });
        }
        call.end();
    });
}

async function testBidiStream() {
    const names = (process.argv[4] || 'Alice,Bob,Charlie').split(',');
    const client = createClient(target);
    
    return new Promise((resolve) => {
        const messages = [];
        const call = client.sayHelloBidiStream();
        
        call.on('data', (response) => {
            console.log('Stream:', response.message);
            messages.push(response.message);
        });
        
        call.on('end', () => {
            if (messages.length === names.length) {
                console.log('PASS: Bidirectional streaming works');
                resolve(true);
            } else {
                console.log(`FAIL: Expected ${names.length} messages, got ${messages.length}`);
                resolve(false);
            }
        });
        
        call.on('error', (err) => {
            console.log('FAIL:', err.message);
            resolve(false);
        });
        
        for (const name of names) {
            call.write({ name });
        }
        call.end();
    });
}

async function main() {
    try {
        let success;
        switch (command) {
            case 'unary':
                success = await testUnary();
                break;
            case 'server-stream':
                success = await testServerStream();
                break;
            case 'client-stream':
                success = await testClientStream();
                break;
            case 'bidi-stream':
                success = await testBidiStream();
                break;
            default:
                console.log('Usage: node client.js <command> [target] [args...]');
                console.log('Commands: unary, server-stream, client-stream, bidi-stream');
                process.exit(1);
        }
        process.exit(success ? 0 : 1);
    } catch (err) {
        console.error('FAIL:', err.message);
        process.exit(1);
    }
}

main();
