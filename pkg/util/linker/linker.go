package linker

import (
	"cmp"
	"encoding/json"
	"fmt"

	"github.com/dgate-io/dgate-api/pkg/util/safe"
	"github.com/dgate-io/dgate-api/pkg/util/tree/avl"
)

type kv[T, U any] struct {
	key T
	val U
}

type Linker[K cmp.Ordered] interface {
	Vertex() Linker[K]
	Get(K) Linker[K]
	Len(K) int
	Find(K, K) (Linker[K], bool)
	Each(K, func(K, Linker[K]))
	LinkOneMany(K, K, Linker[K])
	UnlinkOneMany(K, K) (Linker[K], bool)
	UnlinkAllOneMany(K) []Linker[K]
	LinkOneOne(K, K, Linker[K])
	UnlinkOneOne(K) (Linker[K], bool)
	UnlinkOneOneByKey(K, K) (Linker[K], bool)
	Clone() Linker[K]
}

var _ Linker[string] = &Link[string, any]{}

// Link is a vertex in a graph that can be linked to other vertices.
// It is a named vertex that can have multiple edges to other vertices.
// There are two types of edges: one-to-one and one-to-many.
type Link[K cmp.Ordered, V any] struct {
	item  *safe.Ref[V]
	edges []*kv[K, avl.Tree[K, Linker[K]]]
}

func NamedVertexWithVertex[K cmp.Ordered, V any](vertex Linker[K]) *Link[K, V] {
	return vertex.(*Link[K, V])
}

func NewNamedVertex[K cmp.Ordered, V any](names ...K) *Link[K, V] {
	return NewNamedVertexWithValue[K, V](nil, names...)
}

func NewNamedVertexWithValue[K cmp.Ordered, V any](item *V, names ...K) *Link[K, V] {
	edges := make([]*kv[K, avl.Tree[K, Linker[K]]], len(names))
	for i, name := range names {
		edges[i] = &kv[K, avl.Tree[K, Linker[K]]]{
			key: name, val: avl.NewTree[K, Linker[K]](),
		}
	}

	return &Link[K, V]{
		item:  safe.NewRef(item),
		edges: edges,
	}
}

func (nl *Link[K, V]) Vertex() Linker[K] {
	return nl
}

func (nl *Link[K, V]) Item() *V {
	return nl.item.Read()
}

func (nl *Link[K, V]) SetItem(item *V) {
	nl.item.Replace(item)
}

func (nl *Link[K, V]) Get(name K) Linker[K] {
	for _, edge := range nl.edges {
		if edge.key == name {
			if !edge.val.Empty() {
				count := 0
				edge.val.Each(func(key K, val Linker[K]) bool {
					count++
					return count <= 2
				})
				if _, lk, ok := edge.val.RootKeyValue(); ok && count == 1 {
					return lk
				}
				panic("this function should not be called on a vertex with more than one edge per name")
			}
			return nil
		}
	}
	return nil
}

func (nl *Link[K, V]) Len(name K) int {
	for _, edge := range nl.edges {
		if edge.key == name {
			return edge.val.Length()
		}
	}
	return 0
}

func (nl *Link[K, V]) Find(name K, key K) (Linker[K], bool) {
	for _, edge := range nl.edges {
		if edge.key == name {
			return edge.val.Find(key)
		}
	}
	return nil, false
}

// LinkOneMany adds an edge from this vertex to specified vertex
func (nl *Link[K, V]) LinkOneMany(name K, key K, vtx Linker[K]) {
	for _, edge := range nl.edges {
		if edge.key == name {
			edge.val.Insert(key, vtx)
			return
		}
	}
	panic("name not found for this vertex: " + fmt.Sprint(name))
}

// UnlinkOneMany removes links to a vertex and returns the vertex
func (nl *Link[K, V]) UnlinkOneMany(name K, key K) (Linker[K], bool) {
	for _, edge := range nl.edges {
		if edge.key == name {
			return edge.val.Pop(key)
		}
	}
	panic("name not found for this vertex: " + fmt.Sprint(name))
}

// UnlinkAllOneMany removes all edges from the vertex and returns them
func (nl *Link[K, V]) UnlinkAllOneMany(name K) []Linker[K] {
	for _, edge := range nl.edges {
		if edge.key == name {
			var removed []Linker[K]
			edge.val.Each(func(key K, val Linker[K]) bool {
				removed = append(removed, val)
				return true
			})
			edge.val.Clear()
			return removed
		}
	}
	panic("name not found for this vertex: " + fmt.Sprint(name))
}

// LinkOneOne links a vertex to the vertex
func (nl *Link[K, V]) LinkOneOne(name K, key K, vertex Linker[K]) {
	for _, edge := range nl.edges {
		if edge.key == name {
			edge.val.Insert(key, vertex)
			if edge.val.Length() > 1 {
				panic("this function should not be called on a vertex with more than one edge per name")
			}
			return
		}
	}
	panic("name not found for this vertex: " + fmt.Sprint(name))
}

// UnlinkOneOne unlinks a vertex from the vertex and returns the vertex
func (nl *Link[K, V]) UnlinkOneOneByKey(name K, key K) (Linker[K], bool) {
	for _, edge := range nl.edges {
		if edge.key == name {
			return edge.val.Pop(key)
		}
	}
	panic("name not found for this vertex: " + fmt.Sprint(name))
}

// UnlinkOneOne unlinks a vertex from the vertex and returns the vertex
func (nl *Link[K, V]) UnlinkOneOne(name K) (Linker[K], bool) {
	for _, edge := range nl.edges {
		if edge.key == name {
			_, link, ok := edge.val.RootKeyValue()
			if ok {
				edge.val.Clear()
			}
			return link, ok
		}
	}
	panic("name not found for this vertex: " + fmt.Sprint(name))
}

// Clone returns a copy of the vertex
func (nl *Link[K, V]) Clone() Linker[K] {
	edges := make([]*kv[K, avl.Tree[K, Linker[K]]], len(nl.edges))
	for i, edge := range nl.edges {
		edges[i] = &kv[K, avl.Tree[K, Linker[K]]]{
			key: edge.key, val: edge.val.Clone(),
		}
	}
	copiedItem := *nl.item
	return &Link[K, V]{
		item:  &copiedItem,
		edges: edges,
	}
}

// Each iterates over all edges
func (nl *Link[K, V]) Each(name K, fn func(K, Linker[K])) {
	for _, edge := range nl.edges {
		if edge.key == name {
			edge.val.Each(func(key K, vertex Linker[K]) bool {
				fn(key, vertex)
				return true
			})
			return
		}
	}
	panic("name not found for this vertex: " + fmt.Sprint(name))
}

// MarshalJSON implements the json.Marshaler interface
func (nl *Link[K, V]) MarshalJSON() ([]byte, error) {
	type Alias Link[K, V]
	return json.Marshal(&struct {
		*Alias
		Item *V `json:"item"`
	}{
		Alias: (*Alias)(nl),
		Item:  nl.item.Read(),
	})
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (nl *Link[K, V]) UnmarshalJSON(data []byte) error {
	type Alias Link[K, V]
	aux := &struct {
		*Alias
		Item *V `json:"item"`
	}{
		Alias: (*Alias)(nl),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	nl.item = safe.NewRef(aux.Item)
	return nil
}
