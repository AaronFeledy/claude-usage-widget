# Linux client parity audit

Audited against the Windows source on 2026-09-08. Headroom is a remote API
client with a frameless tray popup. It does not yet reproduce every
Windows feature.

## Restored or improved

- All API buckets, original meter labels, model-specific weekly pools, billing
  status, reset countdowns, provider errors, and authentication guidance.
- Saved drag-and-drop provider ordering. The first provider supplies the tray
  meter. Official provider icons; `Codex` is displayed as ChatGPT in Headroom.
- Pacing markers and above/below-pace text on dashboard meters.
- Period-specific notches: five-hour windows have four hourly ticks; weekly
  windows have six daily ticks. Monthly windows have ticks at days 7, 14, 21,
  and (when inside the window) 28. The remaining days form a shorter segment.
  Windows used three equal monthly notches, approximating weeks. Cursor's
  period remains a 30-day estimate; Grok's monthly estimate uses the reset date
  minus one calendar month. The API does not supply a period start or duration.
  Unknown periods do not get invented time divisions.
- One warning state machine per meter drives dashboard color, attention count,
  the primary tray meter, and notifications. It uses remaining-window pressure,
  capacity guards, recovery thresholds, and transition-based alerts. This is
  implemented in Headroom; Windows still uses its previous color thresholds and
  separate notification logic.
- Background tray operation, single instance, refresh, configurable polling,
  remote bearer authentication, and notification controls.

## Desktop features now restored

| Feature | Headroom behavior |
| --- | --- |
| Launch at login | Explicit settings toggle writes an XDG autostart entry for the current executable with `--background`; preview cannot change it |
| Tray pacing | Cyan expected-spend tick shares the dashboard period calculation |
| Tray identity and secondary usage | Official provider badge; secondary dot uses the highest cached warning tier among that provider's other meters |
| Tray diagnostics | Distinct setup, connecting, idle, offline, bearer-auth, HTTP, malformed-response, provider-error, and exhausted states |
| Tray tooltip | Intentionally compact: selected provider/usage, reset countdown, and warning tier; brief error status when unavailable |
| Version details | Application version and an authenticated server health/version check in settings |
| Debug console | Session-local rolling 500-event log with UTC timestamps, categories, copy and clear; records only controlled summaries without account details, credentials, or response bodies |
| Offline polling | Exponential retry delay, capped at five minutes or the configured interval if longer; success resets the delay and manual Refresh works immediately |
| Tray Settings action | Opens connection settings directly |

## Update availability

Settings now has a manual release check, release notes, and an installed local
source-update guide. The checker only recognizes Headroom Linux assets for the
current CPU; Windows client and usage-server artifacts cannot appear as Linux
client updates. Health requests and public release requests have separate
credentials, bounded responses, deadlines, and no automatic redirects.

The repository's release workflow still publishes Windows executables only.
Linux binary packaging, automatic startup update checks, download/staging, and
self-update remain unimplemented. The current Linux installation is updated by
building, testing, and installing the local checkout. The application explains
this limitation instead of claiming that a Windows release can update it.

## Intentional differences

- Headroom now anchors a frameless popup near the tray, stays above regular
  windows, and dismisses on outside focus. KDE/Wayland uses native tray activation
  coordinates and layer-shell positioning. Its redesigned provider rows remain.
- The tray logo is centered without a number; the tooltip is deliberately two
  lines. Full meter details remain available in the popup.
- Headroom keeps the first provider selected when it fails and shows an unknown
  tray meter. Windows can fall back to the next successful provider. Keeping
  selection stable follows the request that the top provider controls the tray.
- Headroom displays the selected provider's first API bucket in the tray.
  Windows still uses the legacy `current` value, which may not describe the
  first visible bucket for providers with several pools.
- Bundled local-server acquisition, process ownership/restart, and browser
  credential forwarding are Windows workflows. Headroom connects to the
  separately managed WSL service and does not collect browser credentials.

## Source references

- [Windows bucket rendering and period styles](../clients/windows/UI/ProviderUsagePanel.cs)
- [Windows progress bar](../clients/windows/UI/UsageProgressBar.cs)
- [Windows tray icon](../clients/windows/TrayIcon/IconGenerator.cs)
- [Windows tray selection and tooltip](../clients/windows/TrayIcon/TrayApplicationContext.Helpers.cs)
- [Windows settings UI](../clients/windows/UI/UsagePopup.Settings.cs)
- [Windows notification policy](../clients/windows/Services/NotificationService.cs)
- [Windows updater](../clients/windows/Services/UpdateService.cs)
- [Windows debug service](../clients/windows/Services/DebugService.cs)
- [Linux period calculations](../clients/linux/usage.cpp)
- [Linux warning policy](../clients/linux/warning.cpp)
- [Linux controller](../clients/linux/controller.cpp)
- [Linux tray model and rendering](../clients/linux/trayvisual.cpp)
- [Linux startup service](../clients/linux/startup.cpp)
- [Linux version and release checks](../clients/linux/appinfo.cpp)
