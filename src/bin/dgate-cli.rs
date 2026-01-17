//! DGate CLI - Command line interface for DGate Admin API
//!
//! Usage: dgate-cli [OPTIONS] <COMMAND>
//!
//! For more information, see: https://dgate.io/docs/getting-started/dgate-cli

use clap::{Args, Parser, Subcommand};
use colored::Colorize;
use reqwest::header::{HeaderMap, HeaderValue, AUTHORIZATION, CONTENT_TYPE};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::HashMap;
use std::io::{self, Write};
use tabled::{Table, Tabled};

const VERSION: &str = env!("CARGO_PKG_VERSION");

/// DGate CLI - Command line interface for the DGate Admin API
#[derive(Parser)]
#[command(name = "dgate-cli")]
#[command(author = "DGate Team")]
#[command(version = VERSION)]
#[command(about = "Command line interface for the DGate Admin API", long_about = None)]
#[command(propagate_version = true)]
struct Cli {
    /// The URL for the DGate Admin API
    #[arg(long, default_value = "http://localhost:9080", env = "DGATE_ADMIN_API")]
    admin: String,

    /// Basic auth credentials (username:password or just username for prompt)
    #[arg(short, long, env = "DGATE_ADMIN_AUTH")]
    auth: Option<String>,

    /// Follow redirects (useful for raft leader changes)
    #[arg(short, long, default_value_t = false, env = "DGATE_FOLLOW_REDIRECTS")]
    follow: bool,

    /// Enable verbose logging
    #[arg(short = 'V', long, default_value_t = false)]
    verbose: bool,

    /// Output format (table, json, yaml)
    #[arg(short, long, default_value = "table")]
    output: OutputFormat,

    #[command(subcommand)]
    command: Commands,
}

#[derive(Clone, Copy, Default, clap::ValueEnum)]
enum OutputFormat {
    #[default]
    Table,
    Json,
    Yaml,
}

#[derive(Subcommand)]
enum Commands {
    /// Namespace management commands
    #[command(alias = "ns")]
    Namespace(ResourceArgs),

    /// Service management commands
    #[command(alias = "svc")]
    Service(ResourceArgs),

    /// Module management commands
    #[command(alias = "mod")]
    Module(ResourceArgs),

    /// Route management commands
    #[command(alias = "rt")]
    Route(ResourceArgs),

    /// Domain management commands
    #[command(alias = "dom")]
    Domain(ResourceArgs),

    /// Collection management commands
    #[command(alias = "col")]
    Collection(ResourceArgs),

    /// Document management commands
    #[command(alias = "doc")]
    Document(DocumentArgs),

    /// Secret management commands
    #[command(alias = "sec")]
    Secret(ResourceArgs),
}

#[derive(Args)]
struct ResourceArgs {
    #[command(subcommand)]
    action: ResourceAction,
}

#[derive(Subcommand)]
enum ResourceAction {
    /// Create a resource
    #[command(alias = "mk")]
    Create {
        /// Key=value pairs for resource properties
        #[arg(num_args = 1..)]
        props: Vec<String>,
    },

    /// Delete a resource
    #[command(alias = "rm")]
    Delete {
        /// Resource name to delete
        name: String,
        /// Namespace (required for namespaced resources)
        #[arg(short, long)]
        namespace: Option<String>,
    },

    /// List resources
    #[command(alias = "ls")]
    List {
        /// Namespace to filter by
        #[arg(short, long)]
        namespace: Option<String>,
    },

    /// Get a specific resource
    Get {
        /// Resource name
        name: String,
        /// Namespace (required for namespaced resources)
        #[arg(short, long)]
        namespace: Option<String>,
    },
}

#[derive(Args)]
struct DocumentArgs {
    #[command(subcommand)]
    action: DocumentAction,
}

#[derive(Subcommand)]
enum DocumentAction {
    /// Create a document
    #[command(alias = "mk")]
    Create {
        /// Key=value pairs for document properties (namespace, collection, id, data)
        #[arg(num_args = 1..)]
        props: Vec<String>,
    },

    /// Delete a document
    #[command(alias = "rm")]
    Delete {
        /// Document ID
        id: String,
        /// Collection name
        #[arg(short, long)]
        collection: String,
        /// Namespace
        #[arg(short, long, default_value = "default")]
        namespace: String,
    },

    /// List documents in a collection
    #[command(alias = "ls")]
    List {
        /// Collection name
        #[arg(short, long)]
        collection: String,
        /// Namespace
        #[arg(short, long, default_value = "default")]
        namespace: String,
    },

