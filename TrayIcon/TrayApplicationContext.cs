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
    private const int PollIntervalMs = 60_000; // 60 seconds

    private readonly UsageApiClient _usageApiClient;
    private readonly NotifyIcon _notifyIcon;
    private readonly System.Windows.Forms.Timer _pollTimer;

    private UsageData? _lastUsageData;
    private bool _isPolling;
    private Icon? _currentIcon;
    private UsagePopup? _popup;

    public TrayApplicationContext(UsageApiClient usageApiClient)
    {
        _usageApiClient = usageApiClient;

        // Create the context menu
        var contextMenu = new ContextMenuStrip();
        contextMenu.Items.Add("Refresh Now", null, OnRefreshNow);
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

        // Wire up left-click to show popup
        _notifyIcon.MouseClick += OnNotifyIconClick;

        // Set up the polling timer
        _pollTimer = new System.Windows.Forms.Timer
        {
            Interval = PollIntervalMs
        };
        _pollTimer.Tick += async (_, _) => await PollUsageAsync();
        _pollTimer.Start();

        // Do an initial poll
        _ = PollUsageAsync();
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

        var fiveHour = _lastUsageData.FiveHour;
        var sevenDay = _lastUsageData.SevenDay;

        // Format: "Claude: 5h 42% (2h 18m) | 7d 31%"
        var tooltip = $"Claude: 5h {fiveHour.Utilization:F0}% ({fiveHour.TimeUntilReset}) | 7d {sevenDay.Utilization:F0}%";

        // Add warning if weekly > 70%
        if (sevenDay.Utilization > 70)
        {
            var resetDay = GetResetDayName(sevenDay.ResetsAt);
            tooltip += $"\n\u26a0 Weekly: {sevenDay.Utilization:F0}% (resets {resetDay})";
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
                _lastUsageData.FiveHour.Utilization,
                _lastUsageData.SevenDay.Utilization);
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
            _popup.OnRefreshClicked += async (_, _) => await PollUsageAsync();
            _popup.OnQuitClicked += OnQuit;
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
    /// Handles the "Refresh Now" menu click.
    /// </summary>
    private async void OnRefreshNow(object? sender, EventArgs e)
    {
        await PollUsageAsync();
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
