// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package spanmetrics

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// recordSpan builds one real span through an SDK tracer (so buildAttributes runs against a
// genuine ReadOnlySpan) with the given name/kind/attributes and returns it. The span is given
// a fixed 250ms duration so duration assertions are deterministic.
func recordSpan(t *testing.T, name string, kind trace.SpanKind, setErr bool, attrs ...attribute.KeyValue) sdktrace.ReadOnlySpan {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	start := time.Now()
	_, span := tp.Tracer("test").Start(
		context.Background(),
		name,
		trace.WithSpanKind(kind),
		trace.WithTimestamp(start),
	)
	span.SetAttributes(attrs...)
	if setErr {
		span.SetStatus(codes.Error, "boom")
	}
	span.End(trace.WithTimestamp(start.Add(250 * time.Millisecond)))

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 recorded span, got %d", len(spans))
	}
	return spans[0]
}

// attrMap flattens an attribute.Set into a key->KeyValue map for assertions.
func attrMap(set attribute.Set) map[attribute.Key]attribute.KeyValue {
	out := make(map[attribute.Key]attribute.KeyValue)
	for it := set.Iter(); it.Next(); {
		kv := it.Attribute()
		out[kv.Key] = kv
	}
	return out
}

func mustString(t *testing.T, m map[attribute.Key]attribute.KeyValue, key string) string {
	t.Helper()
	kv, ok := m[attribute.Key(key)]
	if !ok {
		t.Fatalf("attribute %q missing; present keys: %v", key, keys(m))
	}
	if kv.Value.Type() != attribute.STRING {
		t.Fatalf("attribute %q type = %v, want STRING", key, kv.Value.Type())
	}
	return kv.Value.AsString()
}

func mustInt(t *testing.T, m map[attribute.Key]attribute.KeyValue, key string) int64 {
	t.Helper()
	kv, ok := m[attribute.Key(key)]
	if !ok {
		t.Fatalf("attribute %q missing; present keys: %v", key, keys(m))
	}
	if kv.Value.Type() != attribute.INT64 {
		t.Fatalf("attribute %q type = %v, want INT64", key, kv.Value.Type())
	}
	return kv.Value.AsInt64()
}

func absent(t *testing.T, m map[attribute.Key]attribute.KeyValue, key string) {
	t.Helper()
	if _, ok := m[attribute.Key(key)]; ok {
		t.Fatalf("attribute %q should be absent but was present", key)
	}
}

func keys(m map[attribute.Key]attribute.KeyValue) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, string(k))
	}
	return out
}
