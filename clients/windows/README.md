# Windows Client

.NET 8 WinForms tray client for Claude Usage Widget.

## Build

From the repository root:

```bash
dotnet build clients/windows/ClaudeUsageWidget.csproj -c Release -r win-x64
dotnet publish clients/windows/ClaudeUsageWidget.csproj -c Release -r win-x64 -p:PublishSingleFile=true -p:SelfContained=true -p:EnableCompressionInSingleFile=true
```

Use `win-arm64` instead of `win-x64` for ARM64 Windows.

Output path:

```text
clients/windows/bin/Release/net8.0-windows/<runtime>/publish/ClaudeUsageWidget.exe
```

## Run

Publish the tray app and place the matching `usage-server.exe` beside it. Running `ClaudeUsageWidget.exe` starts the tray application. Only one instance is allowed.

Settings are stored in `%APPDATA%\ClaudeUsageWidget\settings.json`.

## ApiUrl And ApiToken

`ApiUrl` controls local versus remote API behavior:

- Empty `ApiUrl` is local managed mode. The tray uses `http://127.0.0.1:7823/`, probes for an existing healthy local server, and otherwise starts bundled `usage-server.exe` with `--listen-addr 127.0.0.1:7823`.
- Nonempty `ApiUrl` is remote mode. It must be an absolute `http` or `https` URL with no leading/trailing whitespace and no embedded username/password. The tray does not spawn or probe a local server first; it sends API requests to the normalized remote URL.
- Invalid `ApiUrl` leaves the tray offline until the setting is corrected.

`ApiToken` is trimmed and persisted. When nonempty, every tray API request sends it as `Authorization: Bearer <ApiToken>`.

Use cases:

- Local default server: keep both `ApiUrl` and `ApiToken` empty.
- Remote loopback-only tunnel or reverse proxy with no server auth: set `ApiUrl`, leave `ApiToken` empty only if the server deployment is intentionally unauthenticated and still safe.
- Remote server bound off-loopback: set `ApiUrl` and set `ApiToken` to the same token configured as server `auth_token` or `USAGE_AUTH_TOKEN`.

Settings schema migration adds `ApiUrl`, `ApiToken`, and the current schema version if missing. Existing settings are backed up to `settings.json.bak` before migration.

## Verification

From the repository root:

```bash
dotnet build clients/windows/ClaudeUsageWidget.csproj -c Release -r win-x64
dotnet run --project tests/windows/ServerProcessManagerTests.csproj -c Release
```

Linux cross-builds need a .NET SDK installation that includes `Microsoft.NET.Sdk.WindowsDesktop`; native tray rendering and Windows Job Object runtime behavior still require a Windows host.