    /// Get a specific document
    Get {
        /// Document ID
        id: String,
        /// Collection name
        #[arg(short, long)]
        collection: String,
        /// Namespace
        #[arg(short, long, default_value = "default")]
        namespace: String,
    },
}

/// API response wrapper
#[derive(Debug, Deserialize)]
struct ApiResponse<T> {
    success: bool,
    data: Option<T>,
    error: Option<String>,
}

/// HTTP client for the Admin API
struct AdminClient {
    base_url: String,
    client: reqwest::Client,
    auth: Option<String>,
    verbose: bool,
}

impl AdminClient {
    fn new(base_url: &str, auth: Option<String>, follow: bool, verbose: bool) -> Self {
        let client = reqwest::Client::builder()
            .redirect(if follow {
                reqwest::redirect::Policy::limited(10)
            } else {
                reqwest::redirect::Policy::none()
            })
            .build()
            .expect("Failed to create HTTP client");

        Self {
            base_url: base_url.trim_end_matches('/').to_string(),
            client,
            auth,
            verbose,
        }
    }

    fn headers(&self) -> HeaderMap {
        let mut headers = HeaderMap::new();
        headers.insert(CONTENT_TYPE, HeaderValue::from_static("application/json"));

        if let Some(ref auth) = self.auth {
            // Check if it's username:password or just username
            let encoded = if auth.contains(':') {
                use base64::Engine;
                base64::engine::general_purpose::STANDARD.encode(auth)
            } else {
                // Prompt for password
                let password = rpassword_prompt(&format!("Password for {}: ", auth));
                use base64::Engine;
                base64::engine::general_purpose::STANDARD.encode(format!("{}:{}", auth, password))
            };
            headers.insert(
                AUTHORIZATION,
                HeaderValue::from_str(&format!("Basic {}", encoded)).unwrap(),
            );
        }

        headers
    }

    async fn get(&self, path: &str) -> Result<Value, String> {
        let url = format!("{}{}", self.base_url, path);
        if self.verbose {
            eprintln!("{} GET {}", "→".blue(), url);
        }

        let response = self
            .client
            .get(&url)
            .headers(self.headers())
            .send()
            .await
            .map_err(|e| format!("Request failed: {}", e))?;

        self.handle_response(response).await
    }

    async fn put(&self, path: &str, body: Value) -> Result<Value, String> {
        let url = format!("{}{}", self.base_url, path);
        if self.verbose {
            eprintln!("{} PUT {}", "→".blue(), url);
            eprintln!("{} {}", "→".blue(), serde_json::to_string_pretty(&body).unwrap());
        }

        let response = self
            .client
            .put(&url)
            .headers(self.headers())
            .json(&body)
            .send()
            .await
            .map_err(|e| format!("Request failed: {}", e))?;

        self.handle_response(response).await
    }

    async fn delete(&self, path: &str) -> Result<Value, String> {
        let url = format!("{}{}", self.base_url, path);
        if self.verbose {
            eprintln!("{} DELETE {}", "→".blue(), url);
        }

        let response = self
            .client
            .delete(&url)
            .headers(self.headers())
            .send()
            .await
            .map_err(|e| format!("Request failed: {}", e))?;

        self.handle_response(response).await
    }

    async fn handle_response(&self, response: reqwest::Response) -> Result<Value, String> {
        let status = response.status();
        let body = response
            .text()
            .await
            .map_err(|e| format!("Failed to read response: {}", e))?;

        if self.verbose {
            eprintln!("{} {} {}", "←".green(), status.as_u16(), body);
        }

        if status.is_success() {
            serde_json::from_str(&body).map_err(|e| format!("Invalid JSON response: {}", e))
        } else {
            // Try to parse error message from response
            if let Ok(json) = serde_json::from_str::<Value>(&body) {
                if let Some(error) = json.get("error").and_then(|e| e.as_str()) {
                    return Err(error.to_string());
                }
            }
            Err(format!("Request failed with status {}: {}", status.as_u16(), body))
        }
    }
}

