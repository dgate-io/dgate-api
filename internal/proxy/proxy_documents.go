package proxy

import (
	"github.com/dgate-io/dgate-api/pkg/resources"
	"github.com/dgate-io/dgate-api/pkg/spec"
)

// DocumentManager is an interface that defines the methods for managing documents.
func (ps *ProxyState) DocumentManager() resources.DocumentManager {
	return ps
}

// GetDocuments is a function that returns a list of documents in a collection.
func (ps *ProxyState) GetDocuments(collection, namespace string, limit, offset int) ([]*spec.Document, error) {
	if _, ok := ps.rm.GetNamespace(namespace); !ok {
		return nil, spec.ErrNamespaceNotFound(namespace)
	}
	if _, ok := ps.rm.GetCollection(collection, namespace); !ok {
		return nil, spec.ErrCollectionNotFound(collection)
	}
	return ps.store.FetchDocuments(collection, namespace, limit, offset)
}

// GetDocumentByID is a function that returns a document in a collection by its ID.
func (ps *ProxyState) GetDocumentByID(docId, collection, namespace string) (*spec.Document, error) {
	if _, ok := ps.rm.GetNamespace(namespace); !ok {
		return nil, spec.ErrNamespaceNotFound(namespace)
	}
	if _, ok := ps.rm.GetCollection(collection, namespace); !ok {
		return nil, spec.ErrCollectionNotFound(collection)
	}
	return ps.store.FetchDocument(docId, collection, namespace)
}
