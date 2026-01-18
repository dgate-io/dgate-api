//! Configuration module for DGate
//!
//! Handles loading and parsing configuration from YAML files and environment variables.

use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::path::Path;

/// Main DGate configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DGateConfig {
    #[serde(default = "default_version")]
    pub version: String,

    #[serde(default = "default_log_level")]
    pub log_level: String,

    #[serde(default)]
    pub log_json: bool,

    #[serde(default)]
    pub debug: bool,

    #[serde(default)]
    pub disable_default_namespace: bool,

    #[serde(default)]
    pub disable_metrics: bool,

    #[serde(default)]
    pub tags: Vec<String>,

    #[serde(default)]
    pub storage: StorageConfig,

    #[serde(default)]
    pub proxy: ProxyConfig,

    #[serde(default)]
    pub admin: Option<AdminConfig>,

    #[serde(default)]
    pub test_server: Option<TestServerConfig>,

    /// Directory where the config file is located (for resolving relative paths)
    #[serde(skip)]
    pub config_dir: std::path::PathBuf,
}

fn default_version() -> String {
    "v1".to_string()
}

fn default_log_level() -> String {
    "info".to_string()
}

impl Default for DGateConfig {
    fn default() -> Self {
        Self {
            version: default_version(),
            log_level: default_log_level(),
            log_json: false,
            debug: false,
            disable_default_namespace: false,
            disable_metrics: false,
            tags: Vec::new(),
            storage: StorageConfig::default(),
            proxy: ProxyConfig::default(),
            admin: None,
            test_server: None,
            config_dir: std::env::current_dir().unwrap_or_default(),
        }
    }
}

impl DGateConfig {
    /// Load configuration from a YAML file
    pub fn load_from_file(path: impl AsRef<Path>) -> anyhow::Result<Self> {
        let path = path.as_ref();
        let content = std::fs::read_to_string(path)?;
        let content = Self::expand_env_vars(&content);
        let mut config: Self = serde_yaml::from_str(&content)?;

        // Set the config directory for resolving relative paths
        config.config_dir = path
            .parent()
            .map(|p| p.to_path_buf())
            .unwrap_or_else(|| std::env::current_dir().unwrap_or_default());

        Ok(config)
    }

    /// Expand environment variables in format ${VAR:-default}
    fn expand_env_vars(content: &str) -> String {
        let re = regex::Regex::new(r"\$\{([^}:]+)(?::-([^}]*))?\}").unwrap();
        re.replace_all(content, |caps: &regex::Captures| {
            let var_name = &caps[1];
            let default_value = caps.get(2).map(|m| m.as_str()).unwrap_or("");
            std::env::var(var_name).unwrap_or_else(|_| default_value.to_string())
        })
        .to_string()
    }

    /// Load configuration from path or use defaults
    pub fn load(path: Option<&str>) -> anyhow::Result<Self> {
        match path {
            Some(p) if !p.is_empty() => Self::load_from_file(p),
            _ => {
                // Try common config locations
                let paths = ["config.dgate.yaml", "dgate.yaml", "/etc/dgate/config.yaml"];
                for path in paths {
                    if Path::new(path).exists() {
                        return Self::load_from_file(path);
                    }
                }
                Ok(Self::default())
            }
        }
    }
}

/// Storage configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StorageConfig {
    #[serde(rename = "type", default = "default_storage_type")]
    pub storage_type: StorageType,

    #[serde(default)]
    pub dir: Option<String>,

    #[serde(flatten)]
    pub extra: HashMap<String, serde_json::Value>,
}

fn default_storage_type() -> StorageType {
    StorageType::Memory
}

impl Default for StorageConfig {
    fn default() -> Self {
        Self {
            storage_type: StorageType::Memory,
            dir: None,
            extra: HashMap::new(),
        }
    }
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Default)]
#[serde(rename_all = "lowercase")]
pub enum StorageType {
    #[default]
    Memory,
    File,
}

/// Proxy configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProxyConfig {
    #[serde(default = "default_proxy_host")]
    pub host: String,

    #[serde(default = "default_proxy_port")]
    pub port: u16,

    #[serde(default)]
    pub tls: Option<TlsConfig>,

    #[serde(default)]
    pub enable_h2c: bool,

    #[serde(default)]
    pub enable_http2: bool,

    #[serde(default = "default_console_log_level")]
    pub console_log_level: String,

    #[serde(default)]
    pub redirect_https: Vec<String>,

    #[serde(default)]
    pub allowed_domains: Vec<String>,

    #[serde(default)]
    pub global_headers: HashMap<String, String>,

    #[serde(default)]
    pub strict_mode: bool,

    #[serde(default)]
    pub disable_x_forwarded_headers: bool,

    #[serde(default)]
    pub x_forwarded_for_depth: usize,

    #[serde(default)]
    pub allow_list: Vec<String>,

    #[serde(default)]
    pub client_transport: TransportConfig,

    #[serde(default)]
    pub init_resources: Option<InitResources>,
}

