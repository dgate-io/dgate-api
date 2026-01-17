#!/usr/bin/env node
/**
 * WebSocket Test Server
 * 
 * Echo server with additional commands:
 * - "ping" -> "pong"
 * - "echo <msg>" -> "<msg>"
 * - "json" -> returns JSON object
 * - "close" -> closes connection
 */

const WebSocket = require('ws');
const http = require('http');

const PORT = process.env.TEST_SERVER_PORT || 19999;

// Create HTTP server for WebSocket upgrade
const server = http.createServer((req, res) => {
    res.writeHead(200, { 'Content-Type': 'text/plain' });
    res.end('WebSocket server. Connect to /echo or /chat');
});

// Create WebSocket server
const wss = new WebSocket.Server({ server });

wss.on('connection', (ws, req) => {
    console.log(`[WS] New connection from ${req.socket.remoteAddress} to ${req.url}`);
    
    ws.on('message', (message) => {
        const msg = message.toString();
        console.log(`[WS] Received: ${msg}`);
        
        // Handle commands
        if (msg === 'ping') {
            ws.send('pong');
        } else if (msg.startsWith('echo ')) {
            ws.send(msg.substring(5));
        } else if (msg === 'json') {
            ws.send(JSON.stringify({
                type: 'json_response',
                timestamp: Date.now(),
                random: Math.random()
            }));
        } else if (msg === 'close') {
            ws.close(1000, 'Goodbye');
        } else if (msg === 'binary') {
            // Send binary data
            const buffer = Buffer.from([0x01, 0x02, 0x03, 0x04, 0x05]);
            ws.send(buffer);
        } else {
            // Default: echo back
            ws.send(`Echo: ${msg}`);
        }
    });
    
    ws.on('close', (code, reason) => {
        console.log(`[WS] Connection closed: ${code} ${reason}`);
    });
    
    ws.on('error', (err) => {
        console.log(`[WS] Error: ${err.message}`);
    });
    
    // Send welcome message
    ws.send(JSON.stringify({
        type: 'welcome',
        message: 'Connected to WebSocket test server',
        path: req.url
    }));
});

server.listen(PORT, () => {
    console.log(`WebSocket server listening on ws://localhost:${PORT}`);
});

// Handle shutdown
process.on('SIGINT', () => {
    console.log('Shutting down...');
    wss.close();
    server.close();
    process.exit(0);
});

process.on('SIGTERM', () => {
    wss.close();
    server.close();
    process.exit(0);
});
