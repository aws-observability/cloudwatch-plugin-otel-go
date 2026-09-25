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

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
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

// durationBuckets are the SpanMetricsConnector default boundaries expressed in seconds (spec §1).
var durationBuckets = []float64{
	0.002, 0.004, 0.006, 0.008, 0.01, 0.05, 0.1, 0.2, 0.4, 0.8, 1, 1.4, 2, 5, 10, 15,
}

// spanMetricsProcessor is a trace.SpanProcessor that records the two span metrics from
// every recorded span. Paired with the AlwaysRecordSampler it sees 100% of spans while
// export still honors the sampling rate. It also stamps the identity/dedup markers
// (spec §6) on each span at start so a downstream generator skips spans it already metered.
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
// destination. If mp is nil the processor is inert: it records nothing and stamps no markers.
func NewSpanMetricsProcessor(mp metric.MeterProvider) sdktrace.SpanProcessor {
	if mp == nil {
		return &spanMetricsProcessor{active: false}
	}
	meter := mp.Meter(scopeName)
	// Instrument construction only fails on an invalid configuration (e.g. a bad name); the
	// names/units here are constants, so errors are not expected. If one occurs, stay inert
	// rather than record into a partially-built pair.
	calls, err := meter.Int64Counter(callsMetric, metric.WithUnit(callsUnit))
	if err != nil {
		return &spanMetricsProcessor{active: false}
	}
	duration, err := meter.Float64Histogram(
		durationMetric,
		metric.WithUnit(durationUnit),
		metric.WithExplicitBucketBoundaries(durationBuckets...),
	)
	if err != nil {
		return &spanMetricsProcessor{active: false}
	}
	return &spanMetricsProcessor{calls: calls, duration: duration, active: true}
}

// OnStart stamps the schema + library-version markers (spec §6). The presence of the schema
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
