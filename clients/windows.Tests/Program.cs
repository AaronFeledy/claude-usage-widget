using System.Net;
using System.Text;
using ClaudeUsageWidget.Services;

var tests = new (string Name, Func<Task> Run)[]
{
    ("maps frozen usage JSON", MapsFrozenUsageJson),
    ("adds bearer auth and escapes provider", AddsBearerAuthAndEscapesProvider),
    ("returns unauthorized on 401", ReturnsUnauthorized),
    ("returns provider not found on 404", ReturnsProviderNotFound),
    ("returns API error on server failure", ReturnsApiError),
    ("returns malformed response", ReturnsMalformedResponse),
    ("rejects missing current", RejectsMissingCurrent),
    ("rejects null weekly", RejectsNullWeekly),
    ("rejects missing provider name", RejectsMissingProviderName),
    ("rejects invalid utilization", RejectsInvalidUtilization),
    ("rejects malformed timestamp", RejectsMalformedTimestamp),
    ("throws on blank provider", ThrowsOnBlankProvider),
    ("preserves caller cancellation", PreservesCallerCancellation),
    ("returns offline on refusal", ReturnsOfflineOnRefusal),
    ("returns offline on timeout", ReturnsOfflineOnTimeout),
    ("puts cursor cookie credential", PutsCursorCookieCredential),
    ("puts cursor access token credential", PutsCursorAccessTokenCredential),
    ("puts Grok cookie credential", PutsGrokCookieCredential),
    ("maps provider JSON errors", MapsProviderJsonErrors),
    ("rejects contradictory is_success", RejectsContradictoryIsSuccess),
    ("WIN-1 maps usage buckets", BucketMappingTests.MapsUsageBuckets),
    ("WIN-2 synthesizes missing usage buckets", BucketMappingTests.SynthesizesMissingUsageBuckets),
    ("WIN-3 rejects invalid bucket utilization", BucketMappingTests.RejectsInvalidBucketUtilization),
    ("WIN-4 maps error response with empty buckets", BucketMappingTests.MapsErrorResponseWithEmptyBuckets),
    ("WIN-5 rejects null bucket element", BucketMappingTests.RejectsNullBucketElement),
    ("WIN-6 rejects oversized bucket list", BucketMappingTests.RejectsOversizedBucketList),
    ("maps health version from /api/v1/health", MapsHealthVersion),
    ("rejects blank health version", RejectsBlankHealthVersion),
    ("returns unauthorized on health 401", ReturnsUnauthorizedOnHealth)
};

foreach (var test in tests)
{
    await test.Run();
    Console.WriteLine($"PASS {test.Name}");
}

if (args is ["--live-base-url", var baseUrl])
{
    await LiveServerReturnsEmptyUsage(baseUrl);
    Console.WriteLine("PASS live disabled-provider T9 server returns empty usage");
}

static async Task MapsFrozenUsageJson()
{
    var client = NewClient(new QueueHandler(Json(UsageJson())));
    var result = await client.GetProviderAsync("Cursor");
    Assert(result.Status == ApiResultStatus.Success, result.Error ?? "expected success");
    var usage = result.Value!;
    Assert(usage.ProviderName == "Cursor", "provider_name mapped");
    Assert(usage.PrimaryLabel == "Included", "primary_label mapped");
    Assert(usage.SecondaryLabel == "Usage", "secondary_label mapped");
    Assert(!usage.ShowSecondary, "show_secondary mapped");
    Assert(usage.Subtitle == null, "subtitle null mapped");
    Assert(usage.PrimaryStatusText == "primary", "primary status mapped");
    Assert(usage.SecondaryStatusText == null, "secondary status null mapped");
    Assert(usage.ReauthCommand == "cursor", "reauth command mapped");
    Assert(usage.Current.Utilization == 12.5f, "current utilization mapped");
    Assert(usage.Current.ResetsAt?.ToUniversalTime() == DateTime.Parse("2026-07-12T01:02:03Z").ToUniversalTime(), "current reset mapped");
    Assert(usage.Weekly.Utilization == 70.25f, "weekly utilization mapped");
    Assert(usage.Weekly.ResetsAt == null, "weekly reset null mapped");
    Assert(usage.Error == null, "error mapped");
    Assert(!usage.NeedsReauth, "needs_reauth mapped");
    Assert(usage.IsSuccess, "existing IsSuccess remains derived from Error");
}

