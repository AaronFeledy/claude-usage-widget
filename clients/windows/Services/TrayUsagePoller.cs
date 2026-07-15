using ClaudeUsageWidget.Models;

namespace ClaudeUsageWidget.Services;

public enum TrayApiState
{
    Loading,
    Ready,
    Offline,
    Unauthorized,
    ApiError,
    Malformed
}

public sealed record TrayUsageSnapshot(
    TrayApiState State,
    IReadOnlyList<UsageData> Providers,
    DateTime? LastGoodAt,
    string? Message,
    int RetryAttempt)
{
    public bool IsStale => State != TrayApiState.Ready && LastGoodAt != null;
}

public interface ICursorCookieReader
{
    string? ReadCursorCookieHeader();
}

public sealed class TrayUsagePoller
{
    private readonly ApiClient _apiClient;
    private readonly ServerProcessManager _serverManager;
    private readonly ICursorCookieReader _cookieReader;
    private readonly DebugService _debugService;
    private readonly Dictionary<string, UsageData> _lastGood = new(StringComparer.OrdinalIgnoreCase);
    private int _retryAttempt;
    private DateTime? _lastGoodAt;

    public TrayUsagePoller(ApiClient apiClient, ServerProcessManager serverManager, ICursorCookieReader cookieReader, DebugService debugService)
    {
        _apiClient = apiClient;
        _serverManager = serverManager;
        _cookieReader = cookieReader;
        _debugService = debugService;
    }

    public async Task<TrayUsageSnapshot> PollAsync(CancellationToken cancellationToken = default)
    {
        var server = await EnsureServerAsync(cancellationToken).ConfigureAwait(false);
        var result = await _apiClient.GetUsageAsync(cancellationToken).ConfigureAwait(false);
        if (result.IsSuccess && result.Value != null)
        {
            var providers = await PushCursorCookieIfNeededAsync(result.Value, cancellationToken).ConfigureAwait(false);
            StoreLastGood(providers);
            _retryAttempt = 0;
            return new TrayUsageSnapshot(TrayApiState.Ready, providers, _lastGoodAt, null, 0);
        }

        var state = MapState(result.Status);
        if (state == TrayApiState.Offline && ShouldRequestRecovery(server))
        {
            await _serverManager.RestartOwnedAsync(cancellationToken).ConfigureAwait(false);
        }
        if (state == TrayApiState.Offline)
        {
            _retryAttempt = Math.Min(_retryAttempt + 1, 5);
        }

        var message = BuildMessage(state, result.Error, server);
        _debugService.LogWarning("API", message);
        return new TrayUsageSnapshot(state, SnapshotLastGood(), _lastGoodAt, message, _retryAttempt);
    }

    private async Task<ServerProcessResult> EnsureServerAsync(CancellationToken cancellationToken)
    {
        try
        {
            return await _serverManager.EnsureStartedAsync(cancellationToken).ConfigureAwait(false);
        }
        catch (Exception ex) when (ex is not OperationCanceledException)
        {
            _debugService.LogWarning("Server", "Local usage server startup failed", ex.Message);
            return new ServerProcessResult(ServerProcessState.Failed, _serverManager.EffectiveBaseUrl, false, ServerProcessError.LaunchFailed, ex.Message);
        }
    }

    private async Task<IReadOnlyList<UsageData>> PushCursorCookieIfNeededAsync(IReadOnlyList<UsageData> providers, CancellationToken cancellationToken)
    {
        var cursor = providers.FirstOrDefault(x => x.ProviderName.Equals("Cursor", StringComparison.OrdinalIgnoreCase));
        if (!ShouldAttemptCursorCredentialPush(cursor))
        {
            return providers;
        }

        var cookie = _cookieReader.ReadCursorCookieHeader();
        try
        {
            if (string.IsNullOrWhiteSpace(cookie))
            {
                return providers;
            }
            var credential = CursorCredential.FromCookie(cookie);
            var pushed = await _apiClient.PutCursorCredentialsAsync(credential, cancellationToken).ConfigureAwait(false);
            return pushed.IsSuccess && pushed.Value != null
                ? ReplaceProvider(providers, pushed.Value.Usage)
                : providers;
        }
        finally
        {
            cookie = null;
        }
    }

    private static bool ShouldAttemptCursorCredentialPush(UsageData? data)
    {
        if (data?.ProviderName.Equals("Cursor", StringComparison.OrdinalIgnoreCase) != true)
        {
            return false;
        }
        if (data.NeedsReauth)
        {
            return true;
        }
        return string.Equals(
            data.Error,
            "Log in to cursor.com, or push Cursor credentials from the tray.",
            StringComparison.Ordinal);
    }

    private void StoreLastGood(IReadOnlyList<UsageData> providers)
    {
        _lastGood.Clear();
        foreach (var provider in providers)
        {
            _lastGood[provider.ProviderName] = Clone(provider);
        }
        _lastGoodAt = DateTime.UtcNow;
    }

    private IReadOnlyList<UsageData> SnapshotLastGood() => _lastGood.Values.Select(Clone).ToList();

    private static TrayApiState MapState(ApiResultStatus status) => status switch
    {
        ApiResultStatus.Offline => TrayApiState.Offline,
        ApiResultStatus.Unauthorized => TrayApiState.Unauthorized,
        ApiResultStatus.MalformedResponse => TrayApiState.Malformed,
        _ => TrayApiState.ApiError
    };

    private static bool ShouldRequestRecovery(ServerProcessResult server) =>
        server.Error == ServerProcessError.RestartFailed || server is { State: ServerProcessState.Failed, OwnsProcess: true };

    private static string BuildMessage(TrayApiState state, string? apiError, ServerProcessResult server) => state switch
    {
        TrayApiState.Offline => $"Usage server unreachable ({server.State}{FormatServerError(server)}).",
        TrayApiState.Unauthorized => "Usage API rejected the configured token.",
        TrayApiState.Malformed => "Usage API returned malformed data.",
        _ => apiError ?? "Usage API request failed."
    };

    private static string FormatServerError(ServerProcessResult server) => server.Error == ServerProcessError.None ? string.Empty : $": {server.Error}";

    private static IReadOnlyList<UsageData> ReplaceProvider(IReadOnlyList<UsageData> providers, UsageData replacement) =>
        providers.Select(provider => provider.ProviderName.Equals(replacement.ProviderName, StringComparison.OrdinalIgnoreCase) ? replacement : provider).ToList();

    private static UsageData Clone(UsageData data) => new()
    {
        ProviderName = data.ProviderName,
        PrimaryLabel = data.PrimaryLabel,
        SecondaryLabel = data.SecondaryLabel,
        ShowSecondary = data.ShowSecondary,
        Subtitle = data.Subtitle,
        PrimaryStatusText = data.PrimaryStatusText,
        SecondaryStatusText = data.SecondaryStatusText,
        ReauthCommand = data.ReauthCommand,
        Error = data.Error,
        NeedsReauth = data.NeedsReauth,
        Current = new UsageBucket { Utilization = data.Current.Utilization, ResetsAt = data.Current.ResetsAt },
        Weekly = new UsageBucket { Utilization = data.Weekly.Utilization, ResetsAt = data.Weekly.ResetsAt },
        Buckets = data.Buckets.Select(bucket => new UsageBucketDetail
        {
            Id = bucket.Id,
            Label = bucket.Label,
            Utilization = bucket.Utilization,
            ResetsAt = bucket.ResetsAt,
            StatusText = bucket.StatusText
        }).ToList()
    };
}
