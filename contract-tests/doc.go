// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package contracttests contains end-to-end contract tests for the span metrics plugin.
// They wire the plugin into a real OpenTelemetry SDK pipeline and assert the emitted
// metrics match the span-derived-metrics contract for both supported modes:
//
//   - Mode 1 (manual SDK): the app builds its own TracerProvider and registers the plugin.
//   - Mode 2 (library instrumentation): a component creates spans through the global
//     TracerProvider (as otelhttp/otelgrpc do); one processor on that provider covers them.
package contracttests
