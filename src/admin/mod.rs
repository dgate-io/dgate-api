//! Admin API for DGate
//!
//! Provides REST endpoints for managing all DGate resources:
//! - Namespaces, Routes, Services, Modules
//! - Domains, Secrets, Collections, Documents

use axum::{
    extract::{Path, Query, State},
    http::{HeaderMap, StatusCode},
    response::{IntoResponse, Response},
    routing::{delete, get, put},
    Json, Router,
};
use serde::{Deserialize, Serialize};
use std::sync::Arc;

use crate::config::AdminConfig;
use crate::proxy::ProxyState;
use crate::resources::*;

/// Admin API state
#[derive(Clone)]
pub struct AdminState {
    pub proxy: Arc<ProxyState>,
    pub config: AdminConfig,
    pub version: String,
}

/// API response wrapper
#[derive(Debug, Serialize)]
pub struct ApiResponse<T: Serialize> {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub data: Option<T>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
    pub success: bool,
}

impl<T: Serialize> ApiResponse<T> {
    pub fn success(data: T) -> Self {
        Self {
            data: Some(data),
            error: None,
            success: true,
        }
    }
}

/// Error response type
#[derive(Debug, Serialize)]
pub struct ErrorResponse {
    pub error: String,
    pub success: bool,
}

impl ErrorResponse {
    pub fn new(msg: impl Into<String>) -> Self {
        Self {
            error: msg.into(),
            success: false,
        }
    }
}

/// API error type
pub struct ApiError {
    pub status: StatusCode,
    pub message: String,
}

impl ApiError {
    pub fn not_found(msg: impl Into<String>) -> Self {
        Self {
            status: StatusCode::NOT_FOUND,
            message: msg.into(),
        }
    }

    pub fn internal(msg: impl Into<String>) -> Self {
        Self {
            status: StatusCode::INTERNAL_SERVER_ERROR,
            message: msg.into(),
        }
    }

    pub fn bad_request(msg: impl Into<String>) -> Self {
        Self {
            status: StatusCode::BAD_REQUEST,
            message: msg.into(),
        }
    }
}

impl IntoResponse for ApiError {
    fn into_response(self) -> Response {
        (self.status, Json(ErrorResponse::new(self.message))).into_response()
    }
}

/// Query parameters for list operations
#[derive(Debug, Deserialize, Default)]
pub struct ListQuery {
    pub namespace: Option<String>,
    pub collection: Option<String>,
}

/// Create the admin API router
pub fn create_router(state: AdminState) -> Router {
    Router::new()
        // Root
        .route("/", get(root_handler))
        // Health
        .route("/health", get(health_handler))
        .route("/readyz", get(readyz_handler))
        // API v1
        .nest(
            "/api/v1",
            Router::new()
                // Namespaces
                .route("/namespace", get(list_namespaces))
                .route("/namespace/{name}", get(get_namespace))
                .route("/namespace/{name}", put(put_namespace))
                .route("/namespace/{name}", delete(delete_namespace))
                // Routes
                .route("/route", get(list_routes))
                .route("/route/{namespace}/{name}", get(get_route))
                .route("/route/{namespace}/{name}", put(put_route))
                .route("/route/{namespace}/{name}", delete(delete_route))
                // Services
                .route("/service", get(list_services))
                .route("/service/{namespace}/{name}", get(get_service))
                .route("/service/{namespace}/{name}", put(put_service))
                .route("/service/{namespace}/{name}", delete(delete_service))
                // Modules
                .route("/module", get(list_modules))
                .route("/module/{namespace}/{name}", get(get_module))
                .route("/module/{namespace}/{name}", put(put_module))
                .route("/module/{namespace}/{name}", delete(delete_module))
                // Domains
                .route("/domain", get(list_domains))
                .route("/domain/{namespace}/{name}", get(get_domain))
                .route("/domain/{namespace}/{name}", put(put_domain))
                .route("/domain/{namespace}/{name}", delete(delete_domain))
                // Secrets
                .route("/secret", get(list_secrets))
                .route("/secret/{namespace}/{name}", get(get_secret))
                .route("/secret/{namespace}/{name}", put(put_secret))
                .route("/secret/{namespace}/{name}", delete(delete_secret))
                // Collections
                .route("/collection", get(list_collections))
                .route("/collection/{namespace}/{name}", get(get_collection))
                .route("/collection/{namespace}/{name}", put(put_collection))
                .route("/collection/{namespace}/{name}", delete(delete_collection))
                // Documents
                .route("/document", get(list_documents))
                .route("/document/{namespace}/{collection}/{id}", get(get_document))
                .route("/document/{namespace}/{collection}/{id}", put(put_document))
                .route("/document/{namespace}/{collection}/{id}", delete(delete_document))
                // Change logs
                .route("/changelog", get(list_changelogs)),
        )
        .with_state(state)
}

