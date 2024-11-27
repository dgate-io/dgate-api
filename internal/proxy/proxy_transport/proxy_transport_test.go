package proxy_transport_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/dgate-io/dgate-api/internal/proxy/proxy_transport"
	"github.com/dgate-io/dgate-api/internal/proxy/proxytest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var proxyBuilder = proxy_transport.NewBuilder().
	RequestTimeout(5).
	RetryTimeout(5)

func TestDGateProxy(t *testing.T) {
	mockTp := proxytest.CreateMockTransport()
	header := make(http.Header)
	header.Add("X-Testing", "testing")
	mockTp.On("RoundTrip", mock.Anything).
		Return(nil, errors.New("testing error")).
		Times(4)
	mockTp.On("RoundTrip", mock.Anything).Return(&http.Response{
		StatusCode:    200,
		ContentLength: 0,
		Header:        header,
		Body:          io.NopCloser(strings.NewReader("")),
	}, nil).Once()

	numRetries := 5
	proxy, err := proxyBuilder.Clone().
		Transport(mockTp).Retries(numRetries).Build()
	if err != nil {
		t.Fatal(err)
	}
	req := &http.Request{
		URL:    &url.URL{},
		Header: header,
	}

	mockRw := proxytest.CreateMockResponseWriter()
	mockRw.On("Header").Return(header)
	mockRw.On("WriteHeader", mock.Anything).Return()
	req = req.WithContext(context.WithValue(context.Background(), proxytest.S("testing"), "testing"))
	proxy.RoundTrip(req)

	// ensure roundtrip is called at least once
	mockTp.AssertCalled(t, "RoundTrip", mock.Anything)
	// ensure retries are called
	assert.Equal(t, numRetries, mockTp.CallCount)
	// ensure context is passed through
	assert.Equal(t, "testing", req.Context().Value(proxytest.S("testing")))
}

func TestDGateProxyError(t *testing.T) {
	mockTp := proxytest.CreateMockTransport()
	header := make(http.Header)
	header.Add("X-Testing", "testing")
	mockTp.On("RoundTrip", mock.Anything).
		Return(nil, errors.New("testing error")).
		Times(4)
	mockTp.On("RoundTrip", mock.Anything).Return(&http.Response{
		StatusCode:    200,
		ContentLength: 0,
		Header:        header,
		Body:          io.NopCloser(strings.NewReader("")),
	}, nil).Once()

	proxy, err := proxyBuilder.Clone().
		Transport(mockTp).Build()
	if err != nil {
		t.Fatal(err)
	}
	req := &http.Request{
		URL:    &url.URL{},
		Header: header,
	}

	mockRw := proxytest.CreateMockResponseWriter()
	mockRw.On("Header").Return(header)
	mockRw.On("WriteHeader", mock.Anything).Return()
	req = req.WithContext(context.WithValue(context.Background(), proxytest.S("testing"), "testing"))
	proxy.RoundTrip(req)

	// ensure roundtrip is called at least once
	mockTp.AssertCalled(t, "RoundTrip", mock.Anything)
	// ensure context is passed through
	assert.Equal(t, "testing", req.Context().Value(proxytest.S("testing")))
}
