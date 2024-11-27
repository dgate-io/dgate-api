package linkedlist_test

import (
	"fmt"
	"testing"

	"github.com/dgate-io/dgate-api/pkg/util/linkedlist"
)

func TestMergeSortLinkedListRecursive(t *testing.T) {
	// Example usage
	// Create a linked list: 3 -> 1 -> 4 -> 2 -> 5
	ll := Generate[int](3, 1, 4, 2, 5)

	fmt.Println("Original linked list:")
	linkedlist.DisplayList(ll.Head)

	// Sort the linked list using merge sort
	linkedlist.SortLinkedList(ll, func(i, j int) bool { return i < j })

	fmt.Println("Linked list after sorting:")
	linkedlist.DisplayList(ll.Head)
}

func TestMergeSortLinkedListIterative(t *testing.T) {
	// Example usage
	// Create a linked list: 3 -> 1 -> 4 -> 2 -> 5
	ll := Generate[int](3, 1, 4, 2, 5)

	fmt.Println("Original linked list:")
	linkedlist.DisplayList(ll.Head)

	// Sort the linked list using merge sort
	linkedlist.SortLinkedListIterative(ll, func(i, j int) bool { return i < j })

	fmt.Println("Linked list after sorting:")
	linkedlist.DisplayList(ll.Head)
}

func BenchmarkMergeSortIter(b *testing.B) {
	funcs := map[string]func(*linkedlist.LinkedList[int], func(i, j int) bool){
		"SortLinkedListRecursive": linkedlist.SortLinkedList[int],
		"SortLinkedListIterative": linkedlist.SortLinkedListIterative[int],
	}

	for name, llSortFunc := range funcs {
		b.Run(name, func(b *testing.B) {
			b.StopTimer()
			for i := 0; i < b.N; i++ {
				ll := Generate[int]()
				for j := 10000; j >= 1; j-- {
					ll.Insert((i * j) - (i + j))
				}

				b.StartTimer()
				llSortFunc(ll, func(i, j int) bool { return i < j })
				b.StopTimer()
			}
		})
	}
}

// Generate generates a linked list from the given values.
func Generate[T any](values ...T) *linkedlist.LinkedList[T] {
	l := linkedlist.New[T]()
	for _, v := range values {
		l.Insert(v)
	}
	return l
}
