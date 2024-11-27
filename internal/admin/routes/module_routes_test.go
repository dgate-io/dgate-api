package routes_test

import (
	"encoding/base64"
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

func TestAdminRoutes_Module(t *testing.T) {
	namespaces := []string{"default", "test"}
	config := configtest.NewTest4DGateConfig()
	ps := proxy.NewProxyState(zap.NewNop(), config)
	if err := ps.Start(); err != nil {
		t.Fatal(err)
	}
	mux := chi.NewMux()
	mux.Route("/api/v1", func(r chi.Router) {
		routes.ConfigureModuleAPI(r, zap.NewNop(), ps, config)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	for _, ns := range namespaces {
		client := dgclient.NewDGateClient()
		if err := client.Init(server.URL, server.Client()); err != nil {
			t.Fatal(err)
		}

		if err := client.CreateModule(&spec.Module{
			Name:          "test",
			NamespaceName: ns,
			Payload: base64.StdEncoding.EncodeToString(
				[]byte("\"use test\""),
			),
			Type: spec.ModuleTypeJavascript,
			Tags: []string{"test123"},
		}); err != nil {
			t.Fatal(err)
		}
		rm := ps.ResourceManager()
		if _, ok := rm.GetModule("test", ns); !ok {
			t.Fatal("module not found")
		}
		if modules, err := client.ListModule(ns); err != nil {
			t.Fatal(err)
		} else {
			mods := rm.GetModulesByNamespace(ns)
			assert.Equal(t, len(mods), len(modules))
			assert.Equal(t, spec.TransformDGateModules(mods...), modules)
		}
		if module, err := client.GetModule("test", ns); err != nil {
			t.Fatal(err)
		} else {
			mod, ok := rm.GetModule("test", ns)
			assert.True(t, ok)
			module2 := spec.TransformDGateModule(mod)
			assert.Equal(t, module2, module)
		}
		if err := client.DeleteModule("test", ns); err != nil {
			t.Fatal(err)
		} else if _, ok := rm.GetModule("test", ns); ok {
			t.Fatal("module not deleted")
		}
	}
}

func TestAdminRoutes_ModuleError(t *testing.T) {
	config := configtest.NewTest3DGateConfig()
	cs := testutil.NewMockChangeState()
	rm := resources.NewManager()
	cs.On("ApplyChangeLog", mock.Anything).
		Return(errors.New("test error"))
	cs.On("ResourceManager").Return(rm)
	mux := chi.NewMux()
	mux.Route("/api/v1", func(r chi.Router) {
		routes.ConfigureModuleAPI(r, zap.NewNop(), cs, config)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	namespaces := []string{"default", "test", ""}
	for _, ns := range namespaces {
		client := dgclient.NewDGateClient()
		if err := client.Init(server.URL, server.Client()); err != nil {
			t.Fatal(err)
		}

		if err := client.CreateModule(&spec.Module{
			Name:          "test",
			NamespaceName: ns,
			Payload:       "\"use test\"",
			Type:          spec.ModuleTypeJavascript,
			Tags:          []string{"test123"},
		}); err == nil {
			t.Fatal("expected error")
		}
		if _, err := client.GetModule("", ns); err == nil {
			t.Fatal("expected error")
		}
		if err := client.DeleteModule("", ns); err == nil {
			t.Fatal("expected error")
		}
	}
}
