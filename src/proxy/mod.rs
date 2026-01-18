//! Proxy module for DGate
//!
//! Handles incoming HTTP requests, routes them through modules,
//! and forwards them to upstream services.

use axum::{
    body::Body,
    extract::{Request, State},
    http::{header, HeaderMap, Method, StatusCode, Uri},
    response::{IntoResponse, Response},
    Router,
};
use dashmap::DashMap;
use hyper_util::client::legacy::Client;
use hyper_util::rt::TokioExecutor;
use parking_lot::RwLock;
use regex::Regex;
use std::collections::HashMap;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};
use tracing::{debug, error, info, warn};

use crate::config::{DGateConfig, ProxyConfig};
use crate::modules::{ModuleContext, ModuleError, ModuleExecutor, RequestContext, ResponseContext};
use crate::resources::*;
use crate::storage::{create_storage, ProxyStore, Storage};

/// Compiled route pattern for matching
#[derive(Debug, Clone)]
struct CompiledRoute {
    route: Route,
    namespace: Namespace,
    service: Option<Service>,
    modules: Vec<Module>,
    path_patterns: Vec<Regex>,
    methods: Vec<String>,
}

impl CompiledRoute {
    fn matches(&self, path: &str, method: &str) -> Option<HashMap<String, String>> {
        // Check method
        let method_match = self
            .methods
            .iter()
            .any(|m| m == "*" || m.eq_ignore_ascii_case(method));
        if !method_match {
            return None;
        }

        // Check path patterns
        for pattern in &self.path_patterns {
            if let Some(captures) = pattern.captures(path) {
                let mut params = HashMap::new();
                for name in pattern.capture_names().flatten() {
                    if let Some(value) = captures.name(name) {
                        params.insert(name.to_string(), value.as_str().to_string());
                    }
                }
                return Some(params);
            }
        }

        None
    }
}

/// Namespace router containing compiled routes
struct NamespaceRouter {
    namespace: Namespace,
    routes: Vec<CompiledRoute>,
}

impl NamespaceRouter {
    fn new(namespace: Namespace) -> Self {
        Self {
            namespace,
            routes: Vec::new(),
        }
    }

    fn add_route(&mut self, compiled: CompiledRoute) {
        self.routes.push(compiled);
    }

    fn find_route(
        &self,
        path: &str,
        method: &str,
    ) -> Option<(&CompiledRoute, HashMap<String, String>)> {
        for route in &self.routes {
            if let Some(params) = route.matches(path, method) {
                return Some((route, params));
            }
        }
        None
    }
}

/// Main proxy state
pub struct ProxyState {
    config: DGateConfig,
    store: Arc<ProxyStore>,
    module_executor: RwLock<ModuleExecutor>,
    routers: DashMap<String, NamespaceRouter>,
    domains: RwLock<Vec<ResolvedDomain>>,
    ready: AtomicBool,
    change_hash: AtomicU64,
    http_client: Client<
        hyper_rustls::HttpsConnector<hyper_util::client::legacy::connect::HttpConnector>,
        Body,
    >,
    /// Shared reqwest client for upstream requests
    reqwest_client: reqwest::Client,
}

impl ProxyState {
    pub fn new(config: DGateConfig) -> Arc<Self> {
        // Create storage
        let storage = create_storage(&config.storage);
        let store = Arc::new(ProxyStore::new(storage));

        // Create HTTP client for upstream requests
        let https = hyper_rustls::HttpsConnectorBuilder::new()
            .with_native_roots()
            .expect("Failed to load native roots")
            .https_or_http()
            .enable_http1()
            .enable_http2()
            .build();

        let client = Client::builder(TokioExecutor::new())
            .pool_idle_timeout(Duration::from_secs(30))
            .build(https);

        // Create a shared reqwest client with connection pooling
        let reqwest_client = reqwest::Client::builder()
            .pool_max_idle_per_host(100)
            .pool_idle_timeout(Duration::from_secs(30))
            .timeout(Duration::from_secs(30))
            .build()
            .expect("Failed to create reqwest client");

        let state = Arc::new(Self {
            config,
            store,
            module_executor: RwLock::new(ModuleExecutor::new()),
            routers: DashMap::new(),
            domains: RwLock::new(Vec::new()),
            ready: AtomicBool::new(false),
            change_hash: AtomicU64::new(0),
            http_client: client,
            reqwest_client,
        });

        state
    }

