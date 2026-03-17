using System.Text.Json;
using System.Text.Json.Serialization;
using ClaudeUsageWidget.Models;

namespace ClaudeUsageWidget.Services;

public class CursorUsageClient
{
    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNameCaseInsensitive = true
    };

    private readonly HttpClient _httpClient;
    private readonly WindowsBrowserCookieReader _cookieReader;
    private readonly DebugService? _debugService;
    private string? _cachedCookieHeader;

    public CursorUsageClient(HttpClient httpClient, WindowsBrowserCookieReader cookieReader, DebugService? debugService = null)
    {
        _httpClient = httpClient;
        _cookieReader = cookieReader;
        _debugService = debugService;
    }

    public async Task<UsageData> FetchUsageAsync()
    {
        var usageData = new UsageData
        {
            ProviderName = "Cursor",
            PrimaryLabel = "Included Plan",
            SecondaryLabel = "On-Demand",
            ReauthCommand = null
        };

        try
        {
            var cookieHeader = _cachedCookieHeader ?? _cookieReader.ReadCursorCookieHeader();
            if (string.IsNullOrWhiteSpace(cookieHeader))
            {
                usageData.Error = "Log in to cursor.com in Chrome, Edge, or Brave.";
                return usageData;
            }

            _cachedCookieHeader = cookieHeader;

            var summaryResponse = await SendRequestAsync("https://cursor.com/api/usage-summary", cookieHeader);
            if (summaryResponse.StatusCode is System.Net.HttpStatusCode.Unauthorized or System.Net.HttpStatusCode.Forbidden)
            {
                _cachedCookieHeader = null;
                usageData.Error = "Cursor session expired. Log in to cursor.com again.";
                return usageData;
            }

            summaryResponse.EnsureSuccessStatusCode();
            var summaryJson = await summaryResponse.Content.ReadAsStringAsync();
            var summary = JsonSerializer.Deserialize<CursorUsageSummary>(summaryJson, JsonOptions);
            if (summary == null)
            {
                usageData.Error = "Failed to parse Cursor usage.";
                return usageData;
            }

            CursorUserInfo? userInfo = null;
            try
            {
                var userResponse = await SendRequestAsync("https://cursor.com/api/auth/me", cookieHeader);
                if (userResponse.IsSuccessStatusCode)
                {
                    var userJson = await userResponse.Content.ReadAsStringAsync();
                    userInfo = JsonSerializer.Deserialize<CursorUserInfo>(userJson, JsonOptions);
                }
            }
            catch (Exception ex)
            {
                _debugService?.LogWarning("Cursor", "Failed to fetch user info", ex.Message);
            }

            CursorUsageResponse? legacyUsage = null;
            if (!string.IsNullOrWhiteSpace(userInfo?.Sub))
            {
                try
                {
                    var usageResponse = await SendRequestAsync($"https://cursor.com/api/usage?user={Uri.EscapeDataString(userInfo.Sub)}", cookieHeader);
                    if (usageResponse.IsSuccessStatusCode)
                    {
                        var usageJson = await usageResponse.Content.ReadAsStringAsync();
                        legacyUsage = JsonSerializer.Deserialize<CursorUsageResponse>(usageJson, JsonOptions);
                    }
                }
                catch (Exception ex)
                {
                    _debugService?.LogWarning("Cursor", "Failed to fetch legacy request usage", ex.Message);
                }
            }

            PopulateUsageData(usageData, summary, legacyUsage);
            usageData.Subtitle = summary.MembershipType;
            return usageData;
        }
        catch (Exception ex)
        {
            _debugService?.LogError("Cursor", ex.Message, ex.ToString());
            usageData.Error = ex.Message;
            return usageData;
        }
    }

    private async Task<HttpResponseMessage> SendRequestAsync(string url, string cookieHeader)
    {
        using var request = new HttpRequestMessage(HttpMethod.Get, url);
        request.Headers.Add("Accept", "application/json");
        request.Headers.Add("Cookie", cookieHeader);
        return await _httpClient.SendAsync(request);
    }

    private static void PopulateUsageData(UsageData usageData, CursorUsageSummary summary, CursorUsageResponse? legacyUsage)
    {
        var billingCycleEnd = ParseDate(summary.BillingCycleEnd);

        var planUsedRaw = (double)(summary.IndividualUsage?.Plan?.Used ?? 0);
        var planLimitRaw = (double)(summary.IndividualUsage?.Plan?.Limit ?? 0);
        var planPercent = planLimitRaw > 0
            ? (float)(planUsedRaw / planLimitRaw * 100)
            : NormalizePercent(summary.IndividualUsage?.Plan?.TotalPercentUsed);

        var requestsUsed = legacyUsage?.Gpt4?.NumRequestsTotal ?? legacyUsage?.Gpt4?.NumRequests;
        var requestsLimit = legacyUsage?.Gpt4?.MaxRequestUsage;
        if (requestsUsed.HasValue && requestsLimit.GetValueOrDefault() > 0)
        {
            var requestLimitValue = requestsLimit.GetValueOrDefault();
            usageData.PrimaryLabel = "Requests";
            planPercent = (float)requestsUsed.Value / requestLimitValue * 100;
            usageData.PrimaryStatusText = $"{requestsUsed.Value} / {requestLimitValue} requests this cycle";
        }
        else
        {
            usageData.PrimaryStatusText = $"${planUsedRaw / 100:0.##} / ${planLimitRaw / 100:0.##} this cycle";
        }

        usageData.Current = new UsageBucket
        {
            Utilization = Math.Clamp(planPercent, 0, 100),
            ResetsAt = billingCycleEnd
        };

        var onDemandUsedRaw = (double)(summary.IndividualUsage?.OnDemand?.Used ?? 0);
        var onDemandLimitRaw = summary.IndividualUsage?.OnDemand?.Limit;
        if (onDemandLimitRaw.HasValue && onDemandLimitRaw.Value > 0)
        {
            var onDemandPercent = (float)(onDemandUsedRaw / onDemandLimitRaw.Value * 100);
            usageData.Weekly = new UsageBucket
            {
                Utilization = Math.Clamp(onDemandPercent, 0, 100),
                ResetsAt = billingCycleEnd
            };
            usageData.SecondaryStatusText = $"${onDemandUsedRaw / 100:0.##} / ${onDemandLimitRaw.Value / 100.0:0.##} on-demand";
        }
        else
        {
            usageData.ShowSecondary = false;
            usageData.SecondaryStatusText = onDemandUsedRaw > 0
                ? $"${onDemandUsedRaw / 100:0.##} on-demand this cycle"
                : "No on-demand cap exposed by Cursor";
        }
    }

    private static float NormalizePercent(double? value)
    {
        if (!value.HasValue)
            return 0;

        return value.Value <= 1 ? (float)(value.Value * 100) : (float)value.Value;
    }

    private static DateTime? ParseDate(string? value)
    {
        if (string.IsNullOrWhiteSpace(value))
            return null;

        return DateTime.TryParse(value, out var parsed) ? parsed.ToUniversalTime() : null;
    }

    private sealed class CursorUsageSummary
    {
        [JsonPropertyName("billingCycleEnd")]
        public string? BillingCycleEnd { get; set; }

        [JsonPropertyName("membershipType")]
        public string? MembershipType { get; set; }

        [JsonPropertyName("individualUsage")]
        public CursorIndividualUsage? IndividualUsage { get; set; }
    }

    private sealed class CursorIndividualUsage
    {
        [JsonPropertyName("plan")]
        public CursorPlanUsage? Plan { get; set; }

        [JsonPropertyName("onDemand")]
        public CursorOnDemandUsage? OnDemand { get; set; }
    }

    private sealed class CursorPlanUsage
    {
        [JsonPropertyName("used")]
        public int? Used { get; set; }

        [JsonPropertyName("limit")]
        public int? Limit { get; set; }

        [JsonPropertyName("totalPercentUsed")]
        public double? TotalPercentUsed { get; set; }
    }

    private sealed class CursorOnDemandUsage
    {
        [JsonPropertyName("used")]
        public int? Used { get; set; }

        [JsonPropertyName("limit")]
        public int? Limit { get; set; }
    }

    private sealed class CursorUserInfo
    {
        public string? Email { get; set; }
        public string? Name { get; set; }

        [JsonPropertyName("sub")]
        public string? Sub { get; set; }
    }

    private sealed class CursorUsageResponse
    {
        [JsonPropertyName("gpt-4")]
        public CursorModelUsage? Gpt4 { get; set; }
    }

    private sealed class CursorModelUsage
    {
        [JsonPropertyName("numRequests")]
        public int? NumRequests { get; set; }

        [JsonPropertyName("numRequestsTotal")]
        public int? NumRequestsTotal { get; set; }

        [JsonPropertyName("maxRequestUsage")]
        public int? MaxRequestUsage { get; set; }
    }
}
