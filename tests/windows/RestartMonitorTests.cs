using ClaudeUsageWidget.Services;

internal sealed partial class ServerProcessManagerTests
{
    public async Task Test_RestartMonitorFailure_when_AcquirerThrows()
    {
        var deps = FakeDependencies.CreateReadyToLaunch();
        await using var manager = new ServerProcessManager(new ServerProcessOptions(), deps.Value);
        await manager.EnsureStartedAsync();
        deps.Version.MatchesQueue.Enqueue(false);
        deps.Acquirer.ThrowException = true;

        deps.Launcher.LastProcess?.RaiseExited();
        await deps.Acquirer.CalledAgain.Task.WaitAsync(TimeSpan.FromSeconds(1));
        var result = await manager.EnsureStartedAsync();

        AssertEqual(ServerProcessState.Failed, result.State);
        AssertEqual(ServerProcessError.RestartFailed, result.Error);
        AssertEqual(1, deps.Launcher.Starts);
        await manager.DisposeAsync();
        await manager.DisposeAsync();
    }

    public async Task Test_RestartMonitorFailure_when_ReadinessThrows()
    {
        var deps = FakeDependencies.CreateReadyToLaunch();
        deps.Health.ThrowAfterCalls = 3;
        await using var manager = new ServerProcessManager(new ServerProcessOptions(), deps.Value);
        await manager.EnsureStartedAsync();

        deps.Launcher.LastProcess?.RaiseExited();
        await deps.Health.ThrowObserved.Task.WaitAsync(TimeSpan.FromSeconds(1));
        var result = await manager.EnsureStartedAsync();

        AssertEqual(ServerProcessState.Failed, result.State);
        AssertEqual(ServerProcessError.RestartFailed, result.Error);
        AssertEqual(2, deps.Launcher.Starts);
        AssertEqual(true, deps.Launcher.LastProcess?.Killed);
        AssertEqual(true, deps.Launcher.LastProcess?.Disposed);
        AssertEqual(true, deps.Job.LastJob?.Disposed);
        await manager.DisposeAsync();
        await manager.DisposeAsync();
    }
}
