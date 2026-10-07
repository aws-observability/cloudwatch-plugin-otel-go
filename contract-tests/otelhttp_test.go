// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contracttests

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// TestOtelHTTPServerSpan exercises the real otelhttp instrumentation end-to-end (mode 2):
// otelhttp.NewHandler produces the SERVER span from the tracer it gets via
// WithTracerProvider(p.tp), so the span flows through the plugin's sampler + processor with
// no per-span wiring. Unlike TestMode2_GlobalProviderLibrarySpans (which only simulates a
// global-provider span), this drives the actual contrib instrumentation over a live HTTP
// request, closing the mode-2 gap.
func TestOtelHTTPServerSpan(t *testing.T) {
	p := newPipeline(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})
	// WithTracerProvider(p.tp) routes otelhttp's spans through the plugin's processor.
	handler := otelhttp.NewHandler(inner, "GET /ping",
		otelhttp.WithTracerProvider(p.tp))

	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/ping", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http get: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	m, count := callsDataPoint(t, p.collect(t))
	if count != 1 {
		t.Fatalf("calls = %d, want 1 (real otelhttp SERVER span must reach the processor)", count)
	}
	if got := str(t, m, "span.kind"); got != "SERVER" {
		t.Errorf("span.kind = %q, want SERVER", got)
	}
	// otelhttp emits the request method. Depending on the semconv vintage it is either the
	// current key (http.request.method) or the legacy key (http.method); both flow through
	// the allowlist + legacy fallback, so accept either.
	if !hasKey(m, "http.request.method") && !hasKey(m, "http.method") {
		t.Errorf("neither http.request.method nor http.method present on calls metric; attrs=%v", m)
	}
}
