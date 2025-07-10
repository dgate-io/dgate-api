package proxy

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/dgate-io/dgate-api/pkg/util"
	"go.uber.org/zap"
)

type ProxyHandlerFunc func(ps *ProxyState, reqCtx *RequestContext)

func proxyHandler(ps *ProxyState, reqCtx *RequestContext) {
	defer func() {
		if reqCtx.req.Body != nil {
			// Ensure that the request body is drained/closed, so the connection can be reused
			io.Copy(io.Discard, reqCtx.req.Body)
			reqCtx.req.Body.Close()
		}

		event := ps.logger.
			With(
				zap.String("route", reqCtx.route.Name),
				zap.String("namespace", reqCtx.route.Namespace.Name),
				zap.String("path", reqCtx.req.URL.Path),
				zap.String("method", reqCtx.req.Method),
				zap.String("query", reqCtx.req.URL.RawQuery),
				zap.String("protocol", reqCtx.req.Proto),
				zap.String("remote_address", reqCtx.req.RemoteAddr),
				zap.String("user_agent", reqCtx.req.UserAgent()),
				zap.Int64("content_length", reqCtx.req.ContentLength),
				zap.String("content_type", reqCtx.req.Header.Get("Content-Type")),
			)

		if reqCtx.route.Service != nil {
			event = event.With(zap.String("service", reqCtx.route.Service.Name))
		}
		event.Debug("Request log")
	}()

	defer ps.metrics.MeasureProxyRequest(reqCtx.ctx, reqCtx, time.Now())

	var modExt ModuleExtractor
	if len(reqCtx.route.Modules) != 0 {
		runtimeStart := time.Now()
		if modPool := reqCtx.provider.ModulePool(); modPool == nil {
			ps.logger.Error("Error getting module buffer: invalid state")
			util.WriteStatusCodeError(reqCtx.rw, http.StatusInternalServerError)
			return
		} else {
			if modExt = modPool.Borrow(); modExt == nil {
				ps.metrics.MeasureModuleDuration(
					reqCtx.ctx, reqCtx, "module_extract", runtimeStart,
					errors.New("error borrowing module"),
				)
				ps.logger.Error("Error borrowing module")
				util.WriteStatusCodeError(reqCtx.rw, http.StatusInternalServerError)
				return
			}
			defer modPool.Return(modExt)
		}

		modExt.Start(reqCtx)
		defer modExt.Stop(true)
		ps.metrics.MeasureModuleDuration(
			reqCtx.ctx, reqCtx, "module_extract",
			runtimeStart, nil,
		)
	} else {
		modExt = NewEmptyModuleExtractor()
	}

	if reqCtx.route.Service != nil {
		handleServiceProxy(ps, reqCtx, modExt)
	} else {
		requestHandlerModule(ps, reqCtx, modExt)
	}
}

