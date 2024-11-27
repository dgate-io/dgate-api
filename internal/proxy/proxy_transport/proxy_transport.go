package proxy_transport

import (
	"context"
	"net/http"
	"time"

	"errors"

	"github.com/dgate-io/dgate-api/internal/proxy/proxyerrors"
)

type Builder interface {
	Transport(transport http.RoundTripper) Builder
	RequestTimeout(requestTimeout time.Duration) Builder
	Retries(retries int) Builder
	RetryTimeout(retryTimeout time.Duration) Builder
	Clone() Builder
	Build() (http.RoundTripper, error)
}

type proxyTransportBuilder struct {
	transport      http.RoundTripper
	requestTimeout time.Duration
	retries        int
	retryTimeout   time.Duration
}

var _ Builder = (*proxyTransportBuilder)(nil)

func NewBuilder() Builder {
	return &proxyTransportBuilder{}
}

func (b *proxyTransportBuilder) Transport(transport http.RoundTripper) Builder {
	b.transport = transport
	return b
}

func (b *proxyTransportBuilder) RequestTimeout(requestTimeout time.Duration) Builder {
	b.requestTimeout = requestTimeout
	return b
}

func (b *proxyTransportBuilder) Retries(retries int) Builder {
	b.retries = retries
	return b
}

func (b *proxyTransportBuilder) RetryTimeout(retryTimeout time.Duration) Builder {
	b.retryTimeout = retryTimeout
	return b
}

func (b *proxyTransportBuilder) Clone() Builder {
	return &proxyTransportBuilder{
		transport:      b.transport,
		requestTimeout: b.requestTimeout,
		retries:        b.retries,
		retryTimeout:   b.requestTimeout,
	}
}

func (b *proxyTransportBuilder) Build() (http.RoundTripper, error) {
	return create(b.transport, b.requestTimeout, b.retries, b.retryTimeout)
}

func create(
	transport http.RoundTripper,
	requestTimeout time.Duration,
	retries int,
	retryTimeout time.Duration,
) (http.RoundTripper, error) {
	if retries < 0 {
		return nil, errors.New("retries must be greater than or equal to 0")
	}
	if retryTimeout < 0 {
		return nil, errors.New("retryTimeout must be greater than or equal to 0")
	}
	if transport == nil {
		transport = http.DefaultTransport
	}
	if requestTimeout < 0 {
		return nil, errors.New("requestTimeout must be greater than or equal to 0")
	}
	if requestTimeout == 0 && retries == 0 {
		return transport, nil
	}
	return &retryRoundTripper{
		transport:      transport,
		retries:        retries,
		retryTimeout:   retryTimeout,
		requestTimeout: requestTimeout,
	}, nil
}

type retryRoundTripper struct {
	transport      http.RoundTripper
	requestTimeout time.Duration
	retries        int
	retryTimeout   time.Duration
}

func (m *retryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var (
		resp *http.Response
		err  error
	)
	oreq := req
	for i := 0; i <= m.retries; i++ {
		if m.requestTimeout != 0 {
			ctx, cancel := context.WithTimeout(oreq.Context(), m.requestTimeout)
			req = req.WithContext(ctx)
			defer cancel()
		}
		resp, err = m.transport.RoundTrip(req)
		// Retry only on network errors or if the request is a PUT or POST
		if err == nil || req.Method == http.MethodPut || req.Method == http.MethodPost {
			break
		} else if pxyErr := proxyerrors.GetProxyError(err); pxyErr != nil {
			if !pxyErr.DisableRetry {
				break
			}
		}
		if m.retryTimeout != 0 {
			<-time.After(m.retryTimeout)
		}
	}
	return resp, err
}
