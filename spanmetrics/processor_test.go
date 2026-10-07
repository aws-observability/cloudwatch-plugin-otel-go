// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package spanmetrics

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// newTestPipeline wires the processor and an AlwaysRecordSampler into a TracerProvider that
// records metrics into a ManualReader, plus a span recorder to inspect stamped span attrs.
func newTestPipeline(t *testing.T) (trace.Tracer, *sdkmetric.ManualReader, *tracetest.SpanRecorder) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(NewAlwaysRecordSampler(sdktrace.AlwaysSample())),
		sdktrace.WithSpanProcessor(NewSpanMetricsProcessor(mp)),
		sdktrace.WithSpanProcessor(rec),
	)
	return tp.Tracer("test"), reader, rec
}

func collect(t *testing.T, reader *sdkmetric.ManualReader) map[string]metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	out := make(map[string]metricdata.Metrics)
	for _, sm := range rm.ScopeMetrics {
		if sm.Scope.Name != scopeName {
			t.Errorf("unexpected scope %q, want %q", sm.Scope.Name, scopeName)
		}
		for _, m := range sm.Metrics {
			out[m.Name] = m
		}
	}
	return out
}

func TestProcessorEmitsBothMetrics(t *testing.T) {
	tracer, reader, _ := newTestPipeline(t)
	_, span := tracer.Start(context.Background(), "op", trace.WithSpanKind(trace.SpanKindServer))
	span.End()

	metrics := collect(t, reader)
	if _, ok := metrics[callsMetric]; !ok {
		t.Fatalf("missing %s; got %v", callsMetric, keysOf(metrics))
	}
	if _, ok := metrics[durationMetric]; !ok {
		t.Fatalf("missing %s; got %v", durationMetric, keysOf(metrics))
	}
}

func TestCallsMetricShape(t *testing.T) {
	tracer, reader, _ := newTestPipeline(t)
	for i := 0; i < 3; i++ {
		_, span := tracer.Start(context.Background(), "op", trace.WithSpanKind(trace.SpanKindServer))
		span.End()
	}
	calls := collect(t, reader)[callsMetric]
	if calls.Unit != "{call}" {
		t.Errorf("calls unit = %q, want {call}", calls.Unit)
	}
	sum, ok := calls.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("calls data type = %T, want Sum[int64]", calls.Data)
	}
	if !sum.IsMonotonic {
		t.Error("calls Sum should be monotonic")
	}
	if len(sum.DataPoints) != 1 {
		t.Fatalf("expected 1 datapoint, got %d", len(sum.DataPoints))
	}
	if sum.DataPoints[0].Value != 3 {
		t.Errorf("calls value = %d, want 3", sum.DataPoints[0].Value)
	}
}

func TestDurationMetricShape(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	proc := NewSpanMetricsProcessor(mp)

	// Build a span with a known 250ms duration via the recorder, then feed it to OnEnd.
	span := recordSpan(t, "op", trace.SpanKindServer, false)
	proc.OnEnd(span)

	duration := collect(t, reader)[durationMetric]
	if duration.Unit != "s" {
		t.Errorf("duration unit = %q, want s", duration.Unit)
	}
	hist, ok := duration.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("duration data type = %T, want Histogram[float64]", duration.Data)
	}
	if len(hist.DataPoints) != 1 {
		t.Fatalf("expected 1 datapoint, got %d", len(hist.DataPoints))
	}
	dp := hist.DataPoints[0]
	// Recorded duration is 250ms = 0.25s.
	if dp.Sum < 0.24 || dp.Sum > 0.26 {
		t.Errorf("duration sum = %v, want ~0.25", dp.Sum)
	}
	wantBounds := []float64{0.002, 0.004, 0.006, 0.008, 0.01, 0.05, 0.1, 0.2, 0.4, 0.8, 1, 1.4, 2, 5, 10, 15}
	if len(dp.Bounds) != len(wantBounds) {
		t.Fatalf("bounds len = %d, want %d (%v)", len(dp.Bounds), len(wantBounds), dp.Bounds)
	}
	for i, b := range wantBounds {
		if dp.Bounds[i] != b {
			t.Errorf("bound[%d] = %v, want %v", i, dp.Bounds[i], b)
		}
	}
}

func TestProcessorStampsDedupMarkersOnSpan(t *testing.T) {
	tracer, _, rec := newTestPipeline(t)
	_, span := tracer.Start(context.Background(), "op")
	span.End()

	ended := rec.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 span, got %d", len(ended))
	}
	m := attrMap(attribute.NewSet(ended[0].Attributes()...))
	if got := mustString(t, m, "aws.otel.span.metrics.schema"); got != "v1" {
		t.Errorf("span schema marker = %q, want v1", got)
	}
	if got := mustString(t, m, "aws.otel.extension.lib.version"); got != LibVersion {
		t.Errorf("span lib.version marker = %q", got)
	}
}

