package extractors

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dgate-io/dgate-api/pkg/eventloop"
	"github.com/dgate-io/dgate-api/pkg/modules/types"
	"github.com/dop251/goja"
)

type (
	// this can be used to create custom load balancing strategies, by default it uses round robin
	FetchUpstreamUrlFunc func(*types.ModuleContext) (*url.URL, error)
	RequestModifierFunc  func(*types.ModuleContext) error
	ResponseModifierFunc func(*types.ModuleContext, *http.Response) error
	ErrorHandlerFunc     func(*types.ModuleContext, error) error
	RequestHandlerFunc   func(*types.ModuleContext) error
)

type Results struct {
	Result  goja.Value
	IsError bool
}

func RunAndWait(
	rt *goja.Runtime,
	fn goja.Callable,
	args ...goja.Value,
) error {
	_, err := RunAndWaitForResult(rt, fn, args...)
	return err
}

// RunAndWaitForResult can execute a goja function and wait for the result
// if the result is a promise, it will wait for the promise to resolve
func RunAndWaitForResult(
	rt *goja.Runtime,
	fn goja.Callable,
	args ...goja.Value,
) (res goja.Value, err error) {
	if res, err = fn(nil, args...); err != nil {
		return nil, err
	} else if prom, ok := res.Export().(*goja.Promise); ok {
		ctx, cancel := context.WithTimeout(
			context.TODO(), 30*time.Second)
		defer cancel()
		if err = waitTimeout(ctx, func() bool {
			return prom.State() != goja.PromiseStatePending
		}); err != nil {
			rt.Interrupt(err.Error())
			return nil, errors.New("promise timed out: " + err.Error())
		}
		if prom.State() == goja.PromiseStateRejected {
			// no need to interrupt the runtime here
			return nil, errors.New(prom.Result().String())
		}
		results := prom.Result()
		if nully(results) {
			return nil, nil
		}
		return results, nil
	} else if err != nil {
		return nil, err
	} else {
		return res, nil
	}
}

func nully(val goja.Value) bool {
	return val == nil || goja.IsUndefined(val) || goja.IsNull(val)
}

func waitTimeout(ctx context.Context, doneFn func() bool) error {
	if doneFn() {
		return nil
	}
	maxTimeout := 100 * time.Millisecond
	multiplier := 1.75
	backoffTimeout := 2 * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoffTimeout):
			if !doneFn() {
				backoffTimeout = min(time.Duration(
					float64(backoffTimeout)*multiplier,
				), maxTimeout)
				continue
			}
			return nil
		}
	}
}

func DefaultFetchUpstreamFunction() FetchUpstreamUrlFunc {
	roundRobinIndex := 0
	return func(ctx *types.ModuleContext) (*url.URL, error) {
		svc := ctx.Service()
		if svc.URLs == nil || len(svc.URLs) == 0 {
			return nil, errors.New("service has no URLs")
		}
		roundRobinIndex = (roundRobinIndex + 1) % len(svc.URLs)
		curUrl, err := url.Parse(svc.URLs[roundRobinIndex])
		if err != nil {
			return nil, err
		}
		return curUrl, nil
	}
}

func ExtractFetchUpstreamFunction(
	loop *eventloop.EventLoop,
) (fetchUpstream FetchUpstreamUrlFunc, err error) {
	rt := loop.Runtime()
	if fn, ok, err := functionExtractor(rt, "fetchUpstream"); ok {
		fetchUpstream = func(modCtx *types.ModuleContext) (*url.URL, error) {
			if res, err := RunAndWaitForResult(
				rt, fn, rt.ToValue(modCtx),
			); err != nil {
				return nil, err
			} else if nully(res) || res.String() == "" {
				return nil, errors.New("fetchUpstream returned an invalid URL")
			} else {
				upstreamUrlString := res.String()
				if !strings.Contains(upstreamUrlString, "://") {
					upstreamUrlString += "http://"
				}
				upstreamUrl, err := url.Parse(upstreamUrlString)
				if err != nil {
					return nil, err
				}
				// perhaps add default scheme if not present
				return upstreamUrl, err
			}
		}
	} else if err != nil {
		return nil, err
	} else {
		fetchUpstream = DefaultFetchUpstreamFunction()
	}
	return fetchUpstream, nil
}

