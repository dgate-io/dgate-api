package extractors_test

import (
	"context"
	"testing"

	"github.com/dgate-io/dgate-api/internal/config/configtest"
	"github.com/dgate-io/dgate-api/internal/proxy"
	"github.com/dgate-io/dgate-api/pkg/modules/extractors"
	"github.com/dgate-io/dgate-api/pkg/modules/testutil"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/dgate-io/dgate-api/pkg/typescript"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/console"
	"go.uber.org/zap"
)

const TS_PAYLOAD_CUSTOMFUNC = `
let customFunc = (req: any, upstream: any) => {
	console.log("log 1")
	console.warn("log 2")
	console.error("log 3")
}
export { customFunc }  
`

const JS_PAYLOAD_CUSTOMFUNC = `
let customFunc = (req, upstream) => {
	console.log("log 1")
	console.warn("log 2")
	console.error("log 3")
}
module.exports = { customFunc }
`

type consolePrinter struct {
	calls map[string]int
}

var _ console.Printer = &consolePrinter{}

func (cp *consolePrinter) Log(string) {
	if _, ok := cp.calls["Log"]; !ok {
		cp.calls["Log"] = 1
	} else {
		cp.calls["Log"]++
	}
}

func (cp *consolePrinter) Warn(string) {
	if _, ok := cp.calls["Warn"]; !ok {
		cp.calls["Warn"] = 1
	} else {
		cp.calls["Warn"]++
	}
}

func (cp *consolePrinter) Error(string) {
	if _, ok := cp.calls["Error"]; !ok {
		cp.calls["Error"] = 1
	} else {
		cp.calls["Error"]++
	}
}

func TestNewModuleRuntimeJS(t *testing.T) {
	programs := map[string]*goja.Program{
		"javascript": testutil.CreateJSProgram(t, JS_PAYLOAD_CUSTOMFUNC),
		"typescript": testutil.CreateTSProgram(t, TS_PAYLOAD_CUSTOMFUNC),
	}
	for testName, program := range programs {
		t.Run(testName, func(t *testing.T) {
			printer := testutil.NewMockPrinter()
			printer.On("Log", "log 1").Return().Once()
			printer.On("Warn", "log 2").Return().Once()
			printer.On("Error", "log 3").Return().Once()
			rtCtx := testutil.NewMockRuntimeContext()
			err := extractors.SetupModuleEventLoop(
				printer, rtCtx, program,
			)
			if err != nil {
				t.Fatal(err)
			}
			rt := rtCtx.EventLoop().Start()
			defer rtCtx.EventLoop().Stop()
			val := rt.Get("customFunc")
			if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
				t.Fatal("customFunc not found")
			}
			customFunc, ok := goja.AssertFunction(val)
			if !ok {
				t.Fatal("customFunc is not a function")
			}
			_, err = customFunc(nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			printer.AssertExpectations(t)
		})
	}
}

func TestPrinter(t *testing.T) {
	program := testutil.CreateJSProgram(t, JS_PAYLOAD_CUSTOMFUNC)
	cp := &consolePrinter{make(map[string]int)}
	rt := &spec.DGateRoute{Namespace: &spec.DGateNamespace{}}
	conf := configtest.NewTestDGateConfig()
	ps := proxy.NewProxyState(zap.NewNop(), conf)
	rtCtx := proxy.NewRuntimeContext(ps, rt)
	if err := extractors.SetupModuleEventLoop(
		cp, rtCtx, program,
	); err != nil {
		t.Fatal(err)
	}
	rtCtx.EventLoop().Start()
	defer rtCtx.EventLoop().Stop()
	wait := make(chan struct{})
	rtCtx.EventLoop().RunOnLoop(func(rt *goja.Runtime) {
		val := rt.Get("customFunc")
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			t.Fatal("customFunc not found")
		}
		wait <- struct{}{}
	})
	<-wait
}

func BenchmarkNewModuleRuntime(b *testing.B) {
	program := testutil.CreateTSProgram(b, TS_PAYLOAD_CUSTOMFUNC)
	conf := configtest.NewTestDGateConfig()
	ps := proxy.NewProxyState(zap.NewNop(), conf)

	b.ResetTimer()
	b.Run("CreateModuleRuntime", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StartTimer()
			rt := &spec.DGateRoute{Namespace: &spec.DGateNamespace{}}
			rtCtx := proxy.NewRuntimeContext(ps, rt)
			err := extractors.SetupModuleEventLoop(
				nil, rtCtx, program)
			b.StopTimer()
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Transpile-TS", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StartTimer()
			_, err := typescript.Transpile(context.Background(), TS_PAYLOAD_CUSTOMFUNC)
			if err != nil {
				b.Fatal(err)
			}

			b.StopTimer()
		}
	})

	b.Run("CreateNewProgram-TS", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StartTimer()
			testutil.CreateTSProgram(b, TS_PAYLOAD_CUSTOMFUNC)
			b.StopTimer()
		}
	})

	b.Run("CreateNewProgram-JS", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StartTimer()
			testutil.CreateJSProgram(b, JS_PAYLOAD_CUSTOMFUNC)
			b.StopTimer()
		}
	})

}
