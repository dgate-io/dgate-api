package types

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"

	"github.com/dgate-io/dgate-api/pkg/eventloop"
	"github.com/dgate-io/dgate-api/pkg/util"
	"github.com/dop251/goja"
)

type RequestWrapper struct {
	req  *http.Request
	loop *eventloop.EventLoop

	Method        string
	Path          string
	Headers       http.Header
	Query         url.Values
	Host          string
	RemoteAddress string
	Proto         string
	ContentLength int64
}

func NewRequestWrapper(
	req *http.Request,
	loop *eventloop.EventLoop,
) *RequestWrapper {
	ip, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		ip = req.RemoteAddr
	}
	return &RequestWrapper{
		loop:  loop,
		req:   req,
		Query: req.URL.Query(),
		Path:  req.URL.Path,

		Host:          req.Host,
		Proto:         req.Proto,
		Headers:       req.Header,
		Method:        req.Method,
		RemoteAddress: ip,
		ContentLength: req.ContentLength,
	}
}

func (g *RequestWrapper) clearBody() {
	if g.req.Body != nil {
		// read all data from body
		io.ReadAll(g.req.Body)
		g.req.Body.Close()
		g.req.Body = nil
	}
}

func (g *RequestWrapper) WriteJson(data any) error {
	g.req.Header.Set("Content-Type", "application/json")
	buf, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return g.WriteBody(buf)
}

func (g *RequestWrapper) ReadJson() (any, error) {
	if ab, err := g.ReadBody(); err != nil {
		return nil, err
	} else {
		var data any
		err := json.Unmarshal(ab.Bytes(), &data)
		if err != nil {
			return nil, err
		}
		return data, nil

	}
}

func (g *RequestWrapper) WriteBody(data any) error {
	g.clearBody()
	buf, err := util.ToBytes(data)
	if err != nil {
		return err
	}
	g.req.Body = io.NopCloser(bytes.NewReader(buf))
	g.req.ContentLength = int64(len(buf))
	return nil
}

func (g *RequestWrapper) ReadBody() (*goja.ArrayBuffer, error) {
	if g.req.Body == nil {
		return nil, errors.New("body is not set")
	}
	buf, err := io.ReadAll(g.req.Body)
	if err != nil {
		return nil, err
	}
	defer g.req.Body.Close()
	arrBuf := g.loop.Runtime().NewArrayBuffer(buf)
	return &arrBuf, nil
}
