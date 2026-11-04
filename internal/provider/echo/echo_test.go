package echo

import (
	"context"
	"errors"
	"testing"

	"github.com/raven-clown/transponder/internal/provider"
	"github.com/raven-clown/transponder/internal/schema"
)

func chat(t *testing.T, msgs ...schema.Message) *schema.ChatResponse {
	t.Helper()
	resp, err := New("echo").Chat(context.Background(), &schema.ChatRequest{Model: "m", Messages: msgs})
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestEchoReply(t *testing.T) {
	resp := chat(t,
		schema.Message{Role: "system", Content: "be brief"},
		schema.Message{Role: "user", Content: "hi"},
		schema.Message{Role: "assistant", Content: "yo"},
		schema.Message{Role: "user", Content: "ping"},
	)
	if resp.Message.Content != "echo: ping" || resp.Message.Role != "assistant" || resp.FinishReason != "stop" || resp.Model != "m" {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.ID == "" {
		t.Fatal("empty ID")
	}
}

func TestEchoUsage(t *testing.T) {
	u := chat(t, schema.Message{Role: "user", Content: "abcdefgh"}).Usage
	if u.InputTokens != 2 || u.OutputTokens != 4 {
		t.Fatalf("usage = %+v, want input 2 output 4", u)
	}
}

func TestEchoUsageCountsRunes(t *testing.T) {
	if u := chat(t, schema.Message{Role: "user", Content: "สวัสดี"}).Usage; u.InputTokens != 2 {
		t.Fatalf("input tokens = %d, want 2", u.InputTokens)
	}
}

func TestEchoNoUserMessage(t *testing.T) {
	if got := chat(t, schema.Message{Role: "system", Content: "x"}).Message.Content; got != "echo: " {
		t.Fatalf("content = %q", got)
	}
}

func TestEchoUnsupported(t *testing.T) {
	e := New("echo")
	if _, err := e.ChatStream(context.Background(), &schema.ChatRequest{}); !errors.Is(err, provider.ErrUnsupported) {
		t.Fatalf("ChatStream err = %v", err)
	}
	if _, err := e.Embed(context.Background(), &schema.EmbedRequest{}); !errors.Is(err, provider.ErrUnsupported) {
		t.Fatalf("Embed err = %v", err)
	}
	if e.Name() != "echo" || e.Dialect() != "none" {
		t.Fatalf("name %q dialect %q", e.Name(), e.Dialect())
	}
}
