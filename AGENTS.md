# Claude Usage Widget - Agent Guidelines

## Project Overview
Windows system tray widget (.NET 8, WinForms) that monitors Claude Max subscription usage.

## Architecture
- **ClaudeUsageWidget.csproj** — .NET 8 WinForms project targeting `net8.0-windows`
- **Models/UsageData.cs** — Data models for API response
- **Services/CredentialService.cs** — Reads/refreshes OAuth tokens from `~/.claude/.credentials.json`
- **Services/UsageApiClient.cs** — Calls `GET https://api.anthropic.com/api/oauth/usage`
- **TrayIcon/IconGenerator.cs** — Generates dynamic 16x16 tray icons based on usage %
- **TrayIcon/TrayApplicationContext.cs** — Main app context managing tray icon, tooltip, popup
- **UI/UsagePopup.cs** — Click popup with progress bars, reset timers
- **Program.cs** — Entry point

## API Details
- **Usage endpoint:** `GET https://api.anthropic.com/api/oauth/usage`
- **Auth header:** `Authorization: Bearer <accessToken>` + `anthropic-beta: oauth-2025-04-20`
- **Token refresh:** `POST https://api.anthropic.com/v1/oauth/token` with `{ grant_type: "refresh_token", refresh_token: "<token>" }`
- **Credentials file:** `~/.claude/.credentials.json` → `claudeAiOauth.accessToken`, `.refreshToken`, `.expiresAt` (ms)
- **Response shape:** `{ five_hour: { utilization: float 0-100, resets_at: ISO8601 }, seven_day: { ... } }`

## Icon Design
- 16x16 vertical fill bar, fills bottom-to-top based on 5h utilization
- Green (0-50%), Yellow (50-75%), Orange (75-90%), Red (90%+)
- Weekly overlay: small corner indicator only when weekly > 70%

## Build
```bash
dotnet build
dotnet publish -c Release -r win-x64 --self-contained -p:PublishSingleFile=true
```