// Root handlers

async fn root_handler(State(state): State<AdminState>) -> impl IntoResponse {
    let mut headers = HeaderMap::new();
    headers.insert("X-DGate-Version", state.version.parse().unwrap());
    headers.insert(
        "X-DGate-ChangeHash",
        state.proxy.change_hash().to_string().parse().unwrap(),
    );

    (headers, "DGate Admin API")
}

async fn health_handler() -> impl IntoResponse {
    Json(serde_json::json!({
        "status": "healthy"
    }))
}

async fn readyz_handler(State(state): State<AdminState>) -> impl IntoResponse {
    if state.proxy.ready() {
        (StatusCode::OK, Json(serde_json::json!({ "ready": true })))
    } else {
        (
            StatusCode::SERVICE_UNAVAILABLE,
            Json(serde_json::json!({ "ready": false })),
        )
    }
}

// Namespace handlers

async fn list_namespaces(
    State(state): State<AdminState>,
) -> Result<Json<ApiResponse<Vec<Namespace>>>, ApiError> {
    let namespaces = state
        .proxy
        .store()
        .list_namespaces()
        .map_err(|e| ApiError::internal(e.to_string()))?;
    Ok(Json(ApiResponse::success(namespaces)))
}

async fn get_namespace(
    State(state): State<AdminState>,
    Path(name): Path<String>,
) -> Result<Json<ApiResponse<Namespace>>, ApiError> {
    let ns = state
        .proxy
        .store()
        .get_namespace(&name)
        .map_err(|e| ApiError::internal(e.to_string()))?
        .ok_or_else(|| ApiError::not_found("Namespace not found"))?;
    Ok(Json(ApiResponse::success(ns)))
}

async fn put_namespace(
    State(state): State<AdminState>,
    Path(name): Path<String>,
    Json(mut namespace): Json<Namespace>,
) -> Result<Json<ApiResponse<Namespace>>, ApiError> {
    namespace.name = name;

    let changelog = ChangeLog::new(
        ChangeCommand::AddNamespace,
        &namespace.name,
        &namespace.name,
        &namespace,
    );

    state
        .proxy
        .apply_changelog(changelog)
        .await
        .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(namespace)))
}

async fn delete_namespace(
    State(state): State<AdminState>,
    Path(name): Path<String>,
) -> Result<Json<ApiResponse<()>>, ApiError> {
    let namespace = Namespace::new(&name);
    let changelog = ChangeLog::new(ChangeCommand::DeleteNamespace, &name, &name, &namespace);

    state
        .proxy
        .apply_changelog(changelog)
        .await
        .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(())))
}

// Route handlers

async fn list_routes(
    State(state): State<AdminState>,
    Query(query): Query<ListQuery>,
) -> Result<Json<ApiResponse<Vec<Route>>>, ApiError> {
    let routes = match &query.namespace {
        Some(ns) => state.proxy.store().list_routes(ns),
        None => state.proxy.store().list_all_routes(),
    }
    .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(routes)))
}

async fn get_route(
    State(state): State<AdminState>,
    Path((namespace, name)): Path<(String, String)>,
) -> Result<Json<ApiResponse<Route>>, ApiError> {
    let route = state
        .proxy
        .store()
        .get_route(&namespace, &name)
        .map_err(|e| ApiError::internal(e.to_string()))?
        .ok_or_else(|| ApiError::not_found("Route not found"))?;
    Ok(Json(ApiResponse::success(route)))
}

async fn put_route(
    State(state): State<AdminState>,
    Path((namespace, name)): Path<(String, String)>,
    Json(mut route): Json<Route>,
) -> Result<Json<ApiResponse<Route>>, ApiError> {
    route.name = name;
    route.namespace = namespace.clone();

    let changelog = ChangeLog::new(ChangeCommand::AddRoute, &namespace, &route.name, &route);

    state
        .proxy
        .apply_changelog(changelog)
        .await
        .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(route)))
}

