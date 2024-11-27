package proxy_test

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"testing"

	"github.com/dgate-io/dgate-api/internal/config/configtest"
	"github.com/dgate-io/dgate-api/internal/proxy"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Raft Test -> ApplyChangeLog, WaitForChanges,
//   CaptureState, EnableRaft, Raft, PersistState, RestoreState,

// DynamicTLSConfig

func TestDynamicTLSConfig_DomainCert(t *testing.T) {
	conf := configtest.NewTestDGateConfig_DomainAndNamespaces()
	ps := proxy.NewProxyState(zap.NewNop(), conf)

	tlsConfig := ps.DynamicTLSConfig("", "")
	clientHello := &tls.ClientHelloInfo{
		ServerName: "abc.test.com",
	}
	cert, err := tlsConfig.GetCertificate(clientHello)
	if !assert.Nil(t, err, "error should be nil") {
		t.Fatal(err)
	}
	if !assert.NotNil(t, cert, "should not be nil") {
		return
	}
}

func TestDynamicTLSConfig_DomainCertCache(t *testing.T) {
	conf := configtest.NewTestDGateConfig_DomainAndNamespaces()
	ps := proxy.NewProxyState(zap.NewNop(), conf)
	if err := ps.Start(); err != nil {
		t.Fatal(err)
	}
	domains := ps.ResourceManager().GetDomainsByPriority()
	if !assert.NotEqual(t, len(domains), 0) {
		return
	}
	d := domains[0]
	key := fmt.Sprintf("cert:%s:%s:%d", d.Namespace.Name,
		d.Name, d.CreatedAt.Unix())
	tlsConfig := ps.DynamicTLSConfig("", "")
	clientHello := &tls.ClientHelloInfo{
		ServerName: "abc.test.com",
	}
	cert, err := tlsConfig.GetCertificate(clientHello)
	if !assert.Nil(t, err, "error should be nil") {
		t.Fatal(err)
	}
	if !assert.NotNil(t, cert, "should not be nil") {
		return
	}
	// check cache
	item, ok := ps.SharedCache().Bucket("certs").Get(key)
	if !assert.True(t, ok, "should be true") {
		return
	}
	if _, ok = item.(*tls.Certificate); !ok {
		t.Fatal("should be tls.Certificate")
	}

}

func TestDynamicTLSConfig_Fallback(t *testing.T) {
	conf := configtest.NewTestDGateConfig_DomainAndNamespaces()
	ps := proxy.NewProxyState(zap.NewNop(), conf)

	tlsConfig := ps.DynamicTLSConfig("testdata/server.crt", "testdata/server.key")
	// this should have a match that is not the fallback
	clientHello := &tls.ClientHelloInfo{
		ServerName: "abc.test.com",
	}
	cert, err := tlsConfig.GetCertificate(clientHello)
	if !assert.Nil(t, err, "error should be nil") {
		t.Fatal(err)
	}
	if !assert.NotNil(t, cert, "should not be nil") {
		return
	}
	// this should have a match that is the fallback
	clientHello = &tls.ClientHelloInfo{
		ServerName: "nomatch.com",
	}
	cert, err = tlsConfig.GetCertificate(clientHello)
	if !assert.Nil(t, err, "error should be nil") {
		t.Fatal(err)
	}
	if !assert.NotNil(t, cert, "should not be nil") {
		return
	}
}

func TestFindNamespaceByRequest_OneNamespaceNoDomain(t *testing.T) {
	conf := configtest.NewTestDGateConfig()
	ps := proxy.NewProxyState(zap.NewNop(), conf)
	if err := ps.Store().InitStore(); err != nil {
		t.Fatal(err)
	}
	hostNsPair := map[string]string{
		"":             "test",
		"test.com":     "test",
		"abc.test.com": "test",
	}
	for testHost, nsName := range hostNsPair {
		if req, err := http.NewRequest(http.MethodGet, "/test", nil); err != nil {
			t.Fatal(err)
		} else {
			req.Host = testHost
			n := ps.FindNamespaceByRequest(req)
			if assert.NotNil(t, n, "should not be nil") {
				assert.Equal(t, n.Name, nsName, "expected namespace %s, got %s", nsName, n.Name)
			}
		}
	}
}

