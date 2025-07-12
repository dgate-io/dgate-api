package proxy_test

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/dgate-io/dgate-api/internal/config"
	"github.com/dgate-io/dgate-api/internal/config/configtest"
	"github.com/dgate-io/dgate-api/internal/proxy"
	"github.com/dgate-io/dgate-api/internal/proxy/proxytest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// TODO: clean up the tests - make then simpler, more readable.

func TestProxyHandler_ReverseProxy(t *testing.T) {
	os.Setenv("LOG_NO_COLOR", "true")
	configs := []*config.DGateConfig{
		configtest.NewTestDGateConfig(),
		// configtest.NewTest2DGateConfig(),
	}
	for _, conf := range configs {
		ps := proxy.NewProxyState(zap.NewNop(), conf)

		rt, ok := ps.ResourceManager().GetRoute("test", "test")
		if !ok {
			t.Fatal("namespace not found")
		}
		rpBuilder := proxytest.CreateMockReverseProxyBuilder()
		// rpBuilder.On("FlushInterval", mock.Anything).Return(rpBuilder).Once()
		rpBuilder.On("ModifyResponse", mock.Anything).Return(rpBuilder).Once()
		rpBuilder.On("ErrorHandler", mock.Anything).Return(rpBuilder).Once()
		rpBuilder.On("Clone").Return(rpBuilder).Times(2)
		rpBuilder.On("Transport", mock.Anything).Return(rpBuilder).Once()
		rpBuilder.On("ProxyRewrite",
			rt.StripPath,
			rt.PreserveHost,
			rt.Service.DisableQueryParams,
			conf.ProxyConfig.DisableXForwardedHeaders,
		).Return(rpBuilder).Once()
		rpe := proxytest.CreateMockReverseProxyExecutor()
		rpe.On("ServeHTTP", mock.Anything, mock.Anything).Return().Once()
		rpBuilder.On("Build", mock.Anything, mock.Anything).Return(rpe, nil).Once()
		ps.ReverseProxyBuilder = rpBuilder

		reqCtxProvider := proxy.NewRequestContextProvider(rt, ps)

		req, wr := proxytest.NewMockRequestAndResponseWriter("GET", "http://localhost:8080/test", []byte{})
		// wr.On("WriteHeader", 200).Return().Once()
		wr.SetWriteFallThrough()
		wr.On("Header").Return(http.Header{})
		wr.On("Write", mock.Anything).Return(0, nil).Maybe()
		reqCtx := reqCtxProvider.CreateRequestContext(wr, req, "/")

		modExt := NewMockModuleExtractor()
		modExt.ConfigureDefaultMock(req, wr, ps, rt)
		modBuf := NewMockModulePool()
		modBuf.On("Borrow").Return(modExt).Once()
		modBuf.On("Return", modExt).Return().Once()
		modBuf.On("Close").Return().Once()
		reqCtxProvider.UpdateModulePool(modBuf)

		modPool := NewMockModulePool()
		modPool.On("Borrow").Return(modExt).Once()
		modPool.On("Return", modExt).Return().Once()
		reqCtxProvider.UpdateModulePool(modPool)
		ps.ProxyHandler(ps, reqCtx)

		wr.AssertExpectations(t)
		modPool.AssertExpectations(t)
		modExt.AssertExpectations(t)
		rpBuilder.AssertExpectations(t)
		rpe.AssertExpectations(t)
	}
}

