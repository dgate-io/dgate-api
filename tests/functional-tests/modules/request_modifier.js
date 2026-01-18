/**
 * Request Modifier Module for Testing
 * 
 * Modifies incoming requests by adding headers and query params.
 */

function requestModifier(ctx) {
    // Add custom headers to the request
    ctx.request.headers["X-Modified-By"] = "dgate-module";
    ctx.request.headers["X-Request-Timestamp"] = new Date().toISOString();
    
    // Modify path if needed
    if (ctx.request.path.startsWith("/transform/")) {
        ctx.request.path = ctx.request.path.replace("/transform/", "/api/");
    }
}
