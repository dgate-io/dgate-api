package proxy_test

import (
	"net/http"

	"github.com/dgate-io/dgate-api/internal/proxy"
	"github.com/dgate-io/dgate-api/pkg/modules"
	"github.com/dgate-io/dgate-api/pkg/modules/extractors"
	"github.com/dgate-io/dgate-api/pkg/modules/types"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/stretchr/testify/mock"
)

type mockModulePool struct {
	mock.Mock
}

var _ proxy.ModulePool = &mockModulePool{}

func NewMockModulePool() *mockModulePool {
	return &mockModulePool{}
}

// Borrow implements proxy.ModulePool.
func (mb *mockModulePool) Borrow() proxy.ModuleExtractor {
	args := mb.Called()
	return args.Get(0).(proxy.ModuleExtractor)
}

// Close implements proxy.ModulePool.
func (mb *mockModulePool) Close() {
	mb.Called()
}

// Load implements proxy.ModulePool.
func (mb *mockModulePool) Load(cb func()) {
	mb.Called(cb)
}

// Return implements proxy.ModulePool.
func (mb *mockModulePool) Return(me proxy.ModuleExtractor) {
	mb.Called(me)
}

type mockModuleExtractor struct {
	mock.Mock
}

var _ proxy.ModuleExtractor = &mockModuleExtractor{}

func NewMockModuleExtractor() *mockModuleExtractor {
	return &mockModuleExtractor{}
}

func (m *mockModuleExtractor) ConfigureEmptyMock() {
	rtCtx := proxy.NewRuntimeContext(nil, nil)
	m.On("Start", mock.Anything).Return()
	m.On("Stop", mock.Anything).Return()
	m.On("RuntimeContext").Return(rtCtx)
	m.On("ModuleContext").Return(nil)
	m.On("ModHash").Return(uint32(1))
	m.On("SetModuleContext", mock.Anything).Return()
	m.On("FetchUpstreamUrlFunc").Return(nil, false)
	m.On("RequestModifierFunc").Return(nil, false)
	m.On("ResponseModifierFunc").Return(nil, false)
	m.On("ErrorHandlerFunc").Return(nil, false)
	m.On("RequestHandlerFunc").Return(nil, false)
}

func (m *mockModuleExtractor) ConfigureDefaultMock(
	req *http.Request,
	rw http.ResponseWriter,
	ps *proxy.ProxyState,
	rt *spec.DGateRoute,
	mods ...*spec.DGateModule,
) {
	rtCtx := proxy.NewRuntimeContext(ps, rt, mods...)
	modCtx := types.NewModuleContext(nil, rw, req, rt, nil)
	m.On("Start", mock.Anything).Return().Maybe()
	m.On("Stop", mock.Anything).Return().Maybe()
	m.On("RuntimeContext").Return(rtCtx).Maybe()
	m.On("ModuleContext").Return(modCtx).Maybe()
	m.On("ModHash").Return(uint32(123)).Maybe()
	m.On("SetModuleContext", mock.Anything).Return().Maybe()
	m.On("FetchUpstreamUrlFunc").Return(extractors.DefaultFetchUpstreamFunction(), true).Maybe()
	m.On("ErrorHandlerFunc").Return(extractors.DefaultErrorHandlerFunction(), true).Maybe()
	m.On("RequestModifierFunc").Return(nil, false).Maybe()
	m.On("ResponseModifierFunc").Return(nil, false).Maybe()
	m.On("RequestHandlerFunc").Return(nil, false).Maybe()
}

func (m *mockModuleExtractor) Start(reqCtx *proxy.RequestContext) {
	m.Called(reqCtx)
}

func (m *mockModuleExtractor) Stop(wait bool) {
	m.Called(wait)
}

func (m *mockModuleExtractor) RuntimeContext(*proxy.RequestContext) (modules.RuntimeContext, error) {
	args := m.Called()
	return args.Get(0).(modules.RuntimeContext), args.Error(1)
}

func (m *mockModuleExtractor) ModuleContext() *types.ModuleContext {
	args := m.Called()
	return args.Get(0).(*types.ModuleContext)
}

func (m *mockModuleExtractor) SetModuleContext(modCtx *types.ModuleContext) {
	m.Called(modCtx)
}

func (m *mockModuleExtractor) FetchUpstreamUrlFunc() (extractors.FetchUpstreamUrlFunc, bool) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Bool(1)
	}
	return args.Get(0).(extractors.FetchUpstreamUrlFunc), args.Bool(1)
}

func (m *mockModuleExtractor) RequestModifierFunc() (extractors.RequestModifierFunc, bool) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Bool(1)
	}
	return args.Get(0).(extractors.RequestModifierFunc), args.Bool(1)
}

func (m *mockModuleExtractor) ResponseModifierFunc() (extractors.ResponseModifierFunc, bool) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Bool(1)
	}
	return args.Get(0).(extractors.ResponseModifierFunc), args.Bool(1)
}

func (m *mockModuleExtractor) ErrorHandlerFunc() (extractors.ErrorHandlerFunc, bool) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Bool(1)
	}
	return args.Get(0).(extractors.ErrorHandlerFunc), args.Bool(1)
}

func (m *mockModuleExtractor) RequestHandlerFunc() (extractors.RequestHandlerFunc, bool) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Bool(1)
	}
	return args.Get(0).(extractors.RequestHandlerFunc), args.Bool(1)
}
