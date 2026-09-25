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

// Package contracttests contains end-to-end contract tests for the span metrics plugin.
// They wire the plugin into a real OpenTelemetry SDK pipeline and assert the emitted
// metrics match the span-derived-metrics contract for both supported modes:
//
//   - Mode 1 (manual SDK): the app builds its own TracerProvider and registers the plugin.
//   - Mode 2 (library instrumentation): a component creates spans through the global
//     TracerProvider (as otelhttp/otelgrpc do); one processor on that provider covers them.
package contracttests
