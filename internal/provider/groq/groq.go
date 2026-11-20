// Package groq configures the OpenAI adapter for Groq.
package groq

import (
	"net/http"

	"github.com/raven-clown/transponder/internal/provider/openai"
)

// DefaultBaseURL is Groq's OpenAI-compatible API, from
// https://console.groq.com/docs/openai.
const DefaultBaseURL = "https://api.groq.com/openai/v1"

// New returns an adapter for Groq. An empty baseURL means DefaultBaseURL,
// and an empty apiKey sends no Authorization header.
func New(name, baseURL, apiKey string, client *http.Client) *openai.Adapter {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return openai.New(openai.Options{Name: name, BaseURL: baseURL, APIKey: apiKey, Client: client})
}
