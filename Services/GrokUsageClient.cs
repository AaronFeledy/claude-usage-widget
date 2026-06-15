using System.Net;
using System.Net.Http.Headers;
using System.Text.Json;
using ClaudeUsageWidget.Models;

namespace ClaudeUsageWidget.Services;

/// <summary>
/// Client for fetching Grok (SuperGrok / Grok Build CLI) credit usage.
/// Uses the undocumented-but-stable cli-chat-proxy billing endpoint.
/// </summary>
public class GrokUsageClient
{
    private const string BillingUrl = "https://cli-chat-proxy.grok.com/v1/billing";
    private const string SettingsUrl = "https://cli-chat-proxy.grok.com/v1/settings";
    private const string TokenAuthHeader = "xai-grok-cli";
    private const string UserAgent = "ClaudeUsageWidget";

    private readonly HttpClient _httpClient;
    private readonly GrokCredentialService _credentialService;
    private readonly DebugService? _debugService;

    public GrokUsageClient(HttpClient httpClient, GrokCredentialService credentialService, DebugService? debugService = null)
    {
        _httpClient = httpClient;
        _credentialService = credentialService;
        _debugService = debugService;
    }

    public async Task<UsageData> FetchUsageAsync()
    {
        var usageData = new UsageData
        {
            ProviderName = "Grok",
            PrimaryLabel = "Credits",
            SecondaryLabel = "Pay as you go",
            ReauthCommand = "grok login",
            ShowSecondary = true
        };

        try
        {
            if (_credentialService.NeedsRefresh())
            {
                _debugService?.LogInfo("Grok", "Token needs refresh, attempting refresh");
                var ok = await _credentialService.RefreshTokenAsync();
                if (!ok)
                {
                    usageData.Error = "Grok auth expired. Run `grok login` again.";
                    usageData.NeedsReauth = true;
                    return usageData;
                }
            }

            var token = _credentialService.AccessToken;
            if (string.IsNullOrWhiteSpace(token))
            {
                usageData.Error = "Grok not logged in. Run `grok login`.";
                return usageData;
            }

            var billingResp = await SendRequestAsync(BillingUrl, token);
            if (billingResp.StatusCode is HttpStatusCode.Unauthorized or HttpStatusCode.Forbidden)
            {
                _debugService?.LogWarning("Grok", "Got 401/403, attempting refresh");
                var refreshed = await _credentialService.RefreshTokenAsync();
                if (refreshed)
                {
                    token = _credentialService.AccessToken!;
                    billingResp = await SendRequestAsync(BillingUrl, token);
                }
            }

            if (billingResp.StatusCode is HttpStatusCode.Unauthorized or HttpStatusCode.Forbidden)
            {
                usageData.Error = "Grok auth expired. Run `grok login` again.";
                usageData.NeedsReauth = true;
                return usageData;
            }

            if (!billingResp.IsSuccessStatusCode)
            {
                var body = await billingResp.Content.ReadAsStringAsync();
                _debugService?.LogError("Grok", $"Billing request failed: {billingResp.StatusCode}", body);
                usageData.Error = $"Grok billing request failed ({(int)billingResp.StatusCode}). Try again later.";
                return usageData;
            }

            var json = await billingResp.Content.ReadAsStringAsync();
            var doc = JsonDocument.Parse(json);

            if (!doc.RootElement.TryGetProperty("config", out var config) || config.ValueKind != JsonValueKind.Object)
            {
                usageData.Error = "Grok billing response changed.";
                return usageData;
            }

            var used = GetVal(config, "used");
            var limit = GetVal(config, "monthlyLimit");
            var onDemand = GetVal(config, "onDemandCap");
            var resetsAtStr = config.TryGetProperty("billingPeriodEnd", out var endEl) ? endEl.GetString() : null;

            if (used is null || limit is null || limit.Value <= 0 || onDeman;
                usageData.Error = $"Grok billing request failed ({(int)billingResp.StatusCode}). Try again later.";
                return usageData;
            }

            var json = await billingResp.Content.ReadAsStringAsync();
            var doc = JsonDocument.Parse(json);

            if (!doc.RootElement.TryGetProperty("config", out var config) || config.ValueKind != JsonValueKind.Object)
            {
                usageData.Error = "Grok billing response changed.";
                return usageData;
            }

            var used = GetVal(config, "used");
            var limit = GetVal(config, "monthlyLimit");
            var onDemand = GetVal(config, "onDemandCap");
            var resetsAtStr = config.TryGetProperty("billingPeriodEnd", out var endEl) ? endEl.GetString() : null;

            if (used is null || limit is null || limit.Value <= 0 || onDemand is null || string.IsNullOrWhiteSpace(resetsAtStr))
            {
                usageData.Error = "Grok billing response changed.";
                return usageData;
            }

            var percent = (float)(used.Value / limit.Value * 100.0);
            usageData.Current = new UsageBucket
            {
                Utilization = Math.Clamp(percent, 0f, 100f),
                ResetsAt = DateTime.TryParse(resetsAtStr, out var dt) ? dt.ToUniversalTime() : null
            };

            usageData.Weekly = new UsageBucket
            {
                Utilization = 0,
                ResetsAt = usageData.Current.ResetsAt
            };

            usageData.PrimaryStatusText = $"{used.Value:N0} / {limit.Value:N0} credits";
            usageData.SecondaryStatusText = onDemand.Value > 0
                ? $"{onDemand.Value:N0} pay-as-you-go cap"
                : "Pay as you go disabled";

            // Pay-as-you-go is a cap, not a tracked utilization percentage.
            // Hide the secondary progress bar (we still show the status text above).
            usageData.ShowSecondary = false;

            // Best-effort plan name
            try
            {
                var settingsResp = await SendRequestAsync(SettingsUrl, token);
                if (settingsResp.IsSuccessStatusCode)
                {
                    var sjson = await settingsResp.Content.ReadAsStringAsync();
                    var sdoc = JsonDocument.Parse(sjson);
                    if (sdoc.RootElement.TryGetProperty("subscription_tier_display", out var planEl))
                    {
                        var plan = planEl.GetString();
                        if (!string.IsNullOrWhiteSpace(plan))
                            usageData.Subtitle = plan.Trim();
                    }
                }
            }
            catch (Exception ex)
            {
                _debugService?.LogWarning("Grok", "Failed to fetch plan name", ex.Message);
            }

            return usageData;
        }
        catch (FileNotFoundException)
        {
            usageData.Error = "Grok not logged in. Run `grok login`.";
            return usageData;
        }
        catch (Exception ex)
        {
            _debugService?.LogError("Grok", ex.Message, ex.ToString());
            usageData.Error = ex.Message;
            return usageData;
        }
    }

    private async Task<HttpResponseMessage> SendRequestAsync(string url, string token)
    {
        using var req = new HttpRequestMessage(HttpMethod.Get, url);
        req.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        req.Headers.Add("X-XAI-Token-Auth", TokenAuthHeader);
        req.Headers.Add("Accept", "application/json");
        req.Headers.Add("User-Agent", UserAgent);
        return await _httpClient.SendAsync(req);
    }

    private static double? GetVal(JsonElement obj, string name)
    {
        if (!obj.TryGetProperty(name, out var el) || el.ValueKind != JsonValueKind.Object)
            return null;
        if (!el.TryGetProperty("val", out var vEl))
            return null;
        return vEl.ValueKind == JsonValueKind.Number ? vEl.GetDouble() : null;
    }
}
