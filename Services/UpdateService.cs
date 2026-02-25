using System.Net.Http.Json;
using System.Reflection;
using System.Runtime.InteropServices;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace ClaudeUsageWidget.Services;

/// <summary>
/// Service that checks for updates from GitHub releases and downloads them.
/// </summary>
public class UpdateService
{
    private const string GitHubApiUrl = "https://api.github.com/repos/AaronFeledy/claude-usage-widget/releases/latest";
    private const string UpdateFileName = "ClaudeUsageWidget.update.exe";
    private const string OldFileName = "ClaudeUsageWidget.old.exe";

    private readonly HttpClient _httpClient;
    private readonly NotifyIcon _notifyIcon;

    /// <summary>
    /// Gets the current application version from the assembly.
    /// </summary>
    public static Version CurrentVersion => Assembly.GetExecutingAssembly().GetName().Version ?? new Version(0, 0, 0);

    public UpdateService(HttpClient httpClient, NotifyIcon notifyIcon)
    {
        _httpClient = httpClient;
        _notifyIcon = notifyIcon;
    }

    /// <summary>
    /// Checks for updates and downloads if available. Runs silently in background.
    /// </summary>
    public async Task CheckForUpdatesAsync(bool showNotificationIfNoUpdate = false)
    {
        try
        {
            var release = await FetchLatestReleaseAsync();
            if (release == null)
            {
                if (showNotificationIfNoUpdate)
                {
                    ShowBalloon("Update Check", "Unable to check for updates.", ToolTipIcon.Warning);
                }
                return;
            }

            var latestVersion = ParseVersion(release.TagName);
            if (latestVersion == null)
            {
                if (showNotificationIfNoUpdate)
                {
                    ShowBalloon("Update Check", "Unable to parse version info.", ToolTipIcon.Warning);
                }
                return;
            }

            if (latestVersion <= CurrentVersion)
            {
                if (showNotificationIfNoUpdate)
                {
                    ShowBalloon("Up to Date", $"You're running the latest version (v{CurrentVersion.Major}.{CurrentVersion.Minor}.{CurrentVersion.Build}).", ToolTipIcon.Info);
                }
                return;
            }

            // Find the appropriate asset for this architecture
            var assetUrl = FindAssetUrl(release.Assets);
            if (assetUrl == null)
            {
                return; // No matching asset, fail silently
            }

            // Download the update
            var downloaded = await DownloadUpdateAsync(assetUrl);
            if (downloaded)
            {
                var versionStr = $"v{latestVersion.Major}.{latestVersion.Minor}.{latestVersion.Build}";
                ShowBalloon("Update Available", $"Update available ({versionStr}). Restart to apply.", ToolTipIcon.Info);
            }
        }
        catch
        {
            // Fail silently - don't show any errors to user
            if (showNotificationIfNoUpdate)
            {
                ShowBalloon("Update Check", "Unable to check for updates.", ToolTipIcon.Warning);
            }
        }
    }

    /// <summary>
    /// Fetches the latest release info from GitHub API.
    /// </summary>
    private async Task<GitHubRelease?> FetchLatestReleaseAsync()
    {
        using var request = new HttpRequestMessage(HttpMethod.Get, GitHubApiUrl);
        request.Headers.Add("User-Agent", "ClaudeUsageWidget");
        request.Headers.Add("Accept", "application/vnd.github.v3+json");

        var response = await _httpClient.SendAsync(request);
        if (!response.IsSuccessStatusCode)
        {
            return null;
        }

        return await response.Content.ReadFromJsonAsync<GitHubRelease>(JsonOptions);
    }

    /// <summary>
    /// Parses a version string like "v0.1.1" or "0.1.1" into a Version object.
    /// </summary>
    private static Version? ParseVersion(string? tagName)
    {
        if (string.IsNullOrWhiteSpace(tagName))
            return null;

        // Remove leading 'v' if present
        var versionStr = tagName.TrimStart('v', 'V');
        
        return Version.TryParse(versionStr, out var version) ? version : null;
    }

