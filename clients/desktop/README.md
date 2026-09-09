# Headroom desktop client

A shared Windows and Linux Qt Quick client for the existing usage API. Designed for KDE / Wayland,
with a frameless tray popup, system-tray meter, official provider icons, reset
countdowns, and usage notifications. It connects directly to a remote backend;
it never starts a usage server or reads browser credentials.

## Build and run

Requires CMake 3.21+, a C++17 compiler, and Qt 6.6+ with Quick, Quick Controls 2,
Widgets, Network, SVG image support, and Test. On Arch / EndeavourOS these come
from `base-devel cmake ninja qt6-base qt6-declarative qt6-svg` (plus
`qt6-wayland` for a Wayland session). KDE tray anchoring uses the optional
`kstatusnotifieritem` and `layer-shell-qt` (6.6+) libraries detected by CMake.
These are installed on the target KDE machine. Without them, the client uses
Qt tray activation and the window positioning supported by the desktop.

```bash
cmake -S clients/desktop -B clients/desktop/build -G Ninja -DCMAKE_BUILD_TYPE=Release
cmake --build clients/desktop/build
clients/desktop/build/headroom
```

Install the executable, application-menu entry, and icon for your user:

```bash
cmake --install clients/desktop/build --prefix "$HOME/.local"
```

Ensure `$HOME/.local/bin` is on your desktop session's PATH. Open **Headroom**
from the application menu. For a custom installation prefix, add its `bin`
directory to PATH as well.

## Connect

Open **Connection settings**, enter the server's base HTTP(S) address and bearer
token, then choose **Save & connect**. Reverse-proxy path prefixes are supported.
The client polls `GET /api/v1/usage`; Refresh reads the server's current cache,
not a forced provider refresh. Polling defaults to 60 seconds and can be adjusted
in settings. Requests time out, reject cross-origin redirects, and retain last
readings on failure with a visible offline banner. Empty and malformed responses,
expired provider authentication, and rejected bearer tokens have separate states.
Failures back off exponentially up to five minutes (or the configured interval
if longer), with normal polling restored after success. Manual Refresh bypasses
the wait.

Configuration is saved atomically at `~/.config/Headroom/Headroom/settings.json`
on Linux, honoring `XDG_CONFIG_HOME`, and `%APPDATA%\Headroom\Headroom\settings.json`
on Windows. Linux settings and migration backups use owner-only (`0600`)
permissions. Windows uses the current user's roaming application-data directory;
POSIX mode bits are not used as a claim about Windows ACLs. These files contain
the bearer token in plaintext. Tokens never appear in the exposed UI state or error messages. Leave the token
field blank to keep the saved token for the same address. Changing the address
clears the old token unless a new one is entered. Select **Remove saved token** to clear it.

The remote server must accept connections from your machine. The API server's
default bind is loopback; consult [server configuration](../../server/README.md)
for its listen address and mandatory authentication on non-loopback binds.

## Provider ordering and tray

Provider meters are the first content in the popup. A compact sticky footer
contains provider filtering, Refresh, connection status, and Settings, keeping
controls available while the meters scroll. Providers occupy full-width rows. Meters share the available width and wrap
within their row on smaller windows. In the stacked layout, the first meter
always spans the full width, with remaining meters below it; the final meter
fills any remaining columns. All providers use the same meter colors, with
warning colors reserved for high usage and over-pace indicators. Hovering a
provider highlights its border while preserving contrast with the meter tracks.
Drag a row's six-dot handle onto another row. Drop in the upper half to insert
before it, or the lower half to insert after it. A line marks the insertion point.
Rows read top to bottom. The first provider always supplies the tray meter; the full order
is saved across restarts and polls. Right-click a handle to move a provider
directly to the top. Orders are local to each client. A provider without usable
data produces an unknown tray meter instead of silently switching providers.

The tray shows the first provider's first usage bucket as a ring around a
centered provider logo, without a numeric label. A cyan tick marks expected pace;
a secondary dot shows the highest warning among that provider's other meters.
The tooltip has two lines: provider/primary usage, then reset time and warning
level when needed. Connection errors replace those details with a short status.

