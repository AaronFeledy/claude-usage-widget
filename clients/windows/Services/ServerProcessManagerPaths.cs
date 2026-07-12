namespace ClaudeUsageWidget.Services;

public sealed partial class ServerProcessManager
{
    private string BinaryPath() => Path.Combine(
        _options.ExecutableDirectory ?? AppContext.BaseDirectory,
        _options.ServerFileName);

    private static TimeSpan Backoff(int attempt) => TimeSpan.FromMilliseconds(Math.Min(1000, 50 * (attempt + 1)));

    private static Uri LocalBaseUrl(int port) => new($"http://127.0.0.1:{port}/");

    private static Uri? NormalizeBaseUrl(string? apiUrl)
    {
        if (string.IsNullOrWhiteSpace(apiUrl))
        {
            return null;
        }
        var uri = new Uri(apiUrl.Trim(), UriKind.Absolute);
        return uri.AbsolutePath.EndsWith('/') ? uri : new Uri(uri + "/");
    }
}
