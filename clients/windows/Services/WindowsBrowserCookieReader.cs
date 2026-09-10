using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using Microsoft.Data.Sqlite;

namespace ClaudeUsageWidget.Services;

public sealed record BrowserCookieRoot(string Name, string UserDataPath);

public sealed class WindowsBrowserCookieReaderOptions
{
    public required IReadOnlyList<BrowserCookieRoot> ChromiumRoots { get; init; }
    public required Func<IReadOnlyList<string>> FirefoxProfiles { get; init; }
    public required Func<DateTimeOffset> UtcNow { get; init; }
    public required Func<byte[], byte[]> Unprotect { get; init; }
    public string? TemporaryRoot { get; init; }

    public static WindowsBrowserCookieReaderOptions ForCurrentUser(string? temporaryRoot = null)
    {
        var local = Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData);
        var roaming = Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData);
        var firefoxRoot = Path.Combine(roaming, "Mozilla", "Firefox", "Profiles");
        return new WindowsBrowserCookieReaderOptions
        {
            ChromiumRoots =
            [
                new("Chrome", Path.Combine(local, "Google", "Chrome", "User Data")),
                new("Edge", Path.Combine(local, "Microsoft", "Edge", "User Data")),
                new("Brave", Path.Combine(local, "BraveSoftware", "Brave-Browser", "User Data"))
            ],
            FirefoxProfiles = () => Directory.Exists(firefoxRoot)
                ? Directory.EnumerateDirectories(firefoxRoot).OrderBy(Path.GetFileName, StringComparer.OrdinalIgnoreCase).ToArray()
                : [],
            UtcNow = () => DateTimeOffset.UtcNow,
            Unprotect = DefaultUnprotect,
            TemporaryRoot = temporaryRoot
        };
    }

    private static byte[] DefaultUnprotect(byte[] bytes)
    {
#if WINDOWS
        return ProtectedData.Unprotect(bytes, null, DataProtectionScope.CurrentUser);
#else
        throw new PlatformNotSupportedException("Windows browser decryption requires Windows.");
#endif
    }
}

public class WindowsBrowserCookieReader : IProviderCookieReader
{
    private static readonly BrowserCookieQuery CursorCookies = new(
        "Cursor",
        "(host_key = 'cursor.com' OR host_key = 'cursor.sh' OR host_key LIKE '%.cursor.com' OR host_key LIKE '%.cursor.sh') " +
        "AND name IN ('WorkosCursorSessionToken', '__Secure-next-auth.session-token', 'next-auth.session-token') " +
        "AND (expires_utc = 0 OR expires_utc > $now)",
        "(host = 'cursor.com' OR host = 'cursor.sh' OR host LIKE '%.cursor.com' OR host LIKE '%.cursor.sh') " +
        "AND name IN ('WorkosCursorSessionToken', '__Secure-next-auth.session-token', 'next-auth.session-token') " +
        "AND (expiry = 0 OR expiry > $now)");

    private static readonly BrowserCookieQuery GrokCookies = new(
        "Grok",
        "(host_key = 'grok.com' OR host_key LIKE '%.grok.com') AND name = 'sso' AND (expires_utc = 0 OR expires_utc > $now)",
        "(host = 'grok.com' OR host LIKE '%.grok.com') AND name = 'sso' AND (expiry = 0 OR expiry > $now)");

    private readonly DebugService? _debugService;
    private readonly WindowsBrowserCookieReaderOptions _options;

    public WindowsBrowserCookieReader(DebugService? debugService = null)
        : this(WindowsBrowserCookieReaderOptions.ForCurrentUser(), debugService)
    {
    }

    public WindowsBrowserCookieReader(WindowsBrowserCookieReaderOptions options, DebugService? debugService = null)
    {
        _options = options ?? throw new ArgumentNullException(nameof(options));
        _debugService = debugService;
    }

    public virtual string? ReadCursorCookieHeader() => ReadCookieHeader(CursorCookies);
    public virtual string? ReadGrokCookieHeader() => ReadCookieHeader(GrokCookies);

