# Release History: cloudwatch-plugin-otel-go

## v0.1.0 - 2026-10-12

* Initial release: in-process span-derived RED metrics (`traces.span.metrics.calls`,
  `traces.span.metrics.duration`) for the OpenTelemetry Go SDK, generated from 100% of spans
  while trace export keeps the configured sampling rate.
  * `NewSpanMetricsProcessor`: a `trace.SpanProcessor` that records the two metrics and stamps
    identity/dedup markers on spans and metrics.
  * `NewAlwaysRecordSampler`: wraps a sampler and converts `Drop` decisions to `RecordOnly`.
  * `SamplerFromEnv`: resolves the sampler configured by `OTEL_TRACES_SAMPLER` and
    `OTEL_TRACES_SAMPLER_ARG`, so wrapping it preserves the configured sampling rate.
  * On SERVER spans, the legacy fallback for `server.address`/`server.port` uses
    `net.host.name`/`net.host.port`, not the client's `net.peer.name`/`net.peer.port`, so client
    addresses and ephemeral ports do not become metric dimensions.
    ([#3](https://github.com/aws-observability/cloudwatch-plugin-otel-go/pull/3))
* Requires Go 1.25 or later and OpenTelemetry Go v1.45.0 or later.
