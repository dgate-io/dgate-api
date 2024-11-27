package routes_test

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/dgate-io/chi-router"
	"github.com/dgate-io/dgate-api/internal/admin/changestate/testutil"
	"github.com/dgate-io/dgate-api/internal/admin/routes"
	"github.com/dgate-io/dgate-api/internal/config/configtest"
	"github.com/dgate-io/dgate-api/internal/proxy"
	"github.com/dgate-io/dgate-api/pkg/dgclient"
	"github.com/dgate-io/dgate-api/pkg/resources"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestAdminRoutes_Service(t *testing.T) {
	namespaces := []string{"default", "test"}
	for _, ns := range namespaces {
		config := configtest.NewTest4DGateConfig()
		ps := proxy.NewProxyState(zap.NewNop(), config)
		if err := ps.Start(); err != nil {
			t.Fatal(err)
		}
		mux := chi.NewMux()
		mux.Route("/api/v1", func(r chi.Router) {
			routes.ConfigureServiceAPI(r, zap.NewNop(), ps, config)
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		client := dgclient.NewDGateClient()
		if err := client.Init(server.URL, server.Client()); err != nil {
			t.Fatal(err)
		}

		if err := client.CreateService(&spec.Service{
			Name:          "test",
			URLs:          []string{"http://localhost:8080"},
			NamespaceName: ns,
			Tags:          []string{"test123"},
		}); err != nil {
			t.Fatal(err)
		}
		rm := ps.ResourceManager()
		if _, ok := rm.GetService("test", ns); !ok {
			t.Fatal("service not found")
		}
		if services, err := client.ListService(ns); err != nil {
			t.Fatal(err)
		} else {
			svcs := rm.GetServicesByNamespace(ns)
			assert.Equal(t, len(svcs), len(services))
			assert.Equal(t, spec.TransformDGateServices(svcs...), services)
		}
		if service, err := client.GetService("test", ns); err != nil {
			t.Fatal(err)
		} else {
			svc, ok := rm.GetService("test", ns)
			assert.True(t, ok)
			service2 := spec.TransformDGateService(svc)
			assert.Equal(t, service2, service)
		}
		if err := client.DeleteService("test", ns); err != nil {
			t.Fatal(err)
		} else if _, ok := rm.GetService("test", ns); ok {
			t.Fatal("service not deleted")
		}
	}
}

func TestAdminRoutes_ServiceError(t *testing.T) {
	namespaces := []string{"default", "test", ""}
	for _, ns := range namespaces {
		config := configtest.NewTest3DGateConfig()
		rm := resources.NewManager()
		cs := testutil.NewMockChangeState()
		cs.On("ApplyChangeLog", mock.Anything).
			Return(errors.New("test error"))
		cs.On("ResourceManager").Return(rm)
		mux := chi.NewMux()
		mux.Route("/api/v1", func(r chi.Router) {
			routes.ConfigureServiceAPI(
				r, zap.NewNop(), cs, config)
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		client := dgclient.NewDGateClient()
		if err := client.Init(server.URL, server.Client()); err != nil {
			t.Fatal(err)
		}

		if err := client.CreateService(&spec.Service{
			Name:          "test",
			NamespaceName: ns,
			URLs:          []string{"http://localhost:8080"},
			Tags:          []string{"test123"},
		}); err == nil {
			t.Fatal("expected error")
		}
		if _, err := client.GetService("", ns); err == nil {
			t.Fatal("expected error")
		}
		if err := client.DeleteService("", ns); err == nil {
			t.Fatal("expected error")
		}
	}
}
