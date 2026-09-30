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

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// HTTP server span end-to-end: base attrs + current HTTP semconv (int status_code) + route.
func TestHttpServerFamily(t *testing.T) {
	p := newPipeline(t)
	p.emit(context.Background(), p.tp.Tracer("app"), "GET /users/:id", trace.SpanKindServer,
		attribute.String("http.request.method", "GET"),
		attribute.Int("http.response.status_code", 200),
		attribute.String("http.route", "/users/:id"),
	)
	m, _ := callsDataPoint(t, p.collect(t))
	if got := str(t, m, "http.request.method"); got != "GET" {
		t.Errorf("http.request.method = %q", got)
	}
	if got := str(t, m, "http.route"); got != "/users/:id" {
		t.Errorf("http.route = %q", got)
	}
	kv := m["http.response.status_code"]
	if kv.Value.Type() != attribute.INT64 || kv.Value.AsInt64() != 200 {
		t.Errorf("http.response.status_code = %v (type %v), want int 200", kv.Value, kv.Value.Type())
	}
}

// DB client span with legacy semconv (as OTel instrumentation still emits) passes through
// under the legacy keys/values unchanged.
func TestDbClientFamilyLegacy(t *testing.T) {
	p := newPipeline(t)
	p.emit(context.Background(), p.tp.Tracer("app"), "SELECT items", trace.SpanKindClient,
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.sql.table", "items"),
	)
	m, _ := callsDataPoint(t, p.collect(t))
	if got := str(t, m, "db.system"); got != "postgresql" {
		t.Errorf("db.system = %q", got)
	}
	if got := str(t, m, "db.operation"); got != "SELECT" {
		t.Errorf("db.operation = %q", got)
	}
	if got := str(t, m, "db.sql.table"); got != "items" {
		t.Errorf("db.sql.table = %q", got)
	}
	if hasKey(m, "db.system.name") {
		t.Error("db.system.name should not be synthesized from legacy db.system")
	}
}

// RPC client span with current semconv.
func TestRpcClientFamily(t *testing.T) {
	p := newPipeline(t)
	p.emit(context.Background(), p.tp.Tracer("app"), "Echo/Echo", trace.SpanKindClient,
		attribute.String("rpc.system.name", "grpc"),
		attribute.String("rpc.service", "Echo"),
		attribute.String("rpc.method", "Echo"),
	)
	m, _ := callsDataPoint(t, p.collect(t))
	if got := str(t, m, "rpc.system.name"); got != "grpc" {
		t.Errorf("rpc.system.name = %q", got)
	}
	if got := str(t, m, "rpc.service"); got != "Echo" {
		t.Errorf("rpc.service = %q", got)
	}
	if got := str(t, m, "rpc.method"); got != "Echo" {
		t.Errorf("rpc.method = %q", got)
	}
}

// Messaging producer span; a named destination is kept.
func TestMessagingFamily(t *testing.T) {
	p := newPipeline(t)
	p.emit(context.Background(), p.tp.Tracer("app"), "orders publish", trace.SpanKindProducer,
		attribute.String("messaging.system", "kafka"),
		attribute.String("messaging.operation.name", "publish"),
		attribute.String("messaging.destination.name", "orders"),
	)
	m, _ := callsDataPoint(t, p.collect(t))
	if got := str(t, m, "messaging.system"); got != "kafka" {
		t.Errorf("messaging.system = %q", got)
	}
	if got := str(t, m, "messaging.destination.name"); got != "orders" {
		t.Errorf("messaging.destination.name = %q", got)
	}
}

// A temporary messaging destination is omitted (unbounded name).
func TestMessagingTemporaryDestinationOmitted(t *testing.T) {
	p := newPipeline(t)
	p.emit(context.Background(), p.tp.Tracer("app"), "publish", trace.SpanKindProducer,
		attribute.String("messaging.system", "rabbitmq"),
		attribute.String("messaging.destination.name", "amq.gen-abc"),
		attribute.Bool("messaging.destination.temporary", true),
	)
	m, _ := callsDataPoint(t, p.collect(t))
	if hasKey(m, "messaging.destination.name") {
		t.Error("temporary destination name should be omitted")
	}
}

// An error span carries status.code = ERROR.
func TestErrorSpanStatus(t *testing.T) {
	p := newPipeline(t)
	tracer := p.tp.Tracer("app")
	_, span := tracer.Start(context.Background(), "op", trace.WithSpanKind(trace.SpanKindServer))
	span.SetAttributes(attribute.String("error.type", "500"))
	span.SetStatus(codes.Error, "boom")
	span.End()

	m, _ := callsDataPoint(t, p.collect(t))
	if got := str(t, m, "status.code"); got != "ERROR" {
		t.Errorf("status.code = %q, want ERROR", got)
	}
	if got := str(t, m, "error.type"); got != "500" {
		t.Errorf("error.type = %q", got)
	}
}
