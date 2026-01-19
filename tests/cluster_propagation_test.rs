//! Functional tests for cluster resource propagation
//!
//! These tests verify that all DGate resources are properly propagated
//! through the cluster state machine and notifications are sent correctly.

use std::sync::Arc;
use std::time::Duration;

use tokio::sync::mpsc;
use tokio::time::timeout;

// Import from the dgate crate
use dgate::cluster::ClusterManager;
use dgate::config::{ClusterConfig, ClusterMember, ClusterMode, StorageConfig, StorageType};
use dgate::resources::{
    ChangeCommand, ChangeLog, Collection, CollectionVisibility, Document, Domain, Module,
    ModuleType, Namespace, Route, Secret, Service,
};
use dgate::storage::{create_storage, ProxyStore};

/// Helper to create a test storage
fn create_test_storage() -> Arc<ProxyStore> {
    let config = StorageConfig {
        storage_type: StorageType::Memory,
        dir: None,
        extra: Default::default(),
    };
    Arc::new(ProxyStore::new(create_storage(&config)))
}

/// Helper to create a test cluster config
fn create_test_cluster_config(node_id: u64) -> ClusterConfig {
    ClusterConfig {
        enabled: true,
        mode: ClusterMode::default(),
        node_id,
        advertise_addr: format!("127.0.0.1:{}", 9090 + node_id),
        bootstrap: true,
        initial_members: vec![ClusterMember {
            id: node_id,
            addr: format!("127.0.0.1:{}", 9090 + node_id),
            admin_port: None,
            tls: false,
        }],
        discovery: None,
        tempo: None,
    }
}

/// Test that namespace changes are propagated through the state machine
#[tokio::test]
async fn test_namespace_propagation() {
    let store = create_test_storage();
    let (tx, mut rx) = mpsc::unbounded_channel();

    let config = create_test_cluster_config(1);

    let cluster = ClusterManager::new(config, store.clone(), tx)
        .await
        .expect("Failed to create cluster manager");
    cluster
        .initialize()
        .await
        .expect("Failed to initialize cluster");

    // Create a namespace
    let namespace = Namespace {
        name: "test-namespace".to_string(),
        tags: vec!["test".to_string()],
    };

    let changelog = ChangeLog::new(
        ChangeCommand::AddNamespace,
        &namespace.name,
        &namespace.name,
        &namespace,
    );

    // Propose the change
    let response = cluster.propose(changelog).await.expect("Failed to propose");
    assert!(response.success, "Proposal should succeed");

    // Verify the change was received through the notification channel
    let received = timeout(Duration::from_secs(1), rx.recv())
        .await
        .expect("Timeout waiting for notification")
        .expect("Should receive notification");

    assert_eq!(received.cmd, ChangeCommand::AddNamespace);
    assert_eq!(received.name, "test-namespace");

    // Verify the namespace exists in storage
    let stored = store
        .get_namespace("test-namespace")
        .expect("Storage error")
        .expect("Namespace should exist");
    assert_eq!(stored.name, "test-namespace");
}

/// Test that route changes are propagated
#[tokio::test]
async fn test_route_propagation() {
    let store = create_test_storage();
    let (tx, mut rx) = mpsc::unbounded_channel();

    let config = create_test_cluster_config(1);

    let cluster = ClusterManager::new(config, store.clone(), tx)
        .await
        .expect("Failed to create cluster manager");
    cluster
        .initialize()
        .await
        .expect("Failed to initialize cluster");

    // First create a namespace
    let namespace = Namespace::new("default");
    let ns_changelog = ChangeLog::new(
        ChangeCommand::AddNamespace,
        &namespace.name,
        &namespace.name,
        &namespace,
    );
    cluster
        .propose(ns_changelog)
        .await
        .expect("Failed to propose namespace");
    let _ = rx.recv().await; // Consume namespace notification

    // Create a route
    let route = Route {
        name: "test-route".to_string(),
        namespace: "default".to_string(),
        paths: vec!["/api/**".to_string()],
        methods: vec!["GET".to_string(), "POST".to_string()],
        service: Some("test-service".to_string()),
        modules: vec![],
        strip_path: true,
        preserve_host: false,
        tags: vec![],
    };

    let changelog = ChangeLog::new(
        ChangeCommand::AddRoute,
        &route.namespace,
        &route.name,
        &route,
    );

    let response = cluster.propose(changelog).await.expect("Failed to propose");
    assert!(response.success);

    // Verify notification
    let received = timeout(Duration::from_secs(1), rx.recv())
        .await
        .expect("Timeout")
        .expect("Should receive notification");

    assert_eq!(received.cmd, ChangeCommand::AddRoute);
    assert_eq!(received.name, "test-route");

    // Verify in storage
    let stored = store
        .get_route("default", "test-route")
        .expect("Storage error")
        .expect("Route should exist");
    assert_eq!(stored.paths, vec!["/api/**"]);
}

