package routes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	urllib "net/url"

	"github.com/dgate-io/chi-router"
	"github.com/dgate-io/dgate-api/internal/admin/changestate"
	"github.com/dgate-io/dgate-api/internal/config"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/dgate-io/dgate-api/pkg/util"
	"go.uber.org/zap"
)

func ConfigureServiceAPI(server chi.Router, logger *zap.Logger, cs changestate.ChangeState, appConfig *config.DGateConfig) {
	rm := cs.ResourceManager()
	server.Put("/service", func(w http.ResponseWriter, r *http.Request) {
		eb, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error reading body")
			return
		}
		svc := spec.Service{}
		err = json.Unmarshal(eb, &svc)
		if err != nil {
			if ute, ok := err.(*json.UnmarshalTypeError); ok {
				err = fmt.Errorf("error unmarshalling body: field %s expected type %s", ute.Field, ute.Type.String())
				util.JsonError(w, http.StatusBadRequest, err.Error())
				return
			} else if se, ok := err.(*json.SyntaxError); ok {
				err = fmt.Errorf("error unmarshalling body: syntax error at byte offset %d", se.Offset)
				util.JsonError(w, http.StatusBadRequest, err.Error())
				return
			} else {
				util.JsonError(w, http.StatusBadRequest, "error unmarshalling body")
				return
			}
		}
		if svc.Retries == nil {
			retries := 3
			svc.Retries = &retries
		} else if *svc.Retries < 0 {
			util.JsonError(w, http.StatusBadRequest, "retries must be greater than 0")
			return
		}
		if svc.RetryTimeout != nil && *svc.RetryTimeout < 0 {
			util.JsonError(w, http.StatusBadRequest, "retry timeout must be greater than 0")
			return
		}
		if len(svc.URLs) == 0 {
			util.JsonError(w, http.StatusBadRequest, "urls are required")
			return
		} else {
			for i, url := range svc.URLs {
				errPrefix := fmt.Sprintf(`error on urls["%d"]: `, i)
				if url, err := urllib.Parse(url); err != nil {
					util.JsonError(w, http.StatusBadRequest, errPrefix+err.Error())
				} else {
					if url.Scheme == "" {
						util.JsonError(w, http.StatusBadRequest, errPrefix+"url scheme cannot be empty")
						return
					}
					if url.Host == "" {
						util.JsonError(w, http.StatusBadRequest, errPrefix+"url host cannot be empty")
						return
					}
				}
			}
		}
		if svc.NamespaceName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			svc.NamespaceName = spec.DefaultNamespace.Name
		}

		cl := spec.NewChangeLog(&svc, svc.NamespaceName, spec.AddServiceCommand)
		if err = cs.ApplyChangeLog(cl); err != nil {
			util.JsonError(w, http.StatusBadRequest, err.Error())
			return
		}

		if err := cs.WaitForChanges(cl); err != nil {
			util.JsonError(w, http.StatusInternalServerError, err.Error())
			return
		}

		svcs := rm.GetServicesByNamespace(svc.NamespaceName)
		util.JsonResponse(w, http.StatusCreated,
			spec.TransformDGateServices(svcs...))
	})

	server.Delete("/service", func(w http.ResponseWriter, r *http.Request) {
		eb, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error reading body")
			return
		}
		svc := spec.Service{}
		err = json.Unmarshal(eb, &svc)
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error unmarshalling body")
			return
		}
		if svc.NamespaceName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			svc.NamespaceName = spec.DefaultNamespace.Name
		}
		cl := spec.NewChangeLog(&svc, svc.NamespaceName, spec.DeleteServiceCommand)
		if err = cs.ApplyChangeLog(cl); err != nil {
			util.JsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	server.Get("/service", func(w http.ResponseWriter, r *http.Request) {
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
		util.JsonResponse(w, http.StatusOK, spec.TransformDGateServices(rm.GetServicesByNamespace(nsName)...))
	})

	server.Get("/service/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		nsName := r.URL.Query().Get("namespace")
		if nsName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			nsName = spec.DefaultNamespace.Name
		}
		svc, ok := rm.GetService(name, nsName)
		if !ok {
			util.JsonError(w, http.StatusNotFound, "service not found")
			return
		}
		util.JsonResponse(w, http.StatusOK, spec.TransformDGateService(svc))
	})
}
