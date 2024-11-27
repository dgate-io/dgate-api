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

func TestDGClient_GetDomain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/domain/test", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(&dgclient.ResponseWrapper[*spec.Domain]{
			Data: &spec.Domain{
				Name: "test",
			},
		})
	}))
	client := dgclient.NewDGateClient()
	err := client.Init(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	Domain, err := client.GetDomain("test", "test")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "test", Domain.Name)
}

func TestDGClient_CreateDomain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/domain", r.URL.Path)
		w.WriteHeader(http.StatusCreated)
	}))
	client := dgclient.NewDGateClient()
	err := client.Init(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	err = client.CreateDomain(&spec.Domain{
		Name: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDGClient_DeleteDomain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/domain", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	client := dgclient.NewDGateClient()
	err := client.Init(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	err = client.DeleteDomain("test", "test")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDGClient_ListDomain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/domain", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(&dgclient.ResponseWrapper[[]*spec.Domain]{
			Data: []*spec.Domain{
				{
					Name: "test",
				},
			},
		})
	}))
	client := dgclient.NewDGateClient()
	err := client.Init(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	Domains, err := client.ListDomain("test")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 1, len(Domains))
	assert.Equal(t, "test", Domains[0].Name)
}