func TestFindNamespaceByRequest_DomainsAndNamespaces(t *testing.T) {
	conf := configtest.NewTestDGateConfig_DomainAndNamespaces()
	ps := proxy.NewProxyState(zap.NewNop(), conf)
	if err := ps.Store().InitStore(); err != nil {
		t.Fatal(err)
	}
	hostNsPair := map[string]any{
		"":             nil,
		"test.com.jp":  nil,
		"nomatch.com":  nil,
		"example.com":  "test",
		"any.test.com": "test2",
		"abc.test.com": "test3",
	}
	for testHost, nsName := range hostNsPair {
		if req, err := http.NewRequest(http.MethodGet, "/test", nil); err != nil {
			t.Fatal(err)
		} else {
			req.Host = testHost
			if n := ps.FindNamespaceByRequest(req); nsName == nil {
				assert.Nil(t, n, "should be nil when host is '%s'", testHost)
			} else if assert.NotNil(t, n, "should not be nil") {
				assert.Equal(t, n.Name, nsName, "expected namespace %s, got %s", nsName, n.Name)
			}
		}
	}
}
func TestFindNamespaceByRequest_DomainsAndNamespacesDefault(t *testing.T) {
	conf := configtest.NewTestDGateConfig_DomainAndNamespaces2()
	ps := proxy.NewProxyState(zap.NewNop(), conf)
	if err := ps.Store().InitStore(); err != nil {
		t.Fatal(err)
	}
	hostNsPair := map[string]any{
		"":             "default",
		"nomatch.com":  "default",
		"test.com.jp":  "default",
		"example.com":  "test",
		"any.test.com": "test2",
		"abc.test.com": "test3",
	}
	for testHost, nsName := range hostNsPair {
		if req, err := http.NewRequest(http.MethodGet, "/test", nil); err != nil {
			t.Fatal(err)
		} else {
			req.Host = testHost
			if n := ps.FindNamespaceByRequest(req); nsName == nil {
				assert.Nil(t, n, "should be nil when host is '%s'", testHost)
			} else if assert.NotNil(t, n, "should not be nil") {
				assert.Equal(t, n.Name, nsName, "expected namespace %s, got %s", nsName, n.Name)
			}
		}
	}
}

// ApplyChangeLog

// func TestApplyChangeLog(t *testing.T) {
// 	conf := configtest.NewTestDGateConfig()
// 	ps := proxy.NewProxyState(zap.NewNop(), conf)
// 	if err := ps.Store().InitStore(); err != nil {
// 		t.Fatal(err)
// 	}
// 	err := ps.ApplyChangeLog(nil)
// 	assert.Nil(t, err, "error should be nil")
// }

func TestProcessChangeLog_RMSecrets(t *testing.T) {
	conf := configtest.NewTestDGateConfig()
	ps := proxy.NewProxyState(zap.NewNop(), conf)
	if err := ps.Store().InitStore(); err != nil {
		t.Fatal(err)
	}

	sc := &spec.Secret{
		Name:          "test",
		NamespaceName: "test",
		Data:          "YWJj",
	}

	cl := spec.NewChangeLog(sc, sc.NamespaceName, spec.AddSecretCommand)
	err := ps.ProcessChangeLog(cl, true)
	if !assert.Nil(t, err, "error should be nil") {
		return
	}
	secrets := ps.ResourceManager().GetSecrets()
	assert.Equal(t, 1, len(secrets), "should have 1 item")
	assert.Equal(t, sc.Name, secrets[0].Name, "should have the same name")
	assert.Equal(t, sc.NamespaceName, secrets[0].Namespace.Name, "should have the same namespace")
	// 'YWJj' is base64 encoded 'abc'
	assert.Equal(t, secrets[0].Data, "abc", "should have the same data")

	secrets = ps.ResourceManager().GetSecretsByNamespace(sc.NamespaceName)
	assert.Equal(t, 1, len(secrets), "should have 1 item")
	assert.Equal(t, sc.Name, secrets[0].Name, "should have the same name")
	assert.Equal(t, sc.NamespaceName, secrets[0].Namespace.Name, "should have the same namespace")
	// 'YWJj' is base64 encoded 'abc'
	assert.Equal(t, secrets[0].Data, "abc", "should have the same data")

	cl = spec.NewChangeLog(sc, sc.NamespaceName, spec.DeleteSecretCommand)
	err = ps.ProcessChangeLog(cl, true)
	if !assert.Nil(t, err, "error should be nil") {
		return
	}
	secrets = ps.ResourceManager().GetSecrets()
	assert.Equal(t, 0, len(secrets), "should have 0 item")

}