func ExtractRequestModifierFunction(
	loop *eventloop.EventLoop,
) (requestModifier RequestModifierFunc, err error) {
	rt := loop.Runtime()
	if fn, ok, err := functionExtractor(rt, "requestModifier"); ok {
		requestModifier = func(modCtx *types.ModuleContext) error {
			return RunAndWait(rt, fn, rt.ToValue(modCtx))
		}
	} else if err != nil {
		return nil, err
	} else {
		return nil, nil
	}
	return requestModifier, nil
}

func ExtractResponseModifierFunction(
	loop *eventloop.EventLoop,
) (responseModifier ResponseModifierFunc, err error) {
	rt := loop.Runtime()
	if fn, ok, err := functionExtractor(rt, "responseModifier"); ok {
		responseModifier = func(modCtx *types.ModuleContext, res *http.Response) error {
			modCtx = types.ModuleContextWithResponse(modCtx, res)
			return RunAndWait(rt, fn, rt.ToValue(modCtx))
		}
	} else if err != nil {
		return nil, err
	} else {
		return nil, nil
	}
	return responseModifier, nil
}

func DefaultErrorHandlerFunction() ErrorHandlerFunc {
	return func(modCtx *types.ModuleContext, err error) error {
		rwt := types.GetModuleContextResponseWriterTracker(modCtx)
		if rwt.HeadersSent() {
			return nil
		}
		status := http.StatusBadGateway
		var text string
		switch err {
		case context.Canceled, io.ErrUnexpectedEOF:
			status = 499
			text = "Client Closed Request"
		default:
			text = http.StatusText(status)
		}
		rwt.WriteHeader(status)
		rwt.Write([]byte(text))
		return nil
	}
}

func ExtractErrorHandlerFunction(
	loop *eventloop.EventLoop,
) (errorHandler ErrorHandlerFunc, err error) {
	rt := loop.Runtime()
	if fn, ok, err := functionExtractor(rt, "errorHandler"); ok {
		errorHandler = func(modCtx *types.ModuleContext, upstreamErr error) error {
			modCtx = types.ModuleContextWithError(modCtx, upstreamErr)
			return RunAndWait(
				rt, fn, rt.ToValue(modCtx),
				rt.ToValue(rt.NewGoError(upstreamErr)),
			)
		}
	} else if err != nil {
		return nil, err
	} else {
		errorHandler = DefaultErrorHandlerFunction()
	}
	return errorHandler, nil
}

func ExtractRequestHandlerFunction(
	loop *eventloop.EventLoop,
) (requestHandler RequestHandlerFunc, err error) {
	rt := loop.Runtime()
	if fn, ok, err := functionExtractor(rt, "requestHandler"); ok {
		requestHandler = func(modCtx *types.ModuleContext) error {
			return RunAndWait(
				rt, fn, rt.ToValue(modCtx),
			)
		}
	} else if err != nil {
		return nil, err
	} else {
		return nil, err
	}
	return requestHandler, nil
}

func functionExtractor(rt *goja.Runtime, varName string) (goja.Callable, bool, error) {
	check := fmt.Sprintf(
		"exports?.%s ?? (typeof %s === 'function' ? %s : void 0)",
		varName, varName, varName,
	)
	if fnRef, err := rt.RunString(check); err != nil {
		return nil, false, err
	} else if fn, ok := goja.AssertFunction(fnRef); ok {
		return fn, true, nil
	} else if nully(fnRef) {
		return nil, false, nil
	} else {
		return nil, false, errors.New("extractors: invalid function -> " + varName)
	}
}
