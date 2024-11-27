package dgclient

import (
	"net/url"

	"github.com/dgate-io/dgate-api/pkg/spec"
)

type DGateRouteClient interface {
	GetRoute(name, namespace string) (*spec.Route, error)
	CreateRoute(rt *spec.Route) error
	DeleteRoute(name, namespace string) error
	ListRoute(namespace string) ([]*spec.Route, error)
}

var _ DGateRouteClient = &dgateClient{}

func (d *dgateClient) GetRoute(name, namespace string) (*spec.Route, error) {
	query := d.baseUrl.Query()
	query.Set("namespace", namespace)
	d.baseUrl.RawQuery = query.Encode()
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/route", name)
	if err != nil {
		return nil, err
	}
	return commonGet[spec.Route](d.client, uri)
}

func (d *dgateClient) CreateRoute(rt *spec.Route) error {
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/route")
	if err != nil {
		return err
	}
	return commonPut(d.client, uri, rt)
}

func (d *dgateClient) DeleteRoute(name, namespace string) error {
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/route")
	if err != nil {
		return err
	}
	return commonDelete(d.client, uri, name, namespace)
}

func (d *dgateClient) ListRoute(namespace string) ([]*spec.Route, error) {
	query := d.baseUrl.Query()
	query.Set("namespace", namespace)
	d.baseUrl.RawQuery = query.Encode()
	uri, err := url.JoinPath(d.baseUrl.String(), "/api/v1/route")
	if err != nil {
		return nil, err
	}
	return commonGetList[*spec.Route](d.client, uri)
}
