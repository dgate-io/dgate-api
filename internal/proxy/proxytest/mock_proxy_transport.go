package proxytest

import (
	"net/http"
	"time"

	"github.com/dgate-io/dgate-api/internal/proxy/proxy_transport"
	"github.com/stretchr/testify/mock"
)

type mockProxyTransportBuilder struct {
	mock.Mock
}

var _ proxy_transport.Builder = &mockProxyTransportBuilder{}

func CreateMockProxyTransportBuilder() *mockProxyTransportBuilder {
	return &mockProxyTransportBuilder{}
}

func (m *mockProxyTransportBuilder) Transport(
	transport http.RoundTripper,
) proxy_transport.Builder {
	m.Called(transport)
	return m
}

func (m *mockProxyTransportBuilder) RequestTimeout(
	requestTimeout time.Duration,
) proxy_transport.Builder {
	m.Called(requestTimeout)
	return m
}

func (m *mockProxyTransportBuilder) Retries(
	retries int,
) proxy_transport.Builder {
	m.Called(retries)
	return m
}

func (m *mockProxyTransportBuilder) RetryTimeout(
	retryTimeout time.Duration,
) proxy_transport.Builder {
	m.Called(retryTimeout)
	return m
}

func (b *mockProxyTransportBuilder) Clone() proxy_transport.Builder {
	args := b.Called()
	return args.Get(0).(proxy_transport.Builder)
}

func (m *mockProxyTransportBuilder) Build() (http.RoundTripper, error) {
	args := m.Called()
	return args.Get(0).(http.RoundTripper), args.Error(1)
}
