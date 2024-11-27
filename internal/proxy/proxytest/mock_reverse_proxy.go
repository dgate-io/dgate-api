package proxytest

import (
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/dgate-io/dgate-api/internal/proxy/reverse_proxy"
	"github.com/stretchr/testify/mock"
)

type mockReverseProxyExecutor struct {
	mock.Mock
}

type mockReverseProxyBuilder struct {
	mock.Mock
}

var _ http.Handler = &mockReverseProxyExecutor{}
var _ reverse_proxy.Builder = &mockReverseProxyBuilder{}

func CreateMockReverseProxyExecutor() *mockReverseProxyExecutor {
	return &mockReverseProxyExecutor{}
}

func (m *mockReverseProxyExecutor) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
) {
	m.Called(w, r)
}

func CreateMockReverseProxyBuilder() *mockReverseProxyBuilder {
	return &mockReverseProxyBuilder{}
}
func (m *mockReverseProxyBuilder) Transport(
	transport http.RoundTripper,
) reverse_proxy.Builder {
	m.Called(transport)
	return m
}

func (m *mockReverseProxyBuilder) RequestTimeout(
	requestTimeout time.Duration,
) reverse_proxy.Builder {
	m.Called(requestTimeout)
	return m
}

func (m *mockReverseProxyBuilder) FlushInterval(
	flushInterval time.Duration,
) reverse_proxy.Builder {
	m.Called(flushInterval)
	return m
}

func (m *mockReverseProxyBuilder) ModifyResponse(
	modifyResponse reverse_proxy.ModifyResponseFunc,
) reverse_proxy.Builder {
	m.Called(modifyResponse)
	return m
}

func (m *mockReverseProxyBuilder) ErrorHandler(
	errorHandler reverse_proxy.ErrorHandlerFunc,
) reverse_proxy.Builder {
	m.Called(errorHandler)
	return m
}

func (m *mockReverseProxyBuilder) ErrorLogger(
	logger *log.Logger,
) reverse_proxy.Builder {
	m.Called(logger)
	return m
}

func (m *mockReverseProxyBuilder) ProxyRewrite(
	stripPath bool,
	preserveHost bool,
	disableQueryParams bool,
	disableXForwardedHeaders bool,
) reverse_proxy.Builder {
	m.Called(stripPath, preserveHost, disableQueryParams, disableXForwardedHeaders)
	return m
}

func (m *mockReverseProxyBuilder) CustomRewrite(
	rewrite reverse_proxy.RewriteFunc,
) reverse_proxy.Builder {
	m.Called(rewrite)
	return m
}

func (m *mockReverseProxyBuilder) Clone() reverse_proxy.Builder {
	args := m.Called()
	return args.Get(0).(reverse_proxy.Builder)
}

func (m *mockReverseProxyBuilder) Build(
	upstreamUrl *url.URL, proxyPattern string,
) (http.Handler, error) {
	args := m.Called(upstreamUrl, proxyPattern)
	return args.Get(0).(http.Handler), args.Error(1)
}