async fn delete_route(
    State(state): State<AdminState>,
    Path((namespace, name)): Path<(String, String)>,
) -> Result<Json<ApiResponse<()>>, ApiError> {
    let route = Route::new(&name, &namespace);
    let changelog = ChangeLog::new(ChangeCommand::DeleteRoute, &namespace, &name, &route);

    state
        .proxy
        .apply_changelog(changelog)
        .await
        .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(())))
}

// Service handlers

async fn list_services(
    State(state): State<AdminState>,
    Query(query): Query<ListQuery>,
) -> Result<Json<ApiResponse<Vec<Service>>>, ApiError> {
    let services = match &query.namespace {
        Some(ns) => state.proxy.store().list_services(ns),
        None => {
            // List from all namespaces
            let mut all = Vec::new();
            if let Ok(namespaces) = state.proxy.store().list_namespaces() {
                for ns in namespaces {
                    if let Ok(svcs) = state.proxy.store().list_services(&ns.name) {
                        all.extend(svcs);
                    }
                }
            }
            Ok(all)
        }
    }
    .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(services)))
}

async fn get_service(
    State(state): State<AdminState>,
    Path((namespace, name)): Path<(String, String)>,
) -> Result<Json<ApiResponse<Service>>, ApiError> {
    let service = state
        .proxy
        .store()
        .get_service(&namespace, &name)
        .map_err(|e| ApiError::internal(e.to_string()))?
        .ok_or_else(|| ApiError::not_found("Service not found"))?;
    Ok(Json(ApiResponse::success(service)))
}

async fn put_service(
    State(state): State<AdminState>,
    Path((namespace, name)): Path<(String, String)>,
    Json(mut service): Json<Service>,
) -> Result<Json<ApiResponse<Service>>, ApiError> {
    service.name = name;
    service.namespace = namespace.clone();

    let changelog = ChangeLog::new(
        ChangeCommand::AddService,
        &namespace,
        &service.name,
        &service,
    );

    state
        .proxy
        .apply_changelog(changelog)
        .await
        .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(service)))
}

async fn delete_service(
    State(state): State<AdminState>,
    Path((namespace, name)): Path<(String, String)>,
) -> Result<Json<ApiResponse<()>>, ApiError> {
    let service = Service::new(&name, &namespace);
    let changelog = ChangeLog::new(ChangeCommand::DeleteService, &namespace, &name, &service);

    state
        .proxy
        .apply_changelog(changelog)
        .await
        .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(())))
}

// Module handlers

async fn list_modules(
    State(state): State<AdminState>,
    Query(query): Query<ListQuery>,
) -> Result<Json<ApiResponse<Vec<Module>>>, ApiError> {
    let modules = match &query.namespace {
        Some(ns) => state.proxy.store().list_modules(ns),
        None => {
            let mut all = Vec::new();
            if let Ok(namespaces) = state.proxy.store().list_namespaces() {
                for ns in namespaces {
                    if let Ok(mods) = state.proxy.store().list_modules(&ns.name) {
                        all.extend(mods);
                    }
                }
            }
            Ok(all)
        }
    }
    .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(modules)))
}

async fn get_module(
    State(state): State<AdminState>,
    Path((namespace, name)): Path<(String, String)>,
) -> Result<Json<ApiResponse<Module>>, ApiError> {
    let module = state
        .proxy
        .store()
        .get_module(&namespace, &name)
        .map_err(|e| ApiError::internal(e.to_string()))?
        .ok_or_else(|| ApiError::not_found("Module not found"))?;
    Ok(Json(ApiResponse::success(module)))
}

async fn put_module(
    State(state): State<AdminState>,
    Path((namespace, name)): Path<(String, String)>,
    Json(mut module): Json<Module>,
) -> Result<Json<ApiResponse<Module>>, ApiError> {
    module.name = name;
    module.namespace = namespace.clone();

    let changelog = ChangeLog::new(ChangeCommand::AddModule, &namespace, &module.name, &module);

    state
        .proxy
        .apply_changelog(changelog)
        .await
        .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(module)))
}

async fn delete_module(
    State(state): State<AdminState>,
    Path((namespace, name)): Path<(String, String)>,
) -> Result<Json<ApiResponse<()>>, ApiError> {
    let module = Module::new(&name, &namespace);
    let changelog = ChangeLog::new(ChangeCommand::DeleteModule, &namespace, &name, &module);

    state
        .proxy
        .apply_changelog(changelog)
        .await
        .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(())))
}

// Domain handlers