/// Test that service changes are propagated
#[tokio::test]
async fn test_service_propagation() {
    let store = create_test_storage();
    let (tx, mut rx) = mpsc::unbounded_channel();

    let config = create_test_cluster_config(1);

    let cluster = ClusterManager::new(config, store.clone(), tx)
        .await
        .expect("Failed to create cluster manager");
    cluster
        .initialize()
        .await
        .expect("Failed to initialize cluster");

    // Create namespace first
    let namespace = Namespace::new("default");
    let ns_changelog = ChangeLog::new(
        ChangeCommand::AddNamespace,
        "default",
        "default",
        &namespace,
    );
    cluster.propose(ns_changelog).await.unwrap();
    let _ = rx.recv().await;

    // Create a service
    let service = Service {
        name: "test-service".to_string(),
        namespace: "default".to_string(),
        urls: vec!["http://backend:8080".to_string()],
        request_timeout_ms: Some(5000),
        retries: Some(3),
        retry_timeout_ms: Some(1000),
        connect_timeout_ms: None,
        tls_skip_verify: false,
        http2_only: false,
        hide_dgate_headers: false,
        disable_query_params: false,
        tags: vec![],
    };

    let changelog = ChangeLog::new(
        ChangeCommand::AddService,
        &service.namespace,
        &service.name,
        &service,
    );

    let response = cluster.propose(changelog).await.expect("Failed to propose");
    assert!(response.success);

    let received = timeout(Duration::from_secs(1), rx.recv())
        .await
        .expect("Timeout")
        .expect("Should receive notification");

    assert_eq!(received.cmd, ChangeCommand::AddService);

    let stored = store
        .get_service("default", "test-service")
        .expect("Storage error")
        .expect("Service should exist");
    assert_eq!(stored.urls, vec!["http://backend:8080"]);
}

/// Test that module changes are propagated
#[tokio::test]
async fn test_module_propagation() {
    let store = create_test_storage();
    let (tx, mut rx) = mpsc::unbounded_channel();

    let config = create_test_cluster_config(1);

    let cluster = ClusterManager::new(config, store.clone(), tx)
        .await
        .expect("Failed to create cluster manager");
    cluster
        .initialize()
        .await
        .expect("Failed to initialize cluster");

    // Create namespace
    let namespace = Namespace::new("default");
    let ns_changelog = ChangeLog::new(
        ChangeCommand::AddNamespace,
        "default",
        "default",
        &namespace,
    );
    cluster.propose(ns_changelog).await.unwrap();
    let _ = rx.recv().await;

    // Create a module
    let module = Module {
        name: "test-module".to_string(),
        namespace: "default".to_string(),
        payload: base64::Engine::encode(
            &base64::engine::general_purpose::STANDARD,
            "function requestHandler(ctx) { ctx.json({ok: true}); }",
        ),
        module_type: ModuleType::Javascript,
        tags: vec![],
    };

    let changelog = ChangeLog::new(
        ChangeCommand::AddModule,
        &module.namespace,
        &module.name,
        &module,
    );

    let response = cluster.propose(changelog).await.expect("Failed to propose");
    assert!(response.success);

    let received = timeout(Duration::from_secs(1), rx.recv())
        .await
        .expect("Timeout")
        .expect("Should receive notification");

    assert_eq!(received.cmd, ChangeCommand::AddModule);

    let stored = store
        .get_module("default", "test-module")
        .expect("Storage error")
        .expect("Module should exist");
    assert_eq!(stored.module_type, ModuleType::Javascript);
}

