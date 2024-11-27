package util_test

import (
	"testing"

	"github.com/dgate-io/dgate-api/pkg/util"
)

func TestDefault(t *testing.T) {
	var i *int
	var d int = 10
	if util.Default(&d, nil) == nil {
		t.Error("Default failed")
	}
	if util.Default(i, &d) != &d {
		t.Error("Default failed")
	}
	if util.DefaultString("", "default") != "default" {
		t.Error("DefaultString failed")
	}
	if util.DefaultString("value", "default") != "value" {
		t.Error("DefaultString failed")
	}
}
