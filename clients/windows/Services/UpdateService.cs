using System.Net.Http.Json;
using System.Reflection;

namespace ClaudeUsageWidget.Services;

/// <summary>
/// Service that checks for updates from GitHub releases and downloads them.
/// </summary>
public partial class UpdateService
{
    private const string GitHubApiUrl = "https://api.github.com/repos/AaronFeledy/claude-usage-widget/releases/latest";

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

            var assetUrls = FindAssetUrls(release.Assets);
            if (assetUrls?.AppUrl == null)
            {
                return; // No matching asset, fail silently
            }

            var downloaded = await DownloadUpdateAsync(assetUrls);
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

    private async Task<bool> DownloadUpdateAsync(UpdateAssetUrls assetUrls)
    {
        var currentExePath = Environment.ProcessPath;
        if (string.IsNullOrEmpty(currentExePath))
            return false;

        var exeDir = Path.GetDirectoryName(currentExePath);
        if (string.IsNullOrEmpty(exeDir))
            return false;

        var appUpdatePath = Path.Combine(exeDir, PendingUpdateApplicator.AppUpdateFileName);
        var serverUpdatePath = Path.Combine(exeDir, PendingUpdateApplicator.ServerUpdateFileName);
        var appTempPath = appUpdatePath + ".tmp";
        var serverTempPath = serverUpdatePath + ".tmp";
        try
        {
            await DownloadFileAsync(assetUrls.AppUrl, appTempPath).ConfigureAwait(false);
            if (assetUrls.ServerUrl != null)
            {
                await DownloadFileAsync(assetUrls.ServerUrl, serverTempPath).ConfigureAwait(false);
            }

            File.Move(appTempPath, appUpdatePath, overwrite: true);
            if (assetUrls.ServerUrl != null)
            {
                File.Move(serverTempPath, serverUpdatePath, overwrite: true);
            }

            return true;
        }
        catch
        {
            DeleteIfExists(appTempPath);
            DeleteIfExists(serverTempPath);
            if (assetUrls.ServerUrl != null)
            {
                DeleteIfExists(appUpdatePath);
            }
            return false;
        }
    }

    private async Task DownloadFileAsync(string url, string tempPath)
    {
        using var request = new HttpRequestMessage(HttpMethod.Get, url);
        request.Headers.Add("User-Agent", "ClaudeUsageWidget");

        using var response = await _httpClient.SendAsync(request, HttpCompletionOption.ResponseHeadersRead).ConfigureAwait(false);
        if (!response.IsSuccessStatusCode)
        {
            throw new HttpRequestException("Update asset download failed.");
        }

        await using var stream = await response.Content.ReadAsStreamAsync().ConfigureAwait(false);
        await using var fileStream = new FileStream(tempPath, FileMode.Create, FileAccess.Write, FileShare.None);
        await stream.CopyToAsync(fileStream).ConfigureAwait(false);
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
        => PendingUpdateApplicator.ApplyFromCurrentProcess();

    private static void DeleteIfExists(string path)
    {
        try { File.Delete(path); } catch { }
    }
}
