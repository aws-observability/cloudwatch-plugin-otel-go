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

package spanmetrics

import (
	"context"
	"reflect"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Meter scope for the generated metrics. The "aws" token is intentionally absent from the
// scope name, as CloudWatch reserves "aws" in instrumentation scope names.
const scopeName = "cloudwatch.plugin.otel.span_metrics"

const (
	callsMetric    = "traces.span.metrics.calls"
	durationMetric = "traces.span.metrics.duration"

	callsUnit    = "{call}" // UCUM annotation for counted things (OTel semconv counter convention).
	durationUnit = "s"      // seconds, per OTel semconv duration metrics.

	nanosPerSecond = 1_000_000_000.0
)

// durationBuckets are the SpanMetricsConnector default boundaries expressed in seconds.
var durationBuckets = []float64{
	0.002, 0.004, 0.006, 0.008, 0.01, 0.05, 0.1, 0.2, 0.4, 0.8, 1, 1.4, 2, 5, 10, 15,
}

// noopCounterType is the concrete type the OpenTelemetry no-op MeterProvider yields for an
// Int64Counter. A provider that hands back this type has no real metrics pipeline behind it
// (the global default before SetMeterProvider, or an explicit noop.NewMeterProvider), so the
// processor must stay inert rather than record into nothing and — worse — stamp the dedup
// marker. Mirrors the Java plugin, which detects the no-op meter the same way.
//
// Note: a reader-less SDK MeterProvider (sdkmetric.NewMeterProvider() with no reader) does
// NOT yield this type; it returns a real SDK instrument that silently drops measurements. A
// reader-less SDK provider is therefore treated as active by design — it is a real provider
// the caller deliberately built, and matching Java we only special-case the no-op meter.
var noopCounterType = noopReferenceCounterType()

// noopReferenceCounterType returns the concrete reflect.Type of an Int64Counter built from a
// no-op MeterProvider, used as the comparison reference in buildInstruments.
func noopReferenceCounterType() reflect.Type {
	c, _ := noop.NewMeterProvider().Meter(scopeName).Int64Counter(callsMetric)
	return reflect.TypeOf(c)
}

// spanMetricsProcessor is a trace.SpanProcessor that records the two span metrics from
// every recorded span. Paired with the AlwaysRecordSampler it sees 100% of spans while
// export still honors the sampling rate. It also stamps identity/dedup markers on each
// span at start; these markers tell CloudWatch ingestion not to regenerate the same
// metrics from a span this plugin already metered. They have no effect on any other
// span-to-metrics generator (e.g. the Collector spanmetrics connector), which will still
// produce its own metrics unless separately configured to skip these spans.
type spanMetricsProcessor struct {
	calls    metric.Int64Counter
	duration metric.Float64Histogram
	// active is false when no usable MeterProvider was supplied; the processor then stays
	// inert and, crucially, does not stamp the dedup marker — so a downstream generator can
	// still produce the metric and the exactly-once invariant holds.
	active bool
}

// NewSpanMetricsProcessor returns a trace.SpanProcessor that records span metrics into mp.
// Register it on the TracerProvider alongside the AlwaysRecordSampler:
//
//	tp := sdktrace.NewTracerProvider(
//	    sdktrace.WithResource(res),
//	    sdktrace.WithSampler(spanmetrics.NewAlwaysRecordSampler(userSampler)),
//	    sdktrace.WithSpanProcessor(spanmetrics.NewSpanMetricsProcessor(mp)),
//	    sdktrace.WithBatcher(traceExporter),
//	)
//
// mp is normally the same MeterProvider the application uses for its other metrics, so the
// span metrics share the application's resource (and its service.name) and flow to the same
// destination.
//
// The processor is inert — it records nothing and stamps no markers — whenever there is no
// real metrics pipeline behind mp: mp is nil, mp is a typed-nil provider that would panic, or
// mp is a no-op MeterProvider (see noopCounterType). Staying inert in these cases preserves the exactly-once invariant: a span this processor cannot
// actually meter is never marked as already-metered, so a downstream generator can still
// produce the metric.
func NewSpanMetricsProcessor(mp metric.MeterProvider) sdktrace.SpanProcessor {
	if mp == nil {
		return &spanMetricsProcessor{active: false}
	}
	// buildInstruments builds the metric pair from mp. It is wrapped in a recover so a
	// typed-nil provider (e.g. (*noop.MeterProvider)(nil)) whose Meter method dereferences a
	// nil receiver can never panic the caller — on any failure we fall through to inert.
	calls, duration, ok := buildInstruments(mp)
	if !ok {
		return &spanMetricsProcessor{active: false}
	}
	return &spanMetricsProcessor{calls: calls, duration: duration, active: true}
}

// buildInstruments constructs the calls/duration instruments from mp and reports whether the
// provider is backed by a real metrics pipeline. It returns ok=false when construction fails,
// when the provider panics (typed-nil), or when the provider yields the no-op counter type
// (no pipeline). Detection is constructor-time only; no late binding.
func buildInstruments(mp metric.MeterProvider) (calls metric.Int64Counter, duration metric.Float64Histogram, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			// A provider that panics on use (typed-nil, etc.) is not usable — stay inert.
			calls, duration, ok = nil, nil, false
		}
	}()

	meter := mp.Meter(scopeName)
	// Instrument construction only fails on an invalid configuration (e.g. a bad name); the
	// names/units here are constants, so errors are not expected. If one occurs, stay inert
	// rather than record into a partially-built pair.
	c, err := meter.Int64Counter(callsMetric, metric.WithUnit(callsUnit))
	if err != nil {
		return nil, nil, false
	}
	// A no-op meter hands back the noop instrument type, which means there is no real pipeline
	// behind this provider. Recording would go nowhere, so treat the provider as inert and, in
	// particular, do not stamp the dedup marker.
	if reflect.TypeOf(c) == noopCounterType {
		return nil, nil, false
	}
	h, err := meter.Float64Histogram(
		durationMetric,
		metric.WithUnit(durationUnit),
		metric.WithExplicitBucketBoundaries(durationBuckets...),
	)
	if err != nil {
		return nil, nil, false
	}
	return c, h, true
}

// OnStart stamps the schema + library-version markers. The presence of the schema
// marker on a span is the dedup signal. It is written only when the processor is active, so a
// span the plugin cannot meter is never marked as already-metered.
func (p *spanMetricsProcessor) OnStart(_ context.Context, span sdktrace.ReadWriteSpan) {
	if !p.active {
		return
	}
	span.SetAttributes(
		attribute.String(schemaAttr, schemaVersion),
		attribute.String(libVersionAttr, LibVersion),
	)
}

// OnEnd records +1 to the calls counter and the span duration (seconds) to the histogram,
// both carrying the attribute set from buildAttributes.
func (p *spanMetricsProcessor) OnEnd(span sdktrace.ReadOnlySpan) {
	if !p.active {
		return
	}
	attrs := buildAttributes(span)
	set := metric.WithAttributeSet(attrs)
	// Metrics are recorded outside any request context; a span-processor OnEnd has no
	// obligation to propagate one.
	ctx := context.Background()
	p.calls.Add(ctx, 1, set)
	seconds := float64(span.EndTime().Sub(span.StartTime()).Nanoseconds()) / nanosPerSecond
	p.duration.Record(ctx, seconds, set)
}

// Shutdown and ForceFlush are no-ops: the metric lifecycle belongs to the MeterProvider the
// caller owns, not to this processor.
func (p *spanMetricsProcessor) Shutdown(context.Context) error   { return nil }
func (p *spanMetricsProcessor) ForceFlush(context.Context) error { return nil }
