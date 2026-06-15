# Claude Usage Widget

Windows system tray widget that shows your Claude, Codex, Cursor, and Grok usage at a glance.

[![Build](https://github.com/AaronFeledy/claude-usage-widget/actions/workflows/build.yml/badge.svg)](https://github.com/AaronFeledy/claude-usage-widget/actions/workflows/build.yml)
[![Release](https://img.shields.io/github/v/release/AaronFeledy/claude-usage-widget)](https://github.com/AaronFeledy/claude-usage-widget/releases/latest)
[![License](https://img.shields.io/github/license/AaronFeledy/claude-usage-widget)](LICENSE)

## Features

- **Multi-provider popup** — Claude, Codex, Cursor, and Grok usage in one tray app
- **Primary provider selection** — choose which service drives the tray icon, notifications, and top UI card
- **Tray icon** — vertical fill bar plus provider badge for Claude, Codex, Cursor, or Grok
- **Color-coded** — green → yellow → orange → red as usage increases
- **Weekly overlay** — appears only when weekly usage exceeds 70%
- **Hover tooltip** — shows usage percentages and time until reset
- **Click popup** — detailed view with progress bars and reset timers
- **Toast notifications** — warns at 75% and 90% usage thresholds for the selected primary provider
- **Auto-refresh** — polls every 60 seconds
- **Single exe** — no installer, no runtime dependencies

## Screenshots

*Coming soon*

## Installation

**PowerShell:**
```powershell
irm https://raw.githubusercontent.com/AaronFeledy/claude-usage-widget/main/install.ps1 | iex
```

Auto-detects your architecture (x64/ARM64), downloads the latest release to `%LOCALAPPDATA%\ClaudeUsageWidget\`, and launches it. Future updates are handled automatically by the app.

### Start with Windows

Right-click the tray icon → Settings → check "Start with Windows".

## Requirements

- Windows 10/11
- At least one of [Claude Code](https://docs.anthropic.com/en/docs/claude-code/overview), OpenAI Codex, Cursor, or the Grok CLI (`grok login`) logged in locally

## How It Works

The widget reads provider state from local tools and browser sessions:

- **Claude** — reads OAuth credentials from `~/.claude/.credentials.json` and calls the Anthropic usage API
- **Codex** — reads auth from `~/.codex/auth.json` and calls the OpenAI Codex/ChatGPT usage endpoint
- **Cursor** — reads Cursor session cookies from supported local browsers and calls Cursor's usage APIs
- **Grok** — reads OAuth credentials from `~/.grok/auth.json` (from `grok login`) and calls the Grok CLI billing endpoint for SuperGrok / Grok Build credit usage (monthly included credits + pay-as-you-go cap)

Tokens are refreshed automatically when the provider supports it.

For Cursor, the app currently looks for a logged-in session in supported Windows browsers:

- Chrome
- Edge
- Brave
- Firefox

The tray icon fills from bottom to top based on your primary provider's utilization (5-hour window for Claude/Codex/Grok, billing cycle for Cursor). Colors indicate severity:
- **Green** (0-50%) — plenty of headroom
- **Yellow** (50-75%) — moderate usage
- **Orange** (75-90%) — approaching limit
- **Red** (90%+) — near or at limit

The selected primary provider is also emphasized in the popup and used for notifications.

**Grok note:** Tracks the consumer SuperGrok / Grok Build CLI subscription credits (included monthly credits + optional pay-as-you-go cap). This is separate from the developer `api.x.ai` pay-as-you-go surface.

## Configuration

The widget works with sensible defaults, but you can adjust a few things from the tray icon's Settings menu:

- Primary provider (`Claude`, `Codex`, `Cursor`, or `Grok`)
- Refresh interval
- Notifications on/off
- Start with Windows
- Debug mode

## Building from Source

```bash
# Clone the repo
git clone https://github.com/AaronFeledy/claude-usage-widget.git
cd claude-usage-widget

# Build
dotnet build -c Release

# Publish single-file exe (choose your architecture)
dotnet publish -c Release -r win-x64
dotnet publish -c Release -r win-arm64
```

Output: `bin/Release/net8.0-windows/<runtime>/publish/ClaudeUsageWidget.exe`

## Contributing

Pull requests welcome. Keep it lean.

## License

MIT
