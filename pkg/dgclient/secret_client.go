package dgclient

import (
	"net/url"

	"github.com/dgate-io/dgate-api/pkg/spec"
)

type DGateSecretClient interface {
	GetSecret(name, namespace string) (*spec.Secret, error)
	CreateSecret(svc *spec.Secret) error
	DeleteSecret(name, namespace string) error
	ListSecret(namespace string) ([]*spec.Secret, error)
}

var _ DGateSecretClient = &dgateClient{}

func (d *dgateClient) GetSecret(name, namespace string) (*spec.Secret, error) {
	query := d.baseUrl.Query()
	query.Set("namespace", namespace)
	d.baseUrl.RawQuery = query.Encode()
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/secret", name)
	if err != nil {
		return nil, err
	}
	return commonGet[spec.Secret](d.client, uri)
}

func (d *dgateClient) CreateSecret(sec *spec.Secret) error {
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/secret")
	if err != nil {
		return err
	}
	return commonPut(d.client, uri, sec)
}

func (d *dgateClient) DeleteSecret(name, namespace string) error {
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/secret")
	if err != nil {
		return err
	}
	return commonDelete(d.client, uri, name, namespace)
}

func (d *dgateClient) ListSecret(namespace string) ([]*spec.Secret, error) {
	query := d.baseUrl.Query()
	query.Set("namespace", namespace)
	d.baseUrl.RawQuery = query.Encode()
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/secret")
	if err != nil {
		return nil, err
	}
	return commonGetList[*spec.Secret](d.client, uri)
}