/// Parse key=value pairs into a JSON object
fn parse_props(props: &[String]) -> Result<Value, String> {
    let mut map = serde_json::Map::new();

    for prop in props {
        let parts: Vec<&str> = prop.splitn(2, '=').collect();
        if parts.len() != 2 {
            return Err(format!("Invalid property format: '{}'. Expected key=value", prop));
        }

        let key = parts[0];
        let value = parts[1];

        // Try to parse as JSON first (for arrays, objects, numbers, booleans)
        let json_value = if value.starts_with('[') || value.starts_with('{') {
            serde_json::from_str(value)
                .map_err(|e| format!("Invalid JSON value for '{}': {}", key, e))?
        } else if value == "true" {
            Value::Bool(true)
        } else if value == "false" {
            Value::Bool(false)
        } else if let Ok(n) = value.parse::<i64>() {
            Value::Number(n.into())
        } else if let Ok(n) = value.parse::<f64>() {
            serde_json::Number::from_f64(n)
                .map(Value::Number)
                .unwrap_or_else(|| Value::String(value.to_string()))
        } else {
            Value::String(value.to_string())
        };

        // Handle special syntax for JSON arrays: key:='["a","b"]'
        if key.ends_with(":") {
            let actual_key = key.trim_end_matches(':');
            map.insert(actual_key.to_string(), json_value);
        } else {
            map.insert(key.to_string(), json_value);
        }
    }

    Ok(Value::Object(map))
}

/// Simple table row for resources
#[derive(Tabled)]
struct ResourceRow {
    name: String,
    namespace: String,
    #[tabled(rename = "details")]
    details: String,
}

/// Print output in the requested format
fn print_output(data: &Value, format: OutputFormat) {
    match format {
        OutputFormat::Json => {
            println!("{}", serde_json::to_string_pretty(data).unwrap());
        }
        OutputFormat::Yaml => {
            println!("{}", serde_yaml::to_string(data).unwrap());
        }
        OutputFormat::Table => {
            if let Some(arr) = data.as_array() {
                if arr.is_empty() {
                    println!("{}", "No resources found".yellow());
                    return;
                }

                let rows: Vec<ResourceRow> = arr
                    .iter()
                    .map(|item| {
                        let name = item
                            .get("name")
                            .or_else(|| item.get("id"))
                            .and_then(|v| v.as_str())
                            .unwrap_or("-")
                            .to_string();
                        let namespace = item
                            .get("namespace")
                            .and_then(|v| v.as_str())
                            .unwrap_or("-")
                            .to_string();
                        
                        // Build details from other fields
                        let mut details = Vec::new();
                        if let Some(urls) = item.get("urls").and_then(|v| v.as_array()) {
                            details.push(format!("urls: {}", urls.len()));
                        }
                        if let Some(paths) = item.get("paths").and_then(|v| v.as_array()) {
                            let paths_str: Vec<&str> = paths.iter().filter_map(|p| p.as_str()).collect();
                            details.push(format!("paths: [{}]", paths_str.join(", ")));
                        }
                        if let Some(patterns) = item.get("patterns").and_then(|v| v.as_array()) {
                            let patterns_str: Vec<&str> = patterns.iter().filter_map(|p| p.as_str()).collect();
                            details.push(format!("patterns: [{}]", patterns_str.join(", ")));
                        }
                        if let Some(tags) = item.get("tags").and_then(|v| v.as_array()) {
                            if !tags.is_empty() {
                                let tags_str: Vec<&str> = tags.iter().filter_map(|t| t.as_str()).collect();
                                details.push(format!("tags: [{}]", tags_str.join(", ")));
                            }
                        }
                        if let Some(visibility) = item.get("visibility").and_then(|v| v.as_str()) {
                            details.push(format!("visibility: {}", visibility));
                        }

                        ResourceRow {
                            name,
                            namespace,
                            details: if details.is_empty() {
                                "-".to_string()
                            } else {
                                details.join(", ")
                            },
                        }
                    })
                    .collect();

                let table = Table::new(rows).to_string();
                println!("{}", table);
            } else if data.is_object() {
                // Single resource - print as YAML-like format
                println!("{}", serde_yaml::to_string(data).unwrap());
            } else {
                println!("{}", data);
            }
        }
    }
}

fn rpassword_prompt(prompt: &str) -> String {
    eprint!("{}", prompt);
    io::stderr().flush().unwrap();
    
    // Simple password reading (for production, use rpassword crate)
    let mut password = String::new();
    io::stdin().read_line(&mut password).unwrap();
    password.trim().to_string()
}

#[tokio::main]
async fn main() {
    let cli = Cli::parse();
    let client = AdminClient::new(&cli.admin, cli.auth.clone(), cli.follow, cli.verbose);

    let result = match &cli.command {
        Commands::Namespace(args) => handle_resource(&client, "namespace", &args.action, cli.output).await,
        Commands::Service(args) => handle_resource(&client, "service", &args.action, cli.output).await,
        Commands::Module(args) => handle_resource(&client, "module", &args.action, cli.output).await,
        Commands::Route(args) => handle_resource(&client, "route", &args.action, cli.output).await,
        Commands::Domain(args) => handle_resource(&client, "domain", &args.action, cli.output).await,
        Commands::Collection(args) => handle_resource(&client, "collection", &args.action, cli.output).await,
        Commands::Secret(args) => handle_resource(&client, "secret", &args.action, cli.output).await,
        Commands::Document(args) => handle_document(&client, &args.action, cli.output).await,
    };

    if let Err(e) = result {
        eprintln!("{} {}", "Error:".red().bold(), e);
        std::process::exit(1);
    }
}

