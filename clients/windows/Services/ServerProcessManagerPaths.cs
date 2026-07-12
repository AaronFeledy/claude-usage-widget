namespace ClaudeUsageWidget.Services;

public sealed partial class ServerProcessManager
{
    private string BinaryPath() => Path.Combine(
        _options.ExecutableDirectory ?? AppContext.BaseDirectory,
        _options.ServerFileName);

    private static TimeSpan Backoff(int attempt) => TimeSpan.FromMilliseconds(Math.Min(1000, 50 * (attempt + 1)));

    private static Uri LocalBaseUrl(int port) => new($"http://127.0.0.1:{port}/");

    private static NormalizedApiUrl NormalizeBaseUrl(string? apiUrl)
    {
        if (apiUrl == null || apiUrl.Length == 0)
        {
            return new NormalizedApiUrl(null, null);
        }
        if (string.IsNullOrWhiteSpace(apiUrl) || apiUrl != apiUrl.Trim())
        {
            return new NormalizedApiUrl(null, "Invalid ApiUrl setting.");
        }
        if (!Uri.TryCreate(apiUrl, UriKind.Absolute, out var uri))
        {
            return new NormalizedApiUrl(null, "Invalid ApiUrl setting.");
        }
        if (uri.Scheme is not ("http" or "https") || string.IsNullOrWhiteSpace(uri.Host) || !string.IsNullOrEmpty(uri.UserInfo) || uri.Port < -1)
        {
            return new NormalizedApiUrl(null, "Invalid ApiUrl setting.");
        }
        var normalized = uri.AbsolutePath.EndsWith('/') ? uri : new Uri(uri + "/");
        return new NormalizedApiUrl(normalized, null);
    }

    private sealed record NormalizedApiUrl(Uri? BaseUrl, string? Error);
}
