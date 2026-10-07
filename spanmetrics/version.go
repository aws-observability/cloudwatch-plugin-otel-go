// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package spanmetrics

// LibVersion is the plugin library version. It is stamped onto spans and metrics as a
// debug-only marker (consumers must not key logic on it). Kept as a source constant so
// it needs no build-time generation or runtime lookup.
const LibVersion = "0.1.0"
