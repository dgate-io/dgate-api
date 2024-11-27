package dgclient

import (
	"net/url"

	"github.com/dgate-io/dgate-api/pkg/spec"
)

type DGateNamespaceClient interface {
	GetNamespace(name string) (*spec.Namespace, error)
	CreateNamespace(ns *spec.Namespace) error
	DeleteNamespace(name string) error
	ListNamespace() ([]*spec.Namespace, error)
}

var _ DGateNamespaceClient = &dgateClient{}

func (d *dgateClient) GetNamespace(name string) (*spec.Namespace, error) {
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/namespace", name)
	if err != nil {
		return nil, err
	}
	return commonGet[spec.Namespace](d.client, uri)
}

func (d *dgateClient) CreateNamespace(ns *spec.Namespace) error {
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/namespace")
	if err != nil {
		return err
	}
	return commonPut(d.client, uri, ns)
}

func (d *dgateClient) DeleteNamespace(name string) error {
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/namespace")
	if err != nil {
		return err
	}
	return commonDelete(d.client, uri, name, "")
}

func (d *dgateClient) ListNamespace() ([]*spec.Namespace, error) {
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/namespace")
	if err != nil {
		return nil, err
	}
	return commonGetList[*spec.Namespace](d.client, uri)
}
