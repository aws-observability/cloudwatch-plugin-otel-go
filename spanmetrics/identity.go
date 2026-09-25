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