fn default_proxy_host() -> String {
    "0.0.0.0".to_string()
}

fn default_proxy_port() -> u16 {
    80
}

fn default_console_log_level() -> String {
    "info".to_string()
}

impl Default for ProxyConfig {
    fn default() -> Self {
        Self {
            host: default_proxy_host(),
            port: default_proxy_port(),
            tls: None,
            enable_h2c: false,
            enable_http2: false,
            console_log_level: default_console_log_level(),
            redirect_https: Vec::new(),
            allowed_domains: Vec::new(),
            global_headers: HashMap::new(),
            strict_mode: false,
            disable_x_forwarded_headers: false,
            x_forwarded_for_depth: 0,
            allow_list: Vec::new(),
            client_transport: TransportConfig::default(),
            init_resources: None,
        }
    }
}

/// TLS configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TlsConfig {
    #[serde(default = "default_tls_port")]
    pub port: u16,
    pub cert_file: Option<String>,
    pub key_file: Option<String>,
    #[serde(default)]
    pub auto_generate: bool,
}

fn default_tls_port() -> u16 {
    443
}

impl Default for TlsConfig {
    fn default() -> Self {
        Self {
            port: default_tls_port(),
            cert_file: None,
            key_file: None,
            auto_generate: false,
        }
    }
}

/// HTTP transport configuration
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct TransportConfig {
    #[serde(default)]
    pub dns_server: Option<String>,

    #[serde(default)]
    pub dns_timeout_ms: Option<u64>,

    #[serde(default)]
    pub dns_prefer_go: bool,

    #[serde(default)]
    pub max_idle_conns: Option<usize>,

    #[serde(default)]
    pub max_idle_conns_per_host: Option<usize>,

    #[serde(default)]
    pub max_conns_per_host: Option<usize>,

    #[serde(default)]
    pub idle_conn_timeout_ms: Option<u64>,

    #[serde(default)]
    pub disable_compression: bool,

    #[serde(default)]
    pub disable_keep_alives: bool,

    #[serde(default)]
    pub disable_private_ips: bool,
}

/// Admin API configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AdminConfig {
    #[serde(default = "default_admin_host")]
    pub host: String,

    #[serde(default = "default_admin_port")]
    pub port: u16,

    #[serde(default)]
    pub allow_list: Vec<String>,

    #[serde(default)]
    pub x_forwarded_for_depth: usize,

    #[serde(default)]
    pub watch_only: bool,

    #[serde(default)]
    pub tls: Option<TlsConfig>,

    #[serde(default)]
    pub auth_method: AuthMethod,

    #[serde(default)]
    pub basic_auth: Option<BasicAuthConfig>,

    #[serde(default)]
    pub key_auth: Option<KeyAuthConfig>,
}

fn default_admin_host() -> String {
    "0.0.0.0".to_string()
}

fn default_admin_port() -> u16 {
    9080
}

