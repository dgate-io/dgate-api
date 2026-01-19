/**
 * Hash Handler Module for Testing
 * 
 * Tests the hashString utility function.
 */

function requestHandler(ctx) {
    var input = ctx.queryParam("input");
    
    if (!input) {
        ctx.status(400).json({ error: "input query param required" });
        return;
    }
    
    var hash = ctx.hashString(input);
    
    ctx.json({
        input: input,
        hash: hash,
        length: hash.length
    });
}
