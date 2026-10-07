// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package spanmetrics

import (
	"context"
	"os"
	"strings"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// TestSamplerFromEnv_Mapping asserts each recognized OTEL_TRACES_SAMPLER value maps to the
// expected SDK sampler, compared via the sampler's stable Description() string. Expected
// descriptions are taken from the equivalent public constructor so the test tracks the SDK.
func TestSamplerFromEnv_Mapping(t *testing.T) {
	cases := []struct {
		envValue string
		want     sdktrace.Sampler
	}{
		{"always_on", sdktrace.AlwaysSample()},
		{"always_off", sdktrace.NeverSample()},
		{"traceidratio", sdktrace.TraceIDRatioBased(1.0)}, // no ARG -> ratio 1.0
		{"parentbased_always_on", sdktrace.ParentBased(sdktrace.AlwaysSample())},
		{"parentbased_always_off", sdktrace.ParentBased(sdktrace.NeverSample())},
		{"parentbased_traceidratio", sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1.0))},
	}
	for _, tc := range cases {
		t.Run(tc.envValue, func(t *testing.T) {
			t.Setenv(envTracesSampler, tc.envValue)
			got := SamplerFromEnv().Description()
			if want := tc.want.Description(); got != want {
				t.Errorf("SamplerFromEnv() = %q, want %q", got, want)
			}
		})
	}
}

// TestSamplerFromEnv_TraceIDRatioWithArg verifies traceidratio honors OTEL_TRACES_SAMPLER_ARG:
// the Description must reflect the 0.05 ratio and must not collapse to the default.
func TestSamplerFromEnv_TraceIDRatioWithArg(t *testing.T) {
	t.Setenv(envTracesSampler, "traceidratio")
	t.Setenv(envTracesSamplerArg, "0.05")

	got := SamplerFromEnv().Description()
	if !strings.Contains(got, "TraceIDRatioBased") {
		t.Errorf("Description() = %q, want a TraceIDRatioBased sampler", got)
	}
	if !strings.Contains(got, "0.05") {
		t.Errorf("Description() = %q, want it to encode ratio 0.05", got)
	}
	if got == defaultSampler().Description() {
		t.Errorf("Description() = %q, must not equal the default sampler", got)
	}
	if want := sdktrace.TraceIDRatioBased(0.05).Description(); got != want {
		t.Errorf("Description() = %q, want %q", got, want)
	}
}

// TestSamplerFromEnv_Unset: with OTEL_TRACES_SAMPLER absent, SamplerFromEnv equals defaultSampler
// (ParentBased{AlwaysOnSampler}), so behavior is unchanged from the SDK default.
func TestSamplerFromEnv_Unset(t *testing.T) {
	// Make the test airtight against an ambient OTEL_TRACES_SAMPLER: t.Setenv registers a
	// restore of the current value, then we unset it for the duration of the test.
	t.Setenv(envTracesSampler, "")
	if err := os.Unsetenv(envTracesSampler); err != nil {
		t.Fatalf("unset %s: %v", envTracesSampler, err)
	}
	got := SamplerFromEnv().Description()
	if want := defaultSampler().Description(); got != want {
		t.Errorf("SamplerFromEnv() (unset) = %q, want default %q", got, want)
	}
}

// TestSamplerFromEnv_Unknown: an unrecognized value falls back to the default sampler.
func TestSamplerFromEnv_Unknown(t *testing.T) {
	t.Setenv(envTracesSampler, "banana")
	got := SamplerFromEnv().Description()
	if want := defaultSampler().Description(); got != want {
		t.Errorf("SamplerFromEnv() (unknown) = %q, want default %q", got, want)
	}
}

// TestRatioFromEnv guards the critical JS-parity behavior: blank/absent, non-numeric, negative, and
// >1 all fall back to 1.0 (record everything), NEVER 0; a valid in-range value is returned as-is.
func TestRatioFromEnv(t *testing.T) {
	t.Run("unset", func(t *testing.T) {
		if got := ratioFromEnv(); got != 1.0 {
			t.Errorf("ratioFromEnv() unset = %v, want 1.0", got)
		}
	})
	cases := []struct {
		name string
		arg  string
		want float64
	}{
		{"empty", "", 1.0},
		{"non_numeric", "abc", 1.0},
		{"negative", "-0.5", 1.0},
		{"greater_than_one", "2", 1.0},
		{"valid", "0.25", 0.25},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envTracesSamplerArg, tc.arg)
			if got := ratioFromEnv(); got != tc.want {
				t.Errorf("ratioFromEnv(%q) = %v, want %v", tc.arg, got, tc.want)
			}
		})
	}
}

// TestSamplerFromEnv_PreservesSamplingWhileForcingRecord is the headline preservation test: with
// OTEL_TRACES_SAMPLER=always_off, wrapping SamplerFromEnv in NewAlwaysRecordSampler must yield a
// RecordOnly decision (recorded/metered, but NOT sampled/exported) — not Drop, not RecordAndSample.
// It asserts both the sampler decision directly and the end-to-end metered count.
func TestSamplerFromEnv_PreservesSamplingWhileForcingRecord(t *testing.T) {
	t.Setenv(envTracesSampler, "always_off")

	sampler := NewAlwaysRecordSampler(SamplerFromEnv())

	// (a) Direct sampler-decision assertion: always_off would Drop; the wrapper flips it to
	// RecordOnly. It must be neither Drop nor RecordAndSample.
	res := sampler.ShouldSample(sdktrace.SamplingParameters{Name: "op"})
	if res.Decision != sdktrace.RecordOnly {
		t.Fatalf("decision = %v, want RecordOnly (not Drop, not RecordAndSample)", res.Decision)
	}

	// (b) End-to-end: build a TracerProvider with the wrapped sampler + metrics processor + span
	// recorder, run N spans, and assert every span was metered (calls == N) while NONE were
	// sampled/exported (each recorded span's SpanContext IsSampled == false).
	const n = 5
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sampler),
		sdktrace.WithSpanProcessor(NewSpanMetricsProcessor(mp)),
		sdktrace.WithSpanProcessor(rec),
	)
	tracer := tp.Tracer("test")
	for i := 0; i < n; i++ {
		_, span := tracer.Start(context.Background(), "op", trace.WithSpanKind(trace.SpanKindServer))
		span.End()
	}

	// All N spans metered.
	calls := collect(t, reader)[callsMetric]
	sum, ok := calls.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("calls data type = %T, want Sum[int64]", calls.Data)
	}
	if len(sum.DataPoints) != 1 || sum.DataPoints[0].Value != n {
		t.Fatalf("calls value = %v, want %d (all recorded/metered)", sum.DataPoints, n)
	}

	// None sampled/exported: RecordOnly spans are ended but their SpanContext is not sampled.
	ended := rec.Ended()
	if len(ended) != n {
		t.Fatalf("recorded spans = %d, want %d", len(ended), n)
	}
	for i, s := range ended {
		if s.SpanContext().IsSampled() {
			t.Errorf("span[%d] IsSampled = true, want false (RecordOnly, not RecordAndSample)", i)
		}
	}
}