/// Test that domain changes are propagated
#[tokio::test]
async fn test_domain_propagation() {
    let store = create_test_storage();
    let (tx, mut rx) = mpsc::unbounded_channel();

    let config = create_test_cluster_config(1);

    let cluster = ClusterManager::new(config, store.clone(), tx)
        .await
        .expect("Failed to create cluster manager");
    cluster
        .initialize()
        .await
        .expect("Failed to initialize cluster");

    // Create namespace
    let namespace = Namespace::new("default");
    let ns_changelog = ChangeLog::new(
        ChangeCommand::AddNamespace,
        "default",
        "default",
        &namespace,
    );
    cluster.propose(ns_changelog).await.unwrap();
    let _ = rx.recv().await;

    // Create a domain
    let domain = Domain {
        name: "test-domain".to_string(),
        namespace: "default".to_string(),
        patterns: vec!["*.example.com".to_string()],
        priority: 100,
        cert: String::new(),
        key: String::new(),
        tags: vec![],
    };

    let changelog = ChangeLog::new(
        ChangeCommand::AddDomain,
        &domain.namespace,
        &domain.name,
        &domain,
    );

    let response = cluster.propose(changelog).await.expect("Failed to propose");
    assert!(response.success);

    let received = timeout(Duration::from_secs(1), rx.recv())
        .await
        .expect("Timeout")
        .expect("Should receive notification");

    assert_eq!(received.cmd, ChangeCommand::AddDomain);

    let stored = store
        .get_domain("default", "test-domain")
        .expect("Storage error")
        .expect("Domain should exist");
    assert_eq!(stored.patterns, vec!["*.example.com"]);
}

/// Test that secret changes are propagated
#[tokio::test]
async fn test_secret_propagation() {
    let store = create_test_storage();
    let (tx, mut rx) = mpsc::unbounded_channel();

    let config = create_test_cluster_config(1);

    let cluster = ClusterManager::new(config, store.clone(), tx)
        .await
        .expect("Failed to create cluster manager");
    cluster
        .initialize()
        .await
        .expect("Failed to initialize cluster");

    // Create namespace
    let namespace = Namespace::new("default");
    let ns_changelog = ChangeLog::new(
        ChangeCommand::AddNamespace,
        "default",
        "default",
        &namespace,
    );
    cluster.propose(ns_changelog).await.unwrap();
    let _ = rx.recv().await;

    // Create a secret
    let secret = Secret {
        name: "test-secret".to_string(),
        namespace: "default".to_string(),
        data: "secret-value".to_string(),
        tags: vec![],
        created_at: chrono::Utc::now(),
        updated_at: chrono::Utc::now(),
    };

    let changelog = ChangeLog::new(
        ChangeCommand::AddSecret,
        &secret.namespace,
        &secret.name,
        &secret,
    );

    let response = cluster.propose(changelog).await.expect("Failed to propose");
    assert!(response.success);

    let received = timeout(Duration::from_secs(1), rx.recv())
        .await
        .expect("Timeout")
        .expect("Should receive notification");

    assert_eq!(received.cmd, ChangeCommand::AddSecret);

    let stored = store
        .get_secret("default", "test-secret")
        .expect("Storage error")
        .expect("Secret should exist");
    assert_eq!(stored.data, "secret-value");
}

/// Test that collection changes are propagated
#[tokio::test]
async fn test_collection_propagation() {
    let store = create_test_storage();
    let (tx, mut rx) = mpsc::unbounded_channel();

    let config = create_test_cluster_config(1);

    let cluster = ClusterManager::new(config, store.clone(), tx)
        .await
        .expect("Failed to create cluster manager");
    cluster
        .initialize()
        .await
        .expect("Failed to initialize cluster");

    // Create namespace
    let namespace = Namespace::new("default");
    let ns_changelog = ChangeLog::new(
        ChangeCommand::AddNamespace,
        "default",
        "default",
        &namespace,
    );
    cluster.propose(ns_changelog).await.unwrap();
    let _ = rx.recv().await;

    // Create a collection
    let collection = Collection {
        name: "test-collection".to_string(),
        namespace: "default".to_string(),
        visibility: CollectionVisibility::Private,
        tags: vec![],
    };

    let changelog = ChangeLog::new(
        ChangeCommand::AddCollection,
        &collection.namespace,
        &collection.name,
        &collection,
    );

    let response = cluster.propose(changelog).await.expect("Failed to propose");
    assert!(response.success);

    let received = timeout(Duration::from_secs(1), rx.recv())
        .await
        .expect("Timeout")
        .expect("Should receive notification");

    assert_eq!(received.cmd, ChangeCommand::AddCollection);

    let stored = store
        .get_collection("default", "test-collection")
        .expect("Storage error")
        .expect("Collection should exist");
    assert_eq!(stored.visibility, CollectionVisibility::Private);
}

