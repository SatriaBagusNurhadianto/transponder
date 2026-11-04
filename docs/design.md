# transponder design

Status: draft for review
Date: 2026-11-04
Contract: `mothership/CONTRACT.md`

## 1. What it is

transponder is an LLM gateway in one Go binary. Clients talk to it with the OpenAI API (`/v1/chat/completions`, `/v1/embeddings`) or the Anthropic API (`/v1/messages`). It sends each call to Anthropic, OpenAI, Gemini or Ollama, falls back to the next provider when one fails, records what every call cost and who made it, and stops a user or team once their budget is spent.

It runs alone. In the mothership stack, rosetta agents and logbook call it for chat, embeddings and fact extraction, and the caller's token that airlock and rosetta forward tells transponder which user to charge.

## 2. Why build it next to bifrost and litellm

Research done on 2026-11-04.

- maximhq/bifrost: Go, Apache-2.0, about 9.4k stars, very active. 23+ providers, a web UI, semantic caching, virtual keys with budgets, fallbacks, an MCP gateway and an agent gateway. It is fast and broad, and transponder will not beat it on either.
- BerriAI/litellm: Python, about 63k stars, 100+ providers, the default choice for most teams.
- agentgateway (Linux Foundation, Rust): LLM, MCP and A2A gateway aimed at Kubernetes. Budgets are written as rate limit policies counted in tokens or micro-dollars.

What the open source editions leave out:

| | bifrost OSS | litellm OSS | agentgateway | transponder |
|---|---|---|---|---|
| OIDC / JWT callers | Enterprise (OIDC, SCIM, RBAC) | Enterprise ("JWT-based Auth requires a LiteLLM Enterprise license") | Yes | Yes |
| Budget per user taken from the token | No, budgets hang off virtual keys | Through JWT-to-virtual-key mapping, Enterprise | Yes, as a rate limit expression | Yes, in USD, from a claim |
| Several replicas sharing one budget | Not supported ("Running multiple OSS Bifrost nodes with a Postgres backend is not supported") | Yes, with Postgres and Redis | Needs a global rate limit service | Yes, with Postgres |
| Circuit breaker | Enterprise | Yes, deployment cooldowns | Not checked | Yes, simple cooldown |
| Runs as | npx, Docker, Go SDK | Python service, Postgres, Redis | Kubernetes first | One static binary |
| Size of one provider | bifrost's Anthropic package is over 1 MB of non-test Go source | Large Python modules | Rust | Target under 800 lines |

So transponder is for a team that already has an identity provider and wants to know, for free, what each person spends on models and cap it, on one or several replicas, with a YAML file and one binary. That is a real gap: both popular gateways charge for exactly this. It is also a narrow one. Provider breadth, a UI and raw throughput stay bifrost's. The README should say so plainly and point people to bifrost when breadth is what they need.

The second reason is the stack. A user who signs in once is the same user in rosetta, airlock, logbook and transponder, so an agent run, its tool calls, its memories and its model spend all line up under one identity and one `traceparent`. No other gateway gives that out of the box, and it is what makes the mothership demo work.

## 3. Goals and non-goals for v1

Goals:

- OpenAI-compatible `/v1/chat/completions` (streaming and not) and `/v1/embeddings`.
- Anthropic-compatible `/v1/messages` (streaming and not), good enough that Claude Code and the Anthropic SDKs work with only a base URL change.
- Four backends: Anthropic, OpenAI, Gemini, Ollama. Any OpenAI-compatible server (vLLM, LM Studio, Groq and so on) works through the OpenAI adapter with a `base_url`.
- Ordered fallback across providers, with a cooldown for targets that keep failing.
- Usage and cost per call, in the contract's response headers, in Prometheus metrics, and in a usage store.
- Budgets in USD per user, team or key, per day or month, enforced across replicas when the store is Postgres.
- Callers identified by OIDC JWT or static API key, as the contract requires.
- A demo anyone can run in five minutes.

