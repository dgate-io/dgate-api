package commands

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/dgate-io/dgate-api/pkg/dgclient"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const version = "test"

func TestGenericCommands(t *testing.T) {
	stdout := os.Stdout
	os.Stdout = os.NewFile(0, os.DevNull)
	defer func() { os.Stdout = stdout }()

	resources := []string{
		"namespace", "route", "service",
		"module", "domain", "secret",
		"collection", "document",
	}
	actions := []string{
		"get", "list",
		"create", "delete",
	}

	for _, resource := range resources {
		for _, action := range actions {
			os.Args = []string{
				"dgate-cli",
				"--admin=localhost.com",
				resource, action,
			}
			switch action {
			case "delete", "get", "create":
				if resource == "document" {
					os.Args = append(
						os.Args,
						"id=test",
					)
				} else {
					os.Args = append(os.Args, "name=test")
				}
			}
			if resource == "document" {
				os.Args = append(
					os.Args,
					"collection=test",
				)
			}
			if action == "create" {
				switch resource {
				case "route":
					os.Args = append(os.Args, "paths=/", "methods=GET")
				case "service":
					os.Args = append(os.Args, "urls=http://localhost.net")
				case "module":
					os.Args = append(os.Args, "payload@=testdata/test.ts")
				case "domain":
					os.Args = append(os.Args, "patterns=*")
				case "secret":
					os.Args = append(os.Args, "data=123")
				case "collection":
					os.Args = append(os.Args, "schema:={}")
				case "document":
					os.Args = append(os.Args, "data:={}")
				}
			}
			mockClient := new(mockDGClient)
			funcName := firstUpper(action) + firstUpper(resource)
			mockClient.On(
				funcName,
				mock.Anything,
				mock.Anything,
				mock.Anything,
			).Return(nil, nil)

			mockClient.On("Init", "localhost.com").Return(nil)
			assert.Nil(t, Run(mockClient, version))
		}
	}
}

func TestCommands_ClientError(t *testing.T) {
	stdout := os.Stdout
	os.Stdout = os.NewFile(0, os.DevNull)
	defer func() { os.Stdout = stdout }()

	resources := []string{
		"namespace", "route", "service",
		"module", "domain", "secret",
		"collection", "document",
	}
	actions := []string{
		"get", "list",
		"create", "delete",
	}

	for _, resource := range resources {
		for _, action := range actions {
			os.Args = []string{
				"dgate-cli",
				"--admin=localhost.com",
				resource, action,
			}
			switch action {
			case "delete", "get", "create":
				if resource == "document" {
					os.Args = append(
						os.Args,
						"id=test",
					)
				} else {
					os.Args = append(os.Args, "name=test")
				}
			}
			if resource == "document" {
				os.Args = append(
					os.Args,
					"collection=test",
				)
			}
			if action == "create" {
				switch resource {
				case "route":
					os.Args = append(os.Args, "paths=/", "methods=GET")
				case "service":
					os.Args = append(os.Args, "urls=http://localhost.net")
				case "module":
					os.Args = append(os.Args, "payload@=testdata/test.ts")
				case "domain":
					os.Args = append(os.Args, "patterns=*")
				case "secret":
					os.Args = append(os.Args, "data=123")
				case "collection":
					os.Args = append(os.Args, "schema:={}")
				case "document":
					os.Args = append(os.Args, "data:={}")
				}
			}
			mockClient := new(mockDGClient)
			funcName := firstUpper(action) + firstUpper(resource)
			mockCall := mockClient.On(
				funcName,
				mock.Anything,
				mock.Anything,
				mock.Anything,
			)
			switch action {
			case "get", "list":
				mockCall.Return(nil, errors.New("error"))
			case "create", "delete":
				mockCall.Return(errors.New("error"))
			}
			mockClient.On("Init", "localhost.com").Return(nil)
			err := Run(mockClient, version)
			if assert.NotNil(t, err, "no error on %s:%s", action, resource) {
				assert.Equal(t, "error", err.Error())
			}
			if action == "delete" && resource == "document" {
				mockClient.On("DeleteAllDocument", "test", "test").
					Return(errors.New("error"))
				err = Run(mockClient, version)
				if assert.NotNil(t, err, "no error on %s:%s", action, resource) {
					assert.Equal(t, "error", err.Error())
				}
			}
		}
	}
}

func firstUpper(s string) string {
	if len(s) <= 1 {
		return strings.ToUpper(s)
	}
	rs := []rune(s)
	firstChar := strings.ToUpper(string(rs[0]))
	return firstChar + string(rs[1:])
}

type mockDGClient struct {
	mock.Mock
}

var _ dgclient.DGateClient = &mockDGClient{}

func (m *mockDGClient) Init(
	baseUrl string,
	client *http.Client,
	opts ...dgclient.Options,
) error {
	args := m.Called(baseUrl)
	return args.Error(0)
}

func (m *mockDGClient) GetRoute(name, namespace string) (*spec.Route, error) {
	args := m.Called(name, namespace)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args[0].(*spec.Route), args.Error(1)
}