    pub fn store(&self) -> &ProxyStore {
        &self.store
    }

    pub fn ready(&self) -> bool {
        self.ready.load(Ordering::Relaxed)
    }

    pub fn set_ready(&self, ready: bool) {
        self.ready.store(ready, Ordering::Relaxed);
    }

    pub fn change_hash(&self) -> u64 {
        self.change_hash.load(Ordering::Relaxed)
    }

    /// Apply a change log entry
    pub async fn apply_changelog(&self, changelog: ChangeLog) -> Result<(), ProxyError> {
        // Store the changelog
        self.store
            .append_changelog(&changelog)
            .map_err(|e| ProxyError::Storage(e.to_string()))?;

        // Process the change
        self.process_changelog(&changelog)?;

        // Update change hash
        let new_hash = self.change_hash.fetch_add(1, Ordering::Relaxed) + 1;
        debug!("Applied changelog {}, new hash: {}", changelog.id, new_hash);

        Ok(())
    }

    fn process_changelog(&self, changelog: &ChangeLog) -> Result<(), ProxyError> {
        match changelog.cmd {
            ChangeCommand::AddNamespace => {
                let ns: Namespace = serde_json::from_value(changelog.item.clone())
                    .map_err(|e| ProxyError::Deserialization(e.to_string()))?;
                self.store
                    .set_namespace(&ns)
                    .map_err(|e| ProxyError::Storage(e.to_string()))?;
            }
            ChangeCommand::DeleteNamespace => {
                self.store
                    .delete_namespace(&changelog.name)
                    .map_err(|e| ProxyError::Storage(e.to_string()))?;
                self.routers.remove(&changelog.name);
            }
            ChangeCommand::AddRoute => {
                let route: Route = serde_json::from_value(changelog.item.clone())
                    .map_err(|e| ProxyError::Deserialization(e.to_string()))?;
                self.store
                    .set_route(&route)
                    .map_err(|e| ProxyError::Storage(e.to_string()))?;
                self.rebuild_router(&changelog.namespace)?;
            }
            ChangeCommand::DeleteRoute => {
                self.store
                    .delete_route(&changelog.namespace, &changelog.name)
                    .map_err(|e| ProxyError::Storage(e.to_string()))?;
                self.rebuild_router(&changelog.namespace)?;
            }
            ChangeCommand::AddService => {
                let service: Service = serde_json::from_value(changelog.item.clone())
                    .map_err(|e| ProxyError::Deserialization(e.to_string()))?;
                self.store
                    .set_service(&service)
                    .map_err(|e| ProxyError::Storage(e.to_string()))?;
                self.rebuild_router(&changelog.namespace)?;
            }
            ChangeCommand::DeleteService => {
                self.store
                    .delete_service(&changelog.namespace, &changelog.name)
                    .map_err(|e| ProxyError::Storage(e.to_string()))?;
                self.rebuild_router(&changelog.namespace)?;
            }
            ChangeCommand::AddModule => {
                let module: Module = serde_json::from_value(changelog.item.clone())
                    .map_err(|e| ProxyError::Deserialization(e.to_string()))?;
                self.store
                    .set_module(&module)
                    .map_err(|e| ProxyError::Storage(e.to_string()))?;

                // Add to module executor
                let mut executor = self.module_executor.write();
                executor
                    .add_module(&module)
                    .map_err(|e| ProxyError::Module(e.to_string()))?;

                self.rebuild_router(&changelog.namespace)?;
            }
            ChangeCommand::DeleteModule => {
                self.store
                    .delete_module(&changelog.namespace, &changelog.name)
                    .map_err(|e| ProxyError::Storage(e.to_string()))?;

                let mut executor = self.module_executor.write();
                executor.remove_module(&changelog.namespace, &changelog.name);

                self.rebuild_router(&changelog.namespace)?;
            }
            ChangeCommand::AddDomain => {
                let domain: Domain = serde_json::from_value(changelog.item.clone())
                    .map_err(|e| ProxyError::Deserialization(e.to_string()))?;
                self.store
                    .set_domain(&domain)
                    .map_err(|e| ProxyError::Storage(e.to_string()))?;
                self.rebuild_domains()?;
            }
            ChangeCommand::DeleteDomain => {
                self.store
                    .delete_domain(&changelog.namespace, &changelog.name)
                    .map_err(|e| ProxyError::Storage(e.to_string()))?;
                self.rebuild_domains()?;
            }
            ChangeCommand::AddSecret => {
                let secret: Secret = serde_json::from_value(changelog.item.clone())
                    .map_err(|e| ProxyError::Deserialization(e.to_string()))?;
                self.store
                    .set_secret(&secret)
                    .map_err(|e| ProxyError::Storage(e.to_string()))?;
            }
            ChangeCommand::DeleteSecret => {
                self.store
                    .delete_secret(&changelog.namespace, &changelog.name)
                    .map_err(|e| ProxyError::Storage(e.to_string()))?;
            }
            ChangeCommand::AddCollection => {
                let collection: Collection = serde_json::from_value(changelog.item.clone())
                    .map_err(|e| ProxyError::Deserialization(e.to_string()))?;
                self.store
                    .set_collection(&collection)
                    .map_err(|e| ProxyError::Storage(e.to_string()))?;
            }
            ChangeCommand::DeleteCollection => {
                self.store
                    .delete_collection(&changelog.namespace, &changelog.name)
                    .map_err(|e| ProxyError::Storage(e.to_string()))?;
            }
            ChangeCommand::AddDocument => {
                let document: Document = serde_json::from_value(changelog.item.clone())
                    .map_err(|e| ProxyError::Deserialization(e.to_string()))?;
                self.store
                    .set_document(&document)
                    .map_err(|e| ProxyError::Storage(e.to_string()))?;
            }
            ChangeCommand::DeleteDocument => {
                let doc: Document = serde_json::from_value(changelog.item.clone())
                    .map_err(|e| ProxyError::Deserialization(e.to_string()))?;
                self.store
                    .delete_document(&changelog.namespace, &doc.collection, &changelog.name)
                    .map_err(|e| ProxyError::Storage(e.to_string()))?;
            }
        }

        Ok(())
    }

