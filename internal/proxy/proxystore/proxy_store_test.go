package proxystore_test

import (
	"testing"

	"github.com/dgate-io/dgate-api/internal/proxy/proxystore"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/dgate-io/dgate-api/pkg/storage"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestProxyStory_FileStorage_ChangeLogs(t *testing.T) {
	fstore := storage.NewFileStore(&storage.FileStoreConfig{
		Directory: t.TempDir(),
	})
	pstore := proxystore.New(fstore, zap.NewNop())
	assert.NoError(t, pstore.InitStore())
	defer func() {
		assert.NoError(t, pstore.CloseStore())
	}()

	// Test SaveChangeLog
	cl := &spec.ChangeLog{
		ID:   "test",
		Cmd:  spec.AddNamespaceCommand,
		Item: spec.Namespace{Name: "test"},
	}
	assert.NoError(t, pstore.StoreChangeLog(cl))

	// Test FetchChangeLogs
	logs, err := pstore.FetchChangeLogs()
	assert.NoError(t, err)
	assert.Len(t, logs, 1)

	// Test FetchChangeLogs
	logs, err = pstore.FetchChangeLogs()
	assert.NoError(t, err)
	assert.Len(t, logs, 1)
	assert.Equal(t, cl.ID, logs[0].ID)
	assert.Equal(t, cl.Cmd, logs[0].Cmd)
	assert.NotNil(t, logs[0].Item)

	// Test StoreChangeLog
	assert.NoError(t, pstore.StoreChangeLog(cl))

	// Test DeleteChangeLogs
	err = pstore.DeleteChangeLogs(
		[]*spec.ChangeLog{cl},
	)
	assert.NoError(t, err)

	// Test FetchChangeLogs
	logs, err = pstore.FetchChangeLogs()
	assert.NoError(t, err)
	assert.Len(t, logs, 0)
}

func TestProxyStory_FileStorage_Documents(t *testing.T) {
	fstore := storage.NewFileStore(&storage.FileStoreConfig{
		Directory: t.TempDir(),
	})
	pstore := proxystore.New(fstore, zap.NewNop())
	assert.NoError(t, pstore.InitStore())
	defer func() {
		assert.NoError(t, pstore.CloseStore())
	}()

	// Test StoreDocument
	doc := &spec.Document{
		ID:             "test",
		CollectionName: "col",
		NamespaceName:  "ns",
		Data:           "test",
	}
	assert.NoError(t, pstore.StoreDocument(doc))

	// Test FetchDocument
	doc, err := pstore.FetchDocument("test", "col", "ns")
	assert.NoError(t, err)
	if assert.NotNil(t, doc) {
		assert.NotNil(t, doc.Data)
		if dataString, ok := doc.Data.(string); !ok {
			t.Fatal("failed to convert data to string")
		} else {
			assert.Equal(t, "test", dataString)
		}
	}

	// Test FetchDocuments
	docs, err := pstore.FetchDocuments("col", "ns", 2, 0)
	assert.NoError(t, err)
	assert.Len(t, docs, 1)
	if assert.NotNil(t, doc.Data) {
		assert.Equal(t, "test", docs[0].ID)
		assert.Equal(t, "test", docs[0].Data.(string))
	}

	docs, err = pstore.FetchDocuments("col", "ns", 0, 0)
	assert.NoError(t, err)
	assert.Len(t, docs, 0)

	docs, err = pstore.FetchDocuments("col", "ns", 2, 1)
	assert.NoError(t, err)
	assert.Len(t, docs, 0)

	// Test DeleteDocument
	err = pstore.DeleteDocument("test", "col", "ns")
	assert.NoError(t, err)

	// Test FetchDocument Error
	doc, err = pstore.FetchDocument("test123", "col", "ns")
	assert.NoError(t, err)
	assert.Nil(t, doc)

	// Test FetchDocuments Error
	docs, err = pstore.FetchDocuments("col", "ns", 2, 0)
	assert.NoError(t, err)
	assert.Len(t, docs, 0)

}
