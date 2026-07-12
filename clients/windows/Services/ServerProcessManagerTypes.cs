using System.Diagnostics;

namespace ClaudeUsageWidget.Services;

public enum ServerProcessState
{
    Remote,
    Attached,
    Started,
    Failed
}

public enum ServerProcessError
{
    None,
    BinaryUnavailable,
    JobAssignmentFailed,
    ReadinessTimedOut,
    ReadinessFailed,
    RestartFailed,
    LaunchFailed,
    Disposed,
    InvalidApiUrl
}

public sealed record ServerProcessResult(
    ServerProcessState State,
    Uri EffectiveBaseUrl,
    bool OwnsProcess,
    ServerProcessError Error = ServerProcessError.None,
    string? Detail = null);

public sealed record ServerProcessOptions(
    string? ApiUrl = null,
    string? ExecutableDirectory = null,
    string ServerFileName = "usage-server.exe",
    string ExpectedVersion = "",
    int Port = 7823,
    int MaxReadinessAttempts = 8,
    int MaxRestartAttempts = 3);

public sealed record ServerBinaryRequest(string Path, string ExpectedVersion);

public sealed record ServerLaunchRequest(string ExecutablePath, int Port);

public interface IServerHealthProbe
{
    Task<bool> IsHealthyAsync(Uri baseUrl, string expectedVersion, CancellationToken cancellationToken);
}

public interface IServerBinaryVersionReader
{
    bool Matches(string path, string expectedVersion);
}

public interface IServerBinaryAcquirer
{
    Task<bool> AcquireAsync(ServerBinaryRequest request, CancellationToken cancellationToken);
}

public interface IServerProcessLauncher
{
    IManagedServerProcess Start(ServerLaunchRequest request);
}

public interface IManagedServerProcess : IDisposable
{
    int Id { get; }
    bool HasExited { get; }
    IntPtr Handle { get; }
    event EventHandler? Exited;
    Task<bool> WaitForExitAsync(TimeSpan timeout, CancellationToken cancellationToken);
    void Kill();
}

public interface IServerJob : IDisposable;

public interface IServerJobAssigner
{
    IServerJob Assign(IManagedServerProcess process);
}

public interface IServerDelay
{
    Task DelayAsync(TimeSpan delay, CancellationToken cancellationToken);
}

public sealed record ServerProcessDependencies(
    IServerHealthProbe HealthProbe,
    IServerBinaryVersionReader VersionReader,
    IServerBinaryAcquirer BinaryAcquirer,
    IServerProcessLauncher Launcher,
    IServerJobAssigner JobAssigner,
    IServerDelay Delay);

internal sealed class RealServerDelay : IServerDelay
{
    public Task DelayAsync(TimeSpan delay, CancellationToken cancellationToken) => Task.Delay(delay, cancellationToken);
}