impl Default for AdminConfig {
    fn default() -> Self {
        Self {
            host: default_admin_host(),
            port: default_admin_port(),
            allow_list: Vec::new(),
            x_forwarded_for_depth: 0,
            watch_only: false,
            tls: None,
            auth_method: AuthMethod::None,
            basic_auth: None,
            key_auth: None,
        }
    }
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Default)]
#[serde(rename_all = "lowercase")]
pub enum AuthMethod {
    #[default]
    None,
    Basic,
    Key,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct BasicAuthConfig {
    #[serde(default)]
    pub users: Vec<UserCredentials>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UserCredentials {
    pub username: String,
    pub password: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct KeyAuthConfig {
    pub query_param_name: Option<String>,
    pub header_name: Option<String>,
    #[serde(default)]
    pub keys: Vec<String>,
}

/// Test server configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TestServerConfig {
    #[serde(default = "default_proxy_host")]
    pub host: String,

    #[serde(default = "default_test_port")]
    pub port: u16,

    #[serde(default)]
    pub enable_h2c: bool,

    #[serde(default)]
    pub enable_http2: bool,

    #[serde(default)]
    pub enable_env_vars: bool,

    #[serde(default)]
    pub global_headers: HashMap<String, String>,
}

fn default_test_port() -> u16 {
    8888
}

impl Default for TestServerConfig {
    fn default() -> Self {
        Self {
            host: default_proxy_host(),
            port: default_test_port(),
            enable_h2c: false,
            enable_http2: false,
            enable_env_vars: false,
            global_headers: HashMap::new(),
        }
    }
}

/// Initial resources loaded from config file
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct InitResources {
    #[serde(default)]
    pub skip_validation: bool,

    #[serde(default)]
    pub namespaces: Vec<crate::resources::Namespace>,

    #[serde(default)]
    pub services: Vec<crate::resources::Service>,

    #[serde(default)]
    pub routes: Vec<crate::resources::Route>,

    #[serde(default)]
    pub modules: Vec<ModuleSpec>,

    #[serde(default)]
    pub domains: Vec<DomainSpec>,

    #[serde(default)]
    pub collections: Vec<crate::resources::Collection>,

    #[serde(default)]
    pub documents: Vec<crate::resources::Document>,

    #[serde(default)]
    pub secrets: Vec<crate::resources::Secret>,
}

/// Module specification with optional file loading
///
/// Supports three ways to specify module code:
/// 1. `payload` - base64 encoded module code
/// 2. `payloadRaw` - plain text module code (will be base64 encoded automatically)  
/// 3. `payloadFile` - path to a file containing the module code (relative to config file)
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ModuleSpec {
    pub name: String,
    pub namespace: String,

    /// Base64 encoded payload (original format)
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub payload: Option<String>,

    /// Raw JavaScript/TypeScript code (plain text, will be encoded)
    #[serde(
        default,
        rename = "payloadRaw",
        skip_serializing_if = "Option::is_none"
    )]
    pub payload_raw: Option<String>,

    /// Path to file containing module code (relative to config file location)
    #[serde(
        default,
        rename = "payloadFile",
        skip_serializing_if = "Option::is_none"
    )]
    pub payload_file: Option<String>,

    #[serde(default, rename = "moduleType")]
    pub module_type: crate::resources::ModuleType,

    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub tags: Vec<String>,
}

impl ModuleSpec {
    /// Resolve the module payload from the various input options.
    /// Priority: payload_file > payload_raw > payload
    pub fn resolve_payload(&self, config_dir: &std::path::Path) -> anyhow::Result<String> {
        use base64::Engine;

        // Priority 1: File path (relative to config)
        if let Some(ref file_path) = self.payload_file {
            let full_path = config_dir.join(file_path);
            let content = std::fs::read_to_string(&full_path).map_err(|e| {
                anyhow::anyhow!(
                    "Failed to read module file '{}': {}",
                    full_path.display(),
                    e
                )
            })?;
            return Ok(base64::engine::general_purpose::STANDARD.encode(content));
        }

        // Priority 2: Raw payload (plain text)
        if let Some(ref raw) = self.payload_raw {
            return Ok(base64::engine::general_purpose::STANDARD.encode(raw));
        }

        // Priority 3: Base64 encoded payload
        if let Some(ref payload) = self.payload {
            return Ok(payload.clone());
        }

        // No payload specified
        Err(anyhow::anyhow!(
            "Module '{}' has no payload specified (use payload, payloadRaw, or payloadFile)",
            self.name
        ))
    }

    /// Convert to a Module resource with resolved payload
    pub fn to_module(
        &self,
        config_dir: &std::path::Path,
    ) -> anyhow::Result<crate::resources::Module> {
        Ok(crate::resources::Module {
            name: self.name.clone(),
            namespace: self.namespace.clone(),
            payload: self.resolve_payload(config_dir)?,
            module_type: self.module_type,
            tags: self.tags.clone(),
        })
    }
}

/// Domain specification with optional file loading for certs
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DomainSpec {
    #[serde(flatten)]
    pub domain: crate::resources::Domain,

    #[serde(skip_serializing_if = "Option::is_none")]
    pub cert_file: Option<String>,

    #[serde(skip_serializing_if = "Option::is_none")]
    pub key_file: Option<String>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_env_var_expansion() {
        std::env::set_var("TEST_VAR", "hello");
        let content = "value: ${TEST_VAR:-default}";
        let expanded = DGateConfig::expand_env_vars(content);
        assert_eq!(expanded, "value: hello");

        let content_default = "value: ${NONEXISTENT:-default}";
        let expanded_default = DGateConfig::expand_env_vars(content_default);
        assert_eq!(expanded_default, "value: default");
    }

    #[test]
    fn test_default_config() {
        let config = DGateConfig::default();
        assert_eq!(config.version, "v1");
        assert_eq!(config.proxy.port, 80);
        assert!(!config.debug);
    }
}