func TestProcessChangeLog_Route(t *testing.T) {
	conf := configtest.NewTestDGateConfig()
	ps := proxy.NewProxyState(zap.NewNop(), conf)
	if err := ps.Store().InitStore(); err != nil {
		t.Fatal(err)
	}

	r := &spec.Route{
		Name:          "test",
		NamespaceName: "test",
		Paths:         []string{"/test"},
		Methods:       []string{"GET"},
		ServiceName:   "test",
		Modules:       []string{"test"},
		Tags:          []string{"test"},
	}

	cl := spec.NewChangeLog(r, r.NamespaceName, spec.AddRouteCommand)
	err := ps.ProcessChangeLog(cl, false)
	if !assert.Nil(t, err, "error should be nil") {
		return
	}
	routes := ps.ResourceManager().GetRoutes()
	assert.Equal(t, 1, len(routes), "should have 1 item")
	assert.Equal(t, r.Name, routes[0].Name, "should have the same name")
	assert.Equal(t, r.NamespaceName, routes[0].Namespace.Name, "should have the same namespace")
	assert.Equal(t, r.Paths, routes[0].Paths, "should have the same paths")
	assert.Equal(t, r.Methods, routes[0].Methods, "should have the same methods")
	assert.Equal(t, r.ServiceName, routes[0].Service.Name, "should have the same service")
	assert.Equal(t, len(r.Modules), len(routes[0].Modules), "should have the same modules")
	assert.Equal(t, len(r.Tags), len(routes[0].Tags), "should have the same tags")

	cl = spec.NewChangeLog(r, r.NamespaceName, spec.DeleteRouteCommand)
	err = ps.ProcessChangeLog(cl, false)
	if !assert.Nil(t, err, "error should be nil") {
		return
	}
	routes = ps.ResourceManager().GetRoutes()
	assert.Equal(t, 0, len(routes), "should have 0 item")
}

func TestProcessChangeLog_Service(t *testing.T) {
	conf := configtest.NewTest4DGateConfig()
	ps := proxy.NewProxyState(zap.NewNop(), conf)
	if err := ps.Store().InitStore(); err != nil {
		t.Fatal(err)
	}

	s := &spec.Service{
		Name:          "test123",
		NamespaceName: "test",
		URLs:          []string{"http://localhost:8080"},
		Tags:          []string{"test"},
	}

	cl := spec.NewChangeLog(s, s.NamespaceName, spec.AddServiceCommand)
	err := ps.ProcessChangeLog(cl, false)
	if !assert.Nil(t, err, "error should be nil") {
		return
	}
	services := ps.ResourceManager().GetServices()
	assert.Equal(t, 1, len(services), "should have 1 item")
	assert.Equal(t, s.Name, services[0].Name, "should have the same name")
	assert.Equal(t, s.NamespaceName, services[0].Namespace.Name, "should have the same namespace")
	assert.Equal(t, len(s.URLs), len(services[0].URLs), "should have the same urls")
	assert.Equal(t, len(s.Tags), len(services[0].Tags), "should have the same tags")

	cl = spec.NewChangeLog(s, s.NamespaceName, spec.DeleteServiceCommand)
	err = ps.ProcessChangeLog(cl, false)
	if !assert.Nil(t, err, "error should be nil") {
		return
	}
	services = ps.ResourceManager().GetServices()
	assert.Equal(t, 0, len(services), "should have 0 item")
}

func TestProcessChangeLog_Module(t *testing.T) {
	conf := configtest.NewTest4DGateConfig()
	ps := proxy.NewProxyState(zap.NewNop(), conf)
	if err := ps.Store().InitStore(); err != nil {
		t.Fatal(err)
	}

	m := &spec.Module{
		Name:          "test123",
		NamespaceName: "test",
		Payload:       "",
		Tags:          []string{"test"},
	}

	cl := spec.NewChangeLog(m, m.NamespaceName, spec.AddModuleCommand)
	err := ps.ProcessChangeLog(cl, false)
	if !assert.Nil(t, err, "error should be nil") {
		return
	}
	modules := ps.ResourceManager().GetModules()
	assert.Equal(t, 1, len(modules), "should have 1 item")
	assert.Equal(t, m.Name, modules[0].Name, "should have the same name")
	assert.Equal(t, m.NamespaceName, modules[0].Namespace.Name, "should have the same namespace")
	assert.Equal(t, m.Payload, modules[0].Payload, "should have the same payload")
	assert.Equal(t, len(m.Tags), len(modules[0].Tags), "should have the same tags")

	cl = spec.NewChangeLog(m, m.NamespaceName, spec.DeleteModuleCommand)
	err = ps.ProcessChangeLog(cl, false)
	if !assert.Nil(t, err, "error should be nil") {
		return
	}
	modules = ps.ResourceManager().GetModules()
	assert.Equal(t, 0, len(modules), "should have 0 item")
}

