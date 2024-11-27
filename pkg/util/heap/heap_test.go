package heap_test

import (
	"testing"

	"github.com/dgate-io/dgate-api/pkg/util/heap"
	"github.com/stretchr/testify/assert"
)

func TestHeap_MinHeap(t *testing.T) {
	h := heap.NewHeap[int, any](heap.MinHeapType)
	numElements := 1_000_000
	for i := numElements; i > 0; i-- {
		h.Push(i, i)
	}
	for i := 1; i <= numElements; i++ {
		j, _, ok := h.Pop()
		if !ok {
			t.Fatalf("expected key to be found")
		}
		if i != j {
			t.Fatalf("expected key to be %d, got %d", i, j)
		}
	}
}

func TestHeap_MinHeap_PushPop(t *testing.T) {
	h := heap.NewHeap[int, any](heap.MinHeapType)

	assert.Equal(t, 0, h.Len())
	h.Push(3, 33)
	h.Push(2, 22)
	h.Push(1, 11)

	assert.Equal(t, 3, h.Len())
	min, val, ok := h.Pop()
	if !ok {
		t.Fatalf("expected key to be found")
	}
	if min != 1 {
		t.Fatalf("expected min to be 1, got %d", min)
	}
	if val != 11 {
		t.Fatalf("expected value to be 11, got %v", val)
	}

	assert.Equal(t, 2, h.Len())
	min, val, ok = h.Pop()
	if !ok {
		t.Fatalf("expected key to be found")
	}
	if min != 2 {
		t.Fatalf("expected min to be 2, got %d", min)
	}
	if val != 22 {
		t.Fatalf("expected value to be 22, got %v", val)
	}

	assert.Equal(t, 1, h.Len())
	min, val, ok = h.Pop()
	if !ok {
		t.Fatalf("expected key to be found")
	}
	if min != 3 {
		t.Fatalf("expected min to be 3, got %d", min)
	}
	if val != 33 {
		t.Fatalf("expected value to be 33, got %v", val)
	}

	assert.Equal(t, 0, h.Len())
	_, _, ok = h.Pop()
	if ok {
		t.Fatalf("expected key to be empty")
	}
}

func TestHeap_MaxHeap(t *testing.T) {
	h := heap.NewHeap[int, any](heap.MaxHeapType)
	numElements := 1_000_000
	for i := 1; i <= numElements; i++ {
		h.Push(i, i)
	}
	for i := numElements; i > 0; i-- {
		j, _, ok := h.Pop()
		if !ok {
			t.Fatalf("expected key to be found")
		}
		if i != j {
			t.Fatalf("expected key to be %d, got %d", i, j)
		}
	}
}

func TestHeap_MaxHeap_PushPop(t *testing.T) {
	h := heap.NewHeap[int, any](heap.MaxHeapType)

	assert.Equal(t, 0, h.Len())
	h.Push(1, 11)
	h.Push(2, 22)
	h.Push(3, 33)

	assert.Equal(t, 3, h.Len())
	max, val, ok := h.Pop()
	if !ok {
		t.Fatalf("expected key to be found")
	}
	if max != 3 {
		t.Fatalf("expected max to be 3, got %d", max)
	}
	if val != 33 {
		t.Fatalf("expected value to be 11, got %v", val)
	}

	assert.Equal(t, 2, h.Len())
	max, val, ok = h.Pop()
	if !ok {
		t.Fatalf("expected key to be found")
	}
	if max != 2 {
		t.Fatalf("expected max to be 2, got %d", max)
	}
	if val != 22 {
		t.Fatalf("expected value to be 22, got %v", val)
	}

	assert.Equal(t, 1, h.Len())
	max, val, ok = h.Pop()
	if !ok {
		t.Fatalf("expected key to be found")
	}
	if max != 1 {
		t.Fatalf("expected max to be 1, got %d", max)
	}
	if val != 11 {
		t.Fatalf("expected value to be 11, got %v", val)
	}

	assert.Equal(t, 0, h.Len())
	_, _, ok = h.Pop()
	if ok {
		t.Fatalf("expected key to be empty")
	}
}

func BenchmarkHeap_MinHeap(b *testing.B) {
	b.Run("PushAsc", func(b *testing.B) {
		h := heap.NewHeap[int, any](heap.MinHeapType)
		for i := 0; i < b.N; i++ {
			h.Push(i, nil)
		}
	})
	b.Run("PushDesc", func(b *testing.B) {
		h := heap.NewHeap[int, any](heap.MinHeapType)
		for i := 0; i < b.N; i++ {
			h.Push(b.N-i, nil)
		}
	})
	b.Run("Pop", func(b *testing.B) {
		h := heap.NewHeap[int, any](heap.MinHeapType)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, _, ok := h.Pop(); !ok {
				b.StopTimer()
				for i := 0; i < 10_000_000; i++ {
					h.Push(i, nil)
				}
				b.StartTimer()
			}
		}
	})
}

func BenchmarkHeap_MaxHeap(b *testing.B) {
	b.Run("PushAsc", func(b *testing.B) {
		h := heap.NewHeap[int, any](heap.MaxHeapType)
		for i := 0; i < b.N; i++ {
			h.Push(i, nil)
		}
	})
	b.Run("PushDesc", func(b *testing.B) {
		h := heap.NewHeap[int, any](heap.MaxHeapType)
		for i := 0; i < b.N; i++ {
			h.Push(b.N-i, nil)
		}
	})
	b.Run("Pop", func(b *testing.B) {
		h := heap.NewHeap[int, any](heap.MaxHeapType)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, _, ok := h.Pop(); !ok {
				b.StopTimer()
				for i := 10_000_000; i > 0; i-- {
					h.Push(i, nil)
				}
				b.StartTimer()
			}
		}
	})
}