Not in v1: web UI, semantic or exact caching, request-per-minute rate limits, guardrails, the OpenAI Responses API, batch, images, audio, weighted load balancing, Bedrock, Vertex and Azure, an admin API for managing keys and budgets at runtime. Section 13 lists the ones most likely to come next.

## 4. Architecture

```
client (OpenAI or Anthropic dialect)
   |
   v
[http]  auth -> request id / trace -> decode into dialect
   |
   v
[budget] check every budget that applies to the caller
   |
   v
[router] pick route by model name -> ordered targets
   |      for each target that is not cooling down:
   |        same dialect?  -> native call, body forwarded with model rewritten
   |        other dialect? -> translate through the canonical schema
   |      retryable failure -> next target
   v
[provider adapter] anthropic | openai | gemini | ollama
   |
   v
[usage] parse token counts -> price -> store record + add to spend -> headers, metrics, log
```

Packages:

```
cmd/transponder          main, flags, config load, server start
internal/config          YAML schema, env overrides, ${VAR} expansion, validation
internal/auth            OIDC JWT verification, static keys, Identity type
internal/server          HTTP mux, middleware, handlers for each endpoint
internal/schema          canonical request, response, stream event and usage types
internal/dialect/openai  decode and encode the OpenAI wire format, usage parsing
internal/dialect/anthropic  same for Anthropic
internal/router          routes, target order, fallback rules, cooldown
internal/provider        Provider interface, registry, shared HTTP helpers
internal/provider/anthropic
internal/provider/openai
internal/provider/gemini
internal/provider/ollama
internal/provider/echo   deterministic fake for tests and the mothership e2e test
internal/provider/providertest  conformance suite every adapter must pass
internal/pricing         embedded price table, overrides, cost math
internal/budget          which budgets apply, the check, the period math
internal/store           Store interface, sqlite and postgres implementations, migrations
internal/telemetry       slog JSON logger, Prometheus metrics, OTLP tracing
```

Everything sits under `internal/` in v1 so the interfaces can still change. Making `provider` public, so people can ship adapters out of tree or embed transponder as a library, is an open choice (section 13).

## 5. Provider adapters

This is the main place contributors add code, so the interface stays small.

```go
type Provider interface {
	Name() string
	Dialect() string
	Chat(ctx context.Context, req *schema.ChatRequest) (*schema.ChatResponse, error)
	ChatStream(ctx context.Context, req *schema.ChatRequest) (schema.Stream, error)
	Embed(ctx context.Context, req *schema.EmbedRequest) (*schema.EmbedResponse, error)
}

type Native interface {
	Native(ctx context.Context, call *NativeCall) (*http.Response, error)
}
```

- `Dialect()` returns `openai`, `anthropic` or `none`. When it matches the caller's dialect and the adapter implements `Native`, the router forwards the original body with only `model` rewritten. This keeps every field the gateway does not know about, such as Anthropic thinking blocks, `cache_control` and new beta fields, so Claude Code keeps working as Anthropic adds features.
- In every other case the router goes through `Chat`, `ChatStream` or `Embed` with canonical types.
- `Embed` returns `provider.ErrUnsupported` when a backend has no embeddings (Anthropic). The router treats that target as skipped.
- Errors carry the upstream status code and a retryable flag through a shared `provider.Error` type.

The four adapters:

- anthropic: Messages API. Native for Anthropic callers, translated for OpenAI callers. No embeddings.
- openai: Chat Completions and Embeddings. Native for OpenAI callers. Takes `base_url`, so it also covers any OpenAI-compatible server.
- gemini: `generateContent`, `streamGenerateContent` and `embedContent` on the Generative Language API with an API key. Always translated.
- ollama: a preset over the openai adapter pointing at `http://localhost:11434/v1`, no key, zero price by default. Its own package so Ollama-specific behaviour (model pull hints, `keep_alive`) has a home.

