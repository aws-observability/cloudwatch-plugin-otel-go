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
// sampling rate.
//
// The minimum supported OpenTelemetry Go SDK has no first-party record-forcing sampler,
// so this small wrapper is provided here rather than as an external dependency. SDK
// v1.40.0+ ships sdktrace.AlwaysRecord; users on those versions may use it instead.
//
// Divergence from sdktrace.AlwaysRecord worth noting for users on newer SDKs: when the
// delegate returns Drop, this wrapper preserves the delegate's returned Attributes and
// Tracestate and only changes the decision to RecordOnly. sdktrace.AlwaysRecord instead
// drops those Attributes and resets Tracestate to the parent's. Preserving them keeps any
// delegate-provided sampling state (e.g. probability/threshold Tracestate) on the recorded
// span; users who prefer the upstream behavior can switch to sdktrace.AlwaysRecord on
// v1.40.0+.
type alwaysRecordSampler struct {
	delegate sdktrace.Sampler
}

// NewAlwaysRecordSampler returns a Sampler that wraps delegate and converts its Drop
// decisions to RecordOnly. Pass the result to sdktrace.WithSampler so the metrics
// processor observes 100% of spans:
//
//	sdktrace.WithSampler(spanmetrics.NewAlwaysRecordSampler(mySampler))
func NewAlwaysRecordSampler(delegate sdktrace.Sampler) sdktrace.Sampler {
	// Fail fast on a nil delegate rather than silently substituting a default
	// sampler. This matches the JS/.NET/Python plugins, which reject nil.
	if delegate == nil {
		panic("spanmetrics: NewAlwaysRecordSampler requires a non-nil delegate sampler")
	}
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
