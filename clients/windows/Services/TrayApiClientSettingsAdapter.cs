namespace ClaudeUsageWidget.Services;

public sealed class TrayApiClientSettingsAdapter : ITrayApiClientSettings
{
    private readonly SettingsService _settingsService;

    public TrayApiClientSettingsAdapter(SettingsService settingsService, Uri baseUrl)
    {
        _settingsService = settingsService;
        BaseUrl = baseUrl;
    }

    public Uri BaseUrl { get; private set; }
    public string? BearerToken => string.IsNullOrWhiteSpace(_settingsService.Settings.ApiToken)
        ? null
        : _settingsService.Settings.ApiToken;
    public TimeSpan Timeout { get; init; } = TimeSpan.FromSeconds(10);

    public void SetBaseUrl(Uri baseUrl) => BaseUrl = baseUrl;
}
