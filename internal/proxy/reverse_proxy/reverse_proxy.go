package reverse_proxy

import (
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"time"
)

type RewriteFunc func(*http.Request, *http.Request)

type ModifyResponseFunc func(*http.Response) error

type ErrorHandlerFunc func(http.ResponseWriter, *http.Request, error)

type Builder interface {
	// FlushInterval sets the flush interval for flushable response bodies.
	FlushInterval(time.Duration) Builder

	// Rewrite sets the rewrite function for the reverse proxy.
	CustomRewrite(RewriteFunc) Builder

	// ModifyResponse sets the modify response function for the reverse proxy.
	ModifyResponse(ModifyResponseFunc) Builder

	// ErrorHandler sets the error handler function for the reverse proxy.
	ErrorHandler(ErrorHandlerFunc) Builder

	// Transport sets the transport for the reverse proxy.
	Transport(http.RoundTripper) Builder

	// ErrorLogger sets the (go) logger for the reverse proxy.
	ErrorLogger(*log.Logger) Builder

	// ProxyRewrite sets the proxy rewrite function for the reverse proxy.
	ProxyRewrite(
		stripPath bool,
		preserveHost bool,
		disableQueryParams bool,
		xForwardedHeaders bool,
	) Builder

	// Build builds the reverse proxy executor.
	Build(
		upstreamUrl *url.URL,
		proxyPattern string,
	) (http.Handler, error)

	// Clone clones the builder.
	Clone() Builder
}

var _ Builder = (*reverseProxyBuilder)(nil)

type reverseProxyBuilder struct {
	errorLogger        *log.Logger
	proxyRewrite       RewriteFunc
	customRewrite      RewriteFunc
	upstreamUrl        *url.URL
	proxyPattern       string
	transport          http.RoundTripper
	flushInterval      time.Duration
	modifyResponse     ModifyResponseFunc
	errorHandler       ErrorHandlerFunc
	stripPath          bool
	preserveHost       bool
	disableQueryParams bool
	xForwardedHeaders  bool
}

func NewBuilder() Builder {
	return &reverseProxyBuilder{}
}

func (b *reverseProxyBuilder) Clone() Builder {
	bb := *b
	return &bb
}

func (b *reverseProxyBuilder) FlushInterval(interval time.Duration) Builder {
	b.flushInterval = interval
	return b
}

func (b *reverseProxyBuilder) CustomRewrite(rewrite RewriteFunc) Builder {
	b.customRewrite = rewrite
	return b
}

func (b *reverseProxyBuilder) ModifyResponse(modifyResponse ModifyResponseFunc) Builder {
	b.modifyResponse = modifyResponse
	return b
}

func (b *reverseProxyBuilder) ErrorHandler(errorHandler ErrorHandlerFunc) Builder {
	b.errorHandler = errorHandler
	return b
}

func (b *reverseProxyBuilder) ErrorLogger(logger *log.Logger) Builder {
	b.errorLogger = logger
	return b
}

func (b *reverseProxyBuilder) Transport(transport http.RoundTripper) Builder {
	b.transport = transport
	return b
}

func (b *reverseProxyBuilder) ProxyRewrite(
	stripPath bool,
	preserveHost bool,
	disableQueryParams bool,
	xForwardedHeaders bool,
) Builder {
	b.stripPath = stripPath
	b.preserveHost = preserveHost
	b.disableQueryParams = disableQueryParams
	b.xForwardedHeaders = xForwardedHeaders
	return b
}

func (b *reverseProxyBuilder) rewriteStripPath(strip bool) RewriteFunc {
	return func(in, out *http.Request) {
		reqCall := in.URL.Path
		proxyPatternPath := b.proxyPattern
		upstreamPath := b.upstreamUrl.Path
		in.URL = b.upstreamUrl
		if strip {
			if strings.HasSuffix(proxyPatternPath, "*") {
				proxyPattern := strings.TrimSuffix(proxyPatternPath, "*")
				reqCallNoProxy := strings.TrimPrefix(reqCall, proxyPattern)
				out.URL.Path = path.Join(upstreamPath, reqCallNoProxy)
			} else {
				out.URL.Path = upstreamPath
			}
		} else {
			out.URL.Path = path.Join(upstreamPath, reqCall)
		}
	}
}

