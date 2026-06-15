using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;

namespace ClaudeUsageWidget.Services;

/// <summary>
/// Result of a Grok token refresh attempt.
/// </summary>
public enum GrokRefreshResult
{
    Success,
    InvalidGrant,  // Token revoked/expired — user must run `grok login`
    Failed         // Transient/network error
}

/// <summary>
/// Manages OAuth credentials for the Grok CLI (SuperGrok / X Premium+).
/// Reads from ~/.grok/auth.json (scoped entries) and handles token refresh.
/// </summary>
public class GrokCredentialService
{
    private const string TokenRefreshUrl = "https://auth.x.ai/oauth2/token";
    private const string OAuthClientId = "b1a00492-073a-47ea-816f-4c329264a828";
    private const string TokenAuthHeaderValue = "xai-grok-cli";
    private static readonly TimeSpan RefreshBuffer = TimeSpan.FromMinutes(5);

    private readonly HttpClient _httpClient;
    private readonly DebugService? _debugService;
    private readonly object _lock = new();

    private string? _credentialsPath;
    private string? _accessToken;
    private string? _refreshToken;
    private DateTime? _expiresAt;
    private string? _activeEntryKey;

    public GrokCredentialService(HttpClient httpClient, DebugService? debugService = null)
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

    public bool NeedsRefresh()
    {
        lock (_lock)
        {
            if (_accessToken == null)
                LoadCredentials();

            if (string.IsNullOrWhiteSpace(_refreshToken))
                return false;

            if (_expiresAt.HasValue)
                return DateTime.UtcNow >= _expiresAt.Value - RefreshBuffer;

            return false;
        }
    }

    public void LoadCredentials()
    {
        lock (_lock)
        {
            _credentialsPath = ResolveCredentialsPath();
            _debugService?.LogInfo("GrokAuth", $"Loading credentials from {_credentialsPath}");

            if (!File.Exists(_credentialsPath))
                throw new FileNotFoundException($"Grok auth.json not found: {_credentialsPath}");

            var json = File.ReadAllText(_credentialsPath);
            var doc = JsonDocument.Parse(json);
            var root = doc.RootElement;

            string? token = null;
            string? refresh = null;
            DateTime? expires = null;
            string? entryKey = null;

            foreach (var prop in root.EnumerateObject())
            {
                var key = prop.Name;
                var entry = prop.Value;
                if (!entry.TryGetProperty("key", out var keyEl)) continue;

                var t = keyEl.GetString();
                if (string.IsNullOrWhiteSpace(t)) continue;

                var r = entry.TryGetProperty("refresh_token", out var rtEl) ? rtEl.GetString() :
                        entry.TryGetProperty("refresh", out var rEl) ? rEl.GetString() : null;

                DateTime? exp = null;
                if (entry.TryGetProperty("expires_at", out var expEl))
                {
                    if (expEl.ValueKind == JsonValueKind.Number)
                        exp = DateTimeOffset.FromUnixTimeMilliseconds(expEl.GetInt64()).UtcDateTime;
                    else if (expEl.ValueKind == JsonValueKind.String && DateTime.TryParse(expEl.GetString(), out var dt))
                        exp = dt.ToUniversalTime();
                }

                // Prefer the modern OIDC scope entry
                if (token == null || key.Contains("auth.x.ai", StringComparison.OrdinalIgnoreCase))
                {
                    token = t.Trim();
                    refresh = r?.Trim();
                    expires = exp;
                    entryKey = key;
                }
            }

            if (string.IsNullOrWhiteSpace(token))
                throw new InvalidOperationException("Grok auth.json contains no usable access token (run `grok login`)");

            _accessToken = token;
            _refreshToken = refresh;
            _expiresAt = expires;
            _activeEntryKey = entryKey;

            _debugService?.LogInfo("GrokAuth", "Grok credentials loaded successfully");
        }
    }

