using System.Net;
using System.Text;
using ClaudeUsageWidget.Services;

internal static class BucketMappingTests
{
    internal static async Task MapsUsageBuckets()
    {
        var json = UsageJson().Replace(
            ",\"error\":null",
            ",\"buckets\":[{\"id\":\"fast\",\"label\":\"Fast requests\",\"utilization\":33.5,\"resets_at\":\"2026-07-13T04:05:06Z\"},{\"id\":\"slow\",\"label\":\"Slow requests\",\"utilization\":44,\"resets_at\":null}],\"error\":null",
            StringComparison.Ordinal);
        var client = NewClient(json);

        var result = await client.GetProviderAsync("Cursor");

        Assert(result.Status == ApiResultStatus.Success, result.Error ?? "expected success");
        Assert(result.Value!.Buckets.Count == 2, "bucket count mapped");
        Assert(result.Value.Buckets[0].Id == "fast", "bucket id mapped");
        Assert(result.Value.Buckets[0].Label == "Fast requests", "bucket label mapped");
        Assert(result.Value.Buckets[0].Utilization == 33.5f, "bucket utilization mapped");
        Assert(result.Value.Buckets[0].ResetsAt?.ToUniversalTime() == DateTime.Parse("2026-07-13T04:05:06Z").ToUniversalTime(), "bucket reset mapped");
        Assert(result.Value.Buckets[1].ResetsAt == null, "null bucket reset mapped");
    }

    internal static async Task SynthesizesMissingUsageBuckets()
    {
        var json = UsageJson().Replace("\"show_secondary\":false", "\"show_secondary\":true", StringComparison.Ordinal);
        var client = NewClient(json);

        var result = await client.GetProviderAsync("Cursor");

        Assert(result.Status == ApiResultStatus.Success, result.Error ?? "expected success");
        Assert(result.Value!.Buckets.Count == 2, "current and weekly buckets synthesized");
        Assert(result.Value.Buckets[0].Id == "session", "session id synthesized");
        Assert(result.Value.Buckets[0].Label == "Included", "primary label synthesized");
        Assert(result.Value.Buckets[0].Utilization == 12.5f, "current utilization synthesized");
        Assert(result.Value.Buckets[1].Id == "weekly", "weekly id synthesized");
        Assert(result.Value.Buckets[1].Label == "Usage", "secondary label synthesized");
        Assert(result.Value.Buckets[1].Utilization == 70.25f, "weekly utilization synthesized");
    }

    internal static async Task RejectsInvalidBucketUtilization()
    {
        var json = UsageJson().Replace(
            ",\"error\":null",
            ",\"buckets\":[{\"id\":\"fast\",\"label\":\"Fast requests\",\"utilization\":150,\"resets_at\":null}],\"error\":null",
            StringComparison.Ordinal);
        var client = NewClient(json);

        var result = await client.GetProviderAsync("Cursor");

        Assert(result.Status == ApiResultStatus.MalformedResponse, "invalid bucket utilization rejected");
    }

    internal static async Task RejectsNullBucketElement()
    {
        var json = UsageJson().Replace(
            ",\"error\":null",
            ",\"buckets\":[null],\"error\":null",
            StringComparison.Ordinal);
        var client = NewClient(json);

        var result = await client.GetProviderAsync("Cursor");

        Assert(result.Status == ApiResultStatus.MalformedResponse, "null bucket element rejected without crashing");
    }

    internal static async Task RejectsOversizedBucketList()
    {
        var entries = string.Join(",", Enumerable.Range(0, 13).Select(i =>
            $"{{\"id\":\"b{i}\",\"label\":\"L{i}\",\"utilization\":10,\"resets_at\":null}}"));
        var json = UsageJson().Replace(
            ",\"error\":null",
            $",\"buckets\":[{entries}],\"error\":null",
            StringComparison.Ordinal);
        var client = NewClient(json);

        var result = await client.GetProviderAsync("Cursor");

        Assert(result.Status == ApiResultStatus.MalformedResponse, "bucket list over cap rejected");
    }

    internal static async Task MapsErrorResponseWithEmptyBuckets()
    {
        var json = UsageJson()
            .Replace(",\"error\":null", ",\"buckets\":[],\"error\":\"Cursor auth expired\"", StringComparison.Ordinal)
            .Replace("\"needs_reauth\":false", "\"needs_reauth\":true", StringComparison.Ordinal)
            .Replace("\"is_success\":true", "\"is_success\":false", StringComparison.Ordinal);
        var client = NewClient(json);

        var result = await client.GetProviderAsync("Cursor");

        Assert(result.Status == ApiResultStatus.Success, result.Error ?? "expected valid provider error response");
        Assert(result.Value!.Error == "Cursor auth expired", "provider error mapped");
        Assert(result.Value.Buckets.Count == 1, "empty buckets synthesized from visible legacy bucket");
        Assert(result.Value.Buckets[0].Id == "session", "session fallback synthesized");
    }

    private static ApiClient NewClient(string json)
    {
        var response = new HttpResponseMessage(HttpStatusCode.OK)
        {
            Content = new StringContent(json, Encoding.UTF8, "application/json")
        };
        return new ApiClient(
            new HttpClient(new QueueHandler(response)),
            new TrayApiClientOptions { BaseUrl = new Uri("http://localhost:7823/") });
    }

    private static string UsageJson() => """
        {"provider_name":"Cursor","primary_label":"Included","secondary_label":"Usage","show_secondary":false,"subtitle":null,"primary_status_text":"primary","secondary_status_text":null,"reauth_command":"cursor","current":{"utilization":12.5,"resets_at":"2026-07-12T01:02:03Z"},"weekly":{"utilization":70.25,"resets_at":null},"error":null,"needs_reauth":false,"is_success":true}
        """;

    private static void Assert(bool condition, string message)
    {
        if (!condition) throw new InvalidOperationException(message);
    }
}
