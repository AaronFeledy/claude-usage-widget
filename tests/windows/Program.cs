using ClaudeUsageWidget.Services;

var tests = new ServerProcessManagerTests();
await tests.Test_RemoteMode_when_ApiUrlConfigured();
await tests.Test_AttachFirst_when_DefaultHealthHealthy();
await tests.Test_AcquiresBinary_when_MissingBeforeSpawn();
await tests.Test_StartsOwned_when_HealthBecomesReady();
await tests.Test_CleansUpOwnedProcess_when_ReadinessFails();
await tests.Test_ConcurrentEnsure_when_LocalStartNeeded();
await tests.Test_RestartsOwnedProcess_when_ItExits();
await tests.Test_DoesNotRestart_when_Attached();
await tests.Test_Dispose_when_OwnedAndAttached();
await tests.Test_LauncherUsesListenAddrFlag_when_BuildingStartInfo();
await tests.Test_AssignerThrow_when_ProcessAlreadyLaunched();
await tests.Test_DisposeRace_when_ExitEventArrives();
await tests.Test_StaleExitEvent_when_ProcessWasRestartedManually();
await tests.Test_AcquisitionCleansTemp_when_DownloadFails();
await tests.Test_AcquisitionWritesVersionManifest_when_DownloadSucceeds();
await tests.Test_HealthProbeAcceptsHealthyServer_when_VersionDiffers();
await tests.Test_AcquisitionCancellation_when_BeforeLaunch();
await tests.Test_ReadinessCancellation_when_AfterLaunch();
await tests.Test_ReadinessException_when_AfterLaunch();
await tests.Test_AcquirerRethrowsCancellation_and_CleansTemp();
await tests.Test_RestartMonitorFailure_when_AcquirerThrows();
await tests.Test_RestartMonitorFailure_when_ReadinessThrows();
Console.WriteLine("ServerProcessManager tests passed");

internal sealed partial class ServerProcessManagerTests
{
    public async Task Test_RemoteMode_when_ApiUrlConfigured()
    {
        var deps = FakeDependencies.Create();
        await using var manager = new ServerProcessManager(new ServerProcessOptions(ApiUrl: "http://localhost:9000/api"), deps.Value);

        var result = await manager.EnsureStartedAsync();

        AssertEqual(ServerProcessState.Remote, result.State);
        AssertEqual("http://localhost:9000/api/", result.EffectiveBaseUrl.ToString());
        AssertEqual(0, deps.Health.Calls);
        AssertEqual(0, deps.Launcher.Starts);
        AssertEqual(0, deps.Acquirer.Calls);
    }

    public async Task Test_AttachFirst_when_DefaultHealthHealthy()
    {
        var deps = FakeDependencies.Create();
        deps.Health.Results.Enqueue(true);
        await using var manager = new ServerProcessManager(new ServerProcessOptions(), deps.Value);

        var result = await manager.EnsureStartedAsync();

        AssertEqual(ServerProcessState.Attached, result.State);
        AssertEqual(false, result.OwnsProcess);
        AssertEqual(true, manager.IsAttached);
        AssertEqual(0, deps.Launcher.Starts);
    }

    public async Task Test_AcquiresBinary_when_MissingBeforeSpawn()
    {
        var deps = FakeDependencies.Create();
        deps.Health.Results.Enqueue(false);
        deps.Health.Results.Enqueue(true);
        deps.Version.MatchesQueue.Enqueue(false);
        deps.Version.MatchesQueue.Enqueue(true);
        deps.Acquirer.Result = true;
        await using var manager = new ServerProcessManager(new ServerProcessOptions(ExecutableDirectory: "/tmp"), deps.Value);

        var result = await manager.EnsureStartedAsync();

        AssertEqual(ServerProcessState.Started, result.State);
        AssertEqual(1, deps.Acquirer.Calls);
        AssertEqual(1, deps.Launcher.Starts);
        AssertEqual("version,acquire,version,launch,job", string.Join(',', deps.Order));
    }

    public async Task Test_StartsOwned_when_HealthBecomesReady()
    {
        var deps = FakeDependencies.CreateReadyToLaunch();
        await using var manager = new ServerProcessManager(new ServerProcessOptions(), deps.Value);

        var result = await manager.EnsureStartedAsync();

        AssertEqual(ServerProcessState.Started, result.State);
        AssertEqual(true, result.OwnsProcess);
        AssertEqual(1, deps.Job.Assigns);
    }

