/**
 * Write Handler Module for Testing
 * 
 * Tests setHeader, write, and raw response building.
 */

function requestHandler(ctx) {
    var format = ctx.queryParam("format") || "text";
    
    if (format === "html") {
        ctx.setHeader("Content-Type", "text/html");
        ctx.write("<html><body>");
        ctx.write("<h1>Hello from DGate Module</h1>");
        ctx.write("<p>This is HTML content</p>");
        ctx.write("</body></html>");
    } else if (format === "text") {
        ctx.setHeader("Content-Type", "text/plain");
        ctx.write("Line 1: Hello\n");
        ctx.write("Line 2: World\n");
        ctx.write("Line 3: DGate");
    } else {
        ctx.status(400).json({ error: "Unknown format. Use 'html' or 'text'" });
    }
}
