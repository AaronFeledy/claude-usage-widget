using System.Net.Http.Json;
using System.Runtime.InteropServices;
using System.Text.Json.Serialization;

namespace ClaudeUsageWidget.Services;

public sealed class UpdateServiceServerBinaryAcquirer : IServerBinaryAcquirer
{
    private const string LatestReleaseUrl = "https://api.github.com/repos/AaronFeledy/claude-usage-widget/releases/latest";
    private const long DefaultMaxDownloadBytes = 100 * 1024 * 1024;
    private readonly HttpClient _httpClient;
    private readonly Architecture? _architectureOverride;
    private readonly long _maxDownloadBytes;

    public UpdateServiceServerBinaryAcquirer(HttpClient httpClient)
        : this(httpClient, null, DefaultMaxDownloadBytes)
    {
    }

    internal UpdateServiceServerBinaryAcquirer(HttpClient httpClient, Architecture? architectureOverride, long maxDownloadBytes)
    {
        _httpClient = httpClient;
        _architectureOverride = architectureOverride;
        _maxDownloadBytes = maxDownloadBytes;
    }

    public async Task<bool> AcquireAsync(ServerBinaryRequest request, CancellationToken cancellationToken)
    {
        var temps = TempPaths(request.Path);
        try
        {
            using var releaseRequest = new HttpRequestMessage(HttpMethod.Get, LatestReleaseUrl);
            releaseRequest.Headers.Add("User-Agent", "ClaudeUsageWidget");
            releaseRequest.Headers.Add("Accept", "application/vnd.github.v3+json");
            using var releaseResponse = await _httpClient.SendAsync(releaseRequest, cancellationToken).ConfigureAwait(false);
            if (!releaseResponse.IsSuccessStatusCode)
            {
                return false;
            }
            var release = await releaseResponse.Content.ReadFromJsonAsync<ServerGitHubRelease>(cancellationToken).ConfigureAwait(false);
            var assetUrl = FindServerAssetUrl(release?.Assets);
            return assetUrl != null && await DownloadAssetAsync(assetUrl, request, temps, cancellationToken).ConfigureAwait(false);
        }
        catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
        {
            DeleteIfExists(temps.ExecutableTempPath);
            DeleteIfExists(temps.VersionTempPath);
            throw;
        }
    }

    private async Task<bool> DownloadAssetAsync(string assetUrl, ServerBinaryRequest binary, ServerBinaryTempPaths temps, CancellationToken cancellationToken)
    {
        using var request = new HttpRequestMessage(HttpMethod.Get, assetUrl);
        request.Headers.Add("User-Agent", "ClaudeUsageWidget");
        using var response = await _httpClient.SendAsync(request, HttpCompletionOption.ResponseHeadersRead, cancellationToken).ConfigureAwait(false);
        if (!response.IsSuccessStatusCode || response.Content.Headers.ContentLength is long length && length > _maxDownloadBytes)
        {
            return false;
        }
        Directory.CreateDirectory(Path.GetDirectoryName(binary.Path) ?? AppContext.BaseDirectory);
        try
        {
            await CopyBoundedAsync(response, temps.ExecutableTempPath, cancellationToken).ConfigureAwait(false);
            if (!string.IsNullOrWhiteSpace(binary.ExpectedVersion))
            {
                await File.WriteAllTextAsync(temps.VersionTempPath, binary.ExpectedVersion.Trim(), cancellationToken).ConfigureAwait(false);
            }
            File.Move(temps.ExecutableTempPath, binary.Path, overwrite: true);
            if (!string.IsNullOrWhiteSpace(binary.ExpectedVersion))
            {
                File.Move(temps.VersionTempPath, SidecarServerBinaryVersionReader.VersionPath(binary.Path), overwrite: true);
            }
            return true;
        }
        catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
        {
            DeleteIfExists(temps.ExecutableTempPath);
            DeleteIfExists(temps.VersionTempPath);
            throw;
        }
        catch
        {
            DeleteIfExists(temps.ExecutableTempPath);
            DeleteIfExists(temps.VersionTempPath);
            return false;
        }
    }

    private async Task CopyBoundedAsync(HttpResponseMessage response, string tempPath, CancellationToken cancellationToken)
    {
        var total = 0L;
        var buffer = new byte[81920];
        await using var source = await response.Content.ReadAsStreamAsync(cancellationToken).ConfigureAwait(false);
        await using var target = new FileStream(tempPath, FileMode.Create, FileAccess.Write, FileShare.None);
        while (true)
        {
            var read = await source.ReadAsync(buffer, cancellationToken).ConfigureAwait(false);
            if (read == 0)
            {
                await target.FlushAsync(cancellationToken).ConfigureAwait(false);
                return;
            }
            total += read;
            if (total > _maxDownloadBytes)
            {
                throw new InvalidDataException("usage-server download exceeded size limit");
            }
            await target.WriteAsync(buffer.AsMemory(0, read), cancellationToken).ConfigureAwait(false);
        }
    }

    private string? FindServerAssetUrl(List<ServerGitHubAsset>? assets)
    {
        var arch = (_architectureOverride ?? RuntimeInformation.ProcessArchitecture) switch
        {
            Architecture.X64 => "win-x64",
            Architecture.Arm64 => "win-arm64",
            _ => null
        };
        var expected = $"usage-server-{arch}.exe";
        return assets?.FirstOrDefault(asset => asset.Name?.Equals(expected, StringComparison.OrdinalIgnoreCase) == true)?.BrowserDownloadUrl;
    }

    private static ServerBinaryTempPaths TempPaths(string path)
    {
        var directory = Path.GetDirectoryName(path) ?? AppContext.BaseDirectory;
        return new ServerBinaryTempPaths(Path.Combine(directory, Path.GetFileName(path) + ".tmp"), SidecarServerBinaryVersionReader.VersionPath(path) + ".tmp");
    }

    private static void DeleteIfExists(string path)
    {
        try { File.Delete(path); } catch { }
    }
}

internal static class ArchitectureOverride
{
    public static Architecture X64 => Architecture.X64;
    public static Architecture Arm64 => Architecture.Arm64;
}

internal sealed class ServerGitHubRelease
{
    [JsonPropertyName("assets")]
    public List<ServerGitHubAsset>? Assets { get; set; }
}

internal sealed class ServerGitHubAsset
{
    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("browser_download_url")]
    public string? BrowserDownloadUrl { get; set; }
}

internal sealed record ServerBinaryTempPaths(string ExecutableTempPath, string VersionTempPath);
