using Microsoft.Data.Sqlite;
using ClaudeUsageWidget.Services;

internal sealed class BrowserCookieSnapshotTests
{
    public Task Test_SnapshotCopiesWalDatabaseAndCleansArtifacts()
    {
        using var fixture = CookieDbFixture.Create();
        fixture.InsertCookieWithoutCheckpoint("WorkosCursorSessionToken", "encrypted-value");

        string snapshotRoot;
        using (var snapshot = BrowserCookieDatabaseSnapshot.Create(fixture.DatabasePath))
        {
            snapshotRoot = snapshot.DirectoryPath;
            AssertEqual(true, File.Exists(snapshot.DatabasePath));
            using var connection = new SqliteConnection($"Data Source={snapshot.DatabasePath};Mode=ReadOnly");
            connection.Open();
            using var command = connection.CreateCommand();
            command.CommandText = "SELECT value FROM cookies WHERE name = 'WorkosCursorSessionToken'";
            AssertEqual("encrypted-value", (string?)command.ExecuteScalar());
        }

        AssertEqual(false, Directory.Exists(snapshotRoot));
        return Task.CompletedTask;
    }

    public Task Test_SnapshotCleansArtifactsWhenOpenFails()
    {
        using var fixture = CookieDbFixture.Create();
        fixture.InsertCookieWithoutCheckpoint("WorkosCursorSessionToken", "encrypted-value");
        var rootsBefore = SnapshotRoots();

        try
        {
            using var snapshot = BrowserCookieDatabaseSnapshot.Create(Path.Combine(fixture.TempPath, "missing.sqlite"));
        }
        catch (FileNotFoundException)
        {
        }

        AssertEqual(rootsBefore, SnapshotRoots());
        return Task.CompletedTask;
    }

    private static int SnapshotRoots() => Directory.GetDirectories(Path.GetTempPath(), "claude-usage-cookies-*").Length;

    private static void AssertEqual<T>(T expected, T actual)
    {
        if (!EqualityComparer<T>.Default.Equals(expected, actual))
        {
            throw new InvalidOperationException($"Expected {expected}, got {actual}");
        }
    }
}

internal sealed class CookieDbFixture : IDisposable
{
    private readonly SqliteConnection _writer;
    public string TempPath { get; }
    public string DatabasePath { get; }

    private CookieDbFixture(string tempPath, string databasePath, SqliteConnection writer)
    {
        TempPath = tempPath;
        DatabasePath = databasePath;
        _writer = writer;
    }

    public static CookieDbFixture Create()
    {
        var tempPath = Directory.CreateTempSubdirectory().FullName;
        var databasePath = Path.Combine(tempPath, "Cookies");
        var writer = new SqliteConnection($"Data Source={databasePath}");
        writer.Open();
        using var pragma = writer.CreateCommand();
        pragma.CommandText = "PRAGMA journal_mode=WAL";
        pragma.ExecuteNonQuery();
        using var create = writer.CreateCommand();
        create.CommandText = "CREATE TABLE cookies (host_key TEXT, name TEXT, value TEXT, encrypted_value BLOB, expires_utc INTEGER)";
        create.ExecuteNonQuery();
        return new CookieDbFixture(tempPath, databasePath, writer);
    }

    public void InsertCookieWithoutCheckpoint(string name, string value)
    {
        using var transaction = _writer.BeginTransaction();
        using var insert = _writer.CreateCommand();
        insert.Transaction = transaction;
        insert.CommandText = "INSERT INTO cookies(host_key, name, value, encrypted_value, expires_utc) VALUES('.cursor.com', $name, $value, X'', 1)";
        insert.Parameters.AddWithValue("$name", name);
        insert.Parameters.AddWithValue("$value", value);
        insert.ExecuteNonQuery();
        transaction.Commit();
    }

    public void Dispose()
    {
        _writer.Dispose();
        Directory.Delete(TempPath, recursive: true);
    }
}
