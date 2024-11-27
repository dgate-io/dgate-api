package extractors

import (
	"errors"
	"os"
	"reflect"
	"strings"

	"github.com/dgate-io/dgate-api/pkg/modules"
	"github.com/dgate-io/dgate-api/pkg/modules/dgate"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/buffer"
	"github.com/dop251/goja_nodejs/console"
	"github.com/dop251/goja_nodejs/require"
	"github.com/dop251/goja_nodejs/url"
	"github.com/stoewer/go-strcase"
)

var _ console.Printer = &NoopPrinter{}

type NoopPrinter struct{}

func (p *NoopPrinter) Log(string)   {}
func (p *NoopPrinter) Warn(string)  {}
func (p *NoopPrinter) Error(string) {}

type RuntimeOptions struct {
	Env map[string]string
}

var EnvVarMap = getEnvVarMap()

func getEnvVarMap() map[string]string {
	env := os.Environ()
	envMap := make(map[string]string, len(env))
	for _, e := range env {
		pair := strings.SplitN(e, "=", 2)
		if len(pair) == 2 {
			envMap[pair[0]] = pair[1]
		}
	}
	return envMap
}

func prepareRuntime(rt *goja.Runtime) {
	rt.SetFieldNameMapper(&smartMapper{})
	module := rt.NewObject()
	exports := rt.NewObject()
	module.Set("exports", exports)
	po := processObject(rt)
	rt.GlobalObject().
		Set("process", po)
	rt.Set("module", module)
	rt.Set("exports", exports)
}

func processObject(rt *goja.Runtime) *goja.Object {
	obj := rt.NewObject()
	obj.Set("env", EnvVarMap)
	hostname, _ := os.Hostname()
	obj.Set("host", hostname)
	return obj
}

func SetupModuleEventLoop(
	printer console.Printer,
	rtCtx modules.RuntimeContext,
	programs ...*goja.Program,
) error {
	loop := rtCtx.EventLoop()
	rt := loop.Runtime()
	prepareRuntime(rt)

	req := loop.Registry()
	if registerModules(
		"dgate", rt, req,
		dgate.New(rtCtx),
	); printer == nil {
		printer = &NoopPrinter{}
	}

	req.RegisterNativeModule(
		"dgate_internal:console",
		console.RequireWithPrinter(printer),
	)

	url.Enable(rt)
	buffer.Enable(rt)
	console.Enable(rt)

	rt.Set("fetch", require.Require(rt, "dgate/http").ToObject(rt).Get("fetch"))
	rt.Set("console", require.Require(rt, "dgate_internal:console").ToObject(rt))
	rt.Set("disableSetInterval", disableSetInterval)

	for _, program := range programs {
		_, err := rt.RunProgram(program)
		if err != nil {
			return err
		}
	}

	return nil
}

// registerModules registers a module and its children with the registry (recursively)
func registerModules(
	name string,
	rt *goja.Runtime,
	reg *require.Registry,
	mod modules.GoModule,
) *goja.Object {
	exports := rt.NewObject()
	// defaultExports := rt.NewObject()
	// TODO: Default exports are being ignore, check to see how we can use both named and default together
	if exportsRaw := mod.Exports(); exportsRaw != nil {
		for childName, childMod := range exportsRaw.Named {
			if inst, ok := childMod.(modules.GoModule); ok {
				// only register children if they are modules
				m := registerModules(
					name+"/"+childName,
					rt, reg, inst,
				)
				exports.Set(childName, m)
				// defaultExports.Set(childName, childMod)
				continue
			}
			exports.Set(childName, childMod)
			// defaultExports.Set(childName, childMod)

			reg.RegisterNativeModule(name, func(runtime *goja.Runtime, module *goja.Object) {
				if exportsRaw.Default != nil {
					exports.Set("default", exportsRaw.Default)
				}
				module.Set("exports", exports)
			})
		}
	}
	// reg.RegisterNativeModule(modName, func(runtime *goja.Runtime, module *goja.Object) {
	// 	exports.Set("default", defaultExports)
	// 	module.Set("exports", exports)
	// })
	return exports
}

type smartMapper struct{}

var _ goja.FieldNameMapper = &smartMapper{}

func (*smartMapper) FieldName(_ reflect.Type, f reflect.StructField) string {
	if f.Tag.Get("json") != "" {
		return f.Tag.Get("json")
	}
	return strcase.LowerCamelCase(f.Name)
}

func (*smartMapper) MethodName(_ reflect.Type, m reflect.Method) string {
	return strcase.LowerCamelCase(m.Name)
}

func disableSetInterval(fc *goja.FunctionCall) (goja.Value, error) {
	return nil, errors.New("setInterval is disabled")
}
