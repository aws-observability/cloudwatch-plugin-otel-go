# CloudWatch Plugin for OpenTelemetry (Go)

A plugin for the OpenTelemetry Go SDK that generates request ("RED") metrics from spans
**in-process, from 100% of spans, before trace sampling is applied**.

Span metrics are normally generated downstream (e.g. by the OpenTelemetry Collector's
`spanmetrics` connector) from whatever spans arrive there — which is only the *sampled*
subset. At low sampling rates that undercounts. This plugin records the metrics inside the
SDK, alongside a record-forcing sampler, so the metrics reflect every span while trace
**export** still honors the configured sampling rate.

Span metrics are the first feature of this plugin.

## What it produces

Two metrics, matching the OpenTelemetry SpanMetrics naming:

| Metric | Instrument | Unit |
| --- | --- | --- |
| `traces.span.metrics.calls` | Counter (monotonic sum) | `{call}` |
| `traces.span.metrics.duration` | Histogram | `s` (seconds) |

Each datapoint carries low-cardinality dimensions: `span.name`, `span.kind`, `status.code`,
plus any allowlisted semantic-convention attributes present on the span (e.g.
`http.request.method`, `http.route`, `http.response.status_code`,
`rpc.system.name`/`rpc.service`/`rpc.method`,
`db.system.name`/`db.operation.name`/`db.collection.name`,
`messaging.system`/`messaging.operation.name`/`messaging.destination.name`). Current
semantic-convention keys are used, with recognized legacy keys passed through under their own
key/value when the current key is absent. `service.name` is carried by the metric's
**resource** (the supplied MeterProvider's resource), not duplicated on each datapoint.

Both metrics also carry two identity attributes:

- `aws.otel.span.metrics.schema` — the metric schema version (e.g. `v1`).
- `aws.otel.extension.lib.version` — the plugin's library version (support/debugging only).

The metrics are emitted under the instrumentation scope `cloudwatch.plugin.otel.span_metrics`
and ride the `MeterProvider` you supply — they flow wherever your other metrics already go.

The plugin also writes these two attributes onto every recorded span (not just the metrics),
so a downstream generator can skip regenerating metrics for a span the plugin already metered.

## Supported instrumentation modes

| Mode | Supported | How the plugin is wired |
| --- | --- | --- |
| Manual SDK setup | Yes | Register the sampler + processor on your `TracerProvider` (below). |
| Library instrumentation (`otelhttp`, `otelgrpc`, …) | Yes | Covered automatically once those libraries share your `TracerProvider` (usually the global one). No per-library wiring. |
| eBPF / zero-code (`opentelemetry-go-instrumentation`) | No | That agent runs out-of-process with no in-process SDK to hook. Generate span metrics from the Collector's `spanmetrics` connector instead. |

## Install

```
go get github.com/aws-observability/cloudwatch-plugin-otel-go
```

## Usage

Register the record-forcing sampler and the span-metrics processor on your `TracerProvider`,
and give the processor the `MeterProvider` the metrics should be recorded into (normally the
same one your application already uses for its other metrics):

```go
import (
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/aws-observability/cloudwatch-plugin-otel-go/spanmetrics"
)

// The MeterProvider the span metrics record into. Its resource (service.name, etc.) becomes
// the metrics' resource, so build it with the same resource as your TracerProvider.
mp := sdkmetric.NewMeterProvider(
	sdkmetric.WithResource(res),
	sdkmetric.WithReader(metricReader),
)

tp := sdktrace.NewTracerProvider(
	sdktrace.WithResource(res),
	sdktrace.WithSampler(spanmetrics.NewAlwaysRecordSampler(spanmetrics.SamplerFromEnv())), // wrap the sampler
	sdktrace.WithSpanProcessor(spanmetrics.NewSpanMetricsProcessor(mp)),                     // add the processor
	sdktrace.WithBatcher(traceExporter),
)
otel.SetTracerProvider(tp) // otelhttp/otelgrpc spans now flow through it automatically
```

Three touch-points: wrap the sampler, add the processor, and pass the `MeterProvider`. Any
`otelhttp`/`otelgrpc` middleware you already use is covered the moment it shares this
`TracerProvider`.

The wrapper keeps your sampler's export decision but records the spans it would have dropped,
so the metrics see 100% of spans while trace export stays at the configured rate.

Choose the delegate to wrap based on how you configure sampling:

- **Sampling set by env vars** (`OTEL_TRACES_SAMPLER` / `OTEL_TRACES_SAMPLER_ARG`): use
  `spanmetrics.SamplerFromEnv()` as shown. Passing `WithSampler` overrides the SDK's own env
  handling, so `SamplerFromEnv()` re-resolves those variables — without it, wrapping a literal
  sampler would silently ignore your configured rate.
- **Sampling set in code**: pass your sampler directly, e.g.
  `spanmetrics.NewAlwaysRecordSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.05)))`.

> **Note:** register the processor and the wrapped sampler together, exactly once. Adding the
> processor without the sampler means it only sees exported spans (not 100%); registering it
> twice double-counts.

## Configuration

There are no plugin-specific configuration knobs. Metric destination, export interval, and
temporality are standard `MeterProvider` configuration you already control. Metric shaping
(re-bucketing, dropping a dimension, cardinality caps) is done with the SDK's Views.

## Notes

- **The metrics ride the `MeterProvider` you pass.** If it has no reader/exporter, the metrics
  go nowhere. If you pass `nil`, the processor is inert and (deliberately) does not stamp the
  dedup marker, so a downstream generator can still produce the metric.
- **`service.name` comes from the `MeterProvider`'s resource**, not the `TracerProvider`'s.
  Build the `MeterProvider` with the same resource as the tracer pipeline.

## Security

See [CONTRIBUTING](./CONTRIBUTING.md#security-issue-notifications) for information on reporting a
potential security issue. Please do not create a public GitHub issue for security reports.

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](./CONTRIBUTING.md).

## License

This project is licensed under the Apache-2.0 License. See [LICENSE](./LICENSE).
