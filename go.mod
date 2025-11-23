module github.com/recall-gpt/modelrelay/sdk/go

go 1.25.4

require (
	github.com/google/uuid v1.6.0
	github.com/recall-gpt/modelrelay/billingproxy v0.0.0-20251119210239-1133abe831c1
	github.com/recall-gpt/modelrelay/cloud v0.0.0-20251119210239-1133abe831c1
	github.com/recall-gpt/modelrelay/llmproxy v0.0.0-20251119210239-1133abe831c1
	go.opentelemetry.io/otel/trace v1.38.0
)

require (
	github.com/golang-jwt/jwt/v5 v5.3.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/rs/zerolog v1.34.0 // indirect
	github.com/stripe/stripe-go/v78 v78.12.0 // indirect
	go.opentelemetry.io/otel v1.38.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
)

replace github.com/recall-gpt/modelrelay/billingproxy => ../../billingproxy

replace github.com/recall-gpt/modelrelay/llmproxy => ../../llmproxy

replace github.com/recall-gpt/modelrelay/cloud => ../../cloud
