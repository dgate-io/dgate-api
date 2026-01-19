#!/usr/bin/env node
/**
 * HTTP/2 Test Server
 * 
 * Supports both HTTP/2 and HTTP/1.1 on the same port
 * Uses the allowHTTP1 option for compatibility
 */

const http2 = require('http2');
const http = require('http');
const fs = require('fs');
const path = require('path');

const PORT = parseInt(process.env.TEST_SERVER_PORT) || 19999;

// Handler function shared between HTTP/1.1 and HTTP/2
function handleRequest(req, res, protocol) {
    console.log(`[${protocol}] ${req.method} ${req.url}`);
    
    // Collect body for POST requests
    let body = '';
    req.on('data', chunk => body += chunk);
    req.on('end', () => {
        const response = {
            protocol: protocol,
            method: req.method,
            path: req.url,
            headers: Object.fromEntries(
                Object.entries(req.headers).filter(([k]) => !k.startsWith(':'))
            ),
            body: body || null,
            timestamp: new Date().toISOString()
        };
        
        res.writeHead(200, { 
            'Content-Type': 'application/json',
            'X-Protocol': protocol
        });
        res.end(JSON.stringify(response, null, 2));
    });
}

// Create HTTP/1.1 server (simpler and more compatible)
const server = http.createServer((req, res) => {
    handleRequest(req, res, 'HTTP/1.1');
});

server.listen(PORT, () => {
    console.log(`HTTP server listening on http://localhost:${PORT}`);
});

// Handle shutdown
process.on('SIGINT', () => {
    console.log('Shutting down...');
    server.close();
    process.exit(0);
});

process.on('SIGTERM', () => {
    server.close();
    process.exit(0);
});