    private string? ReadCookieHeader(BrowserCookieQuery query)
    {
        foreach (var root in _options.ChromiumRoots)
        {
            try
            {
                var cookieHeader = TryReadChromiumCookieHeader(root, query);
                if (!string.IsNullOrWhiteSpace(cookieHeader)) return cookieHeader;
            }
            catch (Exception ex)
            {
                _debugService?.LogWarning(query.Provider, $"Cookie import failed for {root.Name}", ex.GetType().Name);
            }
        }

        IReadOnlyList<string> firefoxProfiles;
        try
        {
            firefoxProfiles = _options.FirefoxProfiles();
        }
        catch (Exception ex)
        {
            _debugService?.LogWarning(query.Provider, "Cookie import failed for Firefox", ex.GetType().Name);
            return null;
        }
        foreach (var profile in firefoxProfiles.OrderBy(Path.GetFileName, StringComparer.OrdinalIgnoreCase))
        {
            try
            {
                var cookieHeader = TryReadFirefoxCookieHeader(profile, query);
                if (!string.IsNullOrWhiteSpace(cookieHeader)) return cookieHeader;
            }
            catch (Exception ex)
            {
                _debugService?.LogWarning(query.Provider, "Cookie import failed for Firefox", ex.GetType().Name);
            }
        }
        return null;
    }

    private string? TryReadChromiumCookieHeader(BrowserCookieRoot root, BrowserCookieQuery query)
    {
        if (!Directory.Exists(root.UserDataPath)) return null;
        byte[] masterKey;
        try
        {
            masterKey = GetChromiumMasterKey(root.UserDataPath);
        }
        catch
        {
            masterKey = [];
        }
        foreach (var cookiePath in EnumerateCookieDatabases(root.UserDataPath))
        {
            try
            {
                var cookieHeader = TryReadCookiesFromDatabase(cookiePath, masterKey, query);
                if (!string.IsNullOrWhiteSpace(cookieHeader))
                {
                    _debugService?.LogInfo(query.Provider, $"Using browser cookies from {root.Name}");
                    return cookieHeader;
                }
            }
            catch (Exception ex)
            {
                _debugService?.LogWarning(query.Provider, $"Cookie import failed for {root.Name}", ex.GetType().Name);
            }
        }
        return null;
    }

    private string? TryReadFirefoxCookieHeader(string profilePath, BrowserCookieQuery query)
    {
        var cookieDbPath = Path.Combine(profilePath, "cookies.sqlite");
        if (!File.Exists(cookieDbPath)) return null;
        using var snapshot = BrowserCookieDatabaseSnapshot.Create(cookieDbPath, _options.TemporaryRoot);
        using var connection = OpenReadOnlyCookieDatabase(snapshot.DatabasePath);
        connection.Open();
        using var command = connection.CreateCommand();
        command.CommandText = $"SELECT host, name, value FROM moz_cookies WHERE {query.FirefoxWhere} " +
            "ORDER BY LENGTH(host) DESC, expiry DESC, lastAccessed DESC";
        command.Parameters.AddWithValue("$now", _options.UtcNow().ToUnixTimeSeconds());
        using var reader = command.ExecuteReader();
        var cookies = new Dictionary<string, string>(StringComparer.Ordinal);
        while (reader.Read())
        {
            var name = reader.GetString(1);
            if (cookies.ContainsKey(name)) continue;
            var value = reader.GetString(2);
            if (!string.IsNullOrWhiteSpace(value)) cookies[name] = value;
        }
        if (cookies.Count == 0) return null;
        _debugService?.LogInfo(query.Provider, "Using browser cookies from Firefox");
        return string.Join("; ", cookies.Select(x => $"{x.Key}={x.Value}"));
    }

    private string? TryReadCookiesFromDatabase(string cookieDbPath, byte[] masterKey, BrowserCookieQuery query)
    {
        using var snapshot = BrowserCookieDatabaseSnapshot.Create(cookieDbPath, _options.TemporaryRoot);
        using var connection = OpenReadOnlyCookieDatabase(snapshot.DatabasePath);
        connection.Open();
        using var command = connection.CreateCommand();
        command.CommandText = $"SELECT host_key, name, encrypted_value FROM cookies WHERE {query.ChromiumWhere} " +
            "ORDER BY LENGTH(host_key) DESC, expires_utc DESC";
        command.Parameters.AddWithValue("$now", ChromiumTimestamp(_options.UtcNow()));
        using var reader = command.ExecuteReader();
        var cookies = new Dictionary<string, string>(StringComparer.Ordinal);
        while (reader.Read())
        {
            var name = reader.GetString(1);
            if (cookies.ContainsKey(name)) continue;
            var encryptedValue = (byte[])reader[2];
            var decryptedValue = DecryptCookieValue(encryptedValue, masterKey);
            if (!string.IsNullOrWhiteSpace(decryptedValue)) cookies[name] = decryptedValue;
        }
        return cookies.Count == 0 ? null : string.Join("; ", cookies.Select(x => $"{x.Key}={x.Value}"));
    }

