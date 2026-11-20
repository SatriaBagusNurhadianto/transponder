// Package siliconflow configures the OpenAI adapter for SiliconFlow.
package siliconflow

import (
	"net/http"

	"github.com/raven-clown/transponder/internal/provider/openai"
)

// DefaultBaseURL is SiliconFlow's OpenAI-compatible API, from
// https://docs.siliconflow.com/en/userguide/quickstart. The China host is https://api.siliconflow.cn/v1.
const DefaultBaseURL = "https://api.siliconflow.com/v1"

// New returns an adapter for SiliconFlow. An empty baseURL means DefaultBaseURL,
// and an empty apiKey sends no Authorization header.
func New(name, baseURL, apiKey string, client *http.Client) *openai.Adapter {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return openai.New(openai.Options{Name: name, BaseURL: baseURL, APIKey: apiKey, Client: client})
}
