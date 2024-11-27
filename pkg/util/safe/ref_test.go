package safe_test

import (
	"testing"

	"github.com/dgate-io/dgate-api/pkg/util/safe"
)

type test struct {
	i int
	t *test
}

func TestRef(t *testing.T) {
	src := &test{1, nil}
	src.t = src
	ref := safe.NewRef(src)
	readSrc := ref.Read()
	oldReadSrc := ref.Replace(readSrc)
	newReadSrc := ref.Read()
	checkEqual(t, src, readSrc, oldReadSrc, newReadSrc)
}

func checkEqual(t *testing.T, items ...interface{}) {
	for i, item := range items {
		for j, other := range items {
			if i != j && item == other {
				t.Errorf("item %v should not be equal to item %v", i, j)
			}
		}
	}
}
