package proxystore

import (
	"encoding/json"
	"time"

	"errors"

	"github.com/dgate-io/dgate/pkg/spec"
	"github.com/dgate-io/dgate/pkg/storage"
	"github.com/dgraph-io/badger/v4"
	"go.uber.org/zap"
)

type ProxyStore struct {
	storage storage.Storage
	logger  *zap.Logger
}

func New(storage storage.Storage, logger *zap.Logger) *ProxyStore {
	return &ProxyStore{
		storage: storage,
		logger:  logger,
	}
}

func (store *ProxyStore) InitStore() error {
	err := store.storage.Connect()
	if err != nil {
		return err
	}
	return nil
}

func (store *ProxyStore) FetchChangeLogs() ([]*spec.ChangeLog, error) {
	clBytes, err := store.storage.GetPrefix("changelog/", 0, -1)
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return nil, nil
		}
		return nil, errors.New("failed to fetch changelog" + err.Error())
	}
	if len(clBytes) == 0 {
		return nil, nil
	}
	store.logger.Debug("found changelog entries", zap.Int("numBytes", len(clBytes)))
	logs := make([]*spec.ChangeLog, len(clBytes))
	for i, clKv := range clBytes {
		var clObj spec.ChangeLog
		err = json.Unmarshal(clKv.Value, &clObj)
		if err != nil {
			store.logger.Debug("failed to unmarshal changelog entry", zap.Error(err))
			return nil, errors.New("failed to unmarshal changelog entry: " + err.Error())
		}
		logs[i] = &clObj
	}

	return logs, nil
}

func (store *ProxyStore) StoreChangeLog(cl *spec.ChangeLog) error {
	clBytes, err := json.Marshal(*cl)
	if err != nil {
		return err
	}
	retries, delay := 30, time.Microsecond*100
RETRY:
	err = store.storage.Set("changelog/"+cl.ID, clBytes)
	if err != nil {
		if retries > 0 {
			store.logger.Error("failed to store changelog",
				zap.Error(err), zap.Int("retries", retries),
			)
			time.Sleep(delay)
			retries--
			goto RETRY
		}
		return err
	}
	return nil
}

func (store *ProxyStore) DeleteChangeLogs(logs []*spec.ChangeLog) (int, error) {
	removed := 0
	for _, cl := range logs {
		err := store.storage.Delete("changelog/" + cl.ID)
		if err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func createDocumentKey(docId, colName, nsName string) string {
	return "doc/" + nsName + "/" + colName + "/" + docId
}

func (store *ProxyStore) FetchDocument(docId, colName, nsName string) (*spec.Document, error) {
	docBytes, err := store.storage.Get(createDocumentKey(docId, colName, nsName))
	if err != nil {
		if err == storage.ErrStoreLocked {
			return nil, err
		}
		return nil, errors.New("failed to fetch document: " + err.Error())
	}
	doc := &spec.Document{}
	err = json.Unmarshal(docBytes, doc)
	if err != nil {
		store.logger.Debug("failed to unmarshal document entry: %s, skipping %s",
			zap.Error(err), zap.String("document_id", docId))
		return nil, errors.New("failed to unmarshal document entry" + err.Error())
	}
	return doc, nil
}

func (store *ProxyStore) FetchDocuments(
	namespaceName, collectionName string,
	limit, offset int,
) ([]*spec.Document, error) {
	docs := make([]*spec.Document, 0)
	docPrefix := createDocumentKey("", collectionName, namespaceName)
	err := store.storage.IterateValuesPrefix(docPrefix, func(key string, val []byte) error {
		if offset -= 1; offset > 0 {
			return nil
		} else if limit -= 1; limit != 0 {
			var newDoc spec.Document
			err := json.Unmarshal(val, &newDoc)
			if err != nil {
				return err
			}
			docs = append(docs, &newDoc)
		}
		return nil
	})
	if err != nil {
		return nil, errors.New("failed to fetch documents: " + err.Error())
	}
	return docs, nil
}

func (store *ProxyStore) StoreDocument(doc *spec.Document) error {
	docBytes, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	store.logger.Debug("storing document")
	err = store.storage.Set(createDocumentKey(doc.ID, doc.CollectionName, doc.NamespaceName), docBytes)
	if err != nil {
		return err
	}
	return nil
}

func (store *ProxyStore) DeleteDocument(id, colName, nsName string) error {
	err := store.storage.Delete(createDocumentKey(id, colName, nsName))
	if err != nil {
		if err == badger.ErrKeyNotFound {
			return nil
		}
		return err
	}
	return nil
}

func (store *ProxyStore) DeleteDocuments(doc *spec.Document) error {
	err := store.storage.IterateTxnPrefix(createDocumentKey("", doc.CollectionName, doc.NamespaceName),
		func(txn storage.StorageTxn, key string) error {
			return txn.Delete(key)
		})
	if err != nil {
		return err
	}
	return nil
}
