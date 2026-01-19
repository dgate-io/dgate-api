/**
 * Echo Handler Module for Testing
 * 
 * Returns detailed request information as JSON response.
 */

function requestHandler(ctx) {
    ctx.json({
        method: ctx.request.method,
        path: ctx.request.path,
        query: ctx.request.query,
        headers: ctx.request.headers,
        body: ctx.request.body,
        params: ctx.params
    });
}
