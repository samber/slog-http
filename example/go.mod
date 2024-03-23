module example

go 1.21

replace github.com/samber/slog-http => ../

require (
	github.com/samber/slog-formatter v1.0.0
	github.com/samber/slog-http v1.0.0
)

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/samber/lo v1.38.1 // indirect
	github.com/samber/slog-multi v1.0.0 // indirect
	go.opentelemetry.io/otel v1.19.0 // indirect
	go.opentelemetry.io/otel/trace v1.19.0 // indirect
	golang.org/x/exp v0.0.0-20230522175609-2e198f4a06a1 // indirect
)
