# Claude Usage Widget

Windows tray client plus a local/remote usage API server for Claude, Codex, Cursor, and Grok usage.

[![Build](https://github.com/AaronFeledy/claude-usage-widget/actions/workflows/build.yml/badge.svg)](https://github.com/AaronFeledy/claude-usage-widget/actions/workflows/build.yml)
[![Release](https://img.shields.io/github/v/release/AaronFeledy/claude-usage-widget)](https://github.com/AaronFeledy/claude-usage-widget/releases/latest)
[![License](https://img.shields.io/github/license/AaronFeledy/claude-usage-widget)](LICENSE)

## Layout

- `clients/windows/` - .NET 8 WinForms tray client.
- `server/` - Go usage API server, provider integrations, and Dockerfile.
- `tests/windows/` - Linux-runnable C# harness tests for tray/server lifecycle seams.
- `docs/home-assistant.md` - Home Assistant REST sensor example for the usage API.

## Architecture

```text
Local managed mode, default Windows install

  Windows tray app
    | ApiUrl empty in %APPDATA%/ClaudeUsageWidget/settings.json
    | spawns/probes bundled usage-server.exe on 127.0.0.1:7823
    | discovers Cursor browser credentials and pushes them in memory
    v
  usage-server.exe
    | reads credential files for enabled providers
    v
  Claude / Codex / Cursor / Grok upstream APIs

Remote mode, shared server or Home Assistant

  Windows tray app or Home Assistant REST sensor
    | ApiUrl / resource points at http(s)://server:7823/
    | Authorization: Bearer <token> when server is off-loopback
    v
  usage-server on NAS, Raspberry Pi, container, or another host
    | started with --listen-addr 0.0.0.0:7823 and auth token
    v
  Provider credential files on that server host
```

The server default is intentionally local-only: `127.0.0.1:7823`. Binding to any non-loopback address without a bearer token is rejected before the server listens or providers are constructed.

Claude is enabled by default. Enable Codex, Cursor, and Grok in `%APPDATA%\ClaudeUsageWidget\config.yaml` for a local Windows install, or through the server's YAML/environment configuration for other deployments.

## Features

- Multi-provider popup for enabled Claude, Codex, Cursor, and Grok providers.
- Primary provider selection for tray icon, tooltip, top card, and notifications.
- Dynamic tray icon with provider badge and usage fill color.
- Local managed server mode with matched tray/server release assets.
- Remote API mode for another host, Docker, or Home Assistant.
- Auto-refresh and token refresh where provider credentials support it.

## Installation

**PowerShell:**

```powershell
irm https://raw.githubusercontent.com/AaronFeledy/claude-usage-widget/main/install.ps1 | iex
```

The installer downloads the latest matching Windows tray and server assets for your architecture into `%LOCALAPPDATA%\ClaudeUsageWidget\` and launches the tray app.

## Documentation

- [Windows client](clients/windows/README.md)
- [Usage server](server/README.md)
- [Home Assistant REST sensor](docs/home-assistant.md)

## Provider Credentials

The server reads credentials for enabled providers on the host where `usage-server` runs. The Windows tray can also push discovered Cursor browser credentials to the server in memory.

- Claude: `~/.claude/.credentials.json`, Windows WSL auth, or OpenCode auth fallback.
- Codex: `CODEX_HOME/auth.json`, `~/.codex/auth.json`, Windows WSL auth, or OpenCode auth fallback.
- Cursor: server-side local auth-file discovery on loopback deployments, or browser credentials discovered and pushed in memory by the Windows tray. Remote deployments normally need credentials pushed from the tray.
- Grok: `~/.grok/auth.json`, with Windows WSL fallback.

## API Shape

`GET /api/v1/usage` returns a JSON array. Each entry uses snake_case fields:

```json
[
  {
    "provider_name": "Claude",
    "primary_label": "Current Session",
    "secondary_label": "Weekly",
    "show_secondary": true,
    "subtitle": null,
    "primary_status_text": null,
    "secondary_status_text": null,
    "reauth_command": null,
    "current": { "utilization": 42.5, "resets_at": "2026-07-12T18:00:00Z" },
    "weekly": { "utilization": 17.0, "resets_at": null },
    "error": null,
    "needs_reauth": false,
    "is_success": true
  }
]
```

`is_success` is derived from `error == null`; optional strings and reset timestamps are emitted as explicit `null`.

## Build And Verify

From the repository root:

```bash
dotnet build clients/windows/ClaudeUsageWidget.csproj -c Release -r win-x64
dotnet run --project tests/windows/ServerProcessManagerTests.csproj -c Release

cd server
go test ./...
go vet ./...
go build ./...
```

Release-style Windows artifacts are produced with:

```bash
dotnet publish clients/windows/ClaudeUsageWidget.csproj -c Release -r win-x64 -p:PublishSingleFile=true -p:SelfContained=true -p:EnableCompressionInSingleFile=true
cd server && GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o ../usage-server-win-x64.exe ./cmd/usage-server
```

## License

MIT
