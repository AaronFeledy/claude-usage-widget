# Claude Usage Widget

Windows system tray widget that shows your Claude Max subscription usage at a glance.

![License](https://img.shields.io/github/license/AaronFeledy/claude-usage-widget)

## Features

- **Tray icon** — vertical fill bar shows 5-hour window usage at a glance
- **Color-coded** — green → yellow → orange → red as usage increases
- **Weekly overlay** — appears only when weekly usage exceeds 70%
- **Hover tooltip** — shows usage percentages and time until reset
- **Click popup** — detailed view with progress bars and reset timers
- **Toast notifications** — warns at 80% and 95% usage thresholds
- **Auto-refresh** — polls every 60 seconds (configurable)
- **Single exe** — no installer, no runtime dependencies

## How It Works

Reads OAuth credentials from Claude Code (`~/.claude/.credentials.json`) and calls the Anthropic usage API. Automatically refreshes expired tokens.

## Requirements

- Windows 10/11
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code/overview) installed and logged in

## Building

```bash
dotnet publish -c Release
```

Output: `bin/Release/net8.0-windows/publish/ClaudeUsageWidget.exe`

## License

MIT