Click the tray icon to open or hide the frameless dashboard beside it. The popup
stays above ordinary windows and dismisses when focus moves outside the app;
settings and transient menus keep it open. KDE/Wayland uses native activation
coordinates and LayerShellQt placement, accommodating any panel edge and
clamping the popup to the selected monitor. Without a known icon position,
opening from the app menu uses the lower-right of the active screen. There is
no native title bar, minimize/maximize controls, or taskbar entry in tray mode.
Use **Quit Headroom** from the tray menu or Ctrl+Q to exit. A second launch in the
same user and configuration scope opens the existing popup. An explicit `--config`
path uses its own instance scope and never imports or changes the normal profile.
`--background` starts hidden when a system tray is available.
On desktops without a tray, the frameless app opens as a regular window and
closing it exits normally.

Notifications fire on upward transitions into Warning or Critical, once per
transition. They can be disabled in settings. The client
preserves every API bucket (up to the contract's 12 per provider), including
model-specific and billable meters. Status-only on-demand buckets are labeled
without an invented percentage.

Keyboard shortcuts: **Ctrl+R** refreshes, **Ctrl+,** opens settings, **Ctrl+Q** quits,
and **Escape** closes settings or hides the window to the tray.

## Desktop settings and diagnostics

**Start Headroom when I sign in** enables an XDG autostart entry on Linux or the
current user's `Headroom` Run entry on Windows, launching the quoted executable
with `--background`. This toggle applies immediately and is disabled in preview
and isolated-config modes. On the first normal Windows launch, Headroom imports
schemas 0–3 from `%APPDATA%\ClaudeUsageWidget\settings.json` only when the new
settings file is absent. The legacy file remains untouched and a create-once
backup is kept beside the new settings. Imported empty API addresses retain local
mode; the managed local-server implementation is part of the next migration batch.

Settings shows the app version and checks the configured server's authenticated
health/version endpoint. **Check for updates** reads the public project release
metadata without sending your bearer token. Only a Headroom Linux package for
the current CPU can be offered. The current release workflow publishes Windows
executables, so **Source update guide** opens installed instructions for this
source-built client. Automatic download/install is not implemented.

**Open diagnostics** shows the last 500 events in this session, with UTC times,
categories, Copy log, and Clear. It records controlled connection/settings/tier
summaries, never raw requests, response bodies, tokens, URLs, or account details.
Nothing is written to a log file by this console.

## Preview and verification

`--demo` displays explicitly labeled sample data and makes no API requests.
Reordering sample data changes only the preview session. Saving connection
settings exits preview mode and connects to the real API. `--config PATH`
selects an alternate settings file.

```bash
ctest --test-dir clients/desktop/build --output-on-failure
QT_QPA_PLATFORM=offscreen QT_QUICK_BACKEND=software \
  clients/desktop/build/headroom --demo --screenshot /tmp/headroom.png
```

Tests cover API parsing, legacy bucket fallback, URL handling, local HTTP/Bearer
requests, offline data retention, settings permissions, order persistence, and
actual pointer-driven drag and drop in the QML window. Tests bind a loopback
socket and need permission to do so. UI tests render the desktop, settings, and
compact views with Qt's software renderer. Desktop tray integration still
requires a real desktop session.

Provider assets and their sources are documented in
[the shared icon directory](../shared/provider-icons/README.md).

## Pacing

Every measured usage bar includes a pale marker at its estimated expected usage:
`100 × elapsed time / window length`. The text compares actual usage with that
baseline in percentage points (pp). “10 pp over pace” means usage is ten points
higher than the fraction of the window elapsed; “under pace” means capacity is
being consumed more slowly. Hover the bar or comparison for exact percentages.
The marker advances on the client's 30-second clock, including between polls.

The current API exposes reset times but not period starts. Following the Windows
client's pacing conventions, Headroom estimates five hours for Claude/ChatGPT
sessions, seven days for weekly buckets (including model-specific and Grok Bot
buckets), 30 days for Cursor billing pools, and the previous calendar month for
Grok credits. These are pacing estimates, not provider-guaranteed rate forecasts.
A Claude weekly fallback labeled “Weekly” uses seven days even in a session slot.
Meters with no reset, unknown duration, a reset outside the expected window, or
an expired window show “Pace unavailable.” Status-only billing rows have no
percentage or pacing marker. The existing Windows pacing indicators are retained.

## Windows feature comparison

Headroom uses the server's provider names, subtitles, bucket labels, variable
meter counts, status text, and authentication errors. Preview data uses synthetic
values with representative three/two/four/one-meter layouts; live accounts may
return different buckets. Both clients retain pacing and provider ordering.

Headroom is a remote client. Windows-only local server management, browser-cookie
extraction, application auto-updates, and its debug console have not been ported.
The WSL service deployment is documented in
[the server deployment notes](../../server/deploy/wsl/README.md).

## Appearance and provider names

The interface uses the [Dracula palette](https://draculatheme.com/contribute),
with purple usage bars shared by every provider, cyan under-pace indicators,
yellow, orange, and red concern levels based on the remaining allowance and time. Backgrounds, controls, settings, app icon, and tray
use the same palette. Muted text and surface shades are adapted for readability.

The server's `Codex` provider is displayed as **ChatGPT** throughout Headroom.
Its API identifier and saved ordering key remain `Codex`, preserving existing
connections, pacing calculations, and provider preferences.

## Period divisions and parity

Meter notches represent hours in five-hour windows, days in weekly windows,
and seven-day boundaries in monthly windows. Monthly meters leave a shorter
final segment after day 28 when needed. Hover a notch to see its boundary.
The notches and pacing marker use the same period calculation. Cursor uses a
30-day billing estimate; Grok monthly meters use a calendar-month estimate.
Unknown periods have no time notches.

See the [Windows parity audit](../../docs/linux-parity.md) for restored features,
remaining omissions, and intentional differences.

## Pacing-aware warning colors

The original Windows UI switches between two fixed utilization threshold sets
depending on whether usage is above or below the expected pace. Headroom instead
measures how much of the allowance for the **remaining** window has already been
spent ahead of schedule:

```text
pressure = max(0, (used_percent - elapsed_percent) / (100 - elapsed_percent))
```

Pressure below 10% stays neutral purple; 10% is yellow, 25% orange, and 50% red.
These are presentation thresholds, not limits imposed by a provider. A tiny
positive pacing difference keeps neutral text and does not add a provider to
the attention count. Tooltips show the remaining allowance, time, and calculation.

| Used | Window elapsed | Remaining allowance spent early | Color |
| --- | --- | --- | --- |
| 12% | 10% | 2.2% | Purple |
| 82% | 80% | 10% | Yellow |
| 95% | 90% | 50% | Red |
| 90% | 90% | 0% | Purple |

A separate low-capacity floor applies even on/under pace: 95% used is at least
yellow, 99% at least orange, and 100% red. Without a usable reset window, the
original percentage fallback applies: yellow at 50%, orange at 75%, red at 90%.
Expired or unrecognized windows do not produce an invented pacing estimate.

The dashboard, bar fills, percentages, pacing text, attention count, and primary
tray meter use the same state machine and update as time passes. This
compares against even spending across a window; it does not infer a recent burn
rate or predict future activity.

### Shared state machine

`warning.cpp` defines one tier table: thresholds, colors, labels, and whether an
upward transition alerts. The controller owns one state per provider/bucket and
reset window. QML and the tray read that state; they do not decide severity
independently. The meter's normal 30-second clock also permits recovery as time
passes without additional spending.

| Tier | Enter at pacing pressure | Recover below | Color | Pop-up on upward entry |
| --- | --- | --- | --- | --- |
| Normal | Below Watch | — | Purple | No |
| Watch | 10% | 8% | Yellow | No |
| Warning | 25% | 20% | Orange | Yes |
| Critical | 50% | 40% | Red | Yes |

Low-capacity guards also have hysteresis: Watch enters at 95% usage and releases
below 94%; Warning at 99% / below 98%; Critical at 100% / below 99.5%. With no
pacing, fallback tiers enter at 50/75/90% and release below 45/70/85%. A tier
recovers only when neither its pacing condition nor its capacity guard holds.
Escalation can jump directly to any tier; recovery can skip tiers as well.

Initial connection and each new reset window establish a baseline without
notification bursts. Later upward transitions into Warning or Critical notify
when enabled. Unchanged polls, downward transitions, preview mode, and toggling
notifications back on do not notify. A genuine recovery followed by escalation
can alert again. Provider order changes do not reset state or re-arm alerts.

Connection failures freeze state. Expired known windows retain their state until
a replacement window arrives, avoiding false percentage-only alerts from old
readings at reset time. Provider failures retain prior bucket state until valid
data returns; removed buckets/providers are discarded. States are session-local;
restarting the app establishes a new baseline.
