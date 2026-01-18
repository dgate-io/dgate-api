/**
 * Status Handler Module for Testing
 * 
 * Tests status code setting and response chaining.
 * Responds with different status codes based on query param.
 */

function requestHandler(ctx) {
    var statusCode = parseInt(ctx.queryParam("code")) || 200;
    
    ctx.status(statusCode).json({
        status: statusCode,
        message: "Status code test",
        timestamp: new Date().toISOString()
    });
}
