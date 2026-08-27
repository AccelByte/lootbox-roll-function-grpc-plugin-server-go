# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

An AccelByte `Extend Override` app template: a gRPC server that AGS (AccelByte Gaming Services) calls
*instead of* its built-in loot box roll logic. AGS is the gRPC **client**; this repo is the **server**.
The contract is fixed by `pkg/proto/lootbox.proto` (owned by AGS) — the only method is
`LootBox/RollLootBoxRewards`. Customizing this template means replacing the roll logic in
`pkg/service/lootbox_service.go`, not changing the proto.

## Commands

```shell
make build                  # go build inside a golang:1.24 container (native `go build` if REMOTE_CONTAINERS=true)
make proto                  # regenerate pkg/pb from pkg/proto via a proto-builder image (or ./proto.sh in devcontainer)
docker compose up --build   # build + run the server (gRPC :6565, Prometheus /metrics :8080)
go run main.go              # run directly; needs the .env vars exported first
golangci-lint run           # config in .golangci.yml
```

There are no tests in this repo. Testing is manual, via the Postman gRPC collection in
`demo/lootbox-roll-function-demo.postman_collection.json` — see README "Testing" for the
sample request/response and the ngrok/pinggy tunnel setup for testing against a live AGS environment.

Deployment uses `extend-helper-cli` (`image-upload`, `deploy-app`, `start-app`); `.vscode/tasks.json`
wraps each of those as a VS Code task.

## Architecture

`main.go` wires everything and is where cross-cutting concerns live:

- **Interceptor chain** (order matters): Prometheus metrics → structured logging (slog JSON, trace ID
  pulled from the span context) → auth. Auth interceptors are appended *only* when
  `PLUGIN_GRPC_SERVER_AUTH_ENABLED` is true (the default).
- **Auth** (`pkg/common/auth_interceptor.go`): validates the AGS access token in the `authorization`
  metadata against `AB_NAMESPACE`, using the accelbyte-go-sdk `TokenValidator` with local JWT
  validation and a background JWKS/revocation refresh (`REFRESH_INTERVAL`, default 600s).
  gRPC reflection and health-check methods bypass it.
- **Auth startup is retried and health-gated** (`pkg/common/validator.go`) — do not "simplify" this
  back into a single inline `Initialize` call. The pinned SDK's `TokenValidator.Initialize` starts its
  refresh goroutines only after a successful fetch, so one failed attempt would leave the process with
  no public keys and nothing scheduled to refill them, rejecting every token for its whole lifetime.
  Therefore: `main.go` validates and echoes the required env vars, listens and serves *before*
  initializing the validator (probes must be answerable), keeps both health statuses at `NOT_SERVING`
  until `InitializeValidatorWithRetry` succeeds, and the interceptor returns `Unavailable` — not
  `PermissionDenied` — while `common.IsValidatorReady()` is false. `PLUGIN_GRPC_SERVER_AUTH_ENABLED=false`
  marks ready immediately and logs at Warn. Background: `../EXTEND-AUTH-INIT-FIX.md`.
- **Tracing** (`pkg/common/tracer_provider.go`): OTel with a **Zipkin** exporter
  (`OTEL_EXPORTER_ZIPKIN_ENDPOINT`), B3 multi-header + W3C propagators. `http.DefaultTransport` is
  replaced globally with a tracing round tripper so outgoing AGS SDK calls are traced too.
- Service name is `extend-app-lootbox-roll`, or `extend-app-lr-<OTEL_SERVICE_NAME>` when that env var is set.

`pkg/pb/` is **generated code** — checked into git, but never edit it by hand; change the proto and
run `make proto`. The Dockerfile regenerates it in its own `proto-builder` stage, so the committed
files and the built image can diverge if the proto changes without regeneration.

## Conventions

- Every `.go`, `Makefile`, and `Dockerfile` starts with the AccelByte copyright header — `goheader`
  in `.golangci.yml` enforces it. Copy the 3-line block from a neighbouring file into new files.
- Config comes from env vars only, read through `common.GetEnv` / `common.GetEnvInt` with fallbacks.
  Required at runtime: `AB_BASE_URL`, `AB_CLIENT_ID`, `AB_CLIENT_SECRET`, `AB_NAMESPACE`
  (see `.env.template`).
- Commits and PR titles follow `type(scope): description` with the types listed in CONTRIBUTING.md.

## MCP servers

`.cursor/mcp.json` and `.vscode/mcp.json` define two dockerized MCP servers: `extend-sdk` (AGS Extend
SDK reference for Go) and `ags-api` (live AGS API access, needs `AB_BASE_URL` / `OAUTH_CLIENT_ID` /
`OAUTH_CLIENT_SECRET` in the environment).
