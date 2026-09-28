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
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// fixedSampler is a stub delegate that returns a preset result.
type fixedSampler struct {
	result      sdktrace.SamplingResult
	description string
	called      bool
}

func (f *fixedSampler) ShouldSample(sdktrace.SamplingParameters) sdktrace.SamplingResult {
	f.called = true
	return f.result
}
func (f *fixedSampler) Description() string { return f.description }

func TestAlwaysRecordSampler_DropBecomesRecordOnly(t *testing.T) {
	tsWith, _ := trace.ParseTraceState("vendor=x")
	delegate := &fixedSampler{result: sdktrace.SamplingResult{
		Decision:   sdktrace.Drop,
		Attributes: []attribute.KeyValue{attribute.String("k", "v")},
		Tracestate: tsWith,
	}}

	got := NewAlwaysRecordSampler(delegate).ShouldSample(sdktrace.SamplingParameters{})

	if got.Decision != sdktrace.RecordOnly {
		t.Fatalf("decision = %v, want RecordOnly", got.Decision)
	}
	// Delegate's attributes and trace-state must be preserved when flipping the decision.
	if len(got.Attributes) != 1 || got.Attributes[0].Value.AsString() != "v" {
		t.Errorf("attributes = %v, want [k=v]", got.Attributes)
	}
	if got.Tracestate.Get("vendor") != "x" {
		t.Errorf("tracestate lost: %q", got.Tracestate.String())
	}
}

func TestAlwaysRecordSampler_RecordAndSampleUnchanged(t *testing.T) {
	delegate := &fixedSampler{result: sdktrace.SamplingResult{Decision: sdktrace.RecordAndSample}}
	got := NewAlwaysRecordSampler(delegate).ShouldSample(sdktrace.SamplingParameters{})
	if got.Decision != sdktrace.RecordAndSample {
		t.Errorf("decision = %v, want RecordAndSample (unchanged)", got.Decision)
	}
}

func TestAlwaysRecordSampler_RecordOnlyUnchanged(t *testing.T) {
	delegate := &fixedSampler{result: sdktrace.SamplingResult{Decision: sdktrace.RecordOnly}}
	got := NewAlwaysRecordSampler(delegate).ShouldSample(sdktrace.SamplingParameters{})
	if got.Decision != sdktrace.RecordOnly {
		t.Errorf("decision = %v, want RecordOnly (unchanged)", got.Decision)
	}
}

func TestAlwaysRecordSampler_Description(t *testing.T) {
	delegate := &fixedSampler{description: "TraceIDRatioBased{0.05}"}
	want := "AlwaysRecordSampler{TraceIDRatioBased{0.05}}"
	if got := NewAlwaysRecordSampler(delegate).Description(); got != want {
		t.Errorf("Description() = %q, want %q", got, want)
	}
}

func TestAlwaysRecordSampler_DelegatesEveryCall(t *testing.T) {
	delegate := &fixedSampler{result: sdktrace.SamplingResult{Decision: sdktrace.RecordAndSample}}
	NewAlwaysRecordSampler(delegate).ShouldSample(sdktrace.SamplingParameters{})
	if !delegate.called {
		t.Error("delegate ShouldSample was not called")
	}
}

func TestAlwaysRecordSampler_NilDelegatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on nil delegate")
		}
	}()
	NewAlwaysRecordSampler(nil)
}
