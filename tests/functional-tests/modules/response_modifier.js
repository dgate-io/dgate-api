/**
 * Response Modifier Module for Testing
 * 
 * Modifies outgoing responses by adding headers.
 */

function responseModifier(ctx, res) {
    // Add custom headers to the response
    res.headers["X-Processed-By"] = "dgate-response-modifier";
    res.headers["X-Original-Status"] = res.statusCode.toString();
}