func (b *reverseProxyBuilder) rewritePreserveHost(preserve bool) RewriteFunc {
	return func(in, out *http.Request) {
		if preserve {
			out.Host = in.Host
			inHost := in.Header.Get("Host")
			if inHost == "" {
				inHost = in.Host
			}
			out.Header.Set("Host", inHost)
		} else {
			out.Host = b.upstreamUrl.Host
			out.Header.Set("Host", b.upstreamUrl.Host)
		}
	}
}

func (b *reverseProxyBuilder) rewriteDisableQueryParams(disableQueryParams bool) RewriteFunc {
	return func(in, out *http.Request) {
		if !disableQueryParams {
			targetQuery := b.upstreamUrl.RawQuery
			if targetQuery == "" || in.URL.RawQuery == "" {
				in.URL.RawQuery = targetQuery + in.URL.RawQuery
			} else {
				in.URL.RawQuery = targetQuery + "&" + in.URL.RawQuery
			}
		} else {
			out.URL.RawQuery = ""
		}
	}
}

func (b *reverseProxyBuilder) rewriteXForwardedHeaders(xForwardedHeaders bool) RewriteFunc {
	return func(in, out *http.Request) {
		if xForwardedHeaders {
			clientIP, _, err := net.SplitHostPort(in.RemoteAddr)
			if err == nil {
				out.Header.Add("X-Forwarded-For", clientIP)
				out.Header.Set("X-Real-IP", clientIP)
			} else {
				out.Header.Add("X-Forwarded-For", in.RemoteAddr)
				out.Header.Set("X-Real-IP", in.RemoteAddr)
			}
			out.Header.Set("X-Forwarded-Host", in.Host)
			if in.TLS == nil {
				out.Header.Set("X-Forwarded-Proto", "http")
			} else {
				out.Header.Set("X-Forwarded-Proto", "https")
			}
		} else {
			out.Header.Del("X-Forwarded-For")
			out.Header.Del("X-Forwarded-Host")
			out.Header.Del("X-Forwarded-Proto")
			out.Header.Del("X-Real-IP")
		}
	}
}

var (
	ErrNilUpstreamUrl    = errors.New("upstream url cannot be nil")
	ErrEmptyProxyPattern = errors.New("proxy pattern cannot be empty")
)

func (b *reverseProxyBuilder) Build(upstreamUrl *url.URL, proxyPattern string) (http.Handler, error) {
	if upstreamUrl == nil {
		return nil, ErrNilUpstreamUrl
	}
	b.upstreamUrl = upstreamUrl

	if proxyPattern == "" {
		return nil, ErrEmptyProxyPattern
	}
	b.proxyPattern = proxyPattern

	if b.transport == nil {
		b.transport = http.DefaultTransport
	}

	if b.flushInterval == 0 {
		b.flushInterval = time.Millisecond * 100
	}

	if b.errorHandler == nil {
		b.errorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
			http.Error(rw, err.Error(), http.StatusBadGateway)
		}
	}
	proxy := &httputil.ReverseProxy{}
	proxy.ErrorHandler = b.errorHandler
	proxy.FlushInterval = b.flushInterval
	proxy.ModifyResponse = b.modifyResponse
	proxy.Transport = b.transport
	proxy.ErrorLog = b.errorLogger
	proxy.Rewrite = func(pr *httputil.ProxyRequest) {
		// Ensure scheme and host are set correctly
		pr.Out.URL.Scheme = b.upstreamUrl.Scheme
		pr.Out.URL.Host = b.upstreamUrl.Host
		
		b.rewriteStripPath(b.stripPath)(pr.In, pr.Out)
		b.rewritePreserveHost(b.preserveHost)(pr.In, pr.Out)
		b.rewriteDisableQueryParams(b.disableQueryParams)(pr.In, pr.Out)
		b.rewriteXForwardedHeaders(b.xForwardedHeaders)(pr.In, pr.Out)
		if b.customRewrite != nil {
			b.customRewrite(pr.In, pr.Out)
		}
		if pr.Out.Host == "" {
			pr.Out.Host = upstreamUrl.Host
		}
	}
	return proxy, nil
}
