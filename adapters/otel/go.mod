// Isopace OpenTelemetry observer adapter — a separate module so the stdlib-only
// core never imports a telemetry SDK. require/replace mirror the other adapters;
// the OpenTelemetry requirements are filled in by `go mod tidy`.
module github.com/teqpace-services/isopace/adapters/otel

go 1.26

require (
	github.com/teqpace-services/isopace v0.3.0
	go.opentelemetry.io/otel v1.44.0
	go.opentelemetry.io/otel/metric v1.44.0
	go.opentelemetry.io/otel/sdk v1.44.0
	go.opentelemetry.io/otel/trace v1.44.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	golang.org/x/sys v0.45.0 // indirect
)

replace github.com/teqpace-services/isopace => ../..
