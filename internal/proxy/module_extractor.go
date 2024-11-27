package proxy

import (
	"github.com/dgate-io/dgate-api/pkg/modules"
	"github.com/dgate-io/dgate-api/pkg/modules/extractors"
	"github.com/dgate-io/dgate-api/pkg/modules/types"
)

type ModuleExtractor interface {
	// Start starts the event loop for the module extractor; ret is true if the event loop was started, false otherwise
	Start(*RequestContext)
	// Stop stops the event loop for the module extractor
	Stop(wait bool)
	// RuntimeContext returns the runtime context for the module extractor
	RuntimeContext(*RequestContext) (modules.RuntimeContext, error)
	// ModuleContext returns the module context for the module extractor
	ModuleContext() *types.ModuleContext

	FetchUpstreamUrlFunc() (extractors.FetchUpstreamUrlFunc, bool)
	RequestModifierFunc() (extractors.RequestModifierFunc, bool)
	ResponseModifierFunc() (extractors.ResponseModifierFunc, bool)
	ErrorHandlerFunc() (extractors.ErrorHandlerFunc, bool)
	RequestHandlerFunc() (extractors.RequestHandlerFunc, bool)
}

type moduleExtract struct {
	runtimeContext   *runtimeContext
	moduleContext    *types.ModuleContext
	fetchUpstreamUrl extractors.FetchUpstreamUrlFunc
	requestModifier  extractors.RequestModifierFunc
	responseModifier extractors.ResponseModifierFunc
	errorHandler     extractors.ErrorHandlerFunc
	requestHandler   extractors.RequestHandlerFunc
}

func NewModuleExtractor(
	runtimeCtx *runtimeContext,
	fetchUpstreamUrl extractors.FetchUpstreamUrlFunc,
	requestModifier extractors.RequestModifierFunc,
	responseModifier extractors.ResponseModifierFunc,
	errorHandler extractors.ErrorHandlerFunc,
	requestHandler extractors.RequestHandlerFunc,
) ModuleExtractor {
	return &moduleExtract{
		runtimeContext:   runtimeCtx,
		fetchUpstreamUrl: fetchUpstreamUrl,
		requestModifier:  requestModifier,
		responseModifier: responseModifier,
		errorHandler:     errorHandler,
		requestHandler:   requestHandler,
	}
}

func (me *moduleExtract) Start(reqCtx *RequestContext) {
	me.moduleContext = types.NewModuleContext(
		me.runtimeContext.loop,
		reqCtx.rw, reqCtx.req,
		reqCtx.route, reqCtx.params,
	)
	me.runtimeContext.Runtime().
		ClearInterrupt()
	me.runtimeContext.loop.Start()
	me.runtimeContext.reqCtx = reqCtx
}

// Stop stops the event loop for the module extractor
func (me *moduleExtract) Stop(wait bool) {
	me.moduleContext = nil
	me.runtimeContext.reqCtx = nil
	if wait {
		me.runtimeContext.EventLoop().Stop()
	} else {
		me.runtimeContext.EventLoop().StopNoWait()
	}
}

func (me *moduleExtract) RuntimeContext(reqCtx *RequestContext) (modules.RuntimeContext, error) {
	return me.runtimeContext.Use(reqCtx)
}

func (me *moduleExtract) ModuleContext() *types.ModuleContext {
	return me.moduleContext
}

func (me *moduleExtract) FetchUpstreamUrlFunc() (extractors.FetchUpstreamUrlFunc, bool) {
	return me.fetchUpstreamUrl, me.fetchUpstreamUrl != nil
}

func (me *moduleExtract) RequestModifierFunc() (extractors.RequestModifierFunc, bool) {
	return me.requestModifier, me.requestModifier != nil
}

func (me *moduleExtract) ResponseModifierFunc() (extractors.ResponseModifierFunc, bool) {
	return me.responseModifier, me.responseModifier != nil
}

func (me *moduleExtract) ErrorHandlerFunc() (extractors.ErrorHandlerFunc, bool) {
	return me.errorHandler, me.errorHandler != nil
}

func (me *moduleExtract) RequestHandlerFunc() (extractors.RequestHandlerFunc, bool) {
	return me.requestHandler, me.requestHandler != nil
}

func NewEmptyModuleExtractor() ModuleExtractor {
	return &moduleExtract{}
}

type ModuleExtractorFunc func(*RequestContextProvider) (ModuleExtractor, error)