    fn rebuild_router(&self, namespace: &str) -> Result<(), ProxyError> {
        let ns = self
            .store
            .get_namespace(namespace)
            .map_err(|e| ProxyError::Storage(e.to_string()))?
            .ok_or_else(|| ProxyError::NotFound(format!("Namespace {} not found", namespace)))?;

        let routes = self
            .store
            .list_routes(namespace)
            .map_err(|e| ProxyError::Storage(e.to_string()))?;

        let mut router = NamespaceRouter::new(ns.clone());

        for route in routes {
            // Get service if specified
            let service = if let Some(ref svc_name) = route.service {
                self.store
                    .get_service(namespace, svc_name)
                    .map_err(|e| ProxyError::Storage(e.to_string()))?
            } else {
                None
            };

            // Get modules
            let mut modules = Vec::new();
            for mod_name in &route.modules {
                if let Some(module) = self
                    .store
                    .get_module(namespace, mod_name)
                    .map_err(|e| ProxyError::Storage(e.to_string()))?
                {
                    modules.push(module);
                }
            }

            // Compile path patterns
            let path_patterns: Vec<Regex> = route
                .paths
                .iter()
                .filter_map(|p| compile_path_pattern(p).ok())
                .collect();

            let compiled = CompiledRoute {
                route: route.clone(),
                namespace: ns.clone(),
                service,
                modules,
                path_patterns,
                methods: route.methods.clone(),
            };

            router.add_route(compiled);
        }

        self.routers.insert(namespace.to_string(), router);
        Ok(())
    }

    fn rebuild_domains(&self) -> Result<(), ProxyError> {
        let all_domains = self
            .store
            .list_all_domains()
            .map_err(|e| ProxyError::Storage(e.to_string()))?;

        let mut resolved = Vec::new();
        for domain in all_domains {
            if let Some(ns) = self
                .store
                .get_namespace(&domain.namespace)
                .map_err(|e| ProxyError::Storage(e.to_string()))?
            {
                resolved.push(ResolvedDomain {
                    domain: domain.clone(),
                    namespace: ns,
                });
            }
        }

        // Sort by priority (higher first)
        resolved.sort_by(|a, b| b.domain.priority.cmp(&a.domain.priority));

        *self.domains.write() = resolved;
        Ok(())
    }

