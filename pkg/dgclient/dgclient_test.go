package dgclient_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dgate-io/dgate-api/pkg/dgclient"
	"github.com/stretchr/testify/assert"
)

func TestDGClient_OptionsWithRedirect(t *testing.T) {
	var client = dgclient.NewDGateClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/service/test" {
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"name":"test"}`))
		}
	}))
	defer server.Close()

	err := client.Init(server.URL, server.Client(),
		dgclient.WithFollowRedirect(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.GetService("test", "default")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDGClient_OptionsRedirectError(t *testing.T) {
	var client = dgclient.NewDGateClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, _ := r.BasicAuth(); user != "user" || pass != "password" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/api/v1/service/test" {
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"name":"test"}`))
		}
	}))
	defer server.Close()

	err := client.Init(server.URL, server.Client(),
		dgclient.WithBasicAuth("user", "password"),
		dgclient.WithFollowRedirect(false),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.GetService("test", "default")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDGClient_OptionsWithBasicAuth(t *testing.T) {
	var client = dgclient.NewDGateClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, _ := r.BasicAuth(); user != "user" || pass != "password" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"name":"test"}`))
		}
	}))
	defer server.Close()

	err := client.Init(server.URL, server.Client(),
		dgclient.WithVerboseLogging(true),
		dgclient.WithBasicAuth("user", "password"),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.GetService("test", "default")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDGClient_OptionsBasicAuthError(t *testing.T) {
	var client = dgclient.NewDGateClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, _ := r.BasicAuth(); user != "user" || pass != "password" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"name":"test"}`))
		}
	}))
	defer server.Close()

	err := client.Init(server.URL, server.Client(),
		dgclient.WithBasicAuth("user", "wrongpassword"),
		dgclient.WithVerboseLogging(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.GetService("test", "default")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDGClient_OptionsWithUserAgent(t *testing.T) {
	var client = dgclient.NewDGateClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != "test" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"name":"test"}`))
		}
	}))
	defer server.Close()

	err := client.Init(server.URL, server.Client(),
		dgclient.WithVerboseLogging(true),
		dgclient.WithUserAgent("test"),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.GetService("test", "default")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDGClient_OptionsWithUserAgent2(t *testing.T) {
	var client = dgclient.NewDGateClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != "test" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"name":"test"}`))
		}
	}))
	defer server.Close()

	err := client.Init(server.URL, server.Client(),
		dgclient.WithUserAgent("test"),
		dgclient.WithVerboseLogging(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.GetService("test", "default")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDGClient_Init_ParseURLError(t *testing.T) {
	var client = dgclient.NewDGateClient()
	err := client.Init("://#/:asdm", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDGClient_Init_EmptyHostError(t *testing.T) {
	var client = dgclient.NewDGateClient()
	err := client.Init("", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	assert.Equal(t, "host is empty", err.Error())
}
