/**
 * Modify Response Module for DGate v2
 * 
 * This module demonstrates response modification after receiving from upstream.
 * Common use cases: adding security headers, response transformation, 
 * caching headers, CORS, and response enrichment.
 * 
 * The responseModifier function is called after receiving the upstream response
 * but before sending to the client.
 */

/**
 * Modifies the response before it's sent to the client.
 * @param {object} ctx - The request context  
 * @param {object} res - The response context with status, headers, body
 */
function responseModifier(ctx, res) {
    var req = ctx.request;
    
    // =========================================
    // 1. Security Headers
    // =========================================
    
    // Content Security Policy
    res.headers["Content-Security-Policy"] = 
        "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'";
    
    // Prevent XSS attacks
    res.headers["X-Content-Type-Options"] = "nosniff";
    res.headers["X-Frame-Options"] = "DENY";
    res.headers["X-XSS-Protection"] = "1; mode=block";
    
    // HSTS for HTTPS connections
    res.headers["Strict-Transport-Security"] = "max-age=31536000; includeSubDomains";
    
    // Referrer Policy
    res.headers["Referrer-Policy"] = "strict-origin-when-cross-origin";
    
    // Permissions Policy (formerly Feature-Policy)
    res.headers["Permissions-Policy"] = "geolocation=(), microphone=(), camera=()";
    
    // =========================================
    // 2. CORS Headers
    // =========================================
    
    var origin = req.headers["Origin"] || req.headers["origin"];
    var allowedOrigins = getAllowedOrigins(ctx);
    
    if (origin && isOriginAllowed(origin, allowedOrigins)) {
        res.headers["Access-Control-Allow-Origin"] = origin;
        res.headers["Access-Control-Allow-Methods"] = "GET, POST, PUT, DELETE, PATCH, OPTIONS";
        res.headers["Access-Control-Allow-Headers"] = 
            "Content-Type, Authorization, X-Request-Id, X-API-Key";
        res.headers["Access-Control-Allow-Credentials"] = "true";
        res.headers["Access-Control-Max-Age"] = "86400";
        res.headers["Access-Control-Expose-Headers"] = 
            "X-Request-Id, X-Response-Time, X-RateLimit-Remaining";
    }
    
    // =========================================
    // 3. Request Timing & Tracing
    // =========================================
    
    var requestId = req.headers["X-Request-Id"] || req.headers["x-request-id"];
    if (requestId) {
        res.headers["X-Request-Id"] = requestId;
        
        // Calculate response time from stored metadata
        var meta = ctx.getDocument("request_meta", requestId);
        if (meta && meta.data && meta.data.startTime) {
            var responseTime = Date.now() - meta.data.startTime;
            res.headers["X-Response-Time"] = responseTime + "ms";
            
            // Log slow responses (could trigger alerting in production)
            if (responseTime > 1000) {
                logSlowRequest(ctx, requestId, responseTime, meta.data);
            }
            
            // Clean up metadata
            ctx.deleteDocument("request_meta", requestId);
        }
    }
    
    // =========================================
    // 4. Caching Headers
    // =========================================
    
    var cacheConfig = getCacheConfig(ctx, req.path, req.method);
    
    if (res.statusCode >= 200 && res.statusCode < 300) {
        if (cacheConfig.cacheable) {
            res.headers["Cache-Control"] = "public, max-age=" + cacheConfig.maxAge;
            res.headers["Vary"] = "Accept, Accept-Encoding, Authorization";
            
            // Add ETag if not present
            if (!res.headers["ETag"] && !res.headers["etag"]) {
                var etag = generateETag(res.body);
                res.headers["ETag"] = '"' + etag + '"';
            }
        } else {
            res.headers["Cache-Control"] = "no-store, no-cache, must-revalidate, private";
            res.headers["Pragma"] = "no-cache";
        }
    }
    
    // =========================================
    // 5. Response Transformation
    // =========================================
    
    var contentType = res.headers["Content-Type"] || res.headers["content-type"] || "";
    
    // Wrap JSON responses in a standard envelope if configured
    if (shouldEnvelopeResponse(ctx, req.path) && contentType.indexOf("application/json") >= 0) {
        try {
            var originalBody = JSON.parse(res.body);
            var enveloped = {
                success: res.statusCode >= 200 && res.statusCode < 400,
                status: res.statusCode,
                data: originalBody,
                meta: {
                    requestId: requestId,
                    timestamp: new Date().toISOString()
                }
            };
            res.body = JSON.stringify(enveloped);
        } catch (e) {
            // Body is not valid JSON, leave as-is
        }
    }
    
    // =========================================
    // 6. Error Response Enhancement
    // =========================================
    
    if (res.statusCode >= 400) {
        // Add error tracking headers
        res.headers["X-Error-Code"] = "ERR-" + res.statusCode;
        
        // Enhance error responses with additional context
        if (contentType.indexOf("application/json") >= 0) {
            try {
                var errorBody = JSON.parse(res.body);
                errorBody.requestId = requestId;
                errorBody.timestamp = new Date().toISOString();
                errorBody.path = req.path;
                
                // Add helpful links for common errors
                if (res.statusCode === 401) {
                    errorBody.help = "https://docs.example.com/auth";
                } else if (res.statusCode === 429) {
                    errorBody.help = "https://docs.example.com/rate-limits";
                }
                
                res.body = JSON.stringify(errorBody);
            } catch (e) {
                // Not JSON, leave as-is
            }
        }
    }
    
    // =========================================
    // 7. Response Compression Hint
    // =========================================
    
    // Add compression hint for downstream (if not already compressed)
    if (!res.headers["Content-Encoding"]) {
        var acceptEncoding = req.headers["Accept-Encoding"] || req.headers["accept-encoding"] || "";
        if (acceptEncoding.indexOf("gzip") >= 0 || acceptEncoding.indexOf("br") >= 0) {
            // Hint that response could be compressed
            res.headers["X-Compression-Hint"] = "eligible";
        }
    }
    
    // =========================================
    // 8. Gateway Information
    // =========================================
    
    res.headers["X-Powered-By"] = "DGate/2.0";
    res.headers["X-Gateway-Region"] = getGatewayRegion(ctx);
}

