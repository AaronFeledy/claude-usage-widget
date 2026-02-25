using System.Text.Json;
using System.Text.Json.Nodes;

namespace ClaudeUsageWidget.Services;

/// <summary>
/// Manages OAuth credentials for Claude API access.
/// Reads from ~/.claude/.credentials.json and handles token refresh.
/// </summary>
public class CredentialService
{
    private const string TokenRefreshUrl = "https://platform.claude.com/v1/oauth/token";
    private const string OAuthClientId = "9d1c250a-e61b-44d9-88ed-5944d1962f5e";
    
    private readonly string _credentialsPath;
    private readonly HttpClient _httpClient;
    private readonly object _lock = new();

    private string? _accessToken;
    private string? _refreshToken;
    private DateTime _expiresAt;

    public CredentialService(HttpClient httpClient)
    {
        _httpClient = httpClient;
        _credentialsPath = Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.UserProfile),
            ".claude",
            ".credentials.json"
        );
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
            if (!File.Exists(_credentialsPath))
                throw new FileNotFoundException($"Credentials file not found: {_credentialsPath}");

            var json = File.ReadAllText(_credentialsPath);
            var doc = JsonDocument.Parse(json);

            if (!doc.RootElement.TryGetProperty("claudeAiOauth", out var oauthSection))
                throw new InvalidOperationException("claudeAiOauth section not found in credentials file");

            _accessToken = oauthSection.GetProperty("accessToken").GetString();
            _refreshToken = oauthSection.GetProperty("refreshToken").GetString();
            
            // expiresAt is in milliseconds since Unix epoch
            var expiresAtMs = oauthSection.GetProperty("expiresAt").GetInt64();
            _expiresAt = DateTimeOffset.FromUnixTimeMilliseconds(expiresAtMs).UtcDateTime;
        }
    }

    /// <summary>
    /// Refreshes the access token using the refresh token.
    /// </summary>
    public async Task<bool> RefreshTokenAsync()
    {
        string? refreshToken;
        lock (_lock)
        {
            if (_refreshToken == null)
                LoadCredentials();
            refreshToken = _refreshToken;
        }

        if (string.IsNullOrEmpty(refreshToken))
            return false;

        try
        {
            var requestBody = new Dictionary<string, string>
            {
                ["grant_type"] = "refresh_token",
                ["refresh_token"] = refreshToken,
                ["client_id"] = OAuthClientId
            };

            var content = new FormUrlEncodedContent(requestBody);
            var response = await _httpClient.PostAsync(TokenRefreshUrl, content);

            if (!response.IsSuccessStatusCode)
                return false;

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

            // Save back to credentials file
            await SaveCredentialsAsync(newAccessToken!, newRefreshToken!, newExpiresAt);

            return true;
        }
        catch
        {
            return false;
        }
    }

    /// <summary>
    /// Saves refreshed credentials back to the credentials file.
    /// </summary>
    private async Task SaveCredentialsAsync(string accessToken, string refreshToken, DateTime expiresAt)
    {
        if (!File.Exists(_credentialsPath))
            return;

        var json = await File.ReadAllTextAsync(_credentialsPath);
        var jsonNode = JsonNode.Parse(json);
        
        if (jsonNode == null)
            return;

        var oauthSection = jsonNode["claudeAiOauth"];
        if (oauthSection == null)
            return;

        oauthSection["accessToken"] = accessToken;
        oauthSection["refreshToken"] = refreshToken;
        oauthSection["expiresAt"] = new DateTimeOffset(expiresAt).ToUnixTimeMilliseconds();

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
            _expiresAt = DateTime.MinValue;
            LoadCredentials();
        }
    }
}
