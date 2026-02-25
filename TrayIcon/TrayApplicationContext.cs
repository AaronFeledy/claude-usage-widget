using System.Drawing;
using ClaudeUsageWidget.Models;
using ClaudeUsageWidget.Services;

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

    public TrayApplicationContext(UsageApiClient usageApiClient)
    {
        _usageApiClient = usageApiClient;

        // Create the context menu
        var contextMenu = new ContextMenuStrip();
        contextMenu.Items.Add("Refresh Now", null, OnRefreshNow);
        contextMenu.Items.Add(new ToolStripSeparator());
        contextMenu.Items.Add("Quit", null, OnQuit);

        // Create the notify icon with a placeholder gray icon
        _notifyIcon = new NotifyIcon
        {
            Icon = CreatePlaceholderIcon(),
            Text = "Claude: Loading...",
            ContextMenuStrip = contextMenu,
            Visible = true
        };

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
    /// Creates a simple 16x16 solid gray placeholder icon.
    /// </summary>
    private static Icon CreatePlaceholderIcon()
    {
        using var bitmap = new Bitmap(16, 16);
        using var graphics = Graphics.FromImage(bitmap);
        
        graphics.Clear(Color.Gray);
        
        // Draw a simple border for visibility
        using var borderPen = new Pen(Color.DarkGray, 1);
        graphics.DrawRectangle(borderPen, 0, 0, 15, 15);

        return Icon.FromHandle(bitmap.GetHicon());
    }

    /// <summary>
    /// Polls the usage API and updates the tooltip.
    /// </summary>
    private async Task PollUsageAsync()
    {
        if (_isPolling) return;

        try
        {
            _isPolling = true;
            _lastUsageData = await _usageApiClient.FetchUsageAsync();
            UpdateTooltip();
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
        }
        base.Dispose(disposing);
    }
}
