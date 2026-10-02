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

// LibVersion is the plugin library version. It is stamped onto spans and metrics as a
// debug-only marker (consumers must not key logic on it). Kept as a source constant so
// it needs no build-time generation or runtime lookup.
const LibVersion = "0.1.0"
