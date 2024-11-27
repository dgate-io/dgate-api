package extractors_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/dgate-io/dgate-api/pkg/modules/extractors"
	"github.com/dgate-io/dgate-api/pkg/modules/testutil"
	"github.com/dgate-io/dgate-api/pkg/typescript"
	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
)

const TS_PAYLOAD = `
import { print } from "test"
let numCalls = 0
const customFunc = (req: any, upstream: any) => {
	print()
	numCalls++
	return 0.01
}
function customFunc2(req: any, upstream: any) {
	print()
	numCalls++
	return 0.01
}
const customFunc3 = async (req: any, upstream: any) => {
	await print()
	numCalls++
	return 0.01
}
const customFunc4 = (req: any, upstream: any): Promise<any> => {
	return print().then(() => {
		numCalls++
		return 0.01
	})
}
async function print() {console.log("log")}
`

func Test_runAndWaitForResult(t *testing.T) {
	src, err := typescript.Transpile(context.Background(), TS_PAYLOAD)
	if err != nil {
		t.Fatal(err)
	}
	program, err := goja.Compile("test", src, false)
	if err != nil {
		t.Fatal(err)
	}
	printer := testutil.NewMockPrinter()
	rtCtx := testutil.NewMockRuntimeContext()
	err = extractors.SetupModuleEventLoop(
		printer, rtCtx, program)
	if err != nil {
		t.Fatal(err)
	}
	rt := rtCtx.EventLoop().Start()
	defer rtCtx.EventLoop().Stop()
	funcs := []string{"customFunc", "customFunc2", "customFunc3", "customFunc4"}
	printer.On("Log", "log").Times(len(funcs))
	for _, fn := range funcs {
		val := rt.Get(fn)
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			t.Fatalf("%s not found", fn)
		}
		customFunc, ok := goja.AssertFunction(val)
		if !ok {
			t.Fatalf("%s is not a function", fn)
		}
		val, err := extractors.RunAndWaitForResult(rt, customFunc, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if val.ToFloat() != 0.01 {
			t.Errorf("%s should return return 0.01", fn)
		}
	}
	numCalls := rt.Get("numCalls").ToInteger()
	if numCalls != 4 {
		t.Fatalf("numCalls should be 4, got %d", numCalls)
	}
}

const TS_PAYLOAD_EXPORTED = `
import { test } from "test"

export default function named_func() {
	console.log(test)
	return [1, 2, 3]
}

export async function named_func_async() {
	console.log(test)
	return { x: 1 }
}
`

func TestExportedInformation(t *testing.T) {
	src, err := typescript.Transpile(context.Background(), TS_PAYLOAD_EXPORTED)
	if err != nil {
		t.Fatal(err)
	}
	program, err := goja.Compile("", src, true)
	if err != nil {
		t.Fatal(err)
	}
	printer := testutil.NewMockPrinter()
	printer.On("Log", "testing").Twice()
	rtCtx := testutil.NewMockRuntimeContext()
	rtCtx.On("func1", "node_modules/test").
		Return([]byte("exports.test = 'testing';"), nil).
		Once()
	err = extractors.SetupModuleEventLoop(
		printer, rtCtx, program)
	if err != nil {
		t.Fatal(err)
	}
	rt := rtCtx.EventLoop().Runtime()
	defer rtCtx.EventLoop().Stop()
	v, err := rt.RunString("exports.default")
	if err != nil {
		t.Fatal(err)
	}
	callable, ok := goja.AssertFunction(v)
	if !assert.True(t, ok) {
		return
	}
	v, err = extractors.RunAndWaitForResult(rt, callable)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		vv := v.ToObject(rt).Get(strconv.Itoa(i))
		if vv.ToInteger() != int64(i+1) {
			t.Fatalf("named_func should return [1, 2, 3]; ~[%d] => %v", i, vv)
		}
	}

	v, err = rt.RunString("exports.named_func_async")
	if err != nil {
		t.Fatal(err)
	}
	callable, ok = goja.AssertFunction(v)
	assert.True(t, ok)

	v, err = extractors.RunAndWaitForResult(rt, callable)
	if err != nil {
		t.Fatal(err)
	}
	if v.ToObject(rt).Get("x").ToInteger() != 1 {
		t.Fatal("named_func_async should return {x: 1}")
	}
}

const TS_PAYLOAD_PROMISE = `
// function delay(ms: number) {
//     return new Promise(resolve => setTimeout(resolve, ms) );
// }
function delay(ms: number) {
    return new Promise( resolve => resolve() );
}

export async function test1() {
	await delay(500)
	return 1
}

export async function test2() {
	await delay(500)
	throw new Error("test2 failed successfully")
}
`

func TestExportedPromiseErrors(t *testing.T) {
	src, err := typescript.Transpile(context.Background(), TS_PAYLOAD_PROMISE)
	if err != nil {
		t.Fatal(err)
	}
	program, err := goja.Compile("", src, true)
	if err != nil {
		t.Fatal(err)
	}
	printer := testutil.NewMockPrinter()
	rtCtx := testutil.NewMockRuntimeContext()
	err = extractors.SetupModuleEventLoop(printer, rtCtx, program)
	if err != nil {
		t.Fatal(err)
	}

	rt := rtCtx.EventLoop().Start()
	defer rtCtx.EventLoop().Stop()

	v, err := rt.RunString("exports.test1")
	if err != nil {
		t.Fatal(err)
	}
	callable, ok := goja.AssertFunction(v)
	if !assert.True(t, ok) {
		return
	}
	v, err = extractors.RunAndWaitForResult(rt, callable, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.ToInteger() != 1 {
		t.Fatal("test1 should return 1")
	}
	v, err = rt.RunString("exports.test2")
	if err != nil {
		t.Fatal(err)
	}

	callable, ok = goja.AssertFunction(v)
	if !assert.True(t, ok) {
		return
	}
	_, err = extractors.RunAndWaitForResult(rt, callable, nil, nil, nil)
	assert.Error(t, err, "Error: test2 failed successfully")
}
