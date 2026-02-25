namespace ClaudeUsageWidget.Models;

/// <summary>
/// Represents a usage bucket (5-hour or 7-day) from the API response.
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
    /// 5-hour rolling usage bucket.
    /// </summary>
    public UsageBucket FiveHour { get; set; } = new();

    /// <summary>
    /// 7-day rolling usage bucket.
    /// </summary>
    public UsageBucket SevenDay { get; set; } = new();

    /// <summary>
    /// Error message if the API call failed.
    /// </summary>
    public string? Error { get; set; }

    /// <summary>
    /// Whether the data fetch was successful.
    /// </summary>
    public bool IsSuccess => Error == null;
}
