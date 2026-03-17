using System.Text.Json;
using System.Text.Json.Nodes;

namespace ClaudeUsageWidget.Services;

/// <summary>
/// Result of a token refresh attempt.
/// </summary>
public enum RefreshResult
{
    Success,
    InvalidGrant,  // Token revoked or expired — user must re-authenticate
    Failed         // Transient/network error
}

/// <summary>
/// Manages OAuth credentials for Claude API access.
/// Reads from ~/.claude/.credentials.json and handles token refresh.
/// </summary>
public class CredentialService
{
    private const string TokenRefreshUrl = "https://platform.claude.com/v1/oauth/token";
    private const string OAuthClientId = "9d1c250a-e61b-44d9-88ed-5944d1962f5e";
    
    private readonly HttpClient _httpClient;
    private readonly DebugService? _debugService;
    private readonly object _lock = new();

    private string? _credentialsPath;
    private ClaudeCredentialSource _credentialSource;
    private string? _accessToken;
    private string? _refreshToken;
    private string? _subscriptionType;
    private DateTime _expiresAt;

    public CredentialService(HttpClient httpClient, DebugService? debugService = null)
    {
        _httpClient = httpClient;
        _debugService = debugService;
    }

    /// <summary>
    /// Gets the current access token, loading from file if needed.
    /// </summary>
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

    public string? SubscriptionType
    {
        get
        {
            lock (_lock)
            {
                if (_accessToken == null)
                    LoadCredentials();
                return _subscriptionType;
            }
        }
    }

    /// <summary>
    /// Checks if the current token is expired or about to expire (within 60 seconds).
    /// </summary>
    public bool IsTokenExpired()
    {
        lock (_lock)
        {
            if (_accessToken == null)
                LoadCredentials();
            
            // Consider expired if within 60 seconds of expiration
            return DateTime.UtcNow >= _expiresAt.AddSeconds(-60);
        }
    }

    /// <summary>
    /// Loads credentials from the credentials file.
    /// </summary>
    public void LoadCredentials()
    {
        lock (_lock)
        {
            _credentialsPath = ResolveCredentialsPath();

            _debugService?.LogInfo("Credentials", $"Loading credentials from {_credentialsPath}");
            
            if (!File.Exists(_credentialsPath))
            {
                _debugService?.LogError("Credentials", $"Credentials file not found: {_credentialsPath}");
                throw new FileNotFoundException($"Credentials file not found: {_credentialsPath}");
            }

            var json = File.ReadAllText(_credentialsPath);
            var doc = JsonDocument.Parse(json);

            switch (_credentialSource)
            {
                case ClaudeCredentialSource.ClaudeCredentials:
                    LoadClaudeCredentials(doc.RootElement);
                    break;
                case ClaudeCredentialSource.OpenCodeAuth:
                    LoadOpenCodeCredentials(doc.RootElement);
                    break;
                default:
                    throw new InvalidOperationException("Unknown Claude credential source");
            }
            
            var expiresIn = _expiresAt - DateTime.UtcNow;
            _debugService?.LogInfo("Credentials", "Credentials loaded successfully", 
                $"Token expires in {expiresIn.TotalMinutes:F0} minutes");
        }
    }

