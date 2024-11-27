package linkedlist_test

import (
	"testing"

	"github.com/dgate-io/dgate-api/pkg/util/linkedlist"
	"github.com/stretchr/testify/assert"
)

func TestLinkedListInsert(t *testing.T) {
	l := linkedlist.New[string]()
	l.Insert("a")
	l.Insert("b")
	l.Insert("c")
	assert.Equal(t, 3, l.Len())
	assert.Equal(t, "a", l.Head.Value)
	assert.Equal(t, "c", l.Tail.Value)
}

func TestLinkedListInsertBefore(t *testing.T) {
	l := linkedlist.New[string]()
	l.Insert("a")
	l.Insert("c")
	l.Insert("d")
	assert.Nil(t, l.InsertBefore(l.Head.Next, "b"))
	assert.Equal(t, 4, l.Len())
	assert.Equal(t, "a", l.Head.Value)
	assert.Equal(t, "b", l.Head.Next.Value)
	assert.Equal(t, "c", l.Head.Next.Next.Value)
	assert.Equal(t, "d", l.Head.Next.Next.Next.Value)
	assert.Equal(t, "d", l.Tail.Value)
}

func TestLinkedListInsertBeforeHead(t *testing.T) {
	l := linkedlist.New[string]()
	l.Insert("b")
	l.Insert("c")
	l.Insert("d")
	assert.Nil(t, l.InsertBefore(l.Head, "a"))
	assert.Equal(t, 4, l.Len())
	assert.Equal(t, "a", l.Head.Value)
	assert.Equal(t, "b", l.Head.Next.Value)
	assert.Equal(t, "c", l.Head.Next.Next.Value)
	assert.Equal(t, "d", l.Head.Next.Next.Next.Value)
	assert.Equal(t, "d", l.Tail.Value)
}

func TestLinkedListInsertBeforeTail(t *testing.T) {
	l := linkedlist.New[string]()
	l.Insert("a")
	l.Insert("b")
	l.Insert("d")
	assert.Nil(t, l.InsertBefore(l.Tail, "c"))
	assert.Equal(t, 4, l.Len())
	assert.Equal(t, "a", l.Head.Value)
	assert.Equal(t, "b", l.Head.Next.Value)
	assert.Equal(t, "c", l.Head.Next.Next.Value)
	assert.Equal(t, "d", l.Head.Next.Next.Next.Value)
	assert.Equal(t, "d", l.Tail.Value)
}

func TestLinkedListInsertBeforeNonExisting(t *testing.T) {
	l := linkedlist.New[string]()
	l.Insert("a")
	l.Insert("b")
	l.Insert("c")
	n := &linkedlist.Node[string]{}
	assert.ErrorIs(t, l.InsertBefore(n, "d"),
		linkedlist.ErrNodeNotFound)
	assert.Equal(t, 3, l.Len())
	assert.Equal(t, "a", l.Head.Value)
	assert.Equal(t, "b", l.Head.Next.Value)
	assert.Equal(t, "c", l.Head.Next.Next.Value)
	assert.Nil(t, l.Head.Next.Next.Next)
	assert.Equal(t, "c", l.Tail.Value)
}

func TestLinkedListEmpty(t *testing.T) {
	l := linkedlist.New[string]()
	assert.Equal(t, 0, l.Len())
	assert.True(t, l.Empty())
	assert.Nil(t, l.Head)
	assert.Nil(t, l.Tail)

	l.Insert("a")
	assert.Equal(t, 1, l.Len())
	assert.False(t, l.Empty())
	assert.Nil(t, l.Remove("a"))

	assert.Equal(t, 0, l.Len())
	assert.True(t, l.Empty())
	assert.Nil(t, l.Head)
	assert.Nil(t, l.Tail)
}

func TestLinkedListRemove(t *testing.T) {
	l := linkedlist.New[string]()
	l.Insert("a")
	l.Insert("b")
	l.Insert("c")
	assert.Nil(t, l.Remove("b"))
	assert.Equal(t, 2, l.Len())
	assert.Equal(t, "a", l.Head.Value)
	assert.Equal(t, "c", l.Tail.Value)
}

func TestLinkedListRemoveHead(t *testing.T) {
	l := linkedlist.New[string]()
	assert.ErrorIs(t, l.Remove("d"),
		linkedlist.ErrValueNotFound)
	l.Insert("a")
	l.Insert("b")
	l.Insert("c")
	assert.Nil(t, l.Remove("a"))
	assert.Equal(t, 2, l.Len())
	assert.Equal(t, "b", l.Head.Value)
	assert.Equal(t, "c", l.Tail.Value)
}

func TestLinkedListRemoveTail(t *testing.T) {
	l := linkedlist.New[string]()
	l.Insert("a")
	l.Insert("b")
	l.Insert("c")
	assert.Nil(t, l.Remove("c"))
	assert.Equal(t, 2, l.Len())
	assert.Equal(t, "a", l.Head.Value)
	assert.Equal(t, "b", l.Tail.Value)
}

func TestLinkedListRemoveNonExisting(t *testing.T) {
	l := linkedlist.New[string]()
	l.Insert("a")
	l.Insert("b")
	l.Insert("c")
	assert.ErrorIs(t, l.Remove("d"),
		linkedlist.ErrValueNotFound)
	assert.Equal(t, 3, l.Len())
	assert.Equal(t, "a", l.Head.Value)
	assert.Equal(t, "b", l.Head.Next.Value)
	assert.Equal(t, "c", l.Head.Next.Next.Value)
	assert.Nil(t, l.Head.Next.Next.Next)
	assert.Equal(t, "c", l.Tail.Value)
}

func TestLinkedListContains(t *testing.T) {
	l := linkedlist.New[string]()
	assert.False(t, l.Contains("a"))
	l.Insert("a")
	l.Insert("b")
	l.Insert("c")
	assert.True(t, l.Contains("a"))
	assert.True(t, l.Contains("b"))
	assert.True(t, l.Contains("c"))
	assert.False(t, l.Contains("d"))
}

func TestLinkedListHeadTail(t *testing.T) {
	l := linkedlist.New[string]()
	l.Insert("a")
	l.Insert("b")
	l.Insert("c")
	assert.Equal(t, "a", l.Head.Value)
	assert.Equal(t, "c", l.Tail.Value)
}

func TestLinkedListLen(t *testing.T) {
	l := linkedlist.New[string]()
	l.Insert("a")
	l.Insert("b")
	l.Insert("c")
	assert.Equal(t, 3, l.Len())
}

func TestLinkedListNodeNext(t *testing.T) {
	l := linkedlist.New[string]()
	l.Insert("a")
	l.Insert("b")
	l.Insert("c")
	assert.Equal(t, "a", l.Head.Value)
	assert.Equal(t, "b", l.Head.Next.Value)
	assert.Equal(t, "c", l.Head.Next.Next.Value)
	assert.Nil(t, l.Head.Next.Next.Next)
}

func TestLinkedListNodeValue(t *testing.T) {
	l := linkedlist.New[string]()
	l.Insert("a")
	l.Insert("b")
	l.Insert("c")
	assert.Equal(t, "a", l.Head.Value)
	assert.Equal(t, "b", l.Head.Next.Value)
	assert.Equal(t, "c", l.Head.Next.Next.Value)
}
