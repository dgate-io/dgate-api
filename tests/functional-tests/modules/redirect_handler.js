/**
 * Redirect Handler Module for Testing
 * 
 * Tests redirect functionality.
 */

function requestHandler(ctx) {
    var target = ctx.queryParam("target");
    var permanent = ctx.queryParam("permanent") === "true";
    
    if (!target) {
        ctx.status(400).json({ error: "target query param required" });
        return;
    }
    
    ctx.redirect(target, permanent ? 301 : 302);
}
