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

func TestDGClient_GetDocument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/document/test", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(&dgclient.ResponseWrapper[*spec.Document]{
			Data: &spec.Document{
				ID:             "test",
				CollectionName: "test",
			},
		})
	}))
	client := dgclient.NewDGateClient()
	err := client.Init(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	Document, err := client.GetDocument("test", "test", "test")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "test", Document.ID)
}

func TestDGClient_CreateDocument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/document", r.URL.Path)
		w.WriteHeader(http.StatusCreated)
	}))
	client := dgclient.NewDGateClient()
	err := client.Init(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	err = client.CreateDocument(&spec.Document{
		ID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDGClient_DeleteAllDocument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/document", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	client := dgclient.NewDGateClient()
	err := client.Init(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	err = client.DeleteAllDocument("test", "test")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDGClient_DeleteDocument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/document/test", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	client := dgclient.NewDGateClient()
	err := client.Init(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	err = client.DeleteDocument("test", "test", "test")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDGClient_ListDocument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/document", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(&dgclient.ResponseWrapper[[]*spec.Document]{
			Data: []*spec.Document{
				{
					ID: "test",
				},
			},
		})
	}))
	client := dgclient.NewDGateClient()
	err := client.Init(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	Documents, err := client.ListDocument("test", "test")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 1, len(Documents))
	assert.Equal(t, "test", Documents[0].ID)
}