async fn list_domains(
    State(state): State<AdminState>,
    Query(query): Query<ListQuery>,
) -> Result<Json<ApiResponse<Vec<Domain>>>, ApiError> {
    let domains = match &query.namespace {
        Some(ns) => state.proxy.store().list_domains(ns),
        None => state.proxy.store().list_all_domains(),
    }
    .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(domains)))
}

async fn get_domain(
    State(state): State<AdminState>,
    Path((namespace, name)): Path<(String, String)>,
) -> Result<Json<ApiResponse<Domain>>, ApiError> {
    let domain = state
        .proxy
        .store()
        .get_domain(&namespace, &name)
        .map_err(|e| ApiError::internal(e.to_string()))?
        .ok_or_else(|| ApiError::not_found("Domain not found"))?;
    Ok(Json(ApiResponse::success(domain)))
}

async fn put_domain(
    State(state): State<AdminState>,
    Path((namespace, name)): Path<(String, String)>,
    Json(mut domain): Json<Domain>,
) -> Result<Json<ApiResponse<Domain>>, ApiError> {
    domain.name = name;
    domain.namespace = namespace.clone();

    let changelog = ChangeLog::new(ChangeCommand::AddDomain, &namespace, &domain.name, &domain);

    state
        .proxy
        .apply_changelog(changelog)
        .await
        .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(domain)))
}

async fn delete_domain(
    State(state): State<AdminState>,
    Path((namespace, name)): Path<(String, String)>,
) -> Result<Json<ApiResponse<()>>, ApiError> {
    let domain = Domain::new(&name, &namespace);
    let changelog = ChangeLog::new(ChangeCommand::DeleteDomain, &namespace, &name, &domain);

    state
        .proxy
        .apply_changelog(changelog)
        .await
        .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(())))
}

// Secret handlers

async fn list_secrets(
    State(state): State<AdminState>,
    Query(query): Query<ListQuery>,
) -> Result<Json<ApiResponse<Vec<Secret>>>, ApiError> {
    let secrets = match &query.namespace {
        Some(ns) => {
            let mut secrets = state
                .proxy
                .store()
                .list_secrets(ns)
                .map_err(|e| ApiError::internal(e.to_string()))?;
            // Redact secret data
            for s in &mut secrets {
                s.data = "[REDACTED]".to_string();
            }
            secrets
        }
        None => {
            let mut all = Vec::new();
            if let Ok(namespaces) = state.proxy.store().list_namespaces() {
                for ns in namespaces {
                    if let Ok(mut secrets) = state.proxy.store().list_secrets(&ns.name) {
                        for s in &mut secrets {
                            s.data = "[REDACTED]".to_string();
                        }
                        all.extend(secrets);
                    }
                }
            }
            all
        }
    };

    Ok(Json(ApiResponse::success(secrets)))
}

async fn get_secret(
    State(state): State<AdminState>,
    Path((namespace, name)): Path<(String, String)>,
) -> Result<Json<ApiResponse<Secret>>, ApiError> {
    let mut secret = state
        .proxy
        .store()
        .get_secret(&namespace, &name)
        .map_err(|e| ApiError::internal(e.to_string()))?
        .ok_or_else(|| ApiError::not_found("Secret not found"))?;

    // Redact secret data
    secret.data = "[REDACTED]".to_string();
    Ok(Json(ApiResponse::success(secret)))
}

async fn put_secret(
    State(state): State<AdminState>,
    Path((namespace, name)): Path<(String, String)>,
    Json(mut secret): Json<Secret>,
) -> Result<Json<ApiResponse<Secret>>, ApiError> {
    secret.name = name;
    secret.namespace = namespace.clone();

    let changelog = ChangeLog::new(ChangeCommand::AddSecret, &namespace, &secret.name, &secret);

    state
        .proxy
        .apply_changelog(changelog)
        .await
        .map_err(|e| ApiError::internal(e.to_string()))?;

    secret.data = "[REDACTED]".to_string();
    Ok(Json(ApiResponse::success(secret)))
}

async fn delete_secret(
    State(state): State<AdminState>,
    Path((namespace, name)): Path<(String, String)>,
) -> Result<Json<ApiResponse<()>>, ApiError> {
    let secret = Secret::new(&name, &namespace);
    let changelog = ChangeLog::new(ChangeCommand::DeleteSecret, &namespace, &name, &secret);

    state
        .proxy
        .apply_changelog(changelog)
        .await
        .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(())))
}

// Collection handlers