/// Test that document changes are propagated
#[tokio::test]
async fn test_document_propagation() {
    let store = create_test_storage();
    let (tx, mut rx) = mpsc::unbounded_channel();

    let config = create_test_cluster_config(1);

    let cluster = ClusterManager::new(config, store.clone(), tx)
        .await
        .expect("Failed to create cluster manager");
    cluster
        .initialize()
        .await
        .expect("Failed to initialize cluster");

    // Create namespace and collection first
    let namespace = Namespace::new("default");
    let ns_changelog = ChangeLog::new(
        ChangeCommand::AddNamespace,
        "default",
        "default",
        &namespace,
    );
    cluster.propose(ns_changelog).await.unwrap();
    let _ = rx.recv().await;

    let collection = Collection {
        name: "test-collection".to_string(),
        namespace: "default".to_string(),
        visibility: CollectionVisibility::Private,
        tags: vec![],
    };
    let col_changelog = ChangeLog::new(
        ChangeCommand::AddCollection,
        "default",
        "test-collection",
        &collection,
    );
    cluster.propose(col_changelog).await.unwrap();
    let _ = rx.recv().await;

    // Create a document
    let document = Document {
        id: "doc-1".to_string(),
        namespace: "default".to_string(),
        collection: "test-collection".to_string(),
        data: serde_json::json!({
            "title": "Test Document",
            "content": "This is a test document"
        }),
        created_at: chrono::Utc::now(),
        updated_at: chrono::Utc::now(),
    };

    let changelog = ChangeLog::new(
        ChangeCommand::AddDocument,
        &document.namespace,
        &document.id,
        &document,
    );

    let response = cluster.propose(changelog).await.expect("Failed to propose");
    assert!(response.success);

    let received = timeout(Duration::from_secs(1), rx.recv())
        .await
        .expect("Timeout")
        .expect("Should receive notification");

    assert_eq!(received.cmd, ChangeCommand::AddDocument);

    let stored = store
        .get_document("default", "test-collection", "doc-1")
        .expect("Storage error")
        .expect("Document should exist");
    assert_eq!(stored.data["title"], "Test Document");
}

/// Test that delete operations are propagated
#[tokio::test]
async fn test_delete_propagation() {
    let store = create_test_storage();
    let (tx, mut rx) = mpsc::unbounded_channel();

    let config = create_test_cluster_config(1);

    let cluster = ClusterManager::new(config, store.clone(), tx)
        .await
        .expect("Failed to create cluster manager");
    cluster
        .initialize()
        .await
        .expect("Failed to initialize cluster");

    // Create a namespace
    let namespace = Namespace::new("to-delete");
    let add_changelog = ChangeLog::new(
        ChangeCommand::AddNamespace,
        &namespace.name,
        &namespace.name,
        &namespace,
    );
    cluster
        .propose(add_changelog)
        .await
        .expect("Failed to add namespace");
    let _ = rx.recv().await;

    // Verify it exists
    assert!(store.get_namespace("to-delete").unwrap().is_some());

    // Delete the namespace
    let delete_changelog = ChangeLog::new(
        ChangeCommand::DeleteNamespace,
        &namespace.name,
        &namespace.name,
        &namespace,
    );

    let response = cluster
        .propose(delete_changelog)
        .await
        .expect("Failed to propose delete");
    assert!(response.success);

    let received = timeout(Duration::from_secs(1), rx.recv())
        .await
        .expect("Timeout")
        .expect("Should receive notification");

    assert_eq!(received.cmd, ChangeCommand::DeleteNamespace);

    // Verify it's deleted
    assert!(store.get_namespace("to-delete").unwrap().is_none());
}

