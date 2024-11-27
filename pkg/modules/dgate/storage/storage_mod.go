package storage

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/dgate-io/dgate-api/pkg/modules"
	"github.com/dgate-io/dgate-api/pkg/spec"
)

type StorageModule struct {
	modCtx modules.RuntimeContext
}

var _ modules.GoModule = &StorageModule{}

func New(modCtx modules.RuntimeContext) modules.GoModule {
	return &StorageModule{modCtx}
}

func (sm *StorageModule) Exports() *modules.Exports {
	return &modules.Exports{
		Named: map[string]any{
			"getCache": sm.GetCache,
			"setCache": sm.SetCache,
		},
	}
}

type UpdateFunc func(any, any) any

type CacheOptions struct {
	TTL int `json:"ttl"`
}

func (sm *StorageModule) SetCache(cacheId string, val any, opts CacheOptions) error {
	if cacheId == "" {
		return errors.New("cache id cannot be empty")
	}
	namespace := sm.modCtx.Context().
		Value(spec.Name("namespace"))
	if namespace == nil || namespace.(string) == "" {
		return errors.New("namespace is not set")
	}
	uniqueId := "storage:cache:" + namespace.(string) + ":" + cacheId
	bucket := sm.modCtx.State().SharedCache().Bucket(uniqueId)
	if opts.TTL < 0 {
		return errors.New("TTL cannot be negative")
	} else if opts.TTL > 0 {
		bucket.SetWithTTL(uniqueId, val, time.Duration(opts.TTL)*time.Second)
	}
	return nil
}

func (sm *StorageModule) GetCache(cacheId string) (any, error) {
	if cacheId == "" {
		return nil, errors.New("cache id cannot be empty")
	}
	namespace := sm.modCtx.Context().
		Value(spec.Name("namespace"))
	if namespace == nil || namespace.(string) == "" {
		return nil, errors.New("namespace is not set")
	}
	bucket := sm.modCtx.State().SharedCache().
		Bucket("storage:cache:" + namespace.(string))

	if val, ok := bucket.Get(cacheId); ok {
		return val, nil
	}
	return nil, nil
}

func (sm *StorageModule) ReadWriteBody(res *http.Response, callback func(string) string) string {
	if callback == nil {
		return ""
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		panic(err)
	}
	err = res.Body.Close()
	if err != nil {
		panic(err)
	}
	newBody := callback(string(body))
	res.Body = io.NopCloser(bytes.NewReader([]byte(newBody)))
	res.ContentLength = int64(len(newBody))
	res.Header.Set("Content-Length", strconv.Itoa(len(newBody)))
	return string(newBody)
}

func (sm *StorageModule) GetCollection(collectionName string) *spec.Collection {
	if collectionName == "" {
		return nil
	}
	return nil
}