func TestProxyHandler_ProxyHandler(t *testing.T) {
	os.Setenv("LOG_NO_COLOR", "true")
	configs := []*config.DGateConfig{
		configtest.NewTestDGateConfig(),
		// configtest.NewTest2DGateConfig(),
	}
	for _, conf := range configs {
		ps := proxy.NewProxyState(zap.NewNop(), conf)
		ptBuilder := proxytest.CreateMockProxyTransportBuilder()
		ptBuilder.On("Retries", mock.Anything).Return(ptBuilder).Once()
		ptBuilder.On("Transport", mock.Anything).Return(ptBuilder).Once()
		ptBuilder.On("RequestTimeout", mock.Anything).Return(ptBuilder).Once()
		ptBuilder.On("RetryTimeout", mock.Anything).Return(ptBuilder).Maybe()
		ptBuilder.On("Clone").Return(ptBuilder).Once()
		tp := proxytest.CreateMockTransport()
		resp := &http.Response{
			StatusCode:    200,
			Header:        http.Header{},
			Body:          io.NopCloser(strings.NewReader("abc")),
			ContentLength: 3,
		}
		tp.On("RoundTrip", mock.Anything).Return(resp, nil)
		ptBuilder.On("Build").Return(tp, nil).Once()
		defer ptBuilder.AssertExpectations(t)
		ps.ProxyTransportBuilder = ptBuilder

		rm := ps.ResourceManager()
		rt, ok := rm.GetRoute("test", "test")
		if !ok {
			t.Fatal("namespace not found")
		}
		req, wr := proxytest.NewMockRequestAndResponseWriter("GET", "http://localhost:8080/test", []byte("123"))
		wr.On("WriteHeader", resp.StatusCode).Return().Maybe()
		wr.On("Header").Return(http.Header{}).Maybe()
		wr.On("Write", mock.Anything).Return(3, nil).Once().Run(func(args mock.Arguments) {
			b := args.Get(0).([]byte)
			assert.Equal(t, b, []byte("abc"))
		})

		reqCtxProvider := proxy.NewRequestContextProvider(rt, ps)
		modExt := NewMockModuleExtractor()
		modExt.ConfigureDefaultMock(req, wr, ps, rt)
		modPool := NewMockModulePool()
		modPool.On("Borrow").Return(modExt).Once()
		modPool.On("Return", modExt).Return().Once()
		reqCtxProvider.UpdateModulePool(modPool)

		reqCtx := reqCtxProvider.CreateRequestContext(wr, req, "/")
		ps.ProxyHandler(ps, reqCtx)

		wr.AssertExpectations(t)
		modPool.AssertExpectations(t)
		modExt.AssertExpectations(t)
	}
}

func TestProxyHandler_ProxyHandlerError(t *testing.T) {
	os.Setenv("LOG_NO_COLOR", "true")
	configs := []*config.DGateConfig{
		configtest.NewTestDGateConfig(),
		// configtest.NewTest2DGateConfig(),
	}
	for _, conf := range configs {
		ps := proxy.NewProxyState(zap.NewNop(), conf)
		ptBuilder := proxytest.CreateMockProxyTransportBuilder()
		ptBuilder.On("Retries", mock.Anything).Return(ptBuilder).Maybe()
		ptBuilder.On("Transport", mock.Anything).Return(ptBuilder).Maybe()
		ptBuilder.On("RequestTimeout", mock.Anything).Return(ptBuilder).Maybe()
		ptBuilder.On("RetryTimeout", mock.Anything).Return(ptBuilder).Maybe()
		ptBuilder.On("Clone").Return(ptBuilder).Maybe()
		tp := proxytest.CreateMockTransport()
		tp.On("RoundTrip", mock.Anything).Return(
			nil, errors.New("testing error"),
		)
		ptBuilder.On("Build").Return(tp, nil).Maybe()
		defer ptBuilder.AssertExpectations(t)
		ps.ProxyTransportBuilder = ptBuilder

		rm := ps.ResourceManager()
		rt, ok := rm.GetRoute("test", "test")
		if !ok {
			t.Fatal("namespace not found")
		}

		req, wr := proxytest.NewMockRequestAndResponseWriter("GET", "http://localhost:8080/test", []byte("123"))

		wr.On("WriteHeader", 502).Return().Maybe()
		wr.On("WriteHeader", 500).Return().Maybe()
		wr.On("Header").Return(http.Header{}).Maybe()
		wr.On("Write", mock.Anything).Return(0, nil).Maybe()

		modExt := NewMockModuleExtractor()
		modExt.ConfigureDefaultMock(req, wr, ps, rt)
		modPool := NewMockModulePool()
		modPool.On("Borrow").Return(modExt).Once()
		modPool.On("Return", modExt).Return().Once()
		reqCtxProvider := proxy.NewRequestContextProvider(rt, ps)
		reqCtxProvider.UpdateModulePool(modPool)
		reqCtx := reqCtxProvider.CreateRequestContext(wr, req, "/")
		ps.ProxyHandler(ps, reqCtx)

		wr.AssertExpectations(t)
		modPool.AssertExpectations(t)
		modExt.AssertExpectations(t)
	}
}
