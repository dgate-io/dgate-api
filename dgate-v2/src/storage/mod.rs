//! Storage module for DGate
//!
//! Provides KV storage for resources and documents using redb for file-based
//! persistence and a concurrent hashmap for in-memory storage.

use crate::resources::{
    ChangeLog, Collection, Document, Domain, Module, Namespace, Route, Secret, Service,
};
use dashmap::DashMap;
use redb::ReadableTable;
use serde::{de::DeserializeOwned, Serialize};
use std::path::Path;
use std::sync::Arc;
use thiserror::Error;

#[derive(Error, Debug)]
pub enum StorageError {
    #[error("Key not found: {0}")]
    NotFound(String),

    #[error("Storage error: {0}")]
    Internal(String),

    #[error("Serialization error: {0}")]
    Serialization(#[from] serde_json::Error),

    #[error("Database error: {0}")]
    Database(String),

    #[error("IO error: {0}")]
    Io(#[from] std::io::Error),
}

pub type StorageResult<T> = Result<T, StorageError>;

/// Storage trait for KV operations
pub trait Storage: Send + Sync {
    fn get(&self, table: &str, key: &str) -> StorageResult<Option<Vec<u8>>>;
    fn set(&self, table: &str, key: &str, value: &[u8]) -> StorageResult<()>;
    fn delete(&self, table: &str, key: &str) -> StorageResult<()>;
    fn list(&self, table: &str, prefix: &str) -> StorageResult<Vec<(String, Vec<u8>)>>;
    fn clear(&self, table: &str) -> StorageResult<()>;
}

/// In-memory storage implementation
pub struct MemoryStorage {
    tables: DashMap<String, DashMap<String, Vec<u8>>>,
}

impl MemoryStorage {
    pub fn new() -> Self {
        Self {
            tables: DashMap::new(),
        }
    }

    fn get_table(&self, table: &str) -> dashmap::mapref::one::RefMut<String, DashMap<String, Vec<u8>>> {
        self.tables
            .entry(table.to_string())
            .or_insert_with(DashMap::new)
    }
}

impl Default for MemoryStorage {
    fn default() -> Self {
        Self::new()
    }
}

impl Storage for MemoryStorage {
    fn get(&self, table: &str, key: &str) -> StorageResult<Option<Vec<u8>>> {
        let table_ref = self.get_table(table);
        Ok(table_ref.get(key).map(|v| v.clone()))
    }

    fn set(&self, table: &str, key: &str, value: &[u8]) -> StorageResult<()> {
        let table_ref = self.get_table(table);
        table_ref.insert(key.to_string(), value.to_vec());
        Ok(())
    }

    fn delete(&self, table: &str, key: &str) -> StorageResult<()> {
        let table_ref = self.get_table(table);
        table_ref.remove(key);
        Ok(())
    }

    fn list(&self, table: &str, prefix: &str) -> StorageResult<Vec<(String, Vec<u8>)>> {
        let table_ref = self.get_table(table);
        let results: Vec<(String, Vec<u8>)> = table_ref
            .iter()
            .filter(|entry| entry.key().starts_with(prefix))
            .map(|entry| (entry.key().clone(), entry.value().clone()))
            .collect();
        Ok(results)
    }

    fn clear(&self, table: &str) -> StorageResult<()> {
        if let Some(table_ref) = self.tables.get(table) {
            table_ref.clear();
        }
        Ok(())
    }
}

// Table definitions for redb
const TABLE_NAMESPACES: redb::TableDefinition<'static, &str, &[u8]> =
    redb::TableDefinition::new("namespaces");
const TABLE_ROUTES: redb::TableDefinition<'static, &str, &[u8]> =
    redb::TableDefinition::new("routes");
const TABLE_SERVICES: redb::TableDefinition<'static, &str, &[u8]> =
    redb::TableDefinition::new("services");
const TABLE_MODULES: redb::TableDefinition<'static, &str, &[u8]> =
    redb::TableDefinition::new("modules");
const TABLE_DOMAINS: redb::TableDefinition<'static, &str, &[u8]> =
    redb::TableDefinition::new("domains");
const TABLE_SECRETS: redb::TableDefinition<'static, &str, &[u8]> =
    redb::TableDefinition::new("secrets");
const TABLE_COLLECTIONS: redb::TableDefinition<'static, &str, &[u8]> =
    redb::TableDefinition::new("collections");
const TABLE_DOCUMENTS: redb::TableDefinition<'static, &str, &[u8]> =
    redb::TableDefinition::new("documents");
const TABLE_CHANGELOGS: redb::TableDefinition<'static, &str, &[u8]> =
    redb::TableDefinition::new("changelogs");

/// File-based storage using redb
pub struct FileStorage {
    db: redb::Database,
}

impl FileStorage {
    pub fn new(path: impl AsRef<Path>) -> StorageResult<Self> {
        // Ensure parent directory exists
        if let Some(parent) = path.as_ref().parent() {
            std::fs::create_dir_all(parent)?;
        }

        let db = redb::Database::create(path.as_ref())
            .map_err(|e| StorageError::Database(e.to_string()))?;

        Ok(Self { db })
    }

    fn get_table_def(table: &str) -> redb::TableDefinition<'static, &'static str, &'static [u8]> {
        match table {
            "namespaces" => TABLE_NAMESPACES,
            "routes" => TABLE_ROUTES,
            "services" => TABLE_SERVICES,
            "modules" => TABLE_MODULES,
            "domains" => TABLE_DOMAINS,
            "secrets" => TABLE_SECRETS,
            "collections" => TABLE_COLLECTIONS,
            "documents" => TABLE_DOCUMENTS,
            "changelogs" => TABLE_CHANGELOGS,
            _ => TABLE_NAMESPACES, // fallback
        }
    }
}

impl Storage for FileStorage {
    fn get(&self, table: &str, key: &str) -> StorageResult<Option<Vec<u8>>> {
        let read_txn = self
            .db
            .begin_read()
            .map_err(|e| StorageError::Database(e.to_string()))?;

        let table_def = Self::get_table_def(table);
        let table = match read_txn.open_table(table_def) {
            Ok(t) => t,
            Err(redb::TableError::TableDoesNotExist(_)) => return Ok(None),
            Err(e) => return Err(StorageError::Database(e.to_string())),
        };

        match table.get(key) {
            Ok(Some(value)) => Ok(Some(value.value().to_vec())),
            Ok(None) => Ok(None),
            Err(e) => Err(StorageError::Database(e.to_string())),
        }
    }

