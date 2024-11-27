package proxy

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/dgate-io/dgate-api/pkg/eventloop"
	"github.com/dgate-io/dgate-api/pkg/modules"
	"github.com/dgate-io/dgate-api/pkg/resources"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/dgate-io/dgate-api/pkg/typescript"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
)

// RuntimeContext is the context for the runtime. one per request
type runtimeContext struct {
	reqCtx  *RequestContext
	loop    *eventloop.EventLoop
	state   modules.StateManager
	rm      *resources.ResourceManager
	route   *spec.Route
	modules []*spec.Module
}

var _ modules.RuntimeContext = &runtimeContext{}

func NewRuntimeContext(
	proxyState *ProxyState,
	route *spec.DGateRoute,
	modules ...*spec.DGateModule,
) *runtimeContext {
	rtCtx := &runtimeContext{
		state:   proxyState,
		rm:      proxyState.ResourceManager(),
		modules: spec.TransformDGateModules(modules...),
		route:   spec.TransformDGateRoute(route),
	}

	reg := require.NewRegistryWithLoader(func(path string) ([]byte, error) {
		requireMod := strings.Replace(path, "node_modules/", "", 1)
		// TODO: add support for other module types w/ permissions
		// 'https://' - requires network permissions and must be enabled in the config
		// 'file://' - requires file system permissions and must be enabled in the config
		// 'module://' - requires a module lookup and module permissions
		if mod, ok := findInSortedWith(modules, requireMod,
			func(m *spec.DGateModule) string { return m.Name }); !ok {
			return nil, errors.New(requireMod + " not found")
		} else {
			if mod.Type == spec.ModuleTypeJavascript {
				return []byte(mod.Payload), nil
			}
			var err error
			var key string
			transpileBucket := proxyState.sharedCache.Bucket("ts-transpile")
			if key, err = HashString(0, mod.Payload); err == nil {
				if code, ok := transpileBucket.Get(key); ok {
					return code.([]byte), nil
				}
			}
			payload, err := typescript.Transpile(
				context.TODO(), mod.Payload)
			if err != nil {
				return nil, err
			}
			transpileBucket.SetWithTTL(key, []byte(payload), time.Minute*30)
			return []byte(payload), nil
		}
	})
	rtCtx.loop = eventloop.NewEventLoop(
		eventloop.WithRegistry(reg),
	)
	return rtCtx
}

// UseRequestContext sets the request context
func (rtCtx *runtimeContext) Use(reqCtx *RequestContext) (*runtimeContext, error) {
	if reqCtx != nil {
		if err := reqCtx.ctx.Err(); err != nil {
			return nil, err
		}
	}
	rtCtx.reqCtx = reqCtx
	return rtCtx, nil
}

func (rtCtx *runtimeContext) Clean() {
	rtCtx.loop.StopNoWait()
	rtCtx.loop = nil
	rtCtx.reqCtx = nil
	rtCtx.route = nil
	rtCtx.modules = nil
}

func (rtCtx *runtimeContext) Context() context.Context {
	return rtCtx.reqCtx.ctx
}

func (rtCtx *runtimeContext) EventLoop() *eventloop.EventLoop {
	return rtCtx.loop
}

func (rtCtx *runtimeContext) Runtime() *goja.Runtime {
	return rtCtx.loop.Runtime()
}

func (rtCtx *runtimeContext) State() modules.StateManager {
	return rtCtx.state
}
