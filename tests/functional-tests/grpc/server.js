#!/usr/bin/env node
/**
 * gRPC Test Server using @grpc/grpc-js
 * 
 * Implements a simple Greeter service.
 */

const grpc = require('@grpc/grpc-js');
const protoLoader = require('@grpc/proto-loader');
const path = require('path');

const PORT = process.env.TEST_SERVER_PORT || 50051;
const PROTO_PATH = path.join(__dirname, 'greeter.proto');

// Load proto file
const packageDefinition = protoLoader.loadSync(PROTO_PATH, {
    keepCase: true,
    longs: String,
    enums: String,
    defaults: true,
    oneofs: true
});

const greeterProto = grpc.loadPackageDefinition(packageDefinition).helloworld;

// Implement service methods
function sayHello(call, callback) {
    console.log(`[gRPC] SayHello: ${call.request.name}`);
    callback(null, { message: `Hello ${call.request.name}` });
}

function sayHelloAgain(call, callback) {
    console.log(`[gRPC] SayHelloAgain: ${call.request.name}`);
    callback(null, { message: `Hello again ${call.request.name}` });
}

function sayHelloServerStream(call) {
    const name = call.request.name;
    console.log(`[gRPC] SayHelloServerStream: ${name}`);
    
    for (let i = 1; i <= 5; i++) {
        call.write({ message: `Hello ${name} #${i}` });
    }
    call.end();
}

function sayHelloClientStream(call, callback) {
    const names = [];
    
    call.on('data', (request) => {
        console.log(`[gRPC] SayHelloClientStream received: ${request.name}`);
        names.push(request.name);
    });
    
    call.on('end', () => {
        callback(null, { message: `Hello ${names.join(', ')}` });
    });
}

function sayHelloBidiStream(call) {
    call.on('data', (request) => {
        console.log(`[gRPC] SayHelloBidiStream: ${request.name}`);
        call.write({ message: `Hello ${request.name}` });
    });
    
    call.on('end', () => {
        call.end();
    });
}

// Start server
function main() {
    const server = new grpc.Server();
    
    server.addService(greeterProto.Greeter.service, {
        SayHello: sayHello,
        SayHelloAgain: sayHelloAgain,
        SayHelloServerStream: sayHelloServerStream,
        SayHelloClientStream: sayHelloClientStream,
        SayHelloBidiStream: sayHelloBidiStream
    });
    
    server.bindAsync(
        `0.0.0.0:${PORT}`,
        grpc.ServerCredentials.createInsecure(),
        (err, port) => {
            if (err) {
                console.error('Failed to start server:', err);
                process.exit(1);
            }
            console.log(`gRPC server listening on port ${port}`);
        }
    );
}

main();
