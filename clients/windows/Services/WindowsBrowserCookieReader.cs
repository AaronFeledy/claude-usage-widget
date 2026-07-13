using System.Security.Cryptography;
using System.Text;
using Microsoft.Data.Sqlite;

namespace ClaudeUsageWidget.Services;

public class WindowsBrowserCookieReader : ICursorCookieReader
{
    private static readonly string[] SessionCookieNames =
    {
        "WorkosCursorSessionToken",
        "__Secure-next-auth.session-token",
        "next-auth.session-token"
    };

    private readonly DebugService? _debugService;

    public WindowsBrowserCookieReader(DebugService? debugService = null)
    {
        _debugService = debugService;
    }

    public virtual string? ReadCursorCookieHeader()
    {
        foreach (var chromiumBrowser in GetChromiumBrowserRoots())
        {
            try
            {
                var cookieHeader = TryReadChromiumCookieHeader(chromiumBrowser);
                if (!string.IsNullOrWhiteSpace(cookieHeader))
                    return cookieHeader;
            }
            catch (Exception ex)
            {
                _debugService?.LogWarning("Cursor", $"Cookie import failed for {chromiumBrowser.name}", ex.GetType().Name);
            }
        }

        foreach (var firefoxProfile in GetFirefoxProfiles())
        {
            try
            {
                var cookieHeader = TryReadFirefoxCookieHeader(firefoxProfile);
                if (!string.IsNullOrWhiteSpace(cookieHeader))
                    return cookieHeader;
            }
            catch (Exception ex)
            {
                _debugService?.LogWarning("Cursor", "Cookie import failed for Firefox", ex.GetType().Name);
            }
        }

        return null;
    }

    private string? TryReadChromiumCookieHeader((string name, string userDataPath) browserRoot)
    {
        if (!Directory.Exists(browserRoot.userDataPath))
            return null;

        var masterKey = GetChromiumMasterKey(browserRoot.userDataPath);
        foreach (var cookiePath in EnumerateCookieDatabases(browserRoot.userDataPath))
        {
            var cookieHeader = TryReadCookiesFromDatabase(cookiePath, masterKey);
            if (!string.IsNullOrWhiteSpace(cookieHeader))
            {
                _debugService?.LogInfo("Cursor", $"Using browser cookies from {browserRoot.name}");
                return cookieHeader;
            }
        }

        return null;
    }

    private string? TryReadFirefoxCookieHeader(string profilePath)
    {
        var cookieDbPath = Path.Combine(profilePath, "cookies.sqlite");
        if (!File.Exists(cookieDbPath))
            return null;

        using var snapshot = BrowserCookieDatabaseSnapshot.Create(cookieDbPath);
        using var connection = OpenReadOnlyCookieDatabase(snapshot.DatabasePath);
        connection.Open();

        using var command = connection.CreateCommand();
        command.CommandText = @"
                SELECT host, name, value
                FROM moz_cookies
                WHERE (host LIKE '%cursor.com' OR host LIKE '%cursor.sh')
                  AND name IN ('WorkosCursorSessionToken', '__Secure-next-auth.session-token', 'next-auth.session-token')
                ORDER BY LENGTH(host) DESC, expiry DESC, lastAccessed DESC";

        using var reader = command.ExecuteReader();
        var cookies = new Dictionary<string, string>(StringComparer.Ordinal);

        while (reader.Read())
        {
            var name = reader.GetString(1);
            if (cookies.ContainsKey(name))
                continue;

            var value = reader.GetString(2);
            if (!string.IsNullOrWhiteSpace(value))
                cookies[name] = value;
        }

        if (cookies.Count == 0)
            return null;

        _debugService?.LogInfo("Cursor", "Using browser cookies from Firefox");
        return string.Join("; ", cookies.Select(x => $"{x.Key}={x.Value}"));
    }

    private string? TryReadCookiesFromDatabase(string cookieDbPath, byte[] masterKey)
    {
        using var snapshot = BrowserCookieDatabaseSnapshot.Create(cookieDbPath);
        using var connection = OpenReadOnlyCookieDatabase(snapshot.DatabasePath);
        connection.Open();

        using var command = connection.CreateCommand();
        command.CommandText = @"
                SELECT host_key, name, encrypted_value
                FROM cookies
                WHERE (host_key LIKE '%cursor.com' OR host_key LIKE '%cursor.sh')
                  AND name IN ('WorkosCursorSessionToken', '__Secure-next-auth.session-token', 'next-auth.session-token')
                ORDER BY LENGTH(host_key) DESC, expires_utc DESC";

        using var reader = command.ExecuteReader();
        var cookies = new Dictionary<string, string>(StringComparer.Ordinal);

        while (reader.Read())
        {
            var name = reader.GetString(1);
            if (cookies.ContainsKey(name))
                continue;

            var encryptedValue = (byte[])reader[2];
            var decryptedValue = DecryptCookieValue(encryptedValue, masterKey);
            if (!string.IsNullOrWhiteSpace(decryptedValue))
                cookies[name] = decryptedValue;
        }

        if (cookies.Count == 0)
            return null;

        return string.Join("; ", cookies.Select(x => $"{x.Key}={x.Value}"));
    }

