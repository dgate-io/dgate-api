package dgclient_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dgate-io/dgate-api/pkg/dgclient"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/stretchr/testify/assert"
)

func TestDGClient_GetRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/route/test", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(&dgclient.ResponseWrapper[*spec.Route]{
			Data: &spec.Route{
				Name:    "test",
				Paths:   []string{"/"},
				Methods: []string{"GET"},
			},
		})
	}))
	client := dgclient.NewDGateClient()
	err := client.Init(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	route, err := client.GetRoute("test", "test")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "test", route.Name)
	assert.Equal(t, []string{"/"}, route.Paths)
	assert.Equal(t, []string{"GET"}, route.Methods)
}

func TestDGClient_CreateRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/route", r.URL.Path)
		w.WriteHeader(http.StatusCreated)
	}))
	client := dgclient.NewDGateClient()
	err := client.Init(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	err = client.CreateRoute(&spec.Route{
		Name:    "test",
		Paths:   []string{"/"},
		Methods: []string{"GET"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDGClient_DeleteRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/route", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	client := dgclient.NewDGateClient()
	err := client.Init(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	err = client.DeleteRoute("test", "test")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDGClient_ListRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/route", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(&dgclient.ResponseWrapper[[]*spec.Route]{
			Data: []*spec.Route{
				{
					Name:    "test",
					Paths:   []string{"/"},
					Methods: []string{"GET"},
				},
			},
		})
	}))
	client := dgclient.NewDGateClient()
	err := client.Init(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	routes, err := client.ListRoute("test")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 1, len(routes))
	assert.Equal(t, "test", routes[0].Name)
	assert.Equal(t, []string{"/"}, routes[0].Paths)
	assert.Equal(t, []string{"GET"}, routes[0].Methods)
}