    /// Initialize from stored change logs
    pub async fn restore_from_changelogs(&self) -> Result<(), ProxyError> {
        let changelogs = self
            .store
            .list_changelogs()
            .map_err(|e| ProxyError::Storage(e.to_string()))?;

        info!("Restoring {} change logs", changelogs.len());

        for changelog in changelogs {
            if let Err(e) = self.process_changelog(&changelog) {
                warn!("Failed to restore changelog {}: {}", changelog.id, e);
            }
        }

        // Ensure default namespace exists if not disabled
        if !self.config.disable_default_namespace {
            if self.store.get_namespace("default").ok().flatten().is_none() {
                let default_ns = Namespace::default_namespace();
                self.store
                    .set_namespace(&default_ns)
                    .map_err(|e| ProxyError::Storage(e.to_string()))?;
                self.rebuild_router("default").ok();
            }
        }

        self.set_ready(true);
        Ok(())
    }

    /// Initialize resources from config
    pub async fn init_from_config(&self) -> Result<(), ProxyError> {
        if let Some(ref init) = self.config.proxy.init_resources {
            info!("Initializing resources from config");

            // Add namespaces
            for ns in &init.namespaces {
                let changelog = ChangeLog::new(ChangeCommand::AddNamespace, &ns.name, &ns.name, ns);
                self.apply_changelog(changelog).await?;
            }

            // Add modules
            for mod_spec in &init.modules {
                // Resolve payload from file, raw, or base64 options
                let module = mod_spec
                    .to_module(&self.config.config_dir)
                    .map_err(|e| ProxyError::Io(e.to_string()))?;

                let changelog = ChangeLog::new(
                    ChangeCommand::AddModule,
                    &module.namespace,
                    &module.name,
                    &module,
                );
                self.apply_changelog(changelog).await?;
            }

            // Add services
            for svc in &init.services {
                let changelog =
                    ChangeLog::new(ChangeCommand::AddService, &svc.namespace, &svc.name, svc);
                self.apply_changelog(changelog).await?;
            }

            // Add routes
            for route in &init.routes {
                let changelog = ChangeLog::new(
                    ChangeCommand::AddRoute,
                    &route.namespace,
                    &route.name,
                    route,
                );
                self.apply_changelog(changelog).await?;
            }

            // Add domains
            for dom_spec in &init.domains {
                let mut domain = dom_spec.domain.clone();

                // Load cert from file if specified (relative to config dir)
                if let Some(ref path) = dom_spec.cert_file {
                    let full_path = self.config.config_dir.join(path);
                    domain.cert = std::fs::read_to_string(&full_path).map_err(|e| {
                        ProxyError::Io(format!(
                            "Failed to read cert file '{}': {}",
                            full_path.display(),
                            e
                        ))
                    })?;
                }
                if let Some(ref path) = dom_spec.key_file {
                    let full_path = self.config.config_dir.join(path);
                    domain.key = std::fs::read_to_string(&full_path).map_err(|e| {
                        ProxyError::Io(format!(
                            "Failed to read key file '{}': {}",
                            full_path.display(),
                            e
                        ))
                    })?;
                }

                let changelog = ChangeLog::new(
                    ChangeCommand::AddDomain,
                    &domain.namespace,
                    &domain.name,
                    &domain,
                );
                self.apply_changelog(changelog).await?;
            }

            // Add collections
            for col in &init.collections {
                let changelog =
                    ChangeLog::new(ChangeCommand::AddCollection, &col.namespace, &col.name, col);
                self.apply_changelog(changelog).await?;
            }

            // Add documents
            for doc in &init.documents {
                let changelog =
                    ChangeLog::new(ChangeCommand::AddDocument, &doc.namespace, &doc.id, doc);
                self.apply_changelog(changelog).await?;
            }

            // Add secrets
            for secret in &init.secrets {
                let changelog = ChangeLog::new(
                    ChangeCommand::AddSecret,
                    &secret.namespace,
                    &secret.name,
                    secret,
                );
                self.apply_changelog(changelog).await?;
            }
        }

        Ok(())
    }

    /// Find namespace for a request based on domain patterns
    fn find_namespace_for_host(&self, host: &str) -> Option<Namespace> {
        let domains = self.domains.read();

        // Check domain patterns
        for resolved in domains.iter() {
            for pattern in &resolved.domain.patterns {
                if matches_pattern(pattern, host) {
                    return Some(resolved.namespace.clone());
                }
            }
        }

        // If no domains configured, check if we have only one namespace
        if domains.is_empty() {
            let namespaces = self.store.list_namespaces().ok()?;
            if namespaces.len() == 1 {
                return namespaces.into_iter().next();
            }
        }

        // Fall back to default namespace if not disabled
        if !self.config.disable_default_namespace {
            return self.store.get_namespace("default").ok().flatten();
        }

        None
    }

