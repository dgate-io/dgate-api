package dgclient

import (
	"net/url"

	"github.com/dgate-io/dgate-api/pkg/spec"
)

type DGateDocumentClient interface {
	GetDocument(id, namespace, collection string) (*spec.Document, error)
	CreateDocument(doc *spec.Document) error
	DeleteDocument(id, namespace, collection string) error
	DeleteAllDocument(namespace, collection string) error
	ListDocument(namespace, collection string) ([]*spec.Document, error)
}

func (d *dgateClient) GetDocument(id, namespace, collection string) (*spec.Document, error) {
	query := d.baseUrl.Query()
	query.Set("namespace", namespace)
	query.Set("collection", collection)
	d.baseUrl.RawQuery = query.Encode()
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/document", id)
	if err != nil {
		return nil, err
	}
	return commonGet[spec.Document](d.client, uri)
}

func (d *dgateClient) CreateDocument(doc *spec.Document) error {
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/document")
	if err != nil {
		return err
	}
	return commonPut(d.client, uri, doc)
}

func (d *dgateClient) DeleteDocument(id, namespace, collection string) error {
	query := d.baseUrl.Query()
	query.Set("namespace", namespace)
	query.Set("collection", collection)
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/document", id)
	if err != nil {
		return err
	}
	return basicDelete(d.client, uri, nil)
}

func (d *dgateClient) DeleteAllDocument(namespace, collection string) error {
	query := d.baseUrl.Query()
	query.Set("namespace", namespace)
	query.Set("collection", collection)
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/document")
	if err != nil {
		return err
	}
	return basicDelete(d.client, uri, nil)
}

func (d *dgateClient) ListDocument(namespace, collection string) ([]*spec.Document, error) {
	query := d.baseUrl.Query()
	query.Set("namespace", namespace)
	query.Set("collection", collection)
	d.baseUrl.RawQuery = query.Encode()
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/document")
	if err != nil {
		return nil, err
	}
	return commonGetList[*spec.Document](d.client, uri)
}
