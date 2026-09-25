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

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"
)

// Mode 1: the app builds its own TracerProvider and creates spans from a tracer obtained
// directly from that provider.
func TestMode1_ManualSDK(t *testing.T) {
	p := newPipeline(t)
	tracer := p.tp.Tracer("app")
	p.emit(context.Background(), tracer, "op", trace.SpanKindServer)

	m, count := callsDataPoint(t, p.collect(t))
	if count != 1 {
		t.Errorf("calls = %d, want 1", count)
	}
	if got := str(t, m, "span.kind"); got != "SERVER" {
		t.Errorf("span.kind = %q", got)
	}
}

// Mode 2: a component obtains its tracer from the GLOBAL provider (as otelhttp/otelgrpc do).
// The same single processor on that provider must observe the span with no extra wiring.
func TestMode2_GlobalProviderLibrarySpans(t *testing.T) {
	p := newPipeline(t)
	withGlobalProvider(p, func() {
		// Simulate a library that does otel.Tracer(name).Start(...) internally.
		tracer := otel.Tracer("go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp")
		p.emit(context.Background(), tracer, "GET /ping", trace.SpanKindServer)
	})

	m, count := callsDataPoint(t, p.collect(t))
	if count != 1 {
		t.Errorf("calls = %d, want 1 (library span must reach the processor)", count)
	}
	if got := str(t, m, "span.name"); got != "GET /ping" {
		t.Errorf("span.name = %q", got)
	}
}

// All span kinds are metered (spec §2).
func TestAllSpanKindsMetered(t *testing.T) {
	p := newPipeline(t)
	tracer := p.tp.Tracer("app")
	kinds := []trace.SpanKind{
		trace.SpanKindServer, trace.SpanKindClient, trace.SpanKindInternal,
		trace.SpanKindProducer, trace.SpanKindConsumer,
	}
	for _, k := range kinds {
		p.emit(context.Background(), tracer, "op", k)
	}
	if count := totalCalls(t, p.collect(t)); count != int64(len(kinds)) {
		t.Errorf("calls = %d, want %d (one per span kind)", count, len(kinds))
	}
}

// Duration is emitted in seconds with the connector bucket boundaries (spec §1).
func TestDurationEmittedInSeconds(t *testing.T) {
	p := newPipeline(t)
	p.emit(context.Background(), p.tp.Tracer("app"), "op", trace.SpanKindServer)

	d, ok := p.collect(t)[durationMetric]
	if !ok {
		t.Fatal("duration metric missing")
	}
	if d.Unit != "s" {
		t.Errorf("duration unit = %q, want s", d.Unit)
	}
	hist, ok := d.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("duration type = %T", d.Data)
	}
	wantBounds := []float64{0.002, 0.004, 0.006, 0.008, 0.01, 0.05, 0.1, 0.2, 0.4, 0.8, 1, 1.4, 2, 5, 10, 15}
	if got := hist.DataPoints[0].Bounds; len(got) != len(wantBounds) {
		t.Errorf("bounds = %v, want %v", got, wantBounds)
	}
}

// The dedup + schema markers appear on the metric datapoints (spec §6).
func TestIdentityMarkersOnMetric(t *testing.T) {
	p := newPipeline(t)
	p.emit(context.Background(), p.tp.Tracer("app"), "op", trace.SpanKindServer)
	m, _ := callsDataPoint(t, p.collect(t))
	if got := str(t, m, "aws.otel.span.metrics.schema"); got != "v1" {
		t.Errorf("schema marker = %q, want v1", got)
	}
	if !hasKey(m, "aws.otel.extension.lib.version") {
		t.Error("lib.version marker missing on metric")
	}
}
