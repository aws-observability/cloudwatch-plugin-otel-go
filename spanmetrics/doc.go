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