    /// <summary>
    /// Refreshes the access token using the refresh token.
    /// </summary>
    public async Task<RefreshResult> RefreshTokenAsync()
    {
        string? refreshToken;
        lock (_lock)
        {
            if (_refreshToken == null)
                LoadCredentials();
            refreshToken = _refreshToken;
        }

        if (string.IsNullOrEmpty(refreshToken))
        {
            _debugService?.LogError("Credentials", "No refresh token available");
            return RefreshResult.InvalidGrant;
        }

        try
        {
            _debugService?.LogInfo("Credentials", $"Refreshing token via {TokenRefreshUrl}");
            
            var requestBody = new Dictionary<string, string>
            {
                ["grant_type"] = "refresh_token",
                ["refresh_token"] = refreshToken,
                ["client_id"] = OAuthClientId
            };

            var content = new FormUrlEncodedContent(requestBody);
            var response = await _httpClient.PostAsync(TokenRefreshUrl, content);

            if (!response.IsSuccessStatusCode)
            {
                // Check for invalid_grant (revoked/expired refresh token)
                try
                {
                    var errorJson = await response.Content.ReadAsStringAsync();
                    _debugService?.LogError("Credentials", $"Token refresh failed: {response.StatusCode}", errorJson);
                    if (errorJson.Contains("invalid_grant"))
                        return RefreshResult.InvalidGrant;
                }
                catch { }
                return RefreshResult.Failed;
            }

            var responseJson = await response.Content.ReadAsStringAsync();
            var responseDoc = JsonDocument.Parse(responseJson);
            var root = responseDoc.RootElement;

            var newAccessToken = root.GetProperty("access_token").GetString();
            var newRefreshToken = root.TryGetProperty("refresh_token", out var rt) 
                ? rt.GetString() 
                : refreshToken;
            var expiresIn = root.GetProperty("expires_in").GetInt32();
            var newExpiresAt = DateTime.UtcNow.AddSeconds(expiresIn);

            lock (_lock)
            {
                _accessToken = newAccessToken;
                _refreshToken = newRefreshToken;
                _expiresAt = newExpiresAt;
            }

            _debugService?.LogInfo("Credentials", "Token refreshed successfully", 
                $"New token expires in {expiresIn} seconds");

            // Save back to credentials file
            await SaveCredentialsAsync(newAccessToken!, newRefreshToken!, newExpiresAt);

            return RefreshResult.Success;
        }
        catch (Exception ex)
        {
            _debugService?.LogError("Credentials", $"Token refresh exception: {ex.Message}", ex.ToString());
            return RefreshResult.Failed;
        }
    }

    /// <summary>
    /// Saves refreshed credentials back to the credentials file.
    /// </summary>
    private async Task SaveCredentialsAsync(string accessToken, string refreshToken, DateTime expiresAt)
    {
        _credentialsPath ??= ResolveCredentialsPath();
        if (!File.Exists(_credentialsPath))
            return;

        var json = await File.ReadAllTextAsync(_credentialsPath);
        var jsonNode = JsonNode.Parse(json);
        
        if (jsonNode == null)
            return;

        switch (_credentialSource)
        {
            case ClaudeCredentialSource.ClaudeCredentials:
            {
                var oauthSection = jsonNode["claudeAiOauth"];
                if (oauthSection == null)
                    return;

                oauthSection["accessToken"] = accessToken;
                oauthSection["refreshToken"] = refreshToken;
                oauthSection["expiresAt"] = new DateTimeOffset(expiresAt).ToUnixTimeMilliseconds();
                break;
            }
            case ClaudeCredentialSource.OpenCodeAuth:
            {
                var oauthSection = jsonNode["anthropic"];
                if (oauthSection == null)
                    return;

                oauthSection["access"] = accessToken;
                oauthSection["refresh"] = refreshToken;
                oauthSection["expires"] = new DateTimeOffset(expiresAt).ToUnixTimeMilliseconds();
                break;
            }
            default:
                return;
        }

        var options = new JsonSerializerOptions { WriteIndented = true };
        var updatedJson = jsonNode.ToJsonString(options);
        
        await File.WriteAllTextAsync(_credentialsPath, updatedJson);
    }

    /// <summary>
    /// Forces a reload of credentials from the file.
    /// </summary>
    public void ReloadCredentials()
    {
        lock (_lock)
        {
            _accessToken = null;
            _refreshToken = null;
            _subscriptionType = null;
            _expiresAt = DateTime.MinValue;
            LoadCredentials();
        }
    }

