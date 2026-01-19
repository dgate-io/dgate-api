/**
 * Error Handler Module for Testing
 * 
 * Custom error handling for upstream failures.
 */

function errorHandler(ctx, error) {
    ctx.status(503).json({
        error: "Service temporarily unavailable",
        original_error: error.message || String(error),
        path: ctx.request.path,
        timestamp: new Date().toISOString()
    });
}
