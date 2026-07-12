using System.Runtime.InteropServices;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace ClaudeUsageWidget.Services;

public partial class UpdateService
{
    private static Version? ParseVersion(string? tagName)
    {
        if (string.IsNullOrWhiteSpace(tagName))
        {
            return null;
        }

        return Version.TryParse(tagName.TrimStart('v', 'V'), out var version) ? version : null;
    }

    private static UpdateAssetUrls? FindAssetUrls(List<GitHubAsset>? assets)
    {
        var suffix = RuntimeInformation.ProcessArchitecture switch
        {
            Architecture.X64 => "win-x64",
            Architecture.Arm64 => "win-arm64",
            _ => null
        };
        if (suffix == null || assets == null)
        {
            return null;
        }

        var appUrl = FindAssetUrl(assets, $"ClaudeUsageWidget-{suffix}.exe");
        if (appUrl == null)
        {
            return null;
        }
        return new UpdateAssetUrls(appUrl, FindAssetUrl(assets, $"usage-server-{suffix}.exe"));
    }

    private static string? FindAssetUrl(List<GitHubAsset> assets, string expectedName) => assets
        .FirstOrDefault(asset => asset.Name?.Equals(expectedName, StringComparison.OrdinalIgnoreCase) == true)
        ?.BrowserDownloadUrl;

    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNameCaseInsensitive = true
    };
}

internal sealed record UpdateAssetUrls(string AppUrl, string? ServerUrl);

internal sealed class GitHubRelease
{
    [JsonPropertyName("tag_name")]
    public string? TagName { get; set; }

    [JsonPropertyName("assets")]
    public List<GitHubAsset>? Assets { get; set; }
}

internal sealed class GitHubAsset
{
    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("browser_download_url")]
    public string? BrowserDownloadUrl { get; set; }
}
