package dgclient

import (
	"net/url"

	"github.com/dgate-io/dgate-api/pkg/spec"
)

type DGateModuleClient interface {
	GetModule(name, namespace string) (*spec.Module, error)
	CreateModule(mod *spec.Module) error
	DeleteModule(name, namespace string) error
	ListModule(namespace string) ([]*spec.Module, error)
}

var _ DGateModuleClient = &dgateClient{}

func (d *dgateClient) GetModule(name, namespace string) (*spec.Module, error) {
	query := d.baseUrl.Query()
	query.Set("namespace", namespace)
	d.baseUrl.RawQuery = query.Encode()
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/module", name)
	if err != nil {
		return nil, err
	}
	return commonGet[spec.Module](d.client, uri)
}

func (d *dgateClient) CreateModule(mod *spec.Module) error {
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/module")
	if err != nil {
		return err
	}
	return commonPut(d.client, uri, mod)
}

func (d *dgateClient) DeleteModule(name, namespace string) error {
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/module")
	if err != nil {
		return err
	}
	return commonDelete(d.client, uri, name, namespace)
}

func (d *dgateClient) ListModule(namespace string) ([]*spec.Module, error) {
	query := d.baseUrl.Query()
	query.Set("namespace", namespace)
	d.baseUrl.RawQuery = query.Encode()
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/module")
	if err != nil {
		return nil, err
	}
	return commonGetList[*spec.Module](d.client, uri)
}
