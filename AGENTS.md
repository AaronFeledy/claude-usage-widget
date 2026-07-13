# Claude Usage Widget - Agent Guidelines

## Project Overview

Monorepo for a Windows tray usage client and a Go usage API server. The app monitors Claude, Codex, Cursor, and Grok usage.

## Layout

- `clients/windows/` - .NET 8 WinForms tray client targeting `net8.0-windows`.
- `server/` - Go 1.23 usage API server, provider integrations, config loader, Dockerfile.
- `tests/windows/` - C# harness tests for Windows client/server lifecycle seams that run on Linux.
- `docs/` - user docs, including Home Assistant REST sensor guidance.
- `.github/workflows/` - Windows client, server, Docker, and release automation.

## Architecture

- Windows local mode: empty `ApiUrl` in `%APPDATA%/ClaudeUsageWidget/settings.json` makes the tray use `http://127.0.0.1:7823/`, attach to a healthy local server if present, or spawn bundled `usage-server.exe` with `--listen-addr 127.0.0.1:7823`.
- Windows remote mode: nonempty absolute HTTP(S) `ApiUrl` points the tray at an external server and disables local spawn. Nonempty `ApiToken` is sent as `Authorization: Bearer <token>`.
- Server default bind is `127.0.0.1:7823`; off-loopback binds require `auth_token`, `USAGE_AUTH_TOKEN`, or `--auth-token` before listen/provider construction.
- API contract is frozen in `server/internal/usage`: snake_case fields, explicit `null` optional strings/reset timestamps, and `is_success` derived from `error == null`.

## Key Files

- `clients/windows/Program.cs` - tray entrypoint, settings, local/remote server manager wiring, API client wiring.
- `clients/windows/Services/SettingsService.cs` - persisted settings and schema migration.
- `clients/windows/Services/ServerProcessManager*.cs` - bundled server acquisition, local spawn, health probe, restart, and Job Object ownership.
- `clients/windows/Services/ApiClient*.cs` - tray REST client and wire DTO validation.
- `server/cmd/usage-server/main.go` - config load, startup safety validation, provider construction, poller/server lifecycle.
- `server/internal/config/` - CLI/env/YAML config loading.
- `server/internal/api/` - REST routes, bearer auth middleware, startup bind validation.
- `server/internal/usage/` - provider-independent API response structs.
- `server/internal/providers/` - Claude, Codex, Cursor, and Grok integrations.

## Verification Commands

From repository root:

```bash
dotnet build clients/windows/ClaudeUsageWidget.csproj -c Release -r win-x64
dotnet run --project tests/windows/ServerProcessManagerTests.csproj -c Release
```

From `server/`:

```bash
go test ./...
go vet ./...
go build ./...
go test -race -shuffle=on -count=1 ./...
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /tmp/usage-server-win-x64.exe ./cmd/usage-server
```

Docker validation is configured in CI with Buildx cache-only output. Local Docker may be unavailable in this environment; do not claim local Docker runtime validation unless it was actually run.

## Documentation Rules

- Derive server flags, env vars, YAML keys, provider defaults, and response fields from source, not memory.
- Do not claim a Home Assistant add-on exists; only `docs/home-assistant.md` REST sensor configuration exists.
- Keep bearer tokens, provider credentials, browser cookies, and live output redacted.
- Do not stage `.omo/**`; evidence and notepads are task-local artifacts only.
