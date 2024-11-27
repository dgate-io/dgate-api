package proxy

import (
	"context"
	"time"

	"github.com/dgate-io/dgate-api/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	api "go.opentelemetry.io/otel/metric"
)

type ProxyMetrics struct {
	resolveNamespaceDurInstrument api.Float64Histogram
	resolveCertDurInstrument      api.Float64Histogram
	proxyDurInstrument            api.Float64Histogram
	proxyCountInstrument          api.Int64Counter
	moduleDurInstrument           api.Float64Histogram
	moduleRunCountInstrument      api.Int64Counter
	upstreamDurInstrument         api.Float64Histogram
	errorCountInstrument          api.Int64Counter
}

func NewProxyMetrics() *ProxyMetrics {
	return &ProxyMetrics{}
}

func (pm *ProxyMetrics) Setup(config *config.DGateConfig) {
	if config.DisableMetrics {
		return
	}
	meter := otel.Meter("dgate-proxy-metrics", api.WithInstrumentationAttributes(
		attribute.KeyValue{
			Key: "storage", Value: attribute.StringValue(string(config.Storage.StorageType)),
		},
		attribute.KeyValue{
			Key: "node_id", Value: attribute.StringValue(config.NodeId),
		},
		attribute.KeyValue{
			Key: "tag", Value: attribute.StringSliceValue(config.Tags),
		},
	))

	pm.resolveNamespaceDurInstrument, _ = meter.Float64Histogram(
		"resolve_namespace_duration", api.WithUnit("us"))
	pm.resolveCertDurInstrument, _ = meter.Float64Histogram(
		"resolve_cert_duration", api.WithUnit("ms"))
	pm.proxyDurInstrument, _ = meter.Float64Histogram(
		"request_duration", api.WithUnit("ms"))
	pm.moduleDurInstrument, _ = meter.Float64Histogram(
		"module_duration", api.WithUnit("ms"))
	pm.upstreamDurInstrument, _ = meter.Float64Histogram(
		"upstream_duration", api.WithUnit("ms"))
	pm.proxyCountInstrument, _ = meter.Int64Counter(
		"request_count")
	pm.moduleRunCountInstrument, _ = meter.Int64Counter(
		"module_executions")
	pm.errorCountInstrument, _ = meter.Int64Counter(
		"error_count")
}

func (pm *ProxyMetrics) MeasureProxyRequest(
	ctx context.Context,
	reqCtx *RequestContext,
	start time.Time,
) {
	if pm.proxyDurInstrument == nil || pm.proxyCountInstrument == nil {
		return
	}
	serviceAttr := attribute.NewSet()
	if reqCtx.route.Service != nil {
		serviceAttr = attribute.NewSet(
			attribute.String("service", reqCtx.route.Service.Name),
		)
	}

	elasped := time.Since(start)
	userAgent := reqCtx.req.UserAgent()
	if maxUaLen := 256; len(userAgent) > maxUaLen {
		userAgent = userAgent[:maxUaLen]
	}
	attrSet := attribute.NewSet(
		attribute.String("route", reqCtx.route.Name),
		attribute.String("namespace", reqCtx.route.Namespace.Name),
		attribute.String("method", reqCtx.req.Method),
		attribute.String("path", reqCtx.req.URL.Path),
		attribute.String("pattern", reqCtx.pattern),
		attribute.String("host", reqCtx.req.Host),
		attribute.String("remote_addr", reqCtx.req.RemoteAddr),
		attribute.String("user_agent", userAgent),
		attribute.String("proto", reqCtx.req.Proto),
		attribute.Int64("content_length", reqCtx.req.ContentLength),
		attribute.Int("status_code", reqCtx.rw.Status()),
	)

	pm.proxyDurInstrument.Record(reqCtx.ctx,
		float64(elasped)/float64(time.Millisecond),
		api.WithAttributeSet(attrSet), api.WithAttributeSet(serviceAttr))

	pm.proxyCountInstrument.Add(reqCtx.ctx, 1,
		api.WithAttributeSet(attrSet), api.WithAttributeSet(serviceAttr))
}

