using System.Drawing;
using ClaudeUsageWidget.Models;
using ClaudeUsageWidget.Services;
using ClaudeUsageWidget.UI;

namespace ClaudeUsageWidget.TrayIcon;

/// <summary>
/// Application context that manages the system tray icon and polling.
/// </summary>
public partial class TrayApplicationContext : ApplicationContext
{
    private static readonly string[] DefaultProviderOrder = ["Claude", "Codex", "Cursor", "Grok"];

    private readonly TrayUsagePoller _usagePoller;
    private readonly SettingsService _settingsService;
    private readonly NotificationService _notificationService;
    private readonly UpdateService _updateService;
    private readonly DebugService _debugService;
    private readonly NotifyIcon _notifyIcon;
    private readonly System.Windows.Forms.Timer _pollTimer;
    private readonly ContextMenuStrip _contextMenu;

    private readonly Dictionary<string, UsageData> _providerUsage = new(StringComparer.OrdinalIgnoreCase);
    private TrayUsageSnapshot _snapshot = new(TrayApiState.Loading, [], null, null, 0);
    private bool _isPolling;
    private Icon? _currentIcon;
    private UsagePopup? _popup;
    private DebugConsole? _debugConsole;

    public TrayApplicationContext(TrayUsagePoller usagePoller, SettingsService settingsService, DebugService debugService, HttpClient httpClient)
    {
        _usagePoller = usagePoller;
        _settingsService = settingsService;
        _debugService = debugService;

        // Create the context menu
        _contextMenu = new ContextMenuStrip();
        _contextMenu.Items.Add("Refresh Now", null, OnRefreshNow);
        _contextMenu.Items.Add("Settings", null, OnSettingsClicked);
        _contextMenu.Items.Add("Check for Updates", null, OnCheckForUpdates);
        _contextMenu.Items.Add(new ToolStripSeparator());
        
        // Add debug console option if debug mode is enabled
        if (_settingsService.Settings.DebugMode)
        {
            _contextMenu.Items.Add("Debug Console", null, OnDebugConsoleClicked);
            _contextMenu.Items.Add(new ToolStripSeparator());
        }
        
        _contextMenu.Items.Add("Quit", null, OnQuit);

        // Create the notify icon with a placeholder icon
        _currentIcon = IconGenerator.GeneratePlaceholderIcon(GetPrimaryProviderName());
        _notifyIcon = new NotifyIcon
        {
            Icon = _currentIcon,
            Text = TrayVisualState.TooltipText("Claude: Loading..."),
            ContextMenuStrip = _contextMenu,
            Visible = true
        };

        // Create notification service after NotifyIcon is created
        _notificationService = new NotificationService(_notifyIcon, _settingsService);

        // Create update service
        _updateService = new UpdateService(httpClient, _notifyIcon);

        // Wire up left-click to show popup
        _notifyIcon.MouseClick += OnNotifyIconClick;

        // Set up the polling timer with interval from settings
        _pollTimer = new System.Windows.Forms.Timer
        {
            Interval = _settingsService.Settings.RefreshIntervalSeconds * 1000
        };
        _pollTimer.Tick += async (_, _) => await PollUsageAsync();
        _pollTimer.Start();

        _debugService.LogInfo("App", "Claude Usage Widget started");

        // Do an initial poll
        _ = PollUsageAsync();

        // Check for updates in background (silent, no notification if up-to-date)
        _ = _updateService.CheckForUpdatesAsync(showNotificationIfNoUpdate: false);
    }

    /// <summary>
    /// Polls the usage API and updates the tooltip and icon.
    /// </summary>
    private async Task PollUsageAsync()
    {
        if (_isPolling) return;

        try
        {
            _isPolling = true;
            _snapshot = await _usagePoller.PollAsync();
            _providerUsage.Clear();
            foreach (var result in _snapshot.Providers)
            {
                _providerUsage[result.ProviderName] = result;
            }
            ApplyRetryInterval();

            UpdateTooltip();
            UpdateIcon();
            UpdatePopup();

            if (_snapshot.State == TrayApiState.Ready)
            {
                _notificationService.CheckAndNotify(GetPreferredNotificationData());
            }
        }
        finally
        {
            _isPolling = false;
        }
    }

