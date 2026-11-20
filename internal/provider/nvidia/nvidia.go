// Package nvidia configures the OpenAI adapter for NVIDIA NIM.
package nvidia

import (
	"net/http"

	"github.com/raven-clown/transponder/internal/provider/openai"
)

// DefaultBaseURL is NVIDIA NIM's OpenAI-compatible API, from
// https://docs.api.nvidia.com/nim/reference/llm-apis. A self-hosted NIM is usually http://localhost:8000/v1.
const DefaultBaseURL = "https://integrate.api.nvidia.com/v1"

// New returns an adapter for NVIDIA NIM. An empty baseURL means DefaultBaseURL,
// and an empty apiKey sends no Authorization header.
func New(name, baseURL, apiKey string, client *http.Client) *openai.Adapter {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return openai.New(openai.Options{Name: name, BaseURL: baseURL, APIKey: apiKey, Client: client})
}