async fn handle_resource(
    client: &AdminClient,
    resource_type: &str,
    action: &ResourceAction,
    output: OutputFormat,
) -> Result<(), String> {
    // Namespace is the only non-namespaced resource
    let is_namespace = resource_type == "namespace";

    match action {
        ResourceAction::Create { props } => {
            let body = parse_props(props)?;
            
            // Get name from body
            let name = body.get("name")
                .and_then(|v| v.as_str())
                .ok_or("name is required")?
                .to_string();
            
            // Build path based on resource type
            let path = if is_namespace {
                format!("/api/v1/{}/{}", resource_type, name)
            } else {
                let namespace = body.get("namespace")
                    .and_then(|v| v.as_str())
                    .unwrap_or("default");
                format!("/api/v1/{}/{}/{}", resource_type, namespace, name)
            };
            
            let result = client.put(&path, body).await?;
            
            if let Some(data) = result.get("data") {
                println!("{} {} created successfully", resource_type.green(), name);
                print_output(data, output);
            } else {
                println!("{} {} created successfully", resource_type.green(), name);
            }
            Ok(())
        }
        ResourceAction::Delete { name, namespace } => {
            let path = if is_namespace {
                format!("/api/v1/{}/{}", resource_type, name)
            } else {
                let ns = namespace.as_deref().unwrap_or("default");
                format!("/api/v1/{}/{}/{}", resource_type, ns, name)
            };
            
            client.delete(&path).await?;
            println!("{} {} deleted successfully", resource_type.green(), name);
            Ok(())
        }
        ResourceAction::List { namespace } => {
            let path = if let Some(ns) = namespace {
                format!("/api/v1/{}?namespace={}", resource_type, ns)
            } else {
                format!("/api/v1/{}", resource_type)
            };
            
            let result = client.get(&path).await?;
            if let Some(data) = result.get("data") {
                print_output(data, output);
            }
            Ok(())
        }
        ResourceAction::Get { name, namespace } => {
            let path = if is_namespace {
                format!("/api/v1/{}/{}", resource_type, name)
            } else {
                let ns = namespace.as_deref().unwrap_or("default");
                format!("/api/v1/{}/{}/{}", resource_type, ns, name)
            };
            
            let result = client.get(&path).await?;
            if let Some(data) = result.get("data") {
                print_output(data, output);
            }
            Ok(())
        }
    }
}

async fn handle_document(
    client: &AdminClient,
    action: &DocumentAction,
    output: OutputFormat,
) -> Result<(), String> {
    match action {
        DocumentAction::Create { props } => {
            let body = parse_props(props)?;
            
            let namespace = body.get("namespace")
                .and_then(|v| v.as_str())
                .unwrap_or("default");
            let collection = body.get("collection")
                .and_then(|v| v.as_str())
                .ok_or("collection is required")?;
            let id = body.get("id")
                .and_then(|v| v.as_str())
                .ok_or("id is required")?;
            
            let path = format!("/api/v1/collection/{}/{}/{}", namespace, collection, id);
            
            // Get the data field or use the whole body
            let data = body.get("data").cloned().unwrap_or(body.clone());
            
            let result = client.put(&path, data).await?;
            println!("{} document created successfully", "Document".green());
            if let Some(data) = result.get("data") {
                print_output(data, output);
            }
            Ok(())
        }
        DocumentAction::Delete { id, collection, namespace } => {
            let path = format!("/api/v1/collection/{}/{}/{}", namespace, collection, id);
            client.delete(&path).await?;
            println!("{} {} deleted successfully", "Document".green(), id);
            Ok(())
        }
        DocumentAction::List { collection, namespace } => {
            let path = format!("/api/v1/collection/{}/{}", namespace, collection);
            let result = client.get(&path).await?;
            if let Some(data) = result.get("data") {
                print_output(data, output);
            }
            Ok(())
        }
        DocumentAction::Get { id, collection, namespace } => {
            let path = format!("/api/v1/collection/{}/{}/{}", namespace, collection, id);
            let result = client.get(&path).await?;
            if let Some(data) = result.get("data") {
                print_output(data, output);
            }
            Ok(())
        }
    }
}
