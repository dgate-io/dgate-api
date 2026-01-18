/**
 * URL Fallback Module for DGate v2
 * 
 * This module provides failover/fallback URL selection for upstream services.
 * When the primary upstream fails, it cycles through backup upstreams.
 * 
 * Configuration is stored in documents and can be managed via API:
 *   GET /fallback/config              -> View current fallback configuration
 *   POST /fallback/config             -> Update fallback configuration
 *   GET /fallback/status              -> View health status of all upstreams
 *   POST /fallback/reset              -> Reset failure counters
 * 
 * The fetchUpstreamUrl function is called by DGate to determine the upstream URL.
 */

// Default configuration
var DEFAULT_CONFIG = {
    upstreams: [
        { url: "http://primary.example.com", priority: 1 },
        { url: "http://secondary.example.com", priority: 2 },
        { url: "http://tertiary.example.com", priority: 3 }
    ],
    maxFailures: 3,           // Failures before marking unhealthy
    recoveryTimeMs: 30000,    // Time before retrying failed upstream
    strategy: "priority"      // "priority", "round-robin", or "random"
};

/**
 * Called by DGate to determine which upstream URL to use.
 * Returns the URL of the healthiest available upstream.
 */
function fetchUpstreamUrl(ctx) {
    var config = getConfig(ctx);
    var status = getHealthStatus(ctx);
    var now = Date.now();
    
    // Sort upstreams by priority
    var candidates = [];
    for (var i = 0; i < config.upstreams.length; i++) {
        var upstream = config.upstreams[i];
        var health = status[upstream.url] || { failures: 0, lastFailure: 0 };
        
        // Check if upstream is healthy or has recovered
        var isHealthy = health.failures < config.maxFailures;
        var hasRecovered = (now - health.lastFailure) > config.recoveryTimeMs;
        
        if (isHealthy || hasRecovered) {
            candidates.push({
                url: upstream.url,
                priority: upstream.priority,
                failures: health.failures
            });
        }
    }
    
    // If no healthy upstreams, return the one with least failures
    if (candidates.length === 0) {
        var best = null;
        var leastFailures = Infinity;
        for (var i = 0; i < config.upstreams.length; i++) {
            var upstream = config.upstreams[i];
            var health = status[upstream.url] || { failures: 0 };
            if (health.failures < leastFailures) {
                leastFailures = health.failures;
                best = upstream.url;
            }
        }
        return best || config.upstreams[0].url;
    }
    
    // Select based on strategy
    if (config.strategy === "round-robin") {
        var counter = getCounter(ctx);
        var selected = candidates[counter % candidates.length];
        setCounter(ctx, counter + 1);
        return selected.url;
    } else if (config.strategy === "random") {
        var idx = Math.floor(Math.random() * candidates.length);
        return candidates[idx].url;
    } else {
        // Default: priority (lowest number = highest priority)
        candidates.sort(function(a, b) { return a.priority - b.priority; });
        return candidates[0].url;
    }
}

/**
 * Error handler - called when upstream request fails.
 * Records the failure for circuit breaker logic.
 */
function errorHandler(ctx, error) {
    var upstreamUrl = ctx.upstreamUrl || "";
    
    if (upstreamUrl) {
        var status = getHealthStatus(ctx);
        var health = status[upstreamUrl] || { failures: 0, lastFailure: 0 };
        
        health.failures++;
        health.lastFailure = Date.now();
        health.lastError = error ? error.message : "Unknown error";
        
        status[upstreamUrl] = health;
        ctx.setDocument("fallback", "health_status", status);
    }
    
    // Return error response
    ctx.status(502).json({
        error: "Upstream service unavailable",
        upstream: upstreamUrl,
        message: error ? error.message : "Connection failed"
    });
}

/**
 * Request handler for fallback configuration management.
 */
function requestHandler(ctx) {
    var req = ctx.request;
    var method = req.method;
    var path = req.path;
    
    if (path.indexOf("/fallback/config") === 0) {
        if (method === "GET") {
            ctx.json(getConfig(ctx));
        } else if (method === "POST") {
            var body = req.body;
            try {
                var newConfig = JSON.parse(body || "{}");
                // Merge with existing config
                var config = getConfig(ctx);
                if (newConfig.upstreams) config.upstreams = newConfig.upstreams;
                if (newConfig.maxFailures) config.maxFailures = newConfig.maxFailures;
                if (newConfig.recoveryTimeMs) config.recoveryTimeMs = newConfig.recoveryTimeMs;
                if (newConfig.strategy) config.strategy = newConfig.strategy;
                
                ctx.setDocument("fallback", "config", config);
                ctx.json({ success: true, config: config });
            } catch (e) {
                ctx.status(400).json({ error: "Invalid JSON: " + e.message });
            }
        } else {
            ctx.status(405).json({ error: "Method not allowed" });
        }
        
    } else if (path.indexOf("/fallback/status") === 0) {
        var status = getHealthStatus(ctx);
        var config = getConfig(ctx);
        
        var detailed = [];
        for (var i = 0; i < config.upstreams.length; i++) {
            var upstream = config.upstreams[i];
            var health = status[upstream.url] || { failures: 0, lastFailure: 0 };
            var isHealthy = health.failures < config.maxFailures;
            var hasRecovered = (Date.now() - health.lastFailure) > config.recoveryTimeMs;
            
            detailed.push({
                url: upstream.url,
                priority: upstream.priority,
                status: (isHealthy || hasRecovered) ? "healthy" : "unhealthy",
                failures: health.failures,
                lastFailure: health.lastFailure ? new Date(health.lastFailure).toISOString() : null,
                lastError: health.lastError || null
            });
        }
        
        ctx.json({ upstreams: detailed });
        
    } else if (path.indexOf("/fallback/reset") === 0 && method === "POST") {
        ctx.setDocument("fallback", "health_status", {});
        ctx.setDocument("fallback", "counter", { value: 0 });
        ctx.json({ success: true, message: "Health status reset" });
        
    } else {
        ctx.json({
            name: "DGate URL Fallback",
            version: "1.0",
            endpoints: {
                config: "GET/POST /fallback/config",
                status: "GET /fallback/status",
                reset: "POST /fallback/reset"
            }
        });
    }
}

// Helper functions
function getConfig(ctx) {
    var doc = ctx.getDocument("fallback", "config");
    return (doc && doc.data) ? doc.data : DEFAULT_CONFIG;
}

function getHealthStatus(ctx) {
    var doc = ctx.getDocument("fallback", "health_status");
    return (doc && doc.data) ? doc.data : {};
}

function getCounter(ctx) {
    var doc = ctx.getDocument("fallback", "counter");
    return (doc && doc.data && doc.data.value) ? doc.data.value : 0;
}

function setCounter(ctx, value) {
    ctx.setDocument("fallback", "counter", { value: value });
}
