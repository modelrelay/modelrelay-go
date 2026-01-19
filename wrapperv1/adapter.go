// Package wrapperv1 provides types and interfaces for the wrapper_v1 data source contract.
package wrapperv1

import "context"

// Adapter maps a data source into the wrapperv1 contract.
type Adapter interface {
	Search(ctx context.Context, req SearchRequest) (SearchResponse, error)
	Get(ctx context.Context, req GetRequest) (GetResponse, error)
	Content(ctx context.Context, req ContentRequest) (ContentResponse, error)
}
