using System.Drawing;
using ClaudeUsageWidget.Models;
using ClaudeUsageWidget.Services;
using ClaudeUsageWidget.UI;

namespace ClaudeUsageWidget.TrayIcon;

public partial class TrayApplicationContext
{
    private static float CalculateElapsedPercent(DateTime? resetsAt, TimeSpan windowDuration)
    {
        if (resetsAt == null) return -1;
        var remaining = resetsAt.Value - DateTime.UtcNow;
        if (remaining <= TimeSpan.Zero) return 100;
        var elapsed = windowDuration - remaining;
        return elapsed <= TimeSpan.Zero ? 0 : (float)(elapsed / windowDuration * 100);
    }

    [System.Runtime.InteropServices.DllImport("user32.dll", CharSet = System.Runtime.InteropServices.CharSet.Auto)]
    private static extern bool DestroyIcon(IntPtr handle);

    private IReadOnlyList<string> GetProviderPreferenceOrder()
    {
        var primary = SettingsService.NormalizeProviderName(_settingsService.Settings.PrimaryProvider);
        return DefaultProviderOrder.OrderByDescending(name => name == primary).ThenBy(name => Array.IndexOf(DefaultProviderOrder, name)).ToList();
    }

    private IReadOnlyList<UsageData> GetOrderedUsageData() => GetProviderPreferenceOrder()
        .Where(name => _providerUsage.ContainsKey(name)).Select(name => _providerUsage[name]).ToList();

    private IReadOnlyList<UsageData> GetOrderedUsageDataForDisplay()
    {
        var ordered = GetOrderedUsageData();
        return _snapshot.State == TrayApiState.Ready || ordered.Count == 0 ? ordered : ordered.Select(CloneForStaleDisplay).ToList();
    }

    private UsageData CloneForStaleDisplay(UsageData data)
    {
        var stale = _snapshot.LastGoodAt?.ToLocalTime().ToString("g") ?? "unknown";
        var message = _snapshot.State == TrayApiState.Offline ? $"Stale: server offline since {stale}" : $"Stale: {_snapshot.Message ?? _snapshot.State.ToString()}";
        return new UsageData
        {
            ProviderName = data.ProviderName,
            PrimaryLabel = data.PrimaryLabel,
            SecondaryLabel = data.SecondaryLabel,
            ShowSecondary = data.ShowSecondary,
            Subtitle = data.Subtitle,
            ReauthCommand = data.ReauthCommand,
            Current = data.Current,
            Weekly = data.Weekly,
            PrimaryStatusText = message,
            SecondaryStatusText = data.SecondaryStatusText
        };
    }

    private UsageData? GetPreferredIconData()
    {
        foreach (var providerName in GetProviderPreferenceOrder())
        {
            if (_providerUsage.TryGetValue(providerName, out var data) && data.IsSuccess) return data;
        }
        return GetOrderedUsageData().FirstOrDefault();
    }

    private UsageData? GetPreferredNotificationData() => GetPreferredIconData();

    private void ApplyRetryInterval()
    {
        var baseInterval = Math.Max(1, _settingsService.Settings.RefreshIntervalSeconds) * 1000;
        if (_snapshot.State != TrayApiState.Offline || _snapshot.RetryAttempt <= 0)
        {
            _pollTimer.Interval = baseInterval;
            return;
        }
        var multiplier = 1 << Math.Min(_snapshot.RetryAttempt, 5);
        _pollTimer.Interval = Math.Min(baseInterval * multiplier, 300_000);
    }

    private string GetPrimaryProviderName() => SettingsService.NormalizeProviderName(_settingsService.Settings.PrimaryProvider);

    private static string FormatProviderTooltip(UsageData data)
    {
        var primary = $"{data.ProviderName}: {data.Current.Utilization:F0}% ({data.Current.TimeUntilReset})";
        return data.ShowSecondary ? $"{primary} | {data.Weekly.Utilization:F0}%" : primary;
    }

    private static TimeSpan ResolveWindowDuration(UsageData data) => data.ProviderName switch
    {
        "Claude" => TimeSpan.FromHours(5),
        "Codex" => TimeSpan.FromHours(5),
        "Cursor" => TimeSpan.FromDays(30),
        "Grok" => ResolveMonthEndWindowDuration(data.Current.ResetsAt),
        _ => TimeSpan.FromHours(5)
    };

    private static TimeSpan ResolveMonthEndWindowDuration(DateTime? resetsAt)
    {
        if (resetsAt == null) return TimeSpan.FromDays(30);
        var duration = resetsAt.Value - resetsAt.Value.AddMonths(-1);
        return duration > TimeSpan.Zero ? duration : TimeSpan.FromDays(30);
    }
}