    /// Handle an incoming proxy request
    pub async fn handle_request(&self, req: Request) -> Response {
        let start = Instant::now();
        let method = req.method().clone();
        let uri = req.uri().clone();
        let path = uri.path();

        // Extract host from request
        let host = req
            .headers()
            .get(header::HOST)
            .and_then(|h| h.to_str().ok())
            .map(|h| h.split(':').next().unwrap_or(h))
            .unwrap_or("localhost");

        // Find namespace
        let namespace = match self.find_namespace_for_host(host) {
            Some(ns) => ns,
            None => {
                debug!("No namespace found for host: {}", host);
                return (StatusCode::NOT_FOUND, "No namespace found").into_response();
            }
        };

        // Find router for namespace
        let router = match self.routers.get(&namespace.name) {
            Some(r) => r,
            None => {
                debug!("No router for namespace: {}", namespace.name);
                return (StatusCode::NOT_FOUND, "No routes configured").into_response();
            }
        };

        // Find matching route
        let (compiled_route, params) = match router.find_route(path, method.as_str()) {
            Some((route, params)) => (route.clone(), params),
            None => {
                debug!("No route matched: {} {}", method, path);
                return (StatusCode::NOT_FOUND, "Route not found").into_response();
            }
        };

        drop(router);

        // Build request context for modules
        let query_params: HashMap<String, String> = uri
            .query()
            .map(|q| {
                url::form_urlencoded::parse(q.as_bytes())
                    .map(|(k, v)| (k.to_string(), v.to_string()))
                    .collect()
            })
            .unwrap_or_default();

        let headers: HashMap<String, String> = req
            .headers()
            .iter()
            .filter_map(|(k, v)| v.to_str().ok().map(|s| (k.to_string(), s.to_string())))
            .collect();

        let body_bytes = match axum::body::to_bytes(req.into_body(), 10 * 1024 * 1024).await {
            Ok(bytes) => Some(bytes.to_vec()),
            Err(_) => None,
        };

        // Get documents from storage for module access
        let documents = self.get_documents_for_namespace(&namespace.name).await;

        let mut req_ctx = RequestContext {
            method: method.to_string(),
            path: path.to_string(),
            query: query_params,
            headers,
            body: body_bytes,
            params,
            route_name: compiled_route.route.name.clone(),
            namespace: namespace.name.clone(),
            service_name: compiled_route.route.service.clone(),
            documents,
        };

        // Execute modules (scope to drop executor lock before any awaits)
        let handler_result = if !compiled_route.modules.is_empty() {
            let executor = self.module_executor.read();
            let module_ctx =
                executor.create_context(&compiled_route.route.modules, &namespace.name);

            // Execute request modifier
            match module_ctx.execute_request_modifier(&req_ctx) {
                Ok(modified) => req_ctx = modified,
                Err(e) => {
                    error!("Request modifier error: {}", e);
                    return (StatusCode::INTERNAL_SERVER_ERROR, "Module error").into_response();
                }
            }

            // If no service, use request handler
            if compiled_route.service.is_none() {
                let result = module_ctx.execute_request_handler(&req_ctx);
                let error_result =
                    if result.is_err() {
                        Some(module_ctx.execute_error_handler(
                            &req_ctx,
                            &result.as_ref().unwrap_err().to_string(),
                        ))
                    } else {
                        None
                    };
                Some((result, error_result))
            } else {
                None
            }
        } else {
            None
        };

        // Handle the request handler result (outside the executor lock scope)
        if let Some((result, error_result)) = handler_result {
            match result {
                Ok(response) => {
                    let mut builder = Response::builder().status(response.status_code);

                    for (key, value) in &response.headers {
                        builder = builder.header(key, value);
                    }

                    let elapsed = start.elapsed();
                    debug!(
                        "{} {} -> {} ({}ms, handler)",
                        method,
                        path,
                        response.status_code,
                        elapsed.as_millis()
                    );

                    // Save any modified documents
                    if !response.documents.is_empty() {
                        self.save_module_documents(&namespace.name, &response.documents)
                            .await;
                    }

                    return builder.body(Body::from(response.body)).unwrap_or_else(|_| {
                        (StatusCode::INTERNAL_SERVER_ERROR, "Response build error").into_response()
                    });
                }
                Err(e) => {
                    error!("Request handler error: {}", e);

                    // Try error handler
                    if let Some(Ok(error_response)) = error_result {
                        let mut builder = Response::builder().status(error_response.status_code);
                        for (key, value) in error_response.headers {
                            builder = builder.header(key, value);
                        }
                        return builder
                            .body(Body::from(error_response.body))
                            .unwrap_or_else(|_| {
                                (StatusCode::INTERNAL_SERVER_ERROR, "Response build error")
                                    .into_response()
                            });
                    }

                    return (StatusCode::INTERNAL_SERVER_ERROR, "Handler error").into_response();
                }
            }
        }

        // Proxy to upstream service
        if let Some(ref service) = compiled_route.service {
            self.proxy_to_upstream(&req_ctx, service, &compiled_route, start)
                .await
        } else {
            (
                StatusCode::NOT_IMPLEMENTED,
                "No service or handler configured",
            )
                .into_response()
        }
    }

