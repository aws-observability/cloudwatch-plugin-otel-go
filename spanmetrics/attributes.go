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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Base dimensions on every datapoint (spec §3). service.name is deliberately NOT here:
// it lives on the metric resource (the host MeterProvider's resource), so duplicating it
// per datapoint would add a redundant dimension.
const (
	spanNameKey   = attribute.Key("span.name")
	spanKindKey   = attribute.Key("span.kind")
	statusCodeKey = attribute.Key("status.code")
)

// allowlist is the low-cardinality subset OTel semconv defines on the corresponding
// request metrics (spec §4). It is flat: copy any listed key that is present, regardless
// of span family (a span only carries the keys of its own family). These are the CURRENT
// semconv keys; legacy predecessors are handled by legacyFallbacks below.
//
// Every allowlisted attribute is a string except http.response.status_code and
// server.port (int), and aws.dynamodb.table_names (string slice); those are copied under
// their native types by copyIfPresent, which preserves whatever type the span carries.
var allowlist = []attribute.Key{
	// HTTP
	"http.request.method",
	"http.response.status_code",
	"http.route",
	"error.type",
	// RPC
	"rpc.system.name",
	"rpc.service",
	"rpc.method",
	// Database
	"db.system.name",
	"db.operation.name",
	"db.collection.name",
	// Messaging
	"messaging.system",
	"messaging.operation.name",
	"messaging.operation.type",
	"messaging.consumer.group.name",
	// Peer
	"server.address",
	"server.port",
	// GenAI
	"gen_ai.request.model",
	"gen_ai.provider.name",
	"gen_ai.operation.name",
	// AWS resource identity
	"aws.s3.bucket",
	"aws.dynamodb.table_names",
	"aws.lambda.invoked_arn",
	"aws.sns.topic.arn",
	"aws.sqs.queue.url",
	// FaaS
	"faas.invoked_name",
	"faas.invoked_provider",
	"faas.invoked_region",
	"faas.trigger",
}

// legacyFallback maps a current semconv key to the legacy keys checked (in order) when
// the current key is absent. When a legacy key is present, its key AND value are emitted
// unchanged — no rename, no value translation (spec §4), because some migrations also
// changed the value vocabulary (e.g. db.system=mssql -> db.system.name=microsoft.sql_server).
type legacyFallback struct {
	current attribute.Key
	legacy  []attribute.Key
}

var legacyFallbacks = []legacyFallback{
	{current: "http.request.method", legacy: []attribute.Key{"http.method"}},
	{current: "http.response.status_code", legacy: []attribute.Key{"http.status_code"}},
	{current: "rpc.system.name", legacy: []attribute.Key{"rpc.system"}},
	{current: "db.system.name", legacy: []attribute.Key{"db.system"}},
	{current: "db.operation.name", legacy: []attribute.Key{"db.operation"}},
	{current: "db.collection.name", legacy: []attribute.Key{"db.sql.table"}},
	// Peer network attributes renamed from net.peer.*/net.host.* to server.* in newer semconv.
	{current: "server.address", legacy: []attribute.Key{"net.peer.name", "net.host.name"}},
	{current: "server.port", legacy: []attribute.Key{"net.peer.port", "net.host.port"}},
}

const (
	messagingDestinationName      = attribute.Key("messaging.destination.name")
	messagingDestinationTemporary = attribute.Key("messaging.destination.temporary")
	messagingDestinationAnonymous = attribute.Key("messaging.destination.anonymous")
)

// buildAttributes produces the metric attribute set for a completed span: the three base
// dimensions, the identity/schema markers (spec §6), and every allowlisted semconv
// attribute present (with legacy fallbacks and the temporary/anonymous destination guard).
func buildAttributes(span sdktrace.ReadOnlySpan) attribute.Set {
	source := attributesByKey(span.Attributes())

	kvs := []attribute.KeyValue{
		spanNameKey.String(span.Name()),
		spanKindKey.String(spanKindString(span.SpanKind())),
		statusCodeKey.String(statusCodeString(span.Status().Code)),
		// Schema + library-version markers, on both spans and metrics (spec §6).
		attribute.String(schemaAttr, schemaVersion),
		attribute.String(libVersionAttr, LibVersion),
	}

	for _, key := range allowlist {
		if kv, ok := source[key]; ok {
			kvs = append(kvs, kv)
		}
	}
	kvs = appendLegacyFallbacks(kvs, source)
	kvs = appendDestinationIfNamed(kvs, source)

	return attribute.NewSet(kvs...)
}

// appendLegacyFallbacks emits, for each current key that is absent from the span, the
// first present legacy key/value unchanged.
func appendLegacyFallbacks(kvs []attribute.KeyValue, source map[attribute.Key]attribute.KeyValue) []attribute.KeyValue {
	for _, fb := range legacyFallbacks {
		if _, ok := source[fb.current]; ok {
			continue
		}
		for _, legacyKey := range fb.legacy {
			if kv, ok := source[legacyKey]; ok {
				kvs = append(kvs, kv)
				break
			}
		}
	}
	return kvs
}

// appendDestinationIfNamed copies messaging.destination.name unless the destination is
// temporary or anonymous (unbounded names, spec §4).
func appendDestinationIfNamed(kvs []attribute.KeyValue, source map[attribute.Key]attribute.KeyValue) []attribute.KeyValue {
	kv, ok := source[messagingDestinationName]
	if !ok {
		return kvs
	}
	if isTrue(source, messagingDestinationTemporary) || isTrue(source, messagingDestinationAnonymous) {
		return kvs
	}
	return append(kvs, kv)
}

func isTrue(source map[attribute.Key]attribute.KeyValue, key attribute.Key) bool {
	kv, ok := source[key]
	return ok && kv.Value.Type() == attribute.BOOL && kv.Value.AsBool()
}

func attributesByKey(kvs []attribute.KeyValue) map[attribute.Key]attribute.KeyValue {
	out := make(map[attribute.Key]attribute.KeyValue, len(kvs))
	for _, kv := range kvs {
		out[kv.Key] = kv
	}
	return out
}

// spanKindString maps to the fixed spec vocabulary. Go's SpanKind.String() is lowercase
// ("server"), so an explicit mapping is used to emit the spec's uppercase values.
func spanKindString(kind trace.SpanKind) string {
	switch kind {
	case trace.SpanKindServer:
		return "SERVER"
	case trace.SpanKindClient:
		return "CLIENT"
	case trace.SpanKindProducer:
		return "PRODUCER"
	case trace.SpanKindConsumer:
		return "CONSUMER"
	default:
		// SpanKindInternal and SpanKindUnspecified both map to INTERNAL, matching the SDK
		// default kind.
		return "INTERNAL"
	}
}

// statusCodeString maps to the fixed spec vocabulary. Go's codes.Code.String() is
// "Unset"/"Ok"/"Error" (mixed case), so an explicit mapping emits the spec's uppercase.
func statusCodeString(code codes.Code) string {
	switch code {
	case codes.Ok:
		return "OK"
	case codes.Error:
		return "ERROR"
	default:
		return "UNSET"
	}
}
