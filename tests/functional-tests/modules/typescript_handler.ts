/**
 * TypeScript Handler Module for Testing
 * 
 * Tests TypeScript module support with type annotations.
 * Note: Types are stripped during transpilation.
 */

interface Context {
    request: {
        method: string;
        path: string;
        query: Record<string, string>;
        headers: Record<string, string>;
        body?: string;
    };
    params: Record<string, string>;
    json(data: object): void;
    status(code: number): Context;
    queryParam(name: string): string | null;
    pathParam(name: string): string | null;
}

interface TypedResponse {
    language: string;
    features: string[];
    method: string;
    path: string;
}

function requestHandler(ctx: Context): void {
    const response: TypedResponse = {
        language: "typescript",
        features: ["type-annotations", "interfaces", "generics"],
        method: ctx.request.method,
        path: ctx.request.path
    };
    
    ctx.json(response);
}