    fn set(&self, table: &str, key: &str, value: &[u8]) -> StorageResult<()> {
        let write_txn = self
            .db
            .begin_write()
            .map_err(|e| StorageError::Database(e.to_string()))?;

        {
            let table_def = Self::get_table_def(table);
            let mut table = write_txn
                .open_table(table_def)
                .map_err(|e| StorageError::Database(e.to_string()))?;

            table
                .insert(key, value)
                .map_err(|e| StorageError::Database(e.to_string()))?;
        }

        write_txn
            .commit()
            .map_err(|e| StorageError::Database(e.to_string()))?;
        Ok(())
    }

    fn delete(&self, table: &str, key: &str) -> StorageResult<()> {
        let write_txn = self
            .db
            .begin_write()
            .map_err(|e| StorageError::Database(e.to_string()))?;

        {
            let table_def = Self::get_table_def(table);
            let mut table = match write_txn.open_table(table_def) {
                Ok(t) => t,
                Err(redb::TableError::TableDoesNotExist(_)) => return Ok(()),
                Err(e) => return Err(StorageError::Database(e.to_string())),
            };

            table
                .remove(key)
                .map_err(|e| StorageError::Database(e.to_string()))?;
        }

        write_txn
            .commit()
            .map_err(|e| StorageError::Database(e.to_string()))?;
        Ok(())
    }

    fn list(&self, table: &str, prefix: &str) -> StorageResult<Vec<(String, Vec<u8>)>> {
        let read_txn = self
            .db
            .begin_read()
            .map_err(|e| StorageError::Database(e.to_string()))?;

        let table_def = Self::get_table_def(table);
        let table = match read_txn.open_table(table_def) {
            Ok(t) => t,
            Err(redb::TableError::TableDoesNotExist(_)) => return Ok(Vec::new()),
            Err(e) => return Err(StorageError::Database(e.to_string())),
        };

        let mut results = Vec::new();
        let iter = table
            .iter()
            .map_err(|e| StorageError::Database(e.to_string()))?;

        for entry in iter {
            let entry = entry.map_err(|e| StorageError::Database(e.to_string()))?;
            let key = entry.0.value().to_string();
            if key.starts_with(prefix) {
                results.push((key, entry.1.value().to_vec()));
            }
        }

        Ok(results)
    }

