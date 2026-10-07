// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package spanmetrics

import (
	"log"
	"os"
	"strconv"
	"strings"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Why this file exists (a deliberately small, self-contained copy of OpenTelemetry behavior):
//
// The extension must wrap the user's configured sampler in NewAlwaysRecordSampler and pass the
// result to sdktrace.WithSampler. But sdktrace.NewTracerProvider only reads OTEL_TRACES_SAMPLER /
// OTEL_TRACES_SAMPLER_ARG when NO WithSampler option is given; passing WithSampler overrides that
// env resolution entirely. The SDK's own resolver (samplerFromEnv) is unexported and there is no
// TracerProvider getter to retrieve the sampler it built, so — exactly as the JS and .NET plugins
// had to — we re-implement the env-to-sampler mapping here using only public SDK constructors.
//
// SamplerFromEnv returns the sampler the user configured via env (or the SDK default when unset),
// which the caller then wraps:
//
//	sdktrace.WithSampler(spanmetrics.NewAlwaysRecordSampler(spanmetrics.SamplerFromEnv()))
//
// preserving the configured export/sampling rate while forcing 100% recording for metrics.

// Environment variable names, matching the OpenTelemetry specification.
const (
	envTracesSampler    = "OTEL_TRACES_SAMPLER"
	envTracesSamplerArg = "OTEL_TRACES_SAMPLER_ARG"
)

// defaultRatio is the fallback sampling ratio for traceidratio samplers when
// OTEL_TRACES_SAMPLER_ARG is blank, invalid, or out of range. It is 1.0 (record everything), NEVER
// 0: a 0 fallback would silently drop all trace export while our 100% metrics masked the failure.
const defaultRatio = 1.0

// SamplerFromEnv resolves the sampler the Go SDK would build from OTEL_TRACES_SAMPLER (and, for
// ratio-based samplers, OTEL_TRACES_SAMPLER_ARG), using only public go.opentelemetry.io/otel/sdk/
// trace constructors. It returns the SDK default (defaultSampler) when OTEL_TRACES_SAMPLER is unset
// or holds an unrecognized value. The result is meant to be wrapped by NewAlwaysRecordSampler.
func SamplerFromEnv() sdktrace.Sampler {
	raw, ok := os.LookupEnv(envTracesSampler)
	if !ok {
		return defaultSampler()
	}
	// Be lenient like typical parsers: trim surrounding whitespace and lowercase before matching.
	// The OTel spec tokens are all lowercase; strings.ToLower additionally makes matching
	// case-insensitive (e.g. "Always_On" also works), which is a superset of the JS behavior.
	name := strings.ToLower(strings.TrimSpace(raw))
	switch name {
	case "always_on":
		return sdktrace.AlwaysSample()
	case "always_off":
		return sdktrace.NeverSample()
	case "traceidratio":
		return sdktrace.TraceIDRatioBased(ratioFromEnv())
	case "parentbased_always_on":
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample())
	case "parentbased_traceidratio":
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratioFromEnv()))
	default:
		log.Printf("[span-metrics] unknown %s value %q; using the SDK default sampler", envTracesSampler, raw)
		return defaultSampler()
	}
}

// defaultSampler is the sampler the SDK applies when OTEL_TRACES_SAMPLER is unset or unrecognized.
// Kept equal to the Go SDK's own default (ParentBased(AlwaysSample)) so behavior is unchanged when
// env is unset. Single source of truth so callers do not each hard-code it.
func defaultSampler() sdktrace.Sampler {
	return sdktrace.ParentBased(sdktrace.AlwaysSample())
}

// ratioFromEnv reads OTEL_TRACES_SAMPLER_ARG as a sampling ratio. It returns the parsed value only
// when it is a valid float in [0,1]; for a blank/absent, non-numeric, negative, or >1 value it logs
// a warning and returns defaultRatio (1.0). It NEVER returns 0 on invalid input — dropping to 0
// would silently stop all trace export while our 100% metrics masked the failure (JS parity).
func ratioFromEnv() float64 {
	raw, ok := os.LookupEnv(envTracesSamplerArg)
	if !ok || strings.TrimSpace(raw) == "" {
		log.Printf("[span-metrics] %s is blank or unset; defaulting to %v", envTracesSamplerArg, defaultRatio)
		return defaultRatio
	}
	ratio, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		log.Printf("[span-metrics] %s=%q is not a valid number; defaulting to %v", envTracesSamplerArg, raw, defaultRatio)
		return defaultRatio
	}
	if ratio < 0 || ratio > 1 {
		log.Printf("[span-metrics] %s=%v is out of range [0..1]; defaulting to %v", envTracesSamplerArg, ratio, defaultRatio)
		return defaultRatio
	}
	return ratio
}
