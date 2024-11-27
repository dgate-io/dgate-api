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

func TestAdminRoutes_Namespace(t *testing.T) {
	config := configtest.NewTest3DGateConfig()
	ps := proxy.NewProxyState(zap.NewNop(), config)
	if err := ps.Start(); err != nil {
		t.Fatal(err)
	}
	mux := chi.NewMux()
	mux.Route("/api/v1", func(r chi.Router) {
		routes.ConfigureNamespaceAPI(r, zap.NewNop(), ps, config)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	namespaces := []string{"_test", "default"}
	for _, ns := range namespaces {

		client := dgclient.NewDGateClient()
		if err := client.Init(server.URL, server.Client()); err != nil {
			t.Fatal(err)
		}

		if err := client.CreateNamespace(&spec.Namespace{
			Name: ns,
			Tags: []string{"test123"},
		}); err != nil {
			t.Fatal(err)
		}
		rm := ps.ResourceManager()
		if _, ok := rm.GetNamespace(ns); !ok {
			t.Fatal("namespace not found")
		}
		if namespaces, err := client.ListNamespace(); err != nil {
			t.Fatal(err)
		} else {
			nss := rm.GetNamespaces()
			assert.Equal(t, len(nss), len(namespaces))
			assert.Equal(t, spec.TransformDGateNamespaces(nss...), namespaces)
		}
		if namespace, err := client.GetNamespace(ns); err != nil {
			t.Fatal(err)
		} else {
			nss, ok := rm.GetNamespace(ns)
			assert.True(t, ok)
			namespace2 := spec.TransformDGateNamespace(nss)
			assert.Equal(t, namespace2, namespace)
		}
		if err := client.DeleteNamespace(ns); err != nil {
			t.Fatal(err)
		} else if _, ok := rm.GetNamespace(ns); ok {
			t.Fatal("namespace not deleted")
		}
	}
}

func TestAdminRoutes_NamespaceError(t *testing.T) {
	config := configtest.NewTest3DGateConfig()
	rm := resources.NewManager()
	cs := testutil.NewMockChangeState()
	cs.On("ApplyChangeLog", mock.Anything).
		Return(errors.New("test error"))
	cs.On("ResourceManager").Return(rm)
	mux := chi.NewMux()
	mux.Route("/api/v1", func(r chi.Router) {
		routes.ConfigureNamespaceAPI(r, zap.NewNop(), cs, config)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	namespaces := []string{"default", "test", ""}
	for _, ns := range namespaces {
		client := dgclient.NewDGateClient()
		if err := client.Init(server.URL, server.Client()); err != nil {
			t.Fatal(err)
		}

		if err := client.CreateNamespace(&spec.Namespace{
			Name: "test",
			Tags: []string{"test123"},
		}); err == nil {
			t.Fatal("expected error")
		}
		if _, err := client.GetNamespace(ns); err == nil {
			t.Fatal("expected error")
		}
		if err := client.DeleteNamespace(ns); err == nil {
			t.Fatal("expected error")
		}
	}
}
