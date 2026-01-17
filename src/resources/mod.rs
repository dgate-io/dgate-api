//! Resource types for DGate
//!
//! This module defines all the core resource types:
//! - Namespace: Organizational unit for resources
//! - Route: Request routing configuration
//! - Service: Upstream service definitions
//! - Module: JavaScript/TypeScript modules for request handling
//! - Domain: Ingress domain configuration
//! - Secret: Sensitive data storage
//! - Collection: Document grouping
//! - Document: KV data storage

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use url::Url;

/// Namespace is a way to organize resources in DGate
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct Namespace {
    pub name: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub tags: Vec<String>,
}

impl Namespace {
    pub fn new(name: impl Into<String>) -> Self {
        Self {
            name: name.into(),
            tags: Vec::new(),
        }
    }

    pub fn default_namespace() -> Self {
        Self {
            name: "default".to_string(),
            tags: vec!["default".to_string()],
        }
    }
}

/// Route defines how requests are handled and forwarded
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Route {
    pub name: String,
    pub namespace: String,
    pub paths: Vec<String>,
    pub methods: Vec<String>,
    #[serde(default)]
    pub strip_path: bool,
    #[serde(default)]
    pub preserve_host: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub service: Option<String>,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub modules: Vec<String>,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub tags: Vec<String>,
}

impl Route {
    pub fn new(name: impl Into<String>, namespace: impl Into<String>) -> Self {
        Self {
            name: name.into(),
            namespace: namespace.into(),
            paths: Vec::new(),
            methods: vec!["*".to_string()],
            strip_path: false,
            preserve_host: false,
            service: None,
            modules: Vec::new(),
            tags: Vec::new(),
        }
    }
}

/// Service represents an upstream service
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Service {
    pub name: String,
    pub namespace: String,
    pub urls: Vec<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub retries: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub retry_timeout_ms: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub connect_timeout_ms: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub request_timeout_ms: Option<u64>,
    #[serde(default)]
    pub tls_skip_verify: bool,
    #[serde(default)]
    pub http2_only: bool,
    #[serde(default)]
    pub hide_dgate_headers: bool,
    #[serde(default)]
    pub disable_query_params: bool,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub tags: Vec<String>,
}

impl Service {
    pub fn new(name: impl Into<String>, namespace: impl Into<String>) -> Self {
        Self {
            name: name.into(),
            namespace: namespace.into(),
            urls: Vec::new(),
            retries: None,
            retry_timeout_ms: None,
            connect_timeout_ms: None,
            request_timeout_ms: None,
            tls_skip_verify: false,
            http2_only: false,
            hide_dgate_headers: false,
            disable_query_params: false,
            tags: Vec::new(),
        }
    }

    /// Parse URLs into actual Url objects
    pub fn parsed_urls(&self) -> Vec<Url> {
        self.urls
            .iter()
            .filter_map(|u| Url::parse(u).ok())
            .collect()
    }
}

/// Module type for script processing
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Default)]
#[serde(rename_all = "lowercase")]
pub enum ModuleType {
    #[default]
    Javascript,
    Typescript,
}

impl ModuleType {
    pub fn as_str(&self) -> &'static str {
        match self {
            ModuleType::Javascript => "javascript",
            ModuleType::Typescript => "typescript",
        }
    }
}

/// Module contains JavaScript/TypeScript code for request processing
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Module {
    pub name: String,
    pub namespace: String,
    /// Base64 encoded payload
    pub payload: String,
    #[serde(default, rename = "moduleType")]
    pub module_type: ModuleType,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub tags: Vec<String>,
}

impl Module {
    pub fn new(name: impl Into<String>, namespace: impl Into<String>) -> Self {
        Self {
            name: name.into(),
            namespace: namespace.into(),
            payload: String::new(),
            module_type: ModuleType::Javascript,
            tags: Vec::new(),
        }
    }

    /// Decode the base64 payload to get the actual script content
    pub fn decode_payload(&self) -> Result<String, base64::DecodeError> {
        use base64::Engine;
        let bytes = base64::engine::general_purpose::STANDARD.decode(&self.payload)?;
        Ok(String::from_utf8_lossy(&bytes).to_string())
    }