    /// <summary>
    /// Updates the tooltip text based on the latest usage data.
    /// </summary>
    private void UpdateTooltip()
    {
        if (_providerUsage.Count == 0)
        {
            SetTooltip(FormatNoDataTooltip());
            return;
        }

        if (_snapshot.State != TrayApiState.Ready)
        {
            SetTooltip(FormatApiStateTooltip());
            return;
        }

        var successfulProviders = GetOrderedUsageData()
            .Where(x => x.IsSuccess)
            .Select(FormatProviderTooltip)
            .Where(x => !string.IsNullOrWhiteSpace(x))
            .ToList();

        if (successfulProviders.Count > 0)
        {
            var tooltip = string.Join("\n", successfulProviders);
            SetTooltip(tooltip);
            return;
        }

        var firstFailure = GetOrderedUsageData().FirstOrDefault();
        if (firstFailure == null)
        {
            SetTooltip("Usage: No data");
            return;
        }

        if (firstFailure.NeedsReauth)
        {
            var command = firstFailure.ReauthCommand ?? firstFailure.ProviderName.ToLowerInvariant();
            SetTooltip($"{firstFailure.ProviderName}: run '{command}' to re-auth");
            return;
        }

        var errorMsg = firstFailure.Error ?? "Unknown error";
        if (errorMsg.Length > 90)
            errorMsg = errorMsg[..87] + "...";
        SetTooltip($"{firstFailure.ProviderName}: {errorMsg}");
    }

    private string FormatNoDataTooltip() => _snapshot.State switch
    {
        TrayApiState.Offline => "Usage server unreachable: no cached data",
        TrayApiState.Unauthorized => "Usage API unauthorized: check API token",
        TrayApiState.Malformed => "Usage API returned malformed data",
        TrayApiState.ApiError => _snapshot.Message ?? "Usage API error",
        _ => "Usage: No data"
    };

    private string FormatApiStateTooltip()
    {
        var stale = _snapshot.LastGoodAt?.ToLocalTime().ToString("g") ?? "unknown";
        var state = _snapshot.State switch
        {
            TrayApiState.Offline => "Usage server unreachable",
            TrayApiState.Unauthorized => "Usage API unauthorized",
            TrayApiState.Malformed => "Usage API malformed response",
            _ => "Usage API error"
        };
        return $"{state}; showing stale data from {stale}";
    }

    private void SetTooltip(string tooltip) => _notifyIcon.Text = TrayVisualState.TooltipText(tooltip);

    /// <summary>
    /// Updates the popup if it's visible.
    /// </summary>
    private void UpdatePopup()
    {
        if (_popup != null && _popup.Visible)
        {
            _popup.UpdateData(GetOrderedUsageDataForDisplay());
        }
    }

    /// <summary>
    /// Updates the tray icon based on the latest usage data.
    /// Properly disposes the old icon to prevent GDI handle leaks.
    /// </summary>
    private void UpdateIcon()
    {
        Icon newIcon;

        var preferredData = GetPreferredIconData();

        newIcon = CreateIcon(TrayVisualState.IconKind(_snapshot.State, preferredData), preferredData);

        // Swap the icon and dispose the old one
        var oldIcon = _currentIcon;
        _currentIcon = newIcon;
        _notifyIcon.Icon = newIcon;

        // Dispose old icon to prevent GDI handle leaks
        if (oldIcon != null)
        {
            // Must destroy the native icon handle
            DestroyIcon(oldIcon.Handle);
            oldIcon.Dispose();
        }
    }

    private Icon CreateIcon(TrayIconKind kind, UsageData? data)
    {
        var providerName = data?.ProviderName ?? GetPrimaryProviderName();
        return kind switch
        {
            TrayIconKind.Offline => IconGenerator.GenerateOfflineIcon(providerName),
            TrayIconKind.ApiUnauthorized => IconGenerator.GenerateApiStateIcon(providerName, ApiIconKind.Unauthorized),
            TrayIconKind.ApiMalformed => IconGenerator.GenerateApiStateIcon(providerName, ApiIconKind.Malformed),
            TrayIconKind.ApiError => IconGenerator.GenerateApiStateIcon(providerName, ApiIconKind.Error),
            TrayIconKind.ProviderError => IconGenerator.GenerateErrorIcon(providerName),
            TrayIconKind.Idle => IconGenerator.GenerateAppIcon(providerName),
            TrayIconKind.Usage when data != null => IconGenerator.GenerateIcon(
                providerName,
                data.Current.Utilization,
                data.ShowSecondary ? data.Weekly.Utilization : 0,
                CalculateElapsedPercent(data.Current.ResetsAt, ResolveWindowDuration(data))),
            _ => IconGenerator.GeneratePlaceholderIcon(providerName)
        };
    }

}
