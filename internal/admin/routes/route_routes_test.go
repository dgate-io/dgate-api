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

func TestAdminRoutes_Route(t *testing.T) {
	namespaces := []string{"default", "test"}
	for _, ns := range namespaces {
		config := configtest.NewTest3DGateConfig()
		ps := proxy.NewProxyState(zap.NewNop(), config)
		if err := ps.Start(); err != nil {
			t.Fatal(err)
		}
		mux := chi.NewMux()
		mux.Route("/api/v1", func(r chi.Router) {
			routes.ConfigureRouteAPI(r, zap.NewNop(), ps, config)
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		client := dgclient.NewDGateClient()
		if err := client.Init(server.URL, server.Client()); err != nil {
			t.Fatal(err)
		}

		if err := client.CreateRoute(&spec.Route{
			Name:          "test",
			NamespaceName: ns,
			Paths:         []string{"/test"},
			Methods:       []string{"GET"},
			Tags:          []string{"test123"},
		}); err != nil {
			t.Fatal(err)
		}
		rm := ps.ResourceManager()
		if _, ok := rm.GetRoute("test", ns); !ok {
			t.Fatal("route not found")
		}
		if routes, err := client.ListRoute(ns); err != nil {
			t.Fatal(err)
		} else {
			rts := rm.GetRoutesByNamespace(ns)
			assert.Equal(t, len(rts), len(routes))
			assert.Equal(t, spec.TransformDGateRoutes(rts...), routes)
		}
		if route, err := client.GetRoute("test", ns); err != nil {
			t.Fatal(err)
		} else {
			rt, ok := rm.GetRoute("test", ns)
			assert.True(t, ok)
			route2 := spec.TransformDGateRoute(rt)
			assert.Equal(t, route2, route)
		}
		if err := client.DeleteRoute("test", ns); err != nil {
			t.Fatal(err)
		} else if _, ok := rm.GetRoute("test", ns); ok {
			t.Fatal("route not deleted")
		}
	}
}

func TestAdminRoutes_RouteError(t *testing.T) {
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
			routes.ConfigureRouteAPI(r, zap.NewNop(), cs, config)
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		client := dgclient.NewDGateClient()
		if err := client.Init(server.URL, server.Client()); err != nil {
			t.Fatal(err)
		}

		if err := client.CreateRoute(&spec.Route{
			Name:          "test",
			NamespaceName: ns,
			Paths:         []string{"/test"},
			Methods:       []string{"GET"},
			Tags:          []string{"test123"},
		}); err == nil {
			t.Fatal("expected error")
		}
		if _, err := client.GetRoute("", ns); err == nil {
			t.Fatal("expected error")
		}
		if err := client.DeleteRoute("", ns); err == nil {
			t.Fatal("expected error")
		}
	}
}
