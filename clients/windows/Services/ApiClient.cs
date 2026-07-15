using System.Net;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using ClaudeUsageWidget.Models;

namespace ClaudeUsageWidget.Services;

public sealed class ApiClient
{
    private static readonly JsonSerializerOptions JsonOptions = new(JsonSerializerDefaults.Web)
    {
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull
    };

    private const int MaxBuckets = 12;

    private readonly HttpClient _httpClient;
    private readonly ITrayApiClientSettings _settings;

    public ApiClient(HttpClient httpClient, ITrayApiClientSettings settings)
    {
        _httpClient = httpClient;
        _settings = settings;
    }

    public async Task<ApiResult<IReadOnlyList<UsageData>>> GetUsageAsync(CancellationToken cancellationToken = default)
    {
        return await RequestAsync(HttpMethod.Get, "api/v1/usage", null, MapUsageList, cancellationToken);
    }

    public async Task<ApiResult<UsageData>> GetProviderAsync(string providerName, CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(providerName)) throw new ArgumentException("Provider name must not be blank.", nameof(providerName));
        var provider = Uri.EscapeDataString(providerName.Trim());
        return await RequestAsync(HttpMethod.Get, $"api/v1/usage/{provider}", null, MapUsage, cancellationToken);
    }

    public async Task<ApiResult<ServerHealthInfo>> GetHealthAsync(CancellationToken cancellationToken = default)
    {
        return await RequestAsync(HttpMethod.Get, "api/v1/health", null, MapHealth, cancellationToken);
    }

    public async Task<ApiResult<CursorCredentialResult>> PutCursorCredentialsAsync(
        CursorCredential credential,
        CancellationToken cancellationToken = default)
    {
        var request = new CursorCredentialRequest(credential.Cookie, credential.AccessToken);
        return await RequestAsync(HttpMethod.Put, "api/v1/providers/cursor/credentials", request, MapCredentialResult, cancellationToken);
    }

    public Task<ApiResult<CursorCredentialResult>> PutCursorCredentialAsync(
        CursorCredential credential,
        CancellationToken cancellationToken = default) => PutCursorCredentialsAsync(credential, cancellationToken);

    private async Task<ApiResult<T>> RequestAsync<T>(
        HttpMethod method,
        string relativePath,
        object? body,
        Func<string, ApiResult<T>> parse,
        CancellationToken cancellationToken)
    {
        using var timeout = CreateTimeout(cancellationToken);
        try
        {
            using var response = await SendAsync(method, relativePath, body, timeout.Token);
            return await ReadJsonAsync(response, parse, timeout.Token);
        }
        catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
        {
            throw;
        }
        catch (OperationCanceledException)
        {
            return ApiResult<T>.Failure(ApiResultStatus.Offline, "Server is offline or unreachable.");
        }
        catch (HttpRequestException)
        {
            return ApiResult<T>.Failure(ApiResultStatus.Offline, "Server is offline or unreachable.");
        }
    }

    private async Task<HttpResponseMessage> SendAsync(HttpMethod method, string relativePath, object? body, CancellationToken cancellationToken)
    {
        using var request = new HttpRequestMessage(method, BuildUri(relativePath));
        request.Headers.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));
        var token = _settings.BearerToken?.Trim();
        if (!string.IsNullOrEmpty(token)) request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        if (body != null)
        {
            var json = JsonSerializer.Serialize(body, JsonOptions);
            request.Content = new StringContent(json, Encoding.UTF8, "application/json");
        }
        return await _httpClient.SendAsync(request, HttpCompletionOption.ResponseHeadersRead, cancellationToken);
    }

    private CancellationTokenSource CreateTimeout(CancellationToken cancellationToken)
    {
        var source = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        if (_settings.Timeout > TimeSpan.Zero) source.CancelAfter(_settings.Timeout);
        return source;
    }

    private Uri BuildUri(string relativePath)
    {
        var baseText = _settings.BaseUrl.ToString().TrimEnd('/') + "/";
        return new Uri(new Uri(baseText), relativePath.TrimStart('/'));
    }

    private static async Task<ApiResult<T>> ReadJsonAsync<T>(
        HttpResponseMessage response,
        Func<string, ApiResult<T>> parse,
        CancellationToken cancellationToken)
    {
        if (response.StatusCode == HttpStatusCode.NotFound) return ApiResult<T>.Failure(ApiResultStatus.ProviderNotFound, "Provider not found.");
        if (response.StatusCode == HttpStatusCode.Unauthorized) return ApiResult<T>.Failure(ApiResultStatus.Unauthorized, "Unauthorized.");
        if (!response.IsSuccessStatusCode) return ApiResult<T>.Failure(ApiResultStatus.ApiError, $"API error ({(int)response.StatusCode}).");
        var json = await response.Content.ReadAsStringAsync(cancellationToken);
        return parse(json);
    }

    private static ApiResult<IReadOnlyList<UsageData>> MapUsageList(string json)
    {
        try
        {
            var wire = JsonSerializer.Deserialize<List<UsageWire?>>(json, JsonOptions);
            if (wire == null || wire.Any(x => x == null)) return Malformed<IReadOnlyList<UsageData>>();
            return ApiResult<IReadOnlyList<UsageData>>.Success(wire.Select(x => MapUsageWire(x!)).ToList());
        }
        catch (JsonException)
        {
            return Malformed<IReadOnlyList<UsageData>>();
        }
        catch (WireValidationException)
        {
            return Malformed<IReadOnlyList<UsageData>>();
        }
    }

    private static ApiResult<UsageData> MapUsage(string json)
    {
        try
        {
            var wire = JsonSerializer.Deserialize<UsageWire>(json, JsonOptions);
            return wire == null ? Malformed<UsageData>() : ApiResult<UsageData>.Success(MapUsageWire(wire));
        }
        catch (JsonException)
        {
            return Malformed<UsageData>();
        }
        catch (WireValidationException)
        {
            return Malformed<UsageData>();
        }
    }

    private static ApiResult<CursorCredentialResult> MapCredentialResult(string json)
    {
        try
        {
            var wire = JsonSerializer.Deserialize<CursorCredentialResponse>(json, JsonOptions);
            if (wire?.Usage == null || string.IsNullOrWhiteSpace(wire.Provider) || wire.Refetched == null) return Malformed<CursorCredentialResult>();
            return ApiResult<CursorCredentialResult>.Success(new CursorCredentialResult(wire.Provider, wire.Refetched.Value, MapUsageWire(wire.Usage)));
        }
        catch (JsonException)
        {
            return Malformed<CursorCredentialResult>();
        }
        catch (WireValidationException)
        {
            return Malformed<CursorCredentialResult>();
        }
    }

    private static ApiResult<ServerHealthInfo> MapHealth(string json)
    {
        try
        {
            var wire = JsonSerializer.Deserialize<HealthWire>(json, JsonOptions);
            if (wire == null || string.IsNullOrWhiteSpace(wire.Status) || string.IsNullOrWhiteSpace(wire.Version))
            {
                return Malformed<ServerHealthInfo>();
            }
            return ApiResult<ServerHealthInfo>.Success(new ServerHealthInfo(wire.Status.Trim(), wire.Version.Trim()));
        }
        catch (JsonException)
        {
            return Malformed<ServerHealthInfo>();
        }
    }

    private static UsageData MapUsageWire(UsageWire wire)
    {
        ValidateUsage(wire);
        return new UsageData
        {
            ProviderName = wire.ProviderName!,
            PrimaryLabel = wire.PrimaryLabel!,
            SecondaryLabel = wire.SecondaryLabel!,
            ShowSecondary = wire.ShowSecondary!.Value,
            Subtitle = wire.Subtitle,
            PrimaryStatusText = wire.PrimaryStatusText,
            SecondaryStatusText = wire.SecondaryStatusText,
            ReauthCommand = wire.ReauthCommand,
            Current = MapBucket(wire.Current!),
            Weekly = MapBucket(wire.Weekly!),
            Buckets = MapBuckets(wire),
            Error = wire.Error,
            NeedsReauth = wire.NeedsReauth!.Value
        };
    }

    private static UsageBucket MapBucket(UsageBucketWire bucket) => new()
    {
        Utilization = (float)bucket.Utilization!.Value,
        ResetsAt = bucket.ResetsAt
    };

    private static IReadOnlyList<UsageBucketDetail> MapBuckets(UsageWire wire)
    {
        if (wire.Buckets is { Count: > 0 })
        {
            return wire.Buckets.Select(bucket => new UsageBucketDetail
            {
                Id = bucket!.Id!,
                Label = bucket.Label!,
                Utilization = (float)bucket.Utilization!.Value,
                ResetsAt = bucket.ResetsAt,
                StatusText = bucket.StatusText
            }).ToList();
        }

        var buckets = new List<UsageBucketDetail>
        {
            new()
            {
                Id = "session",
                Label = wire.PrimaryLabel!,
                Utilization = (float)wire.Current!.Utilization!.Value,
                ResetsAt = wire.Current.ResetsAt,
                StatusText = wire.PrimaryStatusText
            }
        };
        if (wire.ShowSecondary!.Value)
        {
            buckets.Add(new UsageBucketDetail
            {
                Id = "weekly",
                Label = wire.SecondaryLabel!,
                Utilization = (float)wire.Weekly!.Utilization!.Value,
                ResetsAt = wire.Weekly.ResetsAt,
                StatusText = wire.SecondaryStatusText
            });
        }
        return buckets;
    }

    private static void ValidateUsage(UsageWire wire)
    {
        RequireText(wire.ProviderName);
        RequireText(wire.PrimaryLabel);
        RequireText(wire.SecondaryLabel);
        if (wire.ShowSecondary == null || wire.Current == null || wire.Weekly == null || wire.NeedsReauth == null || wire.IsSuccess == null) throw new WireValidationException();
        if (wire.IsSuccess.Value != (wire.Error == null)) throw new WireValidationException();
        ValidateBucket(wire.Current);
        ValidateBucket(wire.Weekly);
        if (wire.Buckets is { Count: > 0 })
        {
            if (wire.Buckets.Count > MaxBuckets) throw new WireValidationException();
            foreach (var bucket in wire.Buckets)
            {
                if (bucket == null) throw new WireValidationException();
                RequireText(bucket.Id);
                RequireText(bucket.Label);
                ValidateUtilization(bucket.Utilization);
            }
        }
    }

    private static void ValidateBucket(UsageBucketWire bucket)
    {
        ValidateUtilization(bucket.Utilization);
    }

    private static void ValidateUtilization(double? value)
    {
        if (value == null || !double.IsFinite(value.Value)) throw new WireValidationException();
        var utilization = value.Value;
        if (utilization is < 0 or > 100) throw new WireValidationException();
    }

    private static void RequireText(string? value)
    {
        if (string.IsNullOrWhiteSpace(value)) throw new WireValidationException();
    }

    private static ApiResult<T> Malformed<T>() => ApiResult<T>.Failure(ApiResultStatus.MalformedResponse, "Malformed API response.");
}
