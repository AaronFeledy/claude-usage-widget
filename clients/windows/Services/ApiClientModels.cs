namespace ClaudeUsageWidget.Services;

public interface ITrayApiClientSettings
{
    Uri BaseUrl { get; }
    string? BearerToken { get; }
    TimeSpan Timeout { get; }
}

public sealed class TrayApiClientOptions : ITrayApiClientSettings
{
    public Uri BaseUrl { get; init; } = new("http://127.0.0.1:7823");
    public string? BearerToken { get; init; }
    public TimeSpan Timeout { get; init; } = TimeSpan.FromSeconds(10);
}

public enum ApiResultStatus
{
    Success,
    ProviderNotFound,
    Unauthorized,
    ApiError,
    MalformedResponse,
    Offline
}

public sealed class ApiResult<T>
{
    private ApiResult(ApiResultStatus status, T? value, string? error)
    {
        Status = status;
        Value = value;
        Error = error;
    }

    public ApiResultStatus Status { get; }
    public T? Value { get; }
    public string? Error { get; }
    public bool IsSuccess => Status == ApiResultStatus.Success;

    public static ApiResult<T> Success(T value) => new(ApiResultStatus.Success, value, null);
    public static ApiResult<T> Failure(ApiResultStatus status, string error) => new(status, default, error);
}

public sealed class CursorCredential
{
    private CursorCredential(string? cookie, string? accessToken)
    {
        Cookie = cookie;
        AccessToken = accessToken;
    }

    public string? Cookie { get; }
    public string? AccessToken { get; }

    public static CursorCredential FromCookie(string cookie) => new(RequireValue(cookie, nameof(cookie)), null);
    public static CursorCredential FromAccessToken(string accessToken) => new(null, RequireValue(accessToken, nameof(accessToken)));

    private static string RequireValue(string value, string name)
    {
        var trimmed = value.Trim();
        if (trimmed.Length == 0) throw new ArgumentException("Credential must not be empty.", name);
        return trimmed;
    }
}

public sealed record CursorCredentialResult(string Provider, bool Refetched, Models.UsageData Usage);
