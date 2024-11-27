package testutil

import (
	"context"
	"sync"

	"github.com/dgate-io/dgate-api/pkg/cache"
	"github.com/dgate-io/dgate-api/pkg/eventloop"
	"github.com/dgate-io/dgate-api/pkg/modules"
	"github.com/dgate-io/dgate-api/pkg/resources"
	"github.com/dgate-io/dgate-api/pkg/scheduler"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/dgate-io/dgate-api/pkg/typescript"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/console"
	"github.com/dop251/goja_nodejs/require"
	"github.com/stretchr/testify/mock"
)

type mockRuntimeContext struct {
	mock.Mock
	smap  *sync.Map
	ctx   context.Context
	req   *require.Registry
	loop  *eventloop.EventLoop
	data  any
	state modules.StateManager
}

type mockState struct {
	mock.Mock
}

func (m *mockState) ApplyChangeLog(*spec.ChangeLog) error {
	args := m.Called()
	return args.Error(0)
}

func (m *mockState) ResourceManager() *resources.ResourceManager {
	args := m.Called()
	return args.Get(0).(*resources.ResourceManager)
}

func (m *mockState) DocumentManager() resources.DocumentManager {
	args := m.Called()
	return args.Get(0).(resources.DocumentManager)
}

func (m *mockState) Scheduler() scheduler.Scheduler {
	args := m.Called()
	return args.Get(0).(scheduler.Scheduler)
}

func (m *mockState) SharedCache() cache.TCache {
	args := m.Called()
	return args.Get(0).(cache.TCache)
}

var _ modules.RuntimeContext = &mockRuntimeContext{}

func NewMockRuntimeContext() *mockRuntimeContext {
	modCtx := &mockRuntimeContext{
		ctx:   context.Background(),
		smap:  &sync.Map{},
		data:  make(map[string]any),
		state: &mockState{},
	}
	mockRequireFunc := func(path string) ([]byte, error) {
		args := modCtx.Called(path)
		return args.Get(0).([]byte), args.Error(1)
	}
	modCtx.req = require.NewRegistry(require.WithLoader(mockRequireFunc))
	modCtx.loop = eventloop.NewEventLoop(eventloop.WithRegistry(modCtx.req))
	return modCtx
}

func (m *mockRuntimeContext) Context() context.Context {
	return m.ctx
}

func (m *mockRuntimeContext) EventLoop() *eventloop.EventLoop {
	return m.loop
}

func (m *mockRuntimeContext) Runtime() *goja.Runtime {
	return m.loop.Runtime()
}
func (m *mockRuntimeContext) State() modules.StateManager {
	return m.state
}

func (m *mockRuntimeContext) Registry() *require.Registry {
	return m.req
}

type mockPrinter struct {
	mock.Mock
	logs map[string][]string
}

var _ console.Printer = &mockPrinter{}

func NewMockPrinter() *mockPrinter {
	return &mockPrinter{
		logs: make(map[string][]string),
	}
}

func (m *mockPrinter) log(l, s string) {
	m.MethodCalled(l, s)
}

func (mp *mockPrinter) Error(s string) {
	mp.log("Error", s)
}

func (mp *mockPrinter) Log(s string) {
	mp.log("Log", s)
}

func (mp *mockPrinter) Warn(s string) {
	mp.log("Warn", s)
}

type Crashable interface {
	Fatal(...any)
}

func CreateTSProgram(c Crashable, payload string) *goja.Program {
	src, err := typescript.Transpile(context.Background(), payload)
	if err != nil {
		c.Fatal(err)
	}
	program, err := goja.Compile("test", src, false)
	if err != nil {
		c.Fatal(err)
	}
	return program
}

func CreateJSProgram(c Crashable, payload string) *goja.Program {
	program, err := goja.Compile("test", payload, false)
	if err != nil {
		c.Fatal(err)
	}
	return program
}
