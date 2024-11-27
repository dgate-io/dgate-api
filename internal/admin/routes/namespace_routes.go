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
	"go.uber.org/zap"
)

func ConfigureNamespaceAPI(server chi.Router, logger *zap.Logger, cs changestate.ChangeState, _ *config.DGateConfig) {
	rm := cs.ResourceManager()
	server.Put("/namespace", func(w http.ResponseWriter, r *http.Request) {
		eb, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error reading body")
			return
		}
		namespace := spec.Namespace{}
		err = json.Unmarshal(eb, &namespace)
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error unmarshalling body")
			return
		}

		if namespace.Name == "" {
			util.JsonError(w, http.StatusBadRequest, "name is required")
			return
		}

		cl := spec.NewChangeLog(&namespace, namespace.Name, spec.AddNamespaceCommand)
		if err = cs.ApplyChangeLog(cl); err != nil {
			util.JsonError(w, http.StatusBadRequest, err.Error())
			return
		}

		if err := cs.WaitForChanges(cl); err != nil {
			util.JsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		util.JsonResponse(w, http.StatusCreated, spec.TransformDGateNamespaces(rm.GetNamespaces()...))
	})

	server.Delete("/namespace", func(w http.ResponseWriter, r *http.Request) {
		eb, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error reading body")
			return
		}
		namespace := spec.Namespace{}
		err = json.Unmarshal(eb, &namespace)
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error unmarshalling body")
			return
		}

		if namespace.Name == "" {
			util.JsonError(w, http.StatusBadRequest, "name is required")
			return
		}

		cl := spec.NewChangeLog(&namespace, namespace.Name, spec.DeleteNamespaceCommand)
		if err = cs.ApplyChangeLog(cl); err != nil {
			util.JsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})

	server.Get("/namespace", func(w http.ResponseWriter, r *http.Request) {
		util.JsonResponse(w, http.StatusOK,
			spec.TransformDGateNamespaces(rm.GetNamespaces()...))
	})

	server.Get("/namespace/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		if ns, ok := rm.GetNamespace(name); !ok {
			util.JsonError(w, http.StatusNotFound, "namespace not found")
		} else {
			util.JsonResponse(w, http.StatusOK, ns)
		}
	})
}