    fn clear(&self, table: &str) -> StorageResult<()> {
        let write_txn = self
            .db
            .begin_write()
            .map_err(|e| StorageError::Database(e.to_string()))?;

        {
            let table_def = Self::get_table_def(table);
            // Try to delete the table, ignore if it doesn't exist
            let _ = write_txn.delete_table(table_def);
        }

        write_txn
            .commit()
            .map_err(|e| StorageError::Database(e.to_string()))?;
        Ok(())
    }
}

// Table names for resources (string constants for ProxyStore)
const TBL_NAMESPACES: &str = "namespaces";
const TBL_ROUTES: &str = "routes";
const TBL_SERVICES: &str = "services";
const TBL_MODULES: &str = "modules";
const TBL_DOMAINS: &str = "domains";
const TBL_SECRETS: &str = "secrets";
const TBL_COLLECTIONS: &str = "collections";
const TBL_DOCUMENTS: &str = "documents";
const TBL_CHANGELOGS: &str = "changelogs";

/// Proxy store wraps storage with typed resource operations
pub struct ProxyStore {
    storage: Arc<dyn Storage>,
}

impl ProxyStore {
    pub fn new(storage: Arc<dyn Storage>) -> Self {
        Self { storage }
    }

    /// Create a key for namespace-scoped resources
    fn scoped_key(namespace: &str, name: &str) -> String {
        format!("{}:{}", namespace, name)
    }

    /// Create a key for documents (namespace:collection:id)
    fn document_key(namespace: &str, collection: &str, id: &str) -> String {
        format!("{}:{}:{}", namespace, collection, id)
    }

    fn get_typed<T: DeserializeOwned>(&self, table: &str, key: &str) -> StorageResult<Option<T>> {
        match self.storage.get(table, key)? {
            Some(data) => {
                let item: T = serde_json::from_slice(&data)?;
                Ok(Some(item))
            }
            None => Ok(None),
        }
    }

    fn set_typed<T: Serialize>(&self, table: &str, key: &str, value: &T) -> StorageResult<()> {
        let data = serde_json::to_vec(value)?;
        self.storage.set(table, key, &data)
    }

    fn list_typed<T: DeserializeOwned>(&self, table: &str, prefix: &str) -> StorageResult<Vec<T>> {
        let items = self.storage.list(table, prefix)?;
        items
            .into_iter()
            .map(|(_, data)| serde_json::from_slice(&data).map_err(StorageError::from))
            .collect()
    }

    // Namespace operations
    pub fn get_namespace(&self, name: &str) -> StorageResult<Option<Namespace>> {
        self.get_typed(TBL_NAMESPACES, name)
    }

    pub fn set_namespace(&self, namespace: &Namespace) -> StorageResult<()> {
        self.set_typed(TBL_NAMESPACES, &namespace.name, namespace)
    }

    pub fn delete_namespace(&self, name: &str) -> StorageResult<()> {
        self.storage.delete(TBL_NAMESPACES, name)
    }

    pub fn list_namespaces(&self) -> StorageResult<Vec<Namespace>> {
        self.list_typed(TBL_NAMESPACES, "")
    }

    // Route operations
    pub fn get_route(&self, namespace: &str, name: &str) -> StorageResult<Option<Route>> {
        let key = Self::scoped_key(namespace, name);
        self.get_typed(TBL_ROUTES, &key)
    }

    pub fn set_route(&self, route: &Route) -> StorageResult<()> {
        let key = Self::scoped_key(&route.namespace, &route.name);
        self.set_typed(TBL_ROUTES, &key, route)
    }

    pub fn delete_route(&self, namespace: &str, name: &str) -> StorageResult<()> {
        let key = Self::scoped_key(namespace, name);
        self.storage.delete(TBL_ROUTES, &key)
    }