Adding a provider means one package with an adapter, recorded fixtures in `testdata/`, a line in the registry, and prices in the price table. `providertest.Run(t, factory)` replays the fixtures through a local `httptest` server and checks text, tool calls, streaming order, usage parsing and error mapping. The CONTRIBUTING guide walks through it with the ollama package as the short example.

## 6. Canonical schema and translation

The canonical types follow the OpenAI Chat Completions shape, since most people already know it: messages with roles, content parts (text, image as URL or base64), tools, tool calls and tool results, `max_tokens`, `temperature`, `top_p`, `stop`, a response format, and `stream`. Usage is:

```go
type Usage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
}
```

Stream events are a small set: message start, text delta, tool call start, tool call argument delta, finish with reason, usage, error. Each dialect package encodes these as its own SSE format, so an Anthropic caller whose request lands on OpenAI still gets `message_start`, `content_block_delta` and `message_stop` events.

Rules for translation:

- Fields with an equivalent are mapped (system prompt, tools, tool choice, stop sequences, images, finish reasons).
- Optional tuning fields with no equivalent, such as Anthropic `thinking` or OpenAI `logprobs`, are dropped and the drop is logged at debug level.
- Content the target cannot represent at all, such as a PDF document block going to Ollama, makes that target incompatible. The router skips it and tries the next one. If none are left the caller gets a 400 that names the problem.
- Tool call ids are kept across the translation so multi-turn tool use works after a fallback.

## 7. Routing and fallback

Config:

```yaml
providers:
  - name: anthropic
    type: anthropic
    api_key: ${ANTHROPIC_API_KEY}
  - name: openai
    type: openai
    api_key: ${OPENAI_API_KEY}
  - name: gemini
    type: gemini
    api_key: ${GEMINI_API_KEY}
  - name: local
    type: ollama

routes:
  - match: "claude-*"
    targets:
      - provider: anthropic
      - provider: openai
        model: gpt-5
      - provider: local
        model: qwen3:8b
  - match: "fast"
    targets:
      - provider: gemini
        model: gemini-2.5-flash
      - provider: local
        model: llama3.2
  - match: "text-embedding-3-small"
    targets:
      - provider: openai
      - provider: local
        model: nomic-embed-text

allow_direct: true
```

- The request's `model` is matched against routes in order. `match` is an exact name or a glob. The first route that matches wins.
- A target without `model` keeps the model the caller asked for.
- With `allow_direct`, `provider/model` (for example `local/llama3.2`) goes straight to that provider with no fallback.
- No match gives 404 `model_not_found`.

A target is retried on the next one when the call fails with a connection error, a timeout, 408, 429, any 5xx, Anthropic's 529, or 401 and 403 from the upstream (that means transponder's provider key is bad, which is not the caller's fault). It is not retried on other 4xx: a bad request stays bad on every provider.

Cooldown: after 3 failures in a row a target is skipped for 30 seconds. Both numbers are configurable. A target in cooldown is still tried when it is the only one left. There is no retry of the same target in v1; fallback is the retry.

Streaming: transponder reads the upstream until its first event before writing anything to the client. A failure before that point falls back like any other call. After the first byte is sent the target is committed, and a later failure ends the stream with an error event in the caller's dialect.

Each attempt has a timeout (`attempt_timeout`, default 60s until first byte) and the whole request has `request_timeout` (default 10m, since long generations are normal).

## 8. Identity

Every request needs a caller, as the contract says:

- `Authorization: Bearer <token>`. If the token is a JWT it is verified against `auth.oidc.issuer` (JWKS fetched and cached through `coreos/go-oidc`), audience checked. Otherwise it is compared in constant time against the static keys.
- `x-api-key: <token>` is accepted the same way, because Anthropic clients send that header when given an API key.

```yaml
auth:
  oidc:
    issuer: https://idp.example.com
    audience: transponder
    user_claim: sub
    teams_claim: groups
  keys:
    - name: logbook
      key: ${LOGBOOK_KEY}
      teams: [platform]
    - name: ops-admin
      key: ${ADMIN_KEY}
      admin: true
```

