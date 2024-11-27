package reverse_proxy_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/dgate-io/dgate-api/internal/proxy/proxytest"
	"github.com/dgate-io/dgate-api/internal/proxy/reverse_proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type ProxyParams struct {
	host    string
	newHost string

	upstreamUrl   *url.URL
	newUpsteamURL *url.URL

	proxyPattern string
	proxyPath    string
}

type RewriteParams struct {
	stripPath          bool
	preserveHost       bool
	disableQueryParams bool
	xForwardedHeaders  bool
}

func testDGateProxyRewrite(
	t *testing.T, params ProxyParams,
	rewriteParams RewriteParams,
) {
	mockTp := proxytest.CreateMockTransport()
	header := make(http.Header)
	header.Add("X-Testing", "testing")
	rp, err := reverse_proxy.NewBuilder().
		Transport(mockTp).
		CustomRewrite(func(r1, r2 *http.Request) {
		}).
		ModifyResponse(func(r *http.Response) error {
			r.Header.Set("X-Testing-2",
				r.Header.Get("X-Testing"))
			return nil
		}).
		ErrorHandler(func(w http.ResponseWriter, r *http.Request, err error) {}).
		ErrorLogger(nil).
		ProxyRewrite(
			rewriteParams.stripPath,
			rewriteParams.preserveHost,
			rewriteParams.disableQueryParams,
			rewriteParams.xForwardedHeaders,
		).Build(params.upstreamUrl, params.proxyPattern)
	if err != nil {
		t.Fatal(err)
	}

	req := proxytest.CreateMockRequest("GET", params.proxyPath)
	req.RemoteAddr = "::1"
	req.Host = params.host
	req.URL.Scheme = ""
	req.URL.Host = ""
	mockRw := proxytest.CreateMockResponseWriter()
	mockRw.On("Header").Return(header)
	mockRw.On("WriteHeader", mock.Anything).Return()
	mockRw.On("Write", mock.Anything).Return(0, nil)
	req = req.WithContext(context.WithValue(
		context.Background(), proxytest.S("testing"), "testing"))

	mockTp.On("RoundTrip", mock.Anything).Run(func(args mock.Arguments) {
		req := args.Get(0).(*http.Request)
		if req.URL.String() != params.newUpsteamURL.String() {
			t.Errorf("FAIL: Expected URL %s, got %s", params.newUpsteamURL, req.URL)
		} else {
			t.Logf("PASS: upstreamUrl: %s, proxyPattern: %s, proxyPath: %s, newUpsteamURL: %s",
				params.upstreamUrl, params.proxyPattern, params.proxyPath, params.newUpsteamURL)
		}
		if params.newHost != "" && req.Host != params.newHost {
			t.Errorf("FAIL: Expected Host %s, got %s", params.newHost, req.Host)
		}
		if rewriteParams.xForwardedHeaders {
			if req.Header.Get("X-Forwarded-For") == "" {
				t.Errorf("FAIL: Expected X-Forwarded-For header, got empty")
			}
			if req.Header.Get("X-Real-IP") == "" {
				t.Errorf("FAIL: Expected X-Real-IP header, got empty")
			}
			if req.Header.Get("X-Forwarded-Proto") == "" {
				t.Errorf("FAIL: Expected X-Forwarded-Proto header, got empty")
			}
			if req.Header.Get("X-Forwarded-Host") == "" {
				t.Errorf("FAIL: Expected X-Forwarded-Host header, got empty")
			}
		} else {
			if req.Header.Get("X-Forwarded-For") != "" {
				t.Errorf("FAIL: Expected no X-Forwarded-For header, got %s", req.Header.Get("X-Fowarded-For"))
			}
			if req.Header.Get("X-Real-IP") != "" {
				t.Errorf("FAIL: Expected no X-Real-IP header, got %s", req.Header.Get("X-Real-IP"))
			}
			if req.Header.Get("X-Forwarded-Proto") != "" {
				t.Errorf("FAIL: Expected no X-Forwarded-Proto header, got %s", req.Header.Get("X-Forwarded-Proto"))
			}
			if req.Header.Get("X-Forwarded-Host") != "" {
				t.Errorf("FAIL: Expected no X-Forwarded-Host header, got %s", req.Header.Get("X-Forwarded-Host"))
			}
		}
	}).Return(&http.Response{
		StatusCode:    200,
		ContentLength: 0,
		Header:        header,
		Body:          io.NopCloser(strings.NewReader("")),
	}, nil).Once()

	rp.ServeHTTP(mockRw, req)

	// ensure roundtrip is called at least once
	mockTp.AssertExpectations(t)
	// ensure retries are called
	// ensure context is passed through
	assert.Equal(t, "testing", req.Context().Value(proxytest.S("testing")))
}