    pub fn list_routes(&self, namespace: &str) -> StorageResult<Vec<Route>> {
        let prefix = format!("{}:", namespace);
        self.list_typed(TBL_ROUTES, &prefix)
    }

    pub fn list_all_routes(&self) -> StorageResult<Vec<Route>> {
        self.list_typed(TBL_ROUTES, "")
    }

    // Service operations
    pub fn get_service(&self, namespace: &str, name: &str) -> StorageResult<Option<Service>> {
        let key = Self::scoped_key(namespace, name);
        self.get_typed(TBL_SERVICES, &key)
    }

    pub fn set_service(&self, service: &Service) -> StorageResult<()> {
        let key = Self::scoped_key(&service.namespace, &service.name);
        self.set_typed(TBL_SERVICES, &key, service)
    }

    pub fn delete_service(&self, namespace: &str, name: &str) -> StorageResult<()> {
        let key = Self::scoped_key(namespace, name);
        self.storage.delete(TBL_SERVICES, &key)
    }

    pub fn list_services(&self, namespace: &str) -> StorageResult<Vec<Service>> {
        let prefix = format!("{}:", namespace);
        self.list_typed(TBL_SERVICES, &prefix)
    }

    // Module operations
    pub fn get_module(&self, namespace: &str, name: &str) -> StorageResult<Option<Module>> {
        let key = Self::scoped_key(namespace, name);
        self.get_typed(TBL_MODULES, &key)
    }

    pub fn set_module(&self, module: &Module) -> StorageResult<()> {
        let key = Self::scoped_key(&module.namespace, &module.name);
        self.set_typed(TBL_MODULES, &key, module)
    }

    pub fn delete_module(&self, namespace: &str, name: &str) -> StorageResult<()> {
        let key = Self::scoped_key(namespace, name);
        self.storage.delete(TBL_MODULES, &key)
    }

    pub fn list_modules(&self, namespace: &str) -> StorageResult<Vec<Module>> {
        let prefix = format!("{}:", namespace);
        self.list_typed(TBL_MODULES, &prefix)
    }

    // Domain operations
    pub fn get_domain(&self, namespace: &str, name: &str) -> StorageResult<Option<Domain>> {
        let key = Self::scoped_key(namespace, name);
        self.get_typed(TBL_DOMAINS, &key)
    }

    pub fn set_domain(&self, domain: &Domain) -> StorageResult<()> {
        let key = Self::scoped_key(&domain.namespace, &domain.name);
        self.set_typed(TBL_DOMAINS, &key, domain)
    }

    pub fn delete_domain(&self, namespace: &str, name: &str) -> StorageResult<()> {
        let key = Self::scoped_key(namespace, name);
        self.storage.delete(TBL_DOMAINS, &key)
    }

    pub fn list_domains(&self, namespace: &str) -> StorageResult<Vec<Domain>> {
        let prefix = format!("{}:", namespace);
        self.list_typed(TBL_DOMAINS, &prefix)
    }

    pub fn list_all_domains(&self) -> StorageResult<Vec<Domain>> {
        self.list_typed(TBL_DOMAINS, "")
    }

    // Secret operations
    pub fn get_secret(&self, namespace: &str, name: &str) -> StorageResult<Option<Secret>> {
        let key = Self::scoped_key(namespace, name);
        self.get_typed(TBL_SECRETS, &key)
    }

    pub fn set_secret(&self, secret: &Secret) -> StorageResult<()> {
        let key = Self::scoped_key(&secret.namespace, &secret.name);
        self.set_typed(TBL_SECRETS, &key, secret)
    }

    pub fn delete_secret(&self, namespace: &str, name: &str) -> StorageResult<()> {
        let key = Self::scoped_key(namespace, name);
        self.storage.delete(TBL_SECRETS, &key)
    }

    pub fn list_secrets(&self, namespace: &str) -> StorageResult<Vec<Secret>> {
        let prefix = format!("{}:", namespace);
        self.list_typed(TBL_SECRETS, &prefix)
    }

    // Collection operations
    pub fn get_collection(&self, namespace: &str, name: &str) -> StorageResult<Option<Collection>> {
        let key = Self::scoped_key(namespace, name);
        self.get_typed(TBL_COLLECTIONS, &key)
    }