func (pm *ProxyMetrics) MeasureModuleDuration(
	ctx context.Context, reqCtx *RequestContext,
	moduleFunc string, start time.Time, err error,
) {
	if pm.moduleDurInstrument == nil || pm.moduleRunCountInstrument == nil {
		return
	}
	elasped := time.Since(start)
	attrSet := attribute.NewSet(
		attribute.Bool("error", err != nil),
		attribute.String("route", reqCtx.route.Name),
		attribute.String("namespace", reqCtx.route.Namespace.Name),
		attribute.String("moduleFunc", moduleFunc),
		attribute.String("method", reqCtx.req.Method),
		attribute.String("path", reqCtx.req.URL.Path),
		attribute.String("pattern", reqCtx.pattern),
		attribute.String("host", reqCtx.req.Host),
	)
	pm.addError(ctx, moduleFunc, err, attrSet)

	pm.moduleDurInstrument.Record(reqCtx.ctx,
		float64(elasped)/float64(time.Millisecond),
		api.WithAttributeSet(attrSet))

	pm.moduleRunCountInstrument.Add(reqCtx.ctx, 1,
		api.WithAttributeSet(attrSet))
}

func (pm *ProxyMetrics) MeasureUpstreamDuration(
	ctx context.Context, reqCtx *RequestContext,
	start time.Time, upstreamHost string, err error,
) {
	if pm.upstreamDurInstrument == nil {
		return
	}
	elasped := time.Since(start)
	attrSet := attribute.NewSet(
		attribute.Bool("error", err != nil),
		attribute.String("route", reqCtx.route.Name),
		attribute.String("namespace", reqCtx.route.Namespace.Name),
		attribute.String("method", reqCtx.req.Method),
		attribute.String("path", reqCtx.req.URL.Path),
		attribute.String("pattern", reqCtx.pattern),
		attribute.String("host", reqCtx.req.Host),
		attribute.String("service", reqCtx.route.Service.Name),
		attribute.String("upstream_host", upstreamHost),
	)
	pm.addError(ctx, "upstream_request", err, attrSet)

	pm.upstreamDurInstrument.Record(reqCtx.ctx,
		float64(elasped)/float64(time.Millisecond),
		api.WithAttributeSet(attrSet))
}

func (pm *ProxyMetrics) MeasureNamespaceResolutionDuration(
	ctx context.Context, start time.Time,
	host, namespace string, err error,
) {
	if pm.resolveNamespaceDurInstrument == nil {
		return
	}
	elasped := time.Since(start)
	attrSet := attribute.NewSet(
		attribute.String("host", host),
		attribute.String("namespace", namespace),
	)
	pm.addError(ctx, "namespace_resolution", err, attrSet)

	pm.resolveNamespaceDurInstrument.Record(context.TODO(),
		float64(elasped)/float64(time.Microsecond),
		api.WithAttributeSet(attrSet))
}

func (pm *ProxyMetrics) MeasureCertResolutionDuration(
	ctx context.Context, start time.Time,
	host string, cache bool, err error,
) {
	if pm.resolveCertDurInstrument == nil {
		return
	}

	elasped := time.Since(start)
	attrSet := attribute.NewSet(
		attribute.Bool("error", err != nil),
		attribute.String("host", host),
		attribute.Bool("cache", cache),
	)
	pm.addError(ctx, "cert_resolution", err, attrSet)

	pm.resolveCertDurInstrument.Record(context.TODO(),
		float64(elasped)/float64(time.Millisecond),
		api.WithAttributeSet(attrSet))
}

func (pm *ProxyMetrics) addError(
	ctx context.Context,
	namespace string, err error,
	attrs ...attribute.Set,
) {
	if pm.errorCountInstrument == nil || err == nil {
		return
	}
	attrSet := attribute.NewSet(
		attribute.String("error_value", err.Error()),
		attribute.String("namespace", namespace),
	)

	attrSets := []api.AddOption{
		api.WithAttributeSet(attrSet),
	}
	for _, attr := range attrs {
		attrSets = append(attrSets, api.WithAttributeSet(attr))
	}

	pm.errorCountInstrument.Add(ctx, 1, attrSets...)
}
