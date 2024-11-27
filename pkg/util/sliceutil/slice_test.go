package sliceutil_test

import (
	"testing"

	"github.com/dgate-io/dgate-api/pkg/util/sliceutil"
)

func TestBinarySearch(t *testing.T) {
	tests := []struct {
		name       string
		items      []int
		search     int
		expected   int
		iterations int
	}{
		{
			name:       "empty",
			items:      []int{},
			search:     1,
			expected:   -1,
			iterations: 0,
		},
		{
			name:       "not found/1",
			items:      []int{1, 3, 5, 7, 9},
			search:     6,
			expected:   -1,
			iterations: 2,
		},
		{
			name:       "not found/2",
			items:      []int{1, 3, 5, 7, 9},
			search:     10,
			expected:   -1,
			iterations: 3,
		},
		{
			name:       "not found/3",
			search:     6,
			expected:   -1,
			iterations: 4,
			items: []int{
				1, 2, 3, 4, 5,
				7, 7, 8, 9, 10,
				11, 12, 13, 14, 15,
				16, 17, 18, 19, 20,
			},
		},
		{
			name:       "found/1",
			items:      []int{1, 2, 3, 4, 5},
			search:     4,
			expected:   3,
			iterations: 2,
		},
		{
			name:       "found/2",
			search:     13,
			expected:   12,
			iterations: 4,
			items: []int{
				1, 2, 3, 4, 5,
				7, 7, 8, 9, 10,
				11, 12, 13, 14, 15,
				16, 17, 18, 19, 20,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iters := 0
			actual := sliceutil.BinarySearch(tt.items, tt.search, func(a, b int) int {
				iters++
				return a - b
			})
			if actual != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, actual)
			}
			if iters != tt.iterations {
				t.Errorf("expected %d iterations, got %d", tt.iterations, iters)
			}
		})
	}
}

func BenchmarkCompareLinearAndBinarySearch(b *testing.B) {
	items := make([]int, 1000000)
	for i := range items {
		items[i] = i
	}

	b.Run("linear", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for _, item := range items {
				if item == 999999 {
					break
				}
			}
		}
	})

	b.Run("binary", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			opts := 0
			sliceutil.BinarySearch(items, 999999, func(a, b int) int {
				opts++
				return a - b
			})
			b.ReportMetric(float64(opts), "opts/op")
		}
	})
}
