// Package openai reads and writes the OpenAI Chat Completions wire format.
package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/raven-clown/transponder/internal/schema"
)

// RequestError is a problem with the caller's request, reported back as a 400.
type RequestError struct {
	Param   string
	Code    string
	Message string
}

func (e *RequestError) Error() string {
	if e.Param == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Param, e.Message)
}

type wireMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type wireRequest struct {
	Model       string        `json:"model"`
	Messages    []wireMessage `json:"messages"`
	MaxTokens   *int          `json:"max_tokens"`
	Temperature *float64      `json:"temperature"`
	Stream      bool          `json:"stream"`
}

// DecodeChatRequest parses an OpenAI chat completion request.
func DecodeChatRequest(r io.Reader) (*schema.ChatRequest, error) {
	var w wireRequest
	if err := json.NewDecoder(r).Decode(&w); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, err
		}
		return nil, &RequestError{Code: "invalid_json", Message: "the request body is not valid JSON: " + err.Error()}
	}
	if w.Model == "" {
		return nil, &RequestError{Param: "model", Code: "missing_field", Message: "model is required"}
	}
	if len(w.Messages) == 0 {
		return nil, &RequestError{Param: "messages", Code: "missing_field", Message: "messages must not be empty"}
	}
	req := &schema.ChatRequest{Model: w.Model, MaxTokens: w.MaxTokens, Temperature: w.Temperature, Stream: w.Stream}
	for _, m := range w.Messages {
		content := ""
		raw := bytes.TrimSpace(m.Content)
		if len(raw) > 0 && !bytes.Equal(raw, []byte("null")) {
			// Try a plain string first.
			if err := json.Unmarshal(raw, &content); err != nil {
				// Otherwise accept an array of text parts.
				var parts []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}
				if err2 := json.Unmarshal(raw, &parts); err2 != nil {
					return nil, &RequestError{Param: "messages", Code: "unsupported", Message: "only string message content is supported so far"}
				}
				for _, p := range parts {
					if p.Type != "text" {
						return nil, &RequestError{Param: "messages", Code: "unsupported", Message: "only text content parts are supported so far"}
					}
					content += p.Text
				}
			}
		}
		req.Messages = append(req.Messages, schema.Message{Role: m.Role, Content: content})
	}
	return req, nil
}

type wireResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []wireChoice `json:"choices"`
	Usage   wireUsage    `json:"usage"`
}

type wireChoice struct {
	Index        int        `json:"index"`
	Message      wireOutMsg `json:"message"`
	FinishReason string     `json:"finish_reason"`
}

type wireOutMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type wireUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

// EncodeChatResponse renders a finished chat call as an OpenAI chat completion.
func EncodeChatResponse(resp *schema.ChatResponse, created time.Time) ([]byte, error) {
	return json.Marshal(wireResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: created.Unix(),
		Model:   resp.Model,
		Choices: []wireChoice{{
			Message:      wireOutMsg{Role: resp.Message.Role, Content: resp.Message.Content},
			FinishReason: resp.FinishReason,
		}},
		Usage: wireUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	})
}