    /// Encode a script as base64 payload
    pub fn encode_payload(script: &str) -> String {
        use base64::Engine;
        base64::engine::general_purpose::STANDARD.encode(script)
    }
}

/// Domain controls ingress traffic into namespaces
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Domain {
    pub name: String,
    pub namespace: String,
    pub patterns: Vec<String>,
    #[serde(default)]
    pub priority: i32,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub cert: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub key: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub tags: Vec<String>,
}

impl Domain {
    pub fn new(name: impl Into<String>, namespace: impl Into<String>) -> Self {
        Self {
            name: name.into(),
            namespace: namespace.into(),
            patterns: Vec::new(),
            priority: 0,
            cert: String::new(),
            key: String::new(),
            tags: Vec::new(),
        }
    }
}

/// Secret stores sensitive information
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Secret {
    pub name: String,
    pub namespace: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub data: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub tags: Vec<String>,
    #[serde(default = "Utc::now")]
    pub created_at: DateTime<Utc>,
    #[serde(default = "Utc::now")]
    pub updated_at: DateTime<Utc>,
}

impl Secret {
    pub fn new(name: impl Into<String>, namespace: impl Into<String>) -> Self {
        let now = Utc::now();
        Self {
            name: name.into(),
            namespace: namespace.into(),
            data: String::new(),
            tags: Vec::new(),
            created_at: now,
            updated_at: now,
        }
    }
}

/// Collection groups Documents together
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Collection {
    pub name: String,
    pub namespace: String,
    #[serde(default)]
    pub visibility: CollectionVisibility,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub tags: Vec<String>,
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Default)]
#[serde(rename_all = "lowercase")]
pub enum CollectionVisibility {
    /// Private collections are only accessible by modules in the same namespace
    #[default]
    Private,
    /// Public collections are accessible by modules from any namespace
    Public,
}

impl CollectionVisibility {
    /// Check if this visibility allows access from another namespace
    pub fn is_public(&self) -> bool {
        matches!(self, CollectionVisibility::Public)
    }
}

impl Collection {
    pub fn new(name: impl Into<String>, namespace: impl Into<String>) -> Self {
        Self {
            name: name.into(),
            namespace: namespace.into(),
            visibility: CollectionVisibility::Private,
            tags: Vec::new(),
        }
    }

    /// Check if this collection is accessible from a given namespace.
    ///
    /// - Private collections are only accessible from the same namespace
    /// - Public collections are accessible from any namespace
    pub fn is_accessible_from(&self, requesting_namespace: &str) -> bool {
        match self.visibility {
            CollectionVisibility::Private => self.namespace == requesting_namespace,
            CollectionVisibility::Public => true,
        }
    }
}

/// Document is KV data stored in a collection
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Document {
    pub id: String,
    pub namespace: String,
    pub collection: String,
    /// The document data - stored as JSON value
    pub data: serde_json::Value,
    #[serde(default = "Utc::now")]
    pub created_at: DateTime<Utc>,
    #[serde(default = "Utc::now")]
    pub updated_at: DateTime<Utc>,
}

impl Document {
    pub fn new(
        id: impl Into<String>,
        namespace: impl Into<String>,
        collection: impl Into<String>,
    ) -> Self {
        let now = Utc::now();
        Self {
            id: id.into(),
            namespace: namespace.into(),
            collection: collection.into(),
            data: serde_json::Value::Null,
            created_at: now,
            updated_at: now,
        }
    }
}

/// Change log command types
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "snake_case")]
pub enum ChangeCommand {
    AddNamespace,
    DeleteNamespace,
    AddRoute,
    DeleteRoute,
    AddService,
    DeleteService,
    AddModule,
    DeleteModule,
    AddDomain,
    DeleteDomain,
    AddSecret,
    DeleteSecret,
    AddCollection,
    DeleteCollection,
    AddDocument,
    DeleteDocument,
}

