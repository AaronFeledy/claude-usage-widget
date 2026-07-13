using System.Diagnostics;

namespace ClaudeUsageWidget.Services;

public interface IPendingUpdateServerStopper
{
    Task StopOwnedServerAsync(CancellationToken cancellationToken);
}

public interface IUpdateProcessStarter
{
    void Start(string executablePath);
}

public interface IUpdateFileSystem
{
    bool Exists(string path);
    void DeleteIfExists(string path);
    void Move(string source, string destination, bool overwrite);
}

public sealed record PendingUpdateOptions(
    string AppPath,
    string ServerPath,
    IPendingUpdateServerStopper ServerStopper,
    IUpdateProcessStarter ProcessStarter,
    IUpdateFileSystem FileSystem);

public static class PendingUpdateApplicator
{
    public const string AppUpdateFileName = "ClaudeUsageWidget.update.exe";
    public const string AppOldFileName = "ClaudeUsageWidget.old.exe";
    public const string ServerUpdateFileName = "usage-server.update.exe";
    public const string ServerOldFileName = "usage-server.old.exe";

    public static async Task<bool> ApplyAsync(PendingUpdateOptions options, CancellationToken cancellationToken)
    {
        var paths = PendingUpdatePaths.From(options);
        var files = options.FileSystem;
        files.DeleteIfExists(paths.AppOldPath);
        files.DeleteIfExists(paths.ServerOldPath);
        if (!files.Exists(paths.AppUpdatePath))
        {
            return false;
        }

        var hasServerUpdate = files.Exists(paths.ServerUpdatePath);
        try
        {
            if (hasServerUpdate)
            {
                await options.ServerStopper.StopOwnedServerAsync(cancellationToken).ConfigureAwait(false);
            }
            files.Move(options.AppPath, paths.AppOldPath, overwrite: true);
            files.Move(paths.AppUpdatePath, options.AppPath, overwrite: false);
            if (hasServerUpdate)
            {
                if (files.Exists(options.ServerPath))
                {
                    files.Move(options.ServerPath, paths.ServerOldPath, overwrite: true);
                }
                files.Move(paths.ServerUpdatePath, options.ServerPath, overwrite: true);
            }
            options.ProcessStarter.Start(options.AppPath);
            return true;
        }
        catch
        {
            RollBack(options, paths, hasServerUpdate);
            return false;
        }
    }

    public static bool ApplyFromCurrentProcess() => ApplyFromCurrentProcess(new NoOpPendingUpdateServerStopper());

    public static bool ApplyFromCurrentProcess(IPendingUpdateServerStopper serverStopper)
    {
        var currentExePath = Environment.ProcessPath;
        var directory = string.IsNullOrWhiteSpace(currentExePath) ? null : Path.GetDirectoryName(currentExePath);
        if (string.IsNullOrWhiteSpace(currentExePath) || string.IsNullOrWhiteSpace(directory))
        {
            return false;
        }
        var options = new PendingUpdateOptions(
            currentExePath,
            Path.Combine(directory, "usage-server.exe"),
            serverStopper,
            new ShellUpdateProcessStarter(),
            new RealUpdateFileSystem());
        return ApplyAsync(options, CancellationToken.None).GetAwaiter().GetResult();
    }

    private static void RollBack(PendingUpdateOptions options, PendingUpdatePaths paths, bool hasServerUpdate)
    {
        var files = options.FileSystem;
        TryMove(files, options.AppPath, paths.AppUpdatePath, overwrite: true);
        TryMove(files, paths.AppOldPath, options.AppPath, overwrite: true);
        if (hasServerUpdate)
        {
            TryMove(files, options.ServerPath, paths.ServerUpdatePath, overwrite: true);
            TryMove(files, paths.ServerOldPath, options.ServerPath, overwrite: true);
        }
    }

    private static void TryMove(IUpdateFileSystem files, string source, string destination, bool overwrite)
    {
        try
        {
            if (files.Exists(source))
            {
                files.Move(source, destination, overwrite);
            }
        }
        catch
        {
        }
    }
}

internal sealed record PendingUpdatePaths(string AppUpdatePath, string AppOldPath, string ServerUpdatePath, string ServerOldPath)
{
    public static PendingUpdatePaths From(PendingUpdateOptions options)
    {
        var directory = Path.GetDirectoryName(options.AppPath) ?? AppContext.BaseDirectory;
        return new PendingUpdatePaths(
            Path.Combine(directory, PendingUpdateApplicator.AppUpdateFileName),
            Path.Combine(directory, PendingUpdateApplicator.AppOldFileName),
            Path.Combine(directory, PendingUpdateApplicator.ServerUpdateFileName),
            Path.Combine(directory, PendingUpdateApplicator.ServerOldFileName));
    }
}

internal sealed class NoOpPendingUpdateServerStopper : IPendingUpdateServerStopper
{
    public Task StopOwnedServerAsync(CancellationToken cancellationToken) => Task.CompletedTask;
}

internal sealed class ShellUpdateProcessStarter : IUpdateProcessStarter
{
    public void Start(string executablePath)
    {
        Process.Start(new ProcessStartInfo { FileName = executablePath, UseShellExecute = true });
    }
}

internal sealed class RealUpdateFileSystem : IUpdateFileSystem
{
    public bool Exists(string path) => File.Exists(path);
    public void DeleteIfExists(string path)
    {
        try { File.Delete(path); } catch { }
    }
    public void Move(string source, string destination, bool overwrite) => File.Move(source, destination, overwrite);
}