func TestProcessChangeLog_Namespace(t *testing.T) {
	ps := proxy.NewProxyState(zap.NewNop(), configtest.NewTest4DGateConfig())
	if err := ps.Store().InitStore(); err != nil {
		t.Fatal(err)
	}

	n := &spec.Namespace{
		Name: "test_new",
	}

	cl := spec.NewChangeLog(n, n.Name, spec.AddNamespaceCommand)
	err := ps.ProcessChangeLog(cl, false)
	if !assert.Nil(t, err, "error should be nil") {
		return
	}
	ns, ok := ps.ResourceManager().GetNamespace(n.Name)
	if !assert.True(t, ok, "should be true") {
		return
	}
	assert.Equal(t, n.Name, ns.Name, "should have the same name")

	cl = spec.NewChangeLog(n, n.Name, spec.DeleteNamespaceCommand)
	err = ps.ProcessChangeLog(cl, false)
	if !assert.Nil(t, err, "error should be nil") {
		return
	}
	_, ok = ps.ResourceManager().GetNamespace(n.Name)
	assert.False(t, ok, "should be false")
}

func TestProcessChangeLog_Collection(t *testing.T) {
	conf := configtest.NewTest4DGateConfig()
	ps := proxy.NewProxyState(zap.NewNop(), conf)
	if err := ps.Store().InitStore(); err != nil {
		t.Fatal(err)
	}

	c := &spec.Collection{
		Name:          "test123",
		NamespaceName: "test",
		// Type:          spec.CollectionTypeDocument,
		Visibility: spec.CollectionVisibilityPrivate,
		Tags:       []string{"test"},
	}

	cl := spec.NewChangeLog(c, c.NamespaceName, spec.AddCollectionCommand)
	err := ps.ProcessChangeLog(cl, false)
	if !assert.Nil(t, err, "error should be nil") {
		return
	}
	collections := ps.ResourceManager().GetCollections()
	assert.Equal(t, 1, len(collections), "should have 1 item")
	assert.Equal(t, c.Name, collections[0].Name, "should have the same name")
	assert.Equal(t, c.NamespaceName, collections[0].Namespace.Name, "should have the same namespace")
	// assert.Equal(t, c.Type, collections[0].Type, "should have the same type")
	assert.Equal(t, c.Visibility, collections[0].Visibility, "should have the same visibility")
	assert.Equal(t, len(c.Tags), len(collections[0].Tags), "should have the same tags")

	cl = spec.NewChangeLog(c, c.NamespaceName, spec.DeleteCollectionCommand)
	err = ps.ProcessChangeLog(cl, false)
	if !assert.Nil(t, err, "error should be nil") {
		return
	}
	collections = ps.ResourceManager().GetCollections()
	assert.Equal(t, 0, len(collections), "should have 0 item")
}

func TestProcessChangeLog_Document(t *testing.T) {
	conf := configtest.NewTestDGateConfig()
	ps := proxy.NewProxyState(zap.NewNop(), conf)
	if err := ps.Store().InitStore(); err != nil {
		t.Fatal(err)
	}
	c := &spec.Collection{
		Name:          "test123",
		NamespaceName: "test",
		Type:          spec.CollectionTypeDocument,
		Visibility:    spec.CollectionVisibilityPrivate,
		Tags:          []string{"test"},
	}

	cl := spec.NewChangeLog(c, c.NamespaceName, spec.AddCollectionCommand)
	err := ps.ProcessChangeLog(cl, false)
	if !assert.Nil(t, err, "error should be nil") {
		return
	}

	d := &spec.Document{
		ID:             "test123",
		CollectionName: "test123",
		NamespaceName:  "test",
		Data:           "",
	}

	cl = spec.NewChangeLog(d, d.NamespaceName, spec.AddDocumentCommand)
	err = ps.ProcessChangeLog(cl, true)
	if !assert.Nil(t, err, "error should be nil") {
		return
	}
	documents, err := ps.DocumentManager().GetDocuments(
		d.CollectionName, d.NamespaceName, 999, 0,
	)
	if !assert.Nil(t, err, "error should be nil") {
		return
	}
	require.Equal(t, 1, len(documents), "should have 1 item")
	assert.Equal(t, d.ID, documents[0].ID, "should have the same id")
	assert.Equal(t, d.NamespaceName, documents[0].NamespaceName, "should have the same namespace")
	assert.Equal(t, d.CollectionName, documents[0].CollectionName, "should have the same collection")
	assert.Equal(t, d.Data, documents[0].Data, "should have the same data")

	cl = spec.NewChangeLog(d, d.NamespaceName, spec.DeleteDocumentCommand)
	err = ps.ProcessChangeLog(cl, false)
	if !assert.Nil(t, err, "error should be nil") {
		return
	}
	documents, err = ps.DocumentManager().GetDocuments(
		d.CollectionName, d.NamespaceName, 999, 0,
	)
	if !assert.Nil(t, err, "error should be nil") {
		return
	}
	assert.Equal(t, 0, len(documents), "should have 0 item")
}
