//! Proxy module for DGate
//!
//! Handles incoming HTTP requests, routes them through modules,
//! and forwards them to upstream services. Supports WebSocket upgrades
//! and gRPC proxying.

use axum::{
    body::Body,
    extract::{FromRequest, Request, WebSocketUpgrade},
    http::{header, Method, StatusCode},
    response::{IntoResponse, Response},
};
use bytes::Bytes;
use dashmap::DashMap;
use futures_util::{SinkExt, StreamExt};
use h2::client;
use http_body_util::StreamBody;
use hyper::body::Frame;
use hyper_util::client::legacy::Client;
use hyper_util::rt::TokioExecutor;
use parking_lot::RwLock;
use regex::Regex;
use std::collections::HashMap;
use std::net::ToSocketAddrs;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::net::TcpStream;
use tokio::sync::mpsc;
use tokio_tungstenite::{connect_async, tungstenite::protocol::Message as WsMessage};
use tracing::{debug, error, info, warn};

use crate::cluster::{ClusterManager, DGateStateMachine};
use crate::config::DGateConfig;
use crate::modules::{ModuleExecutor, RequestContext, ResponseContext};
use crate::resources::*;
use crate::storage::{create_storage, ProxyStore};

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
#[allow(dead_code)] // namespace field reserved for future use
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
    #[allow(dead_code)] // Reserved for low-level HTTP/2 operations
    http_client: Client<
        hyper_rustls::HttpsConnector<hyper_util::client::legacy::connect::HttpConnector>,
        Body,
    >,
    /// Shared reqwest client for upstream requests
    reqwest_client: reqwest::Client,
    /// Cluster manager (None if running in standalone mode)
    cluster: RwLock<Option<Arc<ClusterManager>>>,
    /// Channel receiver for cluster-applied changes
    change_rx: RwLock<Option<mpsc::UnboundedReceiver<ChangeLog>>>,
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

        Arc::new(Self {
            config,
            store,
            module_executor: RwLock::new(ModuleExecutor::new()),
            routers: DashMap::new(),
            domains: RwLock::new(Vec::new()),
            ready: AtomicBool::new(false),
            change_hash: AtomicU64::new(0),
            http_client: client,
            reqwest_client,
            cluster: RwLock::new(None),
            change_rx: RwLock::new(None),
        })
    }

    /// Initialize cluster mode if configured
    pub async fn init_cluster(self: &Arc<Self>) -> anyhow::Result<()> {
        let cluster_config = match &self.config.cluster {
            Some(cfg) if cfg.enabled => cfg.clone(),
            _ => {
                info!("Running in standalone mode (cluster not enabled)");
                return Ok(());
            }
        };

        info!(
            "Initializing cluster mode with node_id={}",
            cluster_config.node_id
        );

        // Create channel for change notifications
        let (change_tx, change_rx) = mpsc::unbounded_channel();
        *self.change_rx.write() = Some(change_rx);

        // Create state machine with our store
        let state_machine = Arc::new(DGateStateMachine::with_change_notifier(
            self.store.clone(),
            change_tx,
        ));

        // Create cluster manager
        let cluster_manager = ClusterManager::new(cluster_config.clone(), state_machine).await?;
        let cluster_manager = Arc::new(cluster_manager);

        // Initialize the cluster
        cluster_manager.initialize().await?;

        *self.cluster.write() = Some(cluster_manager);

        // Start background task to process cluster changes
        let proxy_state = self.clone();
        tokio::spawn(async move {
            proxy_state.process_cluster_changes().await;
        });

        info!("Cluster mode initialized successfully");
        Ok(())
    }

    /// Process changes applied through the cluster
    async fn process_cluster_changes(self: Arc<Self>) {
        let mut rx = match self.change_rx.write().take() {
            Some(rx) => rx,
            None => return,
        };

        while let Some(changelog) = rx.recv().await {
            debug!("Processing cluster-applied change: {:?}", changelog.cmd);

            // Rebuild routers/domains as needed based on the change type
            if let Err(e) = self.handle_cluster_change(&changelog) {
                error!("Failed to process cluster change: {}", e);
            }

            // Update change hash
            self.change_hash.fetch_add(1, Ordering::Relaxed);
        }
    }

    /// Handle a change that was applied through the cluster
    fn handle_cluster_change(&self, changelog: &ChangeLog) -> Result<(), ProxyError> {
        match changelog.cmd {
            ChangeCommand::AddRoute
            | ChangeCommand::DeleteRoute
            | ChangeCommand::AddService
            | ChangeCommand::DeleteService
            | ChangeCommand::AddModule
            | ChangeCommand::DeleteModule => {
                // Rebuild router for the affected namespace
                self.rebuild_router(&changelog.namespace)?;

                // Handle module executor updates for modules
                if matches!(
                    changelog.cmd,
                    ChangeCommand::AddModule | ChangeCommand::DeleteModule
                ) {
                    if changelog.cmd == ChangeCommand::AddModule {
                        if let Ok(module) = serde_json::from_value::<Module>(changelog.item.clone())
                        {
                            let mut executor = self.module_executor.write();
                            if let Err(e) = executor.add_module(&module) {
                                warn!("Failed to add module to executor: {}", e);
                            }
                        }
                    } else {
                        let mut executor = self.module_executor.write();
                        executor.remove_module(&changelog.namespace, &changelog.name);
                    }
                }
            }
            ChangeCommand::AddNamespace => {
                // New namespace - will be populated by routes
            }
            ChangeCommand::DeleteNamespace => {
                self.routers.remove(&changelog.name);
            }
            ChangeCommand::AddDomain | ChangeCommand::DeleteDomain => {
                self.rebuild_domains()?;
            }
            _ => {
                // Other changes (secrets, collections, documents) don't require router/domain updates
            }
        }
        Ok(())
    }

    /// Get the cluster manager (if in cluster mode)
    pub fn cluster(&self) -> Option<Arc<ClusterManager>> {
        self.cluster.read().clone()
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
    /// Apply a change log entry
    ///
    /// In cluster mode, this proposes the change to the Raft cluster.
    /// The change will be applied once it's committed and replicated.
    /// In standalone mode, the change is applied directly.
    pub async fn apply_changelog(&self, changelog: ChangeLog) -> Result<(), ProxyError> {
        // Check if we're in cluster mode - clone the Arc to avoid holding lock across await
        let cluster = self.cluster.read().clone();
        if let Some(cluster) = cluster {
            // In cluster mode, propose the change through Raft
            // The change will be applied via the state machine
            cluster
                .propose(changelog.clone())
                .await
                .map_err(|e| ProxyError::Cluster(e.to_string()))?;

            debug!("Proposed changelog {} to cluster", changelog.id);
            return Ok(());
        }

        // Standalone mode - apply directly
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
        let mut changelogs = self
            .store
            .list_changelogs()
            .map_err(|e| ProxyError::Storage(e.to_string()))?;

        // Sort changelogs by timestamp to ensure proper ordering
        changelogs.sort_by(|a, b| a.timestamp.cmp(&b.timestamp));

        info!("Restoring {} change logs", changelogs.len());

        for changelog in changelogs {
            if let Err(e) = self.process_changelog(&changelog) {
                warn!("Failed to restore changelog {}: {}", changelog.id, e);
            }
        }

        // Ensure default namespace exists if not disabled
        if !self.config.disable_default_namespace
            && self.store.get_namespace("default").ok().flatten().is_none()
        {
            let default_ns = Namespace::default_namespace();
            self.store
                .set_namespace(&default_ns)
                .map_err(|e| ProxyError::Storage(e.to_string()))?;
            self.rebuild_router("default").ok();
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

    /// Check if a request is a WebSocket upgrade request
    fn is_websocket_upgrade(req: &Request) -> bool {
        req.headers()
            .get(header::UPGRADE)
            .and_then(|v| v.to_str().ok())
            .map(|v| v.eq_ignore_ascii_case("websocket"))
            .unwrap_or(false)
    }

    /// Check if a request is a gRPC request
    fn is_grpc_request(req: &Request) -> bool {
        req.headers()
            .get(header::CONTENT_TYPE)
            .and_then(|v| v.to_str().ok())
            .map(|v| v.starts_with("application/grpc"))
            .unwrap_or(false)
    }

    /// Handle an incoming proxy request
    pub async fn handle_request(&self, req: Request) -> Response {
        let start = Instant::now();
        let method = req.method().clone();
        let uri = req.uri().clone();
        let path = uri.path();

        // Check if this is a WebSocket upgrade request or gRPC request
        let is_websocket = Self::is_websocket_upgrade(&req);
        let is_grpc = Self::is_grpc_request(&req);

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

        // Handle WebSocket upgrade if detected
        if is_websocket {
            if let Some(ref service) = compiled_route.service {
                return self
                    .handle_websocket_upgrade(req, service, &compiled_route, start)
                    .await;
            } else {
                return (StatusCode::BAD_GATEWAY, "No upstream service for WebSocket")
                    .into_response();
            }
        }

        // Handle gRPC request if detected
        if is_grpc {
            if let Some(ref service) = compiled_route.service {
                return self
                    .handle_grpc_proxy(req, service, &compiled_route, start)
                    .await;
            } else {
                return (StatusCode::BAD_GATEWAY, "No upstream service for gRPC")
                    .into_response();
            }
        }

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
                let error_result = if let Err(ref e) = result {
                    Some(module_ctx.execute_error_handler(&req_ctx, &e.to_string()))
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

                // Copy headers, but filter out hop-by-hop headers that shouldn't be forwarded
                // and headers that conflict with our new body (we've already consumed the chunked stream)
                for (key, value) in headers.iter() {
                    let key_lower = key.as_str().to_lowercase();
                    // Skip hop-by-hop headers and transfer-related headers
                    if matches!(
                        key_lower.as_str(),
                        "transfer-encoding"
                            | "connection"
                            | "keep-alive"
                            | "proxy-authenticate"
                            | "proxy-authorization"
                            | "te"
                            | "trailer"
                            | "upgrade"
                            | "content-length" // We'll set our own based on final_body
                    ) {
                        continue;
                    }
                    if let Ok(v) = value.to_str() {
                        builder = builder.header(key.as_str(), v);
                    }
                }

                // Set correct content-length for the final body
                builder = builder.header("content-length", final_body.len().to_string());

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

    /// Handle WebSocket upgrade requests
    ///
    /// This method handles the WebSocket upgrade handshake and relays
    /// messages between the client and upstream server.
    async fn handle_websocket_upgrade(
        &self,
        req: Request,
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

        // Build the WebSocket URL for upstream
        let request_path = req.uri().path();
        let mut target_path = request_path.to_string();
        if compiled_route.route.strip_path {
            // Strip the matched path prefix
            for pattern in &compiled_route.path_patterns {
                if let Some(caps) = pattern.captures(request_path) {
                    if let Some(m) = caps.get(0) {
                        target_path = request_path[m.end()..].to_string();
                        if !target_path.starts_with('/') {
                            target_path = format!("/{}", target_path);
                        }
                        break;
                    }
                }
            }
        }

        // Convert HTTP URL to WebSocket URL
        let ws_upstream = upstream_url
            .replace("http://", "ws://")
            .replace("https://", "wss://");
        let upstream_ws_url = format!("{}{}", ws_upstream.trim_end_matches('/'), target_path);

        // Add query string if present
        let full_upstream_url = if let Some(query) = req.uri().query() {
            format!("{}?{}", upstream_ws_url, query)
        } else {
            upstream_ws_url
        };

        debug!(
            "WebSocket upgrade: {} -> {}",
            req.uri().path(),
            full_upstream_url
        );

        // Use axum's WebSocket extractor to handle the upgrade
        let ws_upgrade: WebSocketUpgrade = match WebSocketUpgrade::from_request(req, &()).await {
            Ok(ws) => ws,
            Err(e) => {
                error!("Failed to extract WebSocket upgrade: {}", e);
                return (StatusCode::BAD_REQUEST, "Invalid WebSocket request").into_response();
            }
        };

        // Clone values needed in the async closure
        let upstream_url_clone = full_upstream_url.clone();
        let route_name = compiled_route.route.name.clone();

        // Handle the WebSocket upgrade
        ws_upgrade.on_upgrade(move |client_socket| async move {
            Self::relay_websocket(client_socket, upstream_url_clone, route_name, start).await
        })
    }

    /// Relay WebSocket messages between client and upstream
    async fn relay_websocket(
        client_socket: axum::extract::ws::WebSocket,
        upstream_url: String,
        route_name: String,
        start: Instant,
    ) {
        // Connect to upstream WebSocket server
        let upstream_ws = match connect_async(&upstream_url).await {
            Ok((ws, _)) => ws,
            Err(e) => {
                error!(
                    "Failed to connect to upstream WebSocket {}: {}",
                    upstream_url, e
                );
                return;
            }
        };

        debug!(
            "WebSocket connected to upstream: {} (route: {})",
            upstream_url, route_name
        );

        // Split both connections into sender/receiver halves
        let (mut client_tx, mut client_rx) = client_socket.split();
        let (mut upstream_tx, mut upstream_rx) = upstream_ws.split();

        // Relay messages from client to upstream
        let client_to_upstream = async {
            while let Some(msg) = client_rx.next().await {
                match msg {
                    Ok(axum::extract::ws::Message::Text(text)) => {
                        if upstream_tx
                            .send(WsMessage::Text(text.to_string().into()))
                            .await
                            .is_err()
                        {
                            break;
                        }
                    }
                    Ok(axum::extract::ws::Message::Binary(data)) => {
                        if upstream_tx
                            .send(WsMessage::Binary(data.to_vec().into()))
                            .await
                            .is_err()
                        {
                            break;
                        }
                    }
                    Ok(axum::extract::ws::Message::Ping(data)) => {
                        if upstream_tx
                            .send(WsMessage::Ping(data.to_vec().into()))
                            .await
                            .is_err()
                        {
                            break;
                        }
                    }
                    Ok(axum::extract::ws::Message::Pong(data)) => {
                        if upstream_tx
                            .send(WsMessage::Pong(data.to_vec().into()))
                            .await
                            .is_err()
                        {
                            break;
                        }
                    }
                    Ok(axum::extract::ws::Message::Close(_)) | Err(_) => {
                        let _ = upstream_tx.send(WsMessage::Close(None)).await;
                        break;
                    }
                }
            }
        };

        // Relay messages from upstream to client
        let upstream_to_client = async {
            while let Some(msg) = upstream_rx.next().await {
                match msg {
                    Ok(WsMessage::Text(text)) => {
                        if client_tx
                            .send(axum::extract::ws::Message::Text(text.to_string().into()))
                            .await
                            .is_err()
                        {
                            break;
                        }
                    }
                    Ok(WsMessage::Binary(data)) => {
                        if client_tx
                            .send(axum::extract::ws::Message::Binary(data.to_vec().into()))
                            .await
                            .is_err()
                        {
                            break;
                        }
                    }
                    Ok(WsMessage::Ping(data)) => {
                        if client_tx
                            .send(axum::extract::ws::Message::Ping(data.to_vec().into()))
                            .await
                            .is_err()
                        {
                            break;
                        }
                    }
                    Ok(WsMessage::Pong(data)) => {
                        if client_tx
                            .send(axum::extract::ws::Message::Pong(data.to_vec().into()))
                            .await
                            .is_err()
                        {
                            break;
                        }
                    }
                    Ok(WsMessage::Close(_)) | Err(_) => {
                        let _ = client_tx
                            .send(axum::extract::ws::Message::Close(None))
                            .await;
                        break;
                    }
                    Ok(WsMessage::Frame(_)) => {
                        // Raw frames are handled internally
                    }
                }
            }
        };

        // Run both relay tasks concurrently
        tokio::select! {
            _ = client_to_upstream => {},
            _ = upstream_to_client => {},
        }

        let elapsed = start.elapsed();
        debug!(
            "WebSocket connection closed (route: {}, duration: {}ms)",
            route_name,
            elapsed.as_millis()
        );
    }

    /// Handle gRPC proxy request
    ///
    /// gRPC requires:
    /// - HTTP/2 over h2c (cleartext) or TLS
    /// - Streaming request/response bodies
    /// - Proper trailer forwarding for grpc-status
    async fn handle_grpc_proxy(
        &self,
        req: Request,
        service: &Service,
        compiled_route: &CompiledRoute,
        start: Instant,
    ) -> Response {
        // Get upstream URL
        let upstream_url = match service.urls.first() {
            Some(url) => url.clone(),
            None => {
                error!("No upstream URLs configured for gRPC service: {}", service.name);
                return (StatusCode::BAD_GATEWAY, "No upstream configured").into_response();
            }
        };

        // Parse upstream URL to get host:port
        let parsed = match url::Url::parse(&upstream_url) {
            Ok(u) => u,
            Err(e) => {
                error!("Invalid upstream URL: {}", e);
                return (StatusCode::BAD_GATEWAY, "Invalid upstream URL").into_response();
            }
        };

        let host = parsed.host_str().unwrap_or("127.0.0.1");
        let port = parsed.port().unwrap_or(80);
        let addr = format!("{}:{}", host, port);

        // Resolve address
        let socket_addr = match addr.to_socket_addrs() {
            Ok(mut addrs) => match addrs.next() {
                Some(addr) => addr,
                None => {
                    error!("Could not resolve address: {}", addr);
                    return (StatusCode::BAD_GATEWAY, "Address resolution failed").into_response();
                }
            },
            Err(e) => {
                error!("Address resolution error: {}", e);
                return (StatusCode::BAD_GATEWAY, "Address resolution failed").into_response();
            }
        };

        // Connect to upstream via TCP
        let tcp_stream = match TcpStream::connect(socket_addr).await {
            Ok(s) => s,
            Err(e) => {
                error!("Failed to connect to gRPC upstream {}: {}", addr, e);
                return (StatusCode::BAD_GATEWAY, "Upstream connection failed").into_response();
            }
        };

        // Perform HTTP/2 handshake (h2c)
        let (h2_client, h2_connection) = match client::handshake(tcp_stream).await {
            Ok((c, conn)) => (c, conn),
            Err(e) => {
                error!("HTTP/2 handshake failed: {}", e);
                return (StatusCode::BAD_GATEWAY, "HTTP/2 handshake failed").into_response();
            }
        };

        // Spawn a task to drive the connection
        tokio::spawn(async move {
            if let Err(e) = h2_connection.await {
                warn!("HTTP/2 connection error: {}", e);
            }
        });

        // Wait for the client to be ready
        let mut h2_client = match h2_client.ready().await {
            Ok(c) => c,
            Err(e) => {
                error!("HTTP/2 client not ready: {}", e);
                return (StatusCode::BAD_GATEWAY, "HTTP/2 client error").into_response();
            }
        };

        // Build the request path
        let mut target_path = req.uri().path().to_string();
        if compiled_route.route.strip_path {
            for pattern in &compiled_route.path_patterns {
                if let Some(caps) = pattern.captures(&target_path) {
                    if let Some(m) = caps.get(0) {
                        target_path = target_path[m.end()..].to_string();
                        if !target_path.starts_with('/') {
                            target_path = format!("/{}", target_path);
                        }
                        break;
                    }
                }
            }
        }

        // Build query string
        let query_string = req.uri().query().map(|q| format!("?{}", q)).unwrap_or_default();
        
        // Build full URI with scheme and authority (required for h2 client)
        let full_uri = format!("http://{}{}{}", addr, target_path, query_string);

        // Build HTTP/2 request
        let mut h2_request = hyper::Request::builder()
            .method(req.method().clone())
            .uri(&full_uri)
            .version(hyper::Version::HTTP_2);

        // Copy headers (but update authority)
        for (key, value) in req.headers() {
            let key_str = key.as_str().to_lowercase();
            // Skip pseudo headers and connection-specific headers
            if key_str.starts_with(':')
                || key_str == "host"
                || key_str == "connection"
                || key_str == "keep-alive"
                || key_str == "transfer-encoding"
            {
                continue;
            }
            h2_request = h2_request.header(key, value);
        }

        // Set the authority header for gRPC
        h2_request = h2_request.header("host", &addr);

        // Collect body from the original request
        let body_bytes = match axum::body::to_bytes(req.into_body(), 10 * 1024 * 1024).await {
            Ok(bytes) => bytes,
            Err(e) => {
                error!("Failed to read request body: {}", e);
                return (StatusCode::BAD_REQUEST, "Failed to read body").into_response();
            }
        };

        // Finalize request
        let h2_req = match h2_request.body(()) {
            Ok(r) => r,
            Err(e) => {
                error!("Failed to build HTTP/2 request: {}", e);
                return (StatusCode::INTERNAL_SERVER_ERROR, "Request build error").into_response();
            }
        };

        // Determine if we should end the stream immediately (no body) or send body
        let end_of_stream = body_bytes.is_empty();
        
        // Send the request
        let (response_future, mut send_stream) = match h2_client.send_request(h2_req, end_of_stream) {
            Ok(r) => r,
            Err(e) => {
                error!("Failed to send HTTP/2 request: {}", e);
                return (StatusCode::BAD_GATEWAY, "Request send failed").into_response();
            }
        };

        // Send body data if present
        if !body_bytes.is_empty() {
            // Reserve capacity for the body data
            send_stream.reserve_capacity(body_bytes.len());
            
            // Wait for capacity to be available
            match futures_util::future::poll_fn(|cx| send_stream.poll_capacity(cx)).await {
                Some(Ok(_)) => {}
                Some(Err(e)) => {
                    error!("Failed to reserve capacity: {}", e);
                    return (StatusCode::BAD_GATEWAY, "Capacity error").into_response();
                }
                None => {
                    error!("Stream closed before capacity available");
                    return (StatusCode::BAD_GATEWAY, "Stream closed").into_response();
                }
            }
            
            // Send the body data
            if let Err(e) = send_stream.send_data(body_bytes.clone(), true) {
                error!("Failed to send request body: {}", e);
                return (StatusCode::BAD_GATEWAY, "Body send failed").into_response();
            }
        }

        // Wait for response
        let response = match response_future.await {
            Ok(r) => r,
            Err(e) => {
                error!("Failed to receive HTTP/2 response: {}", e);
                return (StatusCode::BAD_GATEWAY, "Response receive failed").into_response();
            }
        };

        let (parts, mut recv_body) = response.into_parts();

        // Read response body
        let mut response_data = Vec::new();
        while let Some(chunk) = recv_body.data().await {
            match chunk {
                Ok(data) => {
                    // Release flow control capacity
                    let _ = recv_body.flow_control().release_capacity(data.len());
                    response_data.extend_from_slice(&data);
                }
                Err(e) => {
                    error!("Error reading response body: {}", e);
                    return (StatusCode::BAD_GATEWAY, "Response body error").into_response();
                }
            }
        }

        // Get trailers (important for gRPC status)
        let trailers = recv_body.trailers().await.ok().flatten();

        // Build response with proper trailer support for gRPC
        let mut builder = Response::builder().status(parts.status);

        // Copy response headers
        for (key, value) in parts.headers.iter() {
            let key_lower = key.as_str().to_lowercase();
            // Skip some headers but keep gRPC-specific ones
            if matches!(key_lower.as_str(), "transfer-encoding" | "connection") {
                continue;
            }
            builder = builder.header(key, value);
        }

        let elapsed = start.elapsed();
        debug!(
            "gRPC {} -> {} {} ({}ms)",
            target_path,
            addr,
            parts.status,
            elapsed.as_millis()
        );

        // Create a body that includes trailers for gRPC
        // gRPC requires trailers to be sent as HTTP/2 trailing headers
        let body = if let Some(trailers) = trailers {
            // Create a stream that sends data frames and then trailers
            let data_frame = Frame::data(Bytes::from(response_data));
            let trailers_frame = Frame::trailers(trailers);
            
            let frames = vec![
                Ok::<_, std::convert::Infallible>(data_frame),
                Ok(trailers_frame),
            ];
            
            let stream = futures_util::stream::iter(frames);
            let stream_body = StreamBody::new(stream);
            Body::new(stream_body)
        } else {
            Body::from(response_data)
        };

        builder
            .body(body)
            .unwrap_or_else(|_| {
                (StatusCode::INTERNAL_SERVER_ERROR, "Response build error").into_response()
            })
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

    #[error("Cluster error: {0}")]
    Cluster(String),
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
                for nc in chars.by_ref() {
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
