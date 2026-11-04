// Package provider defines the interface every backend adapter implements.
package provider

import (
	"context"
	"errors"

	"github.com/raven-clown/transponder/internal/schema"
)

// ErrUnsupported means the backend cannot do the requested operation.
var ErrUnsupported = errors.New("provider: operation not supported")

// Provider is one configured backend.
type Provider interface {
	Name() string
	Dialect() string
	Chat(ctx context.Context, req *schema.ChatRequest) (*schema.ChatResponse, error)
	ChatStream(ctx context.Context, req *schema.ChatRequest) (schema.Stream, error)
	Embed(ctx context.Context, req *schema.EmbedRequest) (*schema.EmbedResponse, error)
}
