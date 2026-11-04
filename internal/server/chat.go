package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/raven-clown/transponder/internal/auth"
	"github.com/raven-clown/transponder/internal/dialect/openai"
	"github.com/raven-clown/transponder/internal/store"
)

type chatHandler struct {
	d Deps
}

func (h *chatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := h.d.Now()
	id, err := h.d.Auth.Authenticate(r)
	if err != nil {
		if !errors.Is(err, auth.ErrUnauthenticated) {
			h.d.Logger.ErrorContext(r.Context(), "authenticating", "err", err)
			openai.WriteError(w, http.StatusInternalServerError, "api_error", "", "", "could not check the API key")
			return
		}
		openai.WriteError(w, http.StatusUnauthorized, "invalid_request_error", "invalid_api_key", "", "a valid API key is required")
		return
	}

	req, err := openai.DecodeChatRequest(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		var bad *openai.RequestError
		switch {
		case errors.As(err, &tooLarge):
			openai.WriteError(w, http.StatusRequestEntityTooLarge, "invalid_request_error", "body_too_large", "", "the request body is larger than 20 MiB")
		case errors.As(err, &bad):
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", bad.Code, bad.Param, bad.Message)
		default:
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "", "", err.Error())
		}
		return
	}
	if req.Stream {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "unsupported", "stream", "streaming is not supported yet")
		return
	}

	rec := store.UsageRecord{
		OrgID:     id.OrgID,
		Time:      start,
		RequestID: r.Header.Get("X-Request-ID"),
		User:      id.User,
		Teams:     id.Teams,
		Key:       id.Key,
		Project:   r.Header.Get("X-Project"),
		Provider:  h.d.Provider.Name(),
		Model:     req.Model,
		Endpoint:  "chat.completions",
	}

	resp, err := h.d.Provider.Chat(r.Context(), req)
	if err != nil {
		rec.Status = http.StatusBadGateway
		h.record(r.Context(), rec)
		h.d.Logger.WarnContext(r.Context(), "provider call failed", "provider", rec.Provider, "err", err)
		openai.WriteError(w, http.StatusBadGateway, "api_error", "upstream_error", "", "the provider did not answer")
		return
	}

	body, err := openai.EncodeChatResponse(resp, start)
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "api_error", "", "", "could not encode the response")
		return
	}
	rec.Model = resp.Model
	rec.InputTokens = resp.Usage.InputTokens
	rec.OutputTokens = resp.Usage.OutputTokens
	rec.CacheReadTokens = resp.Usage.CacheReadTokens
	rec.CacheWriteTokens = resp.Usage.CacheWriteTokens
	rec.Status = http.StatusOK
	h.record(r.Context(), rec)

	setUsageHeaders(w.Header(), resp.Usage, rec.CostNUSD)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (h *chatHandler) record(ctx context.Context, rec store.UsageRecord) {
	rec.Latency = h.d.Now().Sub(rec.Time)
	if err := h.d.Usage.RecordUsage(context.WithoutCancel(ctx), rec); err != nil {
		h.d.Logger.ErrorContext(ctx, "recording usage", "request_id", rec.RequestID, "err", err)
	}
}
