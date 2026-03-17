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

    private readonly Dictionary<string, NotificationState> _providerStates = new(StringComparer.OrdinalIgnoreCase);

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

        var state = GetState(data.ProviderName);
        var utilization = data.Current.Utilization;

        // Reset flags when usage drops below thresholds
        if (utilization < WarningThreshold)
        {
            state.NotifiedWarning = false;
            state.NotifiedCritical = false;
            return;
        }

        if (utilization < CriticalThreshold)
        {
            state.NotifiedCritical = false;
        }

        // Show critical notification at 95%
        if (utilization >= CriticalThreshold && !state.NotifiedCritical)
        {
            state.NotifiedCritical = true;
            state.NotifiedWarning = true;
            ShowNotification(
                $"{data.ProviderName} Usage Critical",
                $"{data.PrimaryLabel} usage is at {utilization:F0}%. You are about to hit the limit.",
                ToolTipIcon.Error);
            return;
        }

        // Show warning notification at 80%
        if (utilization >= WarningThreshold && !state.NotifiedWarning)
        {
            state.NotifiedWarning = true;
            ShowNotification(
                $"{data.ProviderName} Usage Warning",
                $"{data.PrimaryLabel} usage is at {utilization:F0}%. Consider pacing.",
                ToolTipIcon.Warning);
        }
    }

    private NotificationState GetState(string providerName)
    {
        if (!_providerStates.TryGetValue(providerName, out var state))
        {
            state = new NotificationState();
            _providerStates[providerName] = state;
        }

        return state;
    }

    /// <summary>
    /// Shows a Windows balloon tip notification.
    /// </summary>
    private void ShowNotification(string title, string text, ToolTipIcon icon)
    {
        _notifyIcon.ShowBalloonTip(5000, title, text, icon);
    }

    private sealed class NotificationState
    {
        public bool NotifiedWarning { get; set; }
        public bool NotifiedCritical { get; set; }
    }
}
