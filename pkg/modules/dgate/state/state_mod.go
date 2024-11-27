package state

import (
	"encoding/json"
	"errors"

	"github.com/dgate-io/dgate-api/pkg/modules"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/dop251/goja"
)

type ResourcesModule struct {
	modCtx modules.RuntimeContext
}

var _ modules.GoModule = &ResourcesModule{}

func New(modCtx modules.RuntimeContext) modules.GoModule {
	return &ResourcesModule{modCtx}
}

func (hp *ResourcesModule) Exports() *modules.Exports {
	return &modules.Exports{
		Named: map[string]any{
			"getCollection":    hp.fetchCollection,
			"getDocument":      hp.getDocument,
			"getDocuments":     hp.getDocuments,
			"addCollection":    writeFunc[*spec.Collection](hp, spec.AddCollectionCommand),
			"addDocument":      writeFunc[*spec.Document](hp, spec.AddDocumentCommand),
			"deleteCollection": writeFunc[*spec.Collection](hp, spec.DeleteCollectionCommand),
			"deleteDocument":   writeFunc[*spec.Document](hp, spec.DeleteDocumentCommand),
		},
	}
}

func (hp *ResourcesModule) fetchCollection(name string) *goja.Promise {
	ctx := hp.modCtx.Context()
	state := hp.modCtx.State()
	loop := hp.modCtx.EventLoop()
	rt := hp.modCtx.Runtime()
	rm := state.ResourceManager()
	docPromise, resolve, reject := rt.NewPromise()
	loop.RunOnLoop(func(rt *goja.Runtime) {
		namespace := ctx.Value(spec.Name("namespace"))
		if namespace == nil {
			reject(rt.NewGoError(
				errors.New("namespace not found in context"),
			))
			return
		}
		collection, ok := rm.GetCollection(name, namespace.(string))
		if !ok {
			reject(goja.Null())
			return
		}
		resolve(rt.ToValue(collection))
	})
	return docPromise
}

func (hp *ResourcesModule) getDocument(docId, collection string) *goja.Promise {
	ctx := hp.modCtx.Context()
	state := hp.modCtx.State()
	loop := hp.modCtx.EventLoop()
	rt := loop.Runtime()
	docPromise, resolve, reject := rt.NewPromise()
	loop.RunOnLoop(func(rt *goja.Runtime) {
		namespace := ctx.Value(spec.Name("namespace"))
		if namespace == nil {
			reject(rt.NewGoError(errors.New("namespace not found in context")))
			return
		}
		doc, err := state.DocumentManager().
			GetDocumentByID(docId, collection, namespace.(string))
		if err != nil {
			reject(rt.NewGoError(err))
			return
		}
		resolve(rt.ToValue(doc))
	})
	return docPromise
}

type FetchDocumentsPayload struct {
	Collection string `json:"collection"`
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
}

func (hp *ResourcesModule) getDocuments(payload FetchDocumentsPayload) (*goja.Promise, error) {
	ctx := hp.modCtx.Context()
	state := hp.modCtx.State()
	loop := hp.modCtx.EventLoop()
	rt := hp.modCtx.Runtime()

	if payload.Collection == "" {
		return nil, errors.New("collection name is required")
	}

	namespaceVal := ctx.Value(spec.Name("namespace"))
	if namespaceVal == nil {
		return nil, errors.New("namespace not found in context")
	}
	namespace := namespaceVal.(string)

	prom, resolve, reject := rt.NewPromise()
	loop.RunOnLoop(func(rt *goja.Runtime) {
		docs, err := state.DocumentManager().
			GetDocuments(
				payload.Collection,
				namespace,
				payload.Limit,
				payload.Offset,
			)
		if err != nil {
			reject(rt.NewGoError(err))
			return
		}
		resolve(rt.ToValue(docs))
	})
	return prom, nil
}

func writeFunc[T spec.Named](hp *ResourcesModule, cmd spec.Command) func(map[string]any) (*goja.Promise, error) {
	return func(item map[string]any) (*goja.Promise, error) {
		if item == nil {
			return nil, errors.New("item is nil")
		}
		ctx := hp.modCtx.Context()
		state := hp.modCtx.State()
		loop := hp.modCtx.EventLoop()
		rt := hp.modCtx.Runtime()
		docPromise, resolve, reject := rt.NewPromise()
		loop.RunOnLoop(func(rt *goja.Runtime) {
			rs, err := remarshalNamed[T](item)
			if err != nil {
				reject(rt.NewGoError(err))
				return
			}

			namespaceVal := ctx.Value(spec.Name("namespace"))
			if namespaceVal == nil {
				reject(rt.NewGoError(errors.New("namespace not found in context")))
				return
			}
			namespace := namespaceVal.(string)

			err = state.ApplyChangeLog(spec.NewChangeLog(rs, namespace, cmd))
			if err != nil {
				reject(rt.NewGoError(err))
				return
			}
			resolve(rt.ToValue(rs))
		})
		return docPromise, nil
	}
}

func remarshalNamed[T spec.Named](obj map[string]any) (T, error) {
	var str T
	objBytes, err := json.Marshal(obj)
	if err != nil {
		return str, err
	}
	err = json.Unmarshal(objBytes, &str)
	return str, err
}
