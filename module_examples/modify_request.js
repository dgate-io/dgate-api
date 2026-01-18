/**
 * Modify Request Module for DGate v2
 * 
 * This module demonstrates request modification before proxying to upstream.
 * Common use cases: authentication injection, header transformation, 
 * request enrichment, and API versioning.
 * 
 * The requestModifier function is called before forwarding to upstream.
 * Modify ctx.request properties to transform the outgoing request.
 */

/**
 * Modifies the request before it's sent to the upstream service.
 * @param {object} ctx - The request context
 */
function requestModifier(ctx) {
    var req = ctx.request;
    
    // =========================================
    // 1. Add/Modify Headers
    // =========================================
    
    // Add request tracing headers
    var requestId = generateRequestId();
    ctx.setHeader("X-Request-Id", requestId);
    ctx.setHeader("X-Forwarded-By", "DGate/2.0");
    ctx.setHeader("X-Request-Start", Date.now().toString());
    
    // Add service mesh headers
    ctx.setHeader("X-Service-Name", ctx.service || "unknown");
    ctx.setHeader("X-Namespace", ctx.namespace || "default");
    ctx.setHeader("X-Route", ctx.route || "unknown");
    
    // =========================================
    // 2. Authentication Injection
    // =========================================
    
    // Check if request needs auth injection (no existing auth header)
    if (!req.headers["Authorization"] && !req.headers["authorization"]) {
        // Inject service-to-service auth token
        // In production, this would fetch from a secure store
        var serviceToken = getServiceToken(ctx);
        if (serviceToken) {
            ctx.setHeader("Authorization", "Bearer " + serviceToken);
        }
    }
    
    // Add API key for specific upstreams
    var apiKey = getApiKey(ctx, ctx.service);
    if (apiKey) {
        ctx.setHeader("X-API-Key", apiKey);
    }
    
    // =========================================
    // 3. Request Enrichment
    // =========================================
    
    // Add client information headers
    var clientIp = req.headers["X-Forwarded-For"] || req.headers["x-forwarded-for"] || "unknown";
    ctx.setHeader("X-Client-IP", clientIp.split(",")[0].trim());
    
    // Add user context if available (from JWT or session)
    var userContext = extractUserContext(ctx);
    if (userContext) {
        ctx.setHeader("X-User-Id", userContext.userId || "");
        ctx.setHeader("X-User-Roles", (userContext.roles || []).join(","));
        ctx.setHeader("X-Tenant-Id", userContext.tenantId || "");
    }
    
    // =========================================
    // 4. API Version Handling
    // =========================================
    
    // Check for version in Accept header or query param
    var acceptHeader = req.headers["Accept"] || req.headers["accept"] || "";
    var versionFromAccept = extractVersionFromAccept(acceptHeader);
    var versionFromQuery = req.query["api-version"] || req.query["v"];
    
    var apiVersion = versionFromQuery || versionFromAccept || "v1";
    ctx.setHeader("X-API-Version", apiVersion);
    
    // =========================================
    // 5. Request Path Transformation
    // =========================================
    
    // Example: Strip API gateway prefix
    // If path starts with /api/v1/, transform for upstream
    var path = req.path;
    if (path.indexOf("/api/v1/") === 0) {
        // Transform /api/v1/users -> /users
        // Note: Path modification may require upstream path rewrite support
        ctx.setHeader("X-Original-Path", path);
    }
    
    // =========================================
    // 6. Rate Limit Information
    // =========================================
    
    // Add rate limit context for upstream processing
    var rateInfo = getRateLimitInfo(ctx, clientIp);
    ctx.setHeader("X-RateLimit-Remaining", rateInfo.remaining.toString());
    ctx.setHeader("X-RateLimit-Limit", rateInfo.limit.toString());
    
    // =========================================
    // 7. Request Logging/Metrics
    // =========================================
    
    // Store request metadata for response modifier to use
    ctx.setDocument("request_meta", requestId, {
        startTime: Date.now(),
        method: req.method,
        path: req.path,
        clientIp: clientIp,
        userAgent: req.headers["User-Agent"] || req.headers["user-agent"] || "unknown"
    });
}

// =========================================
// Helper Functions
// =========================================

function generateRequestId() {
    var timestamp = Date.now().toString(36);
    var random = Math.random().toString(36).substring(2, 10);
    return timestamp + "-" + random;
}

function getServiceToken(ctx) {
    // In production, fetch from secure token store
    var doc = ctx.getDocument("tokens", "service_token");
    if (doc && doc.data && doc.data.token) {
        // Check expiry
        if (doc.data.expiresAt && doc.data.expiresAt > Date.now()) {
            return doc.data.token;
        }
    }
    
    // Return a demo token for testing
    return "dgate_svc_" + ctx.hashString(ctx.service || "default");
}

function getApiKey(ctx, serviceName) {
    if (!serviceName) return null;
    
    var doc = ctx.getDocument("api_keys", serviceName);
    if (doc && doc.data && doc.data.key) {
        return doc.data.key;
    }
    return null;
}

function extractUserContext(ctx) {
    var authHeader = ctx.request.headers["Authorization"] || ctx.request.headers["authorization"];
    if (!authHeader || !authHeader.startsWith("Bearer ")) {
        return null;
    }
    
    var token = authHeader.substring(7);
    
    // Simple JWT parsing (without verification - that should be done elsewhere)
    try {
        var parts = token.split(".");
        if (parts.length !== 3) return null;
        
        var payload = atob(parts[1].replace(/-/g, "+").replace(/_/g, "/"));
        var claims = JSON.parse(payload);
        
        return {
            userId: claims.sub || claims.user_id,
            roles: claims.roles || claims.scope ? claims.scope.split(" ") : [],
            tenantId: claims.tenant_id || claims.org_id
        };
    } catch (e) {
        return null;
    }
}

function extractVersionFromAccept(acceptHeader) {
    // Parse version from Accept header: application/vnd.api.v2+json
    var match = acceptHeader.match(/vnd\.api\.(v\d+)/);
    return match ? match[1] : null;
}

function getRateLimitInfo(ctx, clientIp) {
    var key = "rate_" + ctx.hashString(clientIp);
    var doc = ctx.getDocument("rate_limits", key);
    
    var now = Date.now();
    var windowMs = 60000; // 1 minute window
    var limit = 100;
    
    if (doc && doc.data) {
        var data = doc.data;
        if (now - data.windowStart < windowMs) {
            // Same window, increment
            data.count++;
            ctx.setDocument("rate_limits", key, data);
            return {
                remaining: Math.max(0, limit - data.count),
                limit: limit
            };
        }
    }
    
    // New window
    ctx.setDocument("rate_limits", key, {
        windowStart: now,
        count: 1
    });
    
    return {
        remaining: limit - 1,
        limit: limit
    };
}
