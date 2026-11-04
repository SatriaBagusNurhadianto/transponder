// Package server serves the gateway endpoints.
package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/raven-clown/transponder/internal/auth"
	"github.com/raven-clown/transponder/internal/provider"
	"github.com/raven-clown/transponder/internal/store"
)

const maxBodyBytes = 20 << 20

// Deps are what the server needs to answer requests.
type Deps struct {
	Auth     *auth.Authenticator
	Provider provider.Provider
	Usage    store.UsageStore
	Logger   *slog.Logger
	Now      func() time.Time
}

// New returns the gateway handler.
func New(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /v1/chat/completions", &chatHandler{d: d})
	return mux
}