func handleServiceProxy(ps *ProxyState, reqCtx *RequestContext, modExt ModuleExtractor) {
	var upstreamUrlString string
	if fetchUpstreamUrl, ok := modExt.FetchUpstreamUrlFunc(); ok {
		fetchUpstreamStart := time.Now()
		hostUrl, err := fetchUpstreamUrl(modExt.ModuleContext())
		ps.metrics.MeasureModuleDuration(
			reqCtx.ctx, reqCtx, "fetch_upstream",
			fetchUpstreamStart, err,
		)
		if err != nil {
			ps.logger.Error("Error fetching upstream",
				zap.String("error", err.Error()),
				zap.String("route", reqCtx.route.Name),
				zap.String("namespace", reqCtx.route.Namespace.Name),
			)
			util.WriteStatusCodeError(reqCtx.rw, http.StatusInternalServerError)
			return
		}
		upstreamUrlString = hostUrl.String()
	} else {
		if len(reqCtx.route.Service.URLs) == 0 {
			ps.logger.Error("Error getting service urls",
				zap.String("service", reqCtx.route.Service.Name),
				zap.String("namespace", reqCtx.route.Namespace.Name),
			)
			util.WriteStatusCodeError(reqCtx.rw, http.StatusInternalServerError)
			return
		}
		upstreamUrlString = reqCtx.route.Service.URLs[0].String()
	}

	if reqCtx.route.Service.HideDGateHeaders {
		// upstream headers
		reqCtx.req.Header.Set("X-DGate-Service", reqCtx.route.Service.Name)
		reqCtx.req.Header.Set("X-DGate-Route", reqCtx.route.Name)
		reqCtx.req.Header.Set("X-DGate-Namespace", reqCtx.route.Namespace.Name)
		for _, tag := range ps.config.Tags {
			reqCtx.req.Header.Add("X-DGate-Tags", tag)
		}

		// downstream headers
		if ps.debugMode {
			reqCtx.rw.Header().Set("X-Upstream-URL", upstreamUrlString)
		}
	}
	upstreamUrl, err := url.Parse(upstreamUrlString)
	if err != nil {
		ps.logger.Error("Error parsing upstream url",
			zap.String("error", err.Error()),
			zap.String("route", reqCtx.route.Name),
			zap.String("service", reqCtx.route.Service.Name),
			zap.String("namespace", reqCtx.route.Namespace.Name),
		)
		util.WriteStatusCodeError(reqCtx.rw, http.StatusBadGateway)
		return
	}

	var upstreamErr error
	rpb := reqCtx.provider.rpb.Clone().
		ModifyResponse(func(res *http.Response) error {
			if reqCtx.route.Service.HideDGateHeaders {
				res.Header.Set("Via", "DGate Proxy")
			}
			if responseModifier, ok := modExt.ResponseModifierFunc(); ok {
				resModifierStart := time.Now()
				err = responseModifier(modExt.ModuleContext(), res)
				ps.metrics.MeasureModuleDuration(
					reqCtx.ctx, reqCtx,
					"response_modifier",
					resModifierStart, err,
				)
				if err != nil {
					ps.logger.Error("Error modifying response",
						zap.String("error", err.Error()),
						zap.String("route", reqCtx.route.Name),
						zap.String("service", reqCtx.route.Service.Name),
						zap.String("namespace", reqCtx.route.Namespace.Name),
					)
					return err
				}
			}
			return nil
		}).
		ErrorHandler(func(w http.ResponseWriter, r *http.Request, reqErr error) {
			upstreamErr = reqErr
			ps.logger.Error("Error proxying request",
				zap.String("error", reqErr.Error()),
				zap.String("route", reqCtx.route.Name),
				zap.String("service", reqCtx.route.Service.Name),
				zap.String("namespace", reqCtx.route.Namespace.Name),
			)
			// TODO: add metric for error
			if reqCtx.rw.HeadersSent() {
				return
			}
			if errorHandler, ok := modExt.ErrorHandlerFunc(); ok {
				errorHandlerStart := time.Now()
				err = errorHandler(modExt.ModuleContext(), reqErr)
				ps.metrics.MeasureModuleDuration(
					reqCtx.ctx, reqCtx, "error_handler",
					errorHandlerStart, err,
				)
				if err != nil {
					ps.logger.Error("Error handling error",
						zap.String("error", err.Error()),
						zap.String("route", reqCtx.route.Name),
						zap.String("service", reqCtx.route.Service.Name),
						zap.String("namespace", reqCtx.route.Namespace.Name),
					)
					util.WriteStatusCodeError(reqCtx.rw, http.StatusInternalServerError)
					return
				}
			}
			if !reqCtx.rw.HeadersSent() && reqCtx.rw.BytesWritten() == 0 {
				util.WriteStatusCodeError(reqCtx.rw, http.StatusBadGateway)
			}
		})

	if requestModifier, ok := modExt.RequestModifierFunc(); ok {
		reqModifierStart := time.Now()
		err = requestModifier(modExt.ModuleContext())
		ps.metrics.MeasureModuleDuration(
			reqCtx.ctx, reqCtx,
			"request_modifier",
			reqModifierStart, err,
		)
		if err != nil {
			ps.logger.Error("Error modifying request",
				zap.String("error", err.Error()),
				zap.String("route", reqCtx.route.Name),
				zap.String("service", reqCtx.route.Service.Name),
				zap.String("namespace", reqCtx.route.Namespace.Name),
			)
			util.WriteStatusCodeError(reqCtx.rw, http.StatusInternalServerError)
			return
		}
	}

	rp, err := rpb.Build(upstreamUrl, reqCtx.pattern)
	if err != nil {
		ps.logger.Error("Error creating reverse proxy",
			zap.String("error", err.Error()),
			zap.String("route", reqCtx.route.Name),
			zap.String("service", reqCtx.route.Service.Name),
			zap.String("namespace", reqCtx.route.Namespace.Name),
		)
		util.WriteStatusCodeError(reqCtx.rw, http.StatusInternalServerError)
		return
	}
	// Set Upstream Response Headers
	for k, v := range ps.config.ProxyConfig.GlobalHeaders {
		reqCtx.rw.Header().Set(k, v)
	}

	upstreamStart := time.Now()
	rp.ServeHTTP(reqCtx.rw, reqCtx.req)
	ps.metrics.MeasureUpstreamDuration(
		reqCtx.ctx, reqCtx,
		upstreamStart,
		upstreamUrl.String(),
		upstreamErr,
	)
}