    private static SqliteConnection OpenReadOnlyCookieDatabase(string path)
    {
        var builder = new SqliteConnectionStringBuilder
        {
            DataSource = path,
            Mode = SqliteOpenMode.ReadOnly
        };
        return new SqliteConnection(builder.ToString());
    }

    private static byte[] GetChromiumMasterKey(string userDataPath)
    {
        var localStatePath = Path.Combine(userDataPath, "Local State");
        if (!File.Exists(localStatePath))
            return Array.Empty<byte>();

        var json = File.ReadAllText(localStatePath);
        using var doc = System.Text.Json.JsonDocument.Parse(json);
        var encryptedKeyBase64 = doc.RootElement
            .GetProperty("os_crypt")
            .GetProperty("encrypted_key")
            .GetString();

        if (string.IsNullOrWhiteSpace(encryptedKeyBase64))
            return Array.Empty<byte>();

        var encryptedKey = Convert.FromBase64String(encryptedKeyBase64);
        var keyBytes = encryptedKey.AsSpan(5).ToArray();
        return ProtectedData.Unprotect(keyBytes, null, DataProtectionScope.CurrentUser);
    }

    private static string DecryptCookieValue(byte[] encryptedValue, byte[] masterKey)
    {
        if (encryptedValue.Length == 0)
            return string.Empty;

        var prefix = Encoding.ASCII.GetString(encryptedValue, 0, Math.Min(3, encryptedValue.Length));
        if ((prefix == "v10" || prefix == "v11") && masterKey.Length > 0)
        {
            var nonce = encryptedValue.AsSpan(3, 12).ToArray();
            var cipherTextLength = encryptedValue.Length - 3 - 12 - 16;
            var cipherText = encryptedValue.AsSpan(15, cipherTextLength).ToArray();
            var tag = encryptedValue.AsSpan(encryptedValue.Length - 16, 16).ToArray();
            var plainText = new byte[cipherText.Length];

            using var aesGcm = new AesGcm(masterKey, 16);
            aesGcm.Decrypt(nonce, cipherText, tag, plainText);
            return Encoding.UTF8.GetString(plainText);
        }

        var decrypted = ProtectedData.Unprotect(encryptedValue, null, DataProtectionScope.CurrentUser);
        return Encoding.UTF8.GetString(decrypted);
    }

    private static IEnumerable<string> EnumerateCookieDatabases(string userDataPath)
    {
        foreach (var profileDir in Directory.EnumerateDirectories(userDataPath))
        {
            var profileName = Path.GetFileName(profileDir);
            if (!profileName.StartsWith("Default", StringComparison.OrdinalIgnoreCase) &&
                !profileName.StartsWith("Profile ", StringComparison.OrdinalIgnoreCase) &&
                !profileName.StartsWith("Guest Profile", StringComparison.OrdinalIgnoreCase))
            {
                continue;
            }

            var networkCookies = Path.Combine(profileDir, "Network", "Cookies");
            if (File.Exists(networkCookies))
                yield return networkCookies;

            var directCookies = Path.Combine(profileDir, "Cookies");
            if (File.Exists(directCookies))
                yield return directCookies;
        }
    }

    private static IEnumerable<(string name, string userDataPath)> GetChromiumBrowserRoots()
    {
        var localAppData = Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData);

        yield return ("Chrome", Path.Combine(localAppData, "Google", "Chrome", "User Data"));
        yield return ("Edge", Path.Combine(localAppData, "Microsoft", "Edge", "User Data"));
        yield return ("Brave", Path.Combine(localAppData, "BraveSoftware", "Brave-Browser", "User Data"));
    }

    private static IEnumerable<string> GetFirefoxProfiles()
    {
        var roamingAppData = Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData);
        var profilesRoot = Path.Combine(roamingAppData, "Mozilla", "Firefox", "Profiles");
        if (!Directory.Exists(profilesRoot))
            yield break;

        foreach (var profileDir in Directory.EnumerateDirectories(profilesRoot))
            yield return profileDir;
    }

}
