package admin

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dgate-io/chi-router"
	"github.com/dgate-io/chi-router/middleware"
	"github.com/dgate-io/dgate-api/internal/admin/changestate"
	"github.com/dgate-io/dgate-api/internal/config"
	"github.com/dgate-io/dgate-api/pkg/util"
	"go.uber.org/zap"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func StartAdminAPI(
	version string, conf *config.DGateConfig,
	logger *zap.Logger, cs changestate.ChangeState,
) error {
	if conf.AdminConfig == nil {
		logger.Warn("Admin API is disabled")
		return nil
	}

	mux := chi.NewRouter()
	if err := configureRoutes(mux, version, logger.Named("routes"), cs, conf); err != nil {
		return err
	}

	// Start HTTP Server
	go func() {
		adminHttpLogger := logger.Named("http")
		hostPort := fmt.Sprintf("%s:%d",
			conf.AdminConfig.Host, conf.AdminConfig.Port)
		logger.Info("Starting admin api on " + hostPort)
		server := &http.Server{
			Addr:     hostPort,
			Handler:  mux,
			ErrorLog: zap.NewStdLog(adminHttpLogger),
		}
		if err := server.ListenAndServe(); err != nil {
			logger.Error("Error starting admin api", zap.Error(err))
			panic(err)
		}
	}()

	// Start HTTPS Server
	go func() {
		if conf.AdminConfig.TLS != nil {
			adminHttpsLog := logger.Named("https")
			secureHostPort := fmt.Sprintf("%s:%d",
				conf.AdminConfig.Host, conf.AdminConfig.TLS.Port)
			secureServer := &http.Server{
				Addr:     secureHostPort,
				Handler:  mux,
				ErrorLog: zap.NewStdLog(adminHttpsLog),
			}
			logger.Info("Starting secure admin api on" + secureHostPort)
			logger.Debug("TLS Cert",
				zap.String("cert_file", conf.AdminConfig.TLS.CertFile),
				zap.String("key_file", conf.AdminConfig.TLS.KeyFile),
			)
			if err := secureServer.ListenAndServeTLS(
				conf.AdminConfig.TLS.CertFile,
				conf.AdminConfig.TLS.KeyFile,
			); err != nil {
				panic(err)
			}
		}
	}()

	// Start Test Server
	if conf.TestServerConfig != nil {
		if !conf.Debug {
			logger.Warn("Test server is disabled in non-debug mode")
		} else {
			go func() {
				testHostPort := fmt.Sprintf("%s:%d",
					conf.TestServerConfig.Host, conf.TestServerConfig.Port)
				testMux := chi.NewRouter()
				testMux.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
					if strings.HasPrefix(r.URL.Path, "/debug") {
						// strip /debug prefix
						r.URL.Path = strings.TrimPrefix(r.URL.Path, "/debug")
						middleware.Profiler().ServeHTTP(w, r)
						return
					}
					respMap := map[string]any{}
					if waitStr := r.URL.Query().Get("wait"); waitStr != "" {
						if waitTime, err := time.ParseDuration(waitStr); err != nil {
							util.JsonResponse(w, http.StatusBadRequest, map[string]string{
								"error": fmt.Sprintf("Invalid wait time: %s", waitStr),
							})
							return
						} else {
							respMap["waited"] = waitTime.String()
							time.Sleep(waitTime)
						}
					}
					respMap["method"] = r.Method
					respMap["path"] = r.URL.String()
					if body, err := io.ReadAll(r.Body); err == nil {
						respMap["body"] = string(body)
					}
					respMap["host"] = r.Host
					respMap["remote_addr"] = r.RemoteAddr
					respMap["req_headers"] = r.Header
					if conf.TestServerConfig.EnableEnvVars {
						respMap["env"] = os.Environ()
					}
					respMap["global_headers"] = conf.TestServerConfig.GlobalHeaders
					for k, v := range conf.TestServerConfig.GlobalHeaders {
						w.Header().Set(k, v)
					}
					util.JsonResponse(w, http.StatusOK, respMap)
				})

				testServerLogger := logger.Named("test-server")
				testServer := &http.Server{
					Addr:     testHostPort,
					Handler:  testMux,
					ErrorLog: zap.NewStdLog(testServerLogger),
				}
				if conf.TestServerConfig.EnableHTTP2 {
					h2Server := &http2.Server{}
					err := http2.ConfigureServer(testServer, h2Server)
					if err != nil {
						panic(err)
					}
					if conf.TestServerConfig.EnableH2C {
						testServer.Handler = h2c.NewHandler(testServer.Handler, h2Server)
					}
				}
				logger.Info("Starting test server on " + testHostPort)

				if err := testServer.ListenAndServe(); err != nil {
					panic(err)
				}
			}()
		}
	}
	return nil
}