The result is an `Identity{User, Teams, Key, Admin}`. For a static key, `User` is the key name. With no `auth` block transponder refuses to start unless `auth.allow_anonymous: true` is set, and then every caller is user `anonymous`. The caller's token is never sent to a provider. Providers get transponder's own provider keys, plus `traceparent`, `tracestate` and `X-Request-ID`.

## 9. Usage, cost and budgets

Usage. Token counts come from the provider's response: the `usage` object for non-streaming calls, and the usage events while streaming. For OpenAI-dialect streams transponder adds `stream_options.include_usage` upstream, and drops the usage-only chunk again if the caller did not ask for it. When a provider returns no usage (some OpenAI-compatible servers), the record is marked `estimated` and counts are approximated from character length divided by 4.

Price. `internal/pricing/prices.yaml` is embedded in the binary, keyed by `type/model`, with USD per million input, output, cache read and cache write tokens. Config can override or add entries. Ollama and `echo` cost zero. A model with no price is served and costs zero, the log line says `unpriced`, and `transponder_unpriced_requests_total` goes up. Money is kept as int64 nano-USD so sums never drift.

Headers on every response:

```
X-Request-ID: <kept or generated>
X-Usage-Input-Tokens: 1234
X-Usage-Output-Tokens: 56
X-Usage-Cost-USD: 0.004542
X-Transponder-Provider: openai
X-Transponder-Model: gpt-5
X-Transponder-Fallbacks: 1
```

For streams, headers go out before the counts are known. transponder declares `Trailer: X-Usage-Input-Tokens, X-Usage-Output-Tokens, X-Usage-Cost-USD` and sends the values as HTTP trailers, and the counts also appear in the stream's normal usage event. This is a clarification the contract needs (section 12).

Budgets:

```yaml
budgets:
  - subject: "user:*"
    period: monthly
    limit_usd: 20
  - subject: "user:alice@example.com"
    period: monthly
    limit_usd: 100
  - subject: "team:platform"
    period: daily
    limit_usd: 50
  - subject: "key:logbook"
    period: monthly
    limit_usd: 10
```

