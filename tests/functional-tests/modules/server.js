/**
 * Echo Server for Module Testing
 * 
 * Simple HTTP server that echoes back request details.
 * Used to test request/response modifiers.
 */

const http = require('http');

const PORT = process.env.TEST_SERVER_PORT || 19999;

const server = http.createServer((req, res) => {
    let body = [];
    
    req.on('data', chunk => {
        body.push(chunk);
    });
    
    req.on('end', () => {
        body = Buffer.concat(body).toString();
        
        const response = {
            method: req.method,
            url: req.url,
            headers: req.headers,
            body: body || null
        };
        
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify(response, null, 2));
    });
});

server.listen(PORT, () => {
    console.log(`Echo server listening on port ${PORT}`);
});
