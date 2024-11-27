package avl_test

import (
	"math/rand"
	"testing"

	"github.com/dgate-io/dgate-api/pkg/util/tree/avl"
)

// Test AVL Tree Insertion
func TestAVLTreeInsertDelete(t *testing.T) {
	tree := avl.NewTree[string, int]() // Example with string keys and int values

	testCases := map[string]int{
		"lemon":      110,
		"lime":       120,
		"lychee":     130,
		"papaya":     140,
		"cherry":     150,
		"berry":      160,
		"fig":        170,
		"strawberry": 180,
		"apricot":    190,
		"avocado":    200,
		"apple":      10,
		"orange":     20,
		"banana":     30,
		"grapes":     40,
		"mango":      50,
		"kiwi":       60,
		"pear":       70,
		"peach":      80,
		"plum":       90,
		"guava":      100,
	}

	// Insertion tests
	for key, value := range testCases {
		tree.Insert(key, value)
	}

	// Inorder traversal to check correctness
	expectedOrder := []string{
		"apple",
		"apricot",
		"avocado",
		"banana",
		"berry",
		"cherry",
		"fig",
		"grapes",
		"guava",
		"kiwi",
		"lemon",
		"lime",
		"lychee",
		"mango",
		"orange",
		"papaya",
		"peach",
		"pear",
		"plum",
		"strawberry",
	}

	var actualOrder []string

	tree.Each(func(key string, value int) bool {
		actualOrder = append(actualOrder, key)
		return true
	})

	// Check if the traversal order matches the expected order
	for i, key := range actualOrder {
		if key != expectedOrder[i] {
			t.Errorf("Expected order %v, got %v", expectedOrder, actualOrder)
			break
		}
	}

	// Deletion tests
	switcher := false
	for len(expectedOrder) > 0 {
		if switcher {
			// delete something that exists
			randomIndex := rand.Intn(len(expectedOrder))
			if !tree.Delete(expectedOrder[randomIndex]) {
				t.Errorf("Expected deletion of %s to succeed", expectedOrder[randomIndex])
			}
			// remove the deleted key from the expected order
			expectedOrder = append(expectedOrder[:randomIndex], expectedOrder[randomIndex+1:]...)
		} else {
			// search for something that doesn't exist
			randomIndex := rand.Intn(len(expectedOrder))
			nonExistentKey := expectedOrder[randomIndex] + "-nonexistent"
			if tree.Delete(nonExistentKey) {
				t.Errorf("Expected deletion of %s to fail", nonExistentKey)
			}
		}

		// Perform inorder traversal
		actualOrder = nil
		tree.Each(func(key string, value int) bool {
			actualOrder = append(actualOrder, key)
			return true
		})

		// Check if the traversal order matches the expected order
		for i, key := range actualOrder {
			if key != expectedOrder[i] {
				t.Errorf("Expected order %v, got %v", expectedOrder, actualOrder)
				break
			}
		}

		switcher = !switcher

	}
}

func TestAVLTreeEach(t *testing.T) {
	tree := avl.NewTree[int, int]() // Example with string keys and int values

	// Insertion tests
	for i := 0; i < 1000; i++ {
		tree.Insert(i, i)
	}

	// Inorder traversal to check correctness
	i := 0
	tree.Each(func(k int, v int) bool {
		if k != i {
			t.Errorf("Expected %d, got %d", i, k)
		}
		i++
		return true
	})

	// Inorder traversal stopping early
	i = 0
	tree.Each(func(k int, v int) bool {
		if k != i {
			t.Errorf("Expected %d, got %d", i, k)
		}
		i++
		return i < 500
	})
	if i != 500 {
		t.Errorf("Expected 500, got %d", i)
	}
}

func TestAVLTreeHeight(t *testing.T) {
	tree := avl.NewTree[int, int]() // Example with string keys and int values
	if tree.Height() != 0 {
		t.Errorf("Expected height 1, got %d", tree.Height())
	}
	tree.Insert(-1, -1)
	if tree.Height() != 1 {
		t.Errorf("Expected height 1, got %d", tree.Height())
	}
	tree.Insert(-2, -2)
	if tree.Height() != 2 {
		t.Errorf("Expected height 2, got %d", tree.Height())
	}

	// Insertion tests
	for i := 0; i < 1000; i++ {
		tree.Insert(i, i)
		treeHeight(t, tree)
	}
}

