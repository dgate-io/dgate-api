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

func TestDGClient_GetCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/collection/test", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(&dgclient.ResponseWrapper[*spec.Collection]{
			Data: &spec.Collection{
				Name: "test",
			},
		})
	}))
	client := dgclient.NewDGateClient()
	err := client.Init(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	Collection, err := client.GetCollection("test", "test")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "test", Collection.Name)
}

func TestDGClient_CreateCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/collection", r.URL.Path)
		w.WriteHeader(http.StatusCreated)
	}))
	client := dgclient.NewDGateClient()
	err := client.Init(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	err = client.CreateCollection(&spec.Collection{
		Name: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDGClient_DeleteCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/collection", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	client := dgclient.NewDGateClient()
	err := client.Init(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	err = client.DeleteCollection("test", "test")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDGClient_ListCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/collection", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(&dgclient.ResponseWrapper[[]*spec.Collection]{
			Data: []*spec.Collection{
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

	Collections, err := client.ListCollection("test")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 1, len(Collections))
	assert.Equal(t, "test", Collections[0].Name)
}