async fn list_collections(
    State(state): State<AdminState>,
    Query(query): Query<ListQuery>,
) -> Result<Json<ApiResponse<Vec<Collection>>>, ApiError> {
    let collections = match &query.namespace {
        Some(ns) => state.proxy.store().list_collections(ns),
        None => {
            let mut all = Vec::new();
            if let Ok(namespaces) = state.proxy.store().list_namespaces() {
                for ns in namespaces {
                    if let Ok(cols) = state.proxy.store().list_collections(&ns.name) {
                        all.extend(cols);
                    }
                }
            }
            Ok(all)
        }
    }
    .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(collections)))
}

async fn get_collection(
    State(state): State<AdminState>,
    Path((namespace, name)): Path<(String, String)>,
) -> Result<Json<ApiResponse<Collection>>, ApiError> {
    let collection = state
        .proxy
        .store()
        .get_collection(&namespace, &name)
        .map_err(|e| ApiError::internal(e.to_string()))?
        .ok_or_else(|| ApiError::not_found("Collection not found"))?;
    Ok(Json(ApiResponse::success(collection)))
}

async fn put_collection(
    State(state): State<AdminState>,
    Path((namespace, name)): Path<(String, String)>,
    Json(mut collection): Json<Collection>,
) -> Result<Json<ApiResponse<Collection>>, ApiError> {
    collection.name = name;
    collection.namespace = namespace.clone();

    let changelog = ChangeLog::new(
        ChangeCommand::AddCollection,
        &namespace,
        &collection.name,
        &collection,
    );

    state
        .proxy
        .apply_changelog(changelog)
        .await
        .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(collection)))
}

async fn delete_collection(
    State(state): State<AdminState>,
    Path((namespace, name)): Path<(String, String)>,
) -> Result<Json<ApiResponse<()>>, ApiError> {
    let collection = Collection::new(&name, &namespace);
    let changelog = ChangeLog::new(
        ChangeCommand::DeleteCollection,
        &namespace,
        &name,
        &collection,
    );

    state
        .proxy
        .apply_changelog(changelog)
        .await
        .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(())))
}

// Document handlers

async fn list_documents(
    State(state): State<AdminState>,
    Query(query): Query<ListQuery>,
) -> Result<Json<ApiResponse<Vec<Document>>>, ApiError> {
    let namespace = query.namespace.as_deref().unwrap_or("default");
    let collection = query
        .collection
        .as_deref()
        .ok_or_else(|| ApiError::bad_request("collection query parameter is required"))?;

    let documents = state
        .proxy
        .store()
        .list_documents(namespace, collection)
        .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(documents)))
}

async fn get_document(
    State(state): State<AdminState>,
    Path((namespace, collection, id)): Path<(String, String, String)>,
) -> Result<Json<ApiResponse<Document>>, ApiError> {
    let document = state
        .proxy
        .store()
        .get_document(&namespace, &collection, &id)
        .map_err(|e| ApiError::internal(e.to_string()))?
        .ok_or_else(|| ApiError::not_found("Document not found"))?;
    Ok(Json(ApiResponse::success(document)))
}

async fn put_document(
    State(state): State<AdminState>,
    Path((namespace, collection, id)): Path<(String, String, String)>,
    Json(mut document): Json<Document>,
) -> Result<Json<ApiResponse<Document>>, ApiError> {
    document.id = id;
    document.namespace = namespace.clone();
    document.collection = collection;

    let changelog = ChangeLog::new(
        ChangeCommand::AddDocument,
        &namespace,
        &document.id,
        &document,
    );

    state
        .proxy
        .apply_changelog(changelog)
        .await
        .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(document)))
}

async fn delete_document(
    State(state): State<AdminState>,
    Path((namespace, collection, id)): Path<(String, String, String)>,
) -> Result<Json<ApiResponse<()>>, ApiError> {
    let document = Document::new(&id, &namespace, &collection);
    let changelog = ChangeLog::new(ChangeCommand::DeleteDocument, &namespace, &id, &document);

    state
        .proxy
        .apply_changelog(changelog)
        .await
        .map_err(|e| ApiError::internal(e.to_string()))?;

    Ok(Json(ApiResponse::success(())))
}

// Changelog handlers

async fn list_changelogs(
    State(state): State<AdminState>,
) -> Result<Json<ApiResponse<Vec<ChangeLog>>>, ApiError> {
    let logs = state
        .proxy
        .store()
        .list_changelogs()
        .map_err(|e| ApiError::internal(e.to_string()))?;
    Ok(Json(ApiResponse::success(logs)))
}