    private void LoadClaudeCredentials(JsonElement root)
    {
        if (!root.TryGetProperty("claudeAiOauth", out var oauthSection))
        {
            _debugService?.LogError("Credentials", "claudeAiOauth section not found in credentials file");
            throw new InvalidOperationException("claudeAiOauth section not found in credentials file");
        }

        _accessToken = oauthSection.GetProperty("accessToken").GetString();
        _refreshToken = oauthSection.GetProperty("refreshToken").GetString();
        _subscriptionType = oauthSection.TryGetProperty("subscriptionType", out var subscriptionType)
            ? NormalizeSubscriptionType(subscriptionType.GetString())
            : TryInferSubscriptionType(oauthSection);
        var expiresAtMs = oauthSection.GetProperty("expiresAt").GetInt64();
        _expiresAt = DateTimeOffset.FromUnixTimeMilliseconds(expiresAtMs).UtcDateTime;
    }

    private void LoadOpenCodeCredentials(JsonElement root)
    {
        if (!root.TryGetProperty("anthropic", out var anthropicSection))
        {
            _debugService?.LogError("Credentials", "anthropic section not found in OpenCode auth file");
            throw new InvalidOperationException("anthropic section not found in OpenCode auth file");
        }

        _accessToken = anthropicSection.GetProperty("access").GetString();
        _refreshToken = anthropicSection.GetProperty("refresh").GetString();
        _subscriptionType = null;
        var expiresAtMs = anthropicSection.GetProperty("expires").GetInt64();
        _expiresAt = DateTimeOffset.FromUnixTimeMilliseconds(expiresAtMs).UtcDateTime;
    }

    private static string? TryInferSubscriptionType(JsonElement oauthSection)
    {
        if (!oauthSection.TryGetProperty("rateLimitTier", out var rateLimitTier))
            return null;

        var tier = rateLimitTier.GetString();
        if (string.IsNullOrWhiteSpace(tier))
            return null;

        if (tier.Contains("max", StringComparison.OrdinalIgnoreCase))
            return "Max";

        if (tier.Contains("pro", StringComparison.OrdinalIgnoreCase))
            return "Pro";

        if (tier.Contains("free", StringComparison.OrdinalIgnoreCase))
            return "Free";

        return null;
    }

    private static string? NormalizeSubscriptionType(string? subscriptionType)
    {
        if (string.IsNullOrWhiteSpace(subscriptionType))
            return null;

        return subscriptionType.Trim().ToLowerInvariant() switch
        {
            "max" => "Max",
            "pro" => "Pro",
            "free" => "Free",
            var other => char.ToUpperInvariant(other[0]) + other[1..]
        };
    }

    private string ResolveCredentialsPath()
    {
        var windowsPath = Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.UserProfile),
            ".claude",
            ".credentials.json"
        );

        if (File.Exists(windowsPath))
        {
            _credentialSource = ClaudeCredentialSource.ClaudeCredentials;
            return windowsPath;
        }

        var wslPath = WslPathResolver.ResolveHomeRelativePath(".claude", ".credentials.json");
        if (!string.IsNullOrWhiteSpace(wslPath) && File.Exists(wslPath))
        {
            _credentialSource = ClaudeCredentialSource.ClaudeCredentials;
            _debugService?.LogInfo("Credentials", $"Using WSL fallback credentials at {wslPath}");
            return wslPath;
        }

        var openCodePath = WslPathResolver.ResolveOpenCodeAuthPath();
        if (!string.IsNullOrWhiteSpace(openCodePath) && File.Exists(openCodePath))
        {
            _credentialSource = ClaudeCredentialSource.OpenCodeAuth;
            _debugService?.LogInfo("Credentials", $"Using OpenCode fallback auth at {openCodePath}");
            return openCodePath;
        }

        _credentialSource = ClaudeCredentialSource.ClaudeCredentials;
        return windowsPath;
    }

    private enum ClaudeCredentialSource
    {
        ClaudeCredentials,
        OpenCodeAuth
    }
}
