# Headroom usage server

The Go server polls enabled providers and serves cached usage to Headroom, Home
Assistant REST sensors, and compatible API clients. The `usage-server` binary,
configuration keys, environment variables, default paths, and API wire names
remain compatible with Claude Usage Widget deployments.

## Build

From `server/`:

```bash
go test ./...
go vet ./...
go build ./...
go build -trimpath -ldflags='-s -w -buildid=' -o usage-server ./cmd/usage-server
```

Cross-build the Windows local server from `server/`:

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

Linux and WSL servers can also enable Headroom's SSH transport with
`--ssh-access` or `ssh_access: true`. The SSH login must use the same OS account
as the service. This adds a protected Unix socket to the existing server while
preserving its HTTP configuration. See the [SSH setup guide](../docs/ssh.md).

## Endpoints

- `GET /api/v1/usage` - array of cached provider usage entries.
- `GET /api/v1/usage/{provider}` - one cached provider entry, for example `Claude` or `codex`.
- `GET /api/v1/health` - server status, version, and provider health.
- `PUT /api/v1/providers/cursor/credentials` - memory-only Cursor credential push with exactly one JSON field: `cookie` or `access_token`.
- `PUT /api/v1/providers/grok/credentials` - memory-only Grok browser credential push with exactly one JSON field: `cookie`.

When `auth_token` or `USAGE_AUTH_TOKEN` is set, every public HTTP endpoint requires
`Authorization: Bearer <token>`. The protected SSH socket authenticates the local
OS account and supplies that token internally; an SSH client does not need it.

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
- `--ssh-access` - opt in to the private per-account SSH socket on Linux/WSL;
  disabled by default and incompatible with `--desktop-session`.
- `--ssh-stdio` - run the restricted one-request SSH receiver. Must be used
  alone; it connects to the running server without loading provider configuration
  or creating a second poller.
- `--desktop-session` - reserved for the bundled Headroom child process; requires
  a numeric loopback bind and a bounded private stdin handshake. It uses a fresh
  in-memory TLS identity and session bearer token instead of the configured
  token, and publishes only the public certificate and listener identity on
  stdout. It has no YAML or environment equivalent. Standalone services should
  omit this flag and retain their normal HTTP/reverse-proxy configuration.

Environment variables:

- `USAGE_CONFIG` - config YAML path.
- `USAGE_LISTEN_ADDR` - HTTP listen address.
- `USAGE_AUTH_TOKEN` - bearer token.
- `USAGE_POLL_INTERVAL` - Go duration.
- `USAGE_SSH_ACCESS` - boolean enabling the Linux/WSL SSH socket.
- `USAGE_PROVIDER_<NAME>_ENABLED` - boolean provider toggle, for example `USAGE_PROVIDER_CODEX_ENABLED=true`.
- `USAGE_PROVIDER_<NAME>_CREDENTIALS_PATH` - provider credential path, for example `USAGE_PROVIDER_GROK_CREDENTIALS_PATH=/var/lib/usage-server/grok-auth.json`.

YAML keys:

```yaml
listen_addr: 127.0.0.1:7823
auth_token: ""
poll_interval: 60s
ssh_access: false
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

All four providers are enabled by default; Claude defaults to
`~/.claude/.credentials.json`, and the others discover credentials automatically
when no path is set. Disable unwanted providers with `enabled: false` or
`USAGE_PROVIDER_<NAME>_ENABLED=false`. The API name `Codex` is intentionally
retained while Headroom displays it as ChatGPT. Codex can also discover
`CODEX_HOME/auth.json`, `~/.codex/auth.json`, Windows WSL auth, and OpenCode
auth. Cursor local discovery is intended for local browser sessions; remote
deployments should prefer an in-memory credential push over HTTPS because
Headroom never sends browser credentials to remote plain HTTP. Grok weekly usage
primarily uses the authenticated CLI billing endpoint with `~/.grok/auth.json`
or Windows WSL auth; the memory-only browser `sso` cookie is an optional fallback
when CLI weekly data is unavailable. Browser discovery is available in official
Windows Headroom packages only; Linux deployments use server-side files or the
documented WSL sync.

## Authentication Safety

The server refuses to start when `listen_addr` is not loopback and the auth token is empty. Safe examples:

```bash
./usage-server --listen-addr 127.0.0.1:7823
USAGE_AUTH_TOKEN='replace-with-a-long-random-token' ./usage-server --listen-addr 0.0.0.0:7823
```

Do not place real provider credentials or bearer tokens in committed files. Prefer systemd environment files, Docker secrets, or Home Assistant `secrets.yaml` for tokens.

## Raspberry Pi Systemd Sample

This compatible sample keeps its existing paths: the binary is installed at
`/opt/claude-usage-widget/usage-server`, config lives at
`/etc/claude-usage-widget/config.yaml`, provider credential copies are owned by
the dedicated `usagewidget` user, and secrets are in
`/etc/claude-usage-widget/usage-server.env`.

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
Description=Headroom usage API server
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
docker build -t headroom-usage-server ./server
docker run --rm -p 7823:7823 \
  -e USAGE_AUTH_TOKEN='replace-with-a-long-random-token' \
  -v "$HOME/.claude/.credentials.json:/home/nonroot/.claude/.credentials.json:ro" \
  headroom-usage-server --listen-addr 0.0.0.0:7823
```

Add read-only credential mounts as needed. All providers, including Grok, are enabled by default, so Grok only needs its credential file made available—for example, mount `~/.grok/auth.json` to the path configured for the container user (override it with `-e USAGE_PROVIDER_GROK_CREDENTIALS_PATH=...` when the mount path differs).
