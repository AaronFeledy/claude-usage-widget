using System.Net;
using System.Net.Http.Headers;
using System.Text.Json;
using ClaudeUsageWidget.Models;

namespace ClaudeUsageWidget.Services;

/// <summary>
/// Client for fetching Claude usage data from the API.
/// </summary>
public class UsageApiClient
{
    private const string UsageApiUrl = "https://api.anthropic.com/api/oauth/usage";
    private const string BetaHeader = "oauth-2025-04-20";

    private readonly HttpClient _httpClient;
    private readonly CredentialService _credentialService;

    public UsageApiClient(HttpClient httpClient, CredentialService credentialService)
    {
        _httpClient = httpClient;
        _credentialService = credentialService;
    }

    /// <summary>
    /// Fetches current usage data from the API.
    /// Auto-refreshes token if expired or on 401.
    /// </summary>
    public async Task<UsageData> FetchUsageAsync()
    {
        try
        {
            // Refresh token if expired
            if (_credentialService.IsTokenExpired())
            {
                var refreshed = await _credentialService.RefreshTokenAsync();
                if (!refreshed)
                {
                    return new UsageData { Error = "Failed to refresh expired token" };
                }
            }

            var response = await SendRequestAsync();

            // Handle 401 by attempting token refresh
            if (response.StatusCode == HttpStatusCode.Unauthorized)
            {
                var refreshed = await _credentialService.RefreshTokenAsync();
                if (!refreshed)
                {
                    return new UsageData { Error = "Authentication failed. Please re-authenticate with Claude." };
                }
                response = await SendRequestAsync();
            }

            if (!response.IsSuccessStatusCode)
            {
                var errorBody = await response.Content.ReadAsStringAsync();
                return new UsageData 
                { 
                    Error = $"API error ({response.StatusCode}): {errorBody}" 
                };
            }

            var json = await response.Content.ReadAsStringAsync();
            return ParseUsageResponse(json);
        }
        catch (HttpRequestException ex)
        {
            return new UsageData { Error = $"Network error: {ex.Message}" };
        }
        catch (TaskCanceledException)
        {
            return new UsageData { Error = "Request timed out" };
        }
        catch (Exception ex)
        {
            return new UsageData { Error = $"Unexpected error: {ex.Message}" };
        }
    }

    /// <summary>
    /// Sends the API request with current credentials.
    /// </summary>
    private async Task<HttpResponseMessage> SendRequestAsync()
    {
        var request = new HttpRequestMessage(HttpMethod.Get, UsageApiUrl);
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", _credentialService.AccessToken);
        request.Headers.Add("anthropic-beta", BetaHeader);

        return await _httpClient.SendAsync(request);
    }

    /// <summary>
    /// Parses the API response JSON into UsageData.
    /// </summary>
    private static UsageData ParseUsageResponse(string json)
    {
        try
        {
            var doc = JsonDocument.Parse(json);
            var root = doc.RootElement;

            var usageData = new UsageData();

            if (root.TryGetProperty("five_hour", out var current))
            {
                usageData.Current = ParseBucket(current);
            }

            if (root.TryGetProperty("seven_day", out var weekly))
            {
                usageData.Weekly = ParseBucket(weekly);
            }

            return usageData;
        }
        catch (JsonException ex)
        {
            return new UsageData { Error = $"Failed to parse response: {ex.Message}" };
        }
    }

    /// <summary>
    /// Parses a usage bucket from JSON.
    /// </summary>
    private static UsageBucket ParseBucket(JsonElement element)
    {
        var bucket = new UsageBucket();

        if (element.TryGetProperty("utilization", out var utilization))
        {
            bucket.Utilization = (float)utilization.GetDouble();
        }

        if (element.TryGetProperty("resets_at", out var resetsAt))
        {
            var resetsAtStr = resetsAt.GetString();
            if (!string.IsNullOrEmpty(resetsAtStr) && DateTime.TryParse(resetsAtStr, out var parsed))
            {
                bucket.ResetsAt = parsed.ToUniversalTime();
            }
        }

        return bucket;
    }
}
