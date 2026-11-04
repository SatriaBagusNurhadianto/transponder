// Package echo is a provider that answers without calling anyone, for tests and demos.
package echo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"unicode/utf8"

	"github.com/raven-clown/transponder/internal/provider"
	"github.com/raven-clown/transponder/internal/schema"
)

// Echo repeats the last user message back.
type Echo struct {
	name string
}

var _ provider.Provider = (*Echo)(nil)

// New returns an echo provider called name.
func New(name string) *Echo {
	return &Echo{name: name}
}

// Name returns the configured name.
func (e *Echo) Name() string { return e.name }

// Dialect returns "none": echo has no native wire format.
func (e *Echo) Dialect() string { return "none" }

// Chat replies with "echo: " and the last user message.
func (e *Echo) Chat(_ context.Context, req *schema.ChatRequest) (*schema.ChatResponse, error) {
	var input int64
	last := ""
	for _, m := range req.Messages {
		input += tokens(m.Content)
		if m.Role == "user" {
			last = m.Content
		}
	}
	reply := "echo: " + last
	id := make([]byte, 8)
	_, _ = rand.Read(id)
	return &schema.ChatResponse{
		ID:           "echo-" + hex.EncodeToString(id),
		Model:        req.Model,
		Message:      schema.Message{Role: "assistant", Content: reply},
		FinishReason: "stop",
		Usage:        schema.Usage{InputTokens: input, OutputTokens: tokens(reply)},
	}, nil
}

// ChatStream is not supported yet.
func (e *Echo) ChatStream(context.Context, *schema.ChatRequest) (schema.Stream, error) {
	return nil, provider.ErrUnsupported
}

// Embed is not supported yet.
func (e *Echo) Embed(context.Context, *schema.EmbedRequest) (*schema.EmbedResponse, error) {
	return nil, provider.ErrUnsupported
}

func tokens(s string) int64 {
	return int64((utf8.RuneCountInString(s) + 3) / 4)
}
