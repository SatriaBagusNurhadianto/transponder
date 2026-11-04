# Changelog

All notable changes are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- `POST /v1/chat/completions` answered by a built-in `echo` provider, with `X-Usage-Input-Tokens`, `X-Usage-Output-Tokens` and `X-Usage-Cost-USD` headers.
- API keys checked from `Authorization: Bearer` or `x-api-key`, and a one-time admin key printed to stderr on first start.
- SQLite storage with migrations that run at startup, and a usage record for every call.
- Bootstrap config from `--config` YAML with `TRANSPONDER_` environment overrides.