    public async Task Test_CleansUpOwnedProcess_when_ReadinessFails()
    {
        var deps = FakeDependencies.Create();
        deps.Health.Results.Enqueue(false);
        deps.Version.MatchesQueue.Enqueue(true);
        await using var manager = new ServerProcessManager(new ServerProcessOptions(MaxReadinessAttempts: 2), deps.Value);

        var result = await manager.EnsureStartedAsync();

        AssertEqual(ServerProcessState.Failed, result.State);
        AssertEqual(ServerProcessError.ReadinessTimedOut, result.Error);
        AssertEqual(true, deps.Launcher.LastProcess?.Disposed);
        AssertEqual(true, deps.Job.LastJob?.Disposed);
    }

    public async Task Test_ConcurrentEnsure_when_LocalStartNeeded()
    {
        var deps = FakeDependencies.CreateReadyToLaunch();
        await using var manager = new ServerProcessManager(new ServerProcessOptions(), deps.Value);

        var results = await Task.WhenAll(manager.EnsureStartedAsync(), manager.EnsureStartedAsync(), manager.EnsureStartedAsync());

        AssertEqual(1, deps.Launcher.Starts);
        AssertEqual(3, results.Length);
        AssertEqual(true, results.All(result => result.State == ServerProcessState.Started));
    }

    public async Task Test_RestartsOwnedProcess_when_ItExits()
    {
        var deps = FakeDependencies.CreateReadyToLaunch();
        deps.Health.Results.Enqueue(true);
        deps.Launcher.SecondStarted = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        await using var manager = new ServerProcessManager(new ServerProcessOptions(), deps.Value);
        await manager.EnsureStartedAsync();

        deps.Launcher.LastProcess?.RaiseExited();
        await deps.Launcher.SecondStarted.Task.WaitAsync(TimeSpan.FromSeconds(1));

        AssertEqual(2, deps.Launcher.Starts);
    }

    public async Task Test_DoesNotRestart_when_Attached()
    {
        var deps = FakeDependencies.Create();
        deps.Health.Results.Enqueue(true);
        await using var manager = new ServerProcessManager(new ServerProcessOptions(), deps.Value);

        await manager.EnsureStartedAsync();
        await manager.RestartOwnedAsync();

        AssertEqual(0, deps.Launcher.Starts);
    }

    public async Task Test_Dispose_when_OwnedAndAttached()
    {
        var owned = FakeDependencies.CreateReadyToLaunch();
        await using (var manager = new ServerProcessManager(new ServerProcessOptions(), owned.Value))
        {
            await manager.EnsureStartedAsync();
        }
        AssertEqual(true, owned.Launcher.LastProcess?.Disposed);
        AssertEqual(true, owned.Job.LastJob?.Disposed);

        var attached = FakeDependencies.Create();
        attached.Health.Results.Enqueue(true);
        await using (var manager = new ServerProcessManager(new ServerProcessOptions(), attached.Value))
        {
            await manager.EnsureStartedAsync();
        }
        AssertEqual(0, attached.Launcher.Starts);
    }

    public Task Test_LauncherUsesListenAddrFlag_when_BuildingStartInfo()
    {
        var startInfo = ServerProcessLauncher.BuildStartInfo(new ServerLaunchRequest("usage-server.exe", 7823));

        AssertEqual("usage-server.exe", startInfo.FileName);
        AssertEqual(false, startInfo.UseShellExecute);
        AssertEqual(true, startInfo.CreateNoWindow);
        AssertEqual("--listen-addr", startInfo.ArgumentList[0]);
        AssertEqual("127.0.0.1:7823", startInfo.ArgumentList[1]);
        return Task.CompletedTask;
    }

    public async Task Test_AssignerThrow_when_ProcessAlreadyLaunched()
    {
        var deps = FakeDependencies.CreateReadyToLaunch();
        deps.Job.ThrowOnAssign = true;
        await using var manager = new ServerProcessManager(new ServerProcessOptions(), deps.Value);

        var result = await manager.EnsureStartedAsync();

        AssertEqual(ServerProcessState.Failed, result.State);
        AssertEqual(ServerProcessError.JobAssignmentFailed, result.Error);
        AssertEqual(true, deps.Launcher.LastProcess?.Killed);
        AssertEqual(true, deps.Launcher.LastProcess?.Disposed);
    }

