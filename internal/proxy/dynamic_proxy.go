package proxy

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dgate-io/dgate-api/internal/router"
	"github.com/dgate-io/dgate-api/pkg/modules/extractors"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/dgate-io/dgate-api/pkg/typescript"
	"github.com/dgate-io/dgate-api/pkg/util/iplist"
	"github.com/dgate-io/dgate-api/pkg/util/tree/avl"
	"github.com/dop251/goja"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/sync/errgroup"
)

func (ps *ProxyState) reconfigureState(log *spec.ChangeLog) (err error) {
	defer func() {
		if err != nil {
			ps.logger.Error("error occurred reloading state, restarting...", zap.Error(err))
			go ps.restartState(func(err error) {
				if err != nil {
					ps.logger.Error("Error restarting state", zap.Error(err))
					ps.Stop()
				}
			})
		}
	}()

	ctx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
	defer cancel()

	start := time.Now()
	if err = ps.setupModules(ctx, log); err != nil {
		ps.logger.Error("Error setting up modules", zap.Error(err))
		return
	}
	if err = ps.setupRoutes(ctx, log); err != nil {
		ps.logger.Error("Error setting up routes", zap.Error(err))
		return
	}
	elapsed := time.Since(start)
	ps.logger.Debug("State reloaded",
		zap.Duration("elapsed", elapsed),
	)
	return nil
}

func customErrGroup(ctx context.Context, count int) (*errgroup.Group, context.Context) {
	grp, ctx := errgroup.WithContext(ctx)
	limit := int(math.Log2(float64(count)))
	limit = min(1, max(16, limit))
	grp.SetLimit(limit)
	return grp, ctx
}

func (ps *ProxyState) setupModules(
	ctx context.Context,
	log *spec.ChangeLog,
) error {
	var routes = []*spec.DGateRoute{}
	if log.Namespace == "" || ps.pendingChanges {
		routes = ps.rm.GetRoutes()
	} else {
		routes = ps.rm.GetRoutesByNamespace(log.Namespace)
	}
	programs := avl.NewTree[string, *goja.Program]()
	grp, ctx := customErrGroup(ctx, len(routes))
	start := time.Now()
	for _, rt := range routes {
		if len(rt.Modules) > 0 {
			route := rt
			grp.Go(func() error {
				mod := route.Modules[0]
				var (
					err        error
					program    *goja.Program
					modPayload string = mod.Payload
				)
				if mod.Type == spec.ModuleTypeTypescript {
					tsBucket := ps.sharedCache.Bucket("typescript")
					// hash the typescript module payload
					tsHash, err := HashString(1337, modPayload)
					if err != nil {
						ps.logger.Error("Error hashing module: " + mod.Name)
					} else if cacheData, ok := tsBucket.Get(tsHash); ok {
						if modPayload, ok = cacheData.(string); ok {
							goto compile
						}
					}
					if modPayload, err = typescript.Transpile(ctx, modPayload); err != nil {
						ps.logger.Error("Error transpiling module: " + mod.Name)
						return err
					} else {
						tsBucket.SetWithTTL(tsHash, modPayload, 5*time.Minute)
					}
				}
			compile:
				if mod.Type == spec.ModuleTypeJavascript || mod.Type == spec.ModuleTypeTypescript {
					if program, err = goja.Compile(mod.Name, modPayload, true); err != nil {
						ps.logger.Error("Error compiling module: " + mod.Name)
						return err
					}
				} else {
					return errors.New("invalid module type: " + mod.Type.String())
				}

				tmpCtx := NewRuntimeContext(ps, route, mod)
				defer tmpCtx.Clean()
				if err = extractors.SetupModuleEventLoop(ps.printer, tmpCtx); err != nil {
					ps.logger.Error("Error applying module changes",
						zap.Error(err), zap.String("module", mod.Name),
					)
					return err
				}
				programs.Insert(mod.Name+"/"+route.Namespace.Name, program)
				return nil
			})
		}
	}

	if err := grp.Wait(); err != nil {
		return err
	}
	programs.Each(func(s string, p *goja.Program) bool {
		ps.modPrograms.Insert(s, p)
		return true
	})
	ps.logger.Debug("Modules setup",
		zap.Duration("elapsed", time.Since(start)),
	)
	return nil
}

