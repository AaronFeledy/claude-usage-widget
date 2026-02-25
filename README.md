# Claude Usage Widget

Windows system tray widget that shows your Claude Max subscription usage at a glance.

[![Build](https://github.com/AaronFeledy/claude-usage-widget/actions/workflows/build.yml/badge.svg)](https://github.com/AaronFeledy/claude-usage-widget/actions/workflows/build.yml)
[![Release](https://img.shields.io/github/v/release/AaronFeledy/claude-usage-widget)](https://github.com/AaronFeledy/claude-usage-widget/releases/latest)
[![License](https://img.shields.io/github/license/AaronFeledy/claude-usage-widget)](LICENSE)

## Features

- **Tray icon** — vertical fill bar shows 5-hour window usage at a glance
- **Color-coded** — green → yellow → orange → red as usage increases
- **Weekly overlay** — appears only when weekly usage exceeds 70%
- **Hover tooltip** — shows usage percentages and time until reset
- **Click popup** — detailed view with progress bars and reset timers
- **Toast notifications** — warns at 80% and 95% usage thresholds
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
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code/overview) installed and logged in

## How It Works

The widget reads OAuth credentials from Claude Code's credential store (`~/.claude/.credentials.json`) and calls the Anthropic usage API to fetch your current consumption. Tokens are automatically refreshed when expired.

The tray icon fills from bottom to top based on your 5-hour utilization. Colors indicate severity:
- **Green** (0-50%) — plenty of headroom
- **Yellow** (50-75%) — moderate usage
- **Orange** (75-90%) — approaching limit
- **Red** (90%+) — near or at limit

## Configuration

The widget uses sensible defaults. No configuration required.

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
