package resources

import (
	"github.com/dgate-io/dgate-api/pkg/spec"
)

type DocumentManager interface {
	GetDocumentByID(id, collection, namespace string) (*spec.Document, error)
	GetDocuments(collection, namespace string, limit, offset int) ([]*spec.Document, error)
}
