package sdk

import (
	"context"
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel/trace"
)

func injectTraceparent(ctx context.Context, req *http.Request) {
	span := trace.SpanFromContext(ctx)
	sc := span.SpanContext()
	if !sc.IsValid() {
		return
	}
	traceparent := fmt.Sprintf("00-%s-%s-01", sc.TraceID().String(), sc.SpanID().String())
	req.Header.Set("Traceparent", traceparent)
}
