module examples/custom_logger_zap

go 1.22.5

require (
	github.com/goflash/flash v0.0.0
	go.uber.org/zap v1.27.0
)

require (
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/julienschmidt/httprouter v1.3.0 // indirect
	go.opentelemetry.io/otel v1.28.0 // indirect
	go.opentelemetry.io/otel/metric v1.28.0 // indirect
	go.opentelemetry.io/otel/trace v1.28.0 // indirect
	go.uber.org/multierr v1.10.0 // indirect
)

replace github.com/goflash/flash => ../../