static async Task AddsBearerAuthAndEscapesProvider()
{
    var handler = new QueueHandler(Json(UsageJson()));
    var client = NewClient(handler, " token ");
    await client.GetProviderAsync("Cursor Pro/Team");
    Assert(handler.Requests[0].RequestUri?.PathAndQuery == "/api/v1/usage/Cursor%20Pro%2FTeam", "provider path escaped");
    Assert(handler.Requests[0].Headers.Authorization?.Scheme == "Bearer", "bearer scheme set");
    Assert(handler.Requests[0].Headers.Authorization?.Parameter == "token", "bearer token trimmed");
    Assert(handler.Requests[0].Headers.Accept.Any(x => x.MediaType == "application/json"), "accept JSON set");
}

static async Task ReturnsProviderNotFound()
{
    var client = NewClient(new QueueHandler(new HttpResponseMessage(HttpStatusCode.NotFound)));
    var result = await client.GetProviderAsync("missing");
    Assert(result.Status == ApiResultStatus.ProviderNotFound, "404 classified");
}

static async Task ReturnsUnauthorized()
{
    var client = NewClient(new QueueHandler(new HttpResponseMessage(HttpStatusCode.Unauthorized)));
    var result = await client.GetUsageAsync();
    Assert(result.Status == ApiResultStatus.Unauthorized, "401 classified");
}

static async Task ReturnsApiError()
{
    var client = NewClient(new QueueHandler(new HttpResponseMessage(HttpStatusCode.InternalServerError)));
    var result = await client.GetProviderAsync("Cursor");
    Assert(result.Status == ApiResultStatus.ApiError, "500 classified API error");
}

static async Task ReturnsMalformedResponse()
{
    var client = NewClient(new QueueHandler(Json("{not-json")));
    var result = await client.GetProviderAsync("Cursor");
    Assert(result.Status == ApiResultStatus.MalformedResponse, "malformed classified");
}

static async Task RejectsMissingCurrent()
{
    var client = NewClient(new QueueHandler(Json(UsageJson().Replace(",\"current\":{\"utilization\":12.5,\"resets_at\":\"2026-07-12T01:02:03Z\"}", "", StringComparison.Ordinal))));
    var result = await client.GetProviderAsync("Cursor");
    Assert(result.Status == ApiResultStatus.MalformedResponse, "missing current rejected");
}

static async Task RejectsNullWeekly()
{
    var client = NewClient(new QueueHandler(Json(UsageJson().Replace("\"weekly\":{\"utilization\":70.25,\"resets_at\":null}", "\"weekly\":null", StringComparison.Ordinal))));
    var result = await client.GetProviderAsync("Cursor");
    Assert(result.Status == ApiResultStatus.MalformedResponse, "null weekly rejected");
}

static async Task RejectsMissingProviderName()
{
    var client = NewClient(new QueueHandler(Json(UsageJson().Replace("\"provider_name\":\"Cursor\",", "", StringComparison.Ordinal))));
    var result = await client.GetProviderAsync("Cursor");
    Assert(result.Status == ApiResultStatus.MalformedResponse, "missing provider_name rejected");
}

static async Task RejectsInvalidUtilization()
{
    foreach (var utilization in new[] { "NaN", "1e400", "-0.1", "100.1" })
    {
        var client = NewClient(new QueueHandler(Json(UsageJson().Replace("\"utilization\":12.5", $"\"utilization\":{utilization}", StringComparison.Ordinal))));
        var result = await client.GetProviderAsync("Cursor");
        Assert(result.Status == ApiResultStatus.MalformedResponse, $"invalid utilization {utilization} rejected");
    }
}

