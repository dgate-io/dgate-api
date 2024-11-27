package dgate

import (
	"errors"
	"time"

	"github.com/dgate-io/dgate-api/pkg/modules"
	"github.com/dgate-io/dgate-api/pkg/modules/dgate/crypto"
	"github.com/dgate-io/dgate-api/pkg/modules/dgate/exp"
	"github.com/dgate-io/dgate-api/pkg/modules/dgate/http"
	"github.com/dgate-io/dgate-api/pkg/modules/dgate/state"
	"github.com/dgate-io/dgate-api/pkg/modules/dgate/storage"
	"github.com/dgate-io/dgate-api/pkg/modules/dgate/util"
	"github.com/dop251/goja"
)

type DGateModule struct {
	modCtx modules.RuntimeContext
}

// New implements the modules.Module interface to return
// a new instance for each ModuleContext.
func New(modCtx modules.RuntimeContext) *DGateModule {
	return &DGateModule{modCtx}
}

// Children returns the exports of the k6 module.
func (x *DGateModule) Exports() *modules.Exports {
	return &modules.Exports{
		Named: map[string]any{
			// Functions
			"fail":       x.Fail,
			"retry":      x.Retry,
			"sleep":      x.Sleep,
			"asyncSleep": x.AsyncSleep,

			// Submodules
			"x":       exp.New(x.modCtx),
			"http":    http.New(x.modCtx),
			"util":    util.New(x.modCtx),
			"state":   state.New(x.modCtx),
			"crypto":  crypto.New(x.modCtx),
			"storage": storage.New(x.modCtx),
		},
	}
}

func (*DGateModule) Fail(msg string) (goja.Value, error) {
	return goja.Undefined(), errors.New(msg)
}

func (x *DGateModule) Sleep(val goja.Value) (err error) {
	var dur time.Duration
	switch v := val.Export().(type) {
	case float64:
		dur = time.Duration(v) * time.Second
	case int64:
		dur = time.Duration(v) * time.Second
	case string:
		dur, err = time.ParseDuration(v)
		if err != nil {
			return err
		}
	default:
		return errors.New("sleep() requires a number or string as a first argument")
	}

	if dur <= 0 {
		return
	}

	wait := time.After(dur)

	ctx := x.modCtx.Context()
	select {
	case <-wait:
	case <-ctx.Done():
	}
	return nil
}

func (x *DGateModule) AsyncSleep(val goja.Value) (*goja.Promise, error) {
	var dur time.Duration
	var err error
	switch v := val.Export().(type) {
	case float64:
		dur = time.Duration(v) * time.Second
	case int64:
		dur = time.Duration(v) * time.Second
	case string:
		dur, err = time.ParseDuration(v)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("sleep() requires a number or string as a first argument")
	}

	wait := time.After(dur)
	rt := x.modCtx.Runtime()
	promise, resolve, _ := rt.NewPromise()

	if dur > 0 {
		x.modCtx.EventLoop().RunOnLoop(func(rt *goja.Runtime) {
			ctx := x.modCtx.Context()
			select {
			case <-wait:
			case <-ctx.Done():
			}
			resolve(goja.Undefined())
		})
	} else {
		resolve(goja.Undefined())
	}

	return promise, nil
}

func (x *DGateModule) Retry(num int, fn goja.Callable) (v goja.Value, err error) {
	if num <= 0 {
		return nil, errors.New("num must be greater than 0")
	}
	if fn == nil {
		return nil, errors.New("retry() requires a callback as a second argument")
	}
	loop := x.modCtx.EventLoop()
	loop.RunOnLoop(func(rt *goja.Runtime) {
		for i := 0; i < num; i++ {
			v, err = fn(goja.Undefined(), rt.ToValue(i))
			if v.ToBoolean() {
				return
			}
		}
	})
	return v, err
}