func TestAVLTreeInsertion(t *testing.T) {
	tree := avl.NewTree[int, int]() // Example with string keys and int values

	t.Run("0-999", func(t *testing.T) {
		// Insertion tests
		for i := 0; i < 1000; i++ {
			tree.Insert(i, i)
		}

		treeHeight(t, tree)
		treeLength(t, tree)

		// Inorder traversal to check correctness
		for i := 0; i < 999; i++ {
			x, ok := tree.Find(i)
			if !ok {
				t.Errorf("Expected %d, got not found", i)
			}
			if x != i {
				t.Errorf("Expected %d, got %d", i, x)
			}
		}
	})
	t.Run("999-0", func(t *testing.T) {
		// Insertion overwrite tests
		for i := 999; i >= 0; i-- {
			tree.Insert(i, i)
		}

		treeHeight(t, tree)
		treeLength(t, tree)

		// Inorder traversal to check correctness
		for i := 0; i < 999; i++ {
			x, ok := tree.Find(i)
			if !ok {
				t.Errorf("Expected %d, got not found", i)
			}
			if x != i {
				t.Errorf("Expected %d, got %d", i, x)
			}
		}
	})
}

func treeHeight(t *testing.T, tree avl.Tree[int, int]) int {
	if tree.Empty() {
		return 0
	}
	l := avl.NewTreeFromLeft(tree)
	r := avl.NewTreeFromRight(tree)
	totalTreeHeight := 1 + max(treeHeight(t, l), treeHeight(t, r))
	if totalTreeHeight != tree.Height() {
		t.Errorf("Expected height %d, got %d", totalTreeHeight, tree.Height())
	}
	return totalTreeHeight
}

func treeLength(t *testing.T, tree avl.Tree[int, int]) int {
	if tree.Empty() {
		return 0
	}
	l := avl.NewTreeFromLeft(tree)
	r := avl.NewTreeFromRight(tree)
	totalTreeLength := 1 + treeLength(t, l) + treeLength(t, r)
	if totalTreeLength != tree.Length() {
		t.Errorf("Expected length %d, got %d", totalTreeLength, tree.Length())
	}
	return totalTreeLength
}

// Benchmark AVL Tree Insertion in ascending order
func BenchmarkAVLTreeInsertAsc(b *testing.B) {
	tree := avl.NewTree[int, int]() // Example with string keys and int values

	// Run the insertion operation b.N times
	for i := 0; i < b.N; i++ {
		tree.Insert(i, i)
	}
}

// Benchmark AVL Tree Insertion in descending order
func BenchmarkAVLTreeInsertDesc(b *testing.B) {
	tree := avl.NewTree[int, int]() // Example with string keys and int values

	// Run the insertion operation b.N times
	for i := 0; i < b.N; i++ {
		tree.Insert(b.N-i, i)
	}
}

// Benchmark AVL Tree Find operation
func BenchmarkAVLTreeFind(b *testing.B) {
	tree := avl.NewTree[int, int]()

	// Insert k nodes into the tree
	k := 1_000_000
	{
		b.StopTimer()
		for i := 0; i < k; i++ {
			tree.Insert(i, i)
		}
		b.StartTimer()
	}

	// Run the find operation b.N times
	for i := 0; i < b.N; i++ {
		tree.Find(i % k)
	}
}

// Benchmark AVL Tree Each operation
func BenchmarkAVLTreeEach(b *testing.B) {
	tree := avl.NewTree[int, int]()

	// Insert k nodes into the tree
	k := 10_000
	{
		b.StopTimer()
		for i := 0; i < k; i++ {
			tree.Insert(i, i)
		}
		b.StartTimer()
	}

	// Run the each operation b.N times
	for i := 0; i < b.N; i++ {
		tree.Each(func(k int, v int) bool {
			return true
		})
	}
}

func BenchmarkAVLTreeInsertAndFindParallel(b *testing.B) {
	tree := avl.NewTree[int, int]()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			tree.Insert(i, i)
			i++
		}
	})

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			tree.Find(0)
			i++
		}
	})
}