static async Task RejectsMalformedTimestamp()
{
    var client = NewClient(new QueueHandler(Json(UsageJson().Replace("2026-07-12T01:02:03Z", "not-a-date", StringComparison.Ordinal))));
    var result = await client.GetProviderAsync("Cursor");
    Assert(result.Status == ApiResultStatus.MalformedResponse, "malformed timestamp rejected");
}

static async Task ThrowsOnBlankProvider()
{
    var client = NewClient(new QueueHandler(Json(UsageJson())));
    try
    {
        await client.GetProviderAsync(" ");
    }
    catch (ArgumentException)
    {
        return;
    }
    throw new InvalidOperationException("blank provider should throw ArgumentException");
}

static async Task PreservesCallerCancellation()
{
    var handler = new BlockingHandler();
    var client = NewClient(handler, timeout: TimeSpan.FromSeconds(5));
    using var cts = new CancellationTokenSource();
    var task = client.GetUsageAsync(cts.Token);
    await handler.Started.Task;
    cts.Cancel();
    try
    {
        await task;
    }
    catch (OperationCanceledException) when (cts.IsCancellationRequested)
    {
        return;
    }
    throw new InvalidOperationException("caller cancellation should be preserved");
}

static async Task ReturnsOfflineOnRefusal()
{
    var client = NewClient(new QueueHandler(new HttpRequestException("connection refused")));
    var result = await client.GetProviderAsync("Cursor");
    Assert(result.Status == ApiResultStatus.Offline, "refusal classified offline");
}

static async Task ReturnsOfflineOnTimeout()
{
    var client = NewClient(new BlockingHandler(), timeout: TimeSpan.FromMilliseconds(20));
    var result = await client.GetUsageAsync();
    Assert(result.Status == ApiResultStatus.Offline, "timeout classified offline");
}

static async Task PutsCursorCookieCredential()
{
    var handler = new QueueHandler(Json(CredentialJson()));
    var client = NewClient(handler);
    var result = await client.PutCursorCredentialsAsync(CursorCredential.FromCookie(" cookie=value "));
    var body = await handler.Requests[0].Content!.ReadAsStringAsync();
    Assert(result.Status == ApiResultStatus.Success, "credential put success");
    Assert(handler.Requests[0].Method == HttpMethod.Put, "uses PUT");
    Assert(handler.Requests[0].Content?.Headers.ContentType?.MediaType == "application/json", "content type JSON");
    Assert(body.Contains("\"cookie\":\"cookie=value\"", StringComparison.Ordinal), "cookie sent");
    Assert(!body.Contains("access_token", StringComparison.Ordinal), "access token omitted");
}

static async Task PutsCursorAccessTokenCredential()
{
    var handler = new QueueHandler(Json(CredentialJson()));
    var client = NewClient(handler);
    await client.PutCursorCredentialsAsync(CursorCredential.FromAccessToken(" access "));
    var body = await handler.Requests[0].Content!.ReadAsStringAsync();
    Assert(body.Contains("\"access_token\":\"access\"", StringComparison.Ordinal), "access token sent");
    Assert(!body.Contains("cookie", StringComparison.Ordinal), "cookie omitted");
}

static async Task PutsGrokCookieCredential()
{
    var handler = new QueueHandler(Json(CredentialJson().Replace("\"provider\":\"Cursor\"", "\"provider\":\"Grok\"", StringComparison.Ordinal)));
    var client = NewClient(handler);
    var result = await client.PutGrokCredentialsAsync(GrokCredential.FromCookie(" sso=session "));
    var body = await handler.Requests[0].Content!.ReadAsStringAsync();
    Assert(result.Status == ApiResultStatus.Success, "Grok credential put success");
    Assert(handler.Requests[0].RequestUri?.PathAndQuery == "/api/v1/providers/grok/credentials", "uses Grok credential path");
    Assert(body.Contains("\"cookie\":\"sso=session\"", StringComparison.Ordinal), "Grok cookie sent");
}

