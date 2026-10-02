# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial span metrics feature for the OpenTelemetry Go SDK:
  - `NewAlwaysRecordSampler` — wraps a delegate sampler and converts `Drop` to `RecordOnly`
    so 100% of spans are recorded while export honors the configured sampling rate.
  - `NewSpanMetricsProcessor` — a `trace.SpanProcessor` that records
    `traces.span.metrics.calls` and `traces.span.metrics.duration` in `OnEnd`, and stamps the
    identity/dedup markers on spans.