func requestHandlerModule(ps *ProxyState, reqCtx *RequestContext, modExt ModuleExtractor) {
	var err error
	if requestModifier, ok := modExt.RequestModifierFunc(); ok {
		reqModifierStart := time.Now()
		err = requestModifier(modExt.ModuleContext())
		ps.metrics.MeasureModuleDuration(
			reqCtx.ctx, reqCtx,
			"request_modifier",
			reqModifierStart, err,
		)
		if err != nil {
			ps.logger.Error("Error modifying request",
				zap.String("error", err.Error()),
				zap.String("route", reqCtx.route.Name),
				zap.String("namespace", reqCtx.route.Namespace.Name),
			)
			util.WriteStatusCodeError(reqCtx.rw, http.StatusInternalServerError)
			return
		}
	}
	if requestHandler, ok := modExt.RequestHandlerFunc(); ok {
		requestHandlerStart := time.Now()
		err := requestHandler(modExt.ModuleContext())
		defer ps.metrics.MeasureModuleDuration(
			reqCtx.ctx, reqCtx,
			"request_handler",
			requestHandlerStart, err,
		)
		if err != nil {
			ps.logger.Error("Error @ request_handler module",
				zap.String("error", err.Error()),
				zap.String("route", reqCtx.route.Name),
				zap.String("namespace", reqCtx.route.Namespace.Name),
			)
			if errorHandler, ok := modExt.ErrorHandlerFunc(); ok {
				// extract error handler function from module
				errorHandlerStart := time.Now()
				err = errorHandler(modExt.ModuleContext(), err)
				ps.metrics.MeasureModuleDuration(
					reqCtx.ctx, reqCtx,
					"error_handler",
					errorHandlerStart, err,
				)
				if err != nil {
					ps.logger.Error("Error handling error",
						zap.String("error", err.Error()),
						zap.String("route", reqCtx.route.Name),
						zap.String("namespace", reqCtx.route.Namespace.Name),
					)
					util.WriteStatusCodeError(reqCtx.rw, http.StatusInternalServerError)
					return
				}
			} else {
				util.WriteStatusCodeError(reqCtx.rw, http.StatusInternalServerError)
				return
			}
		} else {
			if !reqCtx.rw.HeadersSent() {
				if reqCtx.rw.BytesWritten() > 0 {
					reqCtx.rw.WriteHeader(http.StatusOK)
				} else {
					reqCtx.rw.WriteHeader(http.StatusNoContent)
				}
			}
		}
	} else {
		util.WriteStatusCodeError(reqCtx.rw, http.StatusNotImplemented)
		return
	}
}