func (ps *ProxyState) setupRoutes(
	ctx context.Context,
	log *spec.ChangeLog,
) error {
	var rtMap map[string][]*spec.DGateRoute
	if log.Namespace == "" || ps.pendingChanges {
		rtMap = ps.rm.GetRouteNamespaceMap()
		ps.providers.Clear()
	} else {
		rtMap = make(map[string][]*spec.DGateRoute)
		routes := ps.rm.GetRoutesByNamespace(log.Namespace)
		if len(routes) > 0 {
			rtMap[log.Namespace] = routes
		} else {
			// if namespace has no routes, delete the router
			ps.routers.Delete(log.Namespace)
		}
	}
	start := time.Now()
	grp, _ := customErrGroup(ctx, len(rtMap))
	for namespaceName, routes := range rtMap {
		namespaceName, routes := namespaceName, routes
		grp.Go(func() (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("%v", r)
				}
			}()
			mux := router.NewMux()
			for _, rt := range routes {
				reqCtxProvider := NewRequestContextProvider(rt, ps)
				if len(rt.Modules) > 0 {
					modExtFunc := ps.createModuleExtractorFunc(rt)
					if modPool, err := NewModulePool(
						0, 1024, time.Minute*5,
						reqCtxProvider, modExtFunc,
					); err != nil {
						ps.logger.Error("Error creating module buffer", zap.Error(err))
						return err
					} else {
						reqCtxProvider.UpdateModulePool(modPool)
					}
				}
				oldReqCtxProvider := ps.providers.Insert(rt.Namespace.Name+"/"+rt.Name, reqCtxProvider)
				if oldReqCtxProvider != nil {
					oldReqCtxProvider.Close()
				}
				for _, path := range rt.Paths {
					if len(rt.Methods) > 0 && rt.Methods[0] == "*" {
						if len(rt.Methods) > 1 {
							return errors.New("route methods cannot have other methods with *")
						}
						mux.Handle(path, ps.HandleRoute(reqCtxProvider, path))
					} else {
						if len(rt.Methods) == 0 {
							return errors.New("route must have at least one method")
						} else if err = ValidateMethods(rt.Methods); err != nil {
							return err
						}
						for _, method := range rt.Methods {
							mux.Method(method, path, ps.HandleRoute(reqCtxProvider, path))
						}
					}
				}
			}
			if dr, ok := ps.routers.Find(namespaceName); ok {
				dr.ReplaceMux(mux)
			} else {
				dr := router.NewRouterWithMux(mux)
				ps.routers.Insert(namespaceName, dr)
			}
			return nil
		})
	}
	if err := grp.Wait(); err != nil {
		return err
	}
	ps.logger.Debug("Routes setup",
		zap.Duration("elapsed", time.Since(start)),
	)
	return nil
}

func (ps *ProxyState) createModuleExtractorFunc(rt *spec.DGateRoute) ModuleExtractorFunc {
	return func(reqCtx *RequestContextProvider) (_ ModuleExtractor, err error) {
		if len(rt.Modules) == 0 {
			return nil, fmt.Errorf("no modules found for route: %s/%s", rt.Name, rt.Namespace.Name)
		}
		// TODO: Perhaps have some entrypoint flag to determine which module to use
		m := rt.Modules[0]
		if program, ok := ps.modPrograms.Find(m.Name + "/" + rt.Namespace.Name); !ok {
			ps.logger.Error("Error getting module program: invalid state", zap.Error(err))
			return nil, fmt.Errorf("cannot find module program: %s/%s", m.Name, rt.Namespace.Name)
		} else {
			rtCtx := NewRuntimeContext(ps, rt, rt.Modules...)
			if err := extractors.SetupModuleEventLoop(ps.printer, rtCtx, program); err != nil {
				ps.logger.Error("Error creating runtime for route",
					zap.String("route", reqCtx.route.Name),
					zap.String("namespace", reqCtx.route.Namespace.Name),
				)
				return nil, err
			} else {
				loop := rtCtx.EventLoop()
				errorHandler, err := extractors.ExtractErrorHandlerFunction(loop)
				if err != nil {
					ps.logger.Error("Error extracting error handler function", zap.Error(err))
					return nil, err
				}
				fetchUpstream, err := extractors.ExtractFetchUpstreamFunction(loop)
				if err != nil {
					ps.logger.Error("Error extracting fetch upstream function", zap.Error(err))
					return nil, err
				}
				reqModifier, err := extractors.ExtractRequestModifierFunction(loop)
				if err != nil {
					ps.logger.Error("Error extracting request modifier function", zap.Error(err))
					return nil, err
				}
				resModifier, err := extractors.ExtractResponseModifierFunction(loop)
				if err != nil {
					ps.logger.Error("Error extracting response modifier function", zap.Error(err))
					return nil, err
				}
				reqHandler, err := extractors.ExtractRequestHandlerFunction(loop)
				if err != nil {
					ps.logger.Error("Error extracting request handler function", zap.Error(err))
					return nil, err
				}
				return NewModuleExtractor(
					rtCtx, fetchUpstream,
					reqModifier, resModifier,
					errorHandler, reqHandler,
				), nil
			}
		}
	}
}

