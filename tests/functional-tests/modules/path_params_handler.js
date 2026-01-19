/**
 * Path Params Handler Module for Testing
 * 
 * Tests path parameter extraction.
 */

function requestHandler(ctx) {
    var userId = ctx.pathParam("user_id");
    var action = ctx.pathParam("action");
    
    ctx.json({
        user_id: userId,
        action: action,
        all_params: ctx.params
    });
}
