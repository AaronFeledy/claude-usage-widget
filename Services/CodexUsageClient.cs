using System.Net;
using System.Net.Http.Headers;
using System.Text.Json;
using ClaudeUsageWidget.Models;

namespace ClaudeUsageWidget.Services;

public class CodexUsageClient
{
    private const string UsageApiUrl = "https://chatgpt.com/backend-api/wham/usage";
    private readonly HttpClient _httpClient;
    private readonly CodexCredentialService _credentialService;
    private readonly DebugService? _debugService;

    public CodexUsageClient(HttpClient httpClient, CodexCredentialService credentialService, DebugService? debugService = null)
    {
        _httpClient = httpClient;
        _credentialService = credentialService;
        _debugService = debugService;
    }

    public async Task<UsageData> FetchUsageAsync()
    {
        var usageData = new UsageData
        {
            ProviderName = "Codex",
            PrimaryLabel = "5-Hour",
            SecondaryLabel = "Weekly",
            ReauthCommand = "codex"
        };

        try
        {
            if (_credentialService.NeedsRefresh())
            {
                var refresh = await _credentialService.RefreshTokenAsync();
                if (refresh == CodexRefreshResult.InvalidGrant)
                {
                    usageData.Error = "AUTH_EXPIRED";
                    usageData.NeedsReauth = true;
                    return usageData;
                }

                if (refresh == CodexRefreshResult.Failed)
                {
                    usageData.Error = "Token refresh failed. Will retry.";
                    return usageData;
                }
            }

            var response = await SendRequestAsync();

            if (response.StatusCode == HttpStatusCode.Unauthorized || response.StatusCode == HttpStatusCode.Forbidden)
            {
                var refresh = await _credentialService.RefreshTokenAsync();
                if (refresh == CodexRefreshResult.InvalidGrant)
                {
                    usageData.Error = "AUTH_EXPIRED";
                    usageData.NeedsReauth = true;
                    return usageData;
                }

                if (refresh == CodexRefreshResult.Success)
                    response = await SendRequestAsync();
            }

            if (!response.IsSuccessStatusCode)
            {
                usageData.Error = $"API error ({(int)response.StatusCode})";
                return usageData;
            }

            var json = await response.Content.ReadAsStringAsync();
            return ParseUsageResponse(json, usageData);
        }
        catch (FileNotFoundException)
        {
            usageData.Error = "Run `codex` to sign in.";
            return usageData;
        }
        catch (Exception ex)
        {
            _debugService?.LogError("Codex", ex.Message, ex.ToString());
            usageData.Error = ex.Message;
            return usageData;
        }
    }

    private async Task<HttpResponseMessage> SendRequestAsync()
    {
        var request = new HttpRequestMessage(HttpMethod.Get, UsageApiUrl);
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", _credentialService.AccessToken);
        request.Headers.Add("User-Agent", "ClaudeUsageWidget");
        request.Headers.Add("Accept", "application/json");

        if (!string.IsNullOrWhiteSpace(_credentialService.AccountId))
            request.Headers.Add("ChatGPT-Account-Id", _credentialService.AccountId);

        return await _httpClient.SendAsync(request);
    }

    private static UsageData ParseUsageResponse(string json, UsageData usageData)
    {
        var doc = JsonDocument.Parse(json);
        var root = doc.RootElement;

        if (root.TryGetProperty("plan_type", out var planType))
            usageData.Subtitle = planType.GetString();

        if (root.TryGetProperty("rate_limit", out var rateLimit))
        {
            if (rateLimit.TryGetProperty("primary_window", out var primaryWindow))
                usageData.Current = ParseWindow(primaryWindow);

            if (rateLimit.TryGetProperty("secondary_window", out var secondaryWindow))
                usageData.Weekly = ParseWindow(secondaryWindow);
        }

        return usageData;
    }

    private static UsageBucket ParseWindow(JsonElement element)
    {
        var bucket = new UsageBucket();

        if (element.TryGetProperty("used_percent", out var usedPercent))
            bucket.Utilization = usedPercent.GetSingle();

        if (element.TryGetProperty("reset_at", out var resetAt))
            bucket.ResetsAt = DateTimeOffset.FromUnixTimeSeconds(resetAt.GetInt64()).UtcDateTime;

        return bucket;
    }
}