    public async Task Test_DisposeRace_when_ExitEventArrives()
    {
        var deps = FakeDependencies.CreateReadyToLaunch();
        await using var manager = new ServerProcessManager(new ServerProcessOptions(), deps.Value);
        await manager.EnsureStartedAsync();

        var dispose = manager.DisposeAsync().AsTask();
        deps.Launcher.LastProcess?.RaiseExited();
        await dispose.WaitAsync(TimeSpan.FromSeconds(1));

        AssertEqual(1, deps.Launcher.Starts);
        var afterDispose = await manager.EnsureStartedAsync();
        AssertEqual(ServerProcessState.Failed, afterDispose.State);
        AssertEqual(ServerProcessError.Disposed, afterDispose.Error);
    }

    public async Task Test_StaleExitEvent_when_ProcessWasRestartedManually()
    {
        var deps = FakeDependencies.CreateReadyToLaunch();
        deps.Health.Results.Enqueue(true);
        await using var manager = new ServerProcessManager(new ServerProcessOptions(), deps.Value);
        await manager.EnsureStartedAsync();
        var oldProcess = deps.Launcher.LastProcess;

        await manager.RestartOwnedAsync();
        oldProcess?.RaiseExited();
        await Task.Delay(20);

        AssertEqual(2, deps.Launcher.Starts);
    }

    public async Task Test_AcquisitionCancellation_when_BeforeLaunch()
    {
        var deps = FakeDependencies.Create();
        deps.Health.Results.Enqueue(false);
        deps.Version.MatchesQueue.Enqueue(false);
        deps.Acquirer.ThrowCancellation = true;
        await using var manager = new ServerProcessManager(new ServerProcessOptions(), deps.Value);

        await AssertCanceled(() => manager.EnsureStartedAsync());

        AssertEqual(0, deps.Launcher.Starts);
    }

    public async Task Test_ReadinessCancellation_when_AfterLaunch()
    {
        var deps = FakeDependencies.Create();
        deps.Health.Results.Enqueue(false);
        deps.Health.ThrowCancellationAfterCalls = 2;
        using var cancellation = new CancellationTokenSource();
        deps.Health.CancelWhenThrowing = cancellation;
        deps.Version.MatchesQueue.Enqueue(true);
        await using var manager = new ServerProcessManager(new ServerProcessOptions(), deps.Value);

        await AssertCanceled(() => manager.EnsureStartedAsync(cancellation.Token));

        AssertEqual(true, deps.Launcher.LastProcess?.Killed);
        AssertEqual(true, deps.Launcher.LastProcess?.Disposed);
        AssertEqual(true, deps.Job.LastJob?.Disposed);
        AssertEqual(1, deps.Launcher.Starts);
    }

    public async Task Test_ReadinessException_when_AfterLaunch()
    {
        var deps = FakeDependencies.Create();
        deps.Health.Results.Enqueue(false);
        deps.Health.ThrowAfterCalls = 2;
        deps.Version.MatchesQueue.Enqueue(true);
        await using var manager = new ServerProcessManager(new ServerProcessOptions(), deps.Value);

        var result = await manager.EnsureStartedAsync();

        AssertEqual(ServerProcessState.Failed, result.State);
        AssertEqual(ServerProcessError.ReadinessFailed, result.Error);
        AssertEqual(true, deps.Launcher.LastProcess?.Killed);
        AssertEqual(true, deps.Launcher.LastProcess?.Disposed);
        AssertEqual(true, deps.Job.LastJob?.Disposed);
    }

    private static async Task AssertCanceled(Func<Task> action)
    {
        try
        {
            await action();
        }
        catch (OperationCanceledException)
        {
            return;
        }
        throw new InvalidOperationException("Expected cancellation");
    }

    private static void AssertEqual<T>(T expected, T actual)
    {
        if (!EqualityComparer<T>.Default.Equals(expected, actual))
        {
            throw new InvalidOperationException($"Expected {expected}, got {actual}");
        }
    }
}
