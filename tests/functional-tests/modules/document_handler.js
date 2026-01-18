/**
 * Document Handler Module for Testing
 * 
 * Tests document storage operations: getDocument, setDocument, deleteDocument.
 */

function requestHandler(ctx) {
    var method = ctx.request.method;
    var collection = ctx.queryParam("collection") || "test_docs";
    var id = ctx.queryParam("id");
    
    if (!id) {
        ctx.status(400).json({ error: "id query param required" });
        return;
    }
    
    if (method === "GET") {
        var doc = ctx.getDocument(collection, id);
        if (doc && doc.data) {
            ctx.json({ found: true, data: doc.data });
        } else {
            ctx.status(404).json({ found: false, message: "Document not found" });
        }
    } else if (method === "POST" || method === "PUT") {
        var body = ctx.request.body;
        var data;
        try {
            data = JSON.parse(body);
        } catch (e) {
            data = { value: body };
        }
        
        ctx.setDocument(collection, id, data);
        ctx.status(201).json({ created: true, id: id, collection: collection });
    } else if (method === "DELETE") {
        ctx.deleteDocument(collection, id);
        ctx.json({ deleted: true, id: id, collection: collection });
    } else {
        ctx.status(405).json({ error: "Method not allowed" });
    }
}