func (ps *ProxyState) startProxyServer() {
	cfg := ps.config.ProxyConfig
	hostPort := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	ps.logger.Info("Starting proxy server on " + hostPort)
	proxyHttpLogger := ps.logger.Named("http")
	server := &http.Server{
		Addr:     hostPort,
		Handler:  ps,
		ErrorLog: zap.NewStdLog(proxyHttpLogger),
	}
	if cfg.EnableHTTP2 {
		h2Server := &http2.Server{}
		err := http2.ConfigureServer(server, h2Server)
		if err != nil {
			panic(err)
		}
		if cfg.EnableH2C {
			server.Handler = h2c.NewHandler(ps, h2Server)
		}
	}
	if err := server.ListenAndServe(); err != nil {
		ps.logger.Error("error starting proxy server", zap.Error(err))
		panic(err)
	}
}

func (ps *ProxyState) startProxyServerTLS() {
	cfg := ps.config.ProxyConfig
	if cfg.TLS == nil {
		return
	}
	hostPort := fmt.Sprintf("%s:%d", cfg.Host, cfg.TLS.Port)
	ps.logger.Info("Starting secure proxy server on " + hostPort)
	goLogger, err := zap.NewStdLogAt(
		ps.logger.Named("https"),
		zap.DebugLevel,
	)
	if err != nil {
		panic(err)
	}
	secureServer := &http.Server{
		Addr:     hostPort,
		Handler:  ps,
		ErrorLog: goLogger,
		TLSConfig: ps.DynamicTLSConfig(
			cfg.TLS.CertFile,
			cfg.TLS.KeyFile,
		),
	}
	if cfg.EnableHTTP2 {
		h2Server := &http2.Server{}
		err := http2.ConfigureServer(secureServer, h2Server)
		if err != nil {
			panic(err)
		}
		if cfg.EnableH2C {
			secureServer.Handler = h2c.NewHandler(ps, h2Server)
		}
	}
	if err := secureServer.ListenAndServeTLS("", ""); err != nil {
		ps.logger.Error("Error starting secure proxy server", zap.Error(err))
		panic(err)
	}
}

func (ps *ProxyState) Start() (err error) {
	defer func() {
		if err != nil {
			ps.logger.Error("error starting proxy", zap.Error(err))
			ps.Stop()
		}
	}()

	ps.metrics.Setup(ps.config)
	if err = ps.store.InitStore(); err != nil {
		return err
	}

	go ps.startProxyServer()
	go ps.startProxyServerTLS()

	if !ps.raftEnabled {
		if err = ps.restoreFromChangeLogs(false); err != nil {
			return err
		} else {
			ps.SetReady(true)
		}
	}
	return nil
}

func (ps *ProxyState) Stop() {
	go func() {
		defer os.Exit(3)
		<-time.After(7 * time.Second)
		ps.logger.Error("Failed to stop proxy server")
	}()

	ps.logger.Info("Stopping proxy server")
	defer os.Exit(0)
	defer ps.logger.Sync()

	ps.proxyLock.Lock()
	defer ps.proxyLock.Unlock()
	ps.logger.Info("Shutting down raft")

	if raftNode := ps.Raft(); raftNode != nil {
		ps.logger.Info("Stopping Raft node")
		if err := raftNode.Shutdown().Error(); err != nil {
			ps.logger.Error("Error stopping Raft node", zap.Error(err))
		}
	}
}

func (ps *ProxyState) HandleRoute(ctxProvider *RequestContextProvider, pattern string) http.HandlerFunc {
	ipList := iplist.NewIPList()
	if len(ps.config.ProxyConfig.AllowList) > 0 {
		for _, address := range ps.config.ProxyConfig.AllowList {
			if strings.Contains(address, "/") {
				if err := ipList.AddCIDRString(address); err != nil {
					panic(fmt.Errorf("invalid cidr address in proxy.allow_list: %s", address))
				}
			} else {
				if err := ipList.AddIPString(address); err != nil {
					panic(fmt.Errorf("invalid ip address in proxy.allow_list: %s", address))
				}
			}
		}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if ipList.Len() > 0 {
			allowed, err := ipList.Contains(r.RemoteAddr)
			if err != nil {
				ps.logger.Error("Error checking ")
			}
			
			if !allowed {
				http.Error(w, "Forbidden", http.StatusForbidden)
			}
		}
		reqContext := ctxProvider.CreateRequestContext(w, r, pattern)
		ps.ProxyHandler(ps, reqContext)
	}
}
