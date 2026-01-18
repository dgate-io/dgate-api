/**
 * Merge Responses Module for DGate v2
 * 
 * This module demonstrates aggregating data from multiple sources
 * into a single unified response. Useful for API composition patterns.
 * 
 * Example usage:
 *   GET /aggregate           -> Returns merged user profile + preferences + stats
 *   GET /aggregate/:userId   -> Returns merged data for specific user
 *   POST /aggregate          -> Custom merge with body-specified sources
 */

function requestHandler(ctx) {
    var req = ctx.request;
    var method = req.method;
    var userId = ctx.pathParam("userId") || "default";
    
    if (method === "GET") {
        // Simulate fetching from multiple data sources
        var profile = getProfileData(ctx, userId);
        var preferences = getPreferencesData(ctx, userId);
        var stats = getStatsData(ctx, userId);
        
        // Merge all responses into a unified object
        var merged = {
            user: {
                id: userId,
                profile: profile,
                preferences: preferences,
                stats: stats
            },
            metadata: {
                sources: ["profile", "preferences", "stats"],
                mergedAt: new Date().toISOString(),
                version: "1.0"
            }
        };
        
        ctx.json(merged);
        
    } else if (method === "POST") {
        // Allow custom merge configuration via request body
        var body = req.body;
        var config;
        
        try {
            config = JSON.parse(body || "{}");
        } catch (e) {
            ctx.status(400).json({ error: "Invalid JSON body" });
            return;
        }
        
        var sources = config.sources || ["profile", "preferences", "stats"];
        var targetUserId = config.userId || userId;
        var result = { user: { id: targetUserId } };
        
        // Selectively merge requested sources
        for (var i = 0; i < sources.length; i++) {
            var source = sources[i];
            if (source === "profile") {
                result.user.profile = getProfileData(ctx, targetUserId);
            } else if (source === "preferences") {
                result.user.preferences = getPreferencesData(ctx, targetUserId);
            } else if (source === "stats") {
                result.user.stats = getStatsData(ctx, targetUserId);
            }
        }
        
        result.metadata = {
            sources: sources,
            mergedAt: new Date().toISOString(),
            version: "1.0"
        };
        
        ctx.json(result);
        
    } else {
        ctx.status(405).json({ error: "Method not allowed" });
    }
}

// Simulated data sources (in production, these would fetch from upstreams)
function getProfileData(ctx, userId) {
    // Check cache first
    var cached = ctx.getDocument("profiles", userId);
    if (cached && cached.data) {
        return cached.data;
    }
    
    // Simulated profile data
    var profile = {
        displayName: "User " + userId,
        email: userId + "@example.com",
        avatar: "https://api.dicebear.com/7.x/avatars/svg?seed=" + userId,
        createdAt: "2024-01-15T10:30:00Z"
    };
    
    // Cache the profile
    ctx.setDocument("profiles", userId, profile);
    
    return profile;
}

function getPreferencesData(ctx, userId) {
    var cached = ctx.getDocument("preferences", userId);
    if (cached && cached.data) {
        return cached.data;
    }
    
    // Simulated preferences
    var preferences = {
        theme: "dark",
        language: "en",
        notifications: {
            email: true,
            push: false,
            sms: false
        },
        timezone: "UTC"
    };
    
    ctx.setDocument("preferences", userId, preferences);
    
    return preferences;
}

function getStatsData(ctx, userId) {
    var cached = ctx.getDocument("stats", userId);
    if (cached && cached.data) {
        return cached.data;
    }
    
    // Simulated statistics
    var hash = ctx.hashString(userId);
    var stats = {
        totalRequests: parseInt(hash.slice(0, 4), 36) % 10000,
        activeSeconds: parseInt(hash.slice(4, 8), 36) % 86400,
        lastActive: new Date().toISOString(),
        level: (parseInt(hash.slice(0, 2), 36) % 50) + 1
    };
    
    ctx.setDocument("stats", userId, stats);
    
    return stats;
}
