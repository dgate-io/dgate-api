package admin

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/dgate-io/chi-router"
	"github.com/dgate-io/dgate-api/internal/admin/changestate"
	"github.com/dgate-io/dgate-api/internal/admin/routes"
	"github.com/dgate-io/dgate-api/internal/config"
	"github.com/dgate-io/dgate-api/pkg/util"
	"github.com/dgate-io/dgate-api/pkg/util/iplist"
	"github.com/hashicorp/raft"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	api "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.uber.org/zap"
)

func configureRoutes(
	server *chi.Mux,
	version string,
	logger *zap.Logger,
	cs changestate.ChangeState,
	conf *config.DGateConfig,
) error {
	adminConfig := conf.AdminConfig
	ipList := iplist.NewIPList()
	for _, address := range adminConfig.AllowList {
		if strings.Contains(address, "/") {
			if err := ipList.AddCIDRString(address); err != nil {
				return fmt.Errorf("invalid cidr address in admin.allow_list: %s", address)
			}
		} else {
			if err := ipList.AddIPString(address); err != nil {
				return fmt.Errorf("invalid ip address in admin.allow_list: %s", address)
			}
		}
	}
	// basic auth
	var userMap map[string]string
	// key auth
	var keyMap map[string]struct{}

	switch adminConfig.AuthMethod {
	case config.AuthMethodBasicAuth:
		userMap = make(map[string]string)
		if len(adminConfig.BasicAuth.Users) > 0 {
			for i, user := range adminConfig.BasicAuth.Users {
				if user.Username == "" || user.Password == "" {
					return errors.New(fmt.Sprintf("both username and password are required: admin.basic_auth.users[%d]", i))
				}
				userMap[user.Username] = user.Password
			}
		}
	case config.AuthMethodKeyAuth:
		keyMap = make(map[string]struct{})
		if adminConfig.KeyAuth != nil && len(adminConfig.KeyAuth.Keys) > 0 {
			if adminConfig.KeyAuth.QueryParamName != "" && adminConfig.KeyAuth.HeaderName != "" {
				return errors.New("only one of admin.key_auth.query_param_name or admin.key_auth.header_name can be set")
			}
			for _, key := range adminConfig.KeyAuth.Keys {
				keyMap[key] = struct{}{}
			}
		}
	case config.AuthMethodJWTAuth:
		return errors.New("JWT Auth is not supported yet")
	}

	server.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ipList.Len() > 0 {
				remoteIp := util.GetTrustedIP(r,
					conf.AdminConfig.XForwardedForDepth)
				allowed, err := ipList.Contains(remoteIp)
				if err != nil {
					if conf.Debug {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
					http.Error(w, "could not parse X-Forwarded-For IP", http.StatusBadRequest)
					return
				}
				if !allowed {
					if conf.Debug {
						http.Error(w, "Unauthorized IP Address: "+remoteIp, http.StatusUnauthorized)
						return
					}
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
			}
			// basic auth
			if adminConfig.AuthMethod == config.AuthMethodBasicAuth {
				if len(adminConfig.BasicAuth.Users) == 0 {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
				user, pass, ok := r.BasicAuth()
				if userPass, userFound := userMap[user]; !ok || !userFound || userPass != pass {
					w.Header().Set("WWW-Authenticate", `Basic realm="Access to DGate Admin API"`)
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
			} else if adminConfig.KeyAuth != nil && len(adminConfig.KeyAuth.Keys) > 0 {
				// key auth
				var key string
				if adminConfig.KeyAuth.QueryParamName != "" {
					key = r.URL.Query().Get(adminConfig.KeyAuth.QueryParamName)
				} else if adminConfig.KeyAuth.HeaderName != "" {
					key = r.Header.Get(adminConfig.KeyAuth.HeaderName)
				} else {
					key = r.Header.Get("X-DGate-Key")
				}
				if _, keyFound := keyMap[key]; !keyFound {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
			}
			if raftInstance := cs.Raft(); raftInstance != nil {
				if r.Method == http.MethodPut || r.Method == http.MethodDelete {
					leader := raftInstance.Leader()
					if leader == "" {
						// TODO: add a way to wait for a leader with a timeout
						util.JsonError(w, http.StatusServiceUnavailable, "raft: no leader")
						return
					}
					if raftInstance.State() != raft.Leader {
						r.URL.Host = string(leader)
						http.Redirect(w, r, r.URL.String(), http.StatusTemporaryRedirect)
						return
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	})

	server.Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-DGate-WatchOnly", fmt.Sprintf("%t", adminConfig.WatchOnly))
		w.Header().Set("X-DGate-ChangeHash", fmt.Sprintf("%d", cs.ChangeHash()))
		if raftInstance := cs.Raft(); raftInstance != nil {
			w.Header().Set(
				"X-DGate-Raft-State",
				raftInstance.State().String(),
			)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("DGate Admin API"))
	}))

	if adminConfig.Replication != nil {
		setupRaft(server, logger.Named("raft"), conf, cs)
	}
	if adminConfig != nil {
		server.Route("/api/v1", func(api chi.Router) {
			apiLogger := logger.Named("api")
			routes.ConfigureRouteAPI(api, apiLogger, cs, conf)
			routes.ConfigureModuleAPI(api, apiLogger, cs, conf)
			routes.ConfigureServiceAPI(api, apiLogger, cs, conf)
			routes.ConfigureNamespaceAPI(api, apiLogger, cs, conf)
			routes.ConfigureDomainAPI(api, apiLogger, cs, conf)
			routes.ConfigureCollectionAPI(api, apiLogger, cs, conf)
			routes.ConfigureSecretAPI(api, apiLogger, cs, conf)
		})
	}

	server.Group(func(misc chi.Router) {
		routes.ConfigureChangeLogAPI(misc, cs, conf)
		routes.ConfigureHealthAPI(misc, version, cs)
		if setupMetricProvider(conf) {
			misc.Handle("/metrics", promhttp.Handler())
		}
	})
	return nil
}

func setupMetricProvider(
	config *config.DGateConfig,
) bool {
	var provider api.MeterProvider
	if !config.DisableMetrics {
		exporter, err := prometheus.New()
		if err != nil {
			log.Fatal(err)
		}
		provider = metric.NewMeterProvider(metric.WithReader(exporter))
	} else {
		provider = noop.NewMeterProvider()
	}
	otel.SetMeterProvider(provider)
	return !config.DisableMetrics
}
