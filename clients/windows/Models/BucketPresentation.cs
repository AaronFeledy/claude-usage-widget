namespace ClaudeUsageWidget.Models;

/// <summary>
/// Presentation policy for usage meter rows, kept free of WinForms so it can be
/// unit-tested on any host. A meter is "status-only" when it carries a status
/// message but has no truthful measured utilization to plot — e.g. an enabled,
/// cap-only, or used-only pay-as-you-go meter reported with zero utilization.
/// Status-only rows show their section label and status line but hide the percent
/// label and progress bar, so the tray never paints a misleading 0% bar. A
/// truthful used/cap ratio (its status carries the " / " separator) is measured
/// even at zero, so it keeps the percent label and 0% progress bar. Measured rows
/// with utilization above zero always keep the percent label and progress bar.
/// Header status fields are a fallback only: primary text applies to Other Models
/// (api) or the first non-auto row, and secondary text applies to weekly /
/// on-demand meters. Cursor Models (auto) keep a reset countdown.
/// </summary>
public static class BucketPresentation
{
    public static bool IsStatusOnly(UsageBucketDetail bucket)
    {
        return string.Equals(bucket.Id, "on_demand", StringComparison.OrdinalIgnoreCase)
            && bucket.Utilization <= 0f
            && !string.IsNullOrWhiteSpace(bucket.StatusText)
            && !IsMeasuredRatioStatus(bucket.StatusText);
    }

    /// <summary>
    /// Prefers <see cref="UsageBucketDetail.StatusText"/>. When that is empty,
    /// uses <see cref="UsageData.PrimaryStatusText"/> for Other Models or the
    /// first non-auto row, and <see cref="UsageData.SecondaryStatusText"/> for
    /// weekly / on-demand meters. Returns null when the caller should render a
    /// reset countdown instead — including Cursor Models.
    /// </summary>
    public static string? ResolveStatusOverride(UsageData data, int index, UsageBucketDetail bucket)
    {
        if (!string.IsNullOrWhiteSpace(bucket.StatusText))
            return bucket.StatusText;

        if (IsPrimaryStatusFallback(index, bucket) && !string.IsNullOrWhiteSpace(data.PrimaryStatusText))
            return data.PrimaryStatusText;

        if (IsSecondaryHeaderBucket(bucket.Id) && !string.IsNullOrWhiteSpace(data.SecondaryStatusText))
            return data.SecondaryStatusText;

        return null;
    }

    private static bool IsPrimaryStatusFallback(int index, UsageBucketDetail bucket)
    {
        if (string.Equals(bucket.Id, "auto", StringComparison.OrdinalIgnoreCase))
            return false;
        if (string.Equals(bucket.Id, "api", StringComparison.OrdinalIgnoreCase))
            return true;
        return index == 0;
    }

    private static bool IsMeasuredRatioStatus(string? statusText)
    {
        return statusText != null && statusText.Contains(" / ", StringComparison.Ordinal);
    }

    private static bool IsSecondaryHeaderBucket(string id)
    {
        return string.Equals(id, "weekly", StringComparison.OrdinalIgnoreCase)
            || string.Equals(id, "on_demand", StringComparison.OrdinalIgnoreCase);
    }
}
