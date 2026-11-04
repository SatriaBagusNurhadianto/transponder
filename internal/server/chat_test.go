package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/raven-clown/transponder/internal/auth"
	"github.com/raven-clown/transponder/internal/provider"
	"github.com/raven-clown/transponder/internal/provider/echo"
	"github.com/raven-clown/transponder/internal/schema"
	"github.com/raven-clown/transponder/internal/server"
	"github.com/raven-clown/transponder/internal/store"
	"github.com/raven-clown/transponder/internal/store/sqlite"
)

type env struct {
	srv *httptest.Server
	db  *sqlite.DB
	key string
}

type result struct {
	status int
	header http.Header
	body   []byte
}

func setup(t *testing.T, p provider.Provider) *env {
	t.Helper()
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	plain, hash, last4, err := auth.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	k := store.APIKey{OrgID: store.DefaultOrg, Name: "alice", Hash: hash, Last4: last4, Role: string(auth.RoleMember), CreatedAt: time.Now()}
	if err := db.CreateKey(ctx, k); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.New(server.Deps{
		Auth:     auth.NewAuthenticator(db, time.Now),
		Provider: p,
		Usage:    db,
		Logger:   slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Now:      time.Now,
	}))
	t.Cleanup(srv.Close)
	return &env{srv: srv, db: db, key: plain}
}

func (e *env) do(method string, headers map[string]string, body string) (result, error) {
	req, err := http.NewRequestWithContext(context.Background(), method, e.srv.URL+"/v1/chat/completions", strings.NewReader(body))
	if err != nil {
		return result{}, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		return result{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	return result{status: resp.StatusCode, header: resp.Header, body: b}, err
}

func (e *env) post(t *testing.T, body string) result {
	t.Helper()
	res, err := e.do(http.MethodPost, map[string]string{"Authorization": "Bearer " + e.key}, body)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (e *env) usage(t *testing.T) []store.UsageRecord {
	t.Helper()
	recs, err := e.db.RecentUsage(context.Background(), store.DefaultOrg, 100)
	if err != nil {
		t.Fatal(err)
	}
	return recs
}

func errorCode(t *testing.T, res result) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(res.body, &body); err != nil {
		t.Fatalf("decoding %q: %v", res.body, err)
	}
	return body.Error.Code
}

const ping = `{"model":"m","messages":[{"role":"user","content":"ping"}]}`

func TestChatCompletions(t *testing.T) {
	e := setup(t, echo.New("echo"))
	res := e.post(t, ping)
	if res.status != http.StatusOK {
		t.Fatalf("status = %d", res.status)
	}
	var body struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(res.body, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Choices) != 1 || body.Choices[0].Message.Content != "echo: ping" {
		t.Fatalf("body = %s", res.body)
	}
	h := res.header
	if h.Get("X-Usage-Input-Tokens") != "1" || h.Get("X-Usage-Output-Tokens") != "3" || h.Get("X-Usage-Cost-USD") != "0.000000" {
		t.Fatalf("usage headers = %v", h)
	}
	if ct := h.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content type = %q", ct)
	}
	recs := e.usage(t)
	if len(recs) != 1 {
		t.Fatalf("records = %d", len(recs))
	}
	r := recs[0]
	if r.User != "alice" || r.Key != "alice" || r.Provider != "echo" || r.Endpoint != "chat.completions" ||
		r.Status != 200 || r.InputTokens != 1 || r.OutputTokens != 3 || r.Model != "m" {
		t.Fatalf("record = %+v", r)
	}
}

func TestChatCompletionsKeepsRequestID(t *testing.T) {
	e := setup(t, echo.New("echo"))
	if _, err := e.do(http.MethodPost, map[string]string{"Authorization": "Bearer " + e.key, "X-Request-ID": "abc-123"}, ping); err != nil {
		t.Fatal(err)
	}
	if got := e.usage(t)[0].RequestID; got != "abc-123" {
		t.Fatalf("request id = %q", got)
	}
}

func TestChatCompletionsUnauthenticated(t *testing.T) {
	e := setup(t, echo.New("echo"))
	res, err := e.do(http.MethodPost, nil, ping)
	if err != nil {
		t.Fatal(err)
	}
	if res.status != http.StatusUnauthorized {
		t.Fatalf("status = %d", res.status)
	}
	if code := errorCode(t, res); code != "invalid_api_key" {
		t.Fatalf("code = %q", code)
	}
	if n := len(e.usage(t)); n != 0 {
		t.Fatalf("records = %d", n)
	}
}

func TestChatCompletionsBadRequest(t *testing.T) {
	tests := []struct{ name, body, code string }{
		{name: "bad json", body: `{`, code: "invalid_json"},
		{name: "missing model", body: `{"messages":[{"role":"user","content":"x"}]}`, code: "missing_field"},
		{name: "streaming", body: `{"model":"m","stream":true,"messages":[{"role":"user","content":"x"}]}`, code: "unsupported"},
	}
	e := setup(t, echo.New("echo"))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := e.post(t, tt.body)
			if res.status != http.StatusBadRequest {
				t.Fatalf("status = %d", res.status)
			}
			if code := errorCode(t, res); code != tt.code {
				t.Fatalf("code = %q, want %q", code, tt.code)
			}
		})
	}
}

func TestChatCompletionsBodyTooLarge(t *testing.T) {
	e := setup(t, echo.New("echo"))
	big := `{"model":"m","messages":[{"role":"user","content":"` + strings.Repeat("a", 20<<20) + `"}]}`
	if res := e.post(t, big); res.status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d", res.status)
	}
}

func TestChatCompletionsConcurrent(t *testing.T) {
	e := setup(t, echo.New("echo"))
	var wg sync.WaitGroup
	codes := make(chan int, 20)
	for range 20 {
		wg.Go(func() {
			res, err := e.do(http.MethodPost, map[string]string{"Authorization": "Bearer " + e.key}, ping)
			if err != nil {
				codes <- 0
				return
			}
			codes <- res.status
		})
	}
	wg.Wait()
	close(codes)
	for c := range codes {
		if c != http.StatusOK {
			t.Fatalf("status = %d", c)
		}
	}
	if n := len(e.usage(t)); n != 20 {
		t.Fatalf("records = %d, want 20", n)
	}
}

func TestChatCompletionsWrongMethod(t *testing.T) {
	e := setup(t, echo.New("echo"))
	res, err := e.do(http.MethodGet, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.status != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", res.status)
	}
}

type failing struct{ *echo.Echo }

func (failing) Chat(context.Context, *schema.ChatRequest) (*schema.ChatResponse, error) {
	return nil, errors.New("connection refused")
}

func TestChatCompletionsProviderFails(t *testing.T) {
	e := setup(t, failing{echo.New("down")})
	res := e.post(t, ping)
	if res.status != http.StatusBadGateway {
		t.Fatalf("status = %d", res.status)
	}
	if code := errorCode(t, res); code != "upstream_error" {
		t.Fatalf("code = %q", code)
	}
	recs := e.usage(t)
	if len(recs) != 1 || recs[0].Status != http.StatusBadGateway || recs[0].Provider != "down" {
		t.Fatalf("records = %+v", recs)
	}
}
