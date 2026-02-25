using ClaudeUsageWidget.Models;

namespace ClaudeUsageWidget.Services;

/// <summary>
/// Manages toast notifications for usage thresholds.
/// </summary>
public class NotificationService
{
    private const float WarningThreshold = 80f;
    private const float CriticalThreshold = 95f;

    private readonly NotifyIcon _notifyIcon;
    private readonly SettingsService _settingsService;

    private bool _notifiedWarning;
    private bool _notifiedCritical;

    public NotificationService(NotifyIcon notifyIcon, SettingsService settingsService)
    {
        _notifyIcon = notifyIcon;
        _settingsService = settingsService;
    }

    /// <summary>
    /// Checks utilization and shows notifications at 80% and 95% thresholds.
    /// Resets notification state when usage drops below thresholds.
    /// </summary>
    public void CheckAndNotify(UsageData? data)
    {
        if (data == null || !data.IsSuccess)
            return;

        if (!_settingsService.Settings.NotificationsEnabled)
            return;

        var utilization = data.Current.Utilization;

        // Reset flags when usage drops below thresholds
        if (utilization < WarningThreshold)
        {
            _notifiedWarning = false;
            _notifiedCritical = false;
            return;
        }

        if (utilization < CriticalThreshold)
        {
            _notifiedCritical = false;
        }

        // Show critical notification at 95%
        if (utilization >= CriticalThreshold && !_notifiedCritical)
        {
            _notifiedCritical = true;
            _notifiedWarning = true; // Also mark warning as shown
            ShowNotification(
                "Claude Usage Critical",
                "Current session usage at 95%. You are about to hit the rate limit.",
                ToolTipIcon.Error);
            return;
        }

        // Show warning notification at 80%
        if (utilization >= WarningThreshold && !_notifiedWarning)
        {
            _notifiedWarning = true;
            ShowNotification(
                "Claude Usage Warning",
                "Current session usage at 80%. Consider pacing.",
                ToolTipIcon.Warning);
        }
    }

    /// <summary>
    /// Shows a Windows balloon tip notification.
    /// </summary>
    private void ShowNotification(string title, string text, ToolTipIcon icon)
    {
        _notifyIcon.ShowBalloonTip(5000, title, text, icon);
    }
}
