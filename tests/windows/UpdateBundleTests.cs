using ClaudeUsageWidget.Services;

internal sealed partial class ServerProcessManagerTests
{
    public async Task Test_UpdateSwapsAppAndServer_when_BothUpdatesExist()
    {
        using var fixture = UpdateFixture.Create();

        var applied = await PendingUpdateApplicator.ApplyAsync(fixture.Options, CancellationToken.None);

        AssertEqual(true, applied);
        AssertEqual("app-new", File.ReadAllText(fixture.AppPath));
        AssertEqual("server-new", File.ReadAllText(fixture.ServerPath));
        AssertEqual(1, fixture.Starter.Starts);
    }

    public async Task Test_UpdateStopsOwnedServer_before_ServerSwap()
    {
        using var fixture = UpdateFixture.Create();

        var applied = await PendingUpdateApplicator.ApplyAsync(fixture.Options, CancellationToken.None);

        AssertEqual(true, applied);
        AssertEqual("stop,move:ClaudeUsageWidget.exe->ClaudeUsageWidget.old.exe,move:ClaudeUsageWidget.update.exe->ClaudeUsageWidget.exe,move:usage-server.exe->usage-server.old.exe,move:usage-server.update.exe->usage-server.exe,start", string.Join(',', fixture.Order));
    }

    public async Task Test_UpdateRollsBackBothBinaries_when_ServerSwapFails()
    {
        using var fixture = UpdateFixture.Create();
        fixture.Files.FailMovingServerUpdate = true;

        var applied = await PendingUpdateApplicator.ApplyAsync(fixture.Options, CancellationToken.None);

        AssertEqual(false, applied);
        AssertEqual("app-old", File.ReadAllText(fixture.AppPath));
        AssertEqual("server-old", File.ReadAllText(fixture.ServerPath));
        AssertEqual(0, fixture.Starter.Starts);
    }

    public async Task Test_UpdateKeepsServer_when_OnlyTrayUpdateExists()
    {
        using var fixture = UpdateFixture.Create();
        File.Delete(fixture.ServerUpdatePath);

        var applied = await PendingUpdateApplicator.ApplyAsync(fixture.Options, CancellationToken.None);

        AssertEqual(true, applied);
        AssertEqual("app-new", File.ReadAllText(fixture.AppPath));
        AssertEqual("server-old", File.ReadAllText(fixture.ServerPath));
        AssertEqual("move:ClaudeUsageWidget.exe->ClaudeUsageWidget.old.exe,move:ClaudeUsageWidget.update.exe->ClaudeUsageWidget.exe,start", string.Join(',', fixture.Order));
    }
}

internal sealed class UpdateFixture : IDisposable
{
    private readonly DirectoryInfo _directory;

    private UpdateFixture(DirectoryInfo directory)
    {
        _directory = directory;
        AppPath = Path.Combine(directory.FullName, "ClaudeUsageWidget.exe");
        ServerPath = Path.Combine(directory.FullName, "usage-server.exe");
        Order = new List<string>();
        Files = new RecordingUpdateFileSystem(Order);
        Starter = new RecordingProcessStarter(Order);
        Options = new PendingUpdateOptions(AppPath, ServerPath, new RecordingServerStopper(Order), Starter, Files);
    }

    public string AppPath { get; }
    public string ServerPath { get; }
    public string ServerUpdatePath => Path.Combine(_directory.FullName, "usage-server.update.exe");
    public List<string> Order { get; }
    public RecordingUpdateFileSystem Files { get; }
    public RecordingProcessStarter Starter { get; }
    public PendingUpdateOptions Options { get; }

    public static UpdateFixture Create()
    {
        var fixture = new UpdateFixture(Directory.CreateTempSubdirectory());
        File.WriteAllText(fixture.AppPath, "app-old");
        File.WriteAllText(fixture.ServerPath, "server-old");
        File.WriteAllText(Path.Combine(fixture._directory.FullName, "ClaudeUsageWidget.update.exe"), "app-new");
        File.WriteAllText(fixture.ServerUpdatePath, "server-new");
        return fixture;
    }

    public void Dispose() => _directory.Delete(recursive: true);
}

internal sealed class RecordingServerStopper : IPendingUpdateServerStopper
{
    private readonly List<string> _order;
    public RecordingServerStopper(List<string> order) => _order = order;
    public Task StopOwnedServerAsync(CancellationToken cancellationToken)
    {
        _order.Add("stop");
        return Task.CompletedTask;
    }
}

internal sealed class RecordingProcessStarter : IUpdateProcessStarter
{
    private readonly List<string> _order;
    public int Starts { get; private set; }
    public RecordingProcessStarter(List<string> order) => _order = order;
    public void Start(string executablePath)
    {
        Starts++;
        _order.Add("start");
    }
}

internal sealed class RecordingUpdateFileSystem : IUpdateFileSystem
{
    private readonly List<string> _order;
    public bool FailMovingServerUpdate { get; set; }
    public RecordingUpdateFileSystem(List<string> order) => _order = order;
    public bool Exists(string path) => File.Exists(path);
    public void DeleteIfExists(string path) { if (File.Exists(path)) File.Delete(path); }
    public void Move(string source, string destination, bool overwrite)
    {
        _order.Add($"move:{Path.GetFileName(source)}->{Path.GetFileName(destination)}");
        if (FailMovingServerUpdate && Path.GetFileName(source) == "usage-server.update.exe")
        {
            throw new IOException("injected server swap failure");
        }
        File.Move(source, destination, overwrite);
    }
}
