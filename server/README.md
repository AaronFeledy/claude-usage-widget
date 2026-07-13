# Usage Server

Go HTTP API server for Claude Usage Widget. It polls enabled providers and serves cached usage data to the Windows tray app, Home Assistant, or other local clients.

## Build

From `server/`:

```bash
go test ./...
go vet ./...
go build ./...
go build -trimpath -ldflags='-s -w -buildid=' -o usage-server ./cmd/usage-server
```

Cross-build the Windows sidecar from `server/`:

```bash
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o usage-server-win-x64.exe ./cmd/usage-server
GOOS=windows GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o usage-server-win-arm64.exe ./cmd/usage-server
```

## Run

Local-only default:

```bash
./usage-server
```

The default bind is `127.0.0.1:7823`. Off-loopback binds require bearer auth:

```bash
USAGE_AUTH_TOKEN='replace-with-a-long-random-token' \
  ./usage-server --listen-addr 0.0.0.0:7823
```

Call an authenticated remote server with:

```bash
curl -H 'Authorization: Bearer replace-with-a-long-random-token' \
  http://server.example:7823/api/v1/usage
```

## Endpoints

- `GET /api/v1/usage` - array of cached provider usage entries.
- `GET /api/v1/usage/{provider}` - one cached provider entry, for example `Claude` or `codex`.
- `GET /api/v1/health` - server status, version, and provider health.
- `PUT /api/v1/providers/cursor/credentials` - memory-only Cursor credential push with exactly one JSON field: `cookie` or `access_token`.

When `auth_token` or `USAGE_AUTH_TOKEN` is set, every endpoint requires `Authorization: Bearer <token>`.

## Configuration

Config is applied in this order: defaults, YAML, environment variables, then CLI flags.

Default config path:

- Windows: `%APPDATA%\ClaudeUsageWidget\config.yaml`
- Linux/macOS with `XDG_CONFIG_HOME`: `$XDG_CONFIG_HOME/claude-usage-widget/config.yaml`
- Linux/macOS fallback: `~/.config/claude-usage-widget/config.yaml`

CLI flags:

- `--config <path>` - config YAML path.
- `--listen-addr <host:port>` - HTTP listen address.
- `--auth-token <token>` - bearer token.
- `--poll-interval <duration>` - Go duration such as `30s`, `1m`, or `5m`.

Environment variables:

- `USAGE_CONFIG` - config YAML path.
- `USAGE_LISTEN_ADDR` - HTTP listen address.
- `USAGE_AUTH_TOKEN` - bearer token.
- `USAGE_POLL_INTERVAL` - Go duration.
- `USAGE_PROVIDER_<NAME>_ENABLED` - boolean provider toggle, for example `USAGE_PROVIDER_CODEX_ENABLED=true`.
- `USAGE_PROVIDER_<NAME>_CREDENTIALS_PATH` - provider credential path, for example `USAGE_PROVIDER_GROK_CREDENTIALS_PATH=/var/lib/usage-server/grok-auth.json`.

YAML keys:

```yaml
listen_addr: 127.0.0.1:7823
auth_token: ""
poll_interval: 60s
providers:
  claude:
    enabled: true
    credentials_path: ~/.claude/.credentials.json
  codex:
    enabled: true
    credentials_path: ~/.codex/auth.json
  cursor:
    enabled: true
    credentials_path: ""
  grok:
    enabled: true
    credentials_path: ~/.grok/auth.json
```

All four providers are enabled by default; Claude defaults to `~/.claude/.credentials.json`, and the others discover credentials automatically when no path is set. Disable unwanted providers with `enabled: false` or `USAGE_PROVIDER_<NAME>_ENABLED=false`. Codex can also discover `CODEX_HOME/auth.json`, `~/.codex/auth.json`, Windows WSL auth, and OpenCode auth. Cursor local discovery is intended for local browser sessions; remote deployments should prefer tray-pushed in-memory Cursor credentials. Grok defaults to `~/.grok/auth.json` and can discover Windows WSL auth.

## Authentication Safety

The server refuses to start when `listen_addr` is not loopback and the auth token is empty. Safe examples:

```bash
./usage-server --listen-addr 127.0.0.1:7823
USAGE_AUTH_TOKEN='replace-with-a-long-random-token' ./usage-server --listen-addr 0.0.0.0:7823
```

Do not place real provider credentials or bearer tokens in committed files. Prefer systemd environment files, Docker secrets, or Home Assistant `secrets.yaml` for tokens.

## Raspberry Pi Systemd Sample

This sample assumes the binary is installed at `/opt/claude-usage-widget/usage-server`, config lives at `/etc/claude-usage-widget/config.yaml`, provider credential copies are owned by the dedicated `usagewidget` user, and secrets are in `/etc/claude-usage-widget/usage-server.env`.

`/etc/claude-usage-widget/config.yaml`:

```yaml
listen_addr: 0.0.0.0:7823
poll_interval: 60s
providers:
  claude:
    enabled: true
    credentials_path: /var/lib/claude-usage-widget/.claude/.credentials.json
  codex:
    enabled: true
    credentials_path: /var/lib/claude-usage-widget/.codex/auth.json
  cursor:
    enabled: false
  grok:
    enabled: true
    credentials_path: /var/lib/claude-usage-widget/.grok/auth.json
```

`/etc/claude-usage-widget/usage-server.env`:

```text
USAGE_AUTH_TOKEN=replace-with-a-long-random-token
```

`/etc/systemd/system/claude-usage-widget-server.service`:

```ini
[Unit]
Description=Claude Usage Widget API server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=usagewidget
Group=usagewidget
EnvironmentFile=/etc/claude-usage-widget/usage-server.env
ExecStart=/opt/claude-usage-widget/usage-server --config /etc/claude-usage-widget/config.yaml
Restart=on-failure
RestartSec=5s
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/claude-usage-widget

[Install]
WantedBy=multi-user.target
```

Setup sketch:

```bash
sudo useradd --system --home /var/lib/claude-usage-widget --create-home --shell /usr/sbin/nologin usagewidget
sudo install -d -o usagewidget -g usagewidget -m 700 /var/lib/claude-usage-widget
sudo install -d -o root -g root -m 755 /opt/claude-usage-widget /etc/claude-usage-widget
sudo install -o root -g root -m 755 usage-server /opt/claude-usage-widget/usage-server
sudo chmod 600 /etc/claude-usage-widget/usage-server.env
sudo systemctl daemon-reload
sudo systemctl enable --now claude-usage-widget-server
```

Copy provider credential files with owner `usagewidget` and mode `600`.

## Docker

The image default still binds to loopback inside the container, which is not useful for published ports. Override the bind and provide auth:

```bash
docker build -t claude-usage-server ./server
docker run --rm -p 7823:7823 \
  -e USAGE_AUTH_TOKEN='replace-with-a-long-random-token' \
  -v "$HOME/.claude/.credentials.json:/home/nonroot/.claude/.credentials.json:ro" \
  claude-usage-server --listen-addr 0.0.0.0:7823
```

Add provider env/config and read-only credential mounts as needed. For example, enable Grok with `-e USAGE_PROVIDER_GROK_ENABLED=true` and mount `~/.grok/auth.json` to the path configured for the container user.