    async fn proxy_to_upstream(
        &self,
        req_ctx: &RequestContext,
        service: &Service,
        compiled_route: &CompiledRoute,
        start: Instant,
    ) -> Response {
        // Get upstream URL
        let upstream_url = match service.urls.first() {
            Some(url) => url,
            None => {
                error!("No upstream URLs configured for service: {}", service.name);
                return (StatusCode::BAD_GATEWAY, "No upstream configured").into_response();
            }
        };

        // Build the request path
        let mut target_path = req_ctx.path.clone();
        if compiled_route.route.strip_path {
            // Strip the matched path prefix
            for pattern in &compiled_route.path_patterns {
                if let Some(caps) = pattern.captures(&req_ctx.path) {
                    if let Some(m) = caps.get(0) {
                        target_path = req_ctx.path[m.end()..].to_string();
                        if !target_path.starts_with('/') {
                            target_path = format!("/{}", target_path);
                        }
                        break;
                    }
                }
            }
        }

        // Build query string
        let query_string: String = req_ctx
            .query
            .iter()
            .map(|(k, v)| format!("{}={}", k, v))
            .collect::<Vec<_>>()
            .join("&");

        let full_url = if query_string.is_empty() {
            format!("{}{}", upstream_url.trim_end_matches('/'), target_path)
        } else {
            format!(
                "{}{}?{}",
                upstream_url.trim_end_matches('/'),
                target_path,
                query_string
            )
        };

        // Build upstream request using shared client
        let method = req_ctx.method.parse().unwrap_or(Method::GET);
        let mut request_builder = self.reqwest_client.request(method, &full_url);

        // Copy headers
        for (key, value) in &req_ctx.headers {
            if key.to_lowercase() != "host" || compiled_route.route.preserve_host {
                request_builder = request_builder.header(key, value);
            }
        }

        // Add DGate headers if not hidden
        if !service.hide_dgate_headers {
            request_builder = request_builder
                .header("X-DGate-Route", &compiled_route.route.name)
                .header("X-DGate-Namespace", &compiled_route.namespace.name)
                .header("X-DGate-Service", &service.name);
        }

        // Add body
        if let Some(ref body) = req_ctx.body {
            request_builder = request_builder.body(body.clone());
        }

        // Set timeout
        if let Some(timeout_ms) = service.request_timeout_ms {
            request_builder = request_builder.timeout(Duration::from_millis(timeout_ms));
        }

        // Send request
        match request_builder.send().await {
            Ok(upstream_response) => {
                let status = upstream_response.status();
                let headers = upstream_response.headers().clone();

                // Get response body
                let body = match upstream_response.bytes().await {
                    Ok(bytes) => bytes.to_vec(),
                    Err(e) => {
                        error!("Failed to read upstream response: {}", e);
                        return (StatusCode::BAD_GATEWAY, "Upstream read error").into_response();
                    }
                };

                // Execute response modifier if present
                let final_body = if !compiled_route.modules.is_empty() {
                    let executor = self.module_executor.read();
                    let module_ctx = executor.create_context(
                        &compiled_route.route.modules,
                        &compiled_route.namespace.name,
                    );

                    let res_ctx = ResponseContext {
                        status_code: status.as_u16(),
                        headers: headers
                            .iter()
                            .filter_map(|(k, v)| {
                                v.to_str().ok().map(|s| (k.to_string(), s.to_string()))
                            })
                            .collect(),
                        body: Some(body.clone()),
                    };

                    match module_ctx.execute_response_modifier(req_ctx, &res_ctx) {
                        Ok(modified) => modified.body.unwrap_or(body),
                        Err(e) => {
                            warn!("Response modifier error: {}", e);
                            body
                        }
                    }
                } else {
                    body
                };

                // Build response
                let mut builder = Response::builder().status(status);

                for (key, value) in headers.iter() {
                    if let Ok(v) = value.to_str() {
                        builder = builder.header(key.as_str(), v);
                    }
                }

                // Add global headers
                for (key, value) in &self.config.proxy.global_headers {
                    builder = builder.header(key.as_str(), value.as_str());
                }

                let elapsed = start.elapsed();
                debug!(
                    "{} {} -> {} {} ({}ms)",
                    req_ctx.method,
                    req_ctx.path,
                    full_url,
                    status,
                    elapsed.as_millis()
                );

                builder.body(Body::from(final_body)).unwrap_or_else(|_| {
                    (StatusCode::INTERNAL_SERVER_ERROR, "Response build error").into_response()
                })
            }
            Err(e) => {
                error!("Upstream request failed: {}", e);

                // Try error handler
                if !compiled_route.modules.is_empty() {
                    let executor = self.module_executor.read();
                    let module_ctx = executor.create_context(
                        &compiled_route.route.modules,
                        &compiled_route.namespace.name,
                    );

                    if let Ok(error_response) =
                        module_ctx.execute_error_handler(req_ctx, &e.to_string())
                    {
                        let mut builder = Response::builder().status(error_response.status_code);
                        for (key, value) in error_response.headers {
                            builder = builder.header(key, value);
                        }
                        return builder
                            .body(Body::from(error_response.body))
                            .unwrap_or_else(|_| {
                                (StatusCode::INTERNAL_SERVER_ERROR, "Response build error")
                                    .into_response()
                            });
                    }
                }

                (StatusCode::BAD_GATEWAY, "Upstream error").into_response()
            }
        }
    }

