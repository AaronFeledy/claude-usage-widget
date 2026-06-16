using System.Drawing;
using ClaudeUsageWidget.Models;
using ClaudeUsageWidget.Services;
using ClaudeUsageWidget.UI;

namespace ClaudeUsageWidget.TrayIcon;

/// <summary>
/// Application context that manages the system tray icon and polling.
/// </summary>
public class TrayApplicationContext : ApplicationContext
{
    private static readonly string[] DefaultProviderOrder = ["Claude", "Codex", "Cursor", "Grok"];

    private readonly UsageApiClient _usageApiClient;
    private readonly CodexCredentialService _codexCredentialService;
    private readonly CodexUsageClient _codexUsageClient;
    private readonly CursorUsageClient _cursorUsageClient;
    private readonly GrokCredentialService _grokCredentialService;
    private readonly GrokUsageClient _grokUsageClient;
    private readonly CredentialService _credentialService;
    private readonly SettingsService _settingsService;
    private readonly NotificationService _notificationService;
    private readonly UpdateService _updateService;
    private readonly DebugService _debugService;
    private readonly NotifyIcon _notifyIcon;
    private readonly System.Windows.Forms.Timer _pollTimer;
    private readonly ContextMenuStrip _contextMenu;

    private readonly Dictionary<string, UsageData> _providerUsage = new(StringComparer.OrdinalIgnoreCase);
    private bool _isPolling;
    private Icon? _currentIcon;
    private UsagePopup? _popup;
    private DebugConsole? _debugConsole;

    public TrayApplicationContext(UsageApiClient usageApiClient, CodexCredentialService codexCredentialService, CodexUsageClient codexUsageClient, CursorUsageClient cursorUsageClient, GrokCredentialService grokCredentialService, GrokUsageClient grokUsageClient, CredentialService credentialService, SettingsService settingsService, DebugService debugService, HttpClient httpClient)
    {
        _usageApiClient = usageApiClient;
        _codexCredentialService = codexCredentialService;
        _codexUsageClient = codexUsageClient;
        _cursorUsageClient = cursorUsageClient;
        _grokCredentialService = grokCredentialService;
        _grokUsageClient = grokUsageClient;
        _credentialService = credentialService;
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
            Text = "Claude: Loading...",
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

            // If last attempt needed reauth, reload credentials from disk
            // in case the user has re-authenticated externally
            if (GetProviderData("Claude")?.NeedsReauth == true)
                _credentialService.ReloadCredentials();
            if (GetProviderData("Codex")?.NeedsReauth == true)
                _codexCredentialService.ReloadCredentials();
            if (GetProviderData("Grok")?.NeedsReauth == true)
            {
                try { _grokCredentialService.ReloadCredentials(); } catch (FileNotFoundException) { }
            }

            var fetchTasks = new Task<UsageData>[]
            {
                _usageApiClient.FetchUsageAsync(),
                _codexUsageClient.FetchUsageAsync(),
                _cursorUsageClient.FetchUsageAsync(),
                _grokUsageClient.FetchUsageAsync()
            };

            var results = await Task.WhenAll(fetchTasks);
            _providerUsage.Clear();
            foreach (var result in results)
            {
                _providerUsage[result.ProviderName] = result;
            }

            UpdateTooltip();
            UpdateIcon();
            UpdatePopup();

            // Check and show notifications if thresholds are reached
            _notificationService.CheckAndNotify(GetPreferredNotificationData());
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
            _notifyIcon.Text = "Usage: No data";
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
            _notifyIcon.Text = tooltip.Length > 127 ? tooltip[..127] : tooltip;
            return;
        }

        var firstFailure = GetOrderedUsageData().FirstOrDefault();
        if (firstFailure == null)
        {
            _notifyIcon.Text = "Usage: No data";
            return;
        }

        if (firstFailure.NeedsReauth)
        {
            var command = firstFailure.ReauthCommand ?? firstFailure.ProviderName.ToLowerInvariant();
            _notifyIcon.Text = $"{firstFailure.ProviderName}: run '{command}' to re-auth";
            return;
        }

