/**
 * Base64 Handler Module for Testing
 * 
 * Tests btoa and atob functions.
 */

function requestHandler(ctx) {
    var input = ctx.queryParam("input") || "Hello, World!";
    var operation = ctx.queryParam("op") || "encode";
    
    try {
        var result;
        if (operation === "encode") {
            result = btoa(input);
        } else if (operation === "decode") {
            result = atob(input);
        } else {
            ctx.status(400).json({ error: "Invalid operation. Use 'encode' or 'decode'" });
            return;
        }
        
        ctx.json({
            operation: operation,
            input: input,
            result: result
        });
    } catch (e) {
        ctx.status(500).json({ error: e.message });
    }
}
