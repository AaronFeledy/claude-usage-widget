using System.Diagnostics;
using System.Net.Http.Json;
using System.Text.Json.Serialization;

namespace ClaudeUsageWidget.Services;

public sealed class ServerHealthProbe : IServerHealthProbe
{
    private readonly HttpClient _httpClient;

    public ServerHealthProbe(HttpClient httpClient)
    {
        _httpClient = httpClient;
    }

    public async Task<bool> IsHealthyAsync(Uri baseUrl, string expectedVersion, CancellationToken cancellationToken)
    {
        try
        {
            var healthUrl = new Uri(baseUrl, "api/v1/health");
            var health = await _httpClient.GetFromJsonAsync<ServerHealth>(healthUrl, cancellationToken).ConfigureAwait(false);
            // Reachability != provider health: "degraded" still means the server is up and
            // serving. Requiring "ok" would reject a live server while any provider is erroring
            // or mid-first-poll; those are surfaced separately via usage data.
            return health is not null && !string.IsNullOrWhiteSpace(health.Status);
        }
        catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested)
        {
            throw;
        }
        catch
        {
            return false;
        }
    }
}

public sealed class SidecarServerBinaryVersionReader : IServerBinaryVersionReader
{
    public bool Matches(string path, string expectedVersion)
    {
        if (!File.Exists(path))
        {
            return false;
        }

        if (string.IsNullOrWhiteSpace(expectedVersion))
        {
            return true;
        }

        var sidecar = VersionPath(path);
        return File.Exists(sidecar)
            && string.Equals(File.ReadAllText(sidecar).Trim(), expectedVersion.Trim(), StringComparison.OrdinalIgnoreCase);
    }

    internal static string VersionPath(string path) => path + ".version";
}

public sealed class ServerProcessLauncher : IServerProcessLauncher
{
    public IManagedServerProcess Start(ServerLaunchRequest request)
    {
        var process = Process.Start(BuildStartInfo(request)) ?? throw new InvalidOperationException("usage-server did not start");
        process.EnableRaisingEvents = true;
        return new ManagedServerProcess(process);
    }

    internal static ProcessStartInfo BuildStartInfo(ServerLaunchRequest request)
    {
        var startInfo = new ProcessStartInfo
        {
            FileName = request.ExecutablePath,
            UseShellExecute = false,
            CreateNoWindow = true,
            WindowStyle = ProcessWindowStyle.Hidden
        };
        startInfo.ArgumentList.Add("--listen-addr");
        startInfo.ArgumentList.Add($"127.0.0.1:{request.Port}");
        return startInfo;
    }
}

internal sealed class ManagedServerProcess : IManagedServerProcess
{
    private readonly Process _process;
    private event EventHandler? ExitedHandlers;

    public ManagedServerProcess(Process process)
    {
        _process = process;
        _process.Exited += (_, _) => ExitedHandlers?.Invoke(this, EventArgs.Empty);
    }

    public int Id => _process.Id;
    public bool HasExited => _process.HasExited;
    public IntPtr Handle => _process.Handle;
    public event EventHandler? Exited
    {
        add => ExitedHandlers += value;
        remove => ExitedHandlers -= value;
    }

    public async Task<bool> WaitForExitAsync(TimeSpan timeout, CancellationToken cancellationToken)
    {
        var waitTask = _process.WaitForExitAsync(cancellationToken);
        var delayTask = Task.Delay(timeout, cancellationToken);
        return await Task.WhenAny(waitTask, delayTask).ConfigureAwait(false) == waitTask;
    }

    public void Kill()
    {
        if (!_process.HasExited)
        {
            _process.Kill(entireProcessTree: true);
        }
    }

    public void Dispose() => _process.Dispose();
}

internal sealed class ServerHealth
{
    [JsonPropertyName("status")]
    public string? Status { get; set; }

    [JsonPropertyName("version")]
    public string? Version { get; set; }
}