    /// Get all documents for a namespace (for module access)
    ///
    /// Respects collection visibility:
    /// - Loads all documents from collections in the requesting namespace
    /// - Also loads documents from PUBLIC collections in other namespaces
    async fn get_documents_for_namespace(
        &self,
        namespace: &str,
    ) -> std::collections::HashMap<String, serde_json::Value> {
        let mut docs = std::collections::HashMap::new();

        // Get all collections across all namespaces and filter by accessibility
        if let Ok(all_collections) = self.store.list_all_collections() {
            for collection in all_collections.iter() {
                // Check if this collection is accessible from the requesting namespace
                if !collection.is_accessible_from(namespace) {
                    continue;
                }

                if let Ok(documents) = self
                    .store
                    .list_documents(&collection.namespace, &collection.name)
                {
                    for doc in documents {
                        // For collections in other namespaces, prefix with namespace to avoid collisions
                        let key = if collection.namespace == namespace {
                            format!("{}:{}", collection.name, doc.id)
                        } else {
                            // For cross-namespace access, use full path: namespace/collection:id
                            format!("{}/{}:{}", collection.namespace, collection.name, doc.id)
                        };
                        docs.insert(key, doc.data.clone());
                    }
                }
            }
        }

        docs
    }

    /// Save documents that were modified by a module
    ///
    /// Respects collection visibility:
    /// - Modules can only write to collections they have access to
    /// - Private collections: only writable from the same namespace
    /// - Public collections: writable from any namespace
    async fn save_module_documents(
        &self,
        namespace: &str,
        documents: &std::collections::HashMap<String, serde_json::Value>,
    ) {
        for (key, value) in documents {
            // Parse the key - can be "collection:id" or "namespace/collection:id" for cross-namespace
            let (target_namespace, collection, id) = if key.contains('/') {
                // Cross-namespace format: "namespace/collection:id"
                if let Some((ns_col, id)) = key.split_once(':') {
                    if let Some((ns, col)) = ns_col.split_once('/') {
                        (ns.to_string(), col.to_string(), id.to_string())
                    } else {
                        continue;
                    }
                } else {
                    continue;
                }
            } else {
                // Local format: "collection:id"
                if let Some((collection, id)) = key.split_once(':') {
                    (
                        namespace.to_string(),
                        collection.to_string(),
                        id.to_string(),
                    )
                } else {
                    continue;
                }
            };

            // Check visibility before saving
            let can_write =
                if let Ok(Some(col)) = self.store.get_collection(&target_namespace, &collection) {
                    col.is_accessible_from(namespace)
                } else {
                    // Collection doesn't exist - only allow creating in own namespace
                    target_namespace == namespace
                };

            if !can_write {
                warn!(
                    "Module in namespace '{}' tried to write to private collection '{}/{}' - access denied",
                    namespace, target_namespace, collection
                );
                continue;
            }

            let doc = Document {
                id,
                namespace: target_namespace,
                collection,
                data: value.clone(),
                created_at: chrono::Utc::now(),
                updated_at: chrono::Utc::now(),
            };

            if let Err(e) = self.store.set_document(&doc) {
                error!(
                    "Failed to save document {}:{}: {}",
                    doc.collection, doc.id, e
                );
            }
        }
    }
}

