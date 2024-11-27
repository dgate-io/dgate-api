package routes

import (
	"encoding/base64"
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

func ConfigureSecretAPI(server chi.Router, logger *zap.Logger, cs changestate.ChangeState, appConfig *config.DGateConfig) {
	rm := cs.ResourceManager()
	server.Put("/secret", func(w http.ResponseWriter, r *http.Request) {
		eb, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error reading body")
			return
		}
		sec := spec.Secret{}
		err = json.Unmarshal(eb, &sec)
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error unmarshalling body")
			return
		}
		if sec.Data == "" {
			util.JsonError(w, http.StatusBadRequest, "payload is required")
			return
		} else {
			sec.Data = base64.RawStdEncoding.EncodeToString([]byte(sec.Data))
		}
		if sec.NamespaceName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			sec.NamespaceName = spec.DefaultNamespace.Name
		}
		cl := spec.NewChangeLog(&sec, sec.NamespaceName, spec.AddSecretCommand)
		if err = cs.ApplyChangeLog(cl); err != nil {
			util.JsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := cs.WaitForChanges(cl); err != nil {
			util.JsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		secrets := rm.GetSecretsByNamespace(sec.NamespaceName)
		util.JsonResponse(w, http.StatusCreated,
			spec.TransformDGateSecrets(secrets...))
	})

	server.Delete("/secret", func(w http.ResponseWriter, r *http.Request) {
		eb, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error reading body")
			return
		}
		sec := spec.Secret{}
		err = json.Unmarshal(eb, &sec)
		if err != nil {
			util.JsonError(w, http.StatusBadRequest, "error unmarshalling body")
			return
		}
		if sec.NamespaceName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			sec.NamespaceName = spec.DefaultNamespace.Name
		}
		cl := spec.NewChangeLog(&sec, sec.NamespaceName, spec.DeleteSecretCommand)
		if err = cs.ApplyChangeLog(cl); err != nil {
			util.JsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})

	server.Get("/secret", func(w http.ResponseWriter, r *http.Request) {
		nsName := r.URL.Query().Get("namespace")
		if nsName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			nsName = spec.DefaultNamespace.Name
		} else if _, ok := rm.GetNamespace(nsName); !ok {
			util.JsonError(w, http.StatusBadRequest, "namespace not found: "+nsName)
			return
		}
		secrets := rm.GetSecretsByNamespace(nsName)
		util.JsonResponse(w, http.StatusOK,
			spec.TransformDGateSecrets(secrets...))
	})

	server.Get("/secret/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		nsName := r.URL.Query().Get("namespace")
		if nsName == "" {
			if appConfig.DisableDefaultNamespace {
				util.JsonError(w, http.StatusBadRequest, "namespace is required")
				return
			}
			nsName = spec.DefaultNamespace.Name
		}
		if sec, ok := rm.GetSecret(name, nsName); !ok {
			util.JsonError(w, http.StatusNotFound, "secret not found")
		} else {
			util.JsonResponse(w, http.StatusOK,
				spec.TransformDGateSecret(sec))
		}
	})
}
