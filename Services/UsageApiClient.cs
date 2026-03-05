using System.Net;
using System.Net.Http.Headers;
using System.Text.Json;
using ClaudeUsageWidget.Models;
using System.Linq;

namespace ClaudeUsageWidget.Services;

/// <summary>
/// Client for fetching Claude usage data from the API.
/// </summary>
public class UsageApiClient
{
    private const string UsageApiUrl = "https://api.anthropic.com/api/oauth/usage";
    private const string BetaHeader = "oauth-2025-04-20";
    private const int MinBackoffSeconds = 30;  // Minimum 30 seconds
    private const int MaxBackoffSeconds = 300; // 5 minutes max backoff

    private readonly HttpClient _httpClient;
    private readonly CredentialService _credentialService;
    
    // Rate limit state
    private DateTime _rateLimitedUntil = DateTime.MinValue;
    private int _consecutiveRateLimits = 0;
    private UsageData? _lastSuccessfulData;

    public UsageApiClient(HttpClient httpClient, CredentialService credentialService)
    {
        _httpClient = httpClient;
        _credentialService = credentialService;
    }
    
    /// <summary>
    /// Returns true if we're currently in a rate-limited backoff period.
    /// </summary>
    public bool IsRateLimited => DateTime.UtcNow < _rateLimitedUntil;
    
    /// <summary>
    /// Time remaining until rate limit backoff expires.
    /// </summary>
    public TimeSpan RateLimitTimeRemaining => 
        IsRateLimited ? _rateLimitedUntil - DateTime.UtcNow : TimeSpan.Zero;

    /// <summary>
    /// Fetches current usage data from the API.
    /// Auto-refreshes token if expired or on 401.
    /// Implements exponential backoff for rate limits.
    /// </summary>
    public async Task<UsageData> FetchUsageAsync()
    {
        // If we're in a rate limit backoff period, return cached data
        if (IsRateLimited && _lastSuccessfulData != null)
        {
            return _lastSuccessfulData;
        }
        
        try
        {
            // Refresh token if expired
            if (_credentialService.IsTokenExpired())
            {
                var result = await _credentialService.RefreshTokenAsync();
                if (result == RefreshResult.InvalidGrant)
                    return new UsageData { Error = "AUTH_EXPIRED", NeedsReauth = true };
                if (result == RefreshResult.Failed)
                    return new UsageData { Error = "Token refresh failed. Will retry." };
            }

            var response = await SendRequestAsync();

            // Handle 401 by attempting token refresh
            if (response.StatusCode == HttpStatusCode.Unauthorized)
            {
                var result = await _credentialService.RefreshTokenAsync();
                if (result == RefreshResult.InvalidGrant)
                    return new UsageData { Error = "AUTH_EXPIRED", NeedsReauth = true };
                if (result == RefreshResult.Failed)
                    return new UsageData { Error = "Authentication failed. Will retry." };
                response = await SendRequestAsync();
            }

            // Handle rate limiting with exponential backoff
            if (response.StatusCode == HttpStatusCode.TooManyRequests)
            {
                _consecutiveRateLimits++;
                var backoffSeconds = GetBackoffSeconds(response);
                _rateLimitedUntil = DateTime.UtcNow.AddSeconds(backoffSeconds);
                
                // Return cached data if available, otherwise return a friendly error
                if (_lastSuccessfulData != null)
                {
                    return _lastSuccessfulData;
                }
                
                return new UsageData 
                { 
                    Error = $"Rate limited. Retrying in {backoffSeconds}s." 
                };
            }

            if (!response.IsSuccessStatusCode)
            {
                var errorBody = await response.Content.ReadAsStringAsync();
                return new UsageData 
                { 
                    Error = $"API error ({response.StatusCode}): {errorBody}" 
                };
            }

            // Success - reset rate limit state and cache the data
            _consecutiveRateLimits = 0;
            _rateLimitedUntil = DateTime.MinValue;
            
            var json = await response.Content.ReadAsStringAsync();
            var usageData = ParseUsageResponse(json);
            
            if (usageData.IsSuccess)
            {
                _lastSuccessfulData = usageData;
            }
            
            return usageData;
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
    /// Calculates backoff time based on Retry-After header or exponential backoff.
    /// </summary>
    private int GetBackoffSeconds(HttpResponseMessage response)
    {
        // Check for Retry-After header
        if (response.Headers.TryGetValues("Retry-After", out var values))
        {
            var retryAfter = values.FirstOrDefault();
            if (!string.IsNullOrEmpty(retryAfter))
            {
                // Try parsing as seconds
                if (int.TryParse(retryAfter, out var seconds))
                {
                    return Math.Clamp(seconds, MinBackoffSeconds, MaxBackoffSeconds);
                }
                
                // Try parsing as HTTP date
                if (DateTime.TryParse(retryAfter, out var date))
                {
                    var delay = (int)(date.ToUniversalTime() - DateTime.UtcNow).TotalSeconds;
                    return Math.Clamp(delay, MinBackoffSeconds, MaxBackoffSeconds);
                }
            }
        }
        
        // Exponential backoff: 30s, 60s, 120s, 240s, 300s (max)
        var exponentialBackoff = MinBackoffSeconds * (int)Math.Pow(2, _consecutiveRateLimits - 1);
        return Math.Clamp(exponentialBackoff, MinBackoffSeconds, MaxBackoffSeconds);
    }

    /// <summary>
    /// Sends the API request with current credentials.
    /// </summary>
    private async Task<HttpResponseMessage> SendRequestAsync()
    {
        var request = new HttpRequestMessage(HttpMethod.Get, UsageApiUrl);
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", _credentialService.AccessToken);
        request.Headers.Add("anthropic-beta", BetaHeader);
        // Match Claude Code's User-Agent format to avoid rate limiting
        request.Headers.Add("User-Agent", "claude-cli/widget");

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
