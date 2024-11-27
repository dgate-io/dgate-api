package storage

import (
	"errors"
	"strings"

	"github.com/dgate-io/dgate-api/pkg/util/tree/avl"
	"go.uber.org/zap"
)

type MemStoreConfig struct {
	Logger *zap.Logger
}

type MemStore struct {
	tree avl.Tree[string, []byte]
}

type MemStoreTxn struct {
	store *MemStore
}

var _ Storage = &MemStore{}
var _ StorageTxn = &MemStoreTxn{}

func NewMemStore(cfg *MemStoreConfig) *MemStore {
	return &MemStore{
		tree: avl.NewTree[string, []byte](),
	}
}

func (m *MemStore) Connect() error {
	return nil
}

func (m *MemStore) Get(key string) ([]byte, error) {
	if b, ok := m.tree.Find(key); ok {
		return b, nil
	}
	return nil, errors.New("key not found")
}

func (m *MemStore) Set(key string, value []byte) error {
	m.tree.Insert(key, value)
	return nil
}

func (m *MemStore) Txn(write bool, fn func(StorageTxn) error) error {
	txn := &MemStoreTxn{store: m}
	if err := fn(txn); err != nil {
		return err
	}
	return nil
}

func (m *MemStore) IterateValuesPrefix(prefix string, fn func(string, []byte) error) error {
	check := true
	m.tree.Each(func(k string, v []byte) bool {
		if strings.HasPrefix(k, prefix) {
			check = true
			if err := fn(k, v); err != nil {
				return false
			}
			return true
		}
		return check
	})
	return nil
}

func (m *MemStore) IterateTxnPrefix(prefix string, fn func(StorageTxn, string) error) error {
	m.tree.Each(func(k string, v []byte) bool {
		if strings.HasPrefix(k, prefix) {
			txn := &MemStoreTxn{
				store: m,
			}
			if err := fn(txn, k); err != nil {
				return false
			}
		}
		return true
	})
	return nil
}

func (m *MemStore) GetPrefix(prefix string, offset, limit int) ([]*KeyValue, error) {
	if limit <= 0 {
		limit = 0
	}
	kvs := make([]*KeyValue, 0, limit)
	m.IterateValuesPrefix(prefix, func(key string, value []byte) error {
		if offset <= 0 {
			if len(kvs) >= limit {
				return errors.New("limit reached")
			}
			kvs = append(kvs, &KeyValue{
				Key:   key,
				Value: value,
			})
		} else {
			offset--
		}
		return nil
	})
	return kvs, nil
}

func (m *MemStore) Delete(key string) error {
	if ok := m.tree.Delete(key); !ok {
		return errors.New("key not found")
	}
	return nil
}

func (m *MemStore) Close() error {
	return nil
}

func (t *MemStoreTxn) Get(key string) ([]byte, error) {
	return t.store.Get(key)
}

func (t *MemStoreTxn) Set(key string, value []byte) error {
	return t.store.Set(key, value)
}

func (t *MemStoreTxn) Delete(key string) error {
	return t.store.Delete(key)
}

func (t *MemStoreTxn) GetPrefix(prefix string, offset int, limit int) ([]*KeyValue, error) {
	return t.store.GetPrefix(prefix, offset, limit)
}

func (t *MemStoreTxn) IterateTxnPrefix(prefix string, fn func(txn StorageTxn, key string) error) error {
	return t.store.IterateTxnPrefix(prefix, fn)
}

func (t *MemStoreTxn) IterateValuesPrefix(prefix string, fn func(key string, val []byte) error) error {
	return t.store.IterateValuesPrefix(prefix, fn)
}