    pub fn set_collection(&self, collection: &Collection) -> StorageResult<()> {
        let key = Self::scoped_key(&collection.namespace, &collection.name);
        self.set_typed(TBL_COLLECTIONS, &key, collection)
    }

    pub fn delete_collection(&self, namespace: &str, name: &str) -> StorageResult<()> {
        let key = Self::scoped_key(namespace, name);
        self.storage.delete(TBL_COLLECTIONS, &key)
    }

    pub fn list_collections(&self, namespace: &str) -> StorageResult<Vec<Collection>> {
        let prefix = format!("{}:", namespace);
        self.list_typed(TBL_COLLECTIONS, &prefix)
    }

    pub fn list_all_collections(&self) -> StorageResult<Vec<Collection>> {
        self.list_typed(TBL_COLLECTIONS, "")
    }

    // Document operations
    pub fn get_document(
        &self,
        namespace: &str,
        collection: &str,
        id: &str,
    ) -> StorageResult<Option<Document>> {
        let key = Self::document_key(namespace, collection, id);
        self.get_typed(TBL_DOCUMENTS, &key)
    }

    pub fn set_document(&self, document: &Document) -> StorageResult<()> {
        let key = Self::document_key(&document.namespace, &document.collection, &document.id);
        self.set_typed(TBL_DOCUMENTS, &key, document)
    }

    pub fn delete_document(
        &self,
        namespace: &str,
        collection: &str,
        id: &str,
    ) -> StorageResult<()> {
        let key = Self::document_key(namespace, collection, id);
        self.storage.delete(TBL_DOCUMENTS, &key)
    }

    pub fn list_documents(&self, namespace: &str, collection: &str) -> StorageResult<Vec<Document>> {
        let prefix = format!("{}:{}:", namespace, collection);
        self.list_typed(TBL_DOCUMENTS, &prefix)
    }

    // ChangeLog operations
    pub fn append_changelog(&self, changelog: &ChangeLog) -> StorageResult<()> {
        self.set_typed(TBL_CHANGELOGS, &changelog.id, changelog)
    }

    pub fn list_changelogs(&self) -> StorageResult<Vec<ChangeLog>> {
        self.list_typed(TBL_CHANGELOGS, "")
    }

    pub fn clear_changelogs(&self) -> StorageResult<()> {
        self.storage.clear(TBL_CHANGELOGS)
    }
}

/// Create storage based on configuration
pub fn create_storage(config: &crate::config::StorageConfig) -> Arc<dyn Storage> {
    match config.storage_type {
        crate::config::StorageType::Memory => Arc::new(MemoryStorage::new()),
        crate::config::StorageType::File => {
            let dir = config
                .dir
                .as_ref()
                .map(|s| s.as_str())
                .unwrap_or(".dgate/data");
            let path = format!("{}/dgate.redb", dir);
            Arc::new(FileStorage::new(&path).expect("Failed to create file storage"))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_memory_storage() {
        let storage = MemoryStorage::new();

        // Test set and get
        storage.set("test", "key1", b"value1").unwrap();
        let result = storage.get("test", "key1").unwrap();
        assert_eq!(result, Some(b"value1".to_vec()));

        // Test list
        storage.set("test", "prefix:a", b"a").unwrap();
        storage.set("test", "prefix:b", b"b").unwrap();
        storage.set("test", "other:c", b"c").unwrap();

        let list = storage.list("test", "prefix:").unwrap();
        assert_eq!(list.len(), 2);

        // Test delete
        storage.delete("test", "key1").unwrap();
        let result = storage.get("test", "key1").unwrap();
        assert!(result.is_none());
    }

    #[test]
    fn test_proxy_store_namespaces() {
        let storage = Arc::new(MemoryStorage::new());
        let store = ProxyStore::new(storage);

        let ns = Namespace::new("test-ns");
        store.set_namespace(&ns).unwrap();

        let retrieved = store.get_namespace("test-ns").unwrap();
        assert!(retrieved.is_some());
        assert_eq!(retrieved.unwrap().name, "test-ns");

        let namespaces = store.list_namespaces().unwrap();
        assert_eq!(namespaces.len(), 1);

        store.delete_namespace("test-ns").unwrap();
        let retrieved = store.get_namespace("test-ns").unwrap();
        assert!(retrieved.is_none());
    }
}
