package routes

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/dgate-io/chi-router"
	"github.com/dgate-io/dgate-api/internal/admin/changestate"
	"github.com/dgate-io/dgate-api/internal/config"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/dgate-io/dgate-api/pkg/util"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"go.uber.org/zap"
)

func ConfigureCollectionAPI(server chi.Router, logger *zap.Logger, cs changestate.ChangeState, appConfig *config.DGateConfig) {
	rm := cs.ResourceManager()
	dm := cs.DocumentManager()
	server.Put("/collection", func(w http.ResponseWriter, r *http.Request) {
		eb, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error reading body")
			return
		}
		collection := spec.Collection{}
		err = json.Unmarshal(eb, &collection)
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error unmarshalling body")
			return
		}

		if collection.Name == "" {
			util.JsonError(w, http.StatusBadRequest, "name is required")
			return
		}

		if collection.NamespaceName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			collection.NamespaceName = spec.DefaultNamespace.Name
		}

		if oldCollection, ok := rm.GetCollection(collection.Name, collection.NamespaceName); ok {
			if oldCollection.Type == spec.CollectionTypeDocument {
				docs, err := dm.GetDocuments(
					collection.Name,
					collection.NamespaceName,
					0, 0,
				)
				if err != nil {
					util.JsonError(w, http.StatusInternalServerError, err.Error())
					return
				}
				if len(docs) > 0 {
					util.JsonError(w, http.StatusBadRequest, "one or more documents already exist for this collection, please delete existing documents before replacing")
					return
				}
			}
		}

		cl := spec.NewChangeLog(&collection, collection.NamespaceName, spec.AddCollectionCommand)
		if err = cs.ApplyChangeLog(cl); err != nil {
			util.JsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		if err := cs.WaitForChanges(cl); err != nil {
			util.JsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		util.JsonResponse(w, http.StatusCreated, spec.TransformDGateCollections(
			rm.GetCollectionsByNamespace(collection.NamespaceName)...))
	})

	server.Get("/collection", func(w http.ResponseWriter, r *http.Request) {
		namespace := r.URL.Query().Get("namespace")
		if namespace == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			namespace = spec.DefaultNamespace.Name
		}
		collections := rm.GetCollectionsByNamespace(namespace)
		util.JsonResponse(w, http.StatusOK,
			spec.TransformDGateCollections(collections...))
	})

	server.Get("/collection/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		nsName := r.URL.Query().Get("namespace")
		col, ok := rm.GetCollection(name, nsName)
		if !ok {
			util.JsonError(w, http.StatusNotFound, "collection not found")
			return
		}
		util.JsonResponse(w, http.StatusOK, col)
	})

	server.Get("/document", func(w http.ResponseWriter, r *http.Request) {
		namespaceName := r.URL.Query().Get("namespace")
		if namespaceName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			namespaceName = spec.DefaultNamespace.Name
		}
		if _, ok := rm.GetNamespace(namespaceName); !ok {
			util.JsonError(w, http.StatusNotFound, "namespace not found: "+namespaceName)
			return
		}
		collectionName := r.URL.Query().Get("collection")
		if collectionName == "" {
			util.JsonError(w, http.StatusBadRequest, "collection is required")
			return
		}
		if collection, ok := rm.GetCollection(collectionName, namespaceName); !ok {
			util.JsonError(w, http.StatusNotFound, "collection not found")
			return
		} else if collection.Type != "" && collection.Type != spec.CollectionTypeDocument {
			util.JsonError(w, http.StatusBadRequest, "collection is not a document collection")
			return
		} else if collection.Visibility == spec.CollectionVisibilityPrivate {
			util.JsonError(w, http.StatusForbidden, "collection is private")
			return
		}
		limit, err := util.ParseInt(r.URL.Query().Get("limit"), 100)
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "limit must be an integer")
			return
		}
		offset, err := util.ParseInt(r.URL.Query().Get("offset"), 0)
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "offset must be an integer")
			return
		}
		docs, err := dm.GetDocuments(collectionName, namespaceName, offset, limit)
		if err != nil {
			util.JsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		b, err := json.Marshal(map[string]any{
			"documents":  docs,
			"limit":      limit,
			"offset":     offset,
			"count":      len(docs),
			"collection": collectionName,
			"namespace":  namespaceName,
		})
		if err != nil {
			util.JsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	})

	server.Get("/document/{document_id}", func(w http.ResponseWriter, r *http.Request) {
		namespaceName := r.URL.Query().Get("namespace")
		if namespaceName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			namespaceName = spec.DefaultNamespace.Name
		}
		documentId := chi.URLParam(r, "document_id")
		if documentId == "" {
			util.JsonError(w, http.StatusBadRequest, "document_id is required")
			return
		}
		collectionName := r.URL.Query().Get("collection")
		if collectionName == "" {
			util.JsonError(w, http.StatusBadRequest, "collection is required")
			return
		}
		if collection, ok := rm.GetCollection(collectionName, namespaceName); !ok {
			util.JsonError(w, http.StatusNotFound, "collection not found")
			return
		} else if collection.Type != spec.CollectionTypeDocument {
			util.JsonError(w, http.StatusBadRequest, "collection is not a document collection")
			return
		} else if collection.Visibility == spec.CollectionVisibilityPrivate {
			util.JsonError(w, http.StatusForbidden, "collection is private")
			return
		}

		document, err := dm.GetDocumentByID(documentId, collectionName, namespaceName)
		if err != nil {
			util.JsonError(w, http.StatusNotFound, err.Error())
			return
		}

		util.JsonResponse(w, http.StatusOK, document)
	})

	server.Put("/document/{document_id}", func(w http.ResponseWriter, r *http.Request) {
		namespaceName := r.URL.Query().Get("namespace")
		if namespaceName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			namespaceName = spec.DefaultNamespace.Name
		}
		collectionName := r.URL.Query().Get("collection")
		if collectionName == "" {
			util.JsonError(w, http.StatusBadRequest, "collection is required")
			return
		}
		collection, ok := rm.GetCollection(collectionName, namespaceName)
		if !ok {
			util.JsonError(w, http.StatusNotFound, "collection not found")
			return
		}
		documentId := chi.URLParam(r, "document_id")
		if documentId == "" {
			documentId = r.URL.Query().Get("document_id")
			if documentId == "" {
				util.JsonError(w, http.StatusBadRequest, "document_id is required")
				return
			}
		}
		if documentId == "" {
			util.JsonError(w, http.StatusBadRequest, "document_id is required")
			return
		}

		eb, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error reading body")
			return
		}
		var payloadData any
		err = json.Unmarshal(eb, &payloadData)
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error reading body")
			return
		}
		if collection.Type == spec.CollectionTypeDocument && collection.Schema != nil {
			err := collection.Schema.Validate(payloadData)

			if err != nil {
				verrs := err.(*jsonschema.ValidationError)
				validationErrs := make([]string, len(verrs.Causes))
				for i, ve := range verrs.Causes {
					validationErrs[i] = ve.Error()
				}
				util.JsonErrors(w, http.StatusBadRequest, validationErrs)
				return
			}
		}

		doc := spec.Document{
			ID:             documentId,
			NamespaceName:  namespaceName,
			CollectionName: collectionName,
			Data:           payloadData,
		}

		cl := spec.NewChangeLog(&doc, doc.NamespaceName, spec.AddDocumentCommand)
		if err = cs.ApplyChangeLog(cl); err != nil {
			util.JsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		if err := cs.WaitForChanges(cl); err != nil {
			util.JsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		util.JsonResponse(w, http.StatusCreated, doc)
	})

	server.Delete("/document", func(w http.ResponseWriter, r *http.Request) {
		document := spec.Document{}
		namespaceName := r.URL.Query().Get("namespace")
		if namespaceName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			namespaceName = spec.DefaultNamespace.Name
		}

		if _, ok := rm.GetNamespace(namespaceName); !ok {
			util.JsonError(w, http.StatusNotFound, "namespace not found: "+namespaceName)
			return
		}

		collectionName := r.URL.Query().Get("collection")
		if collectionName == "" {
			util.JsonError(w, http.StatusBadRequest, "collection is required")
			return
		}

		if _, ok := rm.GetCollection(collectionName, namespaceName); !ok {
			util.JsonError(w, http.StatusNotFound, "collection not found")
			return
		}

		document.NamespaceName = namespaceName
		document.CollectionName = collectionName

		cl := spec.NewChangeLog(&document, document.NamespaceName, spec.DeleteDocumentCommand)
		err := cs.ApplyChangeLog(cl)
		if err != nil {
			util.JsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})

	server.Delete("/document/{document_id}", func(w http.ResponseWriter, r *http.Request) {
		documentId := chi.URLParam(r, "document_id")
		namespaceName := r.URL.Query().Get("namespace")
		if namespaceName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			namespaceName = spec.DefaultNamespace.Name
		}
		collectionName := r.URL.Query().Get("collection")
		if collectionName == "" {
			util.JsonError(w, http.StatusBadRequest, "collection is required")
			return
		}
		if _, ok := rm.GetCollection(collectionName, namespaceName); !ok {
			util.JsonError(w, http.StatusNotFound, "collection not found")
			return
		}
		if documentId == "" {
			util.JsonError(w, http.StatusBadRequest, "document_id is required")
			return
		}
		document, err := dm.GetDocumentByID(documentId, collectionName, namespaceName)
		if err != nil {
			util.JsonError(w, http.StatusNotFound, err.Error())
			return
		}
		cl := spec.NewChangeLog(document, namespaceName, spec.DeleteDocumentCommand)
		if err = cs.ApplyChangeLog(cl); err != nil {
			util.JsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})

	server.Delete("/collection/{collection_name}", func(w http.ResponseWriter, r *http.Request) {
		collectionName := chi.URLParam(r, "collection_name")
		if collectionName == "" {
			util.JsonError(w, http.StatusBadRequest, "collection_name is required")
			return
		}
		namespaceName := r.URL.Query().Get("namespace")
		if namespaceName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			namespaceName = spec.DefaultNamespace.Name
		}
		var (
			collection *spec.DGateCollection
			ok         bool
		)
		if collection, ok = rm.GetCollection(collectionName, namespaceName); !ok {
			util.JsonError(w, http.StatusNotFound, "collection not found")
			return
		}
		if collection.Type == spec.CollectionTypeDocument {
			docs, err := dm.GetDocuments(
				collectionName,
				namespaceName,
				1, 1,
			)
			if err != nil {
				util.JsonError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if len(docs) > 0 {
				util.JsonError(w, http.StatusBadRequest, "one or more documents already exist for this collection, please delete existing documents before deleting")
				return
			}
		}
		cl := spec.NewChangeLog(collection, namespaceName, spec.DeleteCollectionCommand)
		err := cs.ApplyChangeLog(cl)
		if err != nil {
			util.JsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
}