    /// <summary>
    /// Finds the download URL for the appropriate architecture asset.
    /// </summary>
    private static string? FindAssetUrl(List<GitHubAsset>? assets)
    {
        if (assets == null || assets.Count == 0)
            return null;

        var arch = RuntimeInformation.ProcessArchitecture;
        var archSuffix = arch switch
        {
            Architecture.X64 => "win-x64",
            Architecture.Arm64 => "win-arm64",
            _ => null
        };

        if (archSuffix == null)
            return null;

        var expectedFileName = $"ClaudeUsageWidget-{archSuffix}.exe";
        var asset = assets.FirstOrDefault(a => 
            a.Name?.Equals(expectedFileName, StringComparison.OrdinalIgnoreCase) == true);

        return asset?.BrowserDownloadUrl;
    }

    /// <summary>
    /// Downloads the update to a temp file next to the current exe.
    /// </summary>
    private async Task<bool> DownloadUpdateAsync(string url)
    {
        var currentExePath = Environment.ProcessPath;
        if (string.IsNullOrEmpty(currentExePath))
            return false;

        var exeDir = Path.GetDirectoryName(currentExePath);
        if (string.IsNullOrEmpty(exeDir))
            return false;

        var updatePath = Path.Combine(exeDir, UpdateFileName);

        using var request = new HttpRequestMessage(HttpMethod.Get, url);
        request.Headers.Add("User-Agent", "ClaudeUsageWidget");

        var response = await _httpClient.SendAsync(request, HttpCompletionOption.ResponseHeadersRead);
        if (!response.IsSuccessStatusCode)
            return false;

        // Download to a temp file first, then rename
        var tempPath = updatePath + ".tmp";
        try
        {
            await using var stream = await response.Content.ReadAsStreamAsync();
            await using var fileStream = new FileStream(tempPath, FileMode.Create, FileAccess.Write, FileShare.None);
            await stream.CopyToAsync(fileStream);

            // Rename temp file to final name
            if (File.Exists(updatePath))
                File.Delete(updatePath);
            File.Move(tempPath, updatePath);

            return true;
        }
        catch
        {
            // Clean up temp file on failure
            try { File.Delete(tempPath); } catch { }
            return false;
        }
    }

    /// <summary>
    /// Shows a balloon notification.
    /// </summary>
    private void ShowBalloon(string title, string text, ToolTipIcon icon)
    {
        _notifyIcon.ShowBalloonTip(5000, title, text, icon);
    }

    /// <summary>
    /// Applies a pending update by swapping executables and restarting.
    /// Call this from Program.cs BEFORE the mutex check.
    /// Returns true if the app should exit (update was applied and new process started).
    /// </summary>
    public static bool ApplyPendingUpdate()
    {
        var currentExePath = Environment.ProcessPath;
        if (string.IsNullOrEmpty(currentExePath))
            return false;

        var exeDir = Path.GetDirectoryName(currentExePath);
        if (string.IsNullOrEmpty(exeDir))
            return false;

        var currentExeName = Path.GetFileName(currentExePath);
        var updatePath = Path.Combine(exeDir, UpdateFileName);
        var oldPath = Path.Combine(exeDir, OldFileName);

        // Cleanup: delete old exe from previous update
        if (File.Exists(oldPath))
        {
            try { File.Delete(oldPath); } catch { }
        }

        // Check if there's a pending update
        if (!File.Exists(updatePath))
            return false;

        try
        {
            // Rename current exe to .old.exe
            File.Move(currentExePath, oldPath, overwrite: true);

            // Rename .update.exe to current exe name
            File.Move(updatePath, currentExePath);

            // Start the new process
            System.Diagnostics.Process.Start(new System.Diagnostics.ProcessStartInfo
            {
                FileName = currentExePath,
                UseShellExecute = true
            });

            // Exit current process
            return true;
        }
        catch
        {
            // If something went wrong, try to restore the original exe
            try
            {
                if (File.Exists(oldPath) && !File.Exists(currentExePath))
                {
                    File.Move(oldPath, currentExePath);
                }
            }
            catch { }

            return false;
        }
    }

    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNameCaseInsensitive = true
    };
}

/// <summary>
/// GitHub release response model.
/// </summary>
internal class GitHubRelease
{
    [JsonPropertyName("tag_name")]
    public string? TagName { get; set; }

    [JsonPropertyName("assets")]
    public List<GitHubAsset>? Assets { get; set; }
}

/// <summary>
/// GitHub release asset model.
/// </summary>
internal class GitHubAsset
{
    [JsonPropertyName("name")]
    public string? Name { get; set; }

    [JsonPropertyName("browser_download_url")]
    public string? BrowserDownloadUrl { get; set; }
}
