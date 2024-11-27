package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"

	"github.com/dgate-io/dgate-api/internal/config"
	"golang.org/x/net/http2"
)

func validateAddress(c *config.DGateHttpTransportConfig, address string) error {
	if c.DisablePrivateIPs {
		ip, _, err := net.SplitHostPort(address)
		if err != nil {
			ip = address
		}
		if ipAddr := net.ParseIP(ip); ipAddr == nil {
			return errors.New("could not parse IP: " + ip)
		} else if ipAddr.IsLoopback() || ipAddr.IsPrivate() {
			return errors.New("private IP address not allowed: " + ipAddr.String())
		}
	}
	return nil
}

func setupTranportsFromConfig(
	c *config.DGateHttpTransportConfig,
	modifyTransport func(*net.Dialer, *http.Transport),
) http.RoundTripper {
	t1 := http.DefaultTransport.(*http.Transport).Clone()
	t1.MaxIdleConns = c.MaxIdleConns
	t1.IdleConnTimeout = c.IdleConnTimeout
	t1.TLSHandshakeTimeout = c.TLSHandshakeTimeout
	t1.ExpectContinueTimeout = c.ExpectContinueTimeout
	t1.MaxIdleConnsPerHost = c.MaxIdleConnsPerHost
	t1.MaxConnsPerHost = c.MaxConnsPerHost
	t1.MaxResponseHeaderBytes = c.MaxResponseHeaderBytes
	t1.WriteBufferSize = c.WriteBufferSize
	t1.ReadBufferSize = c.ReadBufferSize
	t1.DisableKeepAlives = c.DisableKeepAlives
	t1.DisableCompression = c.DisableCompression
	t1.ForceAttemptHTTP2 = c.ForceAttemptHttp2
	t1.ResponseHeaderTimeout = c.ResponseHeaderTimeout
	dailer := &net.Dialer{
		Timeout:   c.DialTimeout,
		KeepAlive: c.KeepAlive,
	}
	if t1.DisableKeepAlives {
		dailer.KeepAlive = -1
	}
	resolver := &net.Resolver{
		PreferGo: c.DNSPreferGo,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			if err := validateAddress(c, address); err != nil {
				return nil, err
			}
			if c.DNSServer != "" {
				address = c.DNSServer
			}
			return dailer.DialContext(ctx, network, address)
		},
	}
	dailer.Resolver = resolver
	t1.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := dailer.DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		if err := validateAddress(c, conn.RemoteAddr().String()); err != nil {
			return nil, err
		}
		return conn, nil
	}
	t1.DialTLSContext = t1.DialContext
	modifyTransport(dailer, t1)
	return newRoundTripper(t1)
}

func newRoundTripper(transport *http.Transport) http.RoundTripper {
	transportH2C := &h2cTransport{
		transport: &http2.Transport{
			DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
			AllowHTTP: true,
		},
	}

	return &dynamicRoundTripper{
		hx:  transport,
		h2c: transportH2C,
	}
}

// dynamicRoundTripper implements RoundTrip while making sure that HTTP/2 is not used
// with protocols that start with a Connection Upgrade, such as SPDY or Websocket.
type dynamicRoundTripper struct {
	hx  *http.Transport
	h2c *h2cTransport
}

func (m *dynamicRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.ProtoAtLeast(2, 0) && (req.URL.Scheme == "h2c") {
		return m.h2c.RoundTrip(req)
	}
	return m.hx.RoundTrip(req)
}

type h2cTransport struct {
	transport *http2.Transport
}

func (t *h2cTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	return t.transport.RoundTrip(req)
}
