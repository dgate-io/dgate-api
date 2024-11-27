package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/dgate-io/dgate-api/pkg/modules"
	"github.com/dop251/goja"
)

type HttpModule struct {
	modCtx modules.RuntimeContext
}

var _ modules.GoModule = &HttpModule{}

func New(modCtx modules.RuntimeContext) modules.GoModule {
	return &HttpModule{
		modCtx,
	}
}

func (hp *HttpModule) Exports() *modules.Exports {
	return &modules.Exports{
		Named: map[string]any{
			"fetch": hp.FetchAsync,
		},
	}
}

type FetchOptionsRedirect string

const (
	Follow FetchOptionsRedirect = "follow"
	Error  FetchOptionsRedirect = "error"
	Manual FetchOptionsRedirect = "manual"
)

type FetchOptions struct {
	Method             string               `json:"method"`
	Body               string               `json:"body"`
	Headers            map[string]string    `json:"headers"`
	Redirect           FetchOptionsRedirect `json:"redirects"`
	Follow             int                  `json:"follow"`
	Compress           bool                 `json:"compress"`
	Size               int                  `json:"size"`
	Agent              string               `json:"agent"`
	HighWaterMark      int                  `json:"highWaterMark"`
	InsecureHTTPParser bool                 `json:"insecureHTTPParser"`
	// TODO: add options for timeout (signal which would require AbortController support)
}

func (hp *HttpModule) FetchAsync(url string, fetchOpts FetchOptions) (*goja.Promise, error) {
	loop := hp.modCtx.EventLoop()
	promise, resolve, reject := loop.Runtime().NewPromise()
	redirected := false
	var reader io.Reader
	client := http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if fetchOpts.Redirect == Manual {
				return http.ErrUseLastResponse
			} else if fetchOpts.Redirect == Error {
				return errors.New("redirects not allowed")
			}
			redirected = true
			return nil
		},
	}
	if fetchOpts.Body != "" {
		reader = bytes.NewReader([]byte(fetchOpts.Body))
	}
	req, err := http.NewRequest(fetchOpts.Method, url, reader)
	if err != nil {
		return nil, err
	} else {
		req.Header.Set("User-Agent", "DGate-Client/1.0")
		for k, v := range fetchOpts.Headers {
			req.Header.Set(k, v)
		}
	}
	resultsChan := asyncDo(client, req)
	bodyUsed := false
	loop.RunOnLoop(func(rt *goja.Runtime) {
		results := <-resultsChan
		if results.Error != nil {
			reject(rt.NewGoError(results.Error))
			return
		}
		resp := results.Data
		resolve(map[string]any{
			"_debug_time": results.Time.Seconds(),
			"status":      resp.StatusCode,
			"statusText":  resp.Status,
			"headers":     resp.Header,
			"body":        resp.Body,
			"bodyUsed":    &bodyUsed,
			"ok":          resp.StatusCode >= 200 && resp.StatusCode < 300,
			"url":         resp.Request.URL.String(),
			"redirected":  redirected,
			"json": func() (*goja.Promise, error) {
				bodyPromise, bodyResolve, bodyReject := rt.NewPromise()
				loop.RunOnLoop(func(_ *goja.Runtime) {
					if bodyUsed {
						bodyReject(rt.NewGoError(errors.New("body already used")))
						return
					}
					defer resp.Body.Close()
					var jsonData interface{}
					err := json.NewDecoder(resp.Body).Decode(&jsonData)
					if err != nil {
						bodyReject(err)
						return
					}
					bodyUsed = true
					bodyResolve(jsonData)
				})
				return bodyPromise, nil
			},
			"text": func() (*goja.Promise, error) {
				bodyPromise, bodyResolve, bodyReject := rt.NewPromise()
				loop.RunOnLoop(func(_ *goja.Runtime) {
					if bodyUsed {
						bodyReject(rt.NewGoError(errors.New("body already used")))
						return
					}
					defer resp.Body.Close()
					body, err := io.ReadAll(resp.Body)
					if err != nil {
						bodyReject(err)
						return
					}
					bodyResolve(string(body))
				})
				return bodyPromise, nil
			},
			"arrayBuffer": func() (*goja.Promise, error) {
				bodyPromise, bodyResolve, bodyReject := rt.NewPromise()
				loop.RunOnLoop(func(_ *goja.Runtime) {
					if bodyUsed {
						bodyReject(rt.NewGoError(errors.New("body already used")))
						return
					}
					defer resp.Body.Close()
					body, err := io.ReadAll(resp.Body)
					if err != nil {
						bodyReject(err)
						return
					}
					bodyUsed = true
					bodyResolve(rt.NewArrayBuffer(body))
				})
				return bodyPromise, nil
			},
		})
	})
	return promise, nil
}

type AsyncResults[T any] struct {
	Data  T
	Error error
	Time  time.Duration
}

func asyncDo(client http.Client, req *http.Request) chan AsyncResults[*http.Response] {
	ch := make(chan AsyncResults[*http.Response], 1)
	go func() {
		start := time.Now()
		if resp, err := client.Do(req); err != nil {
			ch <- AsyncResults[*http.Response]{Error: err}
		} else {
			ch <- AsyncResults[*http.Response]{
				Data: resp,
				Time: time.Since(start),
			}
		}
	}()
	return ch
}
