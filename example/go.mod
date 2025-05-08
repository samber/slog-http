module example

go 1.21

replace github.com/samber/slog-http => ../

require (
	github.com/samber/slog-formatter v1.0.0
	github.com/samber/slog-http v1.0.0
)

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/samber/lo v1.47.0 // indirect
	github.com/samber/slog-multi v1.0.0 // indirect
	go.opentelemetry.io/otel v1.29.0 // indirect
	go.opentelemetry.io/otel/trace v1.29.0 // indirect
	golang.org/x/text v0.16.0 // indirect
)
