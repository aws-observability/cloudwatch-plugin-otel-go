// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

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
