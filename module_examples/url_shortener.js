/**
 * URL Shortener Module for DGate v2
 * 
 * This module demonstrates request handling without an upstream service.
 * It creates short URLs and redirects when accessed.
 * 
 * Example usage:
 *   POST /?url=https://example.com  -> Creates short link, returns {"id": "abc12345"}
 *   GET /:id                        -> Redirects to the stored URL
 */

function requestHandler(ctx) {
    var req = ctx.request;
    var method = req.method;
    var path = req.path;
    
    if (method === "POST") {
        // Create a new short link
        var url = ctx.queryParam("url");
        if (!url) {
            ctx.status(400).json({ error: "url query parameter is required" });
            return;
        }
        
        // Validate URL
        if (!url.startsWith("http://") && !url.startsWith("https://")) {
            ctx.status(400).json({ error: "url must start with http:// or https://" });
            return;
        }
        
        // Generate short ID using hash
        var id = ctx.hashString(url);
        
        // Store the URL
        ctx.setDocument("short_links", id, { url: url });
        
        ctx.status(201).json({ 
            id: id,
            short_url: "/" + id
        });
        
    } else if (method === "GET") {
        // Get the ID from the path
        var id = ctx.pathParam("id");
        
        if (!id || id === "/" || id === "") {
            // Show info page
            ctx.json({
                name: "DGate URL Shortener",
                version: "1.0",
                usage: {
                    create: "POST /?url=https://example.com",
                    access: "GET /:id"
                }
            });
            return;
        }
        
        // Look up the short link
        var doc = ctx.getDocument("short_links", id);
        
        if (!doc || !doc.data || !doc.data.url) {
            ctx.status(404).json({ error: "Short link not found" });
            return;
        }
        
        // Redirect to the stored URL
        ctx.redirect(doc.data.url, 302);
        
    } else {
        ctx.status(405).json({ error: "Method not allowed" });
    }
}
