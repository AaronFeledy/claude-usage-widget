namespace ClaudeUsageWidget.Models;

/// <summary>
/// Represents a usage bucket (Current session or weekly) from the API response.
/// </summary>
public class UsageBucket
{
    /// <summary>
    /// Utilization percentage (0-100).
    /// </summary>
    public float Utilization { get; set; }

    /// <summary>
    /// When this usage bucket resets.
    /// </summary>
    public DateTime? ResetsAt { get; set; }

    /// <summary>
    /// Human-readable time until reset (e.g., "2h 18m").
    /// </summary>
    public string TimeUntilReset
    {
        get
        {
            if (ResetsAt == null)
                return "Unknown";

            var remaining = ResetsAt.Value - DateTime.UtcNow;

            if (remaining <= TimeSpan.Zero)
                return "Now";

            if (remaining.TotalDays >= 1)
            {
                var days = (int)remaining.TotalDays;
                var hours = remaining.Hours;
                return hours > 0 ? $"{days}d {hours}h" : $"{days}d";
            }

            if (remaining.TotalHours >= 1)
            {
                var hours = (int)remaining.TotalHours;
                var minutes = remaining.Minutes;
                return minutes > 0 ? $"{hours}h {minutes}m" : $"{hours}h";
            }

            return $"{remaining.Minutes}m";
        }
    }
}

/// <summary>
/// Root usage data model returned by the API.
/// </summary>
public class UsageData
{
    /// <summary>
    /// Current session rolling usage bucket.
    /// </summary>
    public UsageBucket Current { get; set; } = new();

    /// <summary>
    /// weekly rolling usage bucket.
    /// </summary>
    public UsageBucket Weekly { get; set; } = new();

    /// <summary>
    /// Error message if the API call failed.
    /// </summary>
    public string? Error { get; set; }

    /// <summary>
    /// Whether the user needs to re-authenticate (refresh token invalid).
    /// </summary>
    public bool NeedsReauth { get; set; }

    /// <summary>
    /// Whether the data fetch was successful.
    /// </summary>
    public bool IsSuccess => Error == null;
}
