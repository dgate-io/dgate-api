package linker_test

import (
	"testing"

	"github.com/dgate-io/dgate-api/pkg/util/linker"
	"github.com/stretchr/testify/assert"
)

func TestLinkerTests(t *testing.T) {
	linker1 := linker.NewNamedVertex[string, int](
		"top", "bottom",
	)
	linker2 := linker.NewNamedVertex[string, int](
		"top", "bottom",
	)
	linker3 := linker.NewNamedVertex[string, int](
		"top", "bottom",
	)
	linker4 := linker.NewNamedVertex[string, int](
		"top", "bottom",
	)
	linker1.LinkOneOne("top", "l1top", linker2)
	linker2.LinkOneOne("top", "l2top", linker3)
	linker3.LinkOneOne("top", "l3top", linker4)
	linker4.LinkOneOne("bottom", "l4bottom", linker3)
	linker3.LinkOneMany("bottom", "l3bottom", linker2)
	linker2.LinkOneMany("bottom", "l2bottom", linker1)

	assert.True(t, linker1.Len("top") == 1)
	assert.True(t, linker2.Len("top") == 1)
	assert.True(t, linker3.Len("top") == 1)
	assert.True(t, linker4.Len("top") == 0)

	assert.True(t, linker1.Len("bottom") == 0)
	assert.True(t, linker2.Len("bottom") == 1)
	assert.True(t, linker3.Len("bottom") == 1)
	assert.True(t, linker4.Len("bottom") == 1)
}
