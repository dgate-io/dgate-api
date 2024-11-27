package types

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/dgate-io/dgate-api/pkg/eventloop"
	"github.com/dgate-io/dgate-api/pkg/spec"
)

type ModuleContext struct {
	ID string `json:"id"`

	ns     *spec.Namespace
	svc    *spec.Service
	route  *spec.Route
	params map[string]string
	loop   *eventloop.EventLoop
	req    *RequestWrapper
	rwt    *ResponseWriterWrapper
	upResp *ResponseWrapper
	cache  map[string]interface{}
}

func NewModuleContext(
	loop *eventloop.EventLoop,
	rw http.ResponseWriter,
	req *http.Request,
	route *spec.DGateRoute,
	params map[string]string,
) *ModuleContext {
	t := time.Now().UnixNano()
	id := strconv.FormatUint(uint64(t), 36)
	return &ModuleContext{
		ID:     id,
		loop:   loop,
		req:    NewRequestWrapper(req, loop),
		rwt:    NewResponseWriterWrapper(rw, req),
		route:  spec.TransformDGateRoute(route),
		svc:    spec.TransformDGateService(route.Service),
		ns:     spec.TransformDGateNamespace(route.Namespace),
		params: params,
	}
}

func (modCtx *ModuleContext) Set(key string, value any) {
	modCtx.cache[key] = value
}

func (modCtx *ModuleContext) Get(key string) any {
	return modCtx.cache[key]
}

func (modCtx *ModuleContext) Query() url.Values {
	return modCtx.req.Query
}

func (modCtx *ModuleContext) PathParams() map[string]string {
	return modCtx.params
}

func (modCtx *ModuleContext) PathParam(key string) string {
	return modCtx.params[key]
}

func (modCtx *ModuleContext) Route() *spec.Route {
	return modCtx.route
}

func (modCtx *ModuleContext) Service() *spec.Service {
	return modCtx.svc
}

func (modCtx *ModuleContext) Namespace() *spec.Namespace {
	return modCtx.ns
}

func (modCtx *ModuleContext) Request() *RequestWrapper {
	return modCtx.req
}

func (modCtx *ModuleContext) Upstream() *ResponseWrapper {
	return modCtx.upResp
}

func (modCtx *ModuleContext) Response() *ResponseWriterWrapper {
	return modCtx.rwt
}

func ModuleContextWithResponse(
	modCtx *ModuleContext,
	resp *http.Response,
) *ModuleContext {
	modCtx.upResp = NewResponseWrapper(resp, modCtx.loop)
	modCtx.rwt = nil
	return modCtx
}

func ModuleContextWithError(
	modCtx *ModuleContext, err error,
) *ModuleContext {
	modCtx.upResp = nil
	return modCtx
}

// Helper functions to expose private fields

func GetModuleContextRoute(modCtx *ModuleContext) *spec.Route {
	return modCtx.route
}

func GetModuleContextService(modCtx *ModuleContext) *spec.Service {
	return modCtx.svc
}

func GetModuleContextNamespace(modCtx *ModuleContext) *spec.Namespace {
	return modCtx.ns
}

func GetModuleContextRequest(modCtx *ModuleContext) *RequestWrapper {
	return modCtx.req
}

func GetModuleContextResponse(modCtx *ModuleContext) *ResponseWrapper {
	return modCtx.upResp
}

func GetModuleContextResponseWriterTracker(modCtx *ModuleContext) spec.ResponseWriterTracker {
	return modCtx.rwt.rw
}
