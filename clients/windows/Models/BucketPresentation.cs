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

    private static bool IsMeasuredRatioStatus(string? statusText)
    {
        return statusText != null && statusText.Contains(" / ", StringComparison.Ordinal);
    }
}