    public async Task<GrokRefreshResult> RefreshTokenAsync()
    {
        string? refreshToken;
        string? entryKey;
        lock (_lock)
        {
            if (_accessToken == null)
                LoadCredentials();
            refreshToken = _refreshToken;
            entryKey = _activeEntryKey;
        }

        if (string.IsNullOrWhiteSpace(refreshToken))
        {
            _debugService?.LogError("GrokAuth", "No refresh token available for Grok");
            return GrokRefreshResult.InvalidGrant;
        }

        try
        {
            _debugService?.LogInfo("GrokAuth", "Refreshing Grok token");

            var form = new Dictionary<string, string>
            {
                ["grant_type"] = "refresh_token",
                ["refresh_token"] = refreshToken,
                ["client_id"] = OAuthClientId
            };

            var content = new FormUrlEncodedContent(form);
            var resp = await _httpClient.PostAsync(TokenRefreshUrl, content);
            var body = await resp.Content.ReadAsStringAsync();

            if (!resp.IsSuccessStatusCode)
            {
                _debugService?.LogError("GrokAuth", $"Grok token refresh failed: {resp.StatusCode}", body);
                if ((int)resp.StatusCode == 400 || (int)resp.StatusCode == 401)
                {
                    try
                    {
                        if (body.Contains("invalid_grant", StringComparison.OrdinalIgnoreCase))
                            return GrokRefreshResult.InvalidGrant;
                    }
                    catch { }
                }
                return GrokRefreshResult.Failed;
            }

            var doc = JsonDocument.Parse(body);
            var root = doc.RootElement;

            var newAccess = root.GetProperty("access_token").GetString();
            var newRefresh = root.TryGetProperty("refresh_token", out var nr) ? nr.GetString() : refreshToken;

            DateTime? newExpires = null;
            if (root.TryGetProperty("expires_in", out var ei))
            {
                var seconds = ei.GetInt32();
                newExpires = DateTime.UtcNow.AddSeconds(seconds);
            }
            else if (root.TryGetProperty("expires_at", out var ea))
            {
                // some responses may have it
                if (ea.ValueKind == JsonValueKind.Number)
                    newExpires = DateTimeOffset.FromUnixTimeMilliseconds(ea.GetInt64()).UtcDateTime;
            }

            lock (_lock)
            {
                _accessToken = newAccess;
                _refreshToken = newRefresh;
                _expiresAt = newExpires ?? _expiresAt;
            }

            await SaveCredentialsAsync(newAccess!, newRefresh!, _expiresAt);
            _debugService?.LogInfo("GrokAuth", "Grok token refreshed successfully");
            return GrokRefreshResult.Success;
        }
        catch (Exception ex)
        {
            _debugService?.LogError("GrokAuth", $"Grok token refresh exception: {ex.Message}", ex.ToString());
            return GrokRefreshResult.Failed;
        }
    }

    private async Task SaveCredentialsAsync(string accessToken, string? refreshToken, DateTime? expiresAt)
    {
        if (_credentialsPath == null || !File.Exists(_credentialsPath) || _activeEntryKey == null)
            return;

        var json = await File.ReadAllTextAsync(_credentialsPath);
        var node = JsonNode.Parse(json);
        if (node == null) return;

        var entry = node[_activeEntryKey] as JsonObject;
        if (entry == null) return;

        entry["key"] = accessToken;
        if (!string.IsNullOrWhiteSpace(refreshToken))
            entry["refresh_token"] = refreshToken;
        if (expiresAt.HasValue)
            entry["expires_at"] = new DateTimeOffset(expiresAt.Value).ToUnixTimeMilliseconds();

        var opts = new JsonSerializerOptions { WriteIndented = true };
        await File.WriteAllTextAsync(_credentialsPath, node.ToJsonString(opts));
    }

    public void ReloadCredentials()
    {
        lock (_lock)
        {
            _accessToken = null;
            _refreshToken = null;
            _expiresAt = null;
            _activeEntryKey = null;
            LoadCredentials();
        }
    }

    private string ResolveCredentialsPath()
    {
        var home = Environment.GetFolderPath(Environment.SpecialFolder.UserProfile);
        var windows = Path.Combine(home, ".grok", "auth.json");
        if (File.Exists(windows))
            return windows;

        var wsl = WslPathResolver.ResolveGrokAuthPath();
        if (!string.IsNullOrWhiteSpace(wsl) && File.Exists(wsl))
        {
            _debugService?.LogInfo("GrokAuth", $"Using WSL fallback Grok auth at {wsl}");
            return wsl;
        }

        return windows;
    }
}