static async Task MapsProviderJsonErrors()
{
    var json = UsageJson().Replace("\"error\":null", "\"error\":\"Cursor auth expired\"", StringComparison.Ordinal)
        .Replace("\"needs_reauth\":false", "\"needs_reauth\":true", StringComparison.Ordinal)
        .Replace("\"is_success\":true", "\"is_success\":false", StringComparison.Ordinal);
    var client = NewClient(new QueueHandler(Json(json)));
    var result = await client.GetProviderAsync("Cursor");
    Assert(result.Status == ApiResultStatus.Success, "provider error JSON is still a valid response");
    Assert(result.Value!.Error == "Cursor auth expired", "provider error mapped");
    Assert(result.Value.NeedsReauth, "provider reauth mapped");
    Assert(!result.Value.IsSuccess, "UI success remains derived from Error");
}

static async Task RejectsContradictoryIsSuccess()
{
    var client = NewClient(new QueueHandler(Json(UsageJson().Replace("\"is_success\":true", "\"is_success\":false", StringComparison.Ordinal))));
    var result = await client.GetProviderAsync("Cursor");
    Assert(result.Status == ApiResultStatus.MalformedResponse, "contradictory derived is_success rejected");
}

static async Task MapsHealthVersion()
{
    var handler = new QueueHandler(Json("""{"status":"ok","version":"1.2.3","providers":[]}"""));
    var client = NewClient(handler, " secret ");
    var result = await client.GetHealthAsync();
    Assert(result.Status == ApiResultStatus.Success, result.Error ?? "expected health success");
    Assert(result.Value!.Version == "1.2.3", "health version mapped");
    Assert(result.Value.Status == "ok", "health status mapped");
    Assert(handler.Requests[0].RequestUri?.PathAndQuery == "/api/v1/health", "health path used");
    Assert(handler.Requests[0].Headers.Authorization?.Parameter == "secret", "health uses bearer token");
}

static async Task RejectsBlankHealthVersion()
{
    var client = NewClient(new QueueHandler(Json("""{"status":"ok","version":"  ","providers":[]}""")));
    var result = await client.GetHealthAsync();
    Assert(result.Status == ApiResultStatus.MalformedResponse, "blank health version rejected");
}

static async Task ReturnsUnauthorizedOnHealth()
{
    var client = NewClient(new QueueHandler(new HttpResponseMessage(HttpStatusCode.Unauthorized)));
    var result = await client.GetHealthAsync();
    Assert(result.Status == ApiResultStatus.Unauthorized, "health 401 classified");
}

static ApiClient NewClient(HttpMessageHandler handler, string? token = null, TimeSpan? timeout = null)
{
    var http = new HttpClient(handler);
    return new ApiClient(http, new TrayApiClientOptions { BaseUrl = new Uri("http://localhost:7823/base/.."), BearerToken = token, Timeout = timeout ?? TimeSpan.FromSeconds(1) });
}

static async Task LiveServerReturnsEmptyUsage(string baseUrl)
{
    var client = new ApiClient(new HttpClient(), new TrayApiClientOptions { BaseUrl = new Uri(baseUrl), Timeout = TimeSpan.FromSeconds(2) });
    var result = await client.GetUsageAsync();
    Assert(result.Status == ApiResultStatus.Success, result.Error ?? "live usage failed");
    Assert(result.Value!.Count == 0, "disabled-provider server returns no usage entries");
}

static HttpResponseMessage Json(string json) => new(HttpStatusCode.OK)
{
    Content = new StringContent(json, Encoding.UTF8, "application/json")
};

static string CredentialJson() => $$"""
{"provider":"Cursor","refetched":true,"usage":{{UsageJson()}}}
""";

static string UsageJson() => """
{"provider_name":"Cursor","primary_label":"Included","secondary_label":"Usage","show_secondary":false,"subtitle":null,"primary_status_text":"primary","secondary_status_text":null,"reauth_command":"cursor","current":{"utilization":12.5,"resets_at":"2026-07-12T01:02:03Z"},"weekly":{"utilization":70.25,"resets_at":null},"error":null,"needs_reauth":false,"is_success":true}
""";

static void Assert(bool condition, string message)
{
    if (!condition) throw new InvalidOperationException(message);
}
