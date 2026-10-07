// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package spanmetrics generates request ("RED") metrics from every recorded span,
// in-process and before trace sampling is applied. It provides two pieces that wire
// into an OpenTelemetry Go TracerProvider:
//
//   - NewAlwaysRecordSampler wraps the configured sampler so dropped spans are still
//     recorded (and reach the processor) without being exported.
//   - NewSpanMetricsProcessor is a trace.SpanProcessor that records the two metrics in
//     OnEnd using a caller-supplied MeterProvider.
//
// Both metrics use the OpenTelemetry SpanMetrics naming and are emitted under the
// instrumentation scope "cloudwatch.plugin.otel.span_metrics". They ride the supplied
// MeterProvider, so they flow wherever the application's other metrics go.
package spanmetrics
