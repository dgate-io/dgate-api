package routes

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/dgate-io/chi-router"
	"github.com/dgate-io/dgate-api/internal/admin/changestate"
	"github.com/dgate-io/dgate-api/internal/config"
	"github.com/dgate-io/dgate-api/pkg/util"
)

func ConfigureChangeLogAPI(server chi.Router, cs changestate.ChangeState, appConfig *config.DGateConfig) {
	server.Get("/changelog", func(w http.ResponseWriter, r *http.Request) {
		hash := cs.ChangeHash()
		logs := cs.ChangeLogs()
		lastLogId := ""
		if len(logs) > 0 {
			lastLogId = logs[len(logs)-1].ID
		}
		b, err := json.Marshal(map[string]any{
			"count":  len(logs),
			"hash":   strconv.FormatUint(hash, 36),
			"latest": lastLogId,
		})
		if err != nil {
			util.JsonError(w, http.StatusInternalServerError, err.Error())
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(b))
	})
}

func ConfigureHealthAPI(server chi.Router, version string, cs changestate.ChangeState) {
	healthlyResp := []byte(
		`{"status":"ok","version":"` + version + `"}`,
	)
	server.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(healthlyResp)
	})

	server.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if cs.Ready() {
			if r := cs.Raft(); r != nil {
				w.Header().Set("X-Raft-State", r.State().String())
				if leaderAddr := r.Leader(); leaderAddr == "" {
					w.WriteHeader(http.StatusServiceUnavailable)
					w.Write([]byte(`{"status":"no leader"}`))
					return
				} else {
					w.Header().Set("X-Raft-Leader", string(leaderAddr))
				}
			}
			w.Write(healthlyResp)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"not ready"}`))
		}
	})
}