    private static SqliteConnection OpenReadOnlyCookieDatabase(string path)
    {
        var builder = new SqliteConnectionStringBuilder { DataSource = path, Mode = SqliteOpenMode.ReadOnly, Pooling = false };
        return new SqliteConnection(builder.ToString());
    }

    private byte[] GetChromiumMasterKey(string userDataPath)
    {
        var localStatePath = Path.Combine(userDataPath, "Local State");
        if (!File.Exists(localStatePath)) return [];
        using var doc = JsonDocument.Parse(File.ReadAllText(localStatePath));
        var encoded = doc.RootElement.GetProperty("os_crypt").GetProperty("encrypted_key").GetString();
        if (string.IsNullOrWhiteSpace(encoded)) return [];
        var encryptedKey = Convert.FromBase64String(encoded);
        if (encryptedKey.Length <= 5 || !encryptedKey.AsSpan(0, 5).SequenceEqual("DPAPI"u8)) return [];
        return _options.Unprotect(encryptedKey.AsSpan(5).ToArray());
    }

    private string DecryptCookieValue(byte[] encryptedValue, byte[] masterKey)
    {
        if (encryptedValue.Length == 0) return string.Empty;
        var prefix = encryptedValue.Length >= 3 ? Encoding.ASCII.GetString(encryptedValue, 0, 3) : string.Empty;
        if (prefix == "v20") return string.Empty;
        if (prefix is "v10" or "v11")
        {
            if (encryptedValue.Length < 31 || masterKey.Length is not (16 or 24 or 32)) return string.Empty;
            try
            {
                var nonce = encryptedValue.AsSpan(3, 12);
                var cipherText = encryptedValue.AsSpan(15, encryptedValue.Length - 31);
                var tag = encryptedValue.AsSpan(encryptedValue.Length - 16, 16);
                var plainText = new byte[cipherText.Length];
                using var aesGcm = new AesGcm(masterKey, 16);
                aesGcm.Decrypt(nonce, cipherText, tag, plainText);
                return Encoding.UTF8.GetString(plainText);
            }
            catch (CryptographicException)
            {
                return string.Empty;
            }
        }
        try
        {
            return Encoding.UTF8.GetString(_options.Unprotect(encryptedValue));
        }
        catch (CryptographicException)
        {
            return string.Empty;
        }
    }

    private static IReadOnlyList<string> EnumerateCookieDatabases(string userDataPath) =>
        Directory.EnumerateDirectories(userDataPath)
            .Where(path => IsSupportedProfile(Path.GetFileName(path)))
            .OrderBy(path => ProfileRank(Path.GetFileName(path)))
            .ThenBy(path => Path.GetFileName(path), StringComparer.OrdinalIgnoreCase)
            .SelectMany(path => new[] { Path.Combine(path, "Network", "Cookies"), Path.Combine(path, "Cookies") })
            .Where(File.Exists)
            .ToArray();

    private static bool IsSupportedProfile(string name) =>
        name.Equals("Default", StringComparison.OrdinalIgnoreCase) ||
        name.StartsWith("Profile ", StringComparison.OrdinalIgnoreCase) ||
        name.StartsWith("Guest Profile", StringComparison.OrdinalIgnoreCase);

    private static int ProfileRank(string name) => name.Equals("Default", StringComparison.OrdinalIgnoreCase) ? 0
        : name.StartsWith("Profile ", StringComparison.OrdinalIgnoreCase) ? 1 : 2;

    private static long ChromiumTimestamp(DateTimeOffset time) =>
        checked((time.ToUnixTimeSeconds() + 11_644_473_600L) * 1_000_000L);

    private sealed record BrowserCookieQuery(string Provider, string ChromiumWhere, string FirefoxWhere);
}