func (m *mockDGClient) CreateRoute(rt *spec.Route) error {
	args := m.Called(rt)
	return args.Error(0)
}

func (m *mockDGClient) DeleteRoute(name, namespace string) error {
	args := m.Called(name, namespace)
	return args.Error(0)
}

func (m *mockDGClient) ListRoute(namespace string) ([]*spec.Route, error) {
	args := m.Called(namespace)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args[0].([]*spec.Route), args.Error(1)
}

func (m *mockDGClient) GetNamespace(name string) (*spec.Namespace, error) {
	args := m.Called(name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args[0].(*spec.Namespace), args.Error(1)
}

func (m *mockDGClient) CreateNamespace(ns *spec.Namespace) error {
	args := m.Called(ns)
	return args.Error(0)
}

func (m *mockDGClient) DeleteNamespace(name string) error {
	args := m.Called(name)
	return args.Error(0)
}
func (m *mockDGClient) ListNamespace() ([]*spec.Namespace, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args[0].([]*spec.Namespace), args.Error(1)
}

func (m *mockDGClient) CreateSecret(sec *spec.Secret) error {
	args := m.Called(sec)
	return args.Error(0)
}

func (m *mockDGClient) DeleteSecret(name, namespace string) error {
	args := m.Called(name, namespace)
	return args.Error(0)
}

func (m *mockDGClient) GetSecret(name, namespace string) (*spec.Secret, error) {
	args := m.Called(name, namespace)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args[0].(*spec.Secret), args.Error(1)
}

func (m *mockDGClient) ListSecret(namespace string) ([]*spec.Secret, error) {
	args := m.Called(namespace)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args[0].([]*spec.Secret), args.Error(1)
}
func (m *mockDGClient) GetService(name string, namespace string) (*spec.Service, error) {
	args := m.Called(name, namespace)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args[0].(*spec.Service), args.Error(1)
}

func (m *mockDGClient) CreateService(svc *spec.Service) error {
	args := m.Called(svc)
	return args.Error(0)
}

func (m *mockDGClient) DeleteService(name, namespace string) error {
	args := m.Called(name, namespace)
	return args.Error(0)
}

func (m *mockDGClient) ListService(namespace string) ([]*spec.Service, error) {
	args := m.Called(namespace)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args[0].([]*spec.Service), args.Error(1)
}

func (m *mockDGClient) GetModule(name, namespace string) (*spec.Module, error) {
	args := m.Called(name, namespace)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args[0].(*spec.Module), args.Error(1)
}

func (m *mockDGClient) CreateModule(mod *spec.Module) error {
	args := m.Called(mod)
	return args.Error(0)
}

func (m *mockDGClient) DeleteModule(name, namespace string) error {
	args := m.Called(name, namespace)
	return args.Error(0)
}

func (m *mockDGClient) ListModule(namespace string) ([]*spec.Module, error) {
	args := m.Called(namespace)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args[0].([]*spec.Module), args.Error(1)
}

func (m *mockDGClient) CreateDomain(domain *spec.Domain) error {
	args := m.Called(domain)
	return args.Error(0)
}

func (m *mockDGClient) DeleteDomain(name, namespace string) error {
	args := m.Called(name, namespace)
	return args.Error(0)
}

func (m *mockDGClient) GetDomain(name, namespace string) (*spec.Domain, error) {
	args := m.Called(name, namespace)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args[0].(*spec.Domain), args.Error(1)
}

func (m *mockDGClient) ListDomain(namespace string) ([]*spec.Domain, error) {
	args := m.Called(namespace)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args[0].([]*spec.Domain), args.Error(1)
}

func (m *mockDGClient) CreateCollection(svc *spec.Collection) error {
	args := m.Called(svc)
	return args.Error(0)
}

func (m *mockDGClient) DeleteCollection(name, namespace string) error {
	args := m.Called(name, namespace)
	return args.Error(0)
}

func (m *mockDGClient) ListCollection(namespace string) ([]*spec.Collection, error) {
	args := m.Called(namespace)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args[0].([]*spec.Collection), args.Error(1)
}

func (m *mockDGClient) GetCollection(name, namespace string) (*spec.Collection, error) {
	args := m.Called(name, namespace)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args[0].(*spec.Collection), args.Error(1)
}

func (m *mockDGClient) CreateDocument(doc *spec.Document) error {
	args := m.Called(doc)
	return args.Error(0)
}

func (m *mockDGClient) DeleteDocument(id, collection, namespace string) error {
	args := m.Called(id, collection, namespace)
	return args.Error(0)
}

func (m *mockDGClient) ListDocument(namespace, collection string) ([]*spec.Document, error) {
	args := m.Called(namespace)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args[0].([]*spec.Document), args.Error(1)
}

func (m *mockDGClient) GetDocument(id, collection, namespace string) (*spec.Document, error) {
	args := m.Called(id, collection, namespace)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args[0].(*spec.Document), args.Error(1)
}

func (m *mockDGClient) DeleteAllDocument(namespace string, collection string) error {
	args := m.Called(namespace)
	return args.Error(0)
}
