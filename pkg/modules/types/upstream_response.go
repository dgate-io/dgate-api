package types

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/dgate-io/dgate-api/pkg/eventloop"
	"github.com/dgate-io/dgate-api/pkg/util"
	"github.com/dop251/goja"
)

type ResponseWrapper struct {
	response *http.Response
	loop     *eventloop.EventLoop

	Headers          http.Header `json:"headers"`
	StatusCode       int         `json:"statusCode"`
	StatusText       string      `json:"statusText"`
	Trailer          http.Header `json:"trailer"`
	Protocol         string      `json:"protocol"`
	Uncompressed     bool        `json:"uncompressed"`
	ContentLength    int64       `json:"contentLength"`
	TransferEncoding []string    `json:"transferEncoding"`
}

func NewResponseWrapper(
	resp *http.Response,
	loop *eventloop.EventLoop,
) *ResponseWrapper {
	return &ResponseWrapper{
		response:         resp,
		loop:             loop,
		Headers:          resp.Header,
		Protocol:         resp.Proto,
		StatusText:       resp.Status,
		Trailer:          resp.Trailer,
		StatusCode:       resp.StatusCode,
		Uncompressed:     resp.Uncompressed,
		ContentLength:    resp.ContentLength,
		TransferEncoding: resp.TransferEncoding,
	}
}

func (rw *ResponseWrapper) clearBody() {
	if rw.response.Body != nil {
		io.ReadAll(rw.response.Body)
		rw.response.Body.Close()
		rw.response.Body = nil
	}
	rw.response.ContentLength = 0
}

func (rw *ResponseWrapper) ReadBody() *goja.Promise {
	prom, res, rej := rw.loop.Runtime().NewPromise()
	rw.loop.RunOnLoop(func(r *goja.Runtime) {
		buf, err := io.ReadAll(rw.response.Body)
		if err != nil {
			rej(r.ToValue(errors.New(err.Error())))
			return
		}
		defer rw.response.Body.Close()
		res(r.ToValue(r.NewArrayBuffer(buf)))
	})
	return prom
}

func (rw *ResponseWrapper) ReadJson() *goja.Promise {
	prom, res, rej := rw.loop.Runtime().NewPromise()
	rw.loop.RunOnLoop(func(r *goja.Runtime) {
		var data any
		buf, err := io.ReadAll(rw.response.Body)
		if err != nil {
			rej(r.ToValue(errors.New(err.Error())))
			return
		}
		rw.response.Body.Close()
		err = json.Unmarshal(buf, &data)
		if err != nil {
			rej(r.ToValue(errors.New(err.Error())))
			return
		}
		res(r.ToValue(data))
	})
	return prom
}

func (rw *ResponseWrapper) WriteJson(data any) error {
	rw.Headers.Set("Content-Type", "application/json")
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return rw.WriteBody(b)
}

func (rw *ResponseWrapper) WriteBody(data any) error {
	rw.clearBody()
	if rw.StatusCode <= 0 {
		rw.StatusCode = http.StatusOK
		rw.response.Status = rw.StatusText
		if rw.response.Status == "" {
			rw.response.Status = http.StatusText(rw.StatusCode)
		}
	}
	rw.response.StatusCode = rw.StatusCode
	buf, err := util.ToBytes(data)
	if err != nil {
		return err
	}
	rw.response.ContentLength = int64(len(buf))
	rw.response.Header.Set("Content-Length", strconv.FormatInt(rw.response.ContentLength, 10))
	rw.response.Body = io.NopCloser(bytes.NewReader(buf))
	return nil
}

func (rw *ResponseWrapper) Status(status int) *ResponseWrapper {
	rw.response.StatusCode = status
	rw.StatusCode = rw.response.StatusCode
	rw.response.Status = http.StatusText(status)
	rw.StatusText = http.StatusText(status)
	return rw
}

func (rw *ResponseWrapper) Redirect(url string) {
	rw.clearBody()
	rw.Headers.Set("Location", url)
	rw.Status(http.StatusTemporaryRedirect)
}

func (rw *ResponseWrapper) RedirectPermanent(url string) {
	rw.clearBody()
	rw.Headers.Set("Location", url)
	rw.Status(http.StatusMovedPermanently)
}

func (rw *ResponseWrapper) Query() url.Values {
	return rw.response.Request.URL.Query()
}

func (rw *ResponseWrapper) Cookie() []*http.Cookie {
	return rw.response.Cookies()
}
