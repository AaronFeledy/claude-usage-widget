namespace ClaudeUsageWidget.Services;

public sealed class BrowserCookieDatabaseSnapshot : IDisposable
{
    private const string SnapshotPrefix = "claude-usage-cookies-";
    private static readonly object ActiveLock = new();
    private static readonly HashSet<string> ActiveDirectories = new(StringComparer.OrdinalIgnoreCase);
    private bool _disposed;

    static BrowserCookieDatabaseSnapshot()
    {
        AppDomain.CurrentDomain.ProcessExit += (_, _) => CleanupActiveDirectories();
    }

    private BrowserCookieDatabaseSnapshot(string directoryPath, string databasePath)
    {
        DirectoryPath = directoryPath;
        DatabasePath = databasePath;
    }

    public string DirectoryPath { get; }
    public string DatabasePath { get; }

    public static BrowserCookieDatabaseSnapshot Create(string sourceDatabasePath, string? temporaryRoot = null)
    {
        if (!File.Exists(sourceDatabasePath))
        {
            throw new FileNotFoundException("Browser cookie database not found.", sourceDatabasePath);
        }
        var directoryPath = temporaryRoot == null
            ? Directory.CreateTempSubdirectory(SnapshotPrefix).FullName
            : CreateSnapshotDirectory(temporaryRoot);
        Track(directoryPath);
        try
        {
            var targetDatabasePath = Path.Combine(directoryPath, Path.GetFileName(sourceDatabasePath));
            CopyIfExists(sourceDatabasePath, targetDatabasePath);
            CopyIfExists(sourceDatabasePath + "-wal", targetDatabasePath + "-wal");
            CopyIfExists(sourceDatabasePath + "-shm", targetDatabasePath + "-shm");
            return new BrowserCookieDatabaseSnapshot(directoryPath, targetDatabasePath);
        }
        catch
        {
            if (DeleteDirectory(directoryPath)) Untrack(directoryPath);
            throw;
        }
    }

    private static string CreateSnapshotDirectory(string temporaryRoot)
    {
        Directory.CreateDirectory(temporaryRoot);
        for (var attempt = 0; attempt < 10; attempt++)
        {
            var path = Path.Combine(temporaryRoot, SnapshotPrefix + Guid.NewGuid().ToString("N"));
            try
            {
                Directory.CreateDirectory(path);
                return path;
            }
            catch (IOException) when (Directory.Exists(path))
            {
            }
        }
        throw new IOException("Could not create a browser cookie snapshot directory.");
    }

    public void Dispose()
    {
        if (_disposed)
        {
            return;
        }
        _disposed = true;
        if (DeleteDirectory(DirectoryPath)) Untrack(DirectoryPath);
    }

    private static void Track(string directoryPath)
    {
        lock (ActiveLock)
        {
            ActiveDirectories.Add(directoryPath);
        }
    }

    private static void Untrack(string directoryPath)
    {
        lock (ActiveLock)
        {
            ActiveDirectories.Remove(directoryPath);
        }
    }

    private static void CleanupActiveDirectories()
    {
        string[] directories;
        lock (ActiveLock)
        {
            directories = ActiveDirectories.ToArray();
            ActiveDirectories.Clear();
        }
        foreach (var directory in directories)
        {
            DeleteDirectory(directory);
        }
    }

    private static void CopyIfExists(string sourcePath, string targetPath)
    {
        if (!File.Exists(sourcePath))
        {
            return;
        }
        using var source = new FileStream(sourcePath, FileMode.Open, FileAccess.Read, FileShare.ReadWrite | FileShare.Delete);
        using var target = new FileStream(targetPath, FileMode.CreateNew, FileAccess.Write, FileShare.None);
        source.CopyTo(target);
        target.Flush(flushToDisk: true);
    }

    private static bool DeleteDirectory(string directoryPath)
    {
        try
        {
            if (Directory.Exists(directoryPath))
            {
                Directory.Delete(directoryPath, recursive: true);
            }
            return !Directory.Exists(directoryPath);
        }
        catch
        {
            return false;
        }
    }
}
