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

func ConfigureDomainAPI(server chi.Router, logger *zap.Logger, cs changestate.ChangeState, appConfig *config.DGateConfig) {
	rm := cs.ResourceManager()
	server.Put("/domain", func(w http.ResponseWriter, r *http.Request) {
		eb, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error reading body")
			return
		}
		domain := spec.Domain{}
		err = json.Unmarshal(eb, &domain)
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error unmarshalling body")
			return
		}

		if domain.Name == "" {
			util.JsonError(w, http.StatusBadRequest, "name is required")
			return
		}
		if domain.NamespaceName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			domain.NamespaceName = spec.DefaultNamespace.Name
		}
		cl := spec.NewChangeLog(&domain, domain.NamespaceName, spec.AddDomainCommand)
		if err = cs.ApplyChangeLog(cl); err != nil {
			util.JsonError(w, http.StatusBadRequest, err.Error())
			return
		}

		if err := cs.WaitForChanges(cl); err != nil {
			util.JsonError(w, http.StatusInternalServerError, err.Error())
			return

		}
		util.JsonResponse(w, http.StatusCreated, spec.TransformDGateDomains(
			rm.GetDomainsByNamespace(domain.NamespaceName)...))
	})

	server.Delete("/domain", func(w http.ResponseWriter, r *http.Request) {
		eb, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error reading body")
			return
		}
		domain := spec.Domain{}
		err = json.Unmarshal(eb, &domain)
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error unmarshalling body")
			return
		}

		if domain.Name == "" {
			util.JsonError(w, http.StatusBadRequest, "name is required")
			return
		}

		if domain.NamespaceName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			domain.NamespaceName = spec.DefaultNamespace.Name
		}

		_, ok := rm.GetDomain(domain.Name, domain.NamespaceName)
		if !ok {
			util.JsonError(w, http.StatusUnprocessableEntity, "domain not found: "+domain.Name)
			return
		}

		cl := spec.NewChangeLog(&domain, domain.NamespaceName, spec.DeleteDomainCommand)
		if err = cs.ApplyChangeLog(cl); err != nil {
			util.JsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})

	server.Get("/domain", func(w http.ResponseWriter, r *http.Request) {
		nsName := r.URL.Query().Get("namespace")
		var dgateDomains []*spec.DGateDomain
		if nsName != "" {
			if _, ok := rm.GetNamespace(nsName); !ok {
				util.JsonError(w, http.StatusBadRequest, "namespace not found: "+nsName)
				return
			}
		} else {
			if !appConfig.DisableDefaultNamespace {
				nsName = spec.DefaultNamespace.Name
			}
		}
		if nsName == "" {
			dgateDomains = rm.GetDomains()
		} else {
			dgateDomains = rm.GetDomainsByNamespace(nsName)
		}
		util.JsonResponse(w, http.StatusOK, spec.TransformDGateDomains(dgateDomains...))
	})

	server.Get("/domain/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		nsName := r.URL.Query().Get("namespace")
		if nsName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			nsName = spec.DefaultNamespace.Name
		}
		dom, ok := rm.GetDomain(name, nsName)
		if !ok {
			util.JsonError(w, http.StatusNotFound, "domain not found")
			return
		}
		util.JsonResponse(w, http.StatusOK, dom)
	})
}