/// Proxy errors
#[derive(Debug, thiserror::Error)]
pub enum ProxyError {
    #[error("Storage error: {0}")]
    Storage(String),

    #[error("Not found: {0}")]
    NotFound(String),

    #[error("Deserialization error: {0}")]
    Deserialization(String),

    #[error("Module error: {0}")]
    Module(String),

    #[error("IO error: {0}")]
    Io(String),
}

/// Convert a path pattern to a regex
fn compile_path_pattern(pattern: &str) -> Result<Regex, regex::Error> {
    let mut regex_pattern = String::new();
    regex_pattern.push('^');

    let mut chars = pattern.chars().peekable();
    while let Some(c) = chars.next() {
        match c {
            '*' => {
                if chars.peek() == Some(&'*') {
                    // Greedy match
                    chars.next();
                    regex_pattern.push_str(".*");
                } else {
                    // Single segment match
                    regex_pattern.push_str("[^/]*");
                }
            }
            ':' => {
                // Named parameter
                let mut name = String::new();
                while let Some(&nc) = chars.peek() {
                    if nc.is_alphanumeric() || nc == '_' {
                        name.push(chars.next().unwrap());
                    } else {
                        break;
                    }
                }
                regex_pattern.push_str(&format!("(?P<{}>[^/]+)", name));
            }
            '{' => {
                // Named parameter with braces
                let mut name = String::new();
                while let Some(nc) = chars.next() {
                    if nc == '}' {
                        break;
                    }
                    name.push(nc);
                }
                regex_pattern.push_str(&format!("(?P<{}>[^/]+)", name));
            }
            '/' | '.' | '-' | '_' => {
                regex_pattern.push('\\');
                regex_pattern.push(c);
            }
            _ => {
                regex_pattern.push(c);
            }
        }
    }

    regex_pattern.push('$');
    Regex::new(&regex_pattern)
}

/// Match a domain pattern against a host
fn matches_pattern(pattern: &str, host: &str) -> bool {
    if pattern == "*" {
        return true;
    }

    let pattern_parts: Vec<&str> = pattern.split('.').collect();
    let host_parts: Vec<&str> = host.split('.').collect();

    if pattern_parts.len() != host_parts.len() {
        return false;
    }

    for (p, h) in pattern_parts.iter().zip(host_parts.iter()) {
        if *p != "*" && !p.eq_ignore_ascii_case(h) {
            return false;
        }
    }

    true
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_path_pattern_compilation() {
        let pattern = compile_path_pattern("/api/:version/users/:id").unwrap();
        assert!(pattern.is_match("/api/v1/users/123"));
        assert!(!pattern.is_match("/api/v1/posts/123"));

        let pattern = compile_path_pattern("/static/*").unwrap();
        assert!(pattern.is_match("/static/file.js"));
        assert!(!pattern.is_match("/static/nested/file.js"));

        let pattern = compile_path_pattern("/**").unwrap();
        assert!(pattern.is_match("/anything/goes/here"));
    }

    #[test]
    fn test_domain_matching() {
        assert!(matches_pattern("*.example.com", "api.example.com"));
        assert!(matches_pattern("*.example.com", "www.example.com"));
        assert!(!matches_pattern("*.example.com", "example.com"));
        assert!(matches_pattern("*", "anything.com"));
    }
}
