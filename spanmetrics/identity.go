// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package spanmetrics

// Identity/dedup markers, stamped on both spans and generated metrics.
//
// The presence of schemaAttr on a span is the dedup signal: a downstream generator skips
// regenerating metrics for any span already carrying it, so metrics are produced exactly
// once. On a metric, schemaAttr records the schema version the metric was generated
// against. libVersionAttr is debug-only.
const (
	schemaAttr     = "aws.otel.span.metrics.schema"
	libVersionAttr = "aws.otel.extension.lib.version"

	// schemaVersion is the metric schema version. Bump when the metric shape changes.
	schemaVersion = "v1"
)
