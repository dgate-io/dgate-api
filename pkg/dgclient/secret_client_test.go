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

func TestDGClient_GetSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/secret/test", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(&dgclient.ResponseWrapper[*spec.Secret]{
			Data: &spec.Secret{
				Name: "test",
			},
		})
	}))
	client := dgclient.NewDGateClient()
	err := client.Init(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	Secret, err := client.GetSecret("test", "test")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "test", Secret.Name)
}

func TestDGClient_CreateSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/secret", r.URL.Path)
		w.WriteHeader(http.StatusCreated)
	}))
	client := dgclient.NewDGateClient()
	err := client.Init(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	err = client.CreateSecret(&spec.Secret{
		Name: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDGClient_DeleteSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/secret", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	client := dgclient.NewDGateClient()
	err := client.Init(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	err = client.DeleteSecret("test", "test")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDGClient_ListSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/secret", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(&dgclient.ResponseWrapper[[]*spec.Secret]{
			Data: []*spec.Secret{
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

	Secrets, err := client.ListSecret("test")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 1, len(Secrets))
	assert.Equal(t, "test", Secrets[0].Name)
}