// =========================================
// Helper Functions
// =========================================

function getAllowedOrigins(ctx) {
    var doc = ctx.getDocument("config", "cors_origins");
    if (doc && doc.data && doc.data.origins) {
        return doc.data.origins;
    }
    // Default allowed origins
    return ["http://localhost:3000", "https://app.example.com", "*"];
}

function isOriginAllowed(origin, allowedOrigins) {
    for (var i = 0; i < allowedOrigins.length; i++) {
        if (allowedOrigins[i] === "*" || allowedOrigins[i] === origin) {
            return true;
        }
        // Support wildcard subdomains: *.example.com
        if (allowedOrigins[i].startsWith("*.")) {
            var domain = allowedOrigins[i].substring(2);
            if (origin.endsWith(domain)) {
                return true;
            }
        }
    }
    return false;
}

function getCacheConfig(ctx, path, method) {
    // Only cache GET/HEAD requests
    if (method !== "GET" && method !== "HEAD") {
        return { cacheable: false, maxAge: 0 };
    }
    
    // Check for path-specific cache config
    var doc = ctx.getDocument("config", "cache_rules");
    if (doc && doc.data && doc.data.rules) {
        var rules = doc.data.rules;
        for (var i = 0; i < rules.length; i++) {
            var rule = rules[i];
            if (path.indexOf(rule.pathPrefix) === 0) {
                return { cacheable: rule.cacheable, maxAge: rule.maxAge };
            }
        }
    }
    
    // Default cache config based on path patterns
    if (path.indexOf("/static/") >= 0 || path.indexOf("/assets/") >= 0) {
        return { cacheable: true, maxAge: 86400 }; // 1 day
    }
    if (path.indexOf("/api/") >= 0) {
        return { cacheable: false, maxAge: 0 };
    }
    
    return { cacheable: true, maxAge: 300 }; // 5 minutes default
}

function generateETag(body) {
    if (!body) return "empty";
    var hash = 0;
    var str = typeof body === "string" ? body : JSON.stringify(body);
    for (var i = 0; i < str.length; i++) {
        var char = str.charCodeAt(i);
        hash = ((hash << 5) - hash) + char;
        hash = hash & hash;
    }
    return Math.abs(hash).toString(36);
}

function shouldEnvelopeResponse(ctx, path) {
    // Check if response enveloping is enabled for this path
    var doc = ctx.getDocument("config", "envelope_paths");
    if (doc && doc.data && doc.data.paths) {
        for (var i = 0; i < doc.data.paths.length; i++) {
            if (path.indexOf(doc.data.paths[i]) === 0) {
                return true;
            }
        }
    }
    return false;
}

function logSlowRequest(ctx, requestId, responseTime, meta) {
    // Store slow request for monitoring/alerting
    var slowLog = ctx.getDocument("monitoring", "slow_requests") || { data: { requests: [] } };
    var requests = slowLog.data.requests || [];
    
    requests.unshift({
        requestId: requestId,
        responseTime: responseTime,
        path: meta.path,
        method: meta.method,
        timestamp: new Date().toISOString()
    });
    
    // Keep only last 100 slow requests
    if (requests.length > 100) {
        requests = requests.slice(0, 100);
    }
    
    ctx.setDocument("monitoring", "slow_requests", { requests: requests });
}

function getGatewayRegion(ctx) {
    var doc = ctx.getDocument("config", "gateway");
    if (doc && doc.data && doc.data.region) {
        return doc.data.region;
    }
    return "default";
}
