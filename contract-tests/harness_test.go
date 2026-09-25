// Copyright Amazon.com, Inc. or its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License").
// You may not use this file except in compliance with the License.
// A copy of the License is located at
//
//  http://aws.amazon.com/apache2.0
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package contracttests

import (
	"context"
	"testing"

	"github.com/aws-observability/cloudwatch-plugin-otel-go/spanmetrics"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	callsMetric    = "traces.span.metrics.calls"
	durationMetric = "traces.span.metrics.duration"
	scopeName      = "cloudwatch.plugin.otel.span_metrics"
	testService    = "contract-test-service"
)

// pipeline holds a fully wired SDK: the plugin's sampler + processor on a TracerProvider
// whose resource carries service.name, recording into an in-memory metric reader.
type pipeline struct {
	tp     *sdktrace.TracerProvider
	reader *sdkmetric.ManualReader
}

func newPipeline(t *testing.T) *pipeline {
	t.Helper()
	res, err := resource.Merge(resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName(testService)))
	if err != nil {
		t.Fatalf("resource: %v", err)
	}
	// The generated metrics inherit the MeterProvider's resource (spec §5: client resource =
	// the app's SDK resource, which carries service.name). In an app both providers share the
	// same resource; the resource must be set on the MeterProvider for it to reach the metrics.
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
	)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(spanmetrics.NewAlwaysRecordSampler(sdktrace.AlwaysSample())),
		sdktrace.WithSpanProcessor(spanmetrics.NewSpanMetricsProcessor(mp)),
	)
	return &pipeline{tp: tp, reader: reader}
}

func (p *pipeline) collect(t *testing.T) map[string]metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := p.reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	out := make(map[string]metricdata.Metrics)
	for _, sm := range rm.ScopeMetrics {
		if sm.Scope.Name != scopeName {
			t.Errorf("unexpected scope %q", sm.Scope.Name)
		}
		for _, m := range sm.Metrics {
			out[m.Name] = m
		}
	}
	// service.name must live on the metric RESOURCE, not on datapoints (spec §3/§5).
	assertResourceServiceName(t, rm)
	return out
}

func assertResourceServiceName(t *testing.T, rm metricdata.ResourceMetrics) {
	t.Helper()
	for _, kv := range rm.Resource.Attributes() {
		if kv.Key == "service.name" {
			if kv.Value.AsString() != testService {
				t.Errorf("resource service.name = %q, want %q", kv.Value.AsString(), testService)
			}
			return
		}
	}
	t.Error("service.name not found on metric resource")
}

// callsDataPoint returns the single calls datapoint attribute map and count.
func callsDataPoint(t *testing.T, metrics map[string]metricdata.Metrics) (map[attribute.Key]attribute.KeyValue, int64) {
	t.Helper()
	m, ok := metrics[callsMetric]
	if !ok {
		t.Fatal("calls metric missing")
	}
	sum, ok := m.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("calls type = %T", m.Data)
	}
	if len(sum.DataPoints) != 1 {
		t.Fatalf("expected 1 calls datapoint, got %d", len(sum.DataPoints))
	}
	dp := sum.DataPoints[0]
	return setToMap(dp.Attributes), dp.Value
}

// totalCalls sums the calls counter across all datapoints (each unique attribute set is its
// own datapoint).
func totalCalls(t *testing.T, metrics map[string]metricdata.Metrics) int64 {
	t.Helper()
	m, ok := metrics[callsMetric]
	if !ok {
		t.Fatal("calls metric missing")
	}
	sum, ok := m.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("calls type = %T", m.Data)
	}
	var total int64
	for _, dp := range sum.DataPoints {
		total += dp.Value
	}
	return total
}

func setToMap(set attribute.Set) map[attribute.Key]attribute.KeyValue {
	out := make(map[attribute.Key]attribute.KeyValue)
	for it := set.Iter(); it.Next(); {
		kv := it.Attribute()
		out[kv.Key] = kv
	}
	return out
}

func str(t *testing.T, m map[attribute.Key]attribute.KeyValue, key string) string {
	t.Helper()
	kv, ok := m[attribute.Key(key)]
	if !ok {
		t.Fatalf("attribute %q missing", key)
	}
	return kv.Value.AsString()
}

func hasKey(m map[attribute.Key]attribute.KeyValue, key string) bool {
	_, ok := m[attribute.Key(key)]
	return ok
}

// startEndServerSpan is a small helper to emit one finished span of the given kind.
func (p *pipeline) emit(ctx context.Context, tracer trace.Tracer, name string, kind trace.SpanKind, attrs ...attribute.KeyValue) {
	_, span := tracer.Start(ctx, name, trace.WithSpanKind(kind))
	span.SetAttributes(attrs...)
	span.End()
}

// withGlobalProvider installs p.tp as the global TracerProvider for the duration of fn,
// then restores the previous one. This models mode 2, where library instrumentation
// (otelhttp/otelgrpc) obtains its tracer from the global provider.
func withGlobalProvider(p *pipeline, fn func()) {
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(p.tp)
	defer otel.SetTracerProvider(prev)
	fn()
}
