package queue_test

import (
	"testing"

	"github.com/dgate-io/dgate-api/pkg/util/queue"
	"github.com/stretchr/testify/assert"
)

func TestQueue_PushPop(t *testing.T) {
	q := queue.New[int]()
	q.Push(1)
	q.Push(2)
	q.Push(3)

	if q.Len() != 3 {
		t.Fatalf("expected length to be 3, got %d", q.Len())
	}

	v, ok := q.Pop()
	assert.True(t, ok, "expected key to be found")
	assert.Equal(t, 1, v, "expected value to be 1, got %d", v)

	v, ok = q.Pop()
	assert.True(t, ok, "expected key to be found")
	assert.Equal(t, 2, v, "expected value to be 2, got %d", v)

	v, ok = q.Pop()
	assert.True(t, ok, "expected key to be found")
	assert.Equal(t, 3, v, "expected value to be 3, got %d", v)

	if q.Len() != 0 {
		t.Fatalf("expected length to be 0, got %d", q.Len())
	}

	v, ok = q.Pop()
	assert.False(t, ok, "expected key to be not found")
	assert.Equal(t, 0, v, "expected value to be 0, got %d", v)
}

func TestQueue_Peek(t *testing.T) {
	q := queue.New[int]()
	q.Push(1)
	q.Push(2)
	q.Push(3)

	v, ok := q.Peek()
	assert.True(t, ok, "expected key to be found")
	assert.Equal(t, 1, v, "expected value to be 1, got %d", v)

	v, ok = q.Peek()
	assert.True(t, ok, "expected key to be found")
	assert.Equal(t, 1, v, "expected value to be 1, got %d", v)

	q.Pop()

	v, ok = q.Peek()
	assert.True(t, ok, "expected key to be found")
	assert.Equal(t, 2, v, "expected value to be 2, got %d", v)

	q.Pop()

	v, ok = q.Peek()
	assert.True(t, ok, "expected key to be found")
	assert.Equal(t, 3, v, "expected value to be 3, got %d", v)

	q.Pop()

	v, ok = q.Peek()
	assert.False(t, ok, "expected key to be not found")
	assert.Equal(t, 0, v, "expected value to be 0, got %d", v)
}
