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

func ConfigureModuleAPI(server chi.Router, logger *zap.Logger, cs changestate.ChangeState, appConfig *config.DGateConfig) {
	rm := cs.ResourceManager()
	server.Put("/module", func(w http.ResponseWriter, r *http.Request) {
		eb, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error reading body")
			return
		}
		mod := spec.Module{}
		err = json.Unmarshal(eb, &mod)
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error unmarshalling body")
			return
		}
		if mod.Payload == "" {
			util.JsonError(w, http.StatusBadRequest, "payload is required")
			return
		}
		if mod.NamespaceName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			mod.NamespaceName = spec.DefaultNamespace.Name
		}
		if !mod.Type.Valid() {
			mod.Type = spec.ModuleTypeTypescript
		}
		cl := spec.NewChangeLog(&mod, mod.NamespaceName, spec.AddModuleCommand)
		if err = cs.ApplyChangeLog(cl); err != nil {
			util.JsonError(w, http.StatusBadRequest, err.Error())
			return
		}

		if err := cs.WaitForChanges(cl); err != nil {
			util.JsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		util.JsonResponse(w, http.StatusCreated, spec.TransformDGateModules(
			rm.GetModulesByNamespace(mod.NamespaceName)...))
	})

	server.Delete("/module", func(w http.ResponseWriter, r *http.Request) {
		eb, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error reading body")
			return
		}
		mod := spec.Module{}
		err = json.Unmarshal(eb, &mod)
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error unmarshalling body")
			return
		}
		if mod.NamespaceName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			mod.NamespaceName = spec.DefaultNamespace.Name
		}
		cl := spec.NewChangeLog(&mod, mod.NamespaceName, spec.DeleteModuleCommand)
		if err = cs.ApplyChangeLog(cl); err != nil {
			util.JsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})

	server.Get("/module", func(w http.ResponseWriter, r *http.Request) {
		nsName := r.URL.Query().Get("namespace")
		if nsName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			nsName = spec.DefaultNamespace.Name
		} else {
			if _, ok := rm.GetNamespace(nsName); !ok {
				util.JsonError(w, http.StatusBadRequest, "namespace not found: "+nsName)
				return
			}
		}
		util.JsonResponse(w, http.StatusCreated, spec.TransformDGateModules(
			rm.GetModulesByNamespace(nsName)...))
	})

	server.Get("/module/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		nsName := r.URL.Query().Get("namespace")
		if nsName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			nsName = spec.DefaultNamespace.Name
		}
		mod, ok := rm.GetModule(name, nsName)
		if !ok {
			util.JsonError(w, http.StatusNotFound, "module not found")
			return
		}
		util.JsonResponse(w, http.StatusOK, spec.TransformDGateModule(mod))
	})
}