func TestNilProviderIsInert(t *testing.T) {
	proc := NewSpanMetricsProcessor(nil)
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(proc),
		sdktrace.WithSpanProcessor(rec),
	)
	_, span := tp.Tracer("t").Start(context.Background(), "op")
	span.End() // must not panic

	// Inert processor must NOT stamp the dedup marker, so a downstream generator can still
	// produce the metric (exactly-once invariant).
	ended := rec.Ended()
	m := attrMap(attribute.NewSet(ended[0].Attributes()...))
	absent(t, m, "aws.otel.span.metrics.schema")
}

// endedSpanAttrs runs one span through a TracerProvider carrying proc plus a span recorder,
// and returns the attribute map of the single ended span. Used to assert whether the dedup
// marker was stamped.
func endedSpanAttrs(t *testing.T, proc sdktrace.SpanProcessor) map[attribute.Key]attribute.KeyValue {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(proc),
		sdktrace.WithSpanProcessor(rec),
	)
	_, span := tp.Tracer("t").Start(context.Background(), "op")
	span.End() // must not panic
	ended := rec.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 span, got %d", len(ended))
	}
	return attrMap(attribute.NewSet(ended[0].Attributes()...))
}

// TestNoopMeterProviderIsInert: a no-op MeterProvider has no real metrics pipeline, so the
// processor must stay inert. A noop provider has no reader to collect from, so we assert
// inertness via the span side: the ended span must NOT carry the dedup marker, otherwise a
// downstream generator would skip a span that was never actually metered (breaking
// exactly-once).
func TestNoopMeterProviderIsInert(t *testing.T) {
	proc := NewSpanMetricsProcessor(noop.NewMeterProvider())
	m := endedSpanAttrs(t, proc)
	absent(t, m, "aws.otel.span.metrics.schema")
}

// TestTypedNilProviderIsInert: a typed-nil MeterProvider whose Meter method would panic on a
// nil receiver must be handled gracefully — the constructor must not panic, and the resulting
// processor must be inert (no dedup marker stamped).
func TestTypedNilProviderIsInert(t *testing.T) {
	var mp *noop.MeterProvider // typed nil; mp.Meter panics if dereferenced
	proc := NewSpanMetricsProcessor(mp)
	m := endedSpanAttrs(t, proc)
	absent(t, m, "aws.otel.span.metrics.schema")
}

// NOTE on reader-less SDK providers: an empirical probe (see commit) showed that
// sdkmetric.NewMeterProvider() with NO reader returns a REAL SDK instrument
// (*metric.int64Inst) — the SAME concrete type as an SDK provider WITH a reader — not the
// no-op instrument type. The two are therefore indistinguishable by type, so a reader-less SDK
// provider is treated as ACTIVE by design: it is a real provider the caller deliberately built
// (its measurements are silently dropped by the SDK, which is the caller's configuration, not
// ours to second-guess). Only the no-op meter — which unambiguously signals "no pipeline" — is
// treated as inert. Matches the Java plugin. Hence there is no TestReaderlessProviderInert.

func TestDerivedAttributesReachMetric(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	proc := NewSpanMetricsProcessor(mp)

	span := recordSpan(t, "GET /ping", trace.SpanKindServer, false,
		attribute.String("http.request.method", "GET"),
		attribute.Int("http.response.status_code", 200),
	)
	proc.OnEnd(span)

	calls := collect(t, reader)[callsMetric].Data.(metricdata.Sum[int64])
	m := attrMap(calls.DataPoints[0].Attributes)
	if got := mustString(t, m, "http.request.method"); got != "GET" {
		t.Errorf("metric http.request.method = %q", got)
	}
	if got := mustInt(t, m, "http.response.status_code"); got != 200 {
		t.Errorf("metric http.response.status_code = %d", got)
	}
	if got := mustString(t, m, "span.kind"); got != "SERVER" {
		t.Errorf("metric span.kind = %q", got)
	}
}

func TestProcessorInterfaceMethods(t *testing.T) {
	proc := NewSpanMetricsProcessor(nil)
	if err := proc.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown err = %v", err)
	}
	if err := proc.ForceFlush(context.Background()); err != nil {
		t.Errorf("ForceFlush err = %v", err)
	}
}

func keysOf(m map[string]metricdata.Metrics) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
