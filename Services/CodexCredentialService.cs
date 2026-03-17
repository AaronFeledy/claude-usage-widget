using System.Text.Json;
using System.Text.Json.Nodes;
using System.Text;

namespace ClaudeUsageWidget.Services;

public enum CodexRefreshResult
{
    Success,
    InvalidGrant,
    Failed
}

public class CodexCredentialService
{
    private const string TokenRefreshUrl = "https://auth.openai.com/oauth/token";
    private const string OAuthClientId = "app_EMoamEEZ73f0CkXaXp7hrann";
    private static readonly TimeSpan RefreshWindow = TimeSpan.FromDays(8);

    private readonly HttpClient _httpClient;
    private readonly DebugService? _debugService;
    private readonly object _lock = new();

    private string? _accessToken;
    private string? _refreshToken;
    private string? _accountId;
    private DateTime? _lastRefreshUtc;

    public CodexCredentialService(HttpClient httpClient, DebugService? debugService = null)
    {
        _httpClient = httpClient;
        _debugService = debugService;
    }

    public string? AccessToken
    {
        get
        {
            lock (_lock)
            {
                if (_accessToken == null)
                    LoadCredentials();

                return _accessToken;
            }
        }
    }

    public string? AccountId
    {
        get
        {
            lock (_lock)
            {
                if (_accessToken == null)
                    LoadCredentials();

                return _accountId;
            }
        }
    }

    public bool NeedsRefresh()
    {
        lock (_lock)
        {
            if (_accessToken == null)
                LoadCredentials();

            if (string.IsNullOrWhiteSpace(_refreshToken))
                return false;

            return !_lastRefreshUtc.HasValue || DateTime.UtcNow - _lastRefreshUtc.Value > RefreshWindow;
        }
    }

    public void LoadCredentials()
    {
        lock (_lock)
        {
            var credentialsPath = GetCredentialsPath();
            _debugService?.LogInfo("CodexAuth", $"Loading credentials from {credentialsPath}");

            if (!File.Exists(credentialsPath))
                throw new FileNotFoundException($"Codex auth.json not found: {credentialsPath}");

            var json = File.ReadAllText(credentialsPath);
            var doc = JsonDocument.Parse(json);
            var root = doc.RootElement;

            if (root.TryGetProperty("OPENAI_API_KEY", out var apiKeyElement))
            {
                var apiKey = apiKeyElement.GetString();
                if (!string.IsNullOrWhiteSpace(apiKey))
                {
                    _accessToken = apiKey;
                    _refreshToken = null;
                    _accountId = null;
                    _lastRefreshUtc = null;
                    return;
                }
            }

            if (!root.TryGetProperty("tokens", out var tokens))
                throw new InvalidOperationException("Codex auth.json is missing tokens");

            _accessToken = tokens.GetProperty("access_token").GetString();
            _refreshToken = tokens.TryGetProperty("refresh_token", out var refreshToken) ? refreshToken.GetString() : null;
            _accountId = tokens.TryGetProperty("account_id", out var accountId) ? accountId.GetString() : null;

            if (root.TryGetProperty("last_refresh", out var lastRefresh))
            {
                var lastRefreshStr = lastRefresh.GetString();
                if (!string.IsNullOrWhiteSpace(lastRefreshStr) && DateTime.TryParse(lastRefreshStr, out var parsed))
                    _lastRefreshUtc = parsed.ToUniversalTime();
            }

            if (string.IsNullOrWhiteSpace(_accessToken))
                throw new InvalidOperationException("Codex auth.json contains no access token");
        }
    }

    public async Task<CodexRefreshResult> RefreshTokenAsync()
    {
        string? refreshToken;
        lock (_lock)
        {
            if (_accessToken == null)
                LoadCredentials();

            refreshToken = _refreshToken;
        }

        if (string.IsNullOrWhiteSpace(refreshToken))
            return CodexRefreshResult.InvalidGrant;

        try
        {
            var request = new HttpRequestMessage(HttpMethod.Post, TokenRefreshUrl)
            {
                Content = new StringContent(
                    JsonSerializer.Serialize(new
                    {
                        client_id = OAuthClientId,
                        grant_type = "refresh_token",
                        refresh_token = refreshToken,
                        scope = "openid profile email"
                    }),
                    Encoding.UTF8,
                    "application/json")
            };

            var response = await _httpClient.SendAsync(request);
            var responseBody = await response.Content.ReadAsStringAsync();

            if (!response.IsSuccessStatusCode)
            {
                _debugService?.LogError("CodexAuth", $"Token refresh failed: {(int)response.StatusCode}", responseBody);

                if ((int)response.StatusCode == 401 &&
                    (responseBody.Contains("refresh_token_expired", StringComparison.OrdinalIgnoreCase) ||
                     responseBody.Contains("refresh_token_reused", StringComparison.OrdinalIgnoreCase) ||
                     responseBody.Contains("refresh_token_invalidated", StringComparison.OrdinalIgnoreCase)))
                {
                    return CodexRefreshResult.InvalidGrant;
                }

                return CodexRefreshResult.Failed;
            }

            var doc = JsonDocument.Parse(responseBody);
            var root = doc.RootElement;

            var newAccessToken = root.TryGetProperty("access_token", out var accessToken)
                ? accessToken.GetString()
                : _accessToken;
            var newRefreshToken = root.TryGetProperty("refresh_token", out var refreshedToken)
                ? refreshedToken.GetString()
                : refreshToken;

            lock (_lock)
            {
                _accessToken = newAccessToken;
                _refreshToken = newRefreshToken;
                _lastRefreshUtc = DateTime.UtcNow;
            }

            await SaveCredentialsAsync();
            return CodexRefreshResult.Success;
        }
        catch (Exception ex)
        {
            _debugService?.LogError("CodexAuth", $"Token refresh exception: {ex.Message}", ex.ToString());
            return CodexRefreshResult.Failed;
        }
    }

    public void ReloadCredentials()
    {
        lock (_lock)
        {
            _accessToken = null;
            _refreshToken = null;
            _accountId = null;
            _lastRefreshUtc = null;
            LoadCredentials();
        }
    }

    private async Task SaveCredentialsAsync()
    {
        var credentialsPath = GetCredentialsPath();
        if (!File.Exists(credentialsPath))
            return;

        var json = await File.ReadAllTextAsync(credentialsPath);
        var jsonNode = JsonNode.Parse(json) ?? new JsonObject();
        var tokensNode = jsonNode["tokens"] as JsonObject ?? new JsonObject();

        tokensNode["access_token"] = _accessToken;
        if (!string.IsNullOrWhiteSpace(_refreshToken))
            tokensNode["refresh_token"] = _refreshToken;
        if (!string.IsNullOrWhiteSpace(_accountId))
            tokensNode["account_id"] = _accountId;

        jsonNode["tokens"] = tokensNode;
        jsonNode["last_refresh"] = DateTime.UtcNow.ToString("O");

        var updatedJson = jsonNode.ToJsonString(new JsonSerializerOptions { WriteIndented = true });
        await File.WriteAllTextAsync(credentialsPath, updatedJson);
    }

    private static string GetCredentialsPath()
    {
        var codexHome = Environment.GetEnvironmentVariable("CODEX_HOME");
        if (!string.IsNullOrWhiteSpace(codexHome))
            return Path.Combine(codexHome, "auth.json");

        return Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.UserProfile),
            ".codex",
            "auth.json"
        );
    }
}