/// Test that multiple resources are propagated in sequence
#[tokio::test]
async fn test_multiple_resource_propagation() {
    let store = create_test_storage();
    let (tx, mut rx) = mpsc::unbounded_channel();

    let config = create_test_cluster_config(1);

    let cluster = ClusterManager::new(config, store.clone(), tx)
        .await
        .expect("Failed to create cluster manager");
    cluster
        .initialize()
        .await
        .expect("Failed to initialize cluster");

    // Create multiple resources in sequence
    let resources = vec![
        (
            "namespace",
            ChangeLog::new(
                ChangeCommand::AddNamespace,
                "multi-test",
                "multi-test",
                &Namespace::new("multi-test"),
            ),
        ),
        (
            "service",
            ChangeLog::new(
                ChangeCommand::AddService,
                "multi-test",
                "svc-1",
                &Service {
                    name: "svc-1".to_string(),
                    namespace: "multi-test".to_string(),
                    urls: vec!["http://localhost:8080".to_string()],
                    retries: None,
                    retry_timeout_ms: None,
                    connect_timeout_ms: None,
                    request_timeout_ms: None,
                    tls_skip_verify: false,
                    http2_only: false,
                    hide_dgate_headers: false,
                    disable_query_params: false,
                    tags: vec![],
                },
            ),
        ),
        (
            "route",
            ChangeLog::new(
                ChangeCommand::AddRoute,
                "multi-test",
                "route-1",
                &Route {
                    name: "route-1".to_string(),
                    namespace: "multi-test".to_string(),
                    paths: vec!["/test/**".to_string()],
                    methods: vec!["*".to_string()],
                    service: Some("svc-1".to_string()),
                    modules: vec![],
                    strip_path: false,
                    preserve_host: false,
                    tags: vec![],
                },
            ),
        ),
    ];

    let mut received_count = 0;

    for (resource_type, changelog) in resources {
        let response = cluster.propose(changelog).await.expect("Failed to propose");
        assert!(response.success, "Failed to create {}", resource_type);

        let received = timeout(Duration::from_secs(1), rx.recv())
            .await
            .expect("Timeout")
            .expect("Should receive notification");

        received_count += 1;
        println!(
            "Received notification #{}: {:?}",
            received_count, received.cmd
        );
    }

    // Verify all resources exist
    assert!(store.get_namespace("multi-test").unwrap().is_some());
    assert!(store.get_service("multi-test", "svc-1").unwrap().is_some());
    assert!(store.get_route("multi-test", "route-1").unwrap().is_some());

    assert_eq!(received_count, 3);
}

/// Test that notification channel receives correct changelog data
#[tokio::test]
async fn test_changelog_data_integrity() {
    let store = create_test_storage();
    let (tx, mut rx) = mpsc::unbounded_channel();

    let config = create_test_cluster_config(1);

    let cluster = ClusterManager::new(config, store.clone(), tx)
        .await
        .expect("Failed to create cluster manager");
    cluster
        .initialize()
        .await
        .expect("Failed to initialize cluster");

    // Create a namespace with specific data
    let namespace = Namespace {
        name: "data-integrity-test".to_string(),
        tags: vec!["tag1".to_string(), "tag2".to_string()],
    };

    let original_changelog = ChangeLog::new(
        ChangeCommand::AddNamespace,
        &namespace.name,
        &namespace.name,
        &namespace,
    );

    let original_id = original_changelog.id.clone();

    cluster
        .propose(original_changelog)
        .await
        .expect("Failed to propose");

    let received = timeout(Duration::from_secs(1), rx.recv())
        .await
        .expect("Timeout")
        .expect("Should receive notification");

    // Verify the changelog data is intact
    assert_eq!(received.id, original_id);
    assert_eq!(received.namespace, "data-integrity-test");
    assert_eq!(received.name, "data-integrity-test");
    assert_eq!(received.cmd, ChangeCommand::AddNamespace);

    // Verify the item data is correct
    let received_ns: Namespace =
        serde_json::from_value(received.item).expect("Failed to deserialize namespace");
    assert_eq!(received_ns.name, "data-integrity-test");
    assert_eq!(received_ns.tags, vec!["tag1", "tag2"]);
}