func mustParseURL(t *testing.T, s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
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

	upstreamUrl, _ := url.Parse("http://example.com")
	rp, err := reverse_proxy.NewBuilder().
		Clone().FlushInterval(-1).
		Transport(mockTp).
		ProxyRewrite(
			true, true,
			true, true,
		).Build(upstreamUrl, "/test/*")
	if err != nil {
		t.Fatal(err)
	}

	req := proxytest.CreateMockRequest("GET", "http://localhost:80")
	req.RemoteAddr = "::1"
	mockRw := proxytest.CreateMockResponseWriter()
	mockRw.On("Header").Return(header)
	mockRw.On("WriteHeader", mock.Anything).Return()
	mockRw.On("Write", mock.Anything).Return(0, nil)
	req = req.WithContext(context.WithValue(
		context.Background(), proxytest.S("testing"), "testing"))
	rp.ServeHTTP(mockRw, req)

	// ensure roundtrip is called at least once
	mockTp.AssertCalled(t, "RoundTrip", mock.Anything)
	// ensure retries are called
	// ensure context is passed through
	assert.Equal(t, "testing", req.Context().Value(proxytest.S("testing")))
}

func TestDGateProxyRewriteStripPath(t *testing.T) {
	// if proxy pattern is a prefix (ends with *)
	testDGateProxyRewrite(t, ProxyParams{
		host:    "test.net",
		newHost: "example.com",

		upstreamUrl:   mustParseURL(t, "http://example.com"),
		newUpsteamURL: mustParseURL(t, "http://example.com/test/ing"),

		proxyPattern: "/test/*",
		proxyPath:    "/test/test/ing",
	}, RewriteParams{
		stripPath:          true,
		preserveHost:       false,
		disableQueryParams: false,
		xForwardedHeaders:  false,
	})

	testDGateProxyRewrite(t, ProxyParams{
		host:    "test.net",
		newHost: "example.com",

		upstreamUrl:   mustParseURL(t, "http://example.com/pre"),
		newUpsteamURL: mustParseURL(t, "http://example.com/pre"),

		proxyPattern: "/test/*",
		proxyPath:    "/test/",
	}, RewriteParams{
		stripPath:          true,
		preserveHost:       false,
		disableQueryParams: false,
		xForwardedHeaders:  false,
	})
}

func TestDGateProxyRewritePreserveHost(t *testing.T) {
	testDGateProxyRewrite(t, ProxyParams{
		upstreamUrl:   mustParseURL(t, "http://example.com"),
		newUpsteamURL: mustParseURL(t, "http://example.com/test"),

		host:    "test.net",
		newHost: "test.net",

		proxyPattern: "/test",
		proxyPath:    "/test",
	}, RewriteParams{
		stripPath:          false,
		preserveHost:       true,
		disableQueryParams: false,
		xForwardedHeaders:  false,
	})
}

func TestDGateProxyRewriteDisableQueryParams(t *testing.T) {
	testDGateProxyRewrite(t, ProxyParams{
		upstreamUrl:   mustParseURL(t, "http://example.com"),
		newUpsteamURL: mustParseURL(t, "http://example.com/test"),

		host:    "test.net",
		newHost: "example.com",

		proxyPattern: "/test",
		proxyPath:    "/test?testing=testing",
	}, RewriteParams{
		stripPath:          false,
		preserveHost:       false,
		disableQueryParams: true,
		xForwardedHeaders:  false,
	})

	testDGateProxyRewrite(t, ProxyParams{
		upstreamUrl:   mustParseURL(t, "http://example.com"),
		newUpsteamURL: mustParseURL(t, "http://example.com/test?testing=testing"),

		host:    "test.net",
		newHost: "example.com",

		proxyPattern: "/test",
		proxyPath:    "/test?testing=testing",
	}, RewriteParams{
		stripPath:          false,
		preserveHost:       false,
		disableQueryParams: false,
		xForwardedHeaders:  false,
	})
}

func TestDGateProxyRewriteXForwardedHeaders(t *testing.T) {
	testDGateProxyRewrite(t, ProxyParams{
		upstreamUrl:   mustParseURL(t, "http://example.com"),
		newUpsteamURL: mustParseURL(t, "http://example.com/test"),

		host:    "test.net",
		newHost: "example.com",

		proxyPattern: "/test",
		proxyPath:    "/test",
	}, RewriteParams{
		stripPath:          false,
		preserveHost:       false,
		disableQueryParams: false,
		xForwardedHeaders:  true,
	})

	testDGateProxyRewrite(t, ProxyParams{
		upstreamUrl:   mustParseURL(t, "http://example.com"),
		newUpsteamURL: mustParseURL(t, "http://example.com/test"),

		host:    "test.net",
		newHost: "example.com",

		proxyPattern: "/test",
		proxyPath:    "/test",
	}, RewriteParams{
		stripPath:          false,
		preserveHost:       false,
		disableQueryParams: false,
		xForwardedHeaders:  false,
	})
}

func TestReverseProxy_DiffClone(t *testing.T) {
	builder1 := reverse_proxy.NewBuilder().FlushInterval(1)
	builder2 := builder1.Clone().FlushInterval(-1)

	assert.NotEqual(t, builder1, builder2)

	builder1.FlushInterval(-1)
	assert.Equal(t, builder1, builder2)
}

func TestReverseProxy_SameClone(t *testing.T) {
	builder1 := reverse_proxy.NewBuilder().
		CustomRewrite(func(r1, r2 *http.Request) {})
	builder2 := builder1.Clone()

	assert.NotEqual(t, builder1, builder2)

	builder1.CustomRewrite(nil)
	builder2.CustomRewrite(nil)
	assert.Equal(t, builder1, builder2)
}
