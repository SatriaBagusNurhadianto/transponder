// Package cerebras configures the OpenAI adapter for Cerebras.
package cerebras

import (
	"net/http"

	"github.com/raven-clown/transponder/internal/provider/openai"
)

// DefaultBaseURL is Cerebras's OpenAI-compatible API, from
// https://inference-docs.cerebras.ai/resources/openai.
const DefaultBaseURL = "https://api.cerebras.ai/v1"

// New returns an adapter for Cerebras. An empty baseURL means DefaultBaseURL,
// and an empty apiKey sends no Authorization header.
func New(name, baseURL, apiKey string, client *http.Client) *openai.Adapter {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return openai.New(openai.Options{Name: name, BaseURL: baseURL, APIKey: apiKey, Client: client})
}
