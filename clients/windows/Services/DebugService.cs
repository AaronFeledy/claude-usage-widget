using System.Text.RegularExpressions;

namespace ClaudeUsageWidget.Services;

/// <summary>
/// Severity level for debug entries.
/// </summary>
public enum DebugLevel
{
    Info,
    Warning,
    Error
}

/// <summary>
/// Represents a single debug log entry.
/// </summary>
public class DebugEntry
{
    public DateTime Timestamp { get; init; }
    public DebugLevel Level { get; init; }
    public string Category { get; init; } = "";
    public string Message { get; init; } = "";
    public string? Details { get; init; }

    public override string ToString()
    {
        var levelStr = Level switch
        {
            DebugLevel.Error => "ERR",
            DebugLevel.Warning => "WRN",
            _ => "INF"
        };
        var details = string.IsNullOrEmpty(Details) ? "" : $"\n    {Details}";
        return $"[{Timestamp:HH:mm:ss}] [{levelStr}] [{Category}] {Message}{details}";
    }
}

/// <summary>
/// Service for collecting and distributing debug messages.
/// Thread-safe for multi-threaded access.
/// </summary>
public class DebugService
{
    private const int MaxEntries = 500;
    private static readonly Regex SecretPattern = new(
        "(?i)(authorization\\s*[:=]\\s*bearer\\s+|api[-_ ]?token\\s*[:=]\\s*|access_token\\s*[:=]\\s*|WorkosCursorSessionToken=|next-auth\\.session-token=|__Secure-next-auth\\.session-token=)[^\\s;\"']+",
        RegexOptions.Compiled);
    private readonly List<DebugEntry> _entries = new();
    private readonly object _lock = new();

    /// <summary>
    /// Event fired when a new debug entry is added.
    /// </summary>
    public event Action<DebugEntry>? EntryAdded;

    /// <summary>
    /// Gets all current debug entries.
    /// </summary>
    public IReadOnlyList<DebugEntry> GetEntries()
    {
        lock (_lock)
        {
            return _entries.ToList();
        }
    }

    /// <summary>
    /// Logs an info message.
    /// </summary>
    public void LogInfo(string category, string message, string? details = null)
    {
        Log(DebugLevel.Info, category, message, details);
    }

    /// <summary>
    /// Logs a warning message.
    /// </summary>
    public void LogWarning(string category, string message, string? details = null)
    {
        Log(DebugLevel.Warning, category, message, details);
    }

    /// <summary>
    /// Logs an error message.
    /// </summary>
    public void LogError(string category, string message, string? details = null)
    {
        Log(DebugLevel.Error, category, message, details);
    }

    /// <summary>
    /// Logs a message with the specified level.
    /// </summary>
    public void Log(DebugLevel level, string category, string message, string? details = null)
    {
        var entry = new DebugEntry
        {
            Timestamp = DateTime.Now,
            Level = level,
            Category = category,
            Message = Redact(message)!,
            Details = Redact(details)
        };

        lock (_lock)
        {
            _entries.Add(entry);

            // Trim old entries if we exceed max
            while (_entries.Count > MaxEntries)
            {
                _entries.RemoveAt(0);
            }
        }

        // Fire event on UI thread if possible
        EntryAdded?.Invoke(entry);
    }

    /// <summary>
    /// Clears all debug entries.
    /// </summary>
    public void Clear()
    {
        lock (_lock)
        {
            _entries.Clear();
        }
    }

    public static string? Redact(string? value)
    {
        if (string.IsNullOrEmpty(value))
        {
            return value;
        }

        return SecretPattern.Replace(value, match => match.Groups[1].Value + "[redacted]");
    }
}
