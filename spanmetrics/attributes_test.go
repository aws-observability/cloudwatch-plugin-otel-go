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
	"go.opentelemetry.io/otel/trace"
)

func TestBaseAttributes(t *testing.T) {
	span := recordSpan(t, "GET /ping", trace.SpanKindServer, false)
	m := attrMap(buildAttributes(span))

	if got := mustString(t, m, "span.name"); got != "GET /ping" {
		t.Errorf("span.name = %q", got)
	}
	if got := mustString(t, m, "span.kind"); got != "SERVER" {
		t.Errorf("span.kind = %q, want SERVER", got)
	}
	if got := mustString(t, m, "status.code"); got != "UNSET" {
		t.Errorf("status.code = %q, want UNSET", got)
	}
	// Identity markers on every datapoint.
	if got := mustString(t, m, "aws.otel.span.metrics.schema"); got != "v1" {
		t.Errorf("schema = %q, want v1", got)
	}
	if got := mustString(t, m, "aws.otel.extension.lib.version"); got != LibVersion {
		t.Errorf("lib.version = %q, want %q", got, LibVersion)
	}
	// service.name is NOT a datapoint attribute — it lives on the metric resource.
	absent(t, m, "service.name")
}

func TestSpanKindMapping(t *testing.T) {
	cases := map[trace.SpanKind]string{
		trace.SpanKindServer:   "SERVER",
		trace.SpanKindClient:   "CLIENT",
		trace.SpanKindProducer: "PRODUCER",
		trace.SpanKindConsumer: "CONSUMER",
		trace.SpanKindInternal: "INTERNAL",
	}
	for kind, want := range cases {
		span := recordSpan(t, "op", kind, false)
		if got := mustString(t, attrMap(buildAttributes(span)), "span.kind"); got != want {
			t.Errorf("kind %v -> %q, want %q", kind, got, want)
		}
	}
}

func TestStatusCodeError(t *testing.T) {
	span := recordSpan(t, "op", trace.SpanKindServer, true)
	if got := mustString(t, attrMap(buildAttributes(span)), "status.code"); got != "ERROR" {
		t.Errorf("status.code = %q, want ERROR", got)
	}
}

func TestHttpCurrentAttributes(t *testing.T) {
	span := recordSpan(t, "GET /users/:id", trace.SpanKindServer, false,
		attribute.String("http.request.method", "GET"),
		attribute.Int("http.response.status_code", 200),
		attribute.String("http.route", "/users/:id"),
		attribute.String("error.type", "500"),
	)
	m := attrMap(buildAttributes(span))
	if got := mustString(t, m, "http.request.method"); got != "GET" {
		t.Errorf("http.request.method = %q", got)
	}
	// status_code must be emitted as an int, not a string.
	if got := mustInt(t, m, "http.response.status_code"); got != 200 {
		t.Errorf("http.response.status_code = %d, want 200", got)
	}
	if got := mustString(t, m, "http.route"); got != "/users/:id" {
		t.Errorf("http.route = %q", got)
	}
	if got := mustString(t, m, "error.type"); got != "500" {
		t.Errorf("error.type = %q", got)
	}
}

func TestHttpLegacyFallback(t *testing.T) {
	// Only legacy keys present -> passed through under their own key/value, unchanged.
	span := recordSpan(t, "op", trace.SpanKindServer, false,
		attribute.String("http.method", "POST"),
		attribute.Int("http.status_code", 404),
	)
	m := attrMap(buildAttributes(span))
	if got := mustString(t, m, "http.method"); got != "POST" {
		t.Errorf("legacy http.method = %q", got)
	}
	if got := mustInt(t, m, "http.status_code"); got != 404 {
		t.Errorf("legacy http.status_code = %d", got)
	}
	// The current keys must NOT be synthesized from the legacy ones.
	absent(t, m, "http.request.method")
	absent(t, m, "http.response.status_code")
}

func TestCurrentKeyWinsOverLegacy(t *testing.T) {
	// When both are present, only the current key is emitted; the legacy key is dropped.
	span := recordSpan(t, "op", trace.SpanKindClient, false,
		attribute.String("db.system.name", "postgresql"),
		attribute.String("db.system", "postgres"),
	)
	m := attrMap(buildAttributes(span))
	if got := mustString(t, m, "db.system.name"); got != "postgresql" {
		t.Errorf("db.system.name = %q", got)
	}
	absent(t, m, "db.system")
}

