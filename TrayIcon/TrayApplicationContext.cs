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
    private readonly UsageApiClient _usageApiClient;
    private readonly SettingsService _settingsService;
    private readonly NotificationService _notificationService;
    private readonly UpdateService _updateService;
    private readonly NotifyIcon _notifyIcon;
    private readonly System.Windows.Forms.Timer _pollTimer;

    private UsageData? _lastUsageData;
    private bool _isPolling;
    private Icon? _currentIcon;
    private UsagePopup? _popup;

    public TrayApplicationContext(UsageApiClient usageApiClient, SettingsService settingsService, HttpClient httpClient)
    {
        _usageApiClient = usageApiClient;
        _settingsService = settingsService;

        // Create the context menu
        var contextMenu = new ContextMenuStrip();
        contextMenu.Items.Add("Refresh Now", null, OnRefreshNow);
        contextMenu.Items.Add("Check for Updates", null, OnCheckForUpdates);
        contextMenu.Items.Add(new ToolStripSeparator());
        contextMenu.Items.Add("Quit", null, OnQuit);

        // Create the notify icon with a placeholder icon
        _currentIcon = IconGenerator.GeneratePlaceholderIcon();
        _notifyIcon = new NotifyIcon
        {
            Icon = _currentIcon,
            Text = "Claude: Loading...",
            ContextMenuStrip = contextMenu,
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
            _lastUsageData = await _usageApiClient.FetchUsageAsync();
            UpdateTooltip();
            UpdateIcon();
            UpdatePopup();

            // Check and show notifications if thresholds are reached
            _notificationService.CheckAndNotify(_lastUsageData);
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
        if (_lastUsageData == null)
        {
            _notifyIcon.Text = "Claude: No data";
            return;
        }

        if (!_lastUsageData.IsSuccess)
        {
            // Truncate error message to fit tooltip (max 127 chars)
            var errorMsg = _lastUsageData.Error ?? "Unknown error";
            if (errorMsg.Length > 100)
            {
                errorMsg = errorMsg[..97] + "...";
            }
            _notifyIcon.Text = $"Claude: Error - {errorMsg}";
            return;
        }

        var current = _lastUsageData.Current;
        var weekly = _lastUsageData.Weekly;

        // Format: "Claude: Current 42% (2h 18m) | Weekly 31%"
        var tooltip = $"Claude: {current.Utilization:F0}% ({current.TimeUntilReset}) · w:{weekly.Utilization:F0}%";

        // Add warning if weekly > 70%
        if (weekly.Utilization > 70)
        {
            var resetDay = GetResetDayName(weekly.ResetsAt);
            tooltip += $"\n\u26a0 Weekly: {weekly.Utilization:F0}% (resets {resetDay})";
        }

        // NotifyIcon.Text has a max length of 127 characters
        if (tooltip.Length > 127)
        {
            tooltip = tooltip[..127];
        }

        _notifyIcon.Text = tooltip;
    }

    /// <summary>
    /// Updates the popup if it's visible.
    /// </summary>
    private void UpdatePopup()
    {
        if (_popup != null && _popup.Visible)
        {
            _popup.UpdateData(_lastUsageData);
        }
    }

    /// <summary>
    /// Updates the tray icon based on the latest usage data.
    /// Properly disposes the old icon to prevent GDI handle leaks.
    /// </summary>
    private void UpdateIcon()
    {
        Icon newIcon;

        if (_lastUsageData == null)
        {
            newIcon = IconGenerator.GeneratePlaceholderIcon();
        }
        else if (!_lastUsageData.IsSuccess)
        {
            newIcon = IconGenerator.GenerateErrorIcon();
        }
        else
        {
            newIcon = IconGenerator.GenerateIcon(
                _lastUsageData.Current.Utilization,
                _lastUsageData.Weekly.Utilization);
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
    /// Native method to destroy icon handles created with GetHicon().
    /// </summary>
    [System.Runtime.InteropServices.DllImport("user32.dll", CharSet = System.Runtime.InteropServices.CharSet.Auto)]
    private static extern bool DestroyIcon(IntPtr handle);

    /// <summary>
    /// Gets the abbreviated day name for when usage resets.
    /// </summary>
    private static string GetResetDayName(DateTime? resetsAt)
    {
        if (resetsAt == null) return "?";

        var localReset = resetsAt.Value.ToLocalTime();
        var today = DateTime.Today;

        if (localReset.Date == today)
            return "today";
        if (localReset.Date == today.AddDays(1))
            return "tomorrow";

        return localReset.ToString("ddd");
    }

    /// <summary>
    /// Handles left-click on the tray icon to toggle the popup.
    /// </summary>
    private void OnNotifyIconClick(object? sender, MouseEventArgs e)
    {
        if (e.Button != MouseButtons.Left)
            return;

        if (_popup == null)
        {
            _popup = new UsagePopup();
            _popup.SetSettingsService(_settingsService);
            _popup.OnRefreshClicked += async (_, _) => await PollUsageAsync();
            _popup.OnQuitClicked += OnQuit;
            _popup.OnSettingsChanged += OnSettingsChanged;
        }

        if (_popup.Visible)
        {
            _popup.Hide();
        }
        else
        {
            _popup.UpdateData(_lastUsageData);
            _popup.PositionNearTray();
            _popup.Show();
            _popup.Activate();
        }
    }

    /// <summary>
    /// Handles settings changes from the popup.
    /// </summary>
    private void OnSettingsChanged(object? sender, EventArgs e)
    {
        // Update the timer interval when refresh interval changes
        _pollTimer.Interval = _settingsService.Settings.RefreshIntervalSeconds * 1000;
    }

    /// <summary>
    /// Handles the "Refresh Now" menu click.
    /// </summary>
    private async void OnRefreshNow(object? sender, EventArgs e)
    {
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
