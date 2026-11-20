// Package mistral configures the OpenAI adapter for Mistral AI.
package mistral

import (
	"net/http"

	"github.com/raven-clown/transponder/internal/provider/openai"
)

// DefaultBaseURL is Mistral AI's OpenAI-compatible API, from
// https://docs.mistral.ai/api/. Chat and embeddings share this base URL.
const DefaultBaseURL = "https://api.mistral.ai/v1"

// New returns an adapter for Mistral AI. An empty baseURL means DefaultBaseURL,
// and an empty apiKey sends no Authorization header.
func New(name, baseURL, apiKey string, client *http.Client) *openai.Adapter {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return openai.New(openai.Options{Name: name, BaseURL: baseURL, APIKey: apiKey, Client: client})
}
