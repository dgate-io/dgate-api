package typescript_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dgate-io/dgate-api/pkg/typescript"
	"github.com/dop251/goja"
)

// TOOD: add more test cases for errors to ensure the line numbers are correct
// and the error messages are correct; also wrap code in a function and test.

func TestTranspile(t *testing.T) {
	baseDefault := `function validateDataID(data: Data): any | null {
		return (data.id == null) ? null : data;
	}
	interface Data {
		id: string | null;
		name: string;
		description: string;
	}`

	tsSrcList := []string{
		"export " + strings.TrimSpace(baseDefault),
		"export default " + strings.TrimSpace(baseDefault),
		baseDefault + `
		export default validateDataID
		`,
		baseDefault + `
		export { validateDataID }
		`,
		baseDefault + `
		export { validateDataID: validateDataID }
		`,
	}

	for _, tsSrc := range tsSrcList {
		vm := goja.New()
		jsSrc, err := typescript.Transpile(context.Background(), tsSrc)
		if err != nil {
			t.Fatal(err)
			return
		}
		vm.Set("exports", map[string]any{})
		if jsSrc == "" {
			t.Fatal("jsSrc is empty")
			return
		}
		_, err = vm.RunString(jsSrc)
		if err != nil {
			t.Fatal(err)
			return
		}

		val := vm.Get("exports")
		exportMap := val.Export().(map[string]any)

		if _, ok := exportMap["validateDataID"]; !ok {
			if _, ok := exportMap["default"]; !ok {
				t.Log(jsSrc)
				t.Fatal("exports.default or exports.validateDataID not found")
			}
		}
		if esMod, ok := exportMap["__esModule"]; !ok {
			t.Fatal("exports.__esModule not found")
		} else {
			if !esMod.(bool) {
				t.Fatal("exports.__esModule != true")
			}
		}
	}
}
