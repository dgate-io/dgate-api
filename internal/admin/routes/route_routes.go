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

func ConfigureRouteAPI(server chi.Router, logger *zap.Logger, cs changestate.ChangeState, appConfig *config.DGateConfig) {
	rm := cs.ResourceManager()
	server.Put("/route", func(w http.ResponseWriter, r *http.Request) {
		eb, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error reading body")
			return
		}
		route := spec.Route{}
		err = json.Unmarshal(eb, &route)
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error unmarshalling body")
			return
		}

		if route.Name == "" {
			util.JsonError(w, http.StatusBadRequest, "name is required")
			return
		}

		if route.NamespaceName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			route.NamespaceName = spec.DefaultNamespace.Name
		}

		cl := spec.NewChangeLog(&route, route.NamespaceName, spec.AddRouteCommand)
		if err = cs.ApplyChangeLog(cl); err != nil {
			util.JsonError(w, http.StatusBadRequest, err.Error())
			return
		}

		if err := cs.WaitForChanges(cl); err != nil {
			util.JsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		util.JsonResponse(w, http.StatusCreated, spec.TransformDGateRoutes(
			rm.GetRoutesByNamespace(route.NamespaceName)...))
	})

	server.Delete("/route", func(w http.ResponseWriter, r *http.Request) {
		eb, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error reading body")
			return
		}
		route := spec.Route{}
		err = json.Unmarshal(eb, &route)
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error unmarshalling body")
			return
		}

		if route.Name == "" {
			util.JsonError(w, http.StatusBadRequest, "name is required")
			return
		}

		if route.NamespaceName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			route.NamespaceName = spec.DefaultNamespace.Name
		}

		cl := spec.NewChangeLog(&route, route.NamespaceName, spec.DeleteRouteCommand)
		if err = cs.ApplyChangeLog(cl); err != nil {
			util.JsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})

	server.Get("/route", func(w http.ResponseWriter, r *http.Request) {
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
		routes := rm.GetRoutesByNamespace(nsName)
		util.JsonResponse(w, http.StatusCreated,
			spec.TransformDGateRoutes(routes...))
	})

	server.Get("/route/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		nsName := r.URL.Query().Get("namespace")
		if nsName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			nsName = spec.DefaultNamespace.Name
		}
		rt, ok := rm.GetRoute(name, nsName)
		if !ok {
			util.JsonError(w, http.StatusNotFound, "route not found")
			return
		}
		util.JsonResponse(w, http.StatusOK,
			spec.TransformDGateRoute(rt))
	})
}
