# Isopace OpenTelemetry adapter

An implementation of the Isopace `runtime.Observer` traces-and-metrics facade
over [OpenTelemetry](https://opentelemetry.io).

This is a **separate module** so the Isopace core never imports a telemetry SDK
(the core defines a tiny, dependency-free `Observer` interface; this adapter
bridges it to OTel). Wire it in at the edge of your application:

```go
import (
    oteladapter "github.com/teqpace-services/isopace/adapters/otel"
    "github.com/teqpace-services/isopace/runtime"
)

// From explicit providers:
obs := oteladapter.New(tracerProvider, meterProvider)

// …or from the globally-registered OpenTelemetry providers:
obs := oteladapter.Default()

host := runtime.NewHost(runtime.WithObserver(obs))
```

## Mapping

| Isopace | OpenTelemetry |
|---|---|
| `Observer.StartSpan` | `Tracer.Start` (attributes attached at start) |
| `Span.End` / `SetError` / `SetAttr` | `Span.End` / `RecordError`+`SetStatus(Error)` / `SetAttributes` |
| `Observer.Counter(name).Add` | `Int64Counter.Add` |
| `Observer.Histogram(name).Observe` | `Float64Histogram.Record` |
| `runtime.Attr` | `attribute.KeyValue` (string/bool/int/int64/float64; else stringified) |

Note: the `Counter`/`Histogram` contract carries no `context.Context`, so the
background context is used for measurements — OTel metric aggregation does not
depend on request context.