impl ChangeCommand {
    pub fn resource_type(&self) -> ResourceType {
        match self {
            ChangeCommand::AddNamespace | ChangeCommand::DeleteNamespace => ResourceType::Namespace,
            ChangeCommand::AddRoute | ChangeCommand::DeleteRoute => ResourceType::Route,
            ChangeCommand::AddService | ChangeCommand::DeleteService => ResourceType::Service,
            ChangeCommand::AddModule | ChangeCommand::DeleteModule => ResourceType::Module,
            ChangeCommand::AddDomain | ChangeCommand::DeleteDomain => ResourceType::Domain,
            ChangeCommand::AddSecret | ChangeCommand::DeleteSecret => ResourceType::Secret,
            ChangeCommand::AddCollection | ChangeCommand::DeleteCollection => {
                ResourceType::Collection
            }
            ChangeCommand::AddDocument | ChangeCommand::DeleteDocument => ResourceType::Document,
        }
    }

    pub fn is_add(&self) -> bool {
        matches!(
            self,
            ChangeCommand::AddNamespace
                | ChangeCommand::AddRoute
                | ChangeCommand::AddService
                | ChangeCommand::AddModule
                | ChangeCommand::AddDomain
                | ChangeCommand::AddSecret
                | ChangeCommand::AddCollection
                | ChangeCommand::AddDocument
        )
    }
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum ResourceType {
    Namespace,
    Route,
    Service,
    Module,
    Domain,
    Secret,
    Collection,
    Document,
}

/// Change log entry for tracking state changes
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ChangeLog {
    pub id: String,
    pub cmd: ChangeCommand,
    pub namespace: String,
    pub name: String,
    pub item: serde_json::Value,
    #[serde(default = "Utc::now")]
    pub timestamp: DateTime<Utc>,
}

impl ChangeLog {
    pub fn new<T: Serialize>(
        cmd: ChangeCommand,
        namespace: impl Into<String>,
        name: impl Into<String>,
        item: &T,
    ) -> Self {
        Self {
            id: uuid::Uuid::new_v4().to_string(),
            cmd,
            namespace: namespace.into(),
            name: name.into(),
            item: serde_json::to_value(item).unwrap_or(serde_json::Value::Null),
            timestamp: Utc::now(),
        }
    }
}

/// Internal representation of a route with resolved references
#[derive(Debug, Clone)]
pub struct ResolvedRoute {
    pub route: Route,
    pub namespace: Namespace,
    pub service: Option<Service>,
    pub modules: Vec<Module>,
}

/// Internal representation of a domain with resolved namespace
#[derive(Debug, Clone)]
pub struct ResolvedDomain {
    pub domain: Domain,
    pub namespace: Namespace,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_namespace_creation() {
        let ns = Namespace::new("test");
        assert_eq!(ns.name, "test");
        assert!(ns.tags.is_empty());
    }

    #[test]
    fn test_module_payload_encoding() {
        let script = "export function requestHandler(ctx) { return ctx; }";
        let encoded = Module::encode_payload(script);

        let mut module = Module::new("test", "default");
        module.payload = encoded;

        let decoded = module.decode_payload().unwrap();
        assert_eq!(decoded, script);
    }

    #[test]
    fn test_route_creation() {
        let route = Route::new("test-route", "default");
        assert_eq!(route.name, "test-route");
        assert_eq!(route.namespace, "default");
        assert_eq!(route.methods, vec!["*"]);
    }

    #[test]
    fn test_collection_visibility_private() {
        let col = Collection::new("users", "namespace-a");
        
        // Private collection should only be accessible from same namespace
        assert!(col.is_accessible_from("namespace-a"));
        assert!(!col.is_accessible_from("namespace-b"));
        assert!(!col.is_accessible_from("other"));
    }

    #[test]
    fn test_collection_visibility_public() {
        let mut col = Collection::new("shared-data", "namespace-a");
        col.visibility = CollectionVisibility::Public;
        
        // Public collection should be accessible from any namespace
        assert!(col.is_accessible_from("namespace-a"));
        assert!(col.is_accessible_from("namespace-b"));
        assert!(col.is_accessible_from("any-namespace"));
    }

    #[test]
    fn test_collection_visibility_default_is_private() {
        let col = Collection::new("test", "default");
        assert_eq!(col.visibility, CollectionVisibility::Private);
        assert!(!col.visibility.is_public());
    }

    #[test]
    fn test_collection_visibility_is_public() {
        assert!(!CollectionVisibility::Private.is_public());
        assert!(CollectionVisibility::Public.is_public());
    }
}
