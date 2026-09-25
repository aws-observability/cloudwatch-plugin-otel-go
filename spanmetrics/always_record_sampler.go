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
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// alwaysRecordSampler wraps a delegate sampler and turns Drop decisions into RecordOnly.
// Spans the delegate would drop are still recorded, so SpanMetricsProcessor.OnEnd sees
// them, but they are not sampled/exported, so trace volume still honors the configured
// sampling rate. The delegate's attributes and trace-state are preserved.
//
// OpenTelemetry Go has no first-party record-forcing sampler, so this small wrapper is
// provided here rather than as an external dependency.
type alwaysRecordSampler struct {
	delegate sdktrace.Sampler
}

// NewAlwaysRecordSampler returns a Sampler that wraps delegate and converts its Drop
// decisions to RecordOnly. Pass the result to sdktrace.WithSampler so the metrics
// processor observes 100% of spans:
//
//	sdktrace.WithSampler(spanmetrics.NewAlwaysRecordSampler(mySampler))
func NewAlwaysRecordSampler(delegate sdktrace.Sampler) sdktrace.Sampler {
	return alwaysRecordSampler{delegate: delegate}
}

func (s alwaysRecordSampler) ShouldSample(p sdktrace.SamplingParameters) sdktrace.SamplingResult {
	result := s.delegate.ShouldSample(p)
	if result.Decision == sdktrace.Drop {
		// Keep the delegate's Attributes and Tracestate; only the decision changes.
		result.Decision = sdktrace.RecordOnly
	}
	return result
}

func (s alwaysRecordSampler) Description() string {
	return "AlwaysRecordSampler{" + s.delegate.Description() + "}"
}