- Subjects are `user:<id>`, `team:<name>` or `key:<name>`. `user:*` is the default for any user without a rule of their own; the exact user rule replaces it.
- Periods are `daily` or `monthly` on UTC calendar boundaries.
- Before a call, every budget that applies (the caller's user rule, each of their teams, their key) must have room. If one is spent the caller gets HTTP 402 with error type `budget_exceeded`, the subject, the limit and the reset time, in the caller's dialect. 402 is used on purpose: the OpenAI and Anthropic SDKs retry 429 automatically, which would only burn time.
- After the call, the real cost is added to every subject's spend in the same transaction as the usage record. Requests already in flight can push a subject a little past its limit. The overshoot is at most the cost of those requests, and the docs say so.
- `GET /v1/usage` returns the caller's spend and remaining budget for each applicable rule. Admin callers can pass `?user=` or `?team=` and `from`/`to` for totals by model.

## 10. Store

```go
type Store interface {
	Record(ctx context.Context, rec UsageRecord, subjects []Subject) error
	Spend(ctx context.Context, subjects []Subject, at time.Time) (map[Subject]int64, error)
	Usage(ctx context.Context, q UsageQuery) ([]UsageTotal, error)
	Ping(ctx context.Context) error
	Close() error
}
```

Two implementations behind the same SQL:

- SQLite through `modernc.org/sqlite` (pure Go, so the static binary needs no cgo). Default: `store: {sqlite: ./transponder.db}`. One replica.
- Postgres through `pgx/v5`. Several replicas share spend, and budgets hold across all of them.

Tables:

- `usage`: id, time, request_id, user, teams, key, route, provider, model, endpoint, input, output, cache_read, cache_write, cost_nusd, estimated, status, latency_ms, fallbacks.
- `spend`: subject, period, period_start, cost_nusd, primary key on the first three. Updated with `INSERT ... ON CONFLICT DO UPDATE SET cost_nusd = spend.cost_nusd + excluded.cost_nusd`, which both databases support, so concurrent replicas never lose an update.

Spend rows are kept for every user, team and key, budgeted or not, so adding a budget later starts from real numbers. Migrations are embedded SQL files run at start. `usage` rows older than `retention_days` (default 90) are deleted once a day; `spend` rows are kept.

## 11. Operations

- `transponder --config transponder.yaml`. Any key can be overridden by environment variable, nested keys joined by `_`: `TRANSPONDER_LISTEN_ADDR`, `TRANSPONDER_STORE_POSTGRES`. Lists such as providers and routes come from the file; secrets go in with `${VAR}`.
- `/healthz` is always 200 while the process runs. `/readyz` checks the store and, when OIDC is configured, that the JWKS has been fetched. Providers are left out of readiness on purpose: one provider down is what fallback is for.
- `/metrics`: `transponder_requests_total{endpoint,route,provider,model,status}`, `transponder_tokens_total{direction,provider,model}`, `transponder_cost_usd_total{provider,model}`, `transponder_upstream_duration_seconds{provider,model}`, `transponder_fallbacks_total{route,from,to,reason}`, `transponder_budget_rejections_total{subject_kind}`, `transponder_unpriced_requests_total`. No user label, to keep cardinality bounded; per-user numbers live in the store.
- Tracing over OTLP when `otel.endpoint` is set: one span per request, one child span per upstream attempt, with provider, model, tokens and cost as attributes.
- Logs: JSON on stdout through `log/slog`, one line per request with id, user, route, provider, model, tokens, cost, status, latency and fallbacks. Prompts and completions are never logged.
- `GET /v1/models` lists the literal route names and direct `provider/model` entries, so tools such as Open WebUI can show a model picker.
- `POST /v1/messages/count_tokens` is forwarded to the first Anthropic target of the route, and returns 404 when the route has none.
- Releases: GoReleaser builds static binaries for Linux, macOS and Windows on amd64 and arm64, and a distroless container image.

## 12. Contract fit

| Contract item | transponder |
|---|---|
| `/v1/chat/completions`, `/v1/embeddings`, `/v1/messages` | Yes |
| Bearer token, OIDC JWT or static key | Yes, plus `x-api-key` for Anthropic clients |
| Forward caller token unchanged | transponder is the last hop. It never forwards the caller's token to a provider, since providers need their own keys |
| `traceparent`, `tracestate`, `X-Request-ID` | Kept, generated when missing, sent upstream |
| `X-Usage-Input-Tokens`, `X-Usage-Output-Tokens`, `X-Usage-Cost-USD` | Headers on normal responses, trailers on streams |
| YAML via `--config`, env overrides | `TRANSPONDER_` prefix |
| `/healthz`, `/readyz`, `/metrics`, OTLP, JSON logs | Yes |
| `github.com/raven-clown/transponder`, Apache-2.0, Go 1.26+ | `go 1.26` in go.mod |

Contract changes to agree:

1. Streaming usage. The contract puts usage in response headers, which a streaming response sends before any tokens exist. Proposal: for streamed responses the same three names are sent as HTTP trailers, and clients that cannot read trailers take the counts from the stream's own usage event. rosetta and logbook should read trailers when they stream.
2. `x-api-key`. Accepting it next to `Authorization: Bearer` is an addition only transponder needs; worth one line in the contract so airlock does not copy it by accident.
3. The `echo` provider. The mothership e2e test can run transponder with `type: echo` and no real keys, so CI needs no secrets. Worth naming in the contract's e2e section.

## 13. Decisions made, and open choices

Decided here, with the reason:

- Position: per-user USD budgets from OIDC and multi-replica budgets, free. Agreed in chat on 2026-11-04.
- Canonical schema shaped like OpenAI Chat Completions, plus native passthrough when caller and provider speak the same dialect. Passthrough keeps Claude Code working as Anthropic adds fields; translation covers fallback across dialects.
- Ollama through its OpenAI-compatible endpoint. Less code, and the same adapter serves vLLM and friends.
- Fallback is ordered and has no same-target retry. Easier to explain and test than weighted balancing.
- Fall back on upstream 401 and 403. Those are gateway key problems, and falling back is what the demo shows.
- Budget errors use 402 so SDKs do not retry.
- Prices embedded and overridable; unpriced models allowed at zero cost with a metric.
- Money in int64 nano-USD.
- SQLite by default, Postgres for replicas, same SQL.
- No auth means refuse to start, unless anonymous is switched on.
- Everything under `internal/` for v1.
- Gemini through the API key endpoint only; Vertex later.

Open, for the user to decide (my lean in brackets):

1. Make `provider` a public package in v1 so adapters can live out of tree and transponder can be embedded as a Go library. [Later. Keep it internal until the interface has had real contributors.]
2. Unpriced models when a budget applies: serve at zero cost, or reject. [Serve, warn, count. Rejecting breaks Ollama users who forget to add a zero price.]
3. Budget status code: 402 or 429. [402.]
4. Next features after v1, in order. [OpenAI Responses API (Codex CLI uses it), request-per-minute limits per user, an admin API for keys and budgets, Bedrock and Vertex adapters, exact-match caching.]
5. Whether to ship a small read-only usage page at `/ui`. [No in v1. `curl /v1/usage` and a Grafana dashboard JSON in `examples/` are enough for the demo.]

## 14. Testing

Test-driven throughout. No `-race` on the Windows dev machine (no gcc); CI on Linux runs `go test -race ./...`.

- dialect packages: golden JSON and SSE fixtures for decode, encode and usage parsing in both directions.
- provider packages: `providertest` conformance against `httptest` servers that replay recorded provider responses. No network in unit tests.
- router: fake providers that fail on cue, covering every fallback trigger, the non-retry cases, cooldown, and the first-byte rule for streams.
- budget and store: SQLite in a temp dir always; Postgres when `TRANSPONDER_TEST_POSTGRES` holds a DSN, which CI sets with a service container. A concurrency test fires parallel `Record` calls and checks the spend total is exact.
- server: end-to-end through `httptest.Server` with the echo provider: auth, headers, trailers, budgets, `/v1/usage`, health endpoints.
- Live smoke tests behind a build tag, run by hand with real keys before a release.

## 15. Demo

`examples/demo/` holds a compose file with transponder and Ollama, and a config with two static keys, `alice` and `bob`, where bob has a $0.01 monthly budget.

1. Point Claude Code at transponder: `ANTHROPIC_BASE_URL=http://localhost:8080 ANTHROPIC_AUTH_TOKEN=<alice key> claude`. It works unchanged, and every reply has cost headers.
2. Break the Anthropic key in the config and restart transponder. The same session keeps answering from OpenAI or the local Ollama model, and `X-Transponder-Fallbacks: 1` shows it.
3. Run the OpenAI Python SDK as bob until the budget runs out and see the 402 with the reset time.
4. `curl /v1/usage` as alice and as the admin key to see spend per user and model.
5. Run two transponder replicas on one Postgres and show bob's budget holding across both.

## 16. Milestones

1. Skeleton: config, auth, store on SQLite, the echo provider, `/v1/chat/completions` end to end with usage headers.
2. OpenAI and Ollama adapters, native passthrough, streaming with trailers, embeddings.
3. Anthropic dialect and adapter, translation both ways, Claude Code working.
4. Router with fallback and cooldown, Gemini adapter.
5. Budgets, `/v1/usage`, Postgres store.
6. Metrics, tracing, demo, README, CONTRIBUTING with the adapter guide, GoReleaser, first release.

The implementation plan breaks each milestone into test-first steps.