        var errorMsg = firstFailure.Error ?? "Unknown error";
        if (errorMsg.Length > 90)
            errorMsg = errorMsg[..87] + "...";
        _notifyIcon.Text = $"{firstFailure.ProviderName}: {errorMsg}";
    }

    /// <summary>
    /// Updates the popup if it's visible.
    /// </summary>
    private void UpdatePopup()
    {
        if (_popup != null && _popup.Visible)
        {
            _popup.UpdateData(GetOrderedUsageData());
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

        if (preferredData == null)
        {
            newIcon = IconGenerator.GeneratePlaceholderIcon(GetPrimaryProviderName());
        }
        else if (!preferredData.IsSuccess)
        {
            newIcon = IconGenerator.GenerateErrorIcon(preferredData.ProviderName);
        }
        else if (preferredData.Current.Utilization < 1f)
        {
            newIcon = IconGenerator.GenerateAppIcon(preferredData.ProviderName);
        }
        else
        {
            var burnRate = CalculateElapsedPercent(preferredData.Current.ResetsAt, ResolveWindowDuration(preferredData));
            newIcon = IconGenerator.GenerateIcon(
                preferredData.ProviderName,
                preferredData.Current.Utilization,
                preferredData.ShowSecondary ? preferredData.Weekly.Utilization : 0,
                burnRate);
        }

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

    /// <summary>
    /// Calculates what percentage of the window has elapsed (0-100). Returns -1 if unknown.
    /// </summary>
    private static float CalculateElapsedPercent(DateTime? resetsAt, TimeSpan windowDuration)
    {
        if (resetsAt == null)
            return -1;

        var remaining = resetsAt.Value - DateTime.UtcNow;
        if (remaining <= TimeSpan.Zero)
            return 100;

        var elapsed = windowDuration - remaining;
        if (elapsed <= TimeSpan.Zero)
            return 0;

        return (float)(elapsed / windowDuration * 100);
    }

    /// <summary>
    /// Native method to destroy icon handles created with GetHicon().
    /// </summary>
    [System.Runtime.InteropServices.DllImport("user32.dll", CharSet = System.Runtime.InteropServices.CharSet.Auto)]
    private static extern bool DestroyIcon(IntPtr handle);

    /// <summary>
    /// Handles left-click on the tray icon to toggle the popup.
    /// </summary>
    private void OnNotifyIconClick(object? sender, MouseEventArgs e)
    {
        if (e.Button != MouseButtons.Left)
            return;

        if (_popup != null && _popup.Visible)
        {
            _popup.Hide();
        }
        else
        {
            EnsurePopupVisible();
        }
    }

    /// <summary>
    /// Handles settings changes from the popup.
    /// </summary>
    private void OnSettingsChanged(object? sender, EventArgs e)
    {
        // Update the timer interval when refresh interval changes
        _pollTimer.Interval = _settingsService.Settings.RefreshIntervalSeconds * 1000;
        UpdateTooltip();
        UpdateIcon();
        UpdatePopup();
    }

    /// <summary>
    /// Ensures the popup exists and is visible, creating it if needed.
    /// </summary>
    private void EnsurePopupVisible()
    {
        if (_popup == null)
        {
            _popup = new UsagePopup();
            _popup.SetSettingsService(_settingsService);
            _popup.OnSettingsChanged += OnSettingsChanged;
        }

        _popup.UpdateData(GetOrderedUsageData());
        _popup.PositionNearTray();
        _popup.Show();
        _popup.Activate();
    }

    private IReadOnlyList<string> GetProviderPreferenceOrder()
    {
        var primary = SettingsService.NormalizeProviderName(_settingsService.Settings.PrimaryProvider);
        return DefaultProviderOrder
            .OrderByDescending(name => name == primary)
            .ThenBy(name => Array.IndexOf(DefaultProviderOrder, name))
            .ToList();
    }

    private IReadOnlyList<UsageData> GetOrderedUsageData()
    {
        return GetProviderPreferenceOrder()
            .Where(name => _providerUsage.ContainsKey(name))
            .Select(name => _providerUsage[name])
            .ToList();
    }

    private UsageData? GetProviderData(string providerName)
    {
        return _providerUsage.TryGetValue(providerName, out var data) ? data : null;
    }

    private UsageData? GetPreferredIconData()
    {
        foreach (var providerName in GetProviderPreferenceOrder())
        {
            if (_providerUsage.TryGetValue(providerName, out var data) && data.IsSuccess)
                return data;
        }

        return GetOrderedUsageData().FirstOrDefault();
    }

    private UsageData? GetPreferredNotificationData()
    {
        return GetPreferredIconData();
    }

    private string GetPrimaryProviderName()
    {
        return SettingsService.NormalizeProviderName(_settingsService.Settings.PrimaryProvider);
    }

    private static string FormatProviderTooltip(UsageData data)
    {
        var primary = $"{data.ProviderName}: {data.Current.Utilization:F0}% ({data.Current.TimeUntilReset})";
        if (!data.ShowSecondary)
            return primary;

        return $"{primary} | {data.Weekly.Utilization:F0}%";
    }

    private static TimeSpan ResolveWindowDuration(UsageData data)
    {
        return data.ProviderName switch
        {
            "Claude" => TimeSpan.FromHours(5),
            "Codex" => TimeSpan.FromHours(5),
            "Cursor" => TimeSpan.FromDays(30),
            "Grok" => ResolveMonthEndWindowDuration(data.Current.ResetsAt),
            _ => TimeSpan.FromHours(5)
        };
    }

    private static TimeSpan ResolveMonthEndWindowDuration(DateTime? resetsAt)
    {
        return TimeSpan.FromDays(30);
    }

    /// <summary>
    /// Handles the "Settings" menu click — opens popup with settings expanded.
    /// </summary>
    private void OnSettingsClicked(object? sender, EventArgs e)
    {
        EnsurePopupVisible();
        _popup!.ShowSettings();
    }

    /// <summary>
    /// Handles the "Refresh Now" menu click.
    /// </summary>
    private async void OnRefreshNow(object? sender, EventArgs e)
    {
        // Reload credentials from disk in case user re-authenticated
        _credentialService.ReloadCredentials();
        _codexCredentialService.ReloadCredentials();
        try
        {
            _grokCredentialService.ReloadCredentials();
        }
        catch (FileNotFoundException)
        {
        }
        await PollUsageAsync();
    }

    /// <summary>
    /// Handles the "Check for Updates" menu click.
    /// </summary>
    private async void OnCheckForUpdates(object? sender, EventArgs e)
    {
        await _updateService.CheckForUpdatesAsync(showNotificationIfNoUpdate: true);
    }

    /// <summary>
    /// Handles the "Debug Console" menu click.
    /// </summary>
    private void OnDebugConsoleClicked(object? sender, EventArgs e)
    {
        if (_debugConsole == null)
        {
            _debugConsole = new DebugConsole(_debugService);
        }

        if (_debugConsole.Visible)
        {
            _debugConsole.BringToFront();
            _debugConsole.Activate();
        }
        else
        {
            _debugConsole.Show();
        }
    }

    /// <summary>
    /// Handles the "Quit" menu click.
    /// </summary>
    private void OnQuit(object? sender, EventArgs e)
    {
        _pollTimer.Stop();
        _pollTimer.Dispose();
        _notifyIcon.Visible = false;
        _notifyIcon.Dispose();
        Application.Exit();
    }

    /// <summary>
    /// Clean up resources when the context is disposed.
    /// </summary>
    protected override void Dispose(bool disposing)
    {
        if (disposing)
        {
            _pollTimer.Stop();
            _pollTimer.Dispose();
            _notifyIcon.Visible = false;
            _notifyIcon.Dispose();

            // Clean up the popup
            if (_popup != null)
            {
                _popup.Close();
                _popup.Dispose();
                _popup = null;
            }

            // Clean up the debug console
            if (_debugConsole != null)
            {
                _debugConsole.Close();
                _debugConsole.Dispose();
                _debugConsole = null;
            }

            // Clean up the current icon
            if (_currentIcon != null)
            {
                DestroyIcon(_currentIcon.Handle);
                _currentIcon.Dispose();
                _currentIcon = null;
            }
        }
        base.Dispose(disposing);
    }
}