func TestDbLegacyFallback(t *testing.T) {
	span := recordSpan(t, "SELECT items", trace.SpanKindClient, false,
		attribute.String("db.system", "mysql"),
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.sql.table", "items"),
	)
	m := attrMap(buildAttributes(span))
	if got := mustString(t, m, "db.system"); got != "mysql" {
		t.Errorf("db.system = %q", got)
	}
	if got := mustString(t, m, "db.operation"); got != "SELECT" {
		t.Errorf("db.operation = %q", got)
	}
	if got := mustString(t, m, "db.sql.table"); got != "items" {
		t.Errorf("db.sql.table = %q", got)
	}
	absent(t, m, "db.system.name")
}

func TestRpcAttributes(t *testing.T) {
	span := recordSpan(t, "Echo/Echo", trace.SpanKindClient, false,
		attribute.String("rpc.system.name", "grpc"),
		attribute.String("rpc.service", "Echo"),
		attribute.String("rpc.method", "Echo"),
		// Non-allowlisted RPC attribute must be excluded.
		attribute.String("rpc.grpc.request.metadata", "secret"),
	)
	m := attrMap(buildAttributes(span))
	if got := mustString(t, m, "rpc.system.name"); got != "grpc" {
		t.Errorf("rpc.system.name = %q", got)
	}
	mustString(t, m, "rpc.service")
	mustString(t, m, "rpc.method")
	absent(t, m, "rpc.grpc.request.metadata")
}

func TestRpcSystemLegacyFallback(t *testing.T) {
	span := recordSpan(t, "op", trace.SpanKindClient, false,
		attribute.String("rpc.system", "grpc"),
	)
	m := attrMap(buildAttributes(span))
	if got := mustString(t, m, "rpc.system"); got != "grpc" {
		t.Errorf("legacy rpc.system = %q", got)
	}
	absent(t, m, "rpc.system.name")
}

func TestPeerLegacyFallbackFirstPresentWins(t *testing.T) {
	// net.peer.name and net.host.name both fall back to server.address; first present wins.
	span := recordSpan(t, "op", trace.SpanKindClient, false,
		attribute.String("net.peer.name", "db.internal"),
		attribute.String("net.host.name", "should-not-win"),
		attribute.Int("net.peer.port", 5432),
	)
	m := attrMap(buildAttributes(span))
	if got := mustString(t, m, "net.peer.name"); got != "db.internal" {
		t.Errorf("server.address legacy = %q, want db.internal", got)
	}
	absent(t, m, "net.host.name")
	if got := mustInt(t, m, "net.peer.port"); got != 5432 {
		t.Errorf("server.port legacy = %d", got)
	}
}

func TestMessagingDestinationNamed(t *testing.T) {
	span := recordSpan(t, "publish", trace.SpanKindProducer, false,
		attribute.String("messaging.system", "kafka"),
		attribute.String("messaging.operation.name", "publish"),
		attribute.String("messaging.destination.name", "orders"),
	)
	m := attrMap(buildAttributes(span))
	if got := mustString(t, m, "messaging.destination.name"); got != "orders" {
		t.Errorf("destination = %q", got)
	}
	mustString(t, m, "messaging.system")
}

func TestMessagingDestinationTemporarySkipped(t *testing.T) {
	span := recordSpan(t, "publish", trace.SpanKindProducer, false,
		attribute.String("messaging.destination.name", "amq.gen-xyz"),
		attribute.Bool("messaging.destination.temporary", true),
	)
	absent(t, attrMap(buildAttributes(span)), "messaging.destination.name")
}

func TestMessagingDestinationAnonymousSkipped(t *testing.T) {
	span := recordSpan(t, "publish", trace.SpanKindProducer, false,
		attribute.String("messaging.destination.name", "anon"),
		attribute.Bool("messaging.destination.anonymous", true),
	)
	absent(t, attrMap(buildAttributes(span)), "messaging.destination.name")
}

func TestDynamoDbTableNamesSlice(t *testing.T) {
	span := recordSpan(t, "op", trace.SpanKindClient, false,
		attribute.StringSlice("aws.dynamodb.table_names", []string{"t1", "t2"}),
	)
	m := attrMap(buildAttributes(span))
	kv, ok := m["aws.dynamodb.table_names"]
	if !ok {
		t.Fatal("aws.dynamodb.table_names missing")
	}
	if kv.Value.Type() != attribute.STRINGSLICE {
		t.Fatalf("type = %v, want STRINGSLICE", kv.Value.Type())
	}
	if got := kv.Value.AsStringSlice(); len(got) != 2 || got[0] != "t1" || got[1] != "t2" {
		t.Errorf("table_names = %v", got)
	}
}

func TestNonAllowlistedAttributeExcluded(t *testing.T) {
	span := recordSpan(t, "op", trace.SpanKindServer, false,
		attribute.String("http.request.header.authorization", "secret"),
		attribute.String("custom.tenant", "acme"),
	)
	m := attrMap(buildAttributes(span))
	absent(t, m, "http.request.header.authorization")
	absent(t, m, "custom.tenant")
}
